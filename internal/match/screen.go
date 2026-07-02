package match

import (
	"context"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/BrandonDHaskell/RADAR/internal/store"
)

// ScreenResult summarizes a Stage 0 screening pass.
type ScreenResult struct {
	Screened int
	Excluded int
}

// ScreenPostings applies Stage 0 (deterministic, free) to every open
// posting whose screen is pending or was computed against a different
// profile version: title exclusions first, then the location filter.
// Screening is recall-biased by design, since a false negative here is
// invisible to the operator; postings that pass go on to Stage 2
// (embedding). The operator reviews what got excluded via `radar excluded`.
func ScreenPostings(ctx context.Context, pool *pgxpool.Pool, profile *Profile) (ScreenResult, error) {
	var res ScreenResult

	candidates, err := store.ScreenCandidates(ctx, pool, profile.Hash)
	if err != nil {
		return res, err
	}

	for _, c := range candidates {
		reason := screenReason(c.Title, c.Location, c.IsRemote, profile.Preferences)

		status := "passed"
		var reasonPtr *string
		if reason != "" {
			status = "excluded"
			reasonPtr = &reason
			res.Excluded++
		}
		res.Screened++

		if err := store.SetPostingScreen(ctx, pool, c.ID, status, reasonPtr, profile.Hash); err != nil {
			return res, err
		}
	}

	return res, nil
}

// screenReason returns a non-empty, machine-readable exclusion reason if
// the posting should be excluded, or "" if it passes Stage 0.
func screenReason(title, location string, isRemote bool, prefs Preferences) string {
	if phrase, ok := matchedTitleExclusion(title, prefs.TitleExclusions); ok {
		return "title_exclusion:" + phrase
	}
	if locationExcluded(location, isRemote, prefs) {
		return "location"
	}
	return ""
}

// matchedTitleExclusion reports the first exclusion phrase (case
// insensitive, whole-phrase, word-boundary) found in title, if any. Word
// boundaries mean an exclusion like "ea" never fires inside "Team Lead".
func matchedTitleExclusion(title string, exclusions []string) (string, bool) {
	lowerTitle := strings.ToLower(title)
	for _, phrase := range exclusions {
		pattern := `\b` + regexp.QuoteMeta(strings.ToLower(phrase)) + `\b`
		re, err := regexp.Compile(pattern)
		if err != nil {
			continue // an unparseable exclusion phrase is skipped, not fatal
		}
		if re.MatchString(lowerTitle) {
			return phrase, true
		}
	}
	return "", false
}

// locationExcluded reports whether a posting should be excluded based on
// location. Conservative by construction and biased toward false
// positives passing through: remote postings pass whenever the profile
// accepts remote work, postings with no location string always pass, and
// any preference entry (parenthetical qualifier stripped, e.g.
// "Remote (US)" becomes "remote") that is a case-insensitive substring of
// the posting's location also passes. A preference phrase that is not
// literally contained in the location string (for example a location of
// "San Francisco, CA" against a preference of "San Francisco Bay Area")
// will not match and the posting is excluded; this is a known,
// accepted false-negative risk, not a bug, and is exactly what
// `radar excluded` exists to catch.
func locationExcluded(location string, isRemote bool, prefs Preferences) bool {
	if prefs.RemoteOK && isRemote {
		return false
	}
	if location == "" {
		return false
	}

	lowerLoc := strings.ToLower(location)
	for _, pref := range prefs.Locations {
		stripped := strings.ToLower(stripParenthetical(pref))
		if stripped != "" && strings.Contains(lowerLoc, stripped) {
			return false
		}
	}
	return true
}

// stripParenthetical removes a trailing " (...)" qualifier from s, e.g.
// "Remote (US)" becomes "Remote".
func stripParenthetical(s string) string {
	if i := strings.Index(s, "("); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}
