package scripts

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/permitio/permit-golang/v2/tools/internal/execfile"
)

// scanStub stands in for every program security-scan.sh runs but git, jq, gitleaks and secreport.
// It acts by the name it was run as, and appends its name, working directory, arguments and the
// secrets in its environment to the file STUB_LOG names. Each step exits with the code that
// STUB_CODES gives it, as space-separated "<step>=<code>" pairs, and 0 otherwise.
//
//   - go: -C tools build links the program it builds to STUB_SECREPORT for secreport and to the
//     stub for notify (step build:<program>). For the consumer module, mod init writes go.mod;
//     list -m answers by STUB_LATEST: none, error, or the version to print, after a download
//     line on stderr; list -f prints three packages, one of them internal; get does nothing, and
//     mod tidy copies main.go to STUB_DIR.
//   - vuln.sh: --prepare makes the ceiling with a go.mod and links bin/vulnscan to the stub.
//   - vulnscan and trivy.sh: step vulnscan-<tree> and trivy-<tree>; unless the step exits 2 or
//     STUB_NO_FINDINGS names it, they write the findings that STUB_DIR/<step>.findings holds,
//     or an empty findings file.
//   - fetch-tool.sh: links bin/gitleaks to STUB_GITLEAKS and bin/trufflehog to the stub.
//   - gitleaks, when STUB_GITLEAKS is the stub: version prints STUB_GITLEAKS_VERSION; a scan
//     (step gitleaks) logs STUB_DIR/gitleaks.log on stderr and writes an empty report.
//   - grep, when a case puts it first in PATH: -cvxFf fails as a read error does, and any other
//     grep runs STUB_GREP, the real one.
//   - gh: prints STUB_DIR/alerts.json.
//   - trufflehog: --version prints STUB_TRUFFLEHOG_VERSION; a scan logs GIT_CONFIG_GLOBAL,
//     prints STUB_DIR/trufflehog.json and logs STUB_DIR/trufflehog.log on stderr.
//   - notify: copies the file after -body to STUB_DIR/posted.
const scanStub = `#!/bin/sh
name=${0##*/}
{
	printf '%s|%s|%s|' "$name" "$PWD" "$*"
	printf 'GH_TOKEN=%s|GITHUB_TOKEN=%s|' "${GH_TOKEN-unset}" "${GITHUB_TOKEN-unset}"
	printf 'ALERTS=%s|WEBHOOK=%s|' "${DEPENDABOT_ALERTS_TOKEN-unset}" \
		"${SECURITY_SLACK_WEBHOOK_URL-unset}"
	printf 'ACCEPT=%s|GITLEAKS=%s\n' "${GITLEAKS_HISTORY_ACCEPT_SHA256-unset}" \
		"${GITLEAKS_CONFIG-unset}"
} >>"$STUB_LOG"
value_after() {
	want=$1
	shift
	while [ $# -gt 1 ]; do
		if [ "$1" = "$want" ]; then echo "$2"; return; fi
		shift
	done
}
findings() {
	case " $STUB_NO_FINDINGS " in
	*" $step "*) return ;;
	esac
	if [ -f "$STUB_DIR/$step.findings" ]; then
		cp "$STUB_DIR/$step.findings" "$1"
	else
		: >"$1"
	fi
}
code=0
case "$name" in
go)
	if [ "$1" = -C ]; then
		step="build:${6##*/}"
		case $step in
		build:secreport) ln -sf "$STUB_SECREPORT" "$5" ;;
		*) ln -sf "$STUB" "$5" ;;
		esac
	else
		step="go:$1"
		case "$1 $2" in
		"mod init") echo "module $3" >go.mod ;;
		"list -m")
			step=go:latest
			case $STUB_LATEST in
			none) echo "go: module $4: no matching versions for query \"latest\"" >&2; exit 1 ;;
			error) echo "go: dial tcp: lookup proxy.golang.org: no such host" >&2; exit 1 ;;
			*)
				echo "go: downloading ${4%@*} $STUB_LATEST" >&2
				printf '{"Path":"%s","Version":"%s"}\n' "${4%@*}" "$STUB_LATEST"
				;;
			esac
			;;
		"list -f")
			step=go:packages
			m=${4%/...}
			printf '%s\n%s/models\n%s/internal/x\n' "$m" "$m" "$m"
			;;
		"mod tidy") cp main.go "$STUB_DIR/main.go" ;;
		esac
		echo "GOPROXY=$GOPROXY GOFLAGS=$GOFLAGS GOSUMDB=$GOSUMDB" >>"$STUB_LOG"
	fi
	;;
vuln.sh)
	step=prepare
	mkdir "$2" && echo "module x" >"$2/go.mod"
	ln -sf "$STUB" bin/vulnscan
	;;
vulnscan)
	tree=$(value_after -only "$@")
	[ -n "$tree" ] || tree=consumer
	step="vulnscan-$tree"
	echo "vulnscan: $tree"
	;;
trivy.sh)
	step="trivy-$2"
	[ "$2" != . ] || step=trivy-floor
	echo "trivy: report $2"
	;;
fetch-tool.sh)
	step="fetch-$1"
	case $1 in
	gitleaks) ln -sf "$STUB_GITLEAKS" bin/gitleaks ;;
	*) ln -sf "$STUB" "bin/$1" ;;
	esac
	echo "$PWD/bin/$1"
	;;
gitleaks)
	if [ "$1" = version ]; then
		echo "$STUB_GITLEAKS_VERSION"
		exit 0
	fi
	step=gitleaks
	[ ! -f "$STUB_DIR/gitleaks.log" ] || cat "$STUB_DIR/gitleaks.log" >&2
	echo '[]' >"$(value_after --report-path "$@")"
	;;
grep)
	if [ "$1" = -cvxFf ]; then
		echo "grep: $3: Input/output error" >&2
		exit 2
	fi
	exec "$STUB_GREP" "$@"
	;;
gh)
	step=gh
	cat "$STUB_DIR/alerts.json"
	;;
trufflehog)
	if [ "$1" = --version ]; then
		echo "trufflehog $STUB_TRUFFLEHOG_VERSION"
		exit 0
	fi
	step=trufflehog
	echo "GIT_CONFIG_GLOBAL=$GIT_CONFIG_GLOBAL" >>"$STUB_LOG"
	[ ! -f "$STUB_DIR/trufflehog.json" ] || cat "$STUB_DIR/trufflehog.json"
	[ ! -f "$STUB_DIR/trufflehog.log" ] || cat "$STUB_DIR/trufflehog.log" >&2
	;;
notify)
	step=notify
	body=$(value_after -body "$@")
	[ -z "$body" ] || cp "$body" "$STUB_DIR/posted"
	echo "notify: posted"
	;;
*) exit 99 ;;
esac
for pair in $STUB_CODES; do
	if [ "${pair%%=*}" = "$step" ]; then code=${pair#*=}; fi
done
if [ "$code" -le 1 ]; then
	case $step in
	vulnscan-*) findings "$(value_after -findings "$@")" ;;
	trivy-*) findings "$3" ;;
	esac
fi
exit "$code"
`

// scanCase is one run of security-scan.sh.
type scanCase struct {
	name string
	// keys is the number of Permit API keys the history holds, each in a commit of its own, and
	// in keyDir, and accepted how many of them, from the first, GITLEAKS_HISTORY_ACCEPT_SHA256
	// accepts; extraDigest adds a digest that matches no finding.
	keys, accepted int
	keyDir         string
	extraDigest    bool
	// files are written in STUB_DIR: the findings of a step, alerts.json, trufflehog.json,
	// trufflehog.log and gitleaks.log.
	files map[string]string
	codes string
	// latest is STUB_LATEST, none when empty; trufflehog passes --trufflehog run.
	latest     string
	trufflehog bool
	// shallow runs the script in a shallow clone of the repository; fakeGitleaks runs the stub
	// as gitleaks; failGrep puts the stub first in PATH as grep.
	shallow, fakeGitleaks, failGrep bool
	// env is added to the environment; args replace the arguments.
	env  []string
	args []string
	// wantExit is the exit code; wantResult the -result notify gets, when it runs; wantMain the
	// consumer's main.go, when it is written.
	wantExit   int
	wantResult string
	wantMain   string
	// want are parts of stdout, the summary and the posted report; wantNot must be in none, and
	// wantPrivate in the posted report only.
	wantStdout, wantSummary, wantPosted, wantNot, wantPrivate []string
}

// truffleHogLog is the log of a TruffleHog scan that read the given number of chunks and
// verified the given number of secrets. Its last line holds raw, which the script must never
// print.
func truffleHogLog(raw string, chunks, verified int) string {
	return "not JSON\n" + fmt.Sprintf(`{"level":"info-0","logger":"trufflehog","msg":"finished `+
		`scanning","chunks":%d,"bytes":%d,"verified_secrets":%d,"unverified_secrets":0}`,
		chunks, 100*chunks, verified) + "\n" +
		`{"level":"info-0","logger":"trufflehog","msg":"found","raw":"` + raw + "\"}\n"
}

func TestSecurityScan(t *testing.T) {
	t.Parallel()
	const (
		module   = "github.com/permitio/permit-golang/v2"
		highNet  = `{"id":"GHSA-4444-5555-6666","package":"golang.org/x/net","severity":"high"}`
		cleanLog = "security-scan: dependabot-alerts\nsecurity-scan: octo/repo has 0 open " +
			"Dependabot alerts\n"
	)
	// raw is the secret TruffleHog verified; it must never be printed.
	raw := "permit_key_" + rand.Text()
	verified := fmt.Sprintf(`{"SourceMetadata":{"Data":{"Git":{"commit":"%s","file":"key1.go",`+
		`"line":1}}},"DetectorName":"PermitIO","Verified":true,"Raw":"%s",`+
		`"Redacted":"permit_key_***"}`, strings.Repeat("ab", 20), raw)
	alert := `{"security_advisory":{"ghsa_id":"GHSA-1111-2222-3333","severity":"critical"},` +
		`"security_vulnerability":{"package":{"name":"golang.org/x/net"},` +
		`"vulnerable_version_range":"< 0.40.0","first_patched_version":{"identifier":"0.40.0"}}}`
	// longVerified is a verified secret whose file and redacted value are longer than secreport
	// accepts.
	longFile := strings.Repeat("d/", 120) + "key1.go"
	longVerified, err := json.Marshal(map[string]any{
		"SourceMetadata": map[string]any{"Data": map[string]any{"Git": map[string]any{
			"commit": strings.Repeat("ab", 20), "file": longFile, "line": 1,
		}}},
		"DetectorName": "PermitIO", "Verified": true, "Raw": raw,
		"Redacted": "a\tb" + strings.Repeat("r", 320),
	})
	if err != nil {
		t.Fatal(err)
	}
	const (
		notSet = "history: 0 findings, 0 accepted, 0 new; GITLEAKS_HISTORY_ACCEPT_SHA256 is not " +
			"set, so every finding is new"
		noCount = "security-scan: TruffleHog's log does not count a scan that read something " +
			"and verified the "
	)
	tests := []scanCase{
		{
			name: "every source completed and found nothing", wantExit: 0, wantResult: "PASS",
			wantStdout: []string{
				"security-scan: master-floor\nvulnscan: floor\ntrivy: report .\n",
				"security-scan: master-ceiling\nvulnscan: ceiling\ntrivy: report ceiling\n",
				"security-scan: tools\nvulnscan: tools\ntrivy: report tools\n",
				"security-scan: consumer-v2-latest\nsecurity-scan: " + module +
					" has no release yet\n",
				cleanLog,
				" scanned 1 commits of every ref\n" + notSet + "\n",
				"secreport: Clean: all 6 scans completed, and none found anything.\n" +
					"secreport: Security scan: PASS\nnotify: posted\n",
			},
			wantSummary: []string{
				"## Security scan: PASS\n\nClean: all 6 scans completed, and none found anything.",
				"| consumer-v2-latest | PASS; no release yet |\n",
				"| history-secrets | PASS; " + notSet + " |\n",
			},
			wantPosted: []string{
				"*Clean: all 6 scans completed, and none found anything.*\n*Scans*\n" +
					"• master-floor: PASS\n",
				"• history-secrets: PASS; " + notSet + "\n",
			},
		},
		{
			// trivy.sh report exits 0 whatever it finds.
			name:     "a scanner that exits 0 with findings",
			files:    map[string]string{"trivy-floor.findings": highNet + "\n"},
			wantExit: 0, wantResult: "PASS",
			wantStdout: []string{
				"secreport: master-floor: FINDINGS (exit 1)\n",
				"secreport: Findings: 1 (0 critical, 1 high, 0 medium, 0 low, 0 unscored), from " +
					"master-floor; all 6 scans completed.\n",
				"::warning title=Security scan::master-floor: FINDINGS (exit 1); the job summary " +
					"lists them\n",
			},
			wantNot: []string{"Clean"},
		},
		{
			// The planted failure of the history scan: a third key, which no digest accepts.
			name: "findings, a third key in the history among them, are reported and pass",
			keys: 3, accepted: 2,
			files: map[string]string{
				"vulnscan-floor.findings": highNet + "\n",
				"alerts.json":             "[" + alert + "]",
			},
			codes: "vulnscan-floor=1", latest: "v2.0.0",
			wantExit: 0, wantResult: "PASS",
			wantStdout: []string{
				"security-scan: " + module + "@latest is v2.0.0\n",
				"security-scan: dependabot-alerts\nsecurity-scan: octo/repo has 1 open " +
					"Dependabot alerts\n",
				"secreport: dependabot-alerts: FINDINGS (exit 1); 1 open alerts\n",
				"history: 3 findings, 2 accepted, 1 new\n",
				"secreport: history-secrets: FINDINGS (exit 1); history: 3 findings, 2 accepted, " +
					"1 new\n",
				"secreport: Findings: 3 (1 critical, 1 high, 0 medium, 0 low, 1 unscored), from " +
					"master-floor, dependabot-alerts, history-secrets; all 6 scans completed.\n",
				"::warning title=Security scan::master-floor: FINDINGS (exit 1); the job summary " +
					"lists them\n",
				"::warning title=Security scan::dependabot-alerts: FINDINGS (exit 1); 1 open " +
					"alerts; the Slack report lists them\n",
				"::warning title=Security scan::history-secrets: FINDINGS (exit 1); history: 3 " +
					"findings, 2 accepted, 1 new; the Slack report lists them\n",
				"secreport: Security scan: PASS\n",
			},
			wantSummary: []string{
				"| consumer-v2-latest | PASS; latest release v2.0.0 |\n",
				"2 findings are listed in the Slack report only.\n",
			},
			wantPosted: []string{
				"• history-secrets: FINDINGS (exit 1); history: 3 findings, 2 accepted, 1 new\n",
				"*High and critical*\n• CRITICAL <https://github.com/advisories/" +
					"GHSA-1111-2222-3333|GHSA-1111-2222-3333> golang.org/x/net &lt; 0.40.0: fix: " +
					"0.40.0 (dependabot-alerts)\n",
				"*Unscored*\n• permit-api-key key3.go:1 ",
			},
			// The open Dependabot alerts are private.
			wantPrivate: []string{"GHSA-1111-2222-3333", "0.40.0"},
		},
		{
			// Values that secreport would reject are cut or dropped, so no finding is lost.
			name: "an alert and a key with values longer than the report accepts", keys: 1,
			keyDir: strings.Repeat("d/", 150),
			files: map[string]string{
				"alerts.json": `[{"security_advisory":{"ghsa_id":"GHSA-(x)","severity":` +
					`"critical"},"security_vulnerability":{"package":{"name":"` +
					strings.Repeat("p", 320) + `"},"vulnerable_version_range":"< 1.0\n",` +
					`"first_patched_version":{"identifier":"1.0"}}}]`,
			},
			wantExit: 0, wantResult: "PASS",
			wantStdout: []string{
				"secreport: dependabot-alerts: FINDINGS (exit 1); 1 open alerts\n",
				"secreport: history-secrets: FINDINGS (exit 1); history: 1 findings, 0 " +
					"accepted, 1 new",
			},
			wantPosted: []string{
				"• CRITICAL GHSA-(x) " + strings.Repeat("p", 197) + "... &lt; 1.0: fix: 1.0 " +
					"(dependabot-alerts)\n",
				"*Unscored*\n• permit-api-key " +
					(strings.Repeat("d/", 150) + "key1.go:1")[:197] + "... ",
			},
		},
		{
			name: "an accepted digest that matches no finding", keys: 2, accepted: 2,
			extraDigest: true, wantExit: 2, wantResult: "DID NOT RUN",
			wantStdout: []string{
				"history: 2 findings, 2 accepted, 0 new\nsecurity-scan: 1 accepted digests in " +
					"GITLEAKS_HISTORY_ACCEPT_SHA256 match no finding: the scan may have missed " +
					"part of the history, or the list is out of date\n",
				"secreport: Did not complete: history-secrets. ",
			},
			wantPosted: []string{"*Did not complete*\n• history-secrets: DID NOT RUN (exit 2)"},
			wantNot:    []string{"Clean", "clean"},
		},
		{
			name:     "an accept list with a line that is not a digest",
			env:      []string{"GITLEAKS_HISTORY_ACCEPT_SHA256=" + strings.Repeat("A", 64)},
			wantExit: 2, wantResult: "DID NOT RUN",
			wantStdout: []string{
				"security-scan: GITLEAKS_HISTORY_ACCEPT_SHA256 holds a line that is not a " +
					"sha256 digest in lowercase hex\n",
			},
			wantNot: []string{strings.Repeat("A", 64)},
		},
		{
			name: "an accept list that lists a digest twice", keys: 1,
			env: []string{"GITLEAKS_HISTORY_ACCEPT_SHA256=" + strings.Repeat("a", 64) + "\n " +
				strings.Repeat("a", 64) + "\n"},
			wantExit: 2, wantResult: "DID NOT RUN",
			wantStdout: []string{
				"security-scan: GITLEAKS_HISTORY_ACCEPT_SHA256 lists a digest twice\n",
			},
		},
		{
			name:       "a scanner that exits 2",
			codes:      "trivy-ceiling=2",
			wantExit:   2,
			wantResult: "DID NOT RUN",
			wantStdout: []string{
				"secreport: master-ceiling: DID NOT RUN (exit 2)\n",
				"secreport: Did not complete: master-ceiling. Findings from the 5 of 6 scans",
			},
			wantSummary: []string{
				"### Did not complete\n\n- master-ceiling: DID NOT RUN \\(exit 2\\)",
			},
			wantNot: []string{"Clean", "clean"},
		},
		{
			name:       "a scanner that writes no findings",
			env:        []string{"STUB_NO_FINDINGS=vulnscan-tools"},
			wantExit:   2,
			wantResult: "DID NOT RUN",
			wantStdout: []string{
				"security-scan: tools\nvulnscan: tools\ntrivy: report tools\n" +
					"security-scan: vulnscan wrote no findings file\n",
				"secreport: tools: DID NOT RUN (exit 2)\n",
			},
		},
		{
			name: "a scanner that exits 1 and writes no findings", codes: "vulnscan-tools=1",
			env:      []string{"STUB_NO_FINDINGS=vulnscan-tools"},
			wantExit: 2, wantResult: "DID NOT RUN",
			wantStdout: []string{
				"security-scan: vulnscan wrote no findings file\n",
				"secreport: tools: DID NOT RUN (exit 2)\n",
			},
		},
		{
			name:     "a scanner that exits 2 and one that exits 1 with findings",
			codes:    "vulnscan-floor=2 trivy-floor=1",
			files:    map[string]string{"trivy-floor.findings": highNet + "\n"},
			wantExit: 2, wantResult: "DID NOT RUN",
			wantStdout: []string{"secreport: master-floor: DID NOT RUN (exit 2)\n"},
		},
		{
			name: "a shallow clone", keys: 1, shallow: true, wantExit: 2, wantResult: "DID NOT RUN",
			wantStdout: []string{
				"security-scan: history-secrets\nsecurity-scan: the repository is a shallow " +
					"clone, which lacks part of the history; clone the full history " +
					"(fetch-depth: 0)\n",
				"secreport: history-secrets: DID NOT RUN (exit 2)\n",
			},
		},
		{
			// The log is printed without the colors gitleaks gives it.
			name: "gitleaks that logs an error and exits 0", fakeGitleaks: true,
			files: map[string]string{
				"gitleaks.log": "\x1b[90m4:07AM\x1b[0m \x1b[31mERR\x1b[0m [git] fatal: bad " +
					"object\n",
			},
			wantExit: 2, wantResult: "DID NOT RUN",
			wantStdout: []string{
				"security-scan: history-secrets\n4:07AM ERR [git] fatal: bad object\n" +
					"security-scan: gitleaks logged an error, so it may not have scanned every " +
					"commit\n",
				"secreport: history-secrets: DID NOT RUN (exit 2)\n",
			},
			wantNot: []string{"\x1b"},
		},
		{
			name: "gitleaks that exits 1", fakeGitleaks: true, codes: "gitleaks=1",
			wantExit: 2, wantResult: "DID NOT RUN",
			wantStdout: []string{
				"security-scan: history-secrets\nsecurity-scan: gitleaks exited 1\n",
				"secreport: history-secrets: DID NOT RUN (exit 2)\n",
			},
		},
		{
			name: "grep that fails to compare the accepted digests", keys: 1, accepted: 1,
			failGrep: true, wantExit: 2, wantResult: "DID NOT RUN",
			wantStdout: []string{
				"security-scan: grep could not compare the accepted digests with the findings\n",
				"secreport: history-secrets: DID NOT RUN (exit 2)\n",
			},
		},
		{
			name: "no DEPENDABOT_ALERTS_TOKEN", env: []string{"DEPENDABOT_ALERTS_TOKEN="},
			wantExit: 2, wantResult: "DID NOT RUN",
			wantStdout: []string{
				"security-scan: DEPENDABOT_ALERTS_TOKEN is not set, so the open Dependabot " +
					"alerts were not read; the GITHUB_TOKEN cannot read them\n",
				"secreport: dependabot-alerts: DID NOT RUN (exit 2)\n",
			},
			wantNot: []string{"Clean", "clean"},
		},
		{
			name: "alerts that gh cannot read", codes: "gh=1", wantExit: 2,
			wantResult: "DID NOT RUN",
			wantStdout: []string{
				"security-scan: gh api could not read the open Dependabot alerts of octo/repo\n",
			},
		},
		{
			name: "alerts that are not a list", files: map[string]string{"alerts.json": `{}`},
			wantExit: 2, wantResult: "DID NOT RUN",
			wantStdout: []string{
				"security-scan: gh api returned something other than lists of alerts\n",
			},
		},
		{
			name: "the preparation failed", codes: "prepare=2", wantExit: 2,
			wantResult: "DID NOT RUN",
			wantStdout: []string{
				"security-scan: master-floor\nsecurity-scan: vuln.sh --prepare failed, so this " +
					"tree was not scanned\n",
				"secreport: Did not complete: master-floor, master-ceiling, tools, " +
					"consumer-v2-latest. ",
			},
		},
		{
			name: "latest is a commit, not a release", latest: "v2.0.0-20261014120000-0123456789ab",
			wantExit: 0, wantResult: "PASS",
			wantStdout: []string{
				"security-scan: " + module + "@latest is \"v2.0.0-20261014120000-0123456789ab\", " +
					"not a release; it has no release yet\n",
			},
		},
		{
			name: "the latest release cannot be looked up", latest: "error", wantExit: 2,
			wantResult: "DID NOT RUN",
			wantStdout: []string{
				"go: dial tcp: lookup proxy.golang.org: no such host\nsecurity-scan: go list -m " +
					module + "@latest failed\n",
			},
		},
		{
			name: "the consumer's scanners", latest: "v2.1.0", codes: "vulnscan-consumer=1",
			files: map[string]string{
				"trivy-consumer.findings": `{"id":"CVE-1","package":"p",` +
					`"severity":"medium"}` + "\n",
			},
			wantExit: 0, wantResult: "PASS",
			wantMain: "// Command consumer imports every public package of the SDK, as an " +
				"application does.\npackage main\n\nimport (\n\t_ \"" + module + "\"\n\t_ \"" +
				module + "/models\"\n)\n\nfunc main() {}\n",
			wantStdout: []string{
				"secreport: consumer-v2-latest: FINDINGS (exit 1); latest release v2.1.0\n",
				"Findings: 1 (0 critical, 0 high, 1 medium, 0 low, 0 unscored), from " +
					"consumer-v2-latest; all 6 scans completed.",
			},
		},
		{
			// The planted failure of TruffleHog: a verified secret fails the run.
			name: "TruffleHog verified a secret", trufflehog: true,
			files: map[string]string{
				"trufflehog.json": verified + "\n", "trufflehog.log": truffleHogLog(raw, 5, 1),
			},
			wantExit: 1, wantResult: "FAIL",
			wantStdout: []string{
				"security-scan: verified-secrets\nsecurity-scan: TruffleHog verified secrets: 1\n",
				"secreport: verified-secrets: FAIL (exit 1); TruffleHog verified secrets: 1\n",
				"::error title=Security scan::verified-secrets: FAIL (exit 1)",
				"secreport: Security scan: FAIL\n",
			},
			wantPosted: []string{
				"*High and critical*\n• CRITICAL PermitIO key1.go:1 abababababab: fix: revoke it " +
					"now: TruffleHog verified that it works (permit_key_***) (verified-secrets)\n",
			},
		},
		{
			// What TruffleHog says about a secret is cut to fit the report, so a verified
			// secret is never lost.
			name: "TruffleHog verified a secret with long values", trufflehog: true,
			files: map[string]string{
				"trufflehog.json": string(longVerified) + "\n",
				"trufflehog.log":  truffleHogLog(raw, 5, 1),
			},
			wantExit: 1, wantResult: "FAIL",
			wantStdout: []string{
				"secreport: verified-secrets: FAIL (exit 1); TruffleHog verified secrets: 1\n",
			},
			wantPosted: []string{
				"*High and critical*\n• CRITICAL PermitIO " + (longFile + ":1")[:197] + "... " +
					"abababababab: fix: revoke it now: TruffleHog verified that it works (a b" +
					strings.Repeat("r", 74) + "...) (verified-secrets)\n",
			},
		},
		{
			name:       "TruffleHog found nothing",
			trufflehog: true,
			wantExit:   0,
			wantResult: "PASS",
			wantStdout: []string{
				"secreport: Clean: all 7 scans completed, and none found anything.",
			},
		},
		{
			name: "TruffleHog of another version than the lock", trufflehog: true,
			env:      []string{"STUB_TRUFFLEHOG_VERSION=0.0.1"},
			wantExit: 2, wantResult: "DID NOT RUN",
			wantStdout: []string{
				"security-scan: bin/trufflehog reports \"trufflehog 0.0.1\", want version ",
				"secreport: verified-secrets: DID NOT RUN (exit 2)\n",
			},
		},
		{
			name: "TruffleHog without the count of its scan", trufflehog: true,
			files:    map[string]string{"trufflehog.log": ""},
			wantExit: 2, wantResult: "DID NOT RUN",
			wantStdout: []string{
				noCount + "0 secrets it printed, so it may not have scanned the history\n",
				"secreport: verified-secrets: DID NOT RUN (exit 2)\n",
			},
		},
		{
			name: "TruffleHog that scanned nothing", trufflehog: true,
			files:    map[string]string{"trufflehog.log": truffleHogLog(raw, 0, 0)},
			wantExit: 2, wantResult: "DID NOT RUN",
			wantStdout: []string{noCount + "0 secrets it printed"},
		},
		{
			name: "TruffleHog that counts other verified secrets than it printed",
			files: map[string]string{
				"trufflehog.json": verified + "\n", "trufflehog.log": truffleHogLog(raw, 5, 2),
			},
			trufflehog: true, wantExit: 2, wantResult: "DID NOT RUN",
			wantStdout: []string{noCount + "1 secrets it printed"},
		},
		{
			name: "TruffleHog failed", trufflehog: true, codes: "trufflehog=183", wantExit: 2,
			wantResult: "DID NOT RUN",
			wantStdout: []string{
				"security-scan: trufflehog exited 183; its log may hold secrets, so run it on " +
					"your own clone to see why\n",
			},
		},
		{
			name: "TruffleHog output that is not JSON", trufflehog: true,
			files:    map[string]string{"trufflehog.json": "found " + raw + "\n"},
			wantExit: 2, wantResult: "DID NOT RUN",
			wantStdout: []string{"security-scan: TruffleHog's output is not JSON results\n"},
		},
		{
			name: "the report could not be posted", codes: "notify=1", wantExit: 1,
			wantResult: "PASS",
		},
		{
			name: "GITHUB_REPOSITORY that is not owner/name", env: []string{"GITHUB_REPOSITORY=x"},
			wantExit: 2,
			wantStdout: []string{
				"security-scan: GITHUB_REPOSITORY \"x\" is not owner/name; set it to the " +
					"repository whose alerts to read, as CI does\nsecurity-scan: DID NOT RUN\n",
			},
		},
		{
			name: "secreport that cannot be built", codes: "build:secreport=1", wantExit: 2,
			wantStdout: []string{"security-scan: building secreport failed\n"},
		},
		{
			name: "no arguments", args: []string{}, wantExit: 2,
			wantStdout: []string{
				"security-scan: usage: security-scan.sh --trufflehog run|skip | " +
					"--history-digests <file>\nsecurity-scan: DID NOT RUN\n",
			},
		},
		{
			name: "another --trufflehog", args: []string{"--trufflehog", "always"}, wantExit: 2,
			wantStdout: []string{"security-scan: usage: "},
		},
	}
	s := newScanSetup(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s.run(t, tt, raw)
		})
	}
}

// scanSetup is what every run of security-scan.sh shares: the stub, the real programs and the
// versions tools/binaries.lock pins.
type scanSetup struct {
	stub, bin, grepBin, grep, secreport, gitleaks, lock string
	versions                                            map[string]string
}

func newScanSetup(t *testing.T) scanSetup {
	t.Helper()
	s := scanSetup{gitleaks: gitleaksBinary(t), stub: filepath.Join(t.TempDir(), "stub")}
	if err := execfile.WriteFile(s.stub, []byte(scanStub), 0o700); err != nil {
		t.Fatal(err)
	}
	s.bin, s.grepBin = t.TempDir(), t.TempDir()
	for _, link := range []string{
		filepath.Join(s.bin, "go"), filepath.Join(s.bin, "gh"), filepath.Join(s.grepBin, "grep"),
	} {
		if err := os.Symlink(s.stub, link); err != nil {
			t.Fatal(err)
		}
	}
	s.secreport = filepath.Join(t.TempDir(), "secreport")
	build := exec.CommandContext(t.Context(), "go", "build", "-o", s.secreport,
		"github.com/permitio/permit-golang/v2/tools/cmd/secreport")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building secreport: %v: %s", err, out)
	}
	lock, err := os.ReadFile(repositoryFile("tools/binaries.lock"))
	if err != nil {
		t.Fatal(err)
	}
	if s.grep, err = exec.LookPath("grep"); err != nil {
		t.Fatal(err)
	}
	s.lock = string(lock)
	s.versions = map[string]string{}
	for _, name := range []string{"gitleaks", "trufflehog"} {
		m := regexp.MustCompile(`(?m)^\[` + name + `\]\n(?:[^\[\n].*\n)*?version = (\S+)$`).
			FindStringSubmatch(s.lock)
		if m == nil {
			t.Fatalf("tools/binaries.lock has no version for %s", name)
		}
		s.versions[name] = m[1]
	}
	return s
}

// newScanRepository makes a git repository laid out like this one, with security-scan.sh, the
// stub as the scripts it calls, and keys Permit API keys in commits of their own, in files in
// dir. It returns its root and the fingerprint of each key's finding.
func (s scanSetup) newScanRepository(t *testing.T, keys int, dir string) (string, []string) {
	t.Helper()
	root := t.TempDir()
	linkScripts(t, root, "security-scan.sh")
	for _, name := range []string{"vuln.sh", "trivy.sh", "fetch-tool.sh"} {
		if err := os.Symlink(s.stub, filepath.Join(root, ".github", "scripts", name)); err != nil {
			t.Fatal(err)
		}
	}
	writeFiles(t, root, map[string]string{
		"tools/binaries.lock": s.lock, ".gitignore": "/bin/\n",
		"go.mod": "module github.com/permitio/permit-golang/v2\n\ngo 1.26.0\n",
	})
	if err := os.Mkdir(filepath.Join(root, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "init", "--quiet")
	commitFiles(t, root, nil, "First")
	var fingerprints []string
	for i := 1; i <= keys; i++ {
		// Each key is built when the test runs, so that no key is in this file.
		file := fmt.Sprintf("%skey%d.go", dir, i)
		commit := commitFiles(t, root, map[string]string{
			file: "const k = \"permit_key_" + rand.Text() + rand.Text() + "\"\n",
		}, "Add "+file)
		fingerprints = append(fingerprints, commit+":"+file+":permit-api-key:1")
	}
	return root, fingerprints
}

// digest returns the sha256 of s in lowercase hex.
func digest(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func (s scanSetup) run(t *testing.T, tt scanCase, raw string) {
	t.Helper()
	root, fingerprints := s.newScanRepository(t, tt.keys, tt.keyDir)
	if tt.shallow {
		clone := t.TempDir()
		runGit(t, root, "clone", "--quiet", "--depth", "1", "file://"+root, clone)
		if err := os.Mkdir(filepath.Join(clone, "bin"), 0o755); err != nil {
			t.Fatal(err)
		}
		root = clone
	}
	stubDir := t.TempDir()
	files := map[string]string{"alerts.json": "[]", "trufflehog.log": truffleHogLog(raw, 5, 0)}
	maps.Copy(files, tt.files)
	writeFiles(t, stubDir, files)
	var accept []string
	for _, fp := range fingerprints[:tt.accepted] {
		accept = append(accept, digest(fp))
	}
	if tt.extraDigest {
		accept = append(accept, digest("not a finding"))
	}
	tmp, summary, log := t.TempDir(), filepath.Join(t.TempDir(), "summary"),
		filepath.Join(t.TempDir(), "log")
	// The user's git configuration changes the format of git log, which gitleaks and TruffleHog
	// read; the script keeps it from them.
	userConfig := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(
		userConfig,
		[]byte("[format]\n\tpretty = oneline\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	latest := tt.latest
	if latest == "" {
		latest = "none"
	}
	inherited := os.Getenv("PATH") //nolint:forbidigo // the stubs go before it
	path := s.bin + string(os.PathListSeparator) + inherited
	if tt.failGrep {
		path = s.grepBin + string(os.PathListSeparator) + path
	}
	gitleaks := s.gitleaks
	if tt.fakeGitleaks {
		gitleaks = s.stub
	}
	env := append(gitEnv(root), "PATH="+path,
		"TMPDIR="+tmp, "STUB="+s.stub, "STUB_LOG="+log, "STUB_DIR="+stubDir,
		"STUB_CODES="+tt.codes, "STUB_LATEST="+latest, "STUB_SECREPORT="+s.secreport,
		"STUB_GITLEAKS="+gitleaks, "STUB_GITLEAKS_VERSION="+s.versions["gitleaks"],
		"STUB_TRUFFLEHOG_VERSION="+s.versions["trufflehog"], "STUB_GREP="+s.grep,
		"GITHUB_REPOSITORY=octo/repo", "GITHUB_SERVER_URL=https://github.com", "GITHUB_RUN_ID=9",
		"GITHUB_STEP_SUMMARY="+summary, "GITHUB_ACTIONS=", "GH_TOKEN=gh-token",
		"GITHUB_TOKEN=github-token", "DEPENDABOT_ALERTS_TOKEN=alerts-token",
		"SECURITY_SLACK_WEBHOOK_URL=https://hooks.example.com/services/FAKE",
		"GITLEAKS_HISTORY_ACCEPT_SHA256="+strings.Join(accept, "\n"),
		"GITLEAKS_CONFIG=/allow/everything.toml", "GOPROXY=off", "GOFLAGS=-mod=vendor",
		"GIT_CONFIG_GLOBAL="+userConfig)
	env = append(env, tt.env...)
	args := []string{"--trufflehog", "skip"}
	if tt.trufflehog {
		args = []string{"--trufflehog", "run"}
	}
	if tt.args != nil {
		args = tt.args
	}

	got := runScript(t, root, env, filepath.Join(root, ".github", "scripts", "security-scan.sh"),
		args...)

	if got.exit != tt.wantExit {
		t.Errorf("exit = %d, want %d; stdout:\n%s\nstderr:\n%s", got.exit, tt.wantExit,
			got.stdout, got.stderr)
	}
	stopped, after := splitStopped(t, got.stdout)
	if strings.Contains(stopped, "secreport: ") || strings.Contains(stopped, "notify: ") {
		t.Errorf("secreport or notify ran with workflow commands stopped:\n%s", got.stdout)
	}
	summaryData, _ := os.ReadFile(summary)
	posted, _ := os.ReadFile(filepath.Join(stubDir, "posted"))
	logData, _ := os.ReadFile(log)
	for what, check := range map[string]struct {
		got  string
		want []string
	}{
		"stdout":            {got.stdout, tt.wantStdout},
		"the summary":       {string(summaryData), tt.wantSummary},
		"the posted report": {string(posted), tt.wantPosted},
	} {
		for _, want := range check.want {
			if !strings.Contains(check.got, want) {
				t.Errorf("%s does not contain %q:\n%s", what, want, check.got)
			}
		}
		for _, not := range append([]string{raw}, tt.wantNot...) {
			if strings.Contains(check.got, not) {
				t.Errorf("%s holds %q:\n%s", what, not, check.got)
			}
		}
	}
	for _, private := range tt.wantPrivate {
		if !strings.Contains(string(posted), private) ||
			strings.Contains(got.stdout+string(summaryData), private) {
			t.Errorf("%q is not in the posted report only; stdout:\n%s\nthe summary:\n%s\nthe "+
				"posted report:\n%s", private, got.stdout, summaryData, posted)
		}
	}
	for _, fp := range fingerprints {
		commit, file, _ := strings.Cut(fp, ":")
		file, _, _ = strings.Cut(file, ":")
		if strings.Contains(got.stdout+string(summaryData), commit[:12]) ||
			strings.Contains(after+string(summaryData), file+":") {
			t.Errorf("stdout or the summary names where a key of the history is (%s, %s)",
				commit[:12], file)
		}
	}
	checkScanLog(t, string(logData), tt)
	if tt.wantMain != "" {
		main, err := os.ReadFile(filepath.Join(stubDir, "main.go"))
		if err != nil || string(main) != tt.wantMain {
			t.Errorf("the consumer's main.go is:\n%s\n(%v)\nwant:\n%s", main, err, tt.wantMain)
		}
	}
	if entries, err := os.ReadDir(tmp); err != nil || len(entries) != 0 {
		t.Errorf("security-scan.sh left %d entries in TMPDIR (%v), want none", len(entries), err)
	}
}

// checkScanLog checks that each secret reached only the program that needs it, and that notify
// got the report with the result and the run's URL.
func checkScanLog(t *testing.T, log string, tt scanCase) {
	t.Helper()
	const unset = "|GH_TOKEN=unset|GITHUB_TOKEN=unset|ALERTS=unset|WEBHOOK=unset|ACCEPT=unset|" +
		"GITLEAKS=unset"
	var notified []string
	for line := range strings.Lines(log) {
		line = strings.TrimSuffix(line, "\n")
		name, _, _ := strings.Cut(line, "|")
		switch {
		case strings.HasPrefix(line, "GOPROXY="):
			if line != "GOPROXY=https://proxy.golang.org,direct GOFLAGS=-mod=mod "+
				"GOSUMDB=sum.golang.org" {
				t.Errorf("the consumer's go ran with %s", line)
			}
		case strings.HasPrefix(line, "GIT_CONFIG_GLOBAL="):
			if line != "GIT_CONFIG_GLOBAL=/dev/null" {
				t.Errorf("TruffleHog ran with %s", line)
			}
		case name == "gh":
			want := "|GH_TOKEN=alerts-token|GITHUB_TOKEN=unset|ALERTS=unset|WEBHOOK=unset|" +
				"ACCEPT=unset|GITLEAKS=unset"
			if !strings.HasSuffix(line, want) ||
				!strings.Contains(line, "|api --paginate repos/octo/repo/dependabot/alerts?"+
					"state=open&per_page=100|") {
				t.Errorf("gh ran as %s", line)
			}
		case name == "notify":
			notified = append(notified, line)
			if !strings.HasSuffix(line, "|GH_TOKEN=unset|GITHUB_TOKEN=unset|ALERTS=unset|"+
				"WEBHOOK=https://hooks.example.com/services/FAKE|ACCEPT=unset|GITLEAKS=unset") {
				t.Errorf("notify ran with %s", line)
			}
		case !strings.HasSuffix(line, unset):
			t.Errorf("%s ran with a secret: %s", name, line)
		}
	}
	if tt.wantResult == "" {
		if len(notified) != 0 {
			t.Errorf("notify ran: %q", notified)
		}
		return
	}
	want := "|-channel security -repo octo/repo -check security-scan -result " + tt.wantResult +
		" -body "
	if len(notified) != 1 || !strings.Contains(notified[0], want) ||
		!strings.Contains(notified[0], " -run-url https://github.com/octo/repo/actions/runs/9|") {
		t.Errorf("notify ran %q; want it once with %q and the run's URL", notified, want)
	}
}

// TestSecurityScanHistoryDigests runs the mode that writes the digests of the history's findings.
func TestSecurityScanHistoryDigests(t *testing.T) {
	t.Parallel()
	s := newScanSetup(t)
	root, fingerprints := s.newScanRepository(t, 2, "")
	out := filepath.Join(t.TempDir(), "digests")
	inherited := os.Getenv("PATH") //nolint:forbidigo // the stubs go before it
	env := append(gitEnv(root), "PATH="+s.bin+string(os.PathListSeparator)+inherited,
		"STUB="+s.stub, "STUB_LOG="+filepath.Join(t.TempDir(), "log"), "STUB_GITLEAKS="+s.gitleaks,
		"GITHUB_ACTIONS=")
	script := filepath.Join(root, ".github", "scripts", "security-scan.sh")

	got := runScript(t, root, env, script, "--history-digests", out)

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("exit %d, no digests file: %v; stdout:\n%s", got.exit, err, got.stdout)
	}
	lines := strings.Fields(string(data))
	want := []string{digest(fingerprints[0]), digest(fingerprints[1])}
	slices.Sort(lines)
	slices.Sort(want)
	if got.exit != 0 || !slices.Equal(lines, want) {
		t.Errorf("exit %d, digests %q; want exit 0 and %q", got.exit, lines, want)
	}
	for _, fp := range fingerprints {
		commit, rest, _ := strings.Cut(fp, ":")
		file, _, _ := strings.Cut(rest, ":")
		line := digest(fp) + " permit-api-key " + file + ":1 commit " + commit[:12] + "\n"
		if !strings.Contains(got.stdout, line) {
			t.Errorf("stdout does not list %q:\n%s", line, got.stdout)
		}
	}

	// The subtests run in parallel and share root, so at most one of them may scan it: the others
	// stop at the checks of the arguments, before the script touches root.
	t.Run("in GitHub Actions", func(t *testing.T) {
		t.Parallel()
		got := runScript(t, root, slices.Concat(env, []string{"GITHUB_ACTIONS=true"}), script,
			"--history-digests", out+"2")

		if got.exit != 2 || !strings.Contains(got.stdout, "security-scan: --history-digests "+
			"prints where each finding is, and the log of GitHub Actions is public; run it on "+
			"your own clone\n") {
			t.Errorf("exit %d, stdout:\n%s\nwant exit 2 and a refusal", got.exit, got.stdout)
		}
	})

	t.Run("a path relative to another directory", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		got := runScript(t, dir, env, script, "--history-digests", "digests")

		data, err := os.ReadFile(filepath.Join(dir, "digests"))
		if got.exit != 0 || err != nil || len(strings.Fields(string(data))) != 2 {
			t.Errorf("exit %d, digests %q (%v); want exit 0 and 2 digests in the directory it "+
				"ran in; stdout:\n%s", got.exit, data, err, got.stdout)
		}
	})

	// The digests belong in the secret only, never in a file that the clone could commit.
	for _, inside := range []struct{ name, dir, path string }{
		{"a relative path inside the clone", root, "digests"},
		{"an absolute path inside the clone", t.TempDir(), filepath.Join(root, ".git", "digests")},
	} {
		t.Run(inside.name, func(t *testing.T) {
			t.Parallel()
			got := runScript(t, inside.dir, env, script, "--history-digests", inside.path)

			if got.exit != 2 || !strings.Contains(got.stdout, " is inside the repository; "+
				"write the digests to a file outside the clone\nsecurity-scan: DID NOT RUN\n") {
				t.Errorf("exit %d, stdout:\n%s\nwant exit 2 and a refusal", got.exit, got.stdout)
			}
			for _, written := range []string{"digests", ".git/digests"} {
				if _, err := os.Stat(filepath.Join(root, written)); err == nil {
					t.Errorf("security-scan.sh wrote %s in the clone", written)
				}
			}
		})
	}
}

// TestSecurityScanGitleaksRules checks that the history scan gives gitleaks the rules of
// .gitleaks.toml.
func TestSecurityScanGitleaksRules(t *testing.T) {
	t.Parallel()
	script, err := os.ReadFile(repositoryFile(".github/scripts/security-scan.sh"))
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?s)cat >"\$work/config\.toml" <<'TOML' \|\| return 2\n(.*?)\nTOML\n`).
		FindSubmatch(script)
	if m == nil {
		t.Fatal("security-scan.sh writes no gitleaks configuration")
	}
	committed, err := os.ReadFile(repositoryFile(".gitleaks.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := tomlLines(m[1]), tomlLines(committed); !slices.Equal(got, want) {
		t.Errorf("security-scan.sh gives gitleaks:\n%s\nwant the lines of .gitleaks.toml:\n%s",
			strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
