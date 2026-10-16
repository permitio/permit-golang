package main

import (
	"fmt"
	"io"
	"strconv"
	"strings"
)

// The verdicts that apicoverage prints last.
const (
	verdictPass      = "PASS"
	verdictFail      = "FAIL"
	verdictDidNotRun = "DID NOT RUN"
)

// rowCount is a line of the counts: the rows of a section of one stage, or SDK-only, by status.
type rowCount struct {
	section, stage string
	byStatus       map[string]int
	// undecided counts the operations of the stage without a row; it is -1 for the SDK-only
	// line, where it does not apply.
	undecided int
}

// total returns the number of rows the line counts.
func (r rowCount) total() int {
	n := 0
	for _, s := range statuses {
		n += r.byStatus[s]
	}
	return n
}

// counts returns the lines of the counts, the stages of each section and then its SDK-only
// rows, leaving out the empty lines.
func (res *result) counts() []rowCount {
	var lines []rowCount
	for _, s := range res.sections {
		for _, stage := range stages {
			l := rowCount{
				section: s.name, stage: stage, byStatus: s.byStage[stage],
				undecided: s.undecided[stage],
			}
			if l.total() > 0 || l.undecided > 0 {
				lines = append(lines, l)
			}
		}
		if l := (rowCount{
			section: s.name, stage: "SDK-only", byStatus: s.sdkOnly,
			undecided: -1,
		}); l.total() > 0 {
			lines = append(lines, l)
		}
	}
	return lines
}

// rows returns the number of rows of coverage.yaml.
func (res *result) rows() int {
	n := 0
	for _, s := range res.sections {
		n += s.rows
	}
	return n
}

// live reports whether the records hold a live request.
func (res *result) live() bool {
	return res.requests[modeLive] > 0
}

// e2eCounts counts the covered rows by end-to-end state.
func (res *result) e2eCounts() map[string]int {
	n := map[string]int{}
	for _, c := range res.covered {
		n[c.e2e]++
	}
	return n
}

// e2eStates are the end-to-end states, in the order the report lists them.
var e2eStates = []string{e2eCloud, e2eLocal, e2eLocalOnly, e2eAdvisory, e2eDryRun, e2eNone}

// verdict returns the verdict and what it found.
func (res *result) verdict() (string, string) {
	switch {
	case len(res.incomplete) > 0:
		return verdictDidNotRun, strings.Join(res.incomplete, "; ")
	case len(res.problems) > 0:
		return verdictFail, count(len(res.problems), "problem") + " with coverage.yaml, the " +
			"specs and the records"
	}
	return verdictPass, fmt.Sprintf("%d of %d rows covered, every GA operation decided, and "+
		"the records agree with coverage.yaml", len(res.covered), res.rows())
}

// requestSummary says what the records hold.
func (res *result) requestSummary() string {
	total := 0
	for _, n := range res.requests {
		total += n
	}
	return fmt.Sprintf("%s, %s, %s: %d wire, %d dryrun, %d live", count(res.files, "record file"),
		count(res.tests, "test"), count(total, "request"), res.requests[modeWire],
		res.requests[modeDryRun], res.requests[modeLive])
}

// count returns n and the noun, plural unless n is 1.
func count(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// e2eSummary says how the covered rows are proven end to end.
func (res *result) e2eSummary() string {
	n := res.e2eCounts()
	var parts []string
	for _, s := range e2eStates {
		parts = append(parts, fmt.Sprintf("%d %s", n[s], s))
	}
	text := fmt.Sprintf("e2e of the %s: %s", count(len(res.covered), "covered row"),
		strings.Join(parts, ", "))
	if !res.live() {
		text += "; dry run: the records hold no live request, and a request of a scenario run " +
			"offline is no end-to-end evidence"
	}
	return text
}

// statusList returns the counts of a line, as "0 covered, 153 planned, ...".
func statusList(byStatus map[string]int) string {
	var parts []string
	for _, s := range statuses {
		parts = append(parts, fmt.Sprintf("%d %s", byStatus[s], s))
	}
	return strings.Join(parts, ", ")
}

// writeText writes the report for a terminal or a log, with the verdict last.
func writeText(w io.Writer, res *result, sources string) {
	fmt.Fprintf(w, "apicoverage: %s\n", sources)
	fmt.Fprintf(w, "records: %s\n", res.requestSummary())
	for _, s := range res.sections {
		fmt.Fprintf(w, "%s: %d rows: %s\n", s.name, s.rows, statusList(s.byStatus))
		for _, l := range res.counts() {
			if l.section != s.name {
				continue
			}
			fmt.Fprintf(w, "  %s %d: %s", l.stage, l.total(), statusList(l.byStatus))
			if l.undecided >= 0 {
				fmt.Fprintf(w, ", %d undecided", l.undecided)
			}
			fmt.Fprintln(w)
		}
	}
	fmt.Fprintln(w, res.e2eSummary())
	for _, c := range res.covered {
		methods := strings.Join(c.methods, ", ")
		if methods == "" {
			methods = "no wire case names it"
		}
		fmt.Fprintf(w, "covered: %s (%s): %s; %s; e2e %s\n", c.row.name(), c.row.Op, methods,
			count(c.requests, "request"), c.e2e)
	}
	for _, p := range res.problems {
		fmt.Fprintf(w, "problem: %s\n", p)
	}
	verdict, found := res.verdict()
	fmt.Fprintf(w, "%s: %s\n", verdict, found)
}

// writeMarkdown writes the report for a job summary.
func writeMarkdown(w io.Writer, res *result, sources string) {
	verdict, found := res.verdict()
	fmt.Fprintf(w, "### API coverage\n\n**%s**: %s\n\n", verdict, cell(found))
	fmt.Fprintf(w, "%s. Records: %s.\n\n", cell(sources), res.requestSummary())
	fmt.Fprintln(w, "| Section | Stage | Rows | Covered | Planned | Excluded | Deferred | "+
		"Undecided |")
	fmt.Fprintln(w, "| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |")
	for _, l := range res.counts() {
		undecided := ""
		if l.undecided >= 0 {
			undecided = strconv.Itoa(l.undecided)
		}
		fmt.Fprintf(w, "| %s | %s | %d | %d | %d | %d | %d | %s |\n", l.section, l.stage,
			l.total(), l.byStatus[statusCovered], l.byStatus[statusPlanned],
			l.byStatus[statusExcluded], l.byStatus[statusDeferred], undecided)
	}
	fmt.Fprintln(w)
	e2e := "e2e"
	if !res.live() {
		e2e = "e2e (dry run)"
	}
	fmt.Fprintf(w, "%s.\n\n", cell(res.e2eSummary()))
	if len(res.covered) == 0 {
		fmt.Fprintln(w, "No row is covered yet.")
	} else {
		fmt.Fprintf(w, "| Route | Op | Methods | Requests | %s |\n", e2e)
		fmt.Fprintln(w, "| --- | --- | --- | ---: | --- |")
		for _, c := range res.covered {
			fmt.Fprintf(w, "| %s `%s` | `%s` | %s | %d | %s |\n", c.row.section,
				cell(c.row.Route), c.row.Op, cell(strings.Join(c.methods, ", ")), c.requests,
				c.e2e)
		}
	}
	if len(res.problems) > 0 {
		fmt.Fprintln(w, "\n#### Problems")
		fmt.Fprintln(w)
		for _, p := range res.problems {
			fmt.Fprintf(w, "- %s\n", cell(p))
		}
	}
	fmt.Fprintln(w)
}

// cell escapes text for a Markdown table cell or list item.
func cell(text string) string {
	return strings.NewReplacer("|", `\|`, "\n", " ").Replace(text)
}
