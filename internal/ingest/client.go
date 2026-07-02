package ingest

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"golang.org/x/time/rate"
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
// (network errors, 429, and 5xx responses) with exponential backoff.
func (c *Client) Get(ctx context.Context, url string) ([]byte, error) {
	const maxAttempts = 4
	backoff := 500 * time.Millisecond

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := c.limiter.Wait(ctx); err != nil {
			return nil, err
		}

		body, retryable, err := c.doGet(ctx, url)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retryable || attempt == maxAttempts {
			break
		}

		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		backoff *= 2
	}
	return nil, fmt.Errorf("GET %s: %w", url, lastErr)
}

// doGet performs a single request attempt. retryable indicates whether a
// failure is worth retrying (network errors, 429, 5xx) as opposed to a
// permanent failure (4xx other than 429).
func (c *Client) doGet(ctx context.Context, url string) (body []byte, retryable bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", "radar-job-search/0.1 (single-user personal job search tool)")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer resp.Body.Close()

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, err
	}

	switch {
	case resp.StatusCode == http.StatusOK:
		return body, false, nil
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		return nil, true, fmt.Errorf("status %d", resp.StatusCode)
	default:
		return nil, false, fmt.Errorf("status %d", resp.StatusCode)
	}
}
