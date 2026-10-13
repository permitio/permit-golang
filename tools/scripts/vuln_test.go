package scripts

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/permitio/permit-golang/v2/tools/internal/execfile"
)

// vulnStub stands in for every program vuln.sh runs: go on PATH, fetch-tool.sh and trivy.sh in
// .github/scripts, and bin/govulncheck, bin/vulnscan and bin/secreport, which the stub links to
// itself when vuln.sh builds them with go. It acts by the name it was run as. It appends its name,
// its working directory, its arguments and its GH_TOKEN and GITHUB_TOKEN to the file STUB_LOG
// names; go get also logs the files of the ceiling, and secreport the results it was given. Each
// step exits with the code that STUB_CODES gives it, as space-separated "<step>=<code>" pairs, and
// 0 otherwise. A step is build:<package>, get, mod, fetch, vulnscan, trivy:<directory> or
// secreport.
const vulnStub = `#!/bin/sh
name=${0##*/}
{
	printf '%s|%s|' "$name" "$PWD"
	printf '%s ' "$@"
	printf '|GH_TOKEN=%s|GITHUB_TOKEN=%s\n' "${GH_TOKEN-unset}" "${GITHUB_TOKEN-unset}"
} >>"$STUB_LOG"
case "$name" in
go)
	if [ "$1" = -C ]; then
		ln -s "$STUB" "$5"
		step="build:$6"
	else
		step=$1
		[ "$1" != get ] || ls -A | sed 's/^/ceiling has /' >>"$STUB_LOG"
	fi
	;;
fetch-tool.sh) step=fetch ;;
trivy.sh) step="trivy:$2"; echo "trivy $1 $2" ;;
vulnscan) step=vulnscan; echo "vulnscan: scanned" ;;
secreport)
	step=secreport
	for file in "$6"/*; do
		printf 'result %s: %s\n' "${file##*/}" "$(cat "$file")" >>"$STUB_LOG"
	done
	echo "secreport: verdict"
	;;
*) exit 99 ;;
esac
for pair in $STUB_CODES; do
	if [ "${pair%%=*}" = "$step" ]; then exit "${pair#*=}"; fi
done
exit 0
`

func TestVuln(t *testing.T) {
	const (
		scanned = "|GH_TOKEN=unset|GITHUB_TOKEN=unset"
		names   = "govulncheck trivy-floor trivy-ceiling trivy-tools "
	)
	build := func(out, pkg string) string {
		return "go|ROOT|-C tools build -o ROOT/bin/" + out + " " + pkg + " " + scanned
	}
	prepared := []string{
		build("govulncheck", "golang.org/x/vuln/cmd/govulncheck"),
		build("vulnscan", "./cmd/vulnscan"),
		build("secreport", "./cmd/secreport"),
		"fetch-tool.sh|ROOT|trivy " + scanned,
		"go|WORK/ceiling|get -u -t ./... " + scanned,
		"ceiling has .github", "ceiling has go.mod", "ceiling has tools",
		"go|WORK/ceiling|mod tidy " + scanned,
	}
	scans := []string{
		"vulnscan|ROOT|-ceiling WORK/ceiling " + scanned,
		"trivy.sh|ROOT|block . " + scanned,
		"trivy.sh|WORK|block ceiling " + scanned,
		"trivy.sh|ROOT|report tools " + scanned,
	}
	report := "secreport|ROOT|-id vuln -title Vulnerabilities -dir WORK/results "
	// results are the lines secreport logs for the results directory, whose scans exited with
	// codes, in the order of their names.
	results := func(codes ...string) []string {
		outputs := [][2]string{
			{"govulncheck", "vulnscan: scanned"},
			{"trivy-ceiling", "trivy block ceiling"},
			{"trivy-floor", "trivy block ."},
			{"trivy-tools", "trivy report tools"},
		}
		var lines []string
		for i, scan := range outputs {
			lines = append(lines, "result "+scan[0]+".exit: "+codes[i],
				"result "+scan[0]+".out: "+scan[1])
		}
		return lines
	}
	tests := []struct {
		name  string
		args  []string
		env   []string
		codes string
		// wantExit is vuln.sh's exit code, and wantLog the lines the stub logs.
		wantExit   int
		wantLog    []string
		wantStdout []string
	}{
		{
			name: "every scan runs, and secreport decides", wantExit: 0,
			wantLog: slices.Concat(prepared, scans, []string{
				report + names + "|GH_TOKEN=gh-token|GITHUB_TOKEN=unset",
			}, results("0", "0", "0", "0")),
			wantStdout: []string{
				"vuln: govulncheck\nvulnscan: scanned\nvuln: trivy-floor\ntrivy block .\n" +
					"vuln: trivy-ceiling\ntrivy block ceiling\nvuln: trivy-tools\n" +
					"trivy report tools\n::TOKEN::\nsecreport: verdict\n",
			},
		},
		{
			name: "the CI job's variables", wantExit: 0,
			env: []string{
				"GITHUB_STEP_SUMMARY=/summary", "GITHUB_SERVER_URL=https://github.com",
				"GITHUB_REPOSITORY=octo/repo", "GITHUB_RUN_ID=99", "PR_NUMBER=7",
				"PR_HEAD_REPO=fork/repo",
			},
			wantLog: slices.Concat(prepared, scans, []string{
				report + "-summary /summary " +
					"-run-url https://github.com/octo/repo/actions/runs/99 " +
					"-pr 7 -repo octo/repo -head-repo fork/repo " + names +
					"|GH_TOKEN=gh-token|GITHUB_TOKEN=unset",
			}, results("0", "0", "0", "0")),
		},
		{
			name: "scans that fail or do not run", wantExit: 1,
			codes: "vulnscan=1 trivy:ceiling=2 secreport=1",
			wantLog: slices.Concat(prepared, scans, []string{
				report + names + "|GH_TOKEN=gh-token|GITHUB_TOKEN=unset",
			}, results("1", "2", "0", "0")),
		},
		{
			name: "secreport exits 2", codes: "secreport=2", wantExit: 2,
			wantLog: slices.Concat(prepared, scans, []string{
				report + names + "|GH_TOKEN=gh-token|GITHUB_TOKEN=unset",
			}, results("0", "0", "0", "0")),
		},
		{
			name: "a program that does not build", codes: "build:./cmd/vulnscan=1", wantExit: 2,
			wantLog: prepared[:2],
			wantStdout: []string{
				"vuln: building vulnscan failed\nvuln: DID NOT RUN\n::TOKEN::\n",
			},
		},
		{
			name: "Trivy that cannot be installed", codes: "fetch=2", wantExit: 2,
			wantLog: prepared[:4],
			wantStdout: []string{
				"vuln: installing Trivy with .github/scripts/fetch-tool.sh failed\n" +
					"vuln: DID NOT RUN\n::TOKEN::\n",
			},
		},
		{
			name: "a ceiling whose dependencies cannot be updated", codes: "get=1", wantExit: 2,
			wantLog: prepared[:8],
			wantStdout: []string{
				"vuln: moving the dependencies of the ceiling to their newest versions failed\n" +
					"vuln: DID NOT RUN\n::TOKEN::\n",
			},
		},
		{
			name: "an argument", args: []string{"--fast"}, wantExit: 2,
			wantStdout: []string{
				"::stop-commands::TOKEN\nvuln: usage: vuln.sh (it takes no arguments)\n" +
					"vuln: DID NOT RUN\n::TOKEN::\n",
			},
		},
	}
	stub := filepath.Join(t.TempDir(), "stub")
	if err := execfile.WriteFile(stub, []byte(vulnStub), 0o700); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	if err := os.Symlink(stub, filepath.Join(bin, "go")); err != nil {
		t.Fatal(err)
	}
	inherited := os.Getenv("PATH") //nolint:forbidigo // the stub goes before it
	token := regexp.MustCompile(`[0-9a-f]{32}`)
	workDir := regexp.MustCompile(`TMP/vuln\.[A-Za-z0-9]+`)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			copyScripts(t, root, "vuln.sh")
			writeFiles(t, root, map[string]string{
				"go.mod": "module example.com/m\n", "tools/go.mod": "module example.com/m/tools\n",
				".git/HEAD": "ref: refs/heads/main\n", "bin/old": "an old build\n",
			})
			for _, name := range []string{"fetch-tool.sh", "trivy.sh"} {
				if err := os.Symlink(stub, filepath.Join(root, ".github", "scripts",
					name)); err != nil {
					t.Fatal(err)
				}
			}
			tmp := t.TempDir()
			log := filepath.Join(t.TempDir(), "log")
			env := append([]string{
				"PATH=" + bin + string(os.PathListSeparator) + inherited, "TMPDIR=" + tmp,
				"STUB=" + stub, "STUB_LOG=" + log, "STUB_CODES=" + tt.codes,
				"GH_TOKEN=gh-token", "GITHUB_TOKEN=github-token", "GITHUB_STEP_SUMMARY=",
				"GITHUB_SERVER_URL=", "GITHUB_REPOSITORY=", "GITHUB_RUN_ID=", "PR_NUMBER=",
				"PR_HEAD_REPO=",
			}, tt.env...)

			got := runScript(t, root, env, filepath.Join(root, ".github", "scripts", "vuln.sh"),
				tt.args...)

			normalize := func(s string) string {
				s = strings.ReplaceAll(s, tmp, "TMP")
				s = strings.ReplaceAll(s, root, "ROOT")
				return workDir.ReplaceAllString(token.ReplaceAllString(s, "TOKEN"), "WORK")
			}
			stdout := normalize(got.stdout)
			if got.exit != tt.wantExit {
				t.Errorf("exit = %d, want %d; stdout:\n%s\nstderr:\n%s", got.exit, tt.wantExit,
					stdout, got.stderr)
			}
			for _, want := range tt.wantStdout {
				if !strings.Contains(stdout, want) {
					t.Errorf("stdout does not contain %q:\n%s", want, stdout)
				}
			}
			checkStopCommands(t, got.stdout, token)
			data, err := os.ReadFile(log)
			if err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			var lines []string
			if len(data) > 0 {
				lines = strings.Split(strings.TrimSuffix(normalize(string(data)), "\n"), "\n")
			}
			if !slices.Equal(lines, tt.wantLog) {
				t.Errorf("the stub logged:\n%s\nwant:\n%s", strings.Join(lines, "\n"),
					strings.Join(tt.wantLog, "\n"))
			}
			if entries, err := os.ReadDir(tmp); err != nil || len(entries) != 0 {
				t.Errorf("vuln.sh left %d entries in TMPDIR (%v), want none", len(entries), err)
			}
		})
	}
}

// checkStopCommands checks that stdout starts with a stop-commands line and resumes the commands
// once, with the same token, after the scans and before secreport.
func checkStopCommands(t *testing.T, stdout string, token *regexp.Regexp) {
	t.Helper()
	first, _, _ := strings.Cut(stdout, "\n")
	tok := token.FindString(first)
	if first != "::stop-commands::"+tok || tok == "" {
		t.Fatalf("the first line is %q, want ::stop-commands:: and a random token", first)
	}
	if n := strings.Count(stdout, "\n::"+tok+"::\n"); n != 1 || strings.Count(stdout, tok) != 2 {
		t.Errorf("stdout resumes the commands %d times, want once:\n%s", n, stdout)
	}
	if strings.Contains(stdout, "secreport: verdict") &&
		strings.Index(stdout, "::"+tok+"::") > strings.Index(stdout, "secreport: verdict") {
		t.Errorf("secreport runs before the commands resume:\n%s", stdout)
	}
}
