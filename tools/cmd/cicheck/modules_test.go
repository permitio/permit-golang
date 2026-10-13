package main

import (
	_ "embed"
	"path/filepath"
	"strings"
	"testing"
)

// validDependabot is a Dependabot configuration with a gomod entry for the root and for tools,
// and a github-actions entry, each with the cooldown the rules ask for. The tests change one part
// at a time; the line numbers they expect are this file's.
//
//go:embed testdata/dependabot.yml
var validDependabot string

// validHooks is a hook configuration that checks that the root and tools are tidy.
//
//go:embed testdata/pre-commit-config.yaml
var validHooks string

// setExceptions sets moduleExceptions for the rest of the test.
func setExceptions(t *testing.T, exceptions map[string]string) {
	t.Helper()
	saved := moduleExceptions
	t.Cleanup(func() { moduleExceptions = saved })
	moduleExceptions = exceptions
}

func TestCheckModules(t *testing.T) {
	const (
		toolsEntry = "  - package-ecosystem: gomod\n    directory: /tools\n" +
			"    schedule:\n      interval: weekly\n" +
			"    cooldown:\n      default-days: 7\n      semver-major-days: 14\n"
		actionsEntry = "  - package-ecosystem: github-actions\n    directory: /\n" +
			"    schedule:\n      interval: weekly\n" +
			"    cooldown:\n      default-days: 7\n"
		// oldEntry is a gomod entry, at line 23, for a directory without a module.
		oldEntry = "  - package-ecosystem: gomod\n    directory: /old\n" +
			"    schedule:\n      interval: weekly\n" +
			"    cooldown:\n      default-days: 7\n      semver-major-days: 14\n"
		toolsDir     = "    directory: /tools\n"
		toolsAndMore = "    directories: [/tools, /consumer]\n"
		otherBranch  = "    target-branch: release/v1\n"
		toolsHook    = "entry: go -C tools mod tidy -diff"
		consumerHook = "      - id: go-mod-tidy-consumer\n" +
			"        entry: go -C consumer mod tidy -diff\n        language: system\n"
		otherHook     = "      - id: go-vet\n"
		goMod         = "module example.com/m\n"
		upgradeReason = "it pins the last v1 release on purpose"
		noEntry       = "no gomod entry in .github/dependabot.yml without a target-branch has " +
			"the "
		noHook = "no hook in .pre-commit-config.yaml has the entry "
		// notTidy ends a finding about a missing tidy hook, and cooldownHint one about a cooldown.
		notTidy      = ", so a go.mod or go.sum that is not tidy passes; add one\n"
		cooldownHint = "so Dependabot proposes a version only once it is "
		excepted     = "moduleExceptions in tools/cmd/cicheck "
		toolsNoHook  = "cicheck: tools/go.mod: " + noHook + "go -C tools mod tidy -diff" + notTidy
		passed       = "cicheck: checked 2 Go modules and 3 Dependabot entries\n" +
			"cicheck: checked 1 workflows, 3 jobs, 6 steps and 2 scripts\ncicheck: PASS\n"
	)
	inDependabot := func(from, to string) change {
		return change{file: dependabotFile, from: from, to: to}
	}
	inHooks := func(from, to string) change {
		return change{file: hooksFile, from: from, to: to}
	}
	tests := []struct {
		name       string
		changes    []change
		exceptions map[string]string
		wantExit   int
		wantStdout []string
	}{
		{
			name: "every module with a gomod entry and a tidy hook", wantExit: exitPassed,
			wantStdout: []string{passed},
		},
		{
			name:     "an entry with directories",
			changes:  []change{inDependabot(toolsDir, "    directories: [/tools]\n")},
			wantExit: exitPassed,
		},
		{
			name: "an entry whose directories name two modules",
			changes: []change{
				{file: "consumer/go.mod", to: goMod},
				inDependabot(toolsDir, toolsAndMore),
				inHooks(otherHook, consumerHook+otherHook),
			},
			wantExit:   exitPassed,
			wantStdout: []string{"cicheck: checked 3 Go modules and 3 Dependabot entries\n"},
		},
		{
			name: "modules in testdata directories",
			changes: []change{
				{file: "testdata/go.mod", to: goMod},
				{file: "tools/cmd/x/testdata/mod/go.mod", to: goMod},
				{file: "tools/go.mod.orig", to: goMod},
			},
			wantExit:   exitPassed,
			wantStdout: []string{passed},
		},
		{
			name: "a gomod entry for another branch, for a directory without a module",
			changes: []change{
				inDependabot(actionsEntry, actionsEntry+oldEntry+otherBranch),
			},
			wantExit: exitPassed,
		},
		{
			name:     "a hook whose entry is go -C alone",
			changes:  []change{inHooks("entry: go vet ./...", "entry: go -C")},
			wantExit: exitPassed,
		},
		{
			name:       "a module in moduleExceptions without an entry or a tidy hook",
			changes:    []change{{file: "tools/upgrade/go.mod", to: goMod}},
			exceptions: map[string]string{"tools/upgrade": upgradeReason},
			wantExit:   exitPassed,
			wantStdout: []string{"cicheck: checked 3 Go modules and 3 Dependabot entries\n"},
		},
		{
			name: "a removed cooldown",
			changes: []change{
				inDependabot(toolsEntry, strings.Split(toolsEntry, "    cooldown")[0]),
			},
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: .github/dependabot.yml:10: the gomod entry for /tools has no " +
					"cooldown.default-days; set it to at least 7, " + cooldownHint + "7 days old\n",
				"cicheck: .github/dependabot.yml:10: the gomod entry for /tools has no " +
					"cooldown.semver-major-days; set it to at least 14, " + cooldownHint +
					"14 days old\n",
				"cicheck: rules that do not hold: 2\n",
			},
		},
		{
			name: "a github-actions entry without a cooldown",
			changes: []change{
				inDependabot(actionsEntry, strings.Split(actionsEntry, "    cooldown")[0]),
			},
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: .github/dependabot.yml:17: the github-actions entry for / has no " +
					"cooldown.default-days; set it to at least 7",
				"cicheck: rules that do not hold: 1\n",
			},
		},
		{
			name: "a cooldown of 6 days",
			changes: []change{
				inDependabot(actionsEntry, strings.Replace(actionsEntry, "days: 7", "days: 6", 1)),
			},
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: .github/dependabot.yml:17: the github-actions entry for / has " +
					"cooldown.default-days: 6; set it to at least 7, " + cooldownHint +
					"7 days old\n",
			},
		},
		{
			name:     "a cooldown that is not a number",
			changes:  []change{inDependabot("default-days: 7\n", "default-days: seven\n")},
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: .github/dependabot.yml:3: the gomod entry for / has " +
					"cooldown.default-days: seven; set it to at least 7",
			},
		},
		{
			name:     "a major-version cooldown of 13 days",
			changes:  []change{inDependabot("semver-major-days: 14", "semver-major-days: 13")},
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: .github/dependabot.yml:3: the gomod entry for / has " +
					"cooldown.semver-major-days: 13; set it to at least 14, " + cooldownHint +
					"14 days old\n",
			},
		},
		{
			name:     "a directory with a trailing slash",
			changes:  []change{inDependabot(toolsDir, "    directory: /tools/\n")},
			wantExit: exitPassed,
		},
		{
			name: "minor-, patch- and major-version cooldowns shorter than the rule",
			changes: []change{
				inDependabot(toolsEntry, toolsEntry+"      semver-patch-days: 0\n"),
				inDependabot(actionsEntry,
					actionsEntry+"      semver-minor-days: 3\n      semver-major-days: 3\n"),
			},
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: .github/dependabot.yml:10: the gomod entry for /tools has " +
					"cooldown.semver-patch-days: 0; set it to at least 7, " + cooldownHint +
					"7 days old\n",
				"cicheck: .github/dependabot.yml:18: the github-actions entry for / has " +
					"cooldown.semver-major-days: 3; set it to at least 14, " + cooldownHint +
					"14 days old\n",
				"cicheck: .github/dependabot.yml:18: the github-actions entry for / has " +
					"cooldown.semver-minor-days: 3; set it to at least 7",
				"cicheck: rules that do not hold: 3\n",
			},
		},
		{
			name: "cooldown include and exclude lists",
			changes: []change{
				inDependabot(toolsEntry, toolsEntry+"      exclude: [\"*\"]\n"),
				inDependabot(actionsEntry, actionsEntry+"      include: [actions/checkout]\n"),
			},
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: .github/dependabot.yml:10: the gomod entry for /tools has " +
					"cooldown.exclude, which takes dependencies out of the cooldown or limits it " +
					"to some; remove it, so every dependency waits\n",
				"cicheck: .github/dependabot.yml:18: the github-actions entry for / has " +
					"cooldown.include, which",
				"cicheck: rules that do not hold: 2\n",
			},
		},
		{
			name: "a gomod entry for another branch without a cooldown",
			changes: []change{
				inDependabot(actionsEntry,
					actionsEntry+strings.Split(oldEntry, "    cooldown")[0]+otherBranch),
			},
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: .github/dependabot.yml:23: the gomod entry for /old has no " +
					"cooldown.default-days; set it to at least 7",
				"cicheck: rules that do not hold: 2\n",
			},
		},
		{
			name: "a go.mod without an entry",
			changes: []change{
				{file: "consumer/go.mod", to: goMod},
				inHooks(otherHook, consumerHook+otherHook),
			},
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: consumer/go.mod: " + noEntry + "directory /consumer, so Dependabot " +
					"never updates this module; add one, or add the module to " + excepted +
					"with the reason it gets no updates\n",
				"cicheck: rules that do not hold: 1\n",
			},
		},
		{
			name: "a go.mod without a tidy hook",
			changes: []change{
				{file: "consumer/go.mod", to: goMod},
				inDependabot(toolsDir, toolsAndMore),
			},
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: consumer/go.mod: " + noHook + "go -C consumer mod tidy -diff" + notTidy,
				"cicheck: rules that do not hold: 1\n",
			},
		},
		{
			name:       "the root without a tidy hook",
			changes:    []change{inHooks("entry: go mod tidy -diff", "entry: go mod tidy")},
			wantExit:   exitFailed,
			wantStdout: []string{"cicheck: go.mod: " + noHook + "go mod tidy -diff" + notTidy},
		},
		{
			name:       "a tidy hook without -diff",
			changes:    []change{inHooks(toolsHook, "entry: go -C tools mod tidy")},
			wantExit:   exitFailed,
			wantStdout: []string{toolsNoHook},
		},
		{
			name:       "a tidy hook that names the directory another way",
			changes:    []change{inHooks(toolsHook, "entry: go -C ./tools mod tidy -diff")},
			wantExit:   exitFailed,
			wantStdout: []string{toolsNoHook},
		},
		{
			name:       "a tidy hook of another command",
			changes:    []change{inHooks(toolsHook, "entry: gofmt -C tools mod tidy -diff")},
			wantExit:   exitFailed,
			wantStdout: []string{toolsNoHook},
		},
		{
			name: "the only root entry for another branch",
			changes: []change{
				inDependabot("    directory: /\n", "    directory: /\n"+otherBranch),
			},
			wantExit:   exitFailed,
			wantStdout: []string{"cicheck: go.mod: " + noEntry + "directory /, so"},
		},
		{
			name:     "a gomod entry for a directory without a go.mod",
			changes:  []change{inDependabot(actionsEntry, actionsEntry+oldEntry)},
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: .github/dependabot.yml:23: the gomod entry for /old names a directory " +
					"without a go.mod that git tracks; remove it\n",
				"cicheck: checked 2 Go modules and 4 Dependabot entries\n",
			},
		},
		{
			name:       "a module in moduleExceptions without a reason",
			changes:    []change{{file: "tools/upgrade/go.mod", to: goMod}},
			exceptions: map[string]string{"tools/upgrade": " "},
			wantExit:   exitFailed,
			wantStdout: []string{
				"cicheck: tools/upgrade/go.mod: " + excepted + "lists this module without a " +
					"reason; give the reason it gets no Dependabot updates\n",
				"cicheck: rules that do not hold: 1\n",
			},
		},
		{
			name:       "an exception for a directory without a go.mod",
			exceptions: map[string]string{"tools/upgrade": upgradeReason},
			wantExit:   exitFailed,
			wantStdout: []string{
				"cicheck: tools/upgrade/go.mod: " + excepted + "lists tools/upgrade, but git " +
					"tracks no go.mod there; remove the exception\n",
				"cicheck: rules that do not hold: 1\n",
			},
		},
		{
			name:       "an exception without a reason for a directory without a go.mod",
			exceptions: map[string]string{"tools/upgrade": ""},
			wantExit:   exitFailed,
			wantStdout: []string{
				"cicheck: tools/upgrade/go.mod: " + excepted + "lists tools/upgrade, but git " +
					"tracks no go.mod there; remove the exception\n",
				"cicheck: rules that do not hold: 1\n",
			},
		},
		{
			name:       "a module in moduleExceptions with a gomod entry",
			exceptions: map[string]string{"tools": "the tools are pinned by hand"},
			wantExit:   exitFailed,
			wantStdout: []string{
				"cicheck: .github/dependabot.yml:10: the gomod entry for /tools updates a " +
					"module that " + excepted + "keeps out of Dependabot; remove the entry or " +
					"the exception\n",
				"cicheck: rules that do not hold: 1\n",
			},
		},
		{
			name:     "no Dependabot configuration",
			changes:  []change{{file: dependabotFile, remove: true}},
			wantExit: exitDidNotRun,
			wantStdout: []string{
				filepath.FromSlash(dependabotFile) + ": no such file or directory\n",
			},
		},
		{
			name:     "a Dependabot configuration that is not YAML",
			changes:  []change{{file: dependabotFile, to: "updates: [\n"}},
			wantExit: exitDidNotRun,
			wantStdout: []string{
				"cicheck: DID NOT RUN: .github/dependabot.yml: yaml: line ",
			},
		},
		{
			name:     "an empty Dependabot configuration",
			changes:  []change{{file: dependabotFile, to: ""}},
			wantExit: exitDidNotRun,
			wantStdout: []string{
				"cicheck: DID NOT RUN: .github/dependabot.yml: its top level is not a mapping\n",
			},
		},
		{
			name:     "a Dependabot configuration whose top level is a list",
			changes:  []change{{file: dependabotFile, to: "- version: 2\n"}},
			wantExit: exitDidNotRun,
			wantStdout: []string{
				"cicheck: DID NOT RUN: .github/dependabot.yml: its top level is not a mapping\n",
			},
		},
		{
			name:     "no hook configuration",
			changes:  []change{{file: hooksFile, remove: true}},
			wantExit: exitDidNotRun,
			wantStdout: []string{
				filepath.FromSlash(hooksFile) + ": no such file or directory\n",
			},
		},
		{
			name:     "a hook configuration that is not YAML",
			changes:  []change{{file: hooksFile, to: "repos: [\n"}},
			wantExit: exitDidNotRun,
			wantStdout: []string{
				"cicheck: DID NOT RUN: .pre-commit-config.yaml: yaml: line ",
			},
		},
		{
			name:     "no go.mod at the repository root",
			changes:  []change{{file: "go.mod", remove: true}},
			wantExit: exitDidNotRun,
			wantStdout: []string{
				"cicheck: DID NOT RUN: git tracks no go.mod at the repository root; add it with " +
					"git add go.mod\n",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setExceptions(t, tt.exceptions)
			workflows := writeRepository(t, withChanges(t, tt.changes))

			exit, stdout, stderr := runCicheck(t, "-workflows", workflows)

			if exit != tt.wantExit {
				t.Errorf("exit = %d, want %d; stdout:\n%s", exit, tt.wantExit, stdout)
			}
			for _, want := range tt.wantStdout {
				if !strings.Contains(stdout, want) {
					t.Errorf("stdout does not contain %q:\n%s", want, stdout)
				}
			}
			if stderr != "" {
				t.Errorf("stderr = %q, want nothing", stderr)
			}
			if !strings.HasSuffix(stdout, verdicts[tt.wantExit]) {
				t.Errorf("stdout does not end with %q:\n%s", verdicts[tt.wantExit], stdout)
			}
		})
	}
}

// TestCheckUntrackedModules checks that the module rules read the go.mod files git tracks, not
// the working tree: a module that is not committed, such as one in another worktree, needs no
// entry.
func TestCheckUntrackedModules(t *testing.T) {
	workflows := writeRepository(t, validFiles())
	writeFiles(t, filepath.Dir(filepath.Dir(workflows)), map[string]string{
		"consumer/go.mod": "module example.com/consumer\n",
	})

	exit, stdout, _ := runCicheck(t, "-workflows", workflows)

	if exit != exitPassed || !strings.Contains(stdout, "checked 2 Go modules") {
		t.Errorf("exit = %d, stdout:\n%s\nwant exit 0 with 2 Go modules checked", exit, stdout)
	}
}
