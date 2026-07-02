package normalize_test

import (
	"testing"

	"github.com/BrandonDHaskell/RADAR/internal/ingest"
	"github.com/BrandonDHaskell/RADAR/internal/normalize"
)

func TestCanonicalKey(t *testing.T) {
	tests := []struct {
		name      string
		companyA  string
		postingA  ingest.NormalizedPosting
		companyB  string
		postingB  ingest.NormalizedPosting
		wantEqual bool
	}{
		{
			name:      "punctuation and case insensitive",
			companyA:  "Acme, Inc.",
			postingA:  ingest.NormalizedPosting{Title: "Software Engineer", Location: "San Francisco, CA"},
			companyB:  "acme inc",
			postingB:  ingest.NormalizedPosting{Title: "software engineer", Location: "san francisco ca"},
			wantEqual: true,
		},
		{
			name:      "different titles differ",
			companyA:  "Acme",
			postingA:  ingest.NormalizedPosting{Title: "Software Engineer", Location: "Remote"},
			companyB:  "Acme",
			postingB:  ingest.NormalizedPosting{Title: "Staff Engineer", Location: "Remote"},
			wantEqual: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keyA := normalize.CanonicalKey(tt.companyA, tt.postingA)
			keyB := normalize.CanonicalKey(tt.companyB, tt.postingB)
			if (keyA == keyB) != tt.wantEqual {
				t.Errorf("CanonicalKey(%q) = %q, CanonicalKey(%q) = %q, equal = %v, want %v",
					tt.companyA, keyA, tt.companyB, keyB, keyA == keyB, tt.wantEqual)
			}
		})
	}
}

func TestContentHash(t *testing.T) {
	base := ingest.NormalizedPosting{
		Title:       "Software Engineer",
		Location:    "Remote",
		Description: "Build things.",
	}
	changed := base
	changed.Description = "Build other things."

	if normalize.ContentHash(base) != normalize.ContentHash(base) {
		t.Error("ContentHash is not deterministic")
	}
	if normalize.ContentHash(base) == normalize.ContentHash(changed) {
		t.Error("ContentHash did not change when Description changed")
	}
}
