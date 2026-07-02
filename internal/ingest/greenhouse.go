package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// GreenhouseFetcher implements Fetcher for the Greenhouse job board API.
type GreenhouseFetcher struct {
	client *Client
}

// NewGreenhouseFetcher returns a GreenhouseFetcher using client for requests.
func NewGreenhouseFetcher(client *Client) *GreenhouseFetcher {
	return &GreenhouseFetcher{client: client}
}

// Source returns "greenhouse".
func (f *GreenhouseFetcher) Source() string { return "greenhouse" }

type greenhouseResponse struct {
	Jobs []greenhouseJob `json:"jobs"`
}

type greenhouseJob struct {
	ID          int64             `json:"id"`
	Title       string            `json:"title"`
	UpdatedAt   string            `json:"updated_at"`
	Content     string            `json:"content"`
	AbsoluteURL string            `json:"absolute_url"`
	Location    greenhouseNamed   `json:"location"`
	Departments []greenhouseNamed `json:"departments"`
}

type greenhouseNamed struct {
	Name string `json:"name"`
}

// Fetch pulls all published postings for a single Greenhouse board token,
// e.g. https://boards-api.greenhouse.io/v1/boards/{atsToken}/jobs?content=true.
func (f *GreenhouseFetcher) Fetch(ctx context.Context, atsToken string) ([]NormalizedPosting, error) {
	reqURL := fmt.Sprintf("https://boards-api.greenhouse.io/v1/boards/%s/jobs?content=true", url.PathEscape(atsToken))
	body, err := f.client.Get(ctx, reqURL)
	if err != nil {
		return nil, fmt.Errorf("fetching greenhouse board %q: %w", atsToken, err)
	}

	var parsed greenhouseResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parsing greenhouse board %q: %w", atsToken, err)
	}

	postings := make([]NormalizedPosting, 0, len(parsed.Jobs))
	for _, job := range parsed.Jobs {
		postings = append(postings, normalizeGreenhouseJob(job))
	}
	return postings, nil
}

func normalizeGreenhouseJob(job greenhouseJob) NormalizedPosting {
	var department string
	if len(job.Departments) > 0 {
		department = job.Departments[0].Name
	}

	var updatedAt *time.Time
	if t, err := time.Parse(time.RFC3339, job.UpdatedAt); err == nil {
		updatedAt = &t
	}

	return NormalizedPosting{
		ExternalID: strconv.FormatInt(job.ID, 10),
		Title:      job.Title,
		Location:   job.Location.Name,
		IsRemote:   strings.Contains(strings.ToLower(job.Location.Name), "remote"),
		Department: department,
		// Greenhouse returns content HTML-entity-escaped, e.g. "&lt;p&gt;...".
		// One UnescapeString undoes that encoding to real HTML, then
		// htmlToText flattens the HTML itself into plain text.
		Description:     htmlToText(html.UnescapeString(job.Content)),
		ApplyURL:        job.AbsoluteURL,
		SourceURL:       job.AbsoluteURL,
		SourceUpdatedAt: updatedAt,
	}
}
