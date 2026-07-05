package ingest

import (
	"testing"
	"time"
)

func TestNormalizeDayforceJobBuildsLocationAndSalary(t *testing.T) {
	minRate, maxRate := 129200.0, 230600.0
	job := dayforceJob{
		Title:               "Sr Software Developer (Data Agents)",
		Description:         "Design and build data-centric AI agents.",
		City:                "",
		State:               "",
		Country:             "CA",
		JobFunction:         "Software Development",
		EmploymentIndicator: "Regular",
		DatePosted:          "2026-06-17T23:00:00",
		LastUpdated:         "2026-06-18T10:24:59.627",
		ReferenceNumber:     89077,
		JobDetailsUrl:       "https://jobs.dayforcehcm.com/en-US/mydayforce/ALLJOBS/jobs/89077",
		ApplyUrl:            "https://jobs.dayforcehcm.com/en-US/mydayforce/ALLJOBS/jobs/89077/apply",
		MinHiringRate:       &minRate,
		MaxHiringRate:       &maxRate,
		IsVirtualLocation:   false,
	}

	got := normalizeDayforceJob(job)

	if got.ExternalID != "89077" {
		t.Errorf("ExternalID = %q, want %q", got.ExternalID, "89077")
	}
	if got.Location != "CA" {
		t.Errorf("Location = %q, want %q (empty City/State dropped, no stray commas)", got.Location, "CA")
	}
	if got.IsRemote {
		t.Error("IsRemote = true, want false (IsVirtualLocation is false)")
	}
	if got.Department != "Software Development" {
		t.Errorf("Department = %q, want %q", got.Department, "Software Development")
	}
	if got.EmploymentType != "Regular" {
		t.Errorf("EmploymentType = %q, want %q", got.EmploymentType, "Regular")
	}
	if got.SalaryMin == nil || *got.SalaryMin != 129200.0 {
		t.Errorf("SalaryMin = %v, want 129200.0", got.SalaryMin)
	}
	if got.SalaryMax == nil || *got.SalaryMax != 230600.0 {
		t.Errorf("SalaryMax = %v, want 230600.0", got.SalaryMax)
	}
	if got.SalaryCurrency != "" {
		t.Errorf("SalaryCurrency = %q, want empty (Dayforce's feed has no currency field)", got.SalaryCurrency)
	}

	wantTime, err := time.Parse(dayforceTimeLayout, "2026-06-18T10:24:59.627")
	if err != nil {
		t.Fatalf("parsing want time: %v", err)
	}
	if got.SourceUpdatedAt == nil || !got.SourceUpdatedAt.Equal(wantTime) {
		t.Errorf("SourceUpdatedAt = %v, want %v (LastUpdated, not DatePosted)", got.SourceUpdatedAt, wantTime)
	}
}

func TestNormalizeDayforceJobParsesDateOnlyTimestamp(t *testing.T) {
	job := dayforceJob{
		Title:           "Test Automation Engineer Sr",
		ReferenceNumber: 56881,
		DatePosted:      "2023-08-22T00:00:00",
		LastUpdated:     "2023-08-22T10:18:34.24",
	}

	got := normalizeDayforceJob(job)

	wantTime, err := time.Parse(dayforceTimeLayout, "2023-08-22T10:18:34.24")
	if err != nil {
		t.Fatalf("parsing want time: %v", err)
	}
	if got.SourceUpdatedAt == nil || !got.SourceUpdatedAt.Equal(wantTime) {
		t.Errorf("SourceUpdatedAt = %v, want %v", got.SourceUpdatedAt, wantTime)
	}
}

func TestNormalizeDayforceJobFallsBackToDatePostedWhenLastUpdatedMissing(t *testing.T) {
	job := dayforceJob{
		Title:           "No LastUpdated",
		ReferenceNumber: 1,
		DatePosted:      "2023-08-22T00:00:00",
		LastUpdated:     "",
	}

	got := normalizeDayforceJob(job)

	wantTime, err := time.Parse(dayforceTimeLayout, "2023-08-22T00:00:00")
	if err != nil {
		t.Fatalf("parsing want time: %v", err)
	}
	if got.SourceUpdatedAt == nil || !got.SourceUpdatedAt.Equal(wantTime) {
		t.Errorf("SourceUpdatedAt = %v, want %v (fallback to DatePosted)", got.SourceUpdatedAt, wantTime)
	}
}

func TestNormalizeDayforceJobNoSalaryLeavesNil(t *testing.T) {
	job := dayforceJob{Title: "No Salary Listed", ReferenceNumber: 2}

	got := normalizeDayforceJob(job)

	if got.SalaryMin != nil || got.SalaryMax != nil {
		t.Errorf("SalaryMin/Max = %v/%v, want nil when Dayforce omits hiring rate fields", got.SalaryMin, got.SalaryMax)
	}
}
