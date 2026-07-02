// Package ingest fetches job postings from public ATS job-board APIs. Each
// adapter only fetches and shapes raw data into NormalizedPosting; canonical
// key/content hash computation, dedup, and persistence live in
// internal/normalize, internal/dedup, and internal/store.
package ingest

import (
	"context"
	"time"
)

// NormalizedPosting is the common shape every ATS adapter produces.
type NormalizedPosting struct {
	ExternalID      string
	Title           string
	Location        string
	IsRemote        bool
	Department      string
	EmploymentType  string
	SalaryMin       *float64
	SalaryMax       *float64
	SalaryCurrency  string
	Description     string
	ApplyURL        string
	SourceURL       string
	SourceUpdatedAt *time.Time
}

// Fetcher is implemented once per ATS: Greenhouse, Lever, Ashby, Workable.
type Fetcher interface {
	// Source returns the ATS identifier, e.g. "greenhouse".
	Source() string
	// Fetch pulls all published postings for a single company board token.
	Fetch(ctx context.Context, atsToken string) ([]NormalizedPosting, error)
}
