package main

import (
	"fmt"
	"maps"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/mod/semver"
)

// The commits that the tags of the fixtures name.
var (
	v2Commit     = strings.Repeat("4", 40)
	v1Commit     = strings.Repeat("5", 40)
	masterCommit = strings.Repeat("6", 40)
	legacyCommit = strings.Repeat("7", 40)
	movedCommit  = strings.Repeat("8", 40)
)

// The repository of the fixtures, the workflow that signs the attestations of its v2 releases,
// and the SHA-256 of the module zips that tag-watchdog.yml downloaded.
const (
	testRepo      = "octo/repo"
	releaseSigner = "https://github.com/octo/repo/.github/workflows/release.yml@refs/heads/master"
	zipSum        = "9999999999999999999999999999999999999999999999999999999999999999"
	sbomSum       = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

// tagOf is a tag at the commit, as the GitHub API lists it.
func tagOf(name, commit string) string {
	return fmt.Sprintf(`{"name": %q, "commit": {"sha": %q, "url": "https://example.com"}}`, name,
		commit)
}

// pageOf is a page of a list that the GitHub API returns as an array.
func pageOf(items ...string) string {
	return "[" + strings.Join(items, ",\n") + "]\n"
}

// fixtureRun is a run of a workflow in its latest attempt, as the GitHub API lists it.
type fixtureRun struct {
	id                                               int
	title, commit, branch, event, status, conclusion string
}

func (r fixtureRun) json() string {
	return fmt.Sprintf(`{"id": %d, "run_attempt": 1, "display_title": %q, "head_sha": %q, `+
		`"head_branch": %q, "event": %q, "status": %q, "conclusion": %q}`, r.id, r.title,
		r.commit, r.branch, r.event, r.status, r.conclusion)
}

// releaseRun is a run of release.yml from master, whose run-name is release <tag>.
func releaseRun(id int, title, commit, status, conclusion string) string {
	return fixtureRun{id, title, commit, "master", "workflow_dispatch", status, conclusion}.json()
}

// ciRun is a run of ci.yml for a push of the commit to the branch.
func ciRun(id int, commit, branch, status, conclusion string) string {
	return fixtureRun{id, "a change", commit, branch, "push", status, conclusion}.json()
}

// releaseOf is a GitHub release of the tag, as the API lists it.
func releaseOf(tagName string, draft bool) string {
	return fmt.Sprintf(`{"tag_name": %q, "draft": %t, "prerelease": false}`, tagName, draft)
}

// provenance is an attestation of a module zip: its predicate type; the workflow that signed
// it, by its URI; the ref and the commit it was built from; and the name and SHA-256 of the
// zip among its subjects, with the SBOM.
type provenance struct {
	predicateType, signer, ref, commit, zip, sum string
}

// releaseProvenance is the attestation that release.yml, run from master, makes of the module
// zip of the tag at the commit.
func releaseProvenance(tagName, commit string) provenance {
	return provenance{
		provenanceType, releaseSigner, releaseRef, commit,
		"permit-golang-" + tagName + ".zip", zipSum,
	}
}

// json is the attestation as gh attestation verify --format json prints it.
func (p provenance) json() string {
	return fmt.Sprintf(`{"attestation": {"bundle": {"mediaType": `+
		`"application/vnd.dev.sigstore.bundle.v0.3+json"}}, "verificationResult": {"mediaType": `+
		`"application/vnd.dev.sigstore.verificationresult+json;version=0.1", "signature": `+
		`{"certificate": {"certificateIssuer": "CN=sigstore-intermediate,O=sigstore.dev", `+
		`"subjectAlternativeName": %q, "issuer": "https://token.actions.githubusercontent.com", `+
		`"buildSignerURI": %q, "sourceRepositoryRef": %q, "sourceRepositoryDigest": %q, `+
		`"buildTrigger": "workflow_dispatch"}}, "statement": {"_type": `+
		`"https://in-toto.io/Statement/v1", "predicateType": %q, "subject": [{"name": %q, `+
		`"digest": {"sha256": %q}}, {"name": "permit-golang.sbom.json", "digest": {"sha256": `+
		`%q}}], "predicate": {}}}}`, p.signer, p.signer, p.ref, p.commit, p.predicateType, p.zip,
		p.sum, sbomSum)
}

// zipOf is what tag-watchdog.yml found for the module zip of the tag's release: its SHA-256,
// zipSum, and the attestations that gh attestation verify verified.
func zipOf(tagName string, attestations ...provenance) string {
	var verified []string
	for _, a := range attestations {
		verified = append(verified, a.json())
	}
	return fmt.Sprintf(`{"tag": %q, "sha256": %q, "error": "", "verified": [%s]}`, tagName, zipSum,
		strings.Join(verified, ",\n"))
}

// v2ZipError is the module zip of the release of v2.0.0, which tag-watchdog.yml could not
// download or verify, with why.
func v2ZipError(err string) string {
	return fmt.Sprintf(`{"tag": "v2.0.0", "sha256": %q, "error": %q, "verified": null}`, zipSum,
		err)
}

// notAttested is the record of a module zip of v2.0.0 that has no attestation yet, and what
// watchdog says of it.
var (
	notAttested = map[string]string{
		"attestations.json": pageOf(v2ZipError("Error: no attestations found\n")),
	}
	notAttestedSays = "the module zip permit-golang-v2.0.0.zip of its release did not verify: " +
		"Error: no attestations found"
)

// watchdogFiles are the inputs of a repository whose every tag was published by the rules:
// v2.0.0 by release.yml, which attested its module zip; v1.3.0 with a release and CI of a push to
// master; v1.4.0 with a release and CI of a push to release/v1; v1.2.8 and v0.0.1 that
// accepted-tags.txt accepts; and 1.2.1, which is no version. The tags and the release runs come
// in two pages each.
func watchdogFiles() map[string]string {
	return map[string]string{
		"tags.json": pageOf(tagOf("v2.0.0", v2Commit), tagOf("v1.4.0", v1Commit),
			tagOf("1.2.1", legacyCommit)) +
			pageOf(tagOf("v1.3.0", masterCommit), tagOf("v1.2.8", legacyCommit),
				tagOf("v0.0.1", legacyCommit)),
		"release-runs.json": runsOf(releaseRun(12, "release v2.0.0", v2Commit, "completed",
			"success")) + "\n" + runsOf(releaseRun(11, "release v2.0.0-rc.1", otherSHA,
			"completed", "failure")),
		"releases.json": pageOf(releaseOf("v2.0.0", false), releaseOf("v1.4.0", false),
			releaseOf("v1.3.0", false), releaseOf("v1.2.8", false)),
		"ci-runs.json": runsOf() + "\n" + runsOf(ciRun(31, masterCommit, "master", "completed",
			"success")) + "\n" + runsOf(ciRun(42, v1Commit, "release/v1", "completed", "success"),
			ciRun(41, v1Commit, "release/v1", "completed", "failure")),
		"attestations.json": pageOf(zipOf("v2.0.0", releaseProvenance("v2.0.0", v2Commit))),
		acceptedTagsFile: "# Accepted tags.\n\nv1.2.8 " + legacyCommit + " before the release " +
			"rules\nv0.0.1 " + legacyCommit + " before the release rules\n",
	}
}

// withChanges returns the changes with more changes, for a test case.
func withChanges(changes ...map[string]string) map[string]string {
	all := map[string]string{}
	for _, c := range changes {
		maps.Copy(all, c)
	}
	return all
}

// attestedAs returns the inputs in which the module zip of v2.0.0 has the attestation p, and
// release.yml has no run, as when its runs were deleted.
func attestedAs(p provenance) map[string]string {
	return map[string]string{
		"attestations.json": pageOf(zipOf("v2.0.0", p)),
		"release-runs.json": runsOf(),
	}
}

// runWatchdogCase writes the inputs with changes, a map from a file to its content, where an
// empty content removes the file, and runs watchdog.
func runWatchdogCase(t *testing.T, changes map[string]string) (int, string) {
	t.Helper()
	dir := t.TempDir()
	files := watchdogFiles()
	maps.Copy(files, changes)
	for name, content := range files {
		if content == "" {
			delete(files, name)
		}
	}
	writeTree(t, dir, files)
	return runCheck("watchdog", "-root", dir, "-repo", testRepo,
		"-tags", filepath.Join(dir, "tags.json"),
		"-release-runs", filepath.Join(dir, "release-runs.json"),
		"-releases", filepath.Join(dir, "releases.json"),
		"-ci-runs", filepath.Join(dir, "ci-runs.json"),
		"-attestations", filepath.Join(dir, "attestations.json"))
}

// createdOutside ends the failure of a v2 tag that release.yml did not create.
const createdOutside = "release.yml is the one way to publish a v2 version, so the tag was " +
	"created outside it; see Releases in CONTRIBUTING.md for what to do\n"

func TestWatchdog(t *testing.T) {
	// withTag returns the tags of the fixture with one more page that holds the tag.
	withTag := func(name, commit string) map[string]string {
		return map[string]string{"tags.json": watchdogFiles()["tags.json"] +
			pageOf(tagOf(name, commit))}
	}
	// withRuns returns runs of release.yml, and no attestation of the module zip of v2.0.0.
	withRuns := func(runs ...string) map[string]string {
		return withChanges(notAttested, map[string]string{"release-runs.json": runsOf(runs...)})
	}
	// noRunFor is the failure of v2.0.0, whose zip is not attested, with no run of release.yml
	// for its commit.
	noRunFor := "releasecheck: FAIL: v2.0.0 at " + v2Commit + ": " + notAttestedSays +
		", and release.yml has no run named \"release v2.0.0\" from master for that commit"
	// notOurs is the failure of v2.0.0, whose zip has an attestation that is not release.yml's
	// for its commit, for the reason says, and no run of release.yml.
	notOurs := func(says string) string {
		return "releasecheck: FAIL: v2.0.0 at " + v2Commit + ": no attestation of the module " +
			"zip permit-golang-v2.0.0.zip of its release is release.yml's from master for that " +
			"commit: " + says + ", and release.yml has no run named \"release v2.0.0\" from " +
			"master for that commit: " + createdOutside
	}
	tests := []struct {
		name string
		// files replace the inputs of watchdogFiles; an empty content removes the file.
		files    map[string]string
		wantExit int
		want     []string
		// notWant are parts that the output does not hold.
		notWant []string
	}{
		{
			name: "every tag published by the rules", wantExit: exitPassed,
			want: []string{
				"releasecheck: not checked: 1.2.1, which is not a semantic version such as " +
					"v2.1.0, so no version of the module\n",
				"releasecheck: ok: .github/accepted-tags.txt accepts 2 of the tags, each at " +
					"the commit it names\n",
				"releasecheck: ok: v1.3.0 at " + masterCommit + ": its release is published, " +
					"and CI succeeded for a push of it to master, in run 31\n",
				"releasecheck: ok: v1.4.0 at " + v1Commit + ": its release is published, and " +
					"CI succeeded for a push of it to release/v1, in run 42\n",
				"releasecheck: ok: v2.0.0 at " + v2Commit + ": the module zip of its " +
					"published release is attested by release.yml from master for that commit\n",
			},
		},
		{
			name: "a v2 tag older than every run of release.yml", wantExit: exitPassed,
			files: map[string]string{"release-runs.json": runsOf()},
			want: []string{"releasecheck: ok: v2.0.0 at " + v2Commit + ": the module zip of " +
				"its published release is attested by release.yml from master for that commit\n"},
		},
		{
			name: "an attestation of release.yml after another", wantExit: exitPassed,
			files: map[string]string{"attestations.json": pageOf(zipOf("v2.0.0", provenance{
				provenanceType, releaseSigner, releaseRef, otherSHA, "permit-golang-v2.0.0.zip",
				zipSum,
			}, releaseProvenance("v2.0.0", v2Commit)))},
			want: []string{"releasecheck: ok: v2.0.0 at " + v2Commit + ": the module zip"},
		},
		{
			name: "a release run that has not attested the zip yet", wantExit: exitPassed,
			files: withRuns(releaseRun(12, "release v2.0.0", v2Commit, "in_progress", ""),
				releaseRun(11, "release v2.0.0", v2Commit, "completed", "failure")),
			want: []string{
				"releasecheck: pending: v2.0.0 at " + v2Commit + ": " + notAttestedSays +
					", and its release run 12 of release.yml has not finished; the next run of " +
					"the watchdog checks it\n",
			},
		},
		{
			name: "a release run for another commit that has not finished", wantExit: exitFailed,
			files: withRuns(releaseRun(12, "release v2.0.0", otherSHA, "in_progress", "")),
			want: []string{noRunFor + ", only run 12 for " + otherSHA + " on master by " +
				"workflow_dispatch, in_progress: " + createdOutside},
		},
		{
			name: "a release run from another branch", wantExit: exitFailed,
			files: withRuns(fixtureRun{
				12, "release v2.0.0", v2Commit, "fix", "workflow_dispatch",
				"in_progress", "",
			}.json(), fixtureRun{
				13, "release v2.0.0", v2Commit, "fix",
				"workflow_dispatch", "completed", "success",
			}.json()),
			want: []string{noRunFor + ", only run 12 for " + v2Commit + " on fix by " +
				"workflow_dispatch, in_progress; run 13 for " + v2Commit + " on fix by " +
				"workflow_dispatch, completed success: " + createdOutside},
		},
		{
			name: "a release run of another event", wantExit: exitFailed,
			files: withRuns(fixtureRun{
				12, "release v2.0.0", v2Commit, "master", "push",
				"in_progress", "",
			}.json()),
			want: []string{noRunFor + ", only run 12 for " + v2Commit + " on master by push, " +
				"in_progress: "},
		},
		{
			name: "a release run that waits for an approval", wantExit: exitFailed,
			files: withRuns(releaseRun(12, "release v2.0.0", v2Commit, "waiting", "")),
			want: []string{noRunFor + ", only run 12 for " + v2Commit + " on master by " +
				"workflow_dispatch, waiting: " + createdOutside},
		},
		{
			name: "a queued release run", wantExit: exitFailed,
			files: withRuns(releaseRun(12, "release v2.0.0", v2Commit, "queued", "")),
			want: []string{noRunFor + ", only run 12 for " + v2Commit + " on master by " +
				"workflow_dispatch, queued: "},
		},
		{
			name: "a release run named otherwise", wantExit: exitFailed,
			files: withRuns(releaseRun(12, "dry release v2.0.0", v2Commit, "in_progress", "")),
			want:  []string{noRunFor + ": " + createdOutside},
		},
		{
			name: "a release run that failed", wantExit: exitFailed,
			files: withRuns(releaseRun(12, "release v2.0.0", v2Commit, "completed", "failure")),
			want: []string{
				"releasecheck: FAIL: v2.0.0 at " + v2Commit + ": " + notAttestedSays + "; the " +
					"release run 12 for that commit concluded failure: re-run its failed jobs, " +
					"or see Releases in CONTRIBUTING.md\n",
			},
		},
		{
			name: "release runs that failed and were cancelled", wantExit: exitFailed,
			files: withRuns(releaseRun(14, "release v2.0.0", v2Commit, "completed", "cancelled"),
				releaseRun(12, "release v2.0.0", v2Commit, "completed", "failure")),
			want: []string{
				"releasecheck: FAIL: v2.0.0 at " + v2Commit + ": " + notAttestedSays + "; the " +
					"release run 14 for that commit concluded cancelled; run 12 for that commit " +
					"concluded failure: re-run the failed jobs of the one that published the " +
					"tag, or see Releases in CONTRIBUTING.md\n",
			},
		},
		{
			name: "a release run that succeeded without an attestation", wantExit: exitFailed,
			files: withRuns(releaseRun(13, "release v2.0.0", v2Commit, "completed", "failure"),
				releaseRun(12, "release v2.0.0", v2Commit, "completed", "success")),
			want: []string{
				"releasecheck: FAIL: v2.0.0 at " + v2Commit + ": " + notAttestedSays + ", " +
					"although its release run 12 of release.yml succeeded: the release or its " +
					"attestation changed after the run; see Releases in CONTRIBUTING.md for what " +
					"to do\n",
			},
		},
		{
			name: "a v2 tag without a release run", files: withTag("v2.1.0", otherSHA),
			wantExit: exitFailed,
			want: []string{
				"releasecheck: FAIL: v2.1.0 at " + otherSHA + ": it has no published GitHub " +
					"release, and release.yml has no run named \"release v2.1.0\" from master " +
					"for that commit: " + createdOutside,
			},
		},
		{
			name: "a v2 tag whose release run is of another commit", wantExit: exitFailed,
			files: withRuns(releaseRun(12, "release v2.0.0", otherSHA, "completed", "success")),
			want: []string{noRunFor + ", only run 12 for " + otherSHA + " on master by " +
				"workflow_dispatch, completed success: " + createdOutside},
		},
		{
			name: "a v2 release whose attestation is of another commit", wantExit: exitFailed,
			files: attestedAs(releaseProvenance("v2.0.0", otherSHA)),
			want:  []string{notOurs("one is of the commit \"" + otherSHA + "\"")},
		},
		{
			name: "a v2 release whose attestation another workflow signed", wantExit: exitFailed,
			files: attestedAs(provenance{
				provenanceType, "https://github.com/octo/repo/.github/workflows/ci.yml@" +
					"refs/heads/master", releaseRef, v2Commit, "permit-golang-v2.0.0.zip", zipSum,
			}),
			want: []string{notOurs("one is signed by \"https://github.com/octo/repo/.github/" +
				"workflows/ci.yml@refs/heads/master\", not " + releaseSigner)},
		},
		{
			name: "a v2 release whose attestation release.yml of a branch signed",
			files: attestedAs(provenance{
				provenanceType, "https://github.com/octo/repo/.github/workflows/release.yml@" +
					"refs/heads/fix", "refs/heads/fix", v2Commit, "permit-golang-v2.0.0.zip",
				zipSum,
			}),
			wantExit: exitFailed,
			want: []string{notOurs("one is signed by \"https://github.com/octo/repo/.github/" +
				"workflows/release.yml@refs/heads/fix\", not " + releaseSigner)},
		},
		{
			name: "a v2 release whose attestation another repository signed",
			files: attestedAs(provenance{
				provenanceType, "https://github.com/fork/repo/.github/workflows/release.yml@" +
					"refs/heads/master", releaseRef, v2Commit, "permit-golang-v2.0.0.zip", zipSum,
			}),
			wantExit: exitFailed,
			want: []string{notOurs("one is signed by \"https://github.com/fork/repo/.github/" +
				"workflows/release.yml@refs/heads/master\", not " + releaseSigner)},
		},
		{
			name: "a v2 release whose attestation is of another ref", wantExit: exitFailed,
			files: attestedAs(provenance{
				provenanceType, releaseSigner, "refs/heads/fix", v2Commit,
				"permit-golang-v2.0.0.zip", zipSum,
			}),
			want: []string{notOurs("one is of the ref \"refs/heads/fix\", not refs/heads/master")},
		},
		{
			name: "a v2 release whose attestation is no build provenance", wantExit: exitFailed,
			files: attestedAs(provenance{
				"https://spdx.dev/Document/v2.3", releaseSigner, releaseRef, v2Commit,
				"permit-golang-v2.0.0.zip", zipSum,
			}),
			want: []string{notOurs("one has the predicate type \"https://spdx.dev/Document/" +
				"v2.3\", not https://slsa.dev/provenance/v1")},
		},
		{
			name: "a v2 release whose attestation is of another zip", wantExit: exitFailed,
			files: attestedAs(releaseProvenance("v2.0.0-rc.1", v2Commit)),
			want: []string{notOurs("one has no subject permit-golang-v2.0.0.zip with the " +
				"SHA-256 of the zip, " + zipSum)},
		},
		{
			name: "a v2 release whose attestation has another SHA-256 of the zip",
			files: attestedAs(provenance{
				provenanceType, releaseSigner, releaseRef, v2Commit, "permit-golang-v2.0.0.zip",
				sbomSum,
			}),
			wantExit: exitFailed,
			want: []string{notOurs("one has no subject permit-golang-v2.0.0.zip with the " +
				"SHA-256 of the zip, " + zipSum)},
		},
		{
			name: "attestations of two problems", wantExit: exitFailed,
			files: withChanges(attestedAs(releaseProvenance("v2.0.0", otherSHA)),
				map[string]string{"attestations.json": pageOf(zipOf("v2.0.0",
					releaseProvenance("v2.0.0", otherSHA), releaseProvenance("v2.0.0",
						masterCommit)))}),
			want: []string{notOurs("one is of the commit \"" + otherSHA + "\"; one is of the " +
				"commit \"" + masterCommit + "\"")},
		},
		{
			name: "a draft release with an attested zip", wantExit: exitFailed,
			files: map[string]string{
				"releases.json": pageOf(releaseOf("v2.0.0", true), releaseOf("v1.4.0", false),
					releaseOf("v1.3.0", false)),
				"release-runs.json": runsOf(),
			},
			want: []string{"releasecheck: FAIL: v2.0.0 at " + v2Commit + ": it has no " +
				"published GitHub release, and release.yml has no run named \"release v2.0.0\" " +
				"from master for that commit: " + createdOutside},
		},
		{
			name: "a v2 release whose zip was not checked", wantExit: exitFailed,
			files: map[string]string{"attestations.json": pageOf(), "release-runs.json": runsOf()},
			want: []string{"releasecheck: FAIL: v2.0.0 at " + v2Commit + ": the module zip " +
				"permit-golang-v2.0.0.zip of its release was not checked, and release.yml has " +
				"no run named \"release v2.0.0\" from master for that commit: " + createdOutside},
		},
		{
			name: "a v2 release whose zip did not verify", wantExit: exitFailed,
			files: map[string]string{
				"attestations.json": pageOf(v2ZipError("Error: verifying with issuer " +
					"\"sigstore.dev\"\n\n  no matching attestations\n")),
				"release-runs.json": runsOf(),
			},
			want: []string{"releasecheck: FAIL: v2.0.0 at " + v2Commit + ": the module zip " +
				"permit-golang-v2.0.0.zip of its release did not verify: Error: verifying with " +
				"issuer \"sigstore.dev\" no matching attestations, and release.yml has no run "},
		},
		{
			name: "a v2 tag that is no release tag", files: withTag("v2.0", v2Commit),
			wantExit: exitFailed,
			want: []string{"releasecheck: FAIL: v2.0 at " + v2Commit + ": it has no published " +
				"GitHub release, and release.yml has no run named \"release v2.0\""},
		},
		{
			name: "a v1 tag whose CI failed", wantExit: exitFailed,
			files: map[string]string{
				"ci-runs.json": runsOf(ciRun(31, masterCommit, "master", "completed",
					"success"), ciRun(41, v1Commit, "release/v1", "completed", "failure")),
			},
			want: []string{
				"releasecheck: FAIL: v1.4.0 at " + v1Commit + " has no successful run of " +
					"ci.yml for a push of that commit to release/v1, and its runs are run 41 on " +
					"release/v1, completed failure: a v1 version is published only once CI " +
					"succeeded on its commit; see Releases in CONTRIBUTING.md for what to do\n",
			},
		},
		{
			name: "a v1 tag whose CI has not finished", wantExit: exitFailed,
			files: map[string]string{
				"ci-runs.json": runsOf(ciRun(31, masterCommit, "master", "completed",
					"success"), ciRun(43, v1Commit, "release/v1", "in_progress", "")),
			},
			want: []string{"v1.4.0 at " + v1Commit + " has no successful run of ci.yml for a " +
				"push of that commit to release/v1, and its runs are run 43 on release/v1, " +
				"in_progress: "},
		},
		{
			name: "a v1 tag whose CI run reports success before it completed",
			files: map[string]string{
				"ci-runs.json": runsOf(ciRun(31, masterCommit, "master", "completed",
					"success"), ciRun(43, v1Commit, "release/v1", "in_progress", "success")),
			},
			wantExit: exitFailed,
			want: []string{"v1.4.0 at " + v1Commit + " has no successful run of ci.yml for a " +
				"push of that commit to release/v1, and its runs are run 43 on release/v1, " +
				"in_progress success: "},
		},
		{
			name: "a v1 tag whose CI ran for another event", wantExit: exitFailed,
			files: map[string]string{
				"ci-runs.json": runsOf(ciRun(31, masterCommit, "master", "completed",
					"success"), fixtureRun{
					43, "a change", v1Commit, "release/v1",
					"pull_request", "completed", "success",
				}.json()),
			},
			want: []string{"v1.4.0 at " + v1Commit + " has no successful run of ci.yml for a " +
				"push of that commit to release/v1, and it has no such run: "},
		},
		{
			name: "a later v1 tag whose CI ran on master", wantExit: exitFailed,
			files: map[string]string{
				"ci-runs.json": runsOf(ciRun(31, masterCommit, "master", "completed",
					"success"), ciRun(42, v1Commit, "master", "completed", "success")),
			},
			want: []string{"v1.4.0 at " + v1Commit + " has no successful run of ci.yml for a " +
				"push of that commit to release/v1, and its runs are run 42 on master, " +
				"completed success: "},
		},
		{
			name: "v1.3.0, whose CI ran on release/v1 only", wantExit: exitFailed,
			files: map[string]string{
				"ci-runs.json": runsOf(ciRun(31, masterCommit, "release/v1", "completed",
					"success"), ciRun(42, v1Commit, "release/v1", "completed", "success")),
			},
			want: []string{"v1.3.0 at " + masterCommit + " has no successful run of ci.yml " +
				"for a push of that commit to master, and its runs are run 31 on release/v1, " +
				"completed success: "},
		},
		{
			name: "a v1 tag without CI", wantExit: exitFailed,
			files: map[string]string{
				"ci-runs.json": runsOf(ciRun(31, masterCommit, "master", "completed",
					"success"), ciRun(43, v1Commit, "release/v1", "completed", "success")) +
					"\n" + runsOf(ciRun(44, otherSHA, "release/v1", "completed", "success")),
				"tags.json": watchdogFiles()["tags.json"] + pageOf(tagOf("v1.4.1", otherSHA),
					tagOf("v1.4.2", releaseSHA)),
				"releases.json": watchdogFiles()["releases.json"] +
					pageOf(releaseOf("v1.4.1", false), releaseOf("v1.4.2", false)),
			},
			want: []string{"releasecheck: FAIL: v1.4.2 at " + releaseSHA + " has no " +
				"successful run of ci.yml for a push of that commit to release/v1, and it has " +
				"no such run: "},
		},
		{
			name: "a v1 tag without a release", wantExit: exitFailed,
			files: map[string]string{
				"releases.json": pageOf(releaseOf("v2.0.0", false), releaseOf("v1.3.0", false)),
			},
			want: []string{
				"releasecheck: FAIL: v1.4.0 at " + v1Commit + " has no published GitHub " +
					"release: a v1 version is published with gh release create, once CI " +
					"succeeded for a push of its commit to release/v1; see Releases in " +
					"CONTRIBUTING.md for what to do\n",
			},
			notWant: []string{"ok: v1.4.0 at "},
		},
		{
			name: "a v1 tag whose release is a draft", wantExit: exitFailed,
			files: map[string]string{
				"releases.json": pageOf(releaseOf("v2.0.0", false), releaseOf("v1.3.0", false),
					releaseOf("v1.4.0", true)),
			},
			want: []string{"releasecheck: FAIL: v1.4.0 at " + v1Commit + " has no published " +
				"GitHub release:"},
			notWant: []string{"ok: v1.4.0 at "},
		},
		{
			name: "an accepted tag that moved", wantExit: exitFailed,
			files: map[string]string{
				"tags.json": pageOf(tagOf("v1.2.8", movedCommit), tagOf("v0.0.1", legacyCommit)),
			},
			want: []string{
				"releasecheck: FAIL: v1.2.8 names " + movedCommit + ", but " +
					".github/accepted-tags.txt accepts it at " + legacyCommit + ": a published " +
					"version never moves; see Releases in CONTRIBUTING.md for what to do\n",
				"releasecheck: ok: .github/accepted-tags.txt accepts 1 of the tags, each at " +
					"the commit it names\n",
			},
		},
		{
			name: "a tag of another major version", files: withTag("v3.0.0", v2Commit),
			wantExit: exitFailed,
			want: []string{
				"releasecheck: FAIL: v3.0.0 at " + v2Commit + " is neither a v1 nor a v2 " +
					"version, and .github/accepted-tags.txt does not accept it: no workflow of " +
					"this repository publishes it; see Releases in CONTRIBUTING.md for what " +
					"to do\n",
			},
		},
		{
			name: "a v0 tag that is not accepted", files: withTag("v0.0.20", v2Commit),
			wantExit: exitFailed,
			want:     []string{"releasecheck: FAIL: v0.0.20 at " + v2Commit + " is neither"},
		},
		{
			name: "no tag", wantExit: exitPassed,
			files: map[string]string{"tags.json": pageOf()},
			want: []string{"releasecheck: ok: .github/accepted-tags.txt accepts 0 of the " +
				"tags, each at the commit it names\n"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exit, stdout := runWatchdogCase(t, tt.files)

			assertResult(t, exit, tt.wantExit, stdout, tt.want...)
			if fails := strings.Count(stdout, "releasecheck: FAIL: "); tt.wantExit ==
				exitFailed && fails != 1 {
				t.Errorf("output has %d failures, want 1:\n%s", fails, stdout)
			}
			for _, part := range tt.notWant {
				if strings.Contains(stdout, part) {
					t.Errorf("output holds %q:\n%s", part, stdout)
				}
			}
		})
	}
}

func TestWatchdogDidNotRun(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{
			name: "tags that are not JSON", files: map[string]string{"tags.json": "<html>"},
			want: "tags.json: invalid character '<' looking for beginning of value\n",
		},
		{
			name:  "a page of tags that is not a list",
			files: map[string]string{"tags.json": `{"message": "Not Found"}`},
			want:  "tags.json: json: cannot unmarshal object into Go value of type []main.tag\n",
		},
		{
			name:  "no page of tags",
			files: map[string]string{"tags.json": "\n"},
			want:  "tags.json is empty: it is not a list of the repository's tags\n",
		},
		{
			name: "a tag without a commit",
			files: map[string]string{
				"tags.json": pageOf(`{"name": "v2.0.0", "commit": {"sha": "4444"}}`),
			},
			want: "tags.json lists a tag without a name or a full commit hash: it is not a " +
				"list of the repository's tags\n",
		},
		{
			name:  "a tag without a name",
			files: map[string]string{"tags.json": pageOf(tagOf("", v2Commit))},
			want: "tags.json lists a tag without a name or a full commit hash: it is not a " +
				"list of the repository's tags\n",
		},
		{
			name:  "no tags file",
			files: map[string]string{"tags.json": ""},
			want:  "tags.json: no such file or directory\n",
		},
		{
			name: "release runs that are not a list",
			files: map[string]string{
				"release-runs.json": runsOf() + `{"message": "Not Found"}`,
			},
			want: "release-runs.json has no workflow_runs: it is not a list of workflow runs\n",
		},
		{
			name:  "no page of release runs",
			files: map[string]string{"release-runs.json": " "},
			want:  "release-runs.json is empty: it is not a list of workflow runs\n",
		},
		{
			name: "a release run without an id",
			files: map[string]string{
				"release-runs.json": runsOf(releaseRun(0, "release v2.0.0", v2Commit,
					"completed", "success")),
			},
			want: "release-runs.json lists a run without an id or a run_attempt: it is not a " +
				"list of workflow runs\n",
		},
		{
			name:  "releases that are not JSON",
			files: map[string]string{"releases.json": "Not Found"},
			want:  "releases.json: invalid character 'N' looking for beginning of value\n",
		},
		{
			name:  "a release without a tag",
			files: map[string]string{"releases.json": pageOf(`{"tag_name": "", "draft": false}`)},
			want: "releases.json lists a release without a tag_name: it is not a list of " +
				"releases\n",
		},
		{
			name:  "no CI runs",
			files: map[string]string{"ci-runs.json": ""},
			want:  "ci-runs.json: no such file or directory\n",
		},
		{
			name:  "no attestations file",
			files: map[string]string{"attestations.json": ""},
			want:  "attestations.json: no such file or directory\n",
		},
		{
			name:  "no list of attestations",
			files: map[string]string{"attestations.json": "\n"},
			want: "attestations.json is empty: it is not a list of the module zips of the v2 " +
				"releases\n",
		},
		{
			name:  "attestations that are not a list",
			files: map[string]string{"attestations.json": zipOf("v2.0.0")},
			want: "attestations.json: json: cannot unmarshal object into Go value of type " +
				"[]main.zipCheck\n",
		},
		{
			name: "a zip without a tag",
			files: map[string]string{
				"attestations.json": pageOf(zipOf("", releaseProvenance("v2.0.0", v2Commit))),
			},
			want: "attestations.json lists a module zip without a tag, or without either an " +
				"error or its SHA-256 and verified attestations: it is not what tag-watchdog.yml " +
				"writes\n",
		},
		{
			name:  "a zip without an error or an attestation",
			files: map[string]string{"attestations.json": pageOf(zipOf("v2.0.0"))},
			want:  "attestations.json lists a module zip without a tag, or without either",
		},
		{
			name: "a zip with attestations but without its SHA-256",
			files: map[string]string{"attestations.json": pageOf(strings.Replace(zipOf("v2.0.0",
				releaseProvenance("v2.0.0", v2Commit)), zipSum, "", 1))},
			want: "attestations.json lists a module zip without a tag, or without either",
		},
		{
			name: "a zip with an error and attestations",
			files: map[string]string{"attestations.json": pageOf(strings.Replace(zipOf("v2.0.0",
				releaseProvenance("v2.0.0", v2Commit)), `"error": ""`, `"error": "exit 1"`, 1))},
			want: "attestations.json lists a module zip without a tag, or without either",
		},
		{
			name: "a zip listed twice",
			files: map[string]string{"attestations.json": pageOf(v2ZipError("exit 1"),
				v2ZipError("exit 1"))},
			want: "attestations.json lists the module zip of v2.0.0 twice\n",
		},
		{
			name:  "no accepted tags",
			files: map[string]string{acceptedTagsFile: ""},
			want:  "accepted-tags.txt: no such file or directory\n",
		},
		{
			name:  "an accepted tag without a reason",
			files: map[string]string{acceptedTagsFile: "v1.2.8 " + legacyCommit + "\n"},
			want: "accepted-tags.txt: \"v1.2.8 " + legacyCommit + "\" is not <tag> <commit> " +
				"<reason>, with a full commit hash\n",
		},
		{
			name:  "an accepted tag with a short commit",
			files: map[string]string{acceptedTagsFile: "v1.2.8 7777777 before the rules\n"},
			want:  "\"v1.2.8 7777777 before the rules\" is not <tag> <commit> <reason>",
		},
		{
			name: "a tag accepted twice",
			files: map[string]string{acceptedTagsFile: "v1.2.8 " + legacyCommit + " a\n" +
				"v1.2.8 " + movedCommit + " b\n"},
			want: "accepted-tags.txt accepts v1.2.8 twice\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exit, stdout := runWatchdogCase(t, tt.files)

			assertResult(t, exit, exitDidNotRun, stdout, tt.want)
			if strings.Contains(stdout, "ok: ") || strings.Contains(stdout, "FAIL: ") {
				t.Errorf("a run that did not run reports checks:\n%s", stdout)
			}
		})
	}
}

// watchdog prints the tags in version order, v2.9.0 before v2.10.0 whatever their order in the
// list, and no line of tags it does not check when every tag is a semantic version.
func TestWatchdogPrintsTagsInVersionOrder(t *testing.T) {
	exit, stdout := runWatchdogCase(t, map[string]string{
		"tags.json":     pageOf(tagOf("v2.10.0", v2Commit), tagOf("v2.9.0", otherSHA)),
		"releases.json": pageOf(releaseOf("v2.10.0", false), releaseOf("v2.9.0", false)),
		"attestations.json": pageOf(zipOf("v2.10.0", releaseProvenance("v2.10.0", v2Commit)),
			zipOf("v2.9.0", releaseProvenance("v2.9.0", otherSHA))),
	})

	assertResult(t, exit, exitPassed, stdout)
	older := strings.Index(stdout, "ok: v2.9.0 at ")
	newer := strings.Index(stdout, "ok: v2.10.0 at ")
	if older < 0 || newer < 0 || older > newer {
		t.Errorf("watchdog did not print v2.9.0, then v2.10.0:\n%s", stdout)
	}
	if strings.Contains(stdout, "not checked") {
		t.Errorf("watchdog printed tags it does not check, of which there are none:\n%s", stdout)
	}
}

// TestRepositoryAcceptedTags checks that this repository's list of accepted tags is one that
// watchdog reads, and that each tag on it is a semantic version, which watchdog checks.
func TestRepositoryAcceptedTags(t *testing.T) {
	accepted, err := readAcceptedTags(filepath.Join("..", "..", "..",
		filepath.FromSlash(acceptedTagsFile)))
	if err != nil {
		t.Fatal(err)
	}
	if len(accepted) == 0 {
		t.Errorf("%s accepts no tag", acceptedTagsFile)
	}
	for name := range accepted {
		if !semver.IsValid(name) {
			t.Errorf("%s accepts %s, which is not a semantic version, so watchdog never checks "+
				"it", acceptedTagsFile, name)
		}
	}
}

func TestWatchdogFlags(t *testing.T) {
	exit, stdout := runCheck("watchdog", "-tags", "t.json")

	assertResult(t, exit, exitDidNotRun, stdout, "releasecheck: DID NOT RUN: releasecheck "+
		"watchdog needs -attestations, -ci-runs, -release-runs, -releases, -repo\n")
}
