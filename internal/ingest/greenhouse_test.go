package ingest

import (
	"strings"
	"testing"
)

func TestNormalizeGreenhouseJobDecodesEscapedDescription(t *testing.T) {
	job := greenhouseJob{
		ID:      42,
		Title:   "Software Engineer",
		Content: "Intro &lt;p&gt;We build &amp;amp; ship.&lt;/p&gt;",
	}

	got := normalizeGreenhouseJob(job)

	if !strings.Contains(got.Description, "We build & ship.") {
		t.Errorf("Description = %q, want it to contain %q", got.Description, "We build & ship.")
	}
	for _, bad := range []string{"<", ">", "&lt;"} {
		if strings.Contains(got.Description, bad) {
			t.Errorf("Description = %q, must not contain %q", got.Description, bad)
		}
	}
}
