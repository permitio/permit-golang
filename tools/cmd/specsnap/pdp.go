package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// containerSource is the name of the container PDP's spec, which -check-pdp compares.
const containerSource = "pdp-container"

// checkPDPArgs checks the flags that -check-pdp takes: the base URL of the container PDP, an
// http or https URL with a host and without a query or a fragment, and -image, when it is given,
// as -refresh pdp-container takes it.
func checkPDPArgs(opts options) error {
	u, err := url.Parse(opts.checkPDP)
	switch {
	case err != nil || u.Scheme != "http" && u.Scheme != "https" || u.Host == "" ||
		u.RawQuery != "" || u.Fragment != "" || u.User != nil:
		return fmt.Errorf("-check-pdp %q is not the base URL of a PDP, such as "+
			"http://127.0.0.1:7766", opts.checkPDP)
	case opts.image != "" && !imagePattern.MatchString(opts.image):
		return fmt.Errorf("-image %q is not %s:<tag>@sha256:<64 hex digits>", opts.image,
			pdpRepository)
	}
	return nil
}

// checkPDP fetches the spec that the container PDP at opts.checkPDP serves, makes its snapshot as
// refresh does, and compares it with the committed snapshot of the container PDP's spec, and
// opts.image, when it is given, with the image that lock.json records for it.
func checkPDP(ctx context.Context, opts options, stdout io.Writer, sys system) int {
	src, ok := findSource(sys.sources, containerSource)
	if !ok {
		return verdict(stdout, exitDidNotRun, "specsnap keeps no "+containerSource+" spec")
	}
	lock, err := readLock(opts.dir)
	if err != nil {
		return verdict(stdout, exitDidNotRun, err.Error())
	}
	entry, ok := lock[src.name]
	if !ok {
		return verdict(stdout, exitDidNotRun, fmt.Sprintf("%s has no entry for %s", lockFile,
			src.name))
	}
	snapshotOf := entry.Image + "@" + entry.Digest
	committed, err := os.ReadFile(filepath.Join(opts.dir, src.file()))
	if err != nil {
		return verdict(stdout, exitDidNotRun, fmt.Sprintf("reading the snapshot: %v", err))
	}
	doc, err := parse(committed)
	if err != nil {
		return verdict(stdout, exitDidNotRun, fmt.Sprintf("parsing %s: %v", src.file(), err))
	}
	specURL := strings.TrimSuffix(opts.checkPDP, "/") + "/openapi.json"
	raw, err := fetch(ctx, sys, specURL)
	if err != nil {
		return verdict(stdout, exitDidNotRun, fmt.Sprintf("reading the PDP's spec: %v", err))
	}
	live, err := makeSnapshot(src, raw)
	if err != nil {
		return verdict(stdout, exitCode(err), fmt.Sprintf("the spec at %s: %v", specURL, err))
	}
	var problems []string
	if opts.image != "" && opts.image != snapshotOf {
		problems = append(problems, fmt.Sprintf("the PDP runs %s, and %s is the snapshot of %s",
			opts.image, src.file(), snapshotOf))
	}
	if !bytes.Equal(live.data, committed) {
		line, got, want := firstDifference(live.data, committed)
		problems = append(problems, fmt.Sprintf("the spec at %s is not %s, the snapshot of %s: "+
			"its snapshot has sha256 %s and %s, and %s has sha256 %s and %s; they first differ "+
			"at line %d, %q in the PDP's and %q in %s", specURL, src.file(), snapshotOf,
			sha256Hex(live.data), count(live.operations, "operation"), src.file(),
			sha256Hex(committed), count(countOperations(doc), "operation"), line, got, want,
			src.file()))
	}
	if len(problems) > 0 {
		return verdict(stdout, exitFailed, strings.Join(problems, "; "))
	}
	return verdict(stdout, exitPassed, fmt.Sprintf("the PDP at %s serves the spec of %s, the "+
		"snapshot of %s", opts.checkPDP, src.file(), snapshotOf))
}

// endOfFile stands for a line past the end of a file, in what firstDifference returns.
const endOfFile = "<end of file>"

// firstDifference returns the number of the first line, counted from 1, in which a and b differ,
// and that line of each, without its indent, or endOfFile past the end of one of them. For a and
// b that are equal, it returns 0.
func firstDifference(a, b []byte) (int, string, string) {
	aLines := strings.Split(string(a), "\n")
	bLines := strings.Split(string(b), "\n")
	for i := range max(len(aLines), len(bLines)) {
		if i >= len(aLines) || i >= len(bLines) || aLines[i] != bLines[i] {
			return i + 1, lineText(aLines, i), lineText(bLines, i)
		}
	}
	return 0, "", ""
}

// lineText returns lines[i] without its indent, or endOfFile when lines has no line i.
func lineText(lines []string, i int) string {
	if i < len(lines) {
		return strings.TrimSpace(lines[i])
	}
	return endOfFile
}
