package ingest

import (
	"strings"
	"testing"
	"time"
)

func ptr(f float64) *float64 { return &f }

func TestNormalizeAshbyJobUsesPlainDescriptionAndWidestSalaryRange(t *testing.T) {
	job := ashbyJob{
		ID:               "job-1",
		Title:            "Director, Performance Marketing",
		Department:       "Marketing",
		EmploymentType:   "FullTime",
		Location:         "New York, NY (HQ)",
		PublishedAt:      "2026-06-11T17:21:26.410+00:00",
		IsRemote:         true,
		JobURL:           "https://jobs.ashbyhq.com/acme/job-1",
		ApplyURL:         "https://jobs.ashbyhq.com/acme/job-1/application",
		DescriptionPlain: "Plain text description.",
		DescriptionHTML:  "<p>HTML description.</p>",
		Compensation: ashbyCompensation{
			CompensationTiers: []ashbyCompensationTier{
				{Components: []ashbyCompensationComponent{
					{CompensationType: "EquityCashValue"},
					{CompensationType: "Salary", CurrencyCode: "USD", MinValue: ptr(212000), MaxValue: ptr(291000)},
				}},
				{Components: []ashbyCompensationComponent{
					{CompensationType: "Salary", CurrencyCode: "USD", MinValue: ptr(191000), MaxValue: ptr(262000)},
				}},
			},
		},
	}

	got := normalizeAshbyJob(job)

	if got.Description != "Plain text description." {
		t.Errorf("Description = %q, want descriptionPlain to be used over HTML", got.Description)
	}
	if got.SalaryMin == nil || *got.SalaryMin != 191000 {
		t.Errorf("SalaryMin = %v, want 191000 (lowest min across tiers)", got.SalaryMin)
	}
	if got.SalaryMax == nil || *got.SalaryMax != 291000 {
		t.Errorf("SalaryMax = %v, want 291000 (highest max across tiers)", got.SalaryMax)
	}
	if got.SalaryCurrency != "USD" {
		t.Errorf("SalaryCurrency = %q, want %q", got.SalaryCurrency, "USD")
	}
	wantTime, _ := time.Parse(time.RFC3339, "2026-06-11T17:21:26.410+00:00")
	if got.SourceUpdatedAt == nil || !got.SourceUpdatedAt.Equal(wantTime) {
		t.Errorf("SourceUpdatedAt = %v, want %v", got.SourceUpdatedAt, wantTime)
	}
}

func TestNormalizeAshbyJobFallsBackToHTMLDescription(t *testing.T) {
	job := ashbyJob{ID: "job-2", Title: "Engineer", DescriptionHTML: "<p>Only HTML &amp; text.</p>"}

	got := normalizeAshbyJob(job)

	if !strings.Contains(got.Description, "Only HTML & text.") {
		t.Errorf("Description = %q, want it to contain flattened HTML text", got.Description)
	}
	if strings.Contains(got.Description, "<p>") {
		t.Errorf("Description = %q, must not contain raw HTML tags", got.Description)
	}
}

func TestNormalizeAshbyJobNoCompensationLeavesSalaryNil(t *testing.T) {
	job := ashbyJob{ID: "job-3", Title: "Analyst"}

	got := normalizeAshbyJob(job)

	if got.SalaryMin != nil || got.SalaryMax != nil {
		t.Errorf("SalaryMin/Max = %v/%v, want nil when no compensation tiers present", got.SalaryMin, got.SalaryMax)
	}
}
