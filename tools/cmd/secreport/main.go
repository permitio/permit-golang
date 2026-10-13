// Command secreport reports the results of the security scans of a CI job and decides the job's
// exit code from them: the job summary, the annotations, a comment on the pull request and the
// verdict all come from it.
//
// Usage:
//
//	secreport -id id -title title -dir dir [-summary file] [-run-url url]
//	    [-pr number -repo owner/name -head-repo owner/name] name...
//
// Each name is a scan the job ran, in the order the report lists them. The scan wrote what it
// printed to <dir>/<name>.out and its exit code to <dir>/<name>.exit. A scan passed when it exited
// 0 and failed when it exited 1. It did not run when it exited with any other code, when either
// file is missing, when its exit code cannot be read, or when it printed nothing.
//
// secreport prints the result of each scan and, for each scan that did not pass, a workflow command
// that annotates the run with an error. It never prints what a scan printed, so the workflow
// commands it prints are its own. With -summary, it appends the report to that file, the job
// summary: a table of the results and the output of each scan, in code blocks.
//
// With -pr, on a pull request whose head repository, -head-repo, is -repo, secreport keeps one
// comment on the pull request up to date with the same report. It posts the comment when a scan
// failed or did not run, and on every later run it updates the comment it posted, so that the
// comment shows when the pull request passes. When every scan passed and there is no comment to
// update, it posts none: findings that a scan reports without failing, such as vulnerabilities in
// the standard library, reach the job summary only. A pull request from a fork gets no comment,
// since the token of its run cannot write one; secreport annotates the run with a notice instead.
// The comment starts with the marker <!-- secreport:<id> -->, by which secreport finds it among the
// comments of github-actions[bot]. secreport reads and writes comments with gh api, which takes its
// token from GH_TOKEN.
//
// Exit codes:
//
//	0  every scan passed
//	1  a scan failed and every scan ran
//	2  a scan did not run, the command line is wrong, or the job summary or the comment could not
//	   be written
//
// The last line secreport prints is its verdict: PASS, FAIL or DID NOT RUN. Build it before use,
// because go run reports every non-zero exit as 1:
//
//	go -C tools build -o ../bin/secreport ./cmd/secreport
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	exitPassed    = 0
	exitFailed    = 1
	exitDidNotRun = 2
)

const (
	// commentLimit is the most bytes a pull request comment may hold: GitHub accepts 65536
	// characters, and a character is at least one byte.
	commentLimit = 65536
	// summaryLimit is the most bytes secreport appends to the job summary; GitHub accepts 1 MiB
	// for each step.
	summaryLimit = 1 << 20
	// bot is the login of the user that the GITHUB_TOKEN of a workflow run acts as.
	bot = "github-actions[bot]"
)

var (
	// namePattern matches a report id and a scan name.
	namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	// repoPattern matches a repository's owner/name; a name is not . or .., which validRepo
	// checks.
	repoPattern = regexp.MustCompile(`^[A-Za-z0-9-]+/([A-Za-z0-9_.-]+)$`)
	// numberPattern matches a pull request number and a comment ID.
	numberPattern = regexp.MustCompile(`^[1-9][0-9]*$`)
)

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

// options is the command line.
type options struct {
	id, title, dir, summary, runURL string
	pr, repo, headRepo              string
	names                           []string
}

// run parses the command line, reads the results, writes the report and returns the exit code.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	opts, err := parseArgs(args, stderr)
	if err != nil {
		fmt.Fprintf(stdout, "secreport: DID NOT RUN: %v\nsecreport: DID NOT RUN\n", err)
		return exitDidNotRun
	}
	results := make([]result, 0, len(opts.names))
	for _, name := range opts.names {
		results = append(results, readResult(opts.dir, name))
	}
	code := exitPassed
	for _, r := range results {
		code = max(code, r.verdict())
		fmt.Fprintf(stdout, "secreport: %s: %s\n", r.name, r.describe())
	}
	for _, r := range results {
		if r.verdict() != exitPassed {
			fmt.Fprintf(stdout, "::error title=%s::%s\n", escapeProperty(opts.title),
				escapeData(r.name+": "+r.describe()+"; the job summary has its output"))
		}
	}
	if opts.summary != "" {
		if err := appendSummary(opts, results, code); err != nil {
			fmt.Fprintf(stdout, "secreport: writing the job summary failed: %v\n", err)
			code = exitDidNotRun
		}
	}
	if opts.pr != "" {
		if err := comment(ctx, stdout, opts, results, code); err != nil {
			fmt.Fprintf(stdout, "secreport: the comment on pull request %s could not be "+
				"written: %v\n", opts.pr, err)
			code = exitDidNotRun
		}
	}
	fmt.Fprintf(stdout, "secreport: %s: %s\n", opts.title, verdicts[code])
	return code
}

// verdicts names each exit code.
var verdicts = map[int]string{
	exitPassed:    "PASS",
	exitFailed:    "FAIL",
	exitDidNotRun: "DID NOT RUN",
}

func parseArgs(args []string, stderr io.Writer) (options, error) {
	var opts options
	flags := flag.NewFlagSet("secreport", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&opts.id, "id", "", "the report's `id`, which marks its pull request comment")
	flags.StringVar(&opts.title, "title", "", "the report's `title`")
	flags.StringVar(&opts.dir, "dir", "", "the `directory` that holds each scan's .out and .exit")
	flags.StringVar(&opts.summary, "summary", "", "the job summary `file` to append the report to")
	flags.StringVar(&opts.runURL, "run-url", "", "the `URL` of the workflow run")
	flags.StringVar(&opts.pr, "pr", "", "the pull request `number` to comment on")
	flags.StringVar(&opts.repo, "repo", "", "the `owner/name` of the repository")
	flags.StringVar(&opts.headRepo, "head-repo", "",
		"the `owner/name` of the pull request's head repository; empty when it was deleted")
	if err := flags.Parse(args); err != nil {
		return options{}, err
	}
	opts.names = flags.Args()
	switch {
	case !namePattern.MatchString(opts.id):
		return options{}, fmt.Errorf("-id %q is not lowercase letters, digits and dashes", opts.id)
	case opts.title == "" || strings.ContainsAny(opts.title, "\r\n"):
		return options{}, errors.New("-title must be one line of text")
	case opts.dir == "":
		return options{}, errors.New("-dir is missing")
	case len(opts.names) == 0:
		return options{}, errors.New("no scan names: name each scan the job ran")
	case opts.runURL != "" && !strings.HasPrefix(opts.runURL, "https://"):
		return options{}, fmt.Errorf("-run-url %q is not an https URL", opts.runURL)
	}
	seen := map[string]bool{}
	for _, name := range opts.names {
		if !namePattern.MatchString(name) {
			return options{}, fmt.Errorf("the scan name %q is not lowercase letters, digits and "+
				"dashes", name)
		}
		if seen[name] {
			return options{}, fmt.Errorf("the scan name %s is given twice", name)
		}
		seen[name] = true
	}
	if opts.pr == "" {
		if opts.repo != "" || opts.headRepo != "" {
			return options{}, errors.New("-repo and -head-repo need -pr")
		}
		return opts, nil
	}
	switch {
	case !numberPattern.MatchString(opts.pr):
		return options{}, fmt.Errorf("-pr %q is not a pull request number", opts.pr)
	case !validRepo(opts.repo):
		return options{}, fmt.Errorf("-repo %q is not owner/name", opts.repo)
	case opts.headRepo != "" && !validRepo(opts.headRepo):
		return options{}, fmt.Errorf("-head-repo %q is not owner/name", opts.headRepo)
	}
	return opts, nil
}

// validRepo reports whether s is a repository's owner/name.
func validRepo(s string) bool {
	m := repoPattern.FindStringSubmatch(s)
	return m != nil && m[1] != "." && m[1] != ".."
}

// result is what one scan left in the results directory.
type result struct {
	name   string
	code   int
	output string
	// problem says why the scan did not run when its files say so; it is empty otherwise.
	problem string
}

func readResult(dir, name string) result {
	r := result{name: name}
	exit, err := os.ReadFile(filepath.Join(dir, name+".exit"))
	if err != nil {
		r.problem = fmt.Sprintf("it did not finish: %v", err)
		return r
	}
	r.code, err = strconv.Atoi(strings.TrimSpace(string(exit)))
	if err != nil {
		r.problem = fmt.Sprintf("its exit code file holds %q, not a number",
			strings.TrimSpace(string(exit)))
		return r
	}
	output, err := os.ReadFile(filepath.Join(dir, name+".out"))
	if err != nil {
		r.problem = fmt.Sprintf("its output is missing: %v", err)
		return r
	}
	r.output = string(output)
	if strings.TrimSpace(r.output) == "" {
		r.problem = "it printed nothing"
	}
	return r
}

// verdict returns the exit code the scan stands for.
func (r result) verdict() int {
	switch {
	case r.problem != "":
		return exitDidNotRun
	case r.code == exitPassed || r.code == exitFailed:
		return r.code
	}
	return exitDidNotRun
}

// describe returns the scan's verdict, with its exit code or what is wrong with its files when it
// did not pass.
func (r result) describe() string {
	switch {
	case r.problem != "":
		return "DID NOT RUN: " + r.problem
	case r.code == exitPassed:
		return "PASS"
	}
	return fmt.Sprintf("%s (exit %d)", verdicts[r.verdict()], r.code)
}

// escapeData escapes the message of a workflow command.
func escapeData(s string) string {
	return strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A").Replace(s)
}

// escapeProperty escapes a property value of a workflow command.
func escapeProperty(s string) string {
	return strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A", ":", "%3A", ",", "%2C").
		Replace(s)
}

// render returns the report in Markdown: the verdict, a table of the results and the output of
// each scan. When the report would be longer than limit bytes, each output is cut to fit.
func render(opts options, results []result, code, limit int) string {
	var head strings.Builder
	fmt.Fprintf(&head, "## %s: %s\n\n| Scan | Result |\n| --- | --- |\n", opts.title,
		verdicts[code])
	for _, r := range results {
		fmt.Fprintf(&head, "| %s | %s |\n", r.name, strings.ReplaceAll(r.describe(), "|", `\|`))
	}
	tail := ""
	if opts.runURL != "" {
		tail = fmt.Sprintf("\nThe [workflow run](%s) has the full output of every scan.\n",
			opts.runURL)
	}
	outputs := 0
	for _, r := range results {
		outputs += len(r.output)
	}
	// Each output gets an equal share of what the table, the details tags and the code fences
	// leave, with room for the note that says it was cut.
	share := outputs
	const perScan = 200
	if room := limit - head.Len() - len(tail) - len(results)*perScan; outputs > room {
		share = max(room, 0) / max(len(results), 1)
	}
	var b strings.Builder
	b.WriteString(head.String())
	for _, r := range results {
		open := ""
		if r.verdict() != exitPassed {
			open = " open"
		}
		output, cut := cutOutput(r.output, share)
		fence := strings.Repeat("`", max(3, longestRun(output, '`')+1))
		fmt.Fprintf(&b, "\n<details%s><summary>%s: %s</summary>\n\n%stext\n%s", open, r.name,
			verdicts[r.verdict()], fence, output)
		if output != "" && !strings.HasSuffix(output, "\n") {
			b.WriteString("\n")
		}
		if cut > 0 {
			fmt.Fprintf(&b, "... %d more bytes; the job log has the full output\n", cut)
		}
		fmt.Fprintf(&b, "%s\n\n</details>\n", fence)
	}
	b.WriteString(tail)
	return b.String()
}

// cutOutput returns at most limit bytes of the start of output, cut after a line when it can be,
// and how many bytes it left out.
func cutOutput(output string, limit int) (string, int) {
	if len(output) <= limit {
		return output, 0
	}
	kept := output[:limit]
	if i := strings.LastIndexByte(kept, '\n'); i >= 0 {
		kept = kept[:i+1]
	}
	for kept != "" && !utf8.ValidString(kept) {
		kept = kept[:len(kept)-1]
	}
	return kept, len(output) - len(kept)
}

// longestRun returns the length of the longest run of c in s.
func longestRun(s string, c byte) int {
	longest, current := 0, 0
	for i := range len(s) {
		if s[i] != c {
			current = 0
			continue
		}
		current++
		longest = max(longest, current)
	}
	return longest
}

// appendSummary appends the report to the job summary file.
func appendSummary(opts options, results []result, code int) error {
	file, err := os.OpenFile(opts.summary, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, err = io.WriteString(file, render(opts, results, code, summaryLimit)+"\n")
	return errors.Join(err, file.Close())
}

// comment keeps the report's comment on the pull request up to date: it creates the comment when
// a scan did not pass, and updates the comment secreport posted before on any run.
func comment(ctx context.Context, stdout io.Writer, opts options, results []result,
	code int,
) error {
	if opts.headRepo != opts.repo {
		fmt.Fprintf(stdout, "::notice title=%s::%s\n", escapeProperty(opts.title),
			escapeData("This pull request comes from a fork, and the token of its run cannot "+
				"comment on it; the job summary has the report."))
		return nil
	}
	marker := "<!-- secreport:" + opts.id + " -->"
	filter := fmt.Sprintf(`.[] | select(.user.login == %q and (.body | startswith(%q))) | .id`,
		bot, marker)
	listed, err := gh(ctx, "", "api", "--paginate", "--jq", filter,
		"repos/"+opts.repo+"/issues/"+opts.pr+"/comments")
	if err != nil {
		return fmt.Errorf("listing its comments: %w", err)
	}
	ids := strings.Fields(listed)
	body := marker + "\n" + render(opts, results, code, commentLimit-len(marker)-1)
	switch {
	case len(ids) > 0 && !numberPattern.MatchString(ids[0]):
		return fmt.Errorf("gh listed the comment ID %q, which is not a number", ids[0])
	case len(ids) > 0:
		_, err = gh(ctx, body, "api", "--method", "PATCH",
			"repos/"+opts.repo+"/issues/comments/"+ids[0], "-F", "body=@-")
		if err != nil {
			return fmt.Errorf("updating comment %s: %w", ids[0], err)
		}
		fmt.Fprintf(stdout, "secreport: updated comment %s on pull request %s\n", ids[0], opts.pr)
	case code == exitPassed:
		fmt.Fprintf(stdout, "secreport: every scan passed, and pull request %s has no comment "+
			"of this report to update\n", opts.pr)
	default:
		_, err = gh(ctx, body, "api", "--method", "POST",
			"repos/"+opts.repo+"/issues/"+opts.pr+"/comments", "-F", "body=@-")
		if err != nil {
			return fmt.Errorf("posting it: %w", err)
		}
		fmt.Fprintf(stdout, "secreport: posted a comment on pull request %s\n", opts.pr)
	}
	return nil
}

// gh runs gh with args and stdin, and returns what it printed, or an error with what it printed
// on stderr.
func gh(ctx context.Context, stdin string, args ...string) (string, error) {
	//nolint:gosec // G204: the arguments are API paths built from the validated command line.
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("gh %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}
