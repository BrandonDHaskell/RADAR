package ingest

import (
	"strings"
	"testing"
)

func TestHTMLToText(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "entity in text node",
			input: "<p>Fish &amp; Chips</p>",
			want:  "Fish & Chips",
		},
		{
			name:  "unordered list",
			input: "<ul><li>Go</li><li>SQL</li></ul>",
			want:  "- Go\n\n- SQL",
		},
		{
			name:  "line break",
			input: "Line<br>Break",
			want:  "Line\nBreak",
		},
		{
			name:  "script content is skipped",
			input: "<script>var x = 1</script><p>Hi</p>",
			want:  "Hi",
		},
		{
			name:  "nested blocks collapse to at most two newlines",
			input: "<div><p>A</p><p>B</p></div>",
			want:  "A\n\nB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := htmlToText(tt.input)
			if got != tt.want {
				t.Errorf("htmlToText(%q) = %q, want %q", tt.input, got, tt.want)
			}
			if strings.Contains(got, "\n\n\n") {
				t.Errorf("htmlToText(%q) contains a run of 3+ newlines: %q", tt.input, got)
			}
		})
	}
}

func TestHTMLToTextSkipsStyle(t *testing.T) {
	got := htmlToText("<style>.a { color: red; }</style><p>Visible</p>")
	if got != "Visible" {
		t.Errorf("htmlToText with <style> = %q, want %q", got, "Visible")
	}
}
