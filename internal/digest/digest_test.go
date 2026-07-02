package digest

import (
	"bytes"
	"strings"
	"testing"

	"github.com/BrandonDHaskell/RADAR/internal/store"
)

func strPtr(s string) *string   { return &s }
func f64Ptr(f float64) *float64 { return &f }
func f32Ptr(f float32) *float32 { return &f }

func samplePostings() []store.DigestPosting {
	return []store.DigestPosting{
		{
			ID:             1,
			Title:          "Automation Engineer",
			CompanyName:    "Acme",
			Location:       "Remote",
			IsRemote:       true,
			SalaryMin:      f64Ptr(120000),
			SalaryMax:      f64Ptr(150000),
			SalaryCurrency: "USD",
			ApplyURL:       "https://example.com/apply/1",
			LLMVerdict:     strPtr("pursue"),
			MatchedRoleTag: strPtr("automation-engineer"),
			LLMReasoning:   strPtr("Strong match on automation experience."),
			SemanticScore:  f32Ptr(0.87),
		},
		{
			ID:          2,
			Title:       "Account Executive",
			CompanyName: "Beta Corp",
			Location:    "San Francisco, CA",
			ApplyURL:    "https://example.com/apply/2",
		},
	}
}

func TestRenderMarkdownIncludesEntryFields(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, FormatMarkdown, samplePostings()); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		"Automation Engineer at Acme",
		"Verdict: pursue",
		"automation-engineer",
		"Remote",
		"USD 120,000 - 150,000",
		"87%",
		"Strong match on automation experience.",
		"https://example.com/apply/1",
		"Account Executive at Beta Corp",
		"San Francisco, CA",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("markdown digest missing %q in:\n%s", want, out)
		}
	}
}

func TestRenderHTMLIncludesEntryFields(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, FormatHTML, samplePostings()); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		"<!DOCTYPE html>",
		"Automation Engineer at Acme",
		"verdict-pursue",
		"automation-engineer",
		"USD 120,000 - 150,000",
		`href="https://example.com/apply/1"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("html digest missing %q in:\n%s", want, out)
		}
	}
}

func TestRenderEmptyShowsNoMatchesMessage(t *testing.T) {
	for _, format := range []Format{FormatMarkdown, FormatHTML} {
		var buf bytes.Buffer
		if err := Render(&buf, format, nil); err != nil {
			t.Fatalf("Render(%s): %v", format, err)
		}
		if !strings.Contains(buf.String(), "No open, un-applied postings matched your criteria.") {
			t.Errorf("Render(%s) empty output = %q, want the no-matches message", format, buf.String())
		}
	}
}

func TestRenderRejectsUnknownFormat(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, Format("yaml"), samplePostings()); err == nil {
		t.Fatal("Render: got nil error for an unknown format, want an error")
	}
}

func TestRenderNeverProducesEmDashes(t *testing.T) {
	for _, format := range []Format{FormatMarkdown, FormatHTML} {
		var buf bytes.Buffer
		if err := Render(&buf, format, samplePostings()); err != nil {
			t.Fatalf("Render(%s): %v", format, err)
		}
		if strings.Contains(buf.String(), "—") {
			t.Errorf("Render(%s) output contains an em dash, which is banned project-wide", format)
		}
	}
}

func TestFormatMoney(t *testing.T) {
	tests := []struct {
		in   float64
		want string
	}{
		{0, "0"},
		{500, "500"},
		{1000, "1,000"},
		{125000, "125,000"},
		{1250000, "1,250,000"},
	}
	for _, tt := range tests {
		if got := formatMoney(tt.in); got != tt.want {
			t.Errorf("formatMoney(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestFormatSalary(t *testing.T) {
	tests := []struct {
		name     string
		min, max *float64
		currency string
		want     string
	}{
		{"range", f64Ptr(100000), f64Ptr(150000), "USD", "USD 100,000 - 150,000"},
		{"min only", f64Ptr(100000), nil, "USD", "USD 100,000+"},
		{"max only", nil, f64Ptr(150000), "USD", "USD up to 150,000"},
		{"neither", nil, nil, "USD", ""},
		{"defaults to USD", f64Ptr(100000), f64Ptr(150000), "", "USD 100,000 - 150,000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatSalary(tt.min, tt.max, tt.currency); got != tt.want {
				t.Errorf("formatSalary(%v, %v, %q) = %q, want %q", tt.min, tt.max, tt.currency, got, tt.want)
			}
		})
	}
}
