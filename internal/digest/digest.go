// Package digest renders the weekly Markdown or HTML review surface from a
// list of ranked postings. It has no database dependency: callers fetch
// postings (see internal/store.DigestPostings) and hand them to Render.
package digest

import (
	"fmt"
	"html/template"
	"io"
	"strconv"
	"strings"
	txttemplate "text/template"
	"time"

	"github.com/BrandonDHaskell/RADAR/internal/store"
	"github.com/BrandonDHaskell/RADAR/templates"
)

// Format selects Markdown or HTML rendering.
type Format string

const (
	FormatMarkdown Format = "md"
	FormatHTML     Format = "html"
)

type viewModel struct {
	GeneratedAt       string
	Entries           []entryView
	StaleVerdictCount int
}

type entryView struct {
	Title          string
	Company        string
	Location       string
	SalaryRange    string
	Verdict        string
	MatchedRoleTag string
	Reasoning      string
	ApplyURL       string
	SemanticScore  string
}

// Render writes a digest of postings to w in the given format.
func Render(w io.Writer, format Format, postings []store.DigestPosting) error {
	vm := viewModel{
		GeneratedAt: time.Now().Format("Monday, January 2, 2006"),
		Entries:     make([]entryView, len(postings)),
	}
	for i, p := range postings {
		vm.Entries[i] = toView(p)
		if p.VerdictStale {
			vm.StaleVerdictCount++
		}
	}

	switch format {
	case FormatMarkdown:
		tmpl, err := txttemplate.New("digest.md.tmpl").ParseFS(templates.FS, "digest.md.tmpl")
		if err != nil {
			return fmt.Errorf("parsing markdown template: %w", err)
		}
		return tmpl.Execute(w, vm)
	case FormatHTML:
		tmpl, err := template.New("digest.html.tmpl").ParseFS(templates.FS, "digest.html.tmpl")
		if err != nil {
			return fmt.Errorf("parsing html template: %w", err)
		}
		return tmpl.Execute(w, vm)
	default:
		return fmt.Errorf("unknown digest format %q (want %q or %q)", format, FormatMarkdown, FormatHTML)
	}
}

func toView(p store.DigestPosting) entryView {
	v := entryView{
		Title:    p.Title,
		Company:  p.CompanyName,
		Location: p.Location,
		ApplyURL: p.ApplyURL,
	}
	if p.IsRemote && !strings.Contains(strings.ToLower(p.Location), "remote") {
		if v.Location != "" {
			v.Location += " (Remote)"
		} else {
			v.Location = "Remote"
		}
	}
	if p.LLMVerdict != nil {
		v.Verdict = *p.LLMVerdict
	}
	if p.MatchedRoleTag != nil {
		v.MatchedRoleTag = *p.MatchedRoleTag
	}
	if p.LLMReasoning != nil {
		v.Reasoning = *p.LLMReasoning
	}
	if p.SalaryMin != nil || p.SalaryMax != nil {
		v.SalaryRange = formatSalary(p.SalaryMin, p.SalaryMax, p.SalaryCurrency)
	}
	if p.SemanticScore != nil {
		v.SemanticScore = fmt.Sprintf("%.0f%%", *p.SemanticScore*100)
	}
	return v
}

func formatSalary(min, max *float64, currency string) string {
	cur := currency
	if cur == "" {
		cur = "USD"
	}
	switch {
	case min != nil && max != nil:
		return fmt.Sprintf("%s %s - %s", cur, formatMoney(*min), formatMoney(*max))
	case min != nil:
		return fmt.Sprintf("%s %s+", cur, formatMoney(*min))
	case max != nil:
		return fmt.Sprintf("%s up to %s", cur, formatMoney(*max))
	default:
		return ""
	}
}

// formatMoney renders v as a whole-dollar amount with thousands separators,
// e.g. 125000 -> "125,000".
func formatMoney(v float64) string {
	s := strconv.FormatFloat(v, 'f', 0, 64)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}

	var groups []string
	for len(s) > 3 {
		groups = append([]string{s[len(s)-3:]}, groups...)
		s = s[:len(s)-3]
	}
	groups = append([]string{s}, groups...)

	result := strings.Join(groups, ",")
	if neg {
		result = "-" + result
	}
	return result
}
