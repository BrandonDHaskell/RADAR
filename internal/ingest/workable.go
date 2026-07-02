package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"strings"
	"time"
)

// WorkableFetcher implements Fetcher for the Workable job board API.
type WorkableFetcher struct {
	client *Client
}

// NewWorkableFetcher returns a WorkableFetcher using client for requests.
func NewWorkableFetcher(client *Client) *WorkableFetcher {
	return &WorkableFetcher{client: client}
}

// Source returns "workable".
func (f *WorkableFetcher) Source() string { return "workable" }

type workableResponse struct {
	Jobs []workableJob `json:"jobs"`
}

type workableJob struct {
	Title          string `json:"title"`
	Shortcode      string `json:"shortcode"`
	EmploymentType string `json:"employment_type"`
	Telecommuting  bool   `json:"telecommuting"`
	Department     string `json:"department"`
	URL            string `json:"url"`
	ApplicationURL string `json:"application_url"`
	PublishedOn    string `json:"published_on"`
	CreatedAt      string `json:"created_at"`
	Country        string `json:"country"`
	City           string `json:"city"`
	State          string `json:"state"`
	Description    string `json:"description"`
}

// Fetch pulls all published postings for a single Workable account
// subdomain, e.g.
// https://apply.workable.com/api/v1/widget/accounts/{atsToken}?details=true.
// Note this is not the endpoint documented at workable.com/api/accounts,
// which now 302-redirects here.
func (f *WorkableFetcher) Fetch(ctx context.Context, atsToken string) ([]NormalizedPosting, error) {
	reqURL := fmt.Sprintf("https://apply.workable.com/api/v1/widget/accounts/%s?details=true", url.PathEscape(atsToken))
	body, err := f.client.Get(ctx, reqURL)
	if err != nil {
		return nil, fmt.Errorf("fetching workable account %q: %w", atsToken, err)
	}

	var parsed workableResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parsing workable account %q: %w", atsToken, err)
	}

	postings := make([]NormalizedPosting, 0, len(parsed.Jobs))
	for _, job := range parsed.Jobs {
		postings = append(postings, normalizeWorkableJob(job))
	}
	return postings, nil
}

// workableDateLayout is the date-only (no time-of-day) format Workable uses
// for published_on and created_at, e.g. "2026-06-22".
const workableDateLayout = "2006-01-02"

func normalizeWorkableJob(job workableJob) NormalizedPosting {
	location := strings.Join(nonEmpty(job.City, job.State, job.Country), ", ")

	dateStr := job.PublishedOn
	if dateStr == "" {
		dateStr = job.CreatedAt
	}
	var updatedAt *time.Time
	if t, err := time.Parse(workableDateLayout, dateStr); err == nil {
		updatedAt = &t
	}

	sourceURL := job.URL
	if sourceURL == "" {
		sourceURL = job.ApplicationURL
	}

	return NormalizedPosting{
		ExternalID:      job.Shortcode,
		Title:           job.Title,
		Location:        location,
		IsRemote:        job.Telecommuting,
		Department:      job.Department,
		EmploymentType:  job.EmploymentType,
		Description:     htmlToText(html.UnescapeString(job.Description)),
		ApplyURL:        job.ApplicationURL,
		SourceURL:       sourceURL,
		SourceUpdatedAt: updatedAt,
	}
}

func nonEmpty(vals ...string) []string {
	out := make([]string, 0, len(vals))
	for _, v := range vals {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}
