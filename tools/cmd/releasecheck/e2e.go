package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// requiredFile lists the tests that every run of the end-to-end scenarios against the PDP
// container must pass; its subtests are the scenarios.
const requiredFile = "tools/e2e/required.txt"

// job is a job of a workflow run, as the GitHub API lists it.
type job struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
}

// recordLine is a line of a request record, of the fields that e2e reads.
type recordLine struct {
	Type       string `json:"type"`
	Mode       string `json:"mode"`
	ExitStatus *int   `json:"exit_status"`
}

// e2eRecord is what e2e counts in a request record.
type e2eRecord struct {
	// live counts the live requests.
	live int
	// exitStatus is the exit status of the session trailer, or nil without one.
	exitStatus *int
}

func runE2E(args []string, stdout, stderr io.Writer) int {
	var root, jobsFile, ciJob, e2eJob, recordFile string
	flags := newFlags("e2e", stderr, &root)
	flags.StringVar(&jobsFile, "jobs", "", "the jobs of the run, a JSON `file` of pages")
	flags.StringVar(&ciJob, "ci-job", "", "the `name` of the CI job of the called workflow")
	flags.StringVar(&e2eJob, "e2e-job", "", "the `name` of its end-to-end job")
	flags.StringVar(&recordFile, "record", "", "the request record `file` of the end-to-end job")
	err := parse(flags, args, map[string]*string{
		"jobs": &jobsFile, "ci-job": &ciJob, "e2e-job": &e2eJob, "record": &recordFile,
	})
	if err != nil {
		return abort(stdout, err)
	}
	jobs, err := readJobs(jobsFile)
	if err != nil {
		return abort(stdout, err)
	}
	sentinel, err := countScenarios(filepath.Join(root, filepath.FromSlash(requiredFile)))
	if err != nil {
		return abort(stdout, err)
	}
	rec, err := readRecord(recordFile)
	if err != nil {
		return abort(stdout, err)
	}
	r := &report{out: stdout}
	for _, name := range []string{ciJob, e2eJob} {
		checkJob(r, jobs, name)
	}
	checkRecord(r, rec, sentinel)
	return r.finish()
}

// checkJob checks that the run has the job name, which concluded success.
func checkJob(r *report, jobs []job, name string) {
	for _, j := range jobs {
		if j.Name != name {
			continue
		}
		if j.Conclusion == "success" {
			r.passf("the job %q succeeded", name)
			return
		}
		r.failf("the job %q is %s with the conclusion %q, not success: the release needs it to "+
			"succeed in this run", name, j.Status, j.Conclusion)
		return
	}
	r.failf("the run has no job %q, so the gate did not run", name)
}

// checkRecord checks that the record of the end-to-end job is of a session that passed and holds
// at least as many live requests as sentinel, the number of scenarios (a count sentinel: it
// counts the requests, not the scenarios that sent them).
func checkRecord(r *report, rec e2eRecord, sentinel int) {
	switch {
	case rec.exitStatus == nil:
		r.failf("the e2e record has no session trailer: its run did not finish")
	case *rec.exitStatus != 0:
		r.failf("the e2e record is of a run that exited %d", *rec.exitStatus)
	}
	if rec.live < sentinel {
		r.failf("the e2e record holds %d live requests, fewer than the %d scenarios of %s: the "+
			"scenarios did not run against Permit, or did not record", rec.live, sentinel,
			requiredFile)
		return
	}
	r.passf("the e2e record holds %d live requests, at least as many as the %d scenarios of %s "+
		"(a count sentinel)", rec.live, sentinel, requiredFile)
}

// readJobs reads the jobs of a run: one or more pages of the GitHub API's list, each a JSON
// object with jobs, one after the other, as gh api --paginate prints them.
func readJobs(file string) ([]job, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	var jobs []job
	pages := 0
	for {
		var page struct {
			Jobs *[]job `json:"jobs"`
		}
		err := dec.Decode(&page)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		if page.Jobs == nil {
			return nil, fmt.Errorf("%s: page %d has no jobs: it is not a list of a run's jobs",
				file, pages+1)
		}
		jobs = append(jobs, *page.Jobs...)
		pages++
	}
	if pages == 0 {
		return nil, fmt.Errorf("%s is empty: it is not a list of a run's jobs", file)
	}
	return jobs, nil
}

// countScenarios returns the number of subtests that the list of required tests names: the
// scenarios, each of which sends at least one request.
func countScenarios(file string) (int, error) {
	lines, err := readLines(file, true)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, line := range lines {
		if _, name, ok := strings.Cut(line, " "); ok && strings.Contains(name, "/") {
			n++
		}
	}
	if n == 0 {
		return 0, fmt.Errorf("%s names no subtest, so the e2e record has no count to reach",
			file)
	}
	return n, nil
}

// readRecord reads a request record in the JSONL format that internal/testhook describes.
func readRecord(file string) (e2eRecord, error) {
	f, err := os.Open(file)
	if err != nil {
		return e2eRecord{}, err
	}
	defer func() { _ = f.Close() }() // Read only; a close error loses nothing.
	var rec e2eRecord
	scanner := bufio.NewScanner(f)
	scanner.Buffer(nil, 1<<20)
	for n := 1; scanner.Scan(); n++ {
		var line recordLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			return e2eRecord{}, fmt.Errorf("%s:%d: %w", file, n, err)
		}
		switch line.Type {
		case "request":
			if line.Mode == "live" {
				rec.live++
			}
		case "session":
			if line.ExitStatus == nil {
				return e2eRecord{}, fmt.Errorf("%s:%d: a session trailer without exit_status",
					file, n)
			}
			rec.exitStatus = line.ExitStatus
		case "header":
		default:
			return e2eRecord{}, fmt.Errorf("%s:%d: a line of type %q, which a request record "+
				"does not hold", file, n, line.Type)
		}
	}
	if err := scanner.Err(); err != nil {
		return e2eRecord{}, fmt.Errorf("reading %s: %w", file, err)
	}
	return rec, nil
}
