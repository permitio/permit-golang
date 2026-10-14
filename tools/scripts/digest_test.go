package scripts

import (
	"maps"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/permitio/permit-golang/v2/tools/internal/execfile"
)

// digestStub stands in for go, gh, curl and notify. It acts by the name it was run as, and
// appends its name, its arguments and the secrets in its environment to the file STUB_LOG names.
//
//   - go: -C tools build links the program it builds to the stub.
//   - gh and curl: print, or for curl write to the file after --output, the file in STUB_DIR named
//     like the API path or URL, their last argument, with each of / ? & = . : as _. Without that
//     file they fail as a missing page does.
//   - notify: copies the file after -body to STUB_DIR/posted, and exits with STUB_NOTIFY.
const digestStub = `#!/bin/sh
name=${0##*/}
printf '%s|%s|GH_TOKEN=%s|GITHUB_TOKEN=%s|WEBHOOK=%s\n' "$name" "$*" "${GH_TOKEN-unset}" \
	"${GITHUB_TOKEN-unset}" "${SECURITY_SLACK_WEBHOOK_URL-unset}" >>"$STUB_LOG"
for last in "$@"; do :; done
page="$STUB_DIR/$(printf '%s' "$last" | tr '/?&=.:' '______')"
case "$name" in
go) ln -sf "$STUB" "$5" ;;
gh)
	if [ ! -f "$page" ]; then
		echo "HTTP 404: Not Found ($last)" >&2
		exit 1
	fi
	cat "$page"
	;;
curl)
	out=
	prev=
	for arg in "$@"; do
		[ "$prev" != --output ] || out=$arg
		prev=$arg
	done
	if [ ! -f "$page" ]; then
		echo "curl: (22) The requested URL returned error: 404" >&2
		exit 22
	fi
	cp "$page" "$out"
	;;
notify)
	prev=
	for arg in "$@"; do
		[ "$prev" != -body ] || cp "$arg" "$STUB_DIR/posted"
		prev=$arg
	done
	echo "notify: posted"
	exit "${STUB_NOTIFY:-0}"
	;;
*) exit 99 ;;
esac
`

// page names the stub's file for an API path or URL.
func page(path string) string {
	return strings.NewReplacer("/", "_", "?", "_", "&", "_", "=", "_", ".", "_", ":", "_").
		Replace(path)
}

// The paths and URLs digest.sh reads, and the files the default repository holds.
const (
	pullsPath     = "repos/octo/repo/pulls?state=open&per_page=100"
	scheduledPath = "repos/octo/repo/actions/workflows/ci.yml/runs?branch=master&" +
		"event=schedule&per_page=30"
	manualPath = "repos/octo/repo/actions/workflows/ci.yml/runs?branch=master&" +
		"event=workflow_dispatch&per_page=30"
	gitleaksPath = "repos/gitleaks/gitleaks/releases?per_page=30"
	dlURL        = "https://go.dev/dl/?mode=json&include=all"
	releasesURL  = "https://go.dev/doc/devel/release"
	digestLock   = "# Release binaries.\n[gitleaks]\nversion = 8.30.1\nurl = https://github.com/" +
		"gitleaks/gitleaks/releases/download/v{version}/gitleaks_{version}.tar.gz\n"
	acceptHeader = "# Accepted vulnerabilities.\n\n"
	// legs pins the Go releases of ci.yml's test legs.
	legs   = "env:\n  GO_FLOOR: \"1.26.0\"\n  GO_126: \"1.26.9\"\n  GO_127: \"1.27.1\"\n"
	ciYML  = ".github/workflows/ci.yml"
	inCI   = " in .github/workflows/ci.yml"
	noRuns = `{"workflow_runs":[]}`
)

// jobsPath is the path of a run's jobs.
func jobsPath(id string) string {
	return "repos/octo/repo/actions/runs/" + id + "/jobs?per_page=100"
}

// releasedOn is the release history's markup for a Go release.
func releasedOn(version, date string) string {
	return "<p id=\"" + version + "\">\n\t\t" + version + "\n\t\t(released " + date +
		")\n\t\tincludes fixes.\n</p>\n"
}

// digestCase is one run of digest.sh.
type digestCase struct {
	name string
	// pages replace or, when empty, remove the default pages the stub serves; files replace
	// the default files of the repository.
	pages, files map[string]string
	env          []string
	args         []string
	notify       string
	wantExit     int
	// wantResult is the -result notify gets, when it runs; wantDigest is the posted digest.
	wantResult, wantDigest string
	wantStdout             []string
}

func TestDigest(t *testing.T) {
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	ago := func(d time.Duration) string { return now.Add(-d).Format(time.RFC3339) }
	const day = 24 * time.Hour
	pull := func(number int, login, title string, age time.Duration) string {
		return `{"number":` + strconv.Itoa(number) + `,"title":"` + title + `","user":{"login":"` +
			login + `"},"html_url":"https://github.com/octo/repo/pull/` + strconv.Itoa(number) +
			`","created_at":"` + ago(age) + `"}`
	}
	release := func(tag, published string, draft, pre bool) string {
		return `{"tag_name":"` + tag + `","draft":` + strconv.FormatBool(draft) +
			`,"prerelease":` + strconv.FormatBool(pre) + `,"published_at":"` + published + `"}`
	}
	clean := "*Dependabot pull requests*\n• None open.\n" +
		"*Accepted vulnerabilities*\n• None due within 30 days.\n" +
		"*Last security scan*\n• <https://github.com/octo/repo/actions/runs/12/job/1|success> " +
		"on 2026-10-12\n" +
		"*Go*\n• go1.27.1, the toolchain line of go.mod, is the newest go1.27.\n" +
		"• 1.26.9, GO_126" + inCI + ", is the newest go1.26.\n" +
		"• 1.27.1, GO_127" + inCI + ", is the newest go1.27.\n" +
		"*Release binaries*\n• gitleaks 8.30.1 is the newest release.\n"
	tests := []digestCase{
		{
			name: "nothing needs attention", wantExit: 0,
			wantResult: "nothing needs attention", wantDigest: clean,
		},
		{
			name: "items that need attention, at the edges of 14, 30 and 7 days",
			pages: map[string]string{
				pullsPath: "[" + pull(5, "dependabot[bot]", "Bump <x> & y", 24*day) + "," +
					pull(6, "octo", "Fix a typo", 40*day) + "," +
					pull(8, "dependabot[bot]", "Bump a", 14*day) + "]" +
					"[" + pull(9, "dependabot[bot]", "Bump b", 14*day-time.Second) + "," +
					pull(7, "dependabot[bot]", "Bump z", 3*day) + "]",
				scheduledPath: `{"workflow_runs":[{"id":"../21","event":"schedule",` +
					`"created_at":"2026-10-12T04:23:00Z"},{"id":21,"event":"schedule",` +
					`"created_at":"2026-10-12T04:23:00Z"}]}`,
				manualPath: `{"workflow_runs":[{"id":22,"event":"workflow_dispatch",` +
					`"created_at":"2026-10-09T04:00:00Z"}]}`,
				jobsPath("21"): `{"jobs":[{"name":"security-scan","conclusion":"skipped"},` +
					`{"name":"digest","conclusion":"success"}]}`,
				jobsPath("22"): `{"jobs":[{"name":"security-scan","conclusion":"failure",` +
					`"html_url":"https://github.com/octo/repo/actions/runs/22/job/2",` +
					`"completed_at":"2026-10-09T04:41:00Z"}]}`,
				dlURL: `[{"version":"go1.27.3","stable":true},` +
					`{"version":"go1.27.2","stable":true},` +
					`{"version":"go1.28rc1","stable":false},{"version":"go1.27.1","stable":true}]`,
				releasesURL: releasedOn("go1.27.1", "2026-09-01") +
					releasedOn("go1.27.2", "2026-10-07") + releasedOn("go1.27.3", "2026-10-08"),
				gitleaksPath: "[" + release("v9.0.0", ago(30*day), true, false) + "," +
					release("v9.0.0-rc1", ago(30*day), false, true) + "," +
					release("v8.32.0", ago(7*day-time.Second), false, false) + "," +
					release("v8.31.0", ago(7*day), false, false) + "," +
					release("v8.30.1", ago(60*day), false, false) + "]",
			},
			files: map[string]string{
				".github/vulnscan-accept.txt": acceptHeader +
					"GO-2026-0004 golang.org/x/text a reason 2026-11-14\n" +
					"GO-2026-0003 golang.org/x/text a reason 2026-11-13\n" +
					"GO-2026-0002 golang.org/x/text a reason 2026-10-14\n" +
					"  GO-2026-0001 golang.org/x/net a longer reason 2026-10-13\n",
			},
			wantExit: 0, wantResult: "9 items need attention",
			wantDigest: "*Dependabot pull requests*\n" +
				"• <https://github.com/octo/repo/pull/5|#5> Bump &lt;x&gt; &amp; y: open 24 " +
				"days, " +
				"*14 days or more*\n" +
				"• <https://github.com/octo/repo/pull/8|#8> Bump a: open 14 days, *14 days or " +
				"more*\n" +
				"• <https://github.com/octo/repo/pull/9|#9> Bump b: open 13 days\n" +
				"• <https://github.com/octo/repo/pull/7|#7> Bump z: open 3 days\n" +
				"*Accepted vulnerabilities*\n" +
				"• GO-2026-0001 in golang.org/x/net: expired on 2026-10-13\n" +
				"• GO-2026-0002 in golang.org/x/text: its last day is today, 2026-10-14\n" +
				"• GO-2026-0003 in golang.org/x/text: its last day is 2026-11-13, in 30 days\n" +
				"*Last security scan*\n" +
				"• <https://github.com/octo/repo/actions/runs/22/job/2|failure> on 2026-10-09\n" +
				"*Go*\n" +
				"• go1.27.2, released 2026-10-07, can be adopted: move the toolchain line of " +
				"go.mod from go1.27.1.\n" +
				"• go1.27.3, released 2026-10-08, can be adopted for the toolchain line of " +
				"go.mod from 2026-10-15.\n" +
				"• 1.26.9, GO_126" + inCI + ", is the newest go1.26.\n" +
				"• go1.27.2, released 2026-10-07, can be adopted: move GO_127" + inCI +
				" from 1.27.1.\n" +
				"• go1.27.3, released 2026-10-08, can be adopted for GO_127" + inCI +
				" from 2026-10-15.\n" +
				"*Release binaries*\n" +
				"• gitleaks 8.30.1: 8.31.0, published 2026-10-07, can be adopted.\n" +
				"• gitleaks 8.30.1: 8.32.0, published 2026-10-07, can be adopted from " +
				"2026-10-14.\n",
		},
		{
			name: "a lock entry that is not a GitHub release, and no scan",
			pages: map[string]string{
				scheduledPath: `{"workflow_runs":[{"id":11,"event":"push",` +
					`"created_at":"2026-10-13T09:00:00Z"}]}`,
			},
			files: map[string]string{
				"tools/binaries.lock": digestLock + "\n[other]\nversion = 1.0.0\n" +
					"url = https://example.com/other-{version}.tar.gz\n",
			},
			wantExit: 0, wantResult: "2 items need attention",
			wantDigest: strings.Replace(strings.Replace(clean,
				"• <https://github.com/octo/repo/actions/runs/12/job/1|success> on 2026-10-12\n",
				"• No security-scan job in the 30 newest scheduled runs or the 30 newest manual "+
					"runs of CI on master.\n", 1),
				"• gitleaks 8.30.1 is the newest release.\n",
				"• gitleaks 8.30.1 is the newest release.\n• other 1.0.0: its url is not a "+
					"GitHub release, so check it by hand.\n", 1),
		},
		{
			name: "a Go patch released exactly 7 days before",
			env:  []string{"DIGEST_NOW=" + strconv.FormatInt(now.Truncate(day).Unix(), 10)},
			pages: map[string]string{
				dlURL: `[{"version":"go1.27.2","stable":true},` +
					`{"version":"go1.27.1","stable":true}]`,
				releasesURL: releasedOn("go1.27.1", "2026-09-01") +
					releasedOn("go1.27.2", "2026-10-07"),
			},
			wantExit: 0, wantResult: "2 items need attention",
			wantStdout: []string{
				"• go1.27.2, released 2026-10-07, can be adopted: move the toolchain line of " +
					"go.mod from go1.27.1.\n",
				"• go1.27.2, released 2026-10-07, can be adopted: move GO_127" + inCI +
					" from 1.27.1.\n",
			},
		},
		{
			name: "a manual run newer than the scheduled ones",
			pages: map[string]string{
				manualPath: `{"workflow_runs":[{"id":13,"event":"workflow_dispatch",` +
					`"created_at":"2026-10-13T09:00:00Z"}]}`,
				jobsPath("13"): `{"jobs":[{"name":"security-scan","conclusion":"failure",` +
					`"html_url":"https://github.com/octo/repo/actions/runs/13/job/3",` +
					`"completed_at":"2026-10-13T09:30:00Z"}]}`,
			},
			wantExit: 0, wantResult: "1 items need attention",
			wantDigest: strings.Replace(clean,
				"• <https://github.com/octo/repo/actions/runs/12/job/1|success> on 2026-10-12\n",
				"• <https://github.com/octo/repo/actions/runs/13/job/3|failure> on 2026-10-13\n",
				1),
		},
		{
			name:     "a newer Go patch for a test leg",
			files:    map[string]string{ciYML: strings.Replace(legs, `"1.26.9"`, `"1.26.8"`, 1)},
			wantExit: 0, wantResult: "1 items need attention",
			wantDigest: strings.Replace(clean, "• 1.26.9, GO_126"+inCI+", is the newest go1.26.\n",
				"• go1.26.9, released 2026-09-02, can be adopted: move GO_126"+inCI+
					" from 1.26.8.\n", 1),
		},
		{
			name: "ci.yml without the Go release of a test leg", wantExit: 2,
			files:      map[string]string{ciYML: "env:\n  GO_FLOOR: \"1.26.0\"\n"},
			wantResult: "did not complete: 1 sections could not be read",
			wantStdout: []string{
				"digest: Go could not be read:\ndigest: .github/workflows/ci.yml pins no Go " +
					"release for a test leg (GO_1NN)\n",
			},
		},
		{
			name: "a newer Go patch without a release date",
			pages: map[string]string{
				dlURL: `[{"version":"go1.27.2","stable":true},` +
					`{"version":"go1.27.1","stable":true}]`,
			},
			wantExit: 0, wantResult: "nothing needs attention",
			wantDigest: strings.Replace(strings.Replace(clean,
				"• go1.27.1, the toolchain line of go.mod, is the newest go1.27.\n",
				"• go1.27.2, newer than the toolchain line of go.mod, has no release date on "+
					"go.dev yet.\n", 1),
				"• 1.27.1, GO_127"+inCI+", is the newest go1.27.\n",
				"• go1.27.2, newer than GO_127"+inCI+", has no release date on go.dev yet.\n", 1),
		},
		{
			name: "sections that cannot be read", wantExit: 2,
			pages:      map[string]string{pullsPath: "", gitleaksPath: `{"message":"Not Found"}`},
			wantResult: "did not complete: 2 sections could not be read",
			wantDigest: strings.Replace(strings.Replace(clean, "• None open.\n",
				"• Could not be read; the run's log says why.\n", 1),
				"• gitleaks 8.30.1 is the newest release.\n",
				"• Could not be read; the run's log says why.\n", 1),
			wantStdout: []string{
				"digest: Dependabot pull requests could not be read:\nHTTP 404: Not Found (" +
					pullsPath + ")\n",
				"digest: Release binaries could not be read:\njq: error",
				"gh returned something other than a list of releases",
				"digest: DID NOT COMPLETE: 2 sections could not be read\n",
			},
		},
		{
			name: "pull requests that are not a list", wantExit: 2,
			pages:      map[string]string{pullsPath: `{"message":"Bad credentials"}`},
			wantResult: "did not complete: 1 sections could not be read",
			wantStdout: []string{
				"digest: gh returned something other than lists of pull requests\n",
			},
		},
		{
			name: "go.mod without a toolchain line", wantExit: 2,
			files:      map[string]string{"go.mod": "module example.com/m\n\ngo 1.26.0\n"},
			wantResult: "did not complete: 1 sections could not be read",
			wantStdout: []string{"digest: go.mod has no toolchain line with a Go patch release\n"},
		},
		{
			name: "a release history without the toolchain's date", wantExit: 2,
			pages:      map[string]string{releasesURL: releasedOn("go1.27.2", "2026-10-07")},
			wantResult: "did not complete: 1 sections could not be read",
			wantStdout: []string{
				"go.dev/doc/devel/release has no release date for go1.27.1",
			},
		},
		{
			name: "go.dev that cannot be reached", wantExit: 2,
			pages:      map[string]string{dlURL: ""},
			wantResult: "did not complete: 1 sections could not be read",
			wantStdout: []string{
				"digest: Go could not be read:\ncurl: (22) The requested URL returned error: 404\n",
			},
		},
		{
			name: "an accept entry whose last field is not a date", wantExit: 2,
			files: map[string]string{
				".github/vulnscan-accept.txt": acceptHeader + "GO-2026-0001 m why never\n",
			},
			wantResult: "did not complete: 1 sections could not be read",
			wantStdout: []string{"digest: Accepted vulnerabilities could not be read:\njq: error"},
		},
		{
			name: "the digest could not be posted", notify: "1", wantExit: 1,
			wantResult: "nothing needs attention", wantDigest: clean,
		},
		{
			name: "an argument", args: []string{"now"}, wantExit: 2,
			wantStdout: []string{
				"digest: usage: digest.sh (it takes no arguments)\ndigest: DID NOT RUN\n",
			},
		},
		{
			name: "GITHUB_REPOSITORY that is not owner/name", env: []string{"GITHUB_REPOSITORY="},
			wantExit: 2,
			wantStdout: []string{
				"digest: GITHUB_REPOSITORY \"\" is not owner/name; set it to the repository to " +
					"report on, as CI does\n",
			},
		},
		{
			name: "DIGEST_NOW that is not a number", env: []string{"DIGEST_NOW=today"},
			wantExit:   2,
			wantStdout: []string{"digest: DIGEST_NOW \"today\" is not a number of seconds\n"},
		},
	}
	stub := filepath.Join(t.TempDir(), "stub")
	if err := execfile.WriteFile(stub, []byte(digestStub), 0o700); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	for _, name := range []string{"go", "gh", "curl"} {
		if err := os.Symlink(stub, filepath.Join(bin, name)); err != nil {
			t.Fatal(err)
		}
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runDigest(t, tt, stub, bin, now)
		})
	}
}

// runDigest runs digest.sh in a new repository and checks the result.
func runDigest(t *testing.T, tt digestCase, stub, bin string, now time.Time) {
	t.Helper()
	root := t.TempDir()
	copyScripts(t, root, "digest.sh")
	files := map[string]string{
		"go.mod":                      "module example.com/m\n\ngo 1.26.0\n\ntoolchain go1.27.1\n",
		"tools/binaries.lock":         digestLock,
		".github/vulnscan-accept.txt": acceptHeader,
		ciYML:                         legs,
	}
	maps.Copy(files, tt.files)
	writeFiles(t, root, files)
	if err := os.Mkdir(filepath.Join(root, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	pages := map[string]string{
		pullsPath: "[]",
		scheduledPath: `{"workflow_runs":[{"id":12,"event":"schedule",` +
			`"created_at":"2026-10-12T04:23:00Z"}]}`,
		manualPath: noRuns,
		jobsPath("12"): `{"jobs":[{"name":"security-scan","conclusion":"success","html_url":` +
			`"https://github.com/octo/repo/actions/runs/12/job/1",` +
			`"completed_at":"2026-10-12T04:40:00Z"}]}`,
		dlURL: `[{"version":"go1.27.1","stable":true},{"version":"go1.27rc1","stable":false},` +
			`{"version":"go1.27.9","stable":false},{"version":"go1.26.9","stable":true}]`,
		releasesURL: releasedOn("go1.26.9", "2026-09-02") + releasedOn("go1.27.0", "2026-08-19") +
			releasedOn("go1.27.1", "2026-09-01"),
		gitleaksPath: `[{"tag_name":"v8.30.1","draft":false,"prerelease":false,` +
			`"published_at":"2026-09-01T00:00:00Z"}]`,
	}
	maps.Copy(pages, tt.pages)
	stubDir := t.TempDir()
	for path, content := range pages {
		if content == "" {
			continue
		}
		if err := os.WriteFile(filepath.Join(stubDir, page(path)), []byte(content),
			0o600); err != nil {
			t.Fatal(err)
		}
	}
	tmp, log := t.TempDir(), filepath.Join(t.TempDir(), "log")
	env := append([]string{
		//nolint:forbidigo // the stubs go before the inherited PATH
		"PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH"), "TMPDIR=" + tmp,
		"STUB=" + stub, "STUB_LOG=" + log, "STUB_DIR=" + stubDir, "STUB_NOTIFY=" + tt.notify,
		"DIGEST_NOW=" + strconv.FormatInt(now.Unix(), 10), "GITHUB_REPOSITORY=octo/repo",
		"GITHUB_SERVER_URL=https://github.com", "GITHUB_RUN_ID=9", "GH_TOKEN=gh-token",
		"GITHUB_TOKEN=github-token", "SECURITY_SLACK_WEBHOOK_URL=https://hooks.example.com/x",
	}, tt.env...)

	got := runScript(t, root, env, filepath.Join(root, ".github", "scripts", "digest.sh"),
		tt.args...)

	if got.exit != tt.wantExit {
		t.Errorf("exit = %d, want %d; stdout:\n%s\nstderr:\n%s", got.exit, tt.wantExit,
			got.stdout, got.stderr)
	}
	for _, want := range tt.wantStdout {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout does not contain %q:\n%s", want, got.stdout)
		}
	}
	stopped, _ := splitStopped(t, got.stdout)
	posted, _ := os.ReadFile(filepath.Join(stubDir, "posted"))
	if tt.wantDigest != "" && string(posted) != tt.wantDigest {
		t.Errorf("the posted digest is:\n%s\nwant:\n%s", posted, tt.wantDigest)
	}
	if len(posted) > 0 && !strings.Contains(stopped, string(posted)) {
		t.Errorf("stdout does not print the digest with workflow commands stopped:\n%s",
			got.stdout)
	}
	checkDigestLog(t, log, tt.wantResult)
	if entries, err := os.ReadDir(tmp); err != nil || len(entries) != 0 {
		t.Errorf("digest.sh left %d entries in TMPDIR (%v), want none", len(entries), err)
	}
}

// checkDigestLog checks that the token reached gh only, the webhook notify only, and that notify
// got the digest with the result and the run's URL.
func checkDigestLog(t *testing.T, log, wantResult string) {
	t.Helper()
	data, err := os.ReadFile(log)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	var notified []string
	for line := range strings.Lines(string(data)) {
		line = strings.TrimSuffix(line, "\n")
		name, _, _ := strings.Cut(line, "|")
		secrets := "|GH_TOKEN=unset|GITHUB_TOKEN=unset|WEBHOOK=unset"
		switch name {
		case "gh":
			secrets = "|GH_TOKEN=gh-token|GITHUB_TOKEN=unset|WEBHOOK=unset"
		case "notify":
			notified = append(notified, line)
			secrets = "|GH_TOKEN=unset|GITHUB_TOKEN=unset|WEBHOOK=https://hooks.example.com/x"
		}
		if !strings.HasSuffix(line, secrets) {
			t.Errorf("%s ran with %s, want %s", name, line, secrets)
		}
	}
	if wantResult == "" {
		if len(notified) != 0 {
			t.Errorf("notify ran: %q", notified)
		}
		return
	}
	want := "notify|-channel security -repo octo/repo -check security digest -result " +
		wantResult + " -body "
	if len(notified) != 1 || !strings.HasPrefix(notified[0], want) ||
		!strings.Contains(notified[0], " -run-url https://github.com/octo/repo/actions/runs/9|") {
		t.Errorf("notify ran %q; want it once with %q and the run's URL", notified, want)
	}
}
