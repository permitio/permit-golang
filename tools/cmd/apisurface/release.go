package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
)

// releaseResult is what gorelease found for a release.
type releaseResult struct {
	// base is the version gorelease compared with, or "none".
	base string
	// args are gorelease's arguments.
	args []string
	// incompatible lists the incompatible changes since base.
	incompatible []string
	// summary holds the lines of gorelease's summary.
	summary []string
	// allowed says that incompatible changes are allowed, because base is a prerelease.
	allowed bool
	// valid says that gorelease accepted the version.
	valid bool
}

// checkRelease runs gorelease for the release of opts.version by the base rules: with
// -base=none when no earlier tag of the module's major version exists; with -base=<tag> and no
// -version when the newest earlier tag is a prerelease, which allows incompatible changes; and
// otherwise with -base=<tag> -version=<version>, which fails on an incompatible change and on a
// version that does not follow from the changes. It returns an error, for exit 2, when the
// version or the tags cannot be read or gorelease does not complete.
func checkRelease(ctx context.Context, opts options, sys system, modulePath string,
) (releaseResult, error) {
	_, pathMajor, _ := module.SplitPathVersion(modulePath)
	if !semver.IsValid(opts.version) || semver.Canonical(opts.version) != opts.version ||
		module.CheckPathMajor(opts.version, pathMajor) != nil {
		return releaseResult{}, fmt.Errorf("-version %q is not a canonical semantic version of "+
			"the major version of %s, such as %s.0.0", opts.version, modulePath,
			nonEmpty(module.PathMajorPrefix(pathMajor), "v1"))
	}
	if opts.gorelease == "" {
		return releaseResult{}, errors.New("-gorelease names no gorelease binary; build it " +
			"with go -C tools build -o ../bin/gorelease golang.org/x/exp/cmd/gorelease")
	}
	tags, err := sys.git(ctx, opts.root, "tag", "--list")
	if err != nil {
		return releaseResult{}, err
	}
	res := releaseResult{base: "none"}
	for tag := range strings.FieldsSeq(string(tags)) {
		if semver.Canonical(tag) == tag && module.CheckPathMajor(tag, pathMajor) == nil &&
			semver.Compare(tag, opts.version) < 0 &&
			(res.base == "none" || semver.Compare(tag, res.base) > 0) {
			res.base = tag
		}
	}
	res.args = []string{"-base=" + res.base, "-version=" + opts.version}
	if res.base != "none" && semver.Prerelease(res.base) != "" {
		res.allowed = true
		res.args = res.args[:1]
	}
	out, stderr, err := sys.runIn(ctx, opts.root, sys.environ(), opts.gorelease, res.args...)
	code := 0
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		code = exitErr.ExitCode()
	} else if err != nil {
		return releaseResult{}, fmt.Errorf("running gorelease: %w", err)
	}
	res.incompatible, res.summary = parseGorelease(out)
	switch {
	case code == 0 && len(res.summary) > 0:
		res.valid = true
	case code == 1 && !res.allowed && containsLine(res.summary, "is not a valid semantic version"):
	default:
		return releaseResult{}, fmt.Errorf("gorelease %s exited %d without a verdict: %s",
			strings.Join(res.args, " "), code, strings.TrimSpace(string(out)+"\n"+string(stderr)))
	}
	return res, nil
}

// parseGorelease returns the incompatible changes in gorelease's output, the lines under each
// "## incompatible changes", and the lines of its "# summary".
func parseGorelease(out []byte) (incompatible, summary []string) {
	section := ""
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "# summary":
			section = line
		case strings.HasPrefix(line, "# ") && section != "# summary":
			section = ""
		case strings.HasPrefix(line, "## ") && section != "# summary":
			section = line
		case strings.TrimSpace(line) == "":
		case section == "## incompatible changes":
			incompatible = append(incompatible, line)
		case section == "# summary":
			summary = append(summary, line)
		}
	}
	return incompatible, summary
}

// countPending returns the number of lines of tools/migration/pending.txt under root that are
// not blank or comments.
func countPending(root string) (int, error) {
	f, err := os.Open(filepath.Join(root, pendingFile))
	if err != nil {
		return 0, err
	}
	defer f.Close()
	n := 0
	err = eachLine(f, func(int, string) { n++ })
	return n, err
}

// containsLine reports whether a line of lines contains s.
func containsLine(lines []string, s string) bool {
	for _, line := range lines {
		if strings.Contains(line, s) {
			return true
		}
	}
	return false
}
