package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// LeverFetcher implements Fetcher for the Lever job board API.
type LeverFetcher struct {
	client *Client
}

// NewLeverFetcher returns a LeverFetcher using client for requests.
func NewLeverFetcher(client *Client) *LeverFetcher {
	return &LeverFetcher{client: client}
}

// Source returns "lever".
func (f *LeverFetcher) Source() string { return "lever" }

type leverJob struct {
	ID               string             `json:"id"`
	Text             string             `json:"text"`
	HostedURL        string             `json:"hostedUrl"`
	ApplyURL         string             `json:"applyUrl"`
	CreatedAt        int64              `json:"createdAt"`
	Country          string             `json:"country"`
	WorkplaceType    string             `json:"workplaceType"`
	DescriptionPlain string             `json:"descriptionPlain"`
	AdditionalPlain  string             `json:"additionalPlain"`
	Categories       leverCategories    `json:"categories"`
	Lists            []leverDescription `json:"lists"`
}

type leverCategories struct {
	Commitment   string   `json:"commitment"`
	Location     string   `json:"location"`
	Team         string   `json:"team"`
	AllLocations []string `json:"allLocations"`
}

type leverDescription struct {
	Text    string `json:"text"`
	Content string `json:"content"`
}

// Fetch pulls all published postings for a single Lever site token, e.g.
// https://api.lever.co/v0/postings/{atsToken}?mode=json.
func (f *LeverFetcher) Fetch(ctx context.Context, atsToken string) ([]NormalizedPosting, error) {
	reqURL := fmt.Sprintf("https://api.lever.co/v0/postings/%s?mode=json", url.PathEscape(atsToken))
	body, err := f.client.Get(ctx, reqURL)
	if err != nil {
		return nil, fmt.Errorf("fetching lever board %q: %w", atsToken, err)
	}

	var jobs []leverJob
	if err := json.Unmarshal(body, &jobs); err != nil {
		return nil, fmt.Errorf("parsing lever board %q: %w", atsToken, err)
	}

	postings := make([]NormalizedPosting, 0, len(jobs))
	for _, job := range jobs {
		postings = append(postings, normalizeLeverJob(job))
	}
	return postings, nil
}

func normalizeLeverJob(job leverJob) NormalizedPosting {
	location := job.Categories.Location

	var updatedAt *time.Time
	if job.CreatedAt > 0 {
		t := time.UnixMilli(job.CreatedAt).UTC()
		updatedAt = &t
	}

	// Lever splits a posting's body across an intro (descriptionPlain, which
	// already includes the "opening" section), zero or more structured
	// sections (lists, HTML content with no plain-text counterpart), and a
	// trailing free-form section (additionalPlain, often benefits/salary
	// text). Concatenate all three in display order for the full text.
	parts := make([]string, 0, len(job.Lists)+2)
	if job.DescriptionPlain != "" {
		parts = append(parts, job.DescriptionPlain)
	}
	for _, l := range job.Lists {
		var section string
		if l.Text != "" {
			section = l.Text + "\n" + htmlToText(l.Content)
		} else {
			section = htmlToText(l.Content)
		}
		if strings.TrimSpace(section) != "" {
			parts = append(parts, section)
		}
	}
	if job.AdditionalPlain != "" {
		parts = append(parts, job.AdditionalPlain)
	}

	return NormalizedPosting{
		ExternalID:      job.ID,
		Title:           job.Text,
		Location:        location,
		IsRemote:        job.WorkplaceType == "remote" || strings.Contains(strings.ToLower(location), "remote"),
		Department:      job.Categories.Team,
		EmploymentType:  job.Categories.Commitment,
		Description:     strings.Join(parts, "\n\n"),
		ApplyURL:        job.ApplyURL,
		SourceURL:       job.HostedURL,
		SourceUpdatedAt: updatedAt,
	}
}
