package scripts

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// trivyStub is a stand-in for Trivy. For --version it prints the version that STUB_VERSION names.
// For a scan it appends its working directory, its arguments, the files in its working directory
// and the TRIVY_ variables of its environment to the file STUB_LOG names, prints the file that
// STUB_REPORT names, logs a line to stderr and exits with STUB_EXIT.
const trivyStub = `#!/bin/sh
if [ "$1" = --version ]; then
	printf 'Version: %s\nVulnerability DB:\n  Version: 2\n' "$STUB_VERSION"
	exit 0
fi
{
	echo "pwd $PWD"
	for arg in "$@"; do echo "arg $arg"; done
	ls -A | sed 's/^/file /'
	env | grep '^TRIVY_' | sed 's/^/env /'
} >>"$STUB_LOG"
cat "$STUB_REPORT"
echo "FATAL stub log line" >&2
exit "$STUB_EXIT"
`

const trivyLock = "[trivy]\nversion = 0.75.0\nurl = https://example.com/trivy-{version}.tar.gz\n"

// trivyReport returns the Trivy report fixture name, with change applied to its decoded form when
// change is not nil.
func trivyReport(t *testing.T, name string, change func(report map[string]any)) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	if change == nil {
		return string(data)
	}
	var report map[string]any
	if err = json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	change(report)
	data, err = json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// firstVuln applies change to the first vulnerability of a report's first result.
func firstVuln(change func(v map[string]any)) func(report map[string]any) {
	return func(report map[string]any) {
		results, _ := report["Results"].([]any)
		result, _ := results[0].(map[string]any)
		vulns, _ := result["Vulnerabilities"].([]any)
		v, _ := vulns[0].(map[string]any)
		change(v)
	}
}

func TestTrivy(t *testing.T) {
	const (
		noRequires = "module example.com/m\n\ngo 1.26.0\n"
		oneRequire = noRequires + "\nrequire golang.org/x/oauth2 v0.14.0\n"
		xnet       = noRequires + "\nrequire golang.org/x/net v0.23.0\n"
		local      = "require example.com/local v1.0.0\n"
		didNotRun  = "trivy: DID NOT RUN\n"
		oauth2CVE  = "\nCVE-2025-22868 (GO-2025-3488)  "
		oauth2Pkg  = "\n    golang.org/x/oauth2 v0.14.0\n    golang.org/x/oauth2/jws: Unexpected " +
			"memory consumption during token parsing in golang.org/x/oauth2/jws\n"
	)
	resultsNull := func(report map[string]any) { report["Results"] = nil }
	tests := []struct {
		name string
		// policy is the first argument, block when empty; gomod is the module's go.mod, which is
		// not written when empty; report is what Trivy prints, and code its exit code.
		policy, gomod, report string
		code                  string
		// version is what bin/trivy --version reports, 0.75.0 when empty; noTrivy leaves
		// bin/trivy out, and lock replaces tools/binaries.lock.
		version, lock string
		noTrivy       bool
		args          []string
		wantExit      int
		wantStdout    []string
	}{
		{
			name: "Results null and no requirements", gomod: noRequires,
			report:   trivyReport(t, "trivy-empty.json", resultsNull),
			wantExit: 0,
			wantStdout: []string{
				"\ntrivy: MODULE/go.mod: Trivy 0.75.0; packages: 0, require directives: 0\n" +
					"trivy: HIGH or CRITICAL with a fixed version: 0, other vulnerabilities: 0 " +
					"(block)\ntrivy: PASS\n",
			},
		},
		{
			name: "Results null and one requirement", gomod: oneRequire,
			report: trivyReport(t, "trivy-empty.json", resultsNull), wantExit: 2,
			wantStdout: []string{
				"trivy: Trivy reported no results for MODULE/go.mod, which has 1 require " +
					"directives\n" + didNotRun,
			},
		},
		{
			name: "no Results and no requirements", gomod: noRequires,
			report: trivyReport(t, "trivy-empty.json", nil), wantExit: 0,
			wantStdout: []string{"packages: 0, require directives: 0\n"},
		},
		{
			name: "no Results and one requirement", gomod: oneRequire,
			report: trivyReport(t, "trivy-empty.json", nil), wantExit: 2,
			wantStdout: []string{"trivy: Trivy reported no results for MODULE/go.mod"},
		},
		{
			name: "only the module itself", gomod: noRequires,
			report: trivyReport(t, "trivy-root.json", nil), wantExit: 0,
			wantStdout: []string{"packages: 0, require directives: 0\n", "trivy: PASS\n"},
		},
		{
			name: "a HIGH vulnerability with a fixed version", gomod: oneRequire,
			report: trivyReport(t, "trivy-oauth2.json", nil), wantExit: 1,
			wantStdout: []string{
				oauth2CVE + "FAIL, HIGH, fixed in 0.27.0" + oauth2Pkg,
				"trivy: MODULE/go.mod: Trivy 0.75.0; packages: 1, require directives: 1\n" +
					"trivy: HIGH or CRITICAL with a fixed version: 1, other vulnerabilities: 0 " +
					"(block)\ntrivy: FAIL\n",
			},
		},
		{
			name: "a HIGH vulnerability in a reported tree", policy: "report", gomod: oneRequire,
			report: trivyReport(t, "trivy-oauth2.json", nil), wantExit: 0,
			wantStdout: []string{
				oauth2CVE + "reported only, HIGH, fixed in 0.27.0" + oauth2Pkg,
				"with a fixed version: 1, other vulnerabilities: 0 (report)\ntrivy: PASS\n",
			},
		},
		{
			name: "a CRITICAL vulnerability with a fixed version", gomod: oneRequire,
			report: trivyReport(t, "trivy-oauth2.json", firstVuln(func(v map[string]any) {
				v["Severity"] = "CRITICAL"
			})),
			wantExit:   1,
			wantStdout: []string{oauth2CVE + "FAIL, CRITICAL, fixed in 0.27.0\n"},
		},
		{
			name: "a HIGH vulnerability without a fixed version", gomod: oneRequire,
			report: trivyReport(t, "trivy-oauth2.json", firstVuln(func(v map[string]any) {
				delete(v, "FixedVersion")
				v["Status"] = "affected"
			})),
			wantExit: 0,
			wantStdout: []string{
				oauth2CVE + "reported only, HIGH, no fixed version\n",
				"with a fixed version: 0, other vulnerabilities: 1 (block)\ntrivy: PASS\n",
			},
		},
		{
			name: "a MEDIUM vulnerability with a fixed version", gomod: oneRequire,
			report: trivyReport(t, "trivy-oauth2.json", firstVuln(func(v map[string]any) {
				v["Severity"] = "MEDIUM"
				delete(v, "VendorIDs")
			})),
			wantExit: 0,
			wantStdout: []string{
				"\nCVE-2025-22868  reported only, MEDIUM, fixed in 0.27.0\n",
				"trivy: PASS\n",
			},
		},
		{
			name: "HIGH, MEDIUM and UNKNOWN vulnerabilities, failing ones first", gomod: xnet,
			report: trivyReport(t, "trivy-xnet.json", nil), wantExit: 1,
			wantStdout: []string{
				"\nCVE-2024-45338 (GHSA-w32m-9786-jp63, GO-2024-3333)  FAIL, HIGH, fixed in " +
					"0.33.0\n",
				"\nCVE-2026-78669 (GO-2026-6611)  FAIL, HIGH, fixed in 0.60.0\n" +
					"    golang.org/x/net v0.23.0\n",
				"\nCVE-2025-22870 (GHSA-qxp5-gwg8-xv66, GO-2025-3503)  reported only, MEDIUM, " +
					"fixed in 0.36.0\n",
				"\nCVE-2026-78659 (GO-2026-6603)  reported only, UNKNOWN, fixed in 0.60.0\n" +
					"    golang.org/x/net v0.23.0\n\n",
				"with a fixed version: 7, other vulnerabilities: 11 (block)\ntrivy: FAIL\n",
			},
		},
		{
			name: "fewer packages than requirements",
			gomod: oneRequire + "require golang.org/x/text " +
				"v0.42.0 // indirect\n",
			report:   trivyReport(t, "trivy-oauth2.json", nil),
			wantExit: 2,
			wantStdout: []string{
				"trivy: Trivy listed 1 packages for MODULE/go.mod, which has 2 require " +
					"directives\n" + didNotRun,
			},
		},
		{
			name:   "a requirement replaced by a local directory, which Trivy leaves out",
			policy: "report", gomod: oneRequire + local + "replace example.com/local => ../local\n",
			report: trivyReport(t, "trivy-oauth2.json", nil), wantExit: 0,
			wantStdout: []string{
				"\ntrivy: MODULE/go.mod: Trivy 0.75.0; packages: 1, require directives: 2, 1 of " +
					"them replaced by a local directory, which Trivy leaves out\n",
			},
		},
		{
			name:   "the required version replaced by a local directory",
			gomod:  noRequires + local + "replace example.com/local v1.0.0 => ../local\n",
			report: trivyReport(t, "trivy-root.json", nil), wantExit: 0,
			wantStdout: []string{
				"packages: 0, require directives: 1, 1 of them replaced by a local directory",
			},
		},
		{
			name:   "another version replaced by a local directory",
			gomod:  noRequires + local + "replace example.com/local v0.9.0 => ../local\n",
			report: trivyReport(t, "trivy-root.json", nil), wantExit: 2,
			wantStdout: []string{
				"trivy: Trivy listed 0 packages for MODULE/go.mod, which has 1 require " +
					"directives\n" + didNotRun,
			},
		},
		{
			name:   "a requirement replaced by another module version",
			gomod:  noRequires + local + "replace example.com/local => example.com/fork v1.2.0\n",
			report: trivyReport(t, "trivy-root.json", nil), wantExit: 2,
			wantStdout: []string{
				"trivy: Trivy listed 0 packages for MODULE/go.mod, which has 1 require " +
					"directives\n" + didNotRun,
			},
		},
		{
			name: "more packages than requirements", gomod: noRequires,
			report: trivyReport(t, "trivy-oauth2.json", nil), wantExit: 2,
			wantStdout: []string{"trivy: Trivy listed 1 packages for MODULE/go.mod, which has 0"},
		},
		{
			name: "two results for one go.mod", gomod: oneRequire,
			report: trivyReport(t, "trivy-oauth2.json", func(report map[string]any) {
				results, _ := report["Results"].([]any)
				report["Results"] = append(results, results[0])
			}),
			wantExit:   2,
			wantStdout: []string{"trivy: Trivy reported 2 targets for one go.mod\n" + didNotRun},
		},
		{
			name: "Trivy failed", gomod: noRequires, code: "1", wantExit: 2,
			report:     trivyReport(t, "trivy-root.json", nil),
			wantStdout: []string{"FATAL stub log line\ntrivy: Trivy exited 1\n" + didNotRun},
		},
		{
			name: "output that is not JSON", gomod: noRequires, report: "2026-10-13 INFO done\n",
			wantExit:   2,
			wantStdout: []string{"trivy: Trivy's output is not a JSON report\n" + didNotRun},
		},
		{
			name: "empty output", gomod: noRequires, wantExit: 2,
			wantStdout: []string{"trivy: Trivy's output is not a JSON report\n"},
		},
		{
			name: "JSON that is not a Trivy report", gomod: noRequires, report: `{"Results":[]}`,
			wantExit:   2,
			wantStdout: []string{"trivy: Trivy's output is not a JSON report\n"},
		},
		{
			name: "no bin/trivy", gomod: noRequires, noTrivy: true, wantExit: 2,
			wantStdout: []string{
				"trivy: bin/trivy is missing; install it with .github/scripts/fetch-tool.sh " +
					"trivy\n" + didNotRun,
			},
		},
		{
			name: "another Trivy version", gomod: noRequires, version: "0.74.0", wantExit: 2,
			wantStdout: []string{
				"trivy: bin/trivy reports \"Version: 0.74.0\", want version 0.75.0 from " +
					"tools/binaries.lock; install it with .github/scripts/fetch-tool.sh trivy\n",
			},
		},
		{
			name: "a lock without trivy", gomod: noRequires, lock: "[prek]\nversion = 0.5.5\n",
			wantExit:   2,
			wantStdout: []string{"trivy: tools/binaries.lock has no version for trivy\n"},
		},
		{
			name: "a go.mod that cannot be read", gomod: "module example.com/m\nrequire (\n",
			wantExit:   2,
			wantStdout: []string{"trivy: go mod edit -json could not read MODULE/go.mod\n"},
		},
		{
			name: "no go.mod", wantExit: 2,
			wantStdout: []string{"trivy: MODULE has no go.mod\n" + didNotRun},
		},
		{
			name: "another policy", policy: "warn", gomod: noRequires, wantExit: 2,
			wantStdout: []string{"trivy: the policy \"warn\" is not block or report\n"},
		},
		{
			name: "no arguments", args: []string{}, wantExit: 2,
			wantStdout: []string{"trivy: usage: trivy.sh <block|report> <module directory>\n"},
		},
	}
	// Every repository links bin/trivy to one stub, since some systems check each new executable
	// the first time it runs, which is slow.
	stub := filepath.Join(t.TempDir(), "trivy")
	if err := os.WriteFile(stub, []byte(trivyStub), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			copyScripts(t, root, "trivy.sh")
			lock := trivyLock
			if tt.lock != "" {
				lock = tt.lock
			}
			writeFiles(t, root, map[string]string{"tools/binaries.lock": lock})
			if !tt.noTrivy {
				if err := os.Mkdir(filepath.Join(root, "bin"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(stub, filepath.Join(root, "bin", "trivy")); err != nil {
					t.Fatal(err)
				}
			}
			module := t.TempDir()
			if tt.gomod != "" {
				writeFiles(t, module, map[string]string{"go.mod": tt.gomod})
			}
			work := t.TempDir()
			logFile, reportFile := filepath.Join(work, "log"), filepath.Join(work, "report")
			if err := os.WriteFile(reportFile, []byte(tt.report), 0o600); err != nil {
				t.Fatal(err)
			}
			version, code, policy := "0.75.0", "0", "block"
			if tt.version != "" {
				version = tt.version
			}
			if tt.code != "" {
				code = tt.code
			}
			if tt.policy != "" {
				policy = tt.policy
			}
			args := []string{policy, module}
			if tt.args != nil {
				args = tt.args
			}
			env := []string{
				"STUB_LOG=" + logFile, "STUB_REPORT=" + reportFile, "STUB_EXIT=" + code,
				"STUB_VERSION=" + version, "TRIVY_SEVERITY=LOW", "TRIVY_IGNOREFILE=.trivyignore",
			}

			got := runScript(t, root, env, filepath.Join(root, ".github", "scripts", "trivy.sh"),
				args...)

			stdout := strings.ReplaceAll(got.stdout, module, "MODULE")
			if got.exit != tt.wantExit {
				t.Errorf("exit = %d, want %d; stdout:\n%s\nstderr:\n%s", got.exit, tt.wantExit,
					stdout, got.stderr)
			}
			for _, want := range tt.wantStdout {
				if !strings.Contains(stdout, want) {
					t.Errorf("stdout does not contain %q:\n%s", want, stdout)
				}
			}
			if !strings.HasSuffix(stdout, map[int]string{
				0: "trivy: PASS\n", 1: "trivy: FAIL\n", 2: didNotRun,
			}[tt.wantExit]) {
				t.Errorf("stdout does not end with the verdict:\n%s", stdout)
			}
			if tt.wantExit != 2 || strings.Contains(stdout, "packages:") {
				checkTrivyCall(t, logFile, module, root)
			}
		})
	}

	t.Run("without jq", func(t *testing.T) {
		root := t.TempDir()
		copyScripts(t, root, "trivy.sh")
		writeFiles(t, root, map[string]string{"tools/binaries.lock": trivyLock})
		if err := os.Mkdir(filepath.Join(root, "bin"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(stub, filepath.Join(root, "bin", "trivy")); err != nil {
			t.Fatal(err)
		}
		module := t.TempDir()
		writeFiles(t, module, map[string]string{"go.mod": oneRequire})
		// PATH has every program trivy.sh runs but jq.
		bin := t.TempDir()
		for _, tool := range []string{"bash", "dirname", "awk", "go"} {
			path, err := exec.LookPath(tool)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(path, filepath.Join(bin, tool)); err != nil {
				t.Fatal(err)
			}
		}
		env := []string{"PATH=" + bin, "STUB_VERSION=0.75.0"}

		got := runScript(t, root, env, filepath.Join(root, ".github", "scripts", "trivy.sh"),
			"block", module)

		want := "trivy: jq is not installed, so Trivy's report cannot be read\n" + didNotRun
		if got.exit != 2 || got.stdout != want {
			t.Errorf("exit = %d, stdout:\n%s\nwant exit 2, stdout:\n%s", got.exit, got.stdout,
				want)
		}
	})
}

// checkTrivyCall checks that the stub ran once, in an empty directory outside the repository and
// the module, with every setting on its command line and no TRIVY_ variable in its environment.
func checkTrivyCall(t *testing.T, logFile, module, root string) {
	t.Helper()
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	gomod, err := filepath.EvalSymlinks(filepath.Join(module, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"arg fs", "arg --scanners", "arg vuln", "arg --list-all-pkgs", "arg --format", "arg json",
		"arg --exit-code", "arg 0", "arg --config", "arg ", "arg --ignorefile", "arg ",
		"arg --skip-version-check", "arg --disable-telemetry", "arg --no-progress",
	}
	if len(lines) != len(want)+2 || !strings.HasPrefix(lines[0], "pwd ") ||
		!slices.Equal(lines[1:len(want)+1], want) {
		t.Fatalf("Trivy ran with:\n%s\nwant one run with the arguments:\n%s",
			data, strings.Join(want, "\n"))
	}
	scanned, err := filepath.EvalSymlinks(strings.TrimPrefix(lines[len(want)+1], "arg "))
	if err != nil || scanned != gomod {
		t.Errorf("Trivy scanned %s, want %s", lines[len(want)+1], gomod)
	}
	dir := strings.TrimPrefix(lines[0], "pwd ")
	for _, other := range []string{module, root} {
		if resolved, _ := filepath.EvalSymlinks(other); strings.HasPrefix(dir, resolved) ||
			strings.HasPrefix(dir, other) {
			t.Errorf("Trivy ran in %s, inside %s", dir, other)
		}
	}
}
