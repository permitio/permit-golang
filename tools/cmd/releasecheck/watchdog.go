package main

import (
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/mod/semver"
)

// acceptedTagsFile lists the version tags that watchdog accepts at a commit, without the release
// that the rules ask for.
const acceptedTagsFile = ".github/accepted-tags.txt"

// lastV1FromMaster is the last v1 version released from master, before release/v1 held v1. A v1
// tag up to it needs CI of a push to master, and a later one CI of a push to release/v1.
const lastV1FromMaster = "v1.3.0"

// whatToDo ends a failure of watchdog: the section of CONTRIBUTING.md that says how to handle a
// tag created outside the release rules.
const whatToDo = "see Releases in CONTRIBUTING.md for what to do"

// The attestation of the module zip of a v2 release: build provenance, signed by release.yml run
// from master.
const (
	provenanceType = "https://slsa.dev/provenance/v1"
	releaseRef     = "refs/heads/master"
)

// tag is a tag of the repository, as the GitHub API lists it (repos/<repo>/tags): its name and
// the commit it names, an annotated tag's commit included.
type tag struct {
	Name   string `json:"name"`
	Commit struct {
		SHA string `json:"sha"`
	} `json:"commit"`
}

// release is a GitHub release, as the API lists it (repos/<repo>/releases).
type release struct {
	TagName string `json:"tag_name"`
	Draft   bool   `json:"draft"`
}

// zipCheck is what the first step of tag-watchdog.yml found for the module zip of a published v2
// release, permit-golang-<tag>.zip: the zip's SHA-256 and the attestations that gh attestation
// verify verified for it, or why the zip could not be downloaded or verified.
type zipCheck struct {
	Tag      string        `json:"tag"`
	SHA256   string        `json:"sha256"`
	Error    string        `json:"error"`
	Verified []attestation `json:"verified"`
}

// attestation is an attestation as gh attestation verify --format json prints it, with the
// fields that watchdog reads: those of its signing certificate, and the predicate type and
// subjects of its statement. gh names the fields in camel case.
type attestation struct {
	VerificationResult struct {
		Signature struct {
			Certificate certificate `json:"certificate"`
		} `json:"signature"`
		Statement struct {
			PredicateType string    `json:"predicateType"` //nolint:tagliatelle // gh's.
			Subject       []subject `json:"subject"`
		} `json:"statement"`
	} `json:"verificationResult"` //nolint:tagliatelle // gh's.
}

// certificate is the signing certificate of an attestation, with the fields that watchdog reads,
// which GitHub sets from the workflow run that signed it.
type certificate struct {
	BuildSignerURI         string `json:"buildSignerURI"`         //nolint:tagliatelle // gh's.
	SourceRepositoryRef    string `json:"sourceRepositoryRef"`    //nolint:tagliatelle // gh's.
	SourceRepositoryDigest string `json:"sourceRepositoryDigest"` //nolint:tagliatelle // gh's.
}

// subject is a subject of an attestation's statement: an artifact, by its name and its digests.
type subject struct {
	Name   string            `json:"name"`
	Digest map[string]string `json:"digest"`
}

// watchdogInputs is what watchdog checks.
type watchdogInputs struct {
	// repo is the repository, <owner>/<name>, whose release.yml signs the attestations.
	repo     string
	tags     []tag
	releases []release
	// zips maps the tag of each published v2 release to what was found for its module zip.
	zips map[string]zipCheck
	// releaseRuns are the runs of release.yml, and ciRuns the runs of ci.yml for a push.
	releaseRuns, ciRuns []workflowRun
	// accepted maps each tag of acceptedTagsFile to the commit it is accepted at.
	accepted map[string]string
}

func runWatchdog(args []string, stdout, stderr io.Writer) int {
	var root, repo, tagsFile, releaseRuns, releasesFile, ciRuns, zipsFile string
	flags := newFlags("watchdog", stderr, &root)
	flags.StringVar(&repo, "repo", "", "the `repository`, <owner>/<name>, whose release.yml "+
		"signs the attestations of the v2 releases")
	flags.StringVar(&tagsFile, "tags", "", "the tags of the repository, a JSON `file` of pages")
	flags.StringVar(&releaseRuns, "release-runs", "", "the runs of release.yml, a JSON `file` "+
		"of pages")
	flags.StringVar(&releasesFile, "releases", "", "the GitHub releases, a JSON `file` of pages")
	flags.StringVar(&ciRuns, "ci-runs", "", "the runs of ci.yml for a push of each v1 tag's "+
		"commit, a JSON `file` of pages")
	flags.StringVar(&zipsFile, "attestations", "", "the module zip of each published v2 "+
		"release and its verified attestations, a JSON `file`")
	err := parse(flags, args, map[string]*string{
		"repo": &repo, "tags": &tagsFile, "release-runs": &releaseRuns, "releases": &releasesFile,
		"ci-runs": &ciRuns, "attestations": &zipsFile,
	})
	if err != nil {
		return abort(stdout, err)
	}
	in := watchdogInputs{repo: repo}
	if in.tags, err = readTags(tagsFile); err != nil {
		return abort(stdout, err)
	}
	if in.releaseRuns, err = readWorkflowRuns(releaseRuns); err != nil {
		return abort(stdout, err)
	}
	if in.releases, err = readReleases(releasesFile); err != nil {
		return abort(stdout, err)
	}
	if in.ciRuns, err = readWorkflowRuns(ciRuns); err != nil {
		return abort(stdout, err)
	}
	if in.zips, err = readZipChecks(zipsFile); err != nil {
		return abort(stdout, err)
	}
	accepted := filepath.Join(root, filepath.FromSlash(acceptedTagsFile))
	if in.accepted, err = readAcceptedTags(accepted); err != nil {
		return abort(stdout, err)
	}
	r := &report{out: stdout}
	watchdog(r, in)
	return r.finish()
}

// watchdog checks that each version tag was published by the rules: a v2 tag by release.yml,
// which attested the module zip of its release; a v1 tag with a published GitHub release, once CI
// succeeded for a push of its commit to its branch; and any other tag at the commit that
// acceptedTagsFile accepts it at. A tag that is not a semantic version is no version of the
// module, and is not checked.
func watchdog(r *report, in watchdogInputs) {
	slices.SortFunc(in.tags, func(a, b tag) int {
		if c := semver.Compare(a.Name, b.Name); c != 0 {
			return c
		}
		return strings.Compare(a.Name, b.Name)
	})
	var ignored []string
	accepted := 0
	for _, t := range in.tags {
		commit, isAccepted := in.accepted[t.Name]
		switch {
		case !semver.IsValid(t.Name):
			ignored = append(ignored, t.Name)
		case isAccepted && commit == t.Commit.SHA:
			accepted++
		case isAccepted:
			r.failf("%s names %s, but %s accepts it at %s: a published version never moves; %s",
				t.Name, t.Commit.SHA, acceptedTagsFile, commit, whatToDo)
		case semver.Major(t.Name) == "v2":
			checkV2Tag(r, t, in)
		case semver.Major(t.Name) == "v1":
			checkV1Tag(r, t, in.releases, in.ciRuns)
		default:
			r.failf("%s at %s is neither a v1 nor a v2 version, and %s does not accept it: no "+
				"workflow of this repository publishes it; %s", t.Name, t.Commit.SHA,
				acceptedTagsFile, whatToDo)
		}
	}
	if len(ignored) > 0 {
		r.notef("not checked", "%s, which is not a semantic version such as v2.1.0, so no "+
			"version of the module", strings.Join(ignored, ", "))
	}
	r.passf("%s accepts %d of the tags, each at the commit it names", acceptedTagsFile, accepted)
}

// checkV2Tag checks that a published GitHub release of the v2 tag has a module zip that
// release.yml attested from master for the tag's commit. GitHub deletes workflow runs after the
// retention period and keeps attestations, so the runs of release.yml named release <tag>, from
// master, for the commit, only say why a tag has no such attestation: one in progress leaves the
// tag pending, as release.yml creates the tag in publish, before verify attests the zip; one that
// completed is reported with its conclusion; and without one, the tag was created outside
// release.yml.
func checkV2Tag(r *report, t tag, in watchdogInputs) {
	missing := missingAttestation(t, in)
	if missing == "" {
		r.passf("%s at %s: the module zip of its published release is attested by release.yml "+
			"from master for that commit", t.Name, t.Commit.SHA)
		return
	}
	title := "release " + t.Name
	var pending, succeeded, failed, others []string
	for _, run := range in.releaseRuns {
		ours := run.HeadSHA == t.Commit.SHA && run.HeadBranch == "master" &&
			run.Event == "workflow_dispatch"
		switch {
		case run.DisplayTitle != title:
		case ours && run.Status == "in_progress":
			pending = append(pending, fmt.Sprintf("run %d", run.ID))
		case ours && run.Status == "completed":
			if run.Conclusion == "success" {
				succeeded = append(succeeded, fmt.Sprintf("run %d", run.ID))
			} else {
				failed = append(failed, fmt.Sprintf("run %d for that commit concluded %s",
					run.ID, run.Conclusion))
			}
		default:
			others = append(others, fmt.Sprintf("run %d for %s on %s by %s, %s", run.ID,
				run.HeadSHA, run.HeadBranch, run.Event, runState(run)))
		}
	}
	switch {
	case len(pending) > 0:
		r.notef("pending", "%s at %s: %s, and its release %s of release.yml has not finished; "+
			"the next run of the watchdog checks it", t.Name, t.Commit.SHA, missing,
			strings.Join(pending, " and "))
	case len(succeeded) > 0:
		r.failf("%s at %s: %s, although its release %s of release.yml succeeded: the release or "+
			"its attestation changed after the run; %s", t.Name, t.Commit.SHA, missing,
			strings.Join(succeeded, " and "), whatToDo)
	case len(failed) > 0:
		advice := "re-run its failed jobs"
		if len(failed) > 1 {
			advice = "re-run the failed jobs of the one that published the tag"
		}
		r.failf("%s at %s: %s; the release %s: %s, or see Releases in CONTRIBUTING.md", t.Name,
			t.Commit.SHA, missing, strings.Join(failed, "; "), advice)
	default:
		found := ""
		if len(others) > 0 {
			found = ", only " + strings.Join(others, "; ")
		}
		r.failf("%s at %s: %s, and release.yml has no run named %q from master for that "+
			"commit%s: release.yml is the one way to publish a v2 version, so the tag was "+
			"created outside it; %s", t.Name, t.Commit.SHA, missing, title, found, whatToDo)
	}
}

// missingAttestation returns "" when a published GitHub release of the v2 tag has a module zip
// with an attestation that release.yml signed from master for the tag's commit, and otherwise
// what is missing.
func missingAttestation(t tag, in watchdogInputs) string {
	if !isPublished(in.releases, t.Name) {
		return "it has no published GitHub release"
	}
	zip := "permit-golang-" + t.Name + ".zip"
	check, ok := in.zips[t.Name]
	if !ok {
		return fmt.Sprintf("the module zip %s of its release was not checked", zip)
	}
	if check.Error != "" {
		return fmt.Sprintf("the module zip %s of its release did not verify: %s", zip,
			strings.Join(strings.Fields(check.Error), " "))
	}
	signer := "https://github.com/" + in.repo + "/.github/workflows/release.yml@" + releaseRef
	var problems []string
	for _, a := range check.Verified {
		problem := a.problem(t.Commit.SHA, signer, zip, check.SHA256)
		if problem == "" {
			return ""
		}
		problems = append(problems, problem)
	}
	return fmt.Sprintf("no attestation of the module zip %s of its release is release.yml's "+
		"from master for that commit: %s", zip, strings.Join(problems, "; "))
}

// problem returns why the attestation does not show that signer built the module zip named zip,
// whose SHA-256 is sum, from the commit on master, or "" when it does.
func (a attestation) problem(commit, signer, zip, sum string) string {
	cert := a.VerificationResult.Signature.Certificate
	statement := a.VerificationResult.Statement
	switch {
	case statement.PredicateType != provenanceType:
		return fmt.Sprintf("one has the predicate type %q, not %s", statement.PredicateType,
			provenanceType)
	case cert.BuildSignerURI != signer:
		return fmt.Sprintf("one is signed by %q, not %s", cert.BuildSignerURI, signer)
	case cert.SourceRepositoryRef != releaseRef:
		return fmt.Sprintf("one is of the ref %q, not %s", cert.SourceRepositoryRef, releaseRef)
	case cert.SourceRepositoryDigest != commit:
		return fmt.Sprintf("one is of the commit %q", cert.SourceRepositoryDigest)
	case !slices.ContainsFunc(statement.Subject, func(s subject) bool {
		return s.Name == zip && s.Digest["sha256"] == sum
	}):
		return fmt.Sprintf("one has no subject %s with the SHA-256 of the zip, %s", zip, sum)
	}
	return ""
}

// isPublished reports whether the releases include a published release of the tag, one that is
// not a draft.
func isPublished(releases []release, tagName string) bool {
	return slices.ContainsFunc(releases, func(rel release) bool {
		return rel.TagName == tagName && !rel.Draft
	})
}

// checkV1Tag checks that a v1 tag has a published GitHub release, and that CI succeeded for a push
// of its commit to its branch: master up to lastV1FromMaster, and release/v1 after it.
func checkV1Tag(r *report, t tag, releases []release, runs []workflowRun) {
	branch := "release/v1"
	if semver.Compare(t.Name, lastV1FromMaster) <= 0 {
		branch = "master"
	}
	published := isPublished(releases, t.Name)
	if !published {
		r.failf("%s at %s has no published GitHub release: a v1 version is published with gh "+
			"release create, once CI succeeded for a push of its commit to %s; %s", t.Name,
			t.Commit.SHA, branch, whatToDo)
	}
	var found []string
	for _, run := range runs {
		if run.HeadSHA != t.Commit.SHA || run.Event != "push" {
			continue
		}
		if run.HeadBranch == branch && run.Status == "completed" && run.Conclusion == "success" {
			if published {
				r.passf("%s at %s: its release is published, and CI succeeded for a push of it "+
					"to %s, in run %d", t.Name, t.Commit.SHA, branch, run.ID)
			}
			return
		}
		found = append(found, fmt.Sprintf("run %d on %s, %s", run.ID, run.HeadBranch,
			runState(run)))
	}
	runsFound := "it has no such run"
	if len(found) > 0 {
		runsFound = "its runs are " + strings.Join(found, "; ")
	}
	r.failf("%s at %s has no successful run of ci.yml for a push of that commit to %s, and %s: "+
		"a v1 version is published only once CI succeeded on its commit; %s", t.Name,
		t.Commit.SHA, branch, runsFound, whatToDo)
}

// runState returns the status of a run, and its conclusion once it has one.
func runState(run workflowRun) string {
	if run.Conclusion == "" {
		return run.Status
	}
	return run.Status + " " + run.Conclusion
}

// readTags reads the tags of the repository: one or more pages of the GitHub API's list.
func readTags(file string) ([]tag, error) {
	pages, err := readPages[[]tag](file, "a list of the repository's tags")
	if err != nil {
		return nil, err
	}
	tags := slices.Concat(pages...)
	for _, t := range tags {
		if t.Name == "" || !commitPattern.MatchString(t.Commit.SHA) {
			return nil, fmt.Errorf("%s lists a tag without a name or a full commit hash: it is "+
				"not a list of the repository's tags", file)
		}
	}
	return tags, nil
}

// readReleases reads the GitHub releases: one or more pages of the GitHub API's list.
func readReleases(file string) ([]release, error) {
	pages, err := readPages[[]release](file, "a list of releases")
	if err != nil {
		return nil, err
	}
	releases := slices.Concat(pages...)
	for _, rel := range releases {
		if rel.TagName == "" {
			return nil, fmt.Errorf("%s lists a release without a tag_name: it is not a list of "+
				"releases", file)
		}
	}
	return releases, nil
}

// readZipChecks reads what the first step of tag-watchdog.yml found for the module zip of each
// published v2 release: a JSON array, in which each check has a tag and either an error, or the
// zip's SHA-256 and at least one verified attestation. It returns the checks by tag.
func readZipChecks(file string) (map[string]zipCheck, error) {
	pages, err := readPages[[]zipCheck](file, "a list of the module zips of the v2 releases")
	if err != nil {
		return nil, err
	}
	checks := map[string]zipCheck{}
	for _, check := range slices.Concat(pages...) {
		verified := sha256Pattern.MatchString(check.SHA256) && len(check.Verified) > 0
		if check.Tag == "" || verified == (check.Error != "") {
			return nil, fmt.Errorf("%s lists a module zip without a tag, or without either an "+
				"error or its SHA-256 and verified attestations: it is not what tag-watchdog.yml "+
				"writes", file)
		}
		if _, ok := checks[check.Tag]; ok {
			return nil, fmt.Errorf("%s lists the module zip of %s twice", file, check.Tag)
		}
		checks[check.Tag] = check
	}
	return checks, nil
}

// readAcceptedTags reads acceptedTagsFile: <tag> <commit> <reason> per line. It returns the
// commit of each tag.
func readAcceptedTags(file string) (map[string]string, error) {
	lines, err := readLines(file, true)
	if err != nil {
		return nil, err
	}
	accepted := map[string]string{}
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 3 || !commitPattern.MatchString(fields[1]) {
			return nil, fmt.Errorf("%s: %q is not <tag> <commit> <reason>, with a full commit "+
				"hash", file, line)
		}
		if _, ok := accepted[fields[0]]; ok {
			return nil, fmt.Errorf("%s accepts %s twice", file, fields[0])
		}
		accepted[fields[0]] = fields[1]
	}
	return accepted, nil
}
