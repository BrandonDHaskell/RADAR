package match

import "testing"

func TestMatchedTitleExclusion(t *testing.T) {
	exclusions := []string{"executive assistant", "ea", "recruiter"}

	tests := []struct {
		name       string
		title      string
		wantMatch  bool
		wantPhrase string
	}{
		{"exact phrase match", "Executive Assistant to the CEO", true, "executive assistant"},
		{"case insensitive", "EXECUTIVE ASSISTANT", true, "executive assistant"},
		{"word boundary: short exclusion does not fire inside another word", "Team Lead", false, ""},
		{"word boundary: short exclusion does not fire inside 'Idea'", "Idea Generation Lead", false, ""},
		{"standalone short exclusion still matches", "EA to the VP", true, "ea"},
		{"no match", "Software Engineer", false, ""},
		{"recruiter substring inside longer word does not fire", "Recruiterly Analyst", false, ""},
		{"recruiter as whole word matches", "Technical Recruiter", true, "recruiter"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			phrase, ok := matchedTitleExclusion(tt.title, exclusions)
			if ok != tt.wantMatch {
				t.Errorf("matchedTitleExclusion(%q) match = %v, want %v", tt.title, ok, tt.wantMatch)
			}
			if ok && phrase != tt.wantPhrase {
				t.Errorf("matchedTitleExclusion(%q) phrase = %q, want %q", tt.title, phrase, tt.wantPhrase)
			}
		})
	}
}

func TestLocationExcluded(t *testing.T) {
	prefs := Preferences{
		Locations: []string{"San Francisco Bay Area", "Remote (US)"},
		RemoteOK:  true,
	}

	tests := []struct {
		name     string
		location string
		isRemote bool
		want     bool
	}{
		{"remote posting with remote_ok passes", "Anywhere", true, false},
		{"empty location always passes", "", false, false},
		{"location containing stripped preference passes", "Remote - US, PST preferred", false, false},
		{"non-remote posting with unmatched location is excluded", "New York, NY", false, true},
		{"non-remote posting with no location-preference substring match is excluded even if plausible", "San Francisco, CA", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := locationExcluded(tt.location, tt.isRemote, prefs); got != tt.want {
				t.Errorf("locationExcluded(%q, remote=%v) = %v, want %v", tt.location, tt.isRemote, got, tt.want)
			}
		})
	}
}

func TestLocationExcludedRemoteOKFalse(t *testing.T) {
	prefs := Preferences{Locations: []string{"San Francisco Bay Area"}, RemoteOK: false}
	// Even a remote posting is excluded when the operator doesn't want remote
	// and the location string itself doesn't match a preference.
	if !locationExcluded("Remote", true, prefs) {
		t.Error("locationExcluded: remote posting should be excluded when RemoteOK is false and location doesn't match a preference")
	}
}

func TestStripParenthetical(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"Remote (US)", "Remote"},
		{"San Francisco Bay Area", "San Francisco Bay Area"},
		{"Remote (US) ", "Remote"},
	}
	for _, tt := range tests {
		if got := stripParenthetical(tt.in); got != tt.want {
			t.Errorf("stripParenthetical(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestScreenReason(t *testing.T) {
	prefs := Preferences{
		Locations:       []string{"Remote (US)"},
		RemoteOK:        true,
		TitleExclusions: []string{"executive assistant"},
	}

	tests := []struct {
		name     string
		title    string
		location string
		isRemote bool
		want     string
	}{
		{"passes both checks", "Automation Engineer", "Remote", true, ""},
		{"title exclusion wins even with a matching location", "Executive Assistant", "Remote", true, "title_exclusion:executive assistant"},
		{"location exclusion when title is fine", "Automation Engineer", "New York, NY", false, "location"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := screenReason(tt.title, tt.location, tt.isRemote, prefs); got != tt.want {
				t.Errorf("screenReason(%q, %q, %v) = %q, want %q", tt.title, tt.location, tt.isRemote, got, tt.want)
			}
		})
	}
}
