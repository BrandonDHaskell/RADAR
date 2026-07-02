package ingest

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"golang.org/x/time/rate"
)

const (
	maxAttempts = 4
	// maxResponseBytes caps how much of a response body we'll buffer, so a
	// misbehaving or compromised endpoint can't exhaust memory. Legitimate
	// ATS job-board responses are well under this.
	maxResponseBytes = 20 << 20 // 20MB
	maxRetryAfter    = 30 * time.Second
)

// Client is a shared HTTP client used by every ATS adapter: it applies a
// polite rate limit and retries transient failures with backoff, so
// individual adapters don't each reimplement that behavior.
type Client struct {
	http    *http.Client
	limiter *rate.Limiter
}

// NewClient returns a Client that allows at most rps requests per second
// (bursting up to burst) and times out each request after timeout.
func NewClient(rps float64, burst int, timeout time.Duration) *Client {
	return &Client{
		http:    &http.Client{Timeout: timeout},
		limiter: rate.NewLimiter(rate.Limit(rps), burst),
	}
}

// Get performs a rate-limited GET request, retrying transient failures
// (network errors, 429, and 5xx responses) with exponential backoff. On a
// 429 with a valid Retry-After header, it sleeps that long instead of the
// computed backoff, capped at 30 seconds.
func (c *Client) Get(ctx context.Context, url string) ([]byte, error) {
	backoff := 500 * time.Millisecond

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := c.limiter.Wait(ctx); err != nil {
			return nil, err
		}

		body, retryable, retryAfter, err := c.doGet(ctx, url)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retryable || attempt == maxAttempts {
			break
		}

		sleep := backoff
		if retryAfter > 0 {
			sleep = retryAfter
		}
		select {
		case <-time.After(sleep):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		backoff *= 2
	}
	return nil, fmt.Errorf("GET %s: %w", url, lastErr)
}

// doGet performs a single request attempt. retryable indicates whether a
// failure is worth retrying (network errors, 429, 5xx) as opposed to a
// permanent failure (4xx other than 429, or an oversized response).
// retryAfter is non-zero only when the server sent a usable Retry-After
// header on a 429.
func (c *Client) doGet(ctx context.Context, url string) (body []byte, retryable bool, retryAfter time.Duration, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, 0, err
	}
	req.Header.Set("User-Agent", "radar-job-search/0.1 (single-user personal job search tool)")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, true, 0, err
	}
	defer resp.Body.Close()

	// Read one byte past the cap so we can tell "exactly at the cap" from
	// "over the cap" without buffering an unbounded response.
	body, err = io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, true, 0, err
	}
	if len(body) > maxResponseBytes {
		return nil, false, 0, fmt.Errorf("response exceeds %dMB", maxResponseBytes>>20)
	}

	switch {
	case resp.StatusCode == http.StatusOK:
		return body, false, 0, nil
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, true, parseRetryAfter(resp.Header.Get("Retry-After")), fmt.Errorf("status %d", resp.StatusCode)
	case resp.StatusCode >= 500:
		return nil, true, 0, fmt.Errorf("status %d", resp.StatusCode)
	default:
		return nil, false, 0, fmt.Errorf("status %d", resp.StatusCode)
	}
}

// parseRetryAfter reads a Retry-After header expressed in seconds (the
// HTTP-date form is not handled), capped at 30 seconds. It returns 0 if the
// header is absent or not a valid non-negative integer.
func parseRetryAfter(header string) time.Duration {
	if header == "" {
		return 0
	}
	seconds, err := strconv.Atoi(header)
	if err != nil || seconds < 0 {
		return 0
	}
	d := time.Duration(seconds) * time.Second
	if d > maxRetryAfter {
		d = maxRetryAfter
	}
	return d
}
