package ingest

import (
	"strings"
	"testing"
	"time"
)

func TestNormalizeWorkableJobBuildsLocationAndDecodesDescription(t *testing.T) {
	job := workableJob{
		Title:          "Senior Data Engineer",
		Shortcode:      "E0ACB1A065",
		EmploymentType: "Full-time",
		Telecommuting:  true,
		Department:     "Engineering",
		URL:            "https://apply.workable.com/j/E0ACB1A065",
		ApplicationURL: "https://apply.workable.com/j/E0ACB1A065/apply",
		PublishedOn:    "2026-03-16",
		City:           "London",
		Country:        "United Kingdom",
		Description:    "Intro &lt;p&gt;We build &amp;amp; ship.&lt;/p&gt;",
	}

	got := normalizeWorkableJob(job)

	if got.ExternalID != "E0ACB1A065" {
		t.Errorf("ExternalID = %q, want shortcode %q", got.ExternalID, "E0ACB1A065")
	}
	if got.Location != "London, United Kingdom" {
		t.Errorf("Location = %q, want %q", got.Location, "London, United Kingdom")
	}
	if !got.IsRemote {
		t.Error("IsRemote = false, want true (telecommuting is true)")
	}
	if !strings.Contains(got.Description, "We build & ship.") {
		t.Errorf("Description = %q, want it to contain %q", got.Description, "We build & ship.")
	}
	for _, bad := range []string{"<", ">", "&lt;"} {
		if strings.Contains(got.Description, bad) {
			t.Errorf("Description = %q, must not contain %q", got.Description, bad)
		}
	}
	wantTime, _ := time.Parse(workableDateLayout, "2026-03-16")
	if got.SourceUpdatedAt == nil || !got.SourceUpdatedAt.Equal(wantTime) {
		t.Errorf("SourceUpdatedAt = %v, want %v", got.SourceUpdatedAt, wantTime)
	}
}

func TestNormalizeWorkableJobSkipsEmptyLocationParts(t *testing.T) {
	job := workableJob{Title: "Remote Role", Shortcode: "ABC", Country: "United Kingdom"}

	got := normalizeWorkableJob(job)

	if got.Location != "United Kingdom" {
		t.Errorf("Location = %q, want %q (empty city/state dropped, no stray commas)", got.Location, "United Kingdom")
	}
}

func TestNormalizeWorkableJobFallsBackToCreatedAt(t *testing.T) {
	job := workableJob{Title: "Analyst", Shortcode: "DEF", CreatedAt: "2026-01-05"}

	got := normalizeWorkableJob(job)

	wantTime, _ := time.Parse(workableDateLayout, "2026-01-05")
	if got.SourceUpdatedAt == nil || !got.SourceUpdatedAt.Equal(wantTime) {
		t.Errorf("SourceUpdatedAt = %v, want %v (fallback to created_at when published_on is empty)", got.SourceUpdatedAt, wantTime)
	}
}
