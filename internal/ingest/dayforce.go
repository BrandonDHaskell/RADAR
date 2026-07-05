package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DayforceFetcher implements Fetcher for the Dayforce (Ceridian) HCM job
// feed API.
type DayforceFetcher struct {
	client *Client
}

// NewDayforceFetcher returns a DayforceFetcher using client for requests.
func NewDayforceFetcher(client *Client) *DayforceFetcher {
	return &DayforceFetcher{client: client}
}

// Source returns "dayforce".
func (f *DayforceFetcher) Source() string { return "dayforce" }

type dayforceJob struct {
	Title               string   `json:"Title"`
	Description         string   `json:"Description"`
	City                string   `json:"City"`
	State               string   `json:"State"`
	Country             string   `json:"Country"`
	JobFunction         string   `json:"JobFunction"`
	EmploymentIndicator string   `json:"EmploymentIndicator"`
	DatePosted          string   `json:"DatePosted"`
	LastUpdated         string   `json:"LastUpdated"`
	ReferenceNumber     int64    `json:"ReferenceNumber"`
	JobDetailsUrl       string   `json:"JobDetailsUrl"`
	ApplyUrl            string   `json:"ApplyUrl"`
	MinHiringRate       *float64 `json:"MinHiringRate"`
	MaxHiringRate       *float64 `json:"MaxHiringRate"`
	IsVirtualLocation   bool     `json:"IsVirtualLocation"`
}

// dayforceTimeLayout matches Dayforce's LastUpdated/DatePosted timestamps,
// e.g. "2026-06-18T10:24:59.627" or "2023-08-22T00:00:00": no timezone
// offset, and a fractional-second suffix of variable length (including
// none). The repeated 9s make that suffix optional on parse.
const dayforceTimeLayout = "2006-01-02T15:04:05.999999999"

// Fetch pulls all active postings for a single Dayforce company name, e.g.
// https://www.dayforcehcm.com/api/{atsToken}/V1/JobFeeds. Dayforce redirects
// this canonical host to the tenant's own branded career-site domain when
// one is configured (e.g. www.mydayforce.com); Client follows redirects
// transparently, so no special handling is needed here.
func (f *DayforceFetcher) Fetch(ctx context.Context, atsToken string) ([]NormalizedPosting, error) {
	reqURL := fmt.Sprintf("https://www.dayforcehcm.com/api/%s/V1/JobFeeds?includeActivePostingOnly=true", url.PathEscape(atsToken))
	body, err := f.client.Get(ctx, reqURL)
	if err != nil {
		return nil, fmt.Errorf("fetching dayforce company %q: %w", atsToken, err)
	}

	var jobs []dayforceJob
	if err := json.Unmarshal(body, &jobs); err != nil {
		return nil, fmt.Errorf("parsing dayforce company %q: %w", atsToken, err)
	}

	postings := make([]NormalizedPosting, 0, len(jobs))
	for _, job := range jobs {
		postings = append(postings, normalizeDayforceJob(job))
	}
	return postings, nil
}

func normalizeDayforceJob(job dayforceJob) NormalizedPosting {
	location := strings.Join(nonEmpty(job.City, job.State, job.Country), ", ")

	updatedAt, err := time.Parse(dayforceTimeLayout, job.LastUpdated)
	if err != nil {
		updatedAt, err = time.Parse(dayforceTimeLayout, job.DatePosted)
	}
	var sourceUpdatedAt *time.Time
	if err == nil {
		sourceUpdatedAt = &updatedAt
	}

	return NormalizedPosting{
		ExternalID:     strconv.FormatInt(job.ReferenceNumber, 10),
		Title:          job.Title,
		Location:       location,
		IsRemote:       job.IsVirtualLocation,
		Department:     job.JobFunction,
		EmploymentType: job.EmploymentIndicator,
		SalaryMin:      job.MinHiringRate,
		SalaryMax:      job.MaxHiringRate,
		// No currency field exists in Dayforce's job feed schema.
		Description:     job.Description,
		ApplyURL:        job.ApplyUrl,
		SourceURL:       job.JobDetailsUrl,
		SourceUpdatedAt: sourceUpdatedAt,
	}
}
