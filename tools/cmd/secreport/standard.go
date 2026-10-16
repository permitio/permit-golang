package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	// stateLimit is the most bytes a scan's state line may hold.
	stateLimit = 300
	// fieldLimit is the most bytes a finding's field may hold.
	fieldLimit = 300
)

var (
	// urlPattern matches an advisory link: https, without spaces or the characters that end a
	// link in Slack's text or in Markdown.
	urlPattern = regexp.MustCompile(`^https://[^\s<>|()\[\]"]+$`)
	// slackEscaper escapes the characters that Slack's text format gives a meaning.
	slackEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	// markdownEscaper escapes the ASCII punctuation that Markdown gives a meaning.
	markdownEscaper = strings.NewReplacer(`\`, `\\`, "`", "\\`", "*", `\*`, "_", `\_`, "[", `\[`,
		"]", `\]`, "(", `\(`, ")", `\)`, "<", `\<`, ">", `\>`, "|", `\|`, "#", `\#`, "!", `\!`,
		"&", `\&`, "~", `\~`)
)

// severities are the severities a finding may have, from the highest; the empty one is a finding
// without a score.
var severities = []string{"critical", "high", "medium", "low", ""}

// finding is one line of a scan's findings file.
type finding struct {
	ID       string `json:"id"`
	Package  string `json:"package"`
	Version  string `json:"version"`
	Severity string `json:"severity"`
	Fix      string `json:"fix"`
	URL      string `json:"url"`
	Private  bool   `json:"private"`
}

// readState reads a scan's state file, which may be missing.
func readState(path string) (string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("cannot be read: %w", err)
	}
	state := strings.TrimSuffix(string(data), "\n")
	if err := checkText(state, stateLimit); err != nil {
		return "", fmt.Errorf("is not one line of text: %w", err)
	}
	return state, nil
}

// checkText returns an error unless s is valid UTF-8 of at most limit bytes without control
// characters.
func checkText(s string, limit int) error {
	switch {
	case len(s) > limit:
		return fmt.Errorf("it has %d bytes, more than %d", len(s), limit)
	case !utf8.ValidString(s):
		return errors.New("it is not UTF-8")
	case strings.IndexFunc(s, unicode.IsControl) >= 0:
		return errors.New("it holds a control character")
	}
	return nil
}

// readFindings reads a scan's findings file, which may be missing.
func readFindings(path string) ([]finding, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot be read: %w", err)
	}
	var findings []finding
	lines := bufio.NewScanner(bytes.NewReader(data))
	lines.Buffer(nil, len(data)+1)
	for number := 1; lines.Scan(); number++ {
		if strings.TrimSpace(lines.Text()) == "" {
			continue
		}
		f, err := parseFinding(lines.Bytes())
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", number, err)
		}
		findings = append(findings, f)
	}
	return findings, lines.Err()
}

// parseFinding decodes and checks one finding.
func parseFinding(line []byte) (finding, error) {
	var f finding
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&f); err != nil {
		return finding{}, fmt.Errorf("not a finding: %w", err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return finding{}, errors.New("not a finding: more than one JSON value on the line")
	}
	switch {
	case f.ID == "" || strings.ContainsFunc(f.ID, unicode.IsSpace):
		return finding{}, fmt.Errorf("the id %q is empty or holds a space", f.ID)
	case strings.TrimSpace(f.Package) == "":
		return finding{}, errors.New("the package is empty")
	case !slices.Contains(severities, f.Severity):
		return finding{}, fmt.Errorf("the severity %q is not critical, high, medium, low or empty",
			f.Severity)
	case f.URL != "" && (!urlPattern.MatchString(f.URL) || len(f.URL) > fieldLimit):
		return finding{}, fmt.Errorf("the url %q is not an https link", f.URL)
	}
	for _, field := range []struct{ name, value string }{
		{"id", f.ID}, {"package", f.Package}, {"version", f.Version}, {"fix", f.Fix},
	} {
		if err := checkText(field.value, fieldLimit); err != nil {
			return finding{}, fmt.Errorf("the %s: %w", field.name, err)
		}
	}
	return f, nil
}

// merged is one finding across the scans that report it: they share its id and package.
type merged struct {
	id, pkg, severity, url   string
	private                  bool
	versions, fixes, sources []string
}

// standard is what the standard report says about the results.
type standard struct {
	results  []result
	findings []*merged
}

// newStandard merges the findings of the results.
func newStandard(results []result) standard {
	std := standard{results: results}
	byKey := map[[2]string]*merged{}
	for _, r := range results {
		for _, f := range r.findings {
			key := [2]string{f.ID, f.Package}
			m, ok := byKey[key]
			if !ok {
				m = &merged{id: f.ID, pkg: f.Package, severity: f.Severity}
				byKey[key] = m
				std.findings = append(std.findings, m)
			}
			m.severity = severities[min(rank(f.Severity), rank(m.severity))]
			if m.url == "" {
				m.url = f.URL
			}
			m.private = m.private || f.Private
			m.versions = appendNew(m.versions, f.Version)
			m.fixes = appendNew(m.fixes, f.Fix)
			m.sources = appendNew(m.sources, r.name)
		}
	}
	slices.SortStableFunc(std.findings, func(a, b *merged) int {
		if d := rank(a.severity) - rank(b.severity); d != 0 {
			return d
		}
		return strings.Compare(a.id+" "+a.pkg, b.id+" "+b.pkg)
	})
	return std
}

// rank returns a severity's place in severities: 0 for critical.
func rank(severity string) int {
	return slices.Index(severities, severity)
}

// appendNew appends s to list unless s is empty or in the list already.
func appendNew(list []string, s string) []string {
	if s == "" || slices.Contains(list, s) {
		return list
	}
	return append(list, s)
}

// incomplete returns the names of the scans that did not complete.
func (s standard) incomplete() []string {
	var names []string
	for _, r := range s.results {
		if r.verdict() == exitDidNotRun {
			names = append(names, r.name)
		}
	}
	return names
}

// headline returns the report's first line: the findings by severity, and which scans did not
// complete or found something. It holds scan names and counts only.
func (s standard) headline() string {
	counts := map[string]int{}
	for _, f := range s.findings {
		counts[f.severity]++
	}
	total := fmt.Sprintf("%d (%d critical, %d high, %d medium, %d low, %d unscored)",
		len(s.findings), counts["critical"], counts["high"], counts["medium"], counts["low"],
		counts[""])
	if incomplete := s.incomplete(); len(incomplete) > 0 {
		return fmt.Sprintf("Did not complete: %s. Findings from the %d of %d scans that "+
			"completed: %s.", strings.Join(incomplete, ", "), len(s.results)-len(incomplete),
			len(s.results), total)
	}
	// Every scan completed, so each exited 0 or 1.
	var found []string
	for _, r := range s.results {
		if len(r.findings) > 0 || r.code == exitFailed {
			found = append(found, r.name)
		}
	}
	if len(found) == 0 {
		return fmt.Sprintf("Clean: all %d scans completed, and none found anything.",
			len(s.results))
	}
	return fmt.Sprintf("Findings: %s, from %s; all %d scans completed.", total,
		strings.Join(found, ", "), len(s.results))
}

// lists returns the high and critical findings and those without a score, and the number of
// private ones that only the Slack report lists, when public is set.
func (s standard) lists(public bool) (scored, unscored []*merged, private int) {
	for _, f := range s.findings {
		listed := &unscored
		switch f.severity {
		case "critical", "high":
			listed = &scored
		case "":
		default:
			continue
		}
		if public && f.private {
			private++
			continue
		}
		*listed = append(*listed, f)
	}
	return scored, unscored, private
}

// fixText describes the fixes of a finding.
func (m *merged) fixText() string {
	if len(m.fixes) == 0 {
		return "no fix"
	}
	return "fix: " + strings.Join(m.fixes, ", ")
}

// pkgText names the package of a finding with the versions the scans found.
func (m *merged) pkgText() string {
	return strings.TrimSpace(m.pkg + " " + strings.Join(m.versions, ", "))
}

// writeMarkdown writes the sections of the standard report that follow the table of scans, in
// Markdown, without the private findings.
func (s standard) writeMarkdown(b *strings.Builder) {
	scored, unscored, private := s.lists(true)
	row := func(f *merged) string {
		advisory := markdownEscaper.Replace(f.id)
		if f.url != "" {
			advisory = "[" + advisory + "](" + f.url + ")"
		}
		pkg, fix := markdownEscaper.Replace(f.pkgText()), markdownEscaper.Replace(f.fixText())
		return fmt.Sprintf("| %s | %s | %s | %s |\n", advisory, pkg, fix,
			strings.Join(f.sources, ", "))
	}
	b.WriteString("\n### High and critical\n\n")
	if len(scored) == 0 {
		b.WriteString("None.\n")
	} else {
		b.WriteString("| Severity | Advisory | Package | Fix | Scans |\n" +
			"| --- | --- | --- | --- | --- |\n")
		for _, f := range scored {
			b.WriteString("| " + strings.ToUpper(f.severity) + " " + row(f))
		}
	}
	b.WriteString("\n### Unscored\n\n")
	if len(unscored) == 0 {
		b.WriteString("None.\n")
	} else {
		b.WriteString("| Advisory | Package | Fix | Scans |\n| --- | --- | --- | --- |\n")
		for _, f := range unscored {
			b.WriteString(row(f))
		}
	}
	if private > 0 {
		fmt.Fprintf(b, "\n%d findings are listed in the Slack report only.\n", private)
	}
	b.WriteString("\n### Did not complete\n\n")
	if len(s.incomplete()) == 0 {
		b.WriteString("None.\n")
	}
	for _, r := range s.results {
		if r.verdict() == exitDidNotRun {
			fmt.Fprintf(b, "- %s: %s\n", r.name, markdownEscaper.Replace(r.describe()))
		}
	}
}

// renderSlack returns the standard report in Slack's text format, with every finding.
func renderSlack(results []result) string {
	s := newStandard(results)
	scored, unscored, _ := s.lists(false)
	var b strings.Builder
	fmt.Fprintf(&b, "*%s*\n*Scans*\n", s.headline())
	for _, r := range results {
		fmt.Fprintf(&b, "• %s: %s\n", r.name, slackEscaper.Replace(r.describe()))
	}
	item := func(f *merged) string {
		advisory := slackEscaper.Replace(f.id)
		if f.url != "" {
			advisory = "<" + f.url + "|" + advisory + ">"
		}
		return fmt.Sprintf("%s %s: %s (%s)\n", advisory, slackEscaper.Replace(f.pkgText()),
			slackEscaper.Replace(f.fixText()), strings.Join(f.sources, ", "))
	}
	b.WriteString("*High and critical*\n")
	if len(scored) == 0 {
		b.WriteString("• None.\n")
	}
	for _, f := range scored {
		b.WriteString("• " + strings.ToUpper(f.severity) + " " + item(f))
	}
	b.WriteString("*Unscored*\n")
	if len(unscored) == 0 {
		b.WriteString("• None.\n")
	}
	for _, f := range unscored {
		b.WriteString("• " + item(f))
	}
	b.WriteString("*Did not complete*\n")
	if len(s.incomplete()) == 0 {
		b.WriteString("• None.\n")
	}
	for _, r := range results {
		if r.verdict() == exitDidNotRun {
			fmt.Fprintf(&b, "• %s: %s\n", r.name, slackEscaper.Replace(r.describe()))
		}
	}
	return b.String()
}
