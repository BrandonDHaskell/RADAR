package ingest

import (
	"strings"
	"testing"
	"time"
)

func TestNormalizeLeverJobJoinsDescriptionSections(t *testing.T) {
	job := leverJob{
		ID:               "abc123",
		Text:             "Software Engineer",
		HostedURL:        "https://jobs.lever.co/acme/abc123",
		ApplyURL:         "https://jobs.lever.co/acme/abc123/apply",
		CreatedAt:        1700000000000,
		WorkplaceType:    "remote",
		DescriptionPlain: "We build things.",
		AdditionalPlain:  "Salary: $100k-$150k.",
		Categories: leverCategories{
			Commitment: "Full-time",
			Location:   "Remote - US",
			Team:       "Engineering",
		},
		Lists: []leverDescription{
			{Text: "Requirements", Content: "<ul><li>Go experience</li></ul>"},
		},
	}

	got := normalizeLeverJob(job)

	if got.ExternalID != "abc123" {
		t.Errorf("ExternalID = %q, want %q", got.ExternalID, "abc123")
	}
	if got.EmploymentType != "Full-time" {
		t.Errorf("EmploymentType = %q, want %q", got.EmploymentType, "Full-time")
	}
	if got.Department != "Engineering" {
		t.Errorf("Department = %q, want %q", got.Department, "Engineering")
	}
	if !got.IsRemote {
		t.Error("IsRemote = false, want true (workplaceType is \"remote\")")
	}
	for _, want := range []string{"We build things.", "Requirements", "Go experience", "Salary: $100k-$150k."} {
		if !strings.Contains(got.Description, want) {
			t.Errorf("Description = %q, want it to contain %q", got.Description, want)
		}
	}
	if strings.Contains(got.Description, "<ul>") || strings.Contains(got.Description, "<li>") {
		t.Errorf("Description = %q, must not contain raw HTML tags", got.Description)
	}
	wantTime := time.UnixMilli(1700000000000).UTC()
	if got.SourceUpdatedAt == nil || !got.SourceUpdatedAt.Equal(wantTime) {
		t.Errorf("SourceUpdatedAt = %v, want %v", got.SourceUpdatedAt, wantTime)
	}
}

func TestNormalizeLeverJobDetectsRemoteFromLocationFallback(t *testing.T) {
	job := leverJob{
		ID:            "xyz",
		Text:          "Product Manager",
		WorkplaceType: "onsite",
		Categories:    leverCategories{Location: "Remote - APAC"},
	}

	got := normalizeLeverJob(job)

	if !got.IsRemote {
		t.Error("IsRemote = false, want true (location contains \"Remote\" even though workplaceType is onsite)")
	}
}

func TestNormalizeLeverJobHandlesNoCreatedAt(t *testing.T) {
	job := leverJob{ID: "no-date", Text: "Analyst"}

	got := normalizeLeverJob(job)

	if got.SourceUpdatedAt != nil {
		t.Errorf("SourceUpdatedAt = %v, want nil when createdAt is 0", got.SourceUpdatedAt)
	}
}
