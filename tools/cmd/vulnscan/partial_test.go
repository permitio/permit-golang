package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// staleEntry accepts a vulnerability that no tree reports, which is stale only when every tree
// was scanned.
const staleEntry = "GO-2025-3503 golang.org/x/net fixed in a later commit 2026-12-31"

func TestOnlyOneTree(t *testing.T) {
	golist := []string{"list", "-deps", "-f", "{{with .Module}}{{.Path}}{{end}}", "./..."}
	scan := []string{"-mode", "source", "-scan", "symbol", "-format", "json", "./..."}
	tests := []struct {
		tree, dir string
		args      []string
		patterns  []string
	}{
		{tree: "floor", dir: ".", args: []string{"-only", "floor"}},
		{tree: "ceiling", dir: ceiling, args: []string{"-only", "ceiling", "-ceiling", ceiling}},
		{tree: "tools", dir: "tools", args: []string{"-only", "tools"}, patterns: []string{"tool"}},
	}
	for _, tt := range tests {
		t.Run(tt.tree, func(t *testing.T) {
			govulncheck := repository(t, []string{staleEntry}, nil)
			f := newFake(t)

			code, out := runWith(t, f, tt.args...)

			assertOutput(t, code, exitPassed, out,
				"vulnscan: "+tt.tree+": ",
				"\nvulnscan: accept entries not checked for staleness: only the "+tt.tree+
					" tree was scanned\n",
				"0 expired and 0 stale accept entries",
				"; 1 of 1 trees scanned\n")
			want := []call{
				{
					dir:  ".",
					name: "git",
					args: []string{"ls-files", "-z", "--", "go.mod", "*/go.mod"},
				},
				{dir: tt.dir, name: "go", args: append(slices.Clone(golist), tt.patterns...)},
				{dir: tt.dir, name: govulncheck, args: append(slices.Clone(scan), tt.patterns...)},
			}
			if !slices.EqualFunc(f.calls, want, func(a, b call) bool {
				return a.dir == b.dir && a.name == b.name && slices.Equal(a.args, b.args)
			}) {
				t.Errorf("commands:\n%q\nwant:\n%q", f.calls, want)
			}
		})
	}
}

func TestConsumer(t *testing.T) {
	const consumer = "consumer"
	govulncheck := repository(t, []string{staleEntry}, nil)
	writeFile(t, filepath.Join(consumer, "go.mod"), "module example.com/consumer\n")
	if err := os.Remove(filepath.Join(".github", "vulnscan-trees.txt")); err != nil {
		t.Fatal(err)
	}
	f := newFake(t)
	f.goList[consumer] = stdout([]byte(plantModule))
	f.scans[consumer] = stdout(withoutFinding(t, "floor-xnet.jsonl", "GO-2024-3333"))

	code, out := runWith(t, f, "-consumer", consumer)

	assertOutput(t, code, exitFailed, out,
		"vulnscan: consumer: 3 modules, among them the 2 that go list -deps ./... lists;",
		"\nGO-2026-6603  FAIL, required, not imported, not accepted\n"+
			"    golang.org/x/net in consumer v0.23.0, fixed in v0.60.0\n",
		"\nGO-2025-3503  accepted, required, not imported\n",
		"\nvulnscan: accept entries not checked for staleness: only the consumer module was "+
			"scanned\n",
		"1 to fix, 1 accepted, 0 expired and 0 stale accept entries",
		"; 1 of 1 trees scanned\n")
	if len(f.calls) != 2 || f.calls[0].name != "go" || f.calls[1].name != govulncheck ||
		f.calls[0].dir != consumer || f.calls[1].dir != consumer {
		t.Errorf("commands: %q; want go list and govulncheck in %s, and no git", f.calls,
			consumer)
	}
}

func TestFindingsFile(t *testing.T) {
	repository(t, nil, nil)
	f := newFake(t).withPlant(t)
	f.scans["tools"] = stdout(append(fixture(t, "tools-clean.jsonl"),
		`{"finding":{"osv":"GO-2024-3333","fixed_version":"v0.34.0","trace":[{"module":`+
			`"golang.org/x/net","version":"v0.24.0"}]}}`+"\n"...))
	findings := filepath.Join(t.TempDir(), "findings.jsonl")

	code, _ := runWith(t, f, "-ceiling", ceiling, "-findings", findings)

	if code != exitFailed {
		t.Errorf("exit %d, want %d", code, exitFailed)
	}
	data, err := os.ReadFile(findings)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		`{"id":"GO-2024-3333","package":"golang.org/x/net","version":"v0.23.0, v0.24.0",` +
			`"fix":"v0.33.0, v0.34.0","url":"https://pkg.go.dev/vuln/GO-2024-3333"}`,
		`{"id":"GO-2025-3503","package":"golang.org/x/net","version":"v0.23.0","fix":"v0.36.0",` +
			`"url":"https://pkg.go.dev/vuln/GO-2025-3503"}`,
		`{"id":"GO-2026-6603","package":"golang.org/x/net","version":"v0.23.0","fix":"v0.60.0",` +
			`"url":"https://pkg.go.dev/vuln/GO-2026-6603"}`,
		`{"id":"GO-2026-6603","package":"stdlib","version":"go1.26.5","fix":"go1.26.9",` +
			`"url":"https://pkg.go.dev/vuln/GO-2026-6603"}`,
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	for _, w := range want {
		if !slices.Contains(lines, w) {
			t.Errorf("the findings file does not hold the line\n%s\nit holds:\n%s", w, data)
		}
	}
	if !slices.IsSorted(lines) || len(lines) < len(want)+1 {
		t.Errorf("the findings file is not one sorted line for each vulnerability:\n%s", data)
	}

	t.Run("accepted vulnerabilities", func(t *testing.T) {
		const reason = "no package imports the vulnerable code"
		repository(t, []string{
			"GO-2025-3503 golang.org/x/net " + reason + " 2026-12-31",
			// An expired entry and one for called code accept nothing.
			"GO-2026-6603 golang.org/x/net " + reason + " 2026-10-12",
			"GO-2024-3333 golang.org/x/net " + reason + " 2026-12-31",
		}, nil)
		findings := filepath.Join(t.TempDir(), "findings.jsonl")

		code, _ := runWith(t, f, "-ceiling", ceiling, "-findings", findings)

		data, err := os.ReadFile(findings)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
		for _, w := range []string{
			`{"id":"GO-2025-3503","package":"golang.org/x/net","version":"v0.23.0",` +
				`"fix":"accepted until 2026-12-31: ` + reason + `",` +
				`"url":"https://pkg.go.dev/vuln/GO-2025-3503"}`,
			want[0], want[2],
		} {
			if !slices.Contains(lines, w) {
				t.Errorf("exit %d; the findings file does not hold the line\n%s\nit holds:\n%s",
					code, w, data)
			}
		}
	})

	t.Run("a scan without findings", func(t *testing.T) {
		repository(t, nil, nil)
		findings := filepath.Join(t.TempDir(), "findings.jsonl")
		noFindings := func(name string) result {
			return stdout(rewrite(t, fixture(t, name), func(message map[string]any) bool {
				return message["finding"] == nil
			}))
		}
		f := newFake(t)
		f.scans["."] = noFindings("floor-clean.jsonl")
		f.scans[ceiling] = noFindings("floor-clean.jsonl")
		f.scans["tools"] = noFindings("tools-clean.jsonl")

		code, _ := runWith(t, f, "-ceiling", ceiling, "-findings", findings)

		data, err := os.ReadFile(findings)
		if code != exitPassed || err != nil || len(data) != 0 {
			t.Errorf("exit %d, findings file %q (%v); want exit 0 and an empty file", code,
				data, err)
		}
	})

	t.Run("a file that cannot be written", func(t *testing.T) {
		repository(t, nil, nil)

		code, out := runWith(t, newFake(t), "-ceiling", ceiling, "-findings", t.TempDir())

		assertOutput(t, code, exitDidNotRun, out,
			"vulnscan: PASS\nvulnscan: DID NOT RUN: writing the findings: open ")
	})
}

func TestPartialArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "an unknown tree", args: []string{"-only", "examples"},
			want: `-only "examples" is not floor, ceiling or tools`,
		},
		{
			name: "the ceiling without -ceiling", args: []string{"-only", "ceiling"},
			want: "-ceiling is missing",
		},
		{
			name: "-ceiling for a tree that does not use it",
			args: []string{"-only", "tools", "-ceiling", ceiling},
			want: "-ceiling is set, but -only tools does not scan it",
		},
		{
			name: "-consumer with -only", args: []string{"-consumer", "c", "-only", "floor"},
			want: "-consumer scans one module; drop -only and -ceiling",
		},
		{
			name: "-consumer with -ceiling", args: []string{"-consumer", "c", "-ceiling", ceiling},
			want: "-consumer scans one module; drop -only and -ceiling",
		},
		{
			name: "a consumer without go.mod", args: []string{"-consumer", "bin"},
			want: "-consumer bin has no go.mod",
		},
		{
			name: "a ceiling that is a file", args: []string{"-only", "ceiling", "-ceiling", "x"},
			want: "-ceiling x has no go.mod",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository(t, nil, nil)
			writeFile(t, filepath.Join("x", "go.mod", "y"), "a directory named go.mod\n")
			f := newFake(t)

			code, out := runWith(t, f, tt.args...)

			assertOutput(t, code, exitDidNotRun, out, "vulnscan: DID NOT RUN: ", tt.want)
			if len(f.calls) != 0 {
				t.Errorf("vulnscan ran %d commands, want none", len(f.calls))
			}
		})
	}
}

// writeFile writes content to name, a path relative to the working directory, and creates its
// directory.
func writeFile(t *testing.T, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
