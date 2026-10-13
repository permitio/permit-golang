package main

import (
	"bytes"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/permitio/permit-golang/v2/tools/internal/execfile"
)

// scan is what one scan leaves in the results directory; a nil field leaves its file out.
type scan struct {
	out, exit *string
}

// ran returns a scan that printed out and exited with exit.
func ran(out, exit string) scan {
	return scan{out: &out, exit: &exit}
}

// writeResults writes the scans into a new results directory and returns it.
func writeResults(t *testing.T, scans map[string]scan) string {
	t.Helper()
	dir := t.TempDir()
	for name, s := range scans {
		for ext, content := range map[string]*string{".out": s.out, ".exit": s.exit} {
			if content == nil {
				continue
			}
			if err := os.WriteFile(filepath.Join(dir, name+ext), []byte(*content),
				0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	return dir
}

func runSecreport(t *testing.T, args ...string) (int, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(t.Context(), args, &stdout, &stderr)
	t.Logf("exit %d, stdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	return code, stdout.String()
}

const (
	floorOut  = "trivy: ./go.mod: Trivy 0.75.0; packages: 0, require directives: 0\ntrivy: PASS\n"
	vulnOut   = "GO-2024-3333  FAIL, called, not accepted\n\nvulnscan: FAIL\n"
	runURL    = "https://github.com/octo/repo/actions/runs/123"
	cleanHead = "## Vulnerabilities: PASS\n\n| Scan | Result |\n| --- | --- |\n" +
		"| govulncheck | PASS |\n| trivy-floor | PASS |\n"
)

// args returns the command line for the results directory, with extra flags before the names.
func args(dir string, extra ...string) []string {
	return append(append([]string{"-id", "vuln", "-title", "Vulnerabilities", "-dir", dir},
		extra...), "govulncheck", "trivy-floor")
}

func TestReport(t *testing.T) {
	missing := scan{}
	tests := []struct {
		name string
		// scans are the results; govulncheck passes unless they say otherwise.
		scans      map[string]scan
		wantExit   int
		wantStdout []string
		// wantSummary are parts of the job summary.
		wantSummary []string
	}{
		{
			name:     "every scan passed",
			scans:    map[string]scan{"trivy-floor": ran(floorOut, "0\n")},
			wantExit: exitPassed,
			wantStdout: []string{
				"secreport: govulncheck: PASS\nsecreport: trivy-floor: PASS\n" +
					"secreport: Vulnerabilities: PASS\n",
			},
			wantSummary: []string{
				cleanHead + "\n<details><summary>govulncheck: PASS</summary>\n\n```text\n" +
					"vulnscan: PASS\n```\n\n</details>\n\n<details><summary>trivy-floor: PASS" +
					"</summary>\n\n```text\n" + floorOut + "```\n\n</details>\n\nThe [workflow " +
					"run](" + runURL + ") has the full output of every scan.\n\n",
			},
		},
		{
			name: "a scan failed",
			scans: map[string]scan{
				"govulncheck": ran(vulnOut, "1"),
				"trivy-floor": ran(floorOut, "0"),
			},
			wantExit: exitFailed,
			wantStdout: []string{
				"secreport: govulncheck: FAIL (exit 1)\nsecreport: trivy-floor: PASS\n" +
					"::error title=Vulnerabilities::govulncheck: FAIL (exit 1); the job summary " +
					"has its output\nsecreport: Vulnerabilities: FAIL\n",
			},
			wantSummary: []string{
				"## Vulnerabilities: FAIL\n",
				"| govulncheck | FAIL (exit 1) |\n",
				"\n<details open><summary>govulncheck: FAIL</summary>\n\n```text\n" + vulnOut +
					"```\n",
				"\n<details><summary>trivy-floor: PASS</summary>\n",
			},
		},
		{
			name:     "a scan exited 2",
			scans:    map[string]scan{"trivy-floor": ran("trivy: DID NOT RUN\n", "2")},
			wantExit: exitDidNotRun,
			wantStdout: []string{
				"secreport: trivy-floor: DID NOT RUN (exit 2)\n",
				"::error title=Vulnerabilities::trivy-floor: DID NOT RUN (exit 2); the job " +
					"summary has its output\n",
			},
			wantSummary: []string{"| trivy-floor | DID NOT RUN (exit 2) |\n"},
		},
		{
			name:       "a scan that could not start",
			scans:      map[string]scan{"trivy-floor": ran("trivy.sh: not found\n", "127")},
			wantExit:   exitDidNotRun,
			wantStdout: []string{"secreport: trivy-floor: DID NOT RUN (exit 127)\n"},
		},
		{
			name: "a failed scan and one that did not run",
			scans: map[string]scan{
				"govulncheck": ran(vulnOut, "1"), "trivy-floor": ran("x\n", "-1"),
			},
			wantExit:   exitDidNotRun,
			wantStdout: []string{"secreport: Vulnerabilities: DID NOT RUN\n"},
		},
		{
			name:     "a scan without an exit code file",
			scans:    map[string]scan{"trivy-floor": missing},
			wantExit: exitDidNotRun,
			wantStdout: []string{
				"secreport: trivy-floor: DID NOT RUN: it did not finish: open ",
				"trivy-floor.exit: no such file or directory\n",
			},
		},
		{
			name:     "an exit code that is not a number",
			scans:    map[string]scan{"trivy-floor": ran(floorOut, "fail\n")},
			wantExit: exitDidNotRun,
			wantStdout: []string{
				"secreport: trivy-floor: DID NOT RUN: its exit code file holds \"fail\", not a " +
					"number\n",
			},
		},
		{
			name:     "a scan without an output file",
			scans:    map[string]scan{"trivy-floor": {exit: ran("", "0").exit}},
			wantExit: exitDidNotRun,
			wantStdout: []string{
				"secreport: trivy-floor: DID NOT RUN: its output is missing: open ",
			},
		},
		{
			name:       "a scan that printed nothing",
			scans:      map[string]scan{"trivy-floor": ran(" \n\n", "0")},
			wantExit:   exitDidNotRun,
			wantStdout: []string{"secreport: trivy-floor: DID NOT RUN: it printed nothing\n"},
			wantSummary: []string{
				"| trivy-floor | DID NOT RUN: it printed nothing |\n",
				"<details open><summary>trivy-floor: DID NOT RUN</summary>\n\n```text\n \n\n```\n",
			},
		},
		{
			name: "output with a code fence and no final newline",
			scans: map[string]scan{
				"trivy-floor": ran("a ``` b ```` c", "0"),
			},
			wantExit:    exitPassed,
			wantSummary: []string{"\n`````text\na ``` b ```` c\n`````\n"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scans := map[string]scan{"govulncheck": ran("vulnscan: PASS\n", "0")}
			maps.Copy(scans, tt.scans)
			dir := writeResults(t, scans)
			summary := filepath.Join(t.TempDir(), "summary.md")
			if err := os.WriteFile(summary, []byte("earlier\n"), 0o600); err != nil {
				t.Fatal(err)
			}

			code, stdout := runSecreport(t, args(dir, "-summary", summary, "-run-url", runURL)...)

			if code != tt.wantExit {
				t.Errorf("exit = %d, want %d", code, tt.wantExit)
			}
			for _, want := range tt.wantStdout {
				if !strings.Contains(stdout, want) {
					t.Errorf("stdout does not contain %q", want)
				}
			}
			if !strings.HasSuffix(stdout, "secreport: Vulnerabilities: "+verdicts[tt.wantExit]+
				"\n") {
				t.Error("stdout does not end with the verdict")
			}
			if strings.Contains(stdout, "GO-2024-3333") || strings.Contains(stdout, "trivy: ") {
				t.Error("stdout holds the output of a scan")
			}
			data, err := os.ReadFile(summary)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(string(data), "earlier\n## Vulnerabilities: ") {
				t.Errorf("the summary does not start with what it held, then the report:\n%s", data)
			}
			for _, want := range tt.wantSummary {
				if !strings.Contains(string(data), want) {
					t.Errorf("the summary does not contain %q:\n%s", want, data)
				}
			}
		})
	}
}

func TestReportWithoutSummary(t *testing.T) {
	dir := writeResults(t, map[string]scan{
		"govulncheck": ran("vulnscan: PASS\n", "0"), "trivy-floor": ran(floorOut, "0"),
	})

	code, stdout := runSecreport(t, args(dir)...)

	want := "secreport: govulncheck: PASS\nsecreport: trivy-floor: PASS\n" +
		"secreport: Vulnerabilities: PASS\n"
	if code != exitPassed || stdout != want {
		t.Errorf("exit %d, stdout:\n%s\nwant exit 0, stdout:\n%s", code, stdout, want)
	}
}

func TestSummaryThatCannotBeWritten(t *testing.T) {
	dir := writeResults(t, map[string]scan{
		"govulncheck": ran("vulnscan: PASS\n", "0"), "trivy-floor": ran(floorOut, "0"),
	})

	code, stdout := runSecreport(t, args(dir, "-summary", t.TempDir())...)

	if code != exitDidNotRun ||
		!strings.Contains(stdout, "secreport: writing the job summary failed: open ") ||
		!strings.HasSuffix(stdout, "secreport: Vulnerabilities: DID NOT RUN\n") {
		t.Errorf("exit %d, stdout:\n%s\nwant exit 2 and the error", code, stdout)
	}
}

func TestAnnotationEscapes(t *testing.T) {
	dir := writeResults(t, map[string]scan{
		"govulncheck": ran(vulnOut, "1"), "trivy-floor": ran(floorOut, "0"),
	})

	_, stdout := runSecreport(t, "-id", "vuln", "-title", "Scans: 100%, all", "-dir", dir,
		"govulncheck", "trivy-floor")

	want := "::error title=Scans%3A 100%25%2C all::govulncheck: FAIL (exit 1); the job summary " +
		"has its output\n"
	if !strings.Contains(stdout, want) {
		t.Errorf("stdout does not contain %q:\n%s", want, stdout)
	}
	if got := escapeData("a%b\r\nc"); got != "a%25b%0D%0Ac" {
		t.Errorf("escapeData = %q, want a%%25b%%0D%%0Ac", got)
	}
}

func TestTableEscapesPipes(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a|b")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	summary := filepath.Join(t.TempDir(), "summary.md")

	runSecreport(t, args(dir, "-summary", summary)...)

	data, err := os.ReadFile(summary)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `a\|b`) || strings.Contains(string(data), "a|b") {
		t.Errorf("the table does not escape the | in a result:\n%s", data)
	}
}

func TestRenderCutsLongOutputs(t *testing.T) {
	opts := options{title: "Vulnerabilities", runURL: runURL}
	long := strings.Repeat("GO-2026-0001  FAIL, called, not accepted\n", 3000)
	results := []result{
		{name: "govulncheck", code: 1, output: long},
		{name: "trivy-floor", code: 0, output: floorOut},
		{name: "trivy-tools", code: 0, output: strings.Repeat("é", 40000)},
	}

	report := render(opts, results, exitFailed, commentLimit)

	if len(report) > commentLimit {
		t.Errorf("the report has %d bytes, more than %d", len(report), commentLimit)
	}
	if !utf8.ValidString(report) {
		t.Error("the report is not valid UTF-8")
	}
	for _, want := range []string{
		"```text\nGO-2026-0001  FAIL, called, not accepted\n",
		"not accepted\n... ",
		" more bytes; the job log has the full output\n```\n",
		"```text\n" + floorOut + "```\n",
		"has the full output of every scan.\n",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("the report does not contain %q", want)
		}
	}
	if got, _ := cutOutput(floorOut, len(floorOut)); got != floorOut {
		t.Errorf("an output that fits is cut to %q", got)
	}
	if got, cut := cutOutput("ab\ncd", 4); got != "ab\n" || cut != 2 {
		t.Errorf("cutOutput(ab cd, 4) = %q, %d; want the first line and 2", got, cut)
	}
}

func TestBadArgs(t *testing.T) {
	dir := t.TempDir()
	base := []string{"-id", "vuln", "-title", "Vulnerabilities", "-dir", dir}
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "an unknown flag", args: []string{"-fix"}, want: "flag provided but not defined"},
		{name: "no id", args: []string{"-title", "T", "-dir", dir, "a"}, want: `-id ""`},
		{
			name: "an id with capitals",
			args: []string{"-id", "Vuln", "-title", "T", "-dir", dir, "a"},
			want: `-id "Vuln" is not lowercase letters, digits and dashes`,
		},
		{
			name: "no title", args: []string{"-id", "vuln", "-dir", dir, "a"},
			want: "-title must be one line of text",
		},
		{
			name: "a title of two lines",
			args: []string{"-id", "vuln", "-title", "a\nb", "-dir", dir, "a"},
			want: "-title must be one line of text",
		},
		{
			name: "no dir", args: []string{"-id", "vuln", "-title", "T", "a"},
			want: "-dir is missing",
		},
		{name: "no names", args: base, want: "no scan names: name each scan the job ran"},
		{
			name: "a name with an underscore", args: append(slices.Clone(base), "trivy_floor"),
			want: `the scan name "trivy_floor" is not lowercase letters, digits and dashes`,
		},
		{
			name: "a name given twice", args: append(slices.Clone(base), "a", "b", "a"),
			want: "the scan name a is given twice",
		},
		{
			name: "a run URL over http",
			args: append(slices.Clone(base), "-run-url", "http://example.com/run", "a"),
			want: `-run-url "http://example.com/run" is not an https URL`,
		},
		{
			name: "a repository without a pull request",
			args: append(slices.Clone(base), "-repo", "octo/repo", "a"),
			want: "-repo and -head-repo need -pr",
		},
		{
			name: "a head repository without a pull request",
			args: append(slices.Clone(base), "-head-repo", "octo/repo", "a"),
			want: "-repo and -head-repo need -pr",
		},
		{
			name: "a pull request number of 0",
			args: append(slices.Clone(base), "-pr", "0", "-repo", "octo/repo", "a"),
			want: `-pr "0" is not a pull request number`,
		},
		{
			name: "a pull request that is not a number",
			args: append(slices.Clone(base), "-pr", "7;rm", "-repo", "octo/repo", "a"),
			want: `-pr "7;rm" is not a pull request number`,
		},
		{
			name: "a pull request without a repository",
			args: append(slices.Clone(base), "-pr", "7", "a"),
			want: `-repo "" is not owner/name`,
		},
		{
			name: "a repository path",
			args: append(slices.Clone(base), "-pr", "7", "-repo", "octo/repo/x", "a"),
			want: `-repo "octo/repo/x" is not owner/name`,
		},
		{
			name: "a head repository that is not owner/name",
			args: append(slices.Clone(base), "-pr", "7", "-repo", "octo/repo", "-head-repo",
				"../x", "a"),
			want: `-head-repo "../x" is not owner/name`,
		},
		{
			name: "a repository named ..",
			args: append(slices.Clone(base), "-pr", "7", "-repo", "octo/..", "a"),
			want: `-repo "octo/.." is not owner/name`,
		},
		{
			name: "a repository named .",
			args: append(slices.Clone(base), "-pr", "7", "-repo", "octo/.", "a"),
			want: `-repo "octo/." is not owner/name`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout := runSecreport(t, tt.args...)

			if code != exitDidNotRun || !strings.HasPrefix(stdout, "secreport: DID NOT RUN: ") ||
				!strings.Contains(stdout, tt.want) ||
				!strings.HasSuffix(stdout, "\nsecreport: DID NOT RUN\n") {
				t.Errorf("exit %d, stdout:\n%s\nwant exit 2 and %q", code, stdout, tt.want)
			}
		})
	}
}

// ghStub stands in for gh with shell builtins only, since PATH holds nothing else. It appends a
// "call" line, one "arg" line for each argument, its standard input and an "end" line to the file
// GH_LOG names. It prints GH_LIST for a call with --paginate, and exits 1 with an error for each
// call whose method GH_FAIL names: list, PATCH or POST.
const ghStub = `#!/bin/sh
{
	echo call
	for arg in "$@"; do echo "arg $arg"; done
	while IFS= read -r line || [ -n "$line" ]; do echo "stdin $line"; done
	echo end
} >>"$GH_LOG"
case "$*" in
*--paginate*) method=list ;;
*PATCH*) method=PATCH ;;
*) method=POST ;;
esac
case " $GH_FAIL " in
*" $method "*) echo "HTTP 403: Resource not accessible by integration" >&2; exit 1 ;;
esac
[ "$method" != list ] || printf '%s' "$GH_LIST"
`

// ghBin writes the gh stub into a new directory and returns the directory. Tests share one stub,
// since some systems check each new executable the first time it runs, which is slow.
func ghBin(t *testing.T) string {
	t.Helper()
	bin := t.TempDir()
	if err := execfile.WriteFile(filepath.Join(bin, "gh"), []byte(ghStub), 0o700); err != nil {
		t.Fatal(err)
	}
	return bin
}

// useGH makes bin, which holds the gh stub, the only directory on PATH, gives the stub listed and
// fail as GH_LIST and GH_FAIL, and returns the path of the stub's log.
func useGH(t *testing.T, bin, listed, fail string) string {
	t.Helper()
	log := filepath.Join(t.TempDir(), "gh.log")
	t.Setenv("PATH", bin)
	t.Setenv("GH_LOG", log)
	t.Setenv("GH_LIST", listed)
	t.Setenv("GH_FAIL", fail)
	return log
}

// ghCall is one run of the gh stub: its arguments and what it read on standard input.
type ghCall struct {
	args  []string
	stdin string
}

// readGHLog parses the log of the gh stub.
func readGHLog(t *testing.T, path string) []ghCall {
	t.Helper()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var calls []ghCall
	for line := range strings.SplitSeq(string(data), "\n") {
		switch {
		case line == "call":
			calls = append(calls, ghCall{})
		case strings.HasPrefix(line, "arg "):
			calls[len(calls)-1].args = append(calls[len(calls)-1].args, line[len("arg "):])
		case strings.HasPrefix(line, "stdin "):
			calls[len(calls)-1].stdin += line[len("stdin "):] + "\n"
		}
	}
	return calls
}

func TestComment(t *testing.T) {
	const (
		marker = "<!-- secreport:vuln -->\n"
		filter = `.[] | select(.user.login == "github-actions[bot]" and ` +
			`(.body | startswith("<!-- secreport:vuln -->"))) | .id`
	)
	list := []string{"api", "--paginate", "--jq", filter, "repos/octo/repo/issues/7/comments"}
	post := []string{
		"api", "--method", "POST", "repos/octo/repo/issues/7/comments", "-F", "body=@-",
	}
	patch := []string{
		"api", "--method", "PATCH", "repos/octo/repo/issues/comments/42", "-F", "body=@-",
	}
	failing := map[string]scan{"govulncheck": ran(vulnOut, "1")}
	bin := ghBin(t)
	tests := []struct {
		name string
		// scans are the results; each scan passes unless they say otherwise.
		scans map[string]scan
		// headRepo is the pull request's head repository, octo/repo unless set; listed is the
		// stub's GH_LIST, and fail its GH_FAIL.
		headRepo, listed, fail string
		wantExit               int
		wantCalls              [][]string
		// wantBody is the start of the comment the last call wrote.
		wantBody   string
		wantStdout string
	}{
		{
			name: "a failing scan and no comment yet", scans: failing, wantExit: exitFailed,
			wantCalls:  [][]string{list, post},
			wantBody:   marker + "## Vulnerabilities: FAIL\n",
			wantStdout: "secreport: posted a comment on pull request 7\n",
		},
		{
			name:       "a scan that did not run and no comment yet",
			scans:      map[string]scan{"govulncheck": ran("vulnscan: DID NOT RUN\n", "2")},
			wantExit:   exitDidNotRun,
			wantCalls:  [][]string{list, post},
			wantBody:   marker + "## Vulnerabilities: DID NOT RUN\n",
			wantStdout: "secreport: posted a comment on pull request 7\n",
		},
		{
			name: "a failing scan and an earlier comment", scans: failing, listed: "42\n43\n",
			wantExit: exitFailed, wantCalls: [][]string{list, patch},
			wantBody:   marker + "## Vulnerabilities: FAIL\n",
			wantStdout: "secreport: updated comment 42 on pull request 7\n",
		},
		{
			name: "every scan passed and an earlier comment", listed: "42", wantExit: exitPassed,
			wantCalls:  [][]string{list, patch},
			wantBody:   marker + cleanHead,
			wantStdout: "secreport: updated comment 42 on pull request 7\n",
		},
		{
			name: "every scan passed and no comment", wantExit: exitPassed,
			wantCalls: [][]string{list},
			wantStdout: "secreport: every scan passed, and pull request 7 has no comment of this " +
				"report to update\n",
		},
		{
			name:     "a pull request from a fork",
			scans:    failing,
			headRepo: "fork/repo",
			wantExit: exitFailed,
			wantStdout: "::notice title=Vulnerabilities::This pull request comes from a fork, " +
				"and the token of its run cannot comment on it; the job summary has the report.\n",
		},
		{
			name: "a pull request from a deleted fork", scans: failing, headRepo: "-",
			wantExit:   exitFailed,
			wantStdout: "::notice title=Vulnerabilities::This pull request comes from a fork",
		},
		{
			name: "the comments cannot be listed", scans: failing, fail: "list",
			wantExit: exitDidNotRun, wantCalls: [][]string{list},
			wantStdout: "secreport: the comment on pull request 7 could not be written: listing " +
				"its comments: gh api: exit status 1: HTTP 403: Resource not accessible by " +
				"integration\n",
		},
		{
			name: "the comment cannot be posted", scans: failing, fail: "POST",
			wantExit: exitDidNotRun, wantCalls: [][]string{list, post},
			wantStdout: "secreport: the comment on pull request 7 could not be written: posting " +
				"it: gh api: exit status 1: HTTP 403",
		},
		{
			name: "the comment cannot be updated", listed: "42", fail: "PATCH",
			wantExit: exitDidNotRun, wantCalls: [][]string{list, patch},
			wantStdout: "secreport: the comment on pull request 7 could not be written: updating " +
				"comment 42: gh api: exit status 1",
		},
		{
			name: "a comment ID that is not a number", listed: "null\n",
			wantExit: exitDidNotRun, wantCalls: [][]string{list},
			wantStdout: "gh listed the comment ID \"null\", which is not a number\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scans := map[string]scan{
				"govulncheck": ran("vulnscan: PASS\n", "0"), "trivy-floor": ran(floorOut, "0"),
			}
			maps.Copy(scans, tt.scans)
			dir := writeResults(t, scans)
			log := useGH(t, bin, tt.listed, tt.fail)
			headRepo := "octo/repo"
			switch tt.headRepo {
			case "-":
				headRepo = ""
			case "":
			default:
				headRepo = tt.headRepo
			}

			code, stdout := runSecreport(t, args(dir, "-pr", "7", "-repo", "octo/repo",
				"-head-repo", headRepo)...)

			if code != tt.wantExit {
				t.Errorf("exit = %d, want %d", code, tt.wantExit)
			}
			if !strings.Contains(stdout, tt.wantStdout) {
				t.Errorf("stdout does not contain %q", tt.wantStdout)
			}
			calls := readGHLog(t, log)
			var gotArgs [][]string
			for _, c := range calls {
				gotArgs = append(gotArgs, c.args)
			}
			if !slices.EqualFunc(gotArgs, tt.wantCalls, slices.Equal) {
				t.Errorf("gh calls:\n%q\nwant:\n%q", gotArgs, tt.wantCalls)
			}
			if tt.wantBody != "" && (len(calls) == 0 ||
				!strings.HasPrefix(calls[len(calls)-1].stdin, tt.wantBody)) {
				t.Errorf("the comment does not start with %q:\n%v", tt.wantBody, calls)
			}
		})
	}
}

func TestCommentFitsGitHub(t *testing.T) {
	long := strings.Repeat("GO-2026-0001  FAIL, called, not accepted\n", 5000)
	dir := writeResults(t, map[string]scan{
		"govulncheck": ran(long, "1"), "trivy-floor": ran(long, "1"),
	})
	log := useGH(t, ghBin(t), "", "")

	code, _ := runSecreport(t, args(dir, "-pr", "7", "-repo", "octo/repo", "-head-repo",
		"octo/repo")...)

	calls := readGHLog(t, log)
	if code != exitFailed || len(calls) != 2 {
		t.Fatalf("exit %d and %d gh calls, want 1 and 2", code, len(calls))
	}
	// The stub's log adds nothing to the body but a final newline.
	if body := calls[1].stdin; len(body) > commentLimit+1 ||
		!strings.HasPrefix(body, "<!-- secreport:vuln -->\n## Vulnerabilities: FAIL\n") {
		t.Errorf("the comment has %d bytes, more than %d, or does not start with the marker",
			len(body), commentLimit)
	}
}

func TestNoGH(t *testing.T) {
	dir := writeResults(t, map[string]scan{
		"govulncheck": ran(vulnOut, "1"), "trivy-floor": ran(floorOut, "0"),
	})
	t.Setenv("PATH", t.TempDir())

	code, stdout := runSecreport(t, args(dir, "-pr", "7", "-repo", "octo/repo", "-head-repo",
		"octo/repo")...)

	if code != exitDidNotRun || !strings.Contains(stdout, "listing its comments: gh api: exec: "+
		"\"gh\": executable file not found in $PATH") {
		t.Errorf("exit %d, stdout:\n%s\nwant exit 2 and that gh is missing", code, stdout)
	}
}
