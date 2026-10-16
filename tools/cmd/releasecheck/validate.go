package main

import (
	"bufio"
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// The files validate reads, relative to the repository root.
const (
	versionFile   = "version.go"
	changelogFile = "CHANGELOG.md"
	ciFile        = ".github/workflows/ci.yml"
	pendingFile   = "tools/e2e/pending.txt"
	localOnlyFile = "tools/e2e/local-only.txt"
)

var (
	// sha256Pattern matches a SHA-256 hash in hex.
	sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
	// laneVar matches the name of a variable of ci.yml's env that names the Go release of a test
	// leg, and exactGo its value.
	laneVar = regexp.MustCompile(`^GO_(FLOOR|[0-9]+)$`)
	exactGo = regexp.MustCompile(`^1\.[0-9]+\.[0-9]+$`)
)

// validateInputs is what validate checks.
type validateInputs struct {
	tag, sha, master, version string
	ciRuns                    []workflowRun
	changelog                 []string
	pending                   []string
	// localOnly maps each phase of tools/e2e/local-only.txt to its methods.
	localOnly map[string][]string
	lanes     []string
	evidence  evidence
}

// workflowRun is a run of a workflow, as the GitHub API lists it: its status and conclusion are
// those of its latest attempt.
type workflowRun struct {
	ID         int64  `json:"id"`
	RunAttempt int    `json:"run_attempt"`
	HeadSHA    string `json:"head_sha"`
	Event      string `json:"event"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
}

// evidence is the harness evidence: the summary of the report of the maintainer-run local
// full-stack suite.
type evidence struct {
	SDKCommit    string   `json:"sdk_commit"`
	ReportSHA256 string   `json:"report_sha256"`
	Lanes        []string `json:"lanes"`
	PassedPhases []string `json:"passed_phases"`
}

// errNoEvidence says that the harness evidence is empty.
var errNoEvidence = errors.New("the harness evidence is empty: pass the summary of the local " +
	"suite report as harness_evidence, a JSON object with sdk_commit, report_sha256, lanes and " +
	"passed_phases")

func runValidate(args []string, stdout, stderr io.Writer) int {
	var root, tag, sha, master, ciRuns, evidenceFile string
	flags := newFlags("validate", stderr, &root)
	flags.StringVar(&tag, "tag", "", "the release's `tag`, such as v2.1.0")
	flags.StringVar(&sha, "sha", "", "the `commit` the release workflow runs on")
	flags.StringVar(&master, "master", "", "the head `commit` of master")
	flags.StringVar(&ciRuns, "ci-runs", "", "the runs of ci.yml for a push of the commit, a "+
		"JSON `file`")
	flags.StringVar(&evidenceFile, "evidence", "", "the harness evidence, a JSON `file`")
	err := parse(flags, args, map[string]*string{
		"tag": &tag, "sha": &sha, "master": &master, "ci-runs": &ciRuns, "evidence": &evidenceFile,
	})
	if err != nil {
		return abort(stdout, err)
	}
	in, err := readValidateInputs(root, ciRuns, evidenceFile)
	if err != nil {
		return abort(stdout, err)
	}
	for name, value := range map[string]string{"-sha": sha, "-master": master} {
		if !commitPattern.MatchString(value) {
			return abort(stdout, fmt.Errorf("%s %q is not a full commit hash", name, value))
		}
	}
	in.tag, in.sha, in.master = tag, sha, master
	r := &report{out: stdout}
	validate(r, in)
	return r.finish()
}

// readValidateInputs reads the files that validate checks.
func readValidateInputs(root, ciRuns, evidenceFile string) (validateInputs, error) {
	var in validateInputs
	var err error
	if in.version, err = readVersion(filepath.Join(root, versionFile)); err != nil {
		return in, err
	}
	if in.changelog, err = readLines(filepath.Join(root, changelogFile), false); err != nil {
		return in, err
	}
	if in.pending, err = readLines(filepath.Join(root, pendingFile), true); err != nil {
		return in, err
	}
	if in.localOnly, err = readLocalOnly(filepath.Join(root, localOnlyFile)); err != nil {
		return in, err
	}
	if in.lanes, err = readLanes(filepath.Join(root, filepath.FromSlash(ciFile))); err != nil {
		return in, err
	}
	if in.ciRuns, err = readWorkflowRuns(ciRuns); err != nil {
		return in, err
	}
	in.evidence, err = readEvidence(evidenceFile)
	return in, err
}

// validate checks the release before the gates run.
func validate(r *report, in validateInputs) {
	candidate, tagOK := checkTag(r, in.tag, in.version)
	if in.sha == in.master {
		r.passf("%s is the head of master", in.sha)
	} else {
		r.failf("the workflow runs on %s, which is not the head of master, %s: dispatch the "+
			"release on master, and again when master has moved", in.sha, in.master)
	}
	checkCI(r, in.sha, in.ciRuns)
	// Whether the version's section may still say (unreleased) depends on the tag, so a tag that
	// failed its check leaves only the section to check.
	checkChangelog(r, in.changelog, in.version, candidate || !tagOK)
	if len(in.pending) == 0 {
		r.passf("%s has no method left", pendingFile)
	} else {
		r.failf("%s still lists %s, such as %s: a release needs an end-to-end scenario, or a "+
			"local-only or advisory line, for each", pendingFile, plural(len(in.pending), "method"),
			strings.Fields(in.pending[0])[0])
	}
	checkEvidence(r, in)
}

// checkTag checks the tag against the tag pattern and the committed version. It reports whether
// the tag is a release candidate, and whether it passed.
func checkTag(r *report, tag, version string) (candidate, ok bool) {
	m := tagPattern.FindStringSubmatch(tag)
	if m == nil {
		r.failf("the tag %q is not a v2 release tag as a whole string, v2.<minor>.<patch> or "+
			"v2.<minor>.<patch>-rc.<n>, such as v2.1.0 or v2.1.0-rc.1", tag)
		return false, false
	}
	candidate = m[3] != ""
	if base := strings.TrimSuffix(tag, m[3]); base != "v"+version {
		r.failf("the tag %s is not v%s, the Version of %s, or a release candidate of it, "+
			"v%s-rc.<n>: change Version in a pull request, or release that version", tag,
			version, versionFile, version)
		return candidate, false
	}
	r.passf("the tag %s is a v2 release tag of the Version of %s, %s", tag, versionFile, version)
	return candidate, true
}

// checkCI checks that CI succeeded for the commit sha on its push to master: of the runs of
// ci.yml, the newest run for a push of sha completed with success in its latest attempt. A run of
// another event, such as a scheduled or manual run, which skips the pull request checks or some
// of them, does not count, and neither do an earlier attempt and an older run.
func checkCI(r *report, sha string, runs []workflowRun) {
	var pushes []workflowRun
	var others []string
	for _, run := range runs {
		if run.HeadSHA == sha && run.Event == "push" {
			pushes = append(pushes, run)
			continue
		}
		others = append(others, fmt.Sprintf("%s run %d for %s, %s %s", run.Event, run.ID,
			run.HeadSHA, run.Status, run.Conclusion))
	}
	if len(pushes) == 0 {
		found := "none"
		if len(others) > 0 {
			found = strings.Join(others, "; ")
		}
		r.failf("CI did not succeed for %s: ci.yml has no run for a push of it (its other "+
			"runs: %s); wait for CI of the push to master", sha, found)
		return
	}
	latest := slices.MaxFunc(pushes, func(a, b workflowRun) int {
		return cmp.Or(cmp.Compare(a.ID, b.ID), cmp.Compare(a.RunAttempt, b.RunAttempt))
	})
	if latest.Status == "completed" && latest.Conclusion == "success" {
		r.passf("CI succeeded for %s on its push to master, in run %d, attempt %d", sha,
			latest.ID, latest.RunAttempt)
		return
	}
	r.failf("CI did not succeed for %s: the latest run of ci.yml for a push of it, run %d, "+
		"attempt %d, is %s with the conclusion %q, not completed with success; wait for it, or "+
		"fix it", sha, latest.ID, latest.RunAttempt, latest.Status, latest.Conclusion)
}

// checkChangelog checks that CHANGELOG.md has a section for the version, which no longer says
// (unreleased) unless unreleased allows it, as for a release candidate.
func checkChangelog(r *report, lines []string, version string, unreleased bool) {
	heading := "## v" + version
	for _, line := range lines {
		if line != heading && !strings.HasPrefix(line, heading+" (") {
			continue
		}
		if !unreleased && strings.Contains(line, "(unreleased)") {
			r.failf("the heading %q of %s still says (unreleased); write the day of the release "+
				"in its place, such as %s (2026-11-02)", line, changelogFile, heading)
			return
		}
		r.passf("%s has the section %q", changelogFile, line)
		return
	}
	r.failf("%s has no section for the release, a heading %q; write its entry", changelogFile,
		heading)
}

// checkEvidence checks the harness evidence: it is of the release commit, its lanes include each
// Go release that CI's test legs run, and its passed phases include each local-only phase.
func checkEvidence(r *report, in validateInputs) {
	ev := in.evidence
	fmt.Fprintf(r.out, "releasecheck: harness evidence: commit %s, report sha256 %s, lanes %s, "+
		"passed phases %s; the reviewer of the release environment confirms the sha256 against "+
		"the report\n", ev.SDKCommit, ev.ReportSHA256, strings.Join(ev.Lanes, " "),
		strings.Join(ev.PassedPhases, " "))
	if ev.SDKCommit != in.sha {
		r.failf("the harness evidence is of the commit %s, not of the release commit %s: run "+
			"the local full-stack suite on the release commit", ev.SDKCommit, in.sha)
	}
	var missing []string
	for _, lane := range in.lanes {
		if !slices.Contains(ev.Lanes, lane) {
			missing = append(missing, lane)
		}
	}
	if len(missing) > 0 {
		r.failf("the harness evidence has no lane %s: the local full-stack suite runs on the Go "+
			"releases of CI's test legs, %s", strings.Join(missing, ", "),
			strings.Join(in.lanes, ", "))
	}
	for _, phase := range slices.Sorted(maps.Keys(in.localOnly)) {
		if !slices.Contains(ev.PassedPhases, phase) {
			r.failf("the phase %s, which proves %s of %s, is not in the harness evidence's "+
				"passed_phases", phase, strings.Join(in.localOnly[phase], ", "), localOnlyFile)
		}
	}
	if ev.SDKCommit == in.sha && len(missing) == 0 {
		r.passf("the harness evidence is of %s, on the lanes %s", in.sha,
			strings.Join(in.lanes, ", "))
	}
}

// readVersion returns the value of the constant Version in the Go file.
func readVersion(file string) (string, error) {
	f, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.SkipObjectResolution)
	if err != nil {
		return "", err
	}
	for _, decl := range f.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range value.Names {
				if name.Name != "Version" || i >= len(value.Values) {
					continue
				}
				if lit, ok := value.Values[i].(*ast.BasicLit); ok && lit.Kind == token.STRING {
					return strconv.Unquote(lit.Value)
				}
			}
		}
	}
	return "", fmt.Errorf("%s declares no constant Version with a string value", file)
}

// readLines returns the lines of the file; with content, only those that are not blank or
// comments, whose first field starts with #, as the end-to-end completeness gate reads its lists.
func readLines(file string, content bool) ([]string, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var lines []string
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if content && (len(fields) == 0 || strings.HasPrefix(fields[0], "#")) {
			continue
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading %s: %w", file, err)
	}
	return lines, nil
}

// readLocalOnly reads tools/e2e/local-only.txt: <Method> <reason> <phase> per line. It returns
// the methods of each phase.
func readLocalOnly(file string) (map[string][]string, error) {
	lines, err := readLines(file, true)
	if err != nil {
		return nil, err
	}
	phases := map[string][]string{}
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) != 3 {
			return nil, fmt.Errorf("%s: %q is not <Method> <reason> <phase>", file, line)
		}
		phases[fields[2]] = append(phases[fields[2]], fields[0])
	}
	return phases, nil
}

// readLanes returns the Go release of each test leg of CI: go plus the value of each GO_FLOOR and
// GO_<n> variable of the env of ci.yml.
func readLanes(file string) ([]string, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var workflow struct {
		Env map[string]string `yaml:"env"`
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	var lanes []string
	for name, value := range workflow.Env {
		if !laneVar.MatchString(name) {
			continue
		}
		if !exactGo.MatchString(value) {
			return nil, fmt.Errorf("%s: %s is %q, not an exact Go release such as 1.27.1", file,
				name, value)
		}
		lanes = append(lanes, "go"+value)
	}
	if len(lanes) == 0 {
		return nil, fmt.Errorf("%s: its env has no GO_FLOOR or GO_<n> variable, the Go releases "+
			"of the test legs", file)
	}
	slices.Sort(lanes)
	return lanes, nil
}

// readWorkflowRuns reads the runs of a workflow, as the GitHub API lists them.
func readWorkflowRuns(file string) ([]workflowRun, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var listed struct {
		WorkflowRuns *[]workflowRun `json:"workflow_runs"`
	}
	if err := json.Unmarshal(data, &listed); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	if listed.WorkflowRuns == nil {
		return nil, fmt.Errorf("%s has no workflow_runs: it is not a list of workflow runs", file)
	}
	for _, run := range *listed.WorkflowRuns {
		if run.ID <= 0 || run.RunAttempt <= 0 {
			return nil, fmt.Errorf("%s lists a run without an id or a run_attempt: it is not a "+
				"list of workflow runs", file)
		}
	}
	return *listed.WorkflowRuns, nil
}

// readEvidence reads the harness evidence.
func readEvidence(file string) (evidence, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return evidence{}, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return evidence{}, errNoEvidence
	}
	var ev evidence
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&ev); err != nil {
		return evidence{}, fmt.Errorf("the harness evidence is not a JSON object with "+
			"sdk_commit, report_sha256, lanes and passed_phases: %w", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return evidence{}, errors.New("the harness evidence holds more than one JSON value")
	}
	switch {
	case !commitPattern.MatchString(ev.SDKCommit):
		return evidence{}, fmt.Errorf("the harness evidence's sdk_commit %q is not a full "+
			"commit hash", ev.SDKCommit)
	case !sha256Pattern.MatchString(ev.ReportSHA256):
		return evidence{}, fmt.Errorf("the harness evidence's report_sha256 %q is not a "+
			"SHA-256 hash in hex", ev.ReportSHA256)
	}
	return ev, nil
}
