package main

import (
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// source is what one scan of the standard report leaves in the results directory: its output,
// exit code, state and findings; an empty exit leaves out .exit, and an empty state or findings
// leaves out that file.
type source struct {
	out, exit, state, findings string
}

// writeSources writes the scans into a new results directory and returns it.
func writeSources(t *testing.T, sources map[string]source) string {
	t.Helper()
	dir := t.TempDir()
	for name, s := range sources {
		for ext, content := range map[string]string{
			".out": s.out, ".exit": s.exit, ".state": s.state, ".findings": s.findings,
		} {
			if content == "" && ext != ".out" {
				continue
			}
			if err := os.WriteFile(filepath.Join(dir, name+ext), []byte(content),
				0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	return dir
}

const (
	highNet = `{"id":"GHSA-aaaa-bbbb-cccc","package":"golang.org/x/net","version":"v0.1.0",` +
		`"severity":"high","fix":"v0.2.0",` +
		`"url":"https://github.com/advisories/GHSA-aaaa-bbbb-cccc"}`
	unscoredNet = `{"id":"GO-2026-0001","package":"golang.org/x/net","version":"v0.1.0",` +
		`"fix":"v0.2.0","url":"https://pkg.go.dev/vuln/GO-2026-0001"}`
	mediumText = `{"id":"CVE-2026-1","package":"golang.org/x/text","severity":"medium"}`
	// secretFinding is a private finding: where a secret is.
	secretFinding = `{"id":"generic-api-key","package":"config/old.go:3","version":"0123abcd",` +
		`"fix":"revoke it","private":true}`
)

// standardArgs returns the command line of the scheduled report for dir, with extra flags.
func standardArgs(dir string, extra ...string) []string {
	return append(append([]string{
		"-id", "security-scan", "-title", "Security scan", "-dir", dir, "-findings", "report",
	}, extra...), "floor", "alerts", "history")
}

func TestStandardReport(t *testing.T) {
	clean := map[string]source{
		"floor":   {out: "vulnscan: PASS\n", exit: "0"},
		"alerts":  {out: "alerts: 0 open\n", exit: "0"},
		"history": {out: "history: 0 findings\n", exit: "0", state: "history: 0 findings, 0 new"},
	}
	tests := []struct {
		name string
		// sources replace the clean ones they name.
		sources  map[string]source
		extra    []string
		wantExit int
		// want are parts of stdout, the summary and the Slack report; wantNot must be in none.
		wantStdout, wantSummary, wantSlack, wantNot []string
	}{
		{
			name: "every scan completed and found nothing", wantExit: exitPassed,
			wantStdout: []string{
				"secreport: floor: PASS\nsecreport: alerts: PASS\n" +
					"secreport: history: PASS; history: 0 findings, 0 new\n" +
					"secreport: Clean: all 3 scans completed, and none found anything.\n" +
					"secreport: Security scan: PASS\n",
			},
			wantSummary: []string{
				"## Security scan: PASS\n\nClean: all 3 scans completed, and none found " +
					"anything.\n\n| Scan | Result |\n| --- | --- |\n| floor | PASS |\n",
				"| history | PASS; history: 0 findings, 0 new |\n\n### High and critical\n\n" +
					"None.\n\n### Unscored\n\nNone.\n\n### Did not complete\n\nNone.\n\n<details>",
			},
			wantSlack: []string{
				"*Clean: all 3 scans completed, and none found anything.*\n*Scans*\n" +
					"• floor: PASS\n• alerts: PASS\n• history: PASS; history: 0 findings, 0 new\n" +
					"*High and critical*\n• None.\n*Unscored*\n• None.\n*Did not complete*\n" +
					"• None.\n",
			},
			wantNot: []string{"::error", "::warning"},
		},
		{
			name: "findings that do not fail the report",
			sources: map[string]source{
				"floor": {
					out: "vulnscan: FAIL\n", exit: "1",
					findings: highNet + "\n\n" + unscoredNet + "\n" + mediumText + "\n",
				},
				"alerts": {
					out: "alerts: 1 open\n", exit: "1",
					findings: strings.Replace(highNet, `"v0.1.0"`, `"< 0.2.0"`, 1) + "\n",
				},
			},
			wantExit: exitPassed,
			wantStdout: []string{
				"secreport: floor: FINDINGS (exit 1)\nsecreport: alerts: FINDINGS (exit 1)\n",
				"secreport: Findings: 3 (0 critical, 1 high, 1 medium, 0 low, 1 unscored), from " +
					"floor, alerts; all 3 scans completed.\n",
				"::warning title=Security scan::floor: FINDINGS (exit 1); the job summary lists " +
					"them\n",
				"secreport: Security scan: PASS\n",
			},
			wantSummary: []string{
				"| floor | FINDINGS (exit 1) |\n",
				"### High and critical\n\n| Severity | Advisory | Package | Fix | Scans |\n" +
					"| --- | --- | --- | --- | --- |\n| HIGH | [GHSA-aaaa-bbbb-cccc](https://" +
					"github.com/advisories/GHSA-aaaa-bbbb-cccc) | golang.org/x/net v0.1.0, \\< " +
					"0.2.0 | fix: v0.2.0 | floor, alerts |\n\n### Unscored\n\n" +
					"| Advisory | Package | Fix | Scans |\n| --- | --- | --- | --- |\n" +
					"| [GO-2026-0001](https://pkg.go.dev/vuln/GO-2026-0001) | golang.org/x/net " +
					"v0.1.0 | fix: v0.2.0 | floor |\n\n### Did not complete\n\nNone.\n",
				"<details open><summary>floor: FINDINGS</summary>",
			},
			wantSlack: []string{
				"*Findings: 3 (0 critical, 1 high, 1 medium, 0 low, 1 unscored), from floor, " +
					"alerts; all 3 scans completed.*\n",
				"*High and critical*\n• HIGH <https://github.com/advisories/GHSA-aaaa-bbbb-cccc|" +
					"GHSA-aaaa-bbbb-cccc> golang.org/x/net v0.1.0, &lt; 0.2.0: fix: v0.2.0 " +
					"(floor, alerts)\n*Unscored*\n• <https://pkg.go.dev/vuln/GO-2026-0001|" +
					"GO-2026-0001> golang.org/x/net v0.1.0: fix: v0.2.0 (floor)\n" +
					"*Did not complete*\n• None.\n",
			},
			wantNot: []string{"::error", "Clean", "CVE-2026-1"},
		},
		{
			name: "findings of a scan that -fail-on names",
			sources: map[string]source{
				"history": {out: "verified: 1\n", exit: "1", findings: secretFinding + "\n"},
			},
			extra:    []string{"-fail-on", "history"},
			wantExit: exitFailed,
			wantStdout: []string{
				"secreport: history: FAIL (exit 1)\n",
				"::error title=Security scan::history: FAIL (exit 1); the job summary has its " +
					"output\n",
				"secreport: Security scan: FAIL\n",
			},
			wantSummary: []string{
				"### Unscored\n\nNone.\n\n1 findings are listed in the Slack report only.\n",
			},
			wantSlack: []string{
				"*Unscored*\n• generic-api-key config/old.go:3 0123abcd: fix: revoke it " +
					"(history)\n",
			},
		},
		{
			name:     "a scan that is missing",
			sources:  map[string]source{"alerts": {out: ""}},
			wantExit: exitDidNotRun,
			wantStdout: []string{
				"secreport: alerts: DID NOT RUN: it did not finish: open ",
				"secreport: Did not complete: alerts. Findings from the 2 of 3 scans that " +
					"completed: 0 (0 critical, 0 high, 0 medium, 0 low, 0 unscored).\n",
				"secreport: Security scan: DID NOT RUN\n",
			},
			wantSummary: []string{"### Did not complete\n\n- alerts: DID NOT RUN: it did not "},
			wantSlack:   []string{"*Did not complete*\n• alerts: DID NOT RUN: it did not finish"},
			wantNot:     []string{"Clean", "clean"},
		},
		{
			name: "a scan that exited 2",
			sources: map[string]source{
				"alerts": {out: "DEPENDABOT_ALERTS_TOKEN is not set\n", exit: "2"},
			},
			wantExit: exitDidNotRun,
			wantStdout: []string{
				"secreport: alerts: DID NOT RUN (exit 2)\n",
				"::error title=Security scan::alerts: DID NOT RUN (exit 2); the job summary has " +
					"its output\n",
			},
			wantSummary: []string{"### Did not complete\n\n- alerts: DID NOT RUN \\(exit 2\\)\n"},
			wantSlack:   []string{"*Did not complete*\n• alerts: DID NOT RUN (exit 2)\n"},
			wantNot:     []string{"Clean", "clean"},
		},
		{
			name: "a private finding and values that need escaping",
			sources: map[string]source{
				"floor": {
					out: "x\n", exit: "1",
					findings: `{"id":"GO-1","package":"a<b>&c*d|e","fix":"x & y"}` + "\n",
				},
				"history": {out: "history\n", exit: "1", findings: secretFinding + "\n"},
			},
			wantExit: exitPassed,
			wantStdout: []string{
				"::warning title=Security scan::floor: FINDINGS (exit 1); the job summary lists " +
					"them\n",
				"::warning title=Security scan::history: FINDINGS (exit 1); the Slack report " +
					"lists them\n",
			},
			wantSummary: []string{
				"| GO-1 | a\\<b\\>\\&c\\*d\\|e | fix: x \\& y | floor |\n",
				"\n1 findings are listed in the Slack report only.\n",
			},
			wantSlack: []string{
				"• GO-1 a&lt;b&gt;&amp;c*d|e: fix: x &amp; y (floor)\n",
				"• generic-api-key config/old.go:3 0123abcd: fix: revoke it (history)\n",
			},
			wantNot: []string{"&amp;amp;", "&amp;lt;"},
		},
		{
			// The summary lists the findings of a scan unless every one of them is private.
			name: "a scan with private and public findings, and one with no findings",
			sources: map[string]source{
				"floor": {
					out: "x\n", exit: "1", findings: secretFinding + "\n" + unscoredNet + "\n",
				},
				"alerts": {out: "alerts: 1 open\n", exit: "1"},
			},
			wantExit: exitPassed,
			wantStdout: []string{
				"::warning title=Security scan::floor: FINDINGS (exit 1); the job summary lists " +
					"them\n",
				"::warning title=Security scan::alerts: FINDINGS (exit 1); the job summary lists " +
					"them\n",
			},
			wantNot: []string{"the Slack report lists them"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sources := maps.Clone(clean)
			maps.Copy(sources, tt.sources)
			dir := writeSources(t, sources)
			summary := filepath.Join(t.TempDir(), "summary.md")
			slack := filepath.Join(t.TempDir(), "report.slack")

			code, stdout := runSecreport(t, standardArgs(dir, append([]string{
				"-summary", summary, "-slack", slack,
			}, tt.extra...)...)...)

			if code != tt.wantExit {
				t.Errorf("exit = %d, want %d", code, tt.wantExit)
			}
			summaryData, err := os.ReadFile(summary)
			if err != nil {
				t.Fatal(err)
			}
			slackData, err := os.ReadFile(slack)
			if err != nil {
				t.Fatal(err)
			}
			for what, check := range map[string]struct {
				got  string
				want []string
			}{
				"stdout":           {stdout, tt.wantStdout},
				"the summary":      {string(summaryData), tt.wantSummary},
				"the Slack report": {string(slackData), tt.wantSlack},
			} {
				for _, want := range check.want {
					if !strings.Contains(check.got, want) {
						t.Errorf("%s does not contain %q:\n%s", what, want, check.got)
					}
				}
				for _, not := range tt.wantNot {
					if strings.Contains(check.got, not) {
						t.Errorf("%s contains %q:\n%s", what, not, check.got)
					}
				}
			}
			for _, private := range []string{"config/old.go", "0123abcd", "revoke it"} {
				if strings.Contains(stdout+string(summaryData), private) {
					t.Errorf("stdout or the summary holds the private %q", private)
				}
			}
		})
	}
}

func TestStandardMerge(t *testing.T) {
	results := []result{
		{name: "a", findings: []finding{
			{ID: "GO-2", Package: "p", Version: "v1", Fix: "v2"},
			{ID: "GO-1", Package: "p", Severity: "low"},
			{ID: "GO-1", Package: "q", Severity: "critical"},
		}},
		{name: "b", findings: []finding{
			{ID: "GO-2", Package: "p", Version: "v1.5", Fix: "v2", Severity: "high"},
			{ID: "GO-2", Package: "p", Version: "v1", Severity: "medium", URL: "https://x.test/2"},
			{ID: "GO-3", Package: "p", Private: true},
			{ID: "GO-3", Package: "p", Private: false},
		}},
	}

	std := newStandard(results)

	var got []string
	for _, f := range std.findings {
		got = append(got, strings.Join([]string{
			f.severity, f.id, f.pkgText(), f.fixText(), f.url, strings.Join(f.sources, "+"),
		}, "|")+map[bool]string{true: "|private"}[f.private])
	}
	want := []string{
		"critical|GO-1|q|no fix||a",
		"high|GO-2|p v1, v1.5|fix: v2|https://x.test/2|a+b",
		"low|GO-1|p|no fix||a",
		"|GO-3|p|no fix||b|private",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("merged findings:\n%s\nwant:\n%s", strings.Join(got, "\n"),
			strings.Join(want, "\n"))
	}
	scored, unscored, private := std.lists(true)
	if len(scored) != 2 || len(unscored) != 0 || private != 1 {
		t.Errorf("public lists: %d scored, %d unscored, %d private; want 2, 0, 1", len(scored),
			len(unscored), private)
	}
	scored, unscored, private = std.lists(false)
	if len(scored) != 2 || len(unscored) != 1 || private != 0 {
		t.Errorf("Slack lists: %d scored, %d unscored, %d private; want 2, 1, 0", len(scored),
			len(unscored), private)
	}
	if got := std.headline(); got != "Findings: 4 (1 critical, 1 high, 0 medium, 1 low, 1 "+
		"unscored), from a, b; all 2 scans completed." {
		t.Errorf("headline = %q", got)
	}
}

// url300 is an https link of exactly fieldLimit bytes.
var url300 = "https://x.test/" + strings.Repeat("x", fieldLimit-len("https://x.test/"))

func TestStandardFilesThatAreNotValid(t *testing.T) {
	long := strings.Repeat("x", fieldLimit+1)
	tests := []struct {
		name, state, findings, want string
	}{
		{
			name: "a state of two lines", state: "a\nb\n",
			want: "its state file is not one line of text: it holds a control character",
		},
		{
			name: "a state that is too long", state: strings.Repeat("x", stateLimit+1),
			want: "its state file is not one line of text: it has 301 bytes, more than 300",
		},
		{
			name: "a state that starts with a control character", state: "\x1b[31m",
			want: "its state file is not one line of text: it holds a control character",
		},
		{
			name: "a state that is not UTF-8", state: "\xff",
			want: "its state file is not one line of text: it is not UTF-8",
		},
		{
			name: "a finding that is not JSON", findings: "GO-1 x\n",
			want: "its findings file line 1: not a finding: ",
		},
		{
			name: "an unknown field", findings: `{"id":"a","package":"p","cvss":9}`,
			want: `line 1: not a finding: json: unknown field "cvss"`,
		},
		{
			name: "two values on a line", findings: "\n" + `{"id":"a","package":"p"} {}`,
			want: "line 2: not a finding: more than one JSON value on the line",
		},
		{
			name: "no id", findings: `{"package":"p"}`,
			want: `line 1: the id "" is empty or holds a space`,
		},
		{
			name: "an id with a space", findings: `{"id":"a b","package":"p"}`,
			want: `line 1: the id "a b" is empty or holds a space`,
		},
		{
			name: "no package", findings: `{"id":"a","package":" "}`,
			want: "line 1: the package is empty",
		},
		{
			name: "an unknown severity", findings: `{"id":"a","package":"p","severity":"HIGH"}`,
			want: `line 1: the severity "HIGH" is not critical, high, medium, low or empty`,
		},
		{
			name: "an http url", findings: `{"id":"a","package":"p","url":"http://x.test"}`,
			want: `line 1: the url "http://x.test" is not an https link`,
		},
		{
			name:     "a url that ends a link",
			findings: `{"id":"a","package":"p","url":"https://x|y"}`,
			want:     `is not an https link`,
		},
		{
			name:     "a url one byte longer than the limit",
			findings: `{"id":"a","package":"p","url":"https://x.test/` + url300[15:] + `x"}`,
			want:     "is not an https link",
		},
		{
			name:     "a url that is too long",
			findings: `{"id":"a","package":"p","url":"https://x.test/` + long + `"}`,
			want:     "is not an https link",
		},
		{
			name:     "a fix that is too long",
			findings: `{"id":"a","package":"p","fix":"` + long + `"}`,
			want:     "line 1: the fix: it has 301 bytes, more than 300",
		},
		{
			name: "a version with a newline", findings: `{"id":"a","package":"p","version":"1\n2"}`,
			want: "line 1: the version: it holds a control character",
		},
		{
			name:     "a version that starts with a control character",
			findings: `{"id":"a","package":"p","version":"\u0007x"}`,
			want:     "line 1: the version: it holds a control character",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeSources(t, map[string]source{
				"floor":   {out: "x\n", exit: "0", state: tt.state, findings: tt.findings},
				"alerts":  {out: "x\n", exit: "0"},
				"history": {out: "x\n", exit: "0"},
			})

			code, stdout := runSecreport(t, standardArgs(dir)...)

			if code != exitDidNotRun ||
				!strings.Contains(stdout, "secreport: floor: DID NOT RUN: ") ||
				!strings.Contains(stdout, tt.want) {
				t.Errorf("exit %d; want exit 2 and floor: DID NOT RUN: ...%s", code, tt.want)
			}
		})
	}

	t.Run("files that cannot be read", func(t *testing.T) {
		for _, ext := range []string{".state", ".findings"} {
			dir := writeSources(t, map[string]source{
				"floor": {out: "x\n", exit: "0"}, "alerts": {out: "x\n", exit: "0"},
				"history": {out: "x\n", exit: "0"},
			})
			if err := os.Mkdir(filepath.Join(dir, "floor"+ext), 0o755); err != nil {
				t.Fatal(err)
			}

			code, stdout := runSecreport(t, standardArgs(dir)...)

			if code != exitDidNotRun || !strings.Contains(stdout, " file cannot be read: ") {
				t.Errorf("a directory as %s: exit %d; want exit 2 and that it cannot be read", ext,
					code)
			}
		}
	})
}

// TestStandardTextAtTheLimit checks that a state and a url of exactly the most bytes they may hold
// are accepted.
func TestStandardTextAtTheLimit(t *testing.T) {
	state := strings.Repeat("s", stateLimit)
	dir := writeSources(t, map[string]source{
		"floor": {
			out: "x\n", exit: "1", state: state,
			findings: `{"id":"a","package":"p","url":"` + url300 + `"}` + "\n",
		},
		"alerts": {out: "x\n", exit: "0"}, "history": {out: "x\n", exit: "0"},
	})
	slack := filepath.Join(t.TempDir(), "report.slack")

	code, stdout := runSecreport(t, standardArgs(dir, "-slack", slack)...)

	data, err := os.ReadFile(slack)
	if err != nil {
		t.Fatal(err)
	}
	if code != exitPassed || !strings.Contains(stdout, "secreport: floor: FINDINGS (exit 1); "+
		state+"\n") || !strings.Contains(string(data), "<"+url300+"|a>") {
		t.Errorf("exit %d, Slack report:\n%s\nwant exit 0, the state and the url", code, data)
	}
}

func TestStandardArgs(t *testing.T) {
	dir := t.TempDir()
	base := []string{"-id", "s", "-title", "S", "-dir", dir}
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "another -findings", args: append(base, "-findings", "warn", "a"),
			want: `-findings "warn" is not fail or report`,
		},
		{
			name: "-fail-on without -findings report", args: append(base, "-fail-on", "a", "a"),
			want: "-fail-on needs -findings report",
		},
		{
			name: "-fail-on a scan the report does not name",
			args: append(base, "-findings", "report", "-fail-on", "b", "a"),
			want: `-fail-on "b" names no scan of the report`,
		},
		{
			name: "-fail-on twice",
			args: append(base, "-findings", "report", "-fail-on", "a", "-fail-on", "a", "a"),
			want: "-fail-on a is given twice",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout := runSecreport(t, tt.args...)

			if code != exitDidNotRun || !strings.Contains(stdout, tt.want) {
				t.Errorf("exit %d, stdout:\n%s\nwant exit 2 and %q", code, stdout, tt.want)
			}
		})
	}
}

func TestSlackReportThatCannotBeWritten(t *testing.T) {
	dir := writeSources(t, map[string]source{
		"floor": {out: "x\n", exit: "0"}, "alerts": {out: "x\n", exit: "0"},
		"history": {out: "x\n", exit: "0"},
	})

	code, stdout := runSecreport(t, standardArgs(dir, "-slack", t.TempDir())...)

	if code != exitDidNotRun ||
		!strings.Contains(stdout, "secreport: writing the Slack report failed: open ") ||
		!strings.HasSuffix(stdout, "secreport: Security scan: DID NOT RUN\n") {
		t.Errorf("exit %d, stdout:\n%s\nwant exit 2 and the error", code, stdout)
	}
}
