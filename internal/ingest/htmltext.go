package ingest

import (
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// blockTags produce a newline at both their open and close boundary when
// flattened to plain text.
var blockTags = map[string]bool{
	"p": true, "div": true, "ul": true, "ol": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"tr": true,
}

// skipTags' contents are omitted entirely from the flattened text.
var skipTags = map[string]bool{
	"script": true, "style": true,
}

var (
	spaceTabRun = regexp.MustCompile(`[ \t]+`)
	newlineRun  = regexp.MustCompile(`\n{3,}`)
)

// htmlToText flattens an HTML fragment into readable plain text suitable
// for embedding and LLM input. It is shared by all ATS adapters.
func htmlToText(s string) string {
	tokenizer := html.NewTokenizer(strings.NewReader(s))

	var b strings.Builder
	var skipDepth int
	var inListItem bool

	for {
		tt := tokenizer.Next()
		switch tt {
		case html.ErrorToken:
			return collapseWhitespace(b.String())

		case html.TextToken:
			if skipDepth > 0 {
				continue
			}
			text := string(tokenizer.Text())
			if inListItem && strings.TrimSpace(text) != "" {
				b.WriteString("- ")
				inListItem = false
			}
			b.WriteString(text)

		case html.StartTagToken, html.SelfClosingTagToken:
			name, _ := tokenizer.TagName()
			tag := string(name)

			if skipTags[tag] {
				// A self-closing skip tag (e.g. <script/>) has no content
				// and thus no matching end tag, so only a real start tag
				// opens a depth that must later be closed.
				if tt == html.StartTagToken {
					skipDepth++
				}
				continue
			}
			if skipDepth > 0 {
				continue
			}

			switch tag {
			case "br":
				b.WriteString("\n")
			case "li":
				b.WriteString("\n")
				inListItem = true
			default:
				if blockTags[tag] {
					b.WriteString("\n")
				}
			}

		case html.EndTagToken:
			name, _ := tokenizer.TagName()
			tag := string(name)

			if skipTags[tag] {
				if skipDepth > 0 {
					skipDepth--
				}
				continue
			}
			if skipDepth > 0 {
				continue
			}

			if blockTags[tag] || tag == "li" {
				b.WriteString("\n")
			}
		}
	}
}

func collapseWhitespace(s string) string {
	s = spaceTabRun.ReplaceAllString(s, " ")
	s = newlineRun.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}
