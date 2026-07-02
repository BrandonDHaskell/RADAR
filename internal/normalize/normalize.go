// Package normalize derives dedup and change-detection keys from a
// NormalizedPosting: a canonical key for cross-source matching and a content
// hash for detecting when a posting's substantive fields changed.
package normalize

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode"

	"github.com/BrandonDHaskell/RADAR/internal/ingest"
)

// CanonicalKey normalizes companyName, the posting title, and its location
// into a stable, whitespace- and punctuation-insensitive key so the same
// role can be recognized across sources.
func CanonicalKey(companyName string, p ingest.NormalizedPosting) string {
	return strings.Join([]string{
		normalizeToken(companyName),
		normalizeToken(p.Title),
		normalizeToken(p.Location),
	}, "|")
}

// normalizeToken lowercases s and collapses runs of non-alphanumeric
// characters to a single space, so punctuation and spacing differences
// don't affect matching.
func normalizeToken(s string) string {
	var b strings.Builder
	lastWasSpace := true // trims leading space
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastWasSpace = false
			continue
		}
		if !lastWasSpace {
			b.WriteRune(' ')
			lastWasSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}

// ContentHash hashes the posting fields that, when changed, should trigger
// re-embedding and re-scoring in later phases. Bookkeeping fields
// (first_seen_at, last_seen_at, etc.) are intentionally excluded.
func ContentHash(p ingest.NormalizedPosting) string {
	fields := []string{
		p.Title,
		p.Location,
		fmt.Sprintf("%v", p.IsRemote),
		p.Department,
		p.EmploymentType,
		floatPtrString(p.SalaryMin),
		floatPtrString(p.SalaryMax),
		p.SalaryCurrency,
		p.Description,
		p.ApplyURL,
	}
	sum := sha256.Sum256([]byte(strings.Join(fields, "\x1f")))
	return hex.EncodeToString(sum[:])
}

func floatPtrString(f *float64) string {
	if f == nil {
		return ""
	}
	return fmt.Sprintf("%.2f", *f)
}
