package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"
)

// AshbyFetcher implements Fetcher for the Ashby job board API.
type AshbyFetcher struct {
	client *Client
}

// NewAshbyFetcher returns an AshbyFetcher using client for requests.
func NewAshbyFetcher(client *Client) *AshbyFetcher {
	return &AshbyFetcher{client: client}
}

// Source returns "ashby".
func (f *AshbyFetcher) Source() string { return "ashby" }

type ashbyResponse struct {
	Jobs []ashbyJob `json:"jobs"`
}

type ashbyJob struct {
	ID               string            `json:"id"`
	Title            string            `json:"title"`
	Department       string            `json:"department"`
	EmploymentType   string            `json:"employmentType"`
	Location         string            `json:"location"`
	PublishedAt      string            `json:"publishedAt"`
	IsRemote         bool              `json:"isRemote"`
	JobURL           string            `json:"jobUrl"`
	ApplyURL         string            `json:"applyUrl"`
	DescriptionHTML  string            `json:"descriptionHtml"`
	DescriptionPlain string            `json:"descriptionPlain"`
	Compensation     ashbyCompensation `json:"compensation"`
}

type ashbyCompensation struct {
	CompensationTiers []ashbyCompensationTier `json:"compensationTiers"`
}

type ashbyCompensationTier struct {
	Components []ashbyCompensationComponent `json:"components"`
}

type ashbyCompensationComponent struct {
	CompensationType string   `json:"compensationType"`
	CurrencyCode     string   `json:"currencyCode"`
	MinValue         *float64 `json:"minValue"`
	MaxValue         *float64 `json:"maxValue"`
}

// Fetch pulls all published postings for a single Ashby job board name, e.g.
// https://api.ashbyhq.com/posting-api/job-board/{atsToken}?includeCompensation=true.
func (f *AshbyFetcher) Fetch(ctx context.Context, atsToken string) ([]NormalizedPosting, error) {
	reqURL := fmt.Sprintf("https://api.ashbyhq.com/posting-api/job-board/%s?includeCompensation=true", url.PathEscape(atsToken))
	body, err := f.client.Get(ctx, reqURL)
	if err != nil {
		return nil, fmt.Errorf("fetching ashby board %q: %w", atsToken, err)
	}

	var parsed ashbyResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parsing ashby board %q: %w", atsToken, err)
	}

	postings := make([]NormalizedPosting, 0, len(parsed.Jobs))
	for _, job := range parsed.Jobs {
		postings = append(postings, normalizeAshbyJob(job))
	}
	return postings, nil
}

func normalizeAshbyJob(job ashbyJob) NormalizedPosting {
	description := job.DescriptionPlain
	if description == "" {
		description = htmlToText(job.DescriptionHTML)
	}

	var updatedAt *time.Time
	if t, err := time.Parse(time.RFC3339, job.PublishedAt); err == nil {
		updatedAt = &t
	}

	salaryMin, salaryMax, salaryCurrency := ashbySalaryRange(job.Compensation)

	return NormalizedPosting{
		ExternalID:      job.ID,
		Title:           job.Title,
		Location:        job.Location,
		IsRemote:        job.IsRemote,
		Department:      job.Department,
		EmploymentType:  job.EmploymentType,
		SalaryMin:       salaryMin,
		SalaryMax:       salaryMax,
		SalaryCurrency:  salaryCurrency,
		Description:     description,
		ApplyURL:        job.ApplyURL,
		SourceURL:       job.JobURL,
		SourceUpdatedAt: updatedAt,
	}
}

// ashbySalaryRange finds the widest advertised base salary range across all
// compensation tiers a posting lists (e.g. separate SF/NY vs. nationwide
// tiers), taking the currency from the first salary component found.
func ashbySalaryRange(comp ashbyCompensation) (min, max *float64, currency string) {
	for _, tier := range comp.CompensationTiers {
		for _, c := range tier.Components {
			if c.CompensationType != "Salary" {
				continue
			}
			if currency == "" {
				currency = c.CurrencyCode
			}
			if c.MinValue != nil && (min == nil || *c.MinValue < *min) {
				min = c.MinValue
			}
			if c.MaxValue != nil && (max == nil || *c.MaxValue > *max) {
				max = c.MaxValue
			}
		}
	}
	return min, max, currency
}
