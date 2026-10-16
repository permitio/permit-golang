package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The tests below read files of the repository outside this module: MIGRATION.md, SKILL.md,
// tools/migration and tools/upgrade. Their names start with TestRepository, as the mutation tests
// skip such tests, which fail in the copy of the module that each mutant is tested in.

// repositoryRoot is the root of the repository, from the directory of this package.
var repositoryRoot = filepath.Join("..", "..", "..")

// TestRepositoryV1API checks that v1-api.txt holds the lines of blocks a and c of
// tools/migration/v1-surface.txt, in their order, and nothing else.
func TestRepositoryV1API(t *testing.T) {
	t.Parallel()
	surface, err := os.ReadFile(filepath.Join(repositoryRoot, "tools", "migration",
		"v1-surface.txt"))
	if err != nil {
		t.Fatal(err)
	}
	var want []string
	for line := range strings.Lines(string(surface)) {
		if strings.HasPrefix(line, "a ") || strings.HasPrefix(line, "c ") {
			want = append(want, strings.TrimSuffix(line, "\n"))
		}
	}
	var got []string
	for line := range strings.Lines(v1API) {
		line = strings.TrimSuffix(line, "\n")
		if line != "" && !strings.HasPrefix(line, "#") {
			got = append(got, line)
		}
	}
	if len(want) == 0 || !slices.Equal(got, want) {
		t.Errorf("v1-api.txt has %d lines of the v1 API, and blocks a and c of "+
			"tools/migration/v1-surface.txt %d; copy those blocks into v1-api.txt as they are",
			len(got), len(want))
	}
}

// TestRepositoryModelsConsts checks that v1-models-consts.txt lists each exported constant of
// pkg/models, with its type, of the v1 version that tools/upgrade requires, and nothing else. It
// reads that version's source in the module cache.
func TestRepositoryModelsConsts(t *testing.T) {
	t.Parallel()
	cmd := exec.CommandContext(t.Context(), "go", "list", "-m", "-f", "{{.Dir}}", v1Module)
	cmd.Dir = filepath.Join(repositoryRoot, "tools", "upgrade")
	out, err := cmd.Output()
	module := strings.TrimSpace(string(out))
	if err != nil || module == "" {
		t.Fatalf("find the source of the v1 that tools/upgrade requires: %v; run "+
			"go -C tools/upgrade mod download first", err)
	}
	dir := filepath.Join(module, "pkg", "models")
	names, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil || len(names) == 0 {
		t.Fatalf("list the Go files of %s: %v", dir, err)
	}
	var want []string
	for _, name := range names {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.CONST {
				continue
			}
			for _, spec := range gen.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				typ, typed := value.Type.(*ast.Ident)
				for _, constName := range value.Names {
					switch {
					case !constName.IsExported():
					case !typed:
						t.Errorf("%s: the constant %s has no type name, which "+
							"v1-models-consts.txt needs", name, constName.Name)
					default:
						want = append(want, "models."+constName.Name+" models."+typ.Name)
					}
				}
			}
		}
	}
	slices.Sort(want)
	var got []string
	for line := range strings.Lines(v1Consts) {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
			got = append(got, strings.Join(strings.Fields(line), " "))
		}
	}
	if len(want) == 0 || !slices.Equal(got, want) {
		t.Errorf("v1-models-consts.txt differs from the constants of %s; write these lines "+
			"below its comments:\n%s", dir, strings.Join(want, "\n"))
	}
}

// TestRepositoryUpgrade runs the scanner with the repository's MIGRATION.md over tools/upgrade,
// whose test uses v1.3.0 as an application does: the guide must map each use of v1 in it.
func TestRepositoryUpgrade(t *testing.T) {
	t.Parallel()
	code, stdout, stderr := runScanner("-guide", filepath.Join(repositoryRoot, "MIGRATION.md"),
		filepath.Join(repositoryRoot, "tools", "upgrade"))
	var out report
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout is not a report: %v\nstderr:\n%s", err, stderr)
	}
	if code != exitPassed || len(out.Hits) == 0 {
		t.Errorf("exit code %d with %d uses of v1, want %d with at least one; stderr:\n%s", code,
			len(out.Hits), exitPassed, stderr)
	}
}

// TestRepositorySkill checks the entries that SKILL.md links: tools/migration/ids.txt lists
// each, and its SyncUser step links each entry that the guide maps the SyncUser methods to.
func TestRepositorySkill(t *testing.T) {
	t.Parallel()
	skill, err := os.ReadFile(filepath.Join("..", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	text, err := os.ReadFile(filepath.Join(repositoryRoot, "MIGRATION.md"))
	if err != nil {
		t.Fatal(err)
	}
	g, err := parseGuide(string(text))
	if err != nil {
		t.Fatal(err)
	}
	_, listed := guideIDs(t)
	fragment := regexp.MustCompile(`#(M-[A-Za-z0-9-]+)`)
	links := fragment.FindAllStringSubmatch(string(skill), -1)
	if len(links) == 0 {
		t.Fatal("SKILL.md links no entry")
	}
	for _, link := range links {
		if !listed[link[1]] {
			t.Errorf("SKILL.md links %s, which tools/migration/ids.txt does not list", link[1])
		}
	}
	var step string
	for section := range strings.SplitSeq(string(skill), "\n## ") {
		if heading, _, _ := strings.Cut(section, "\n"); strings.Contains(heading, "SyncUser") {
			step = section
		}
	}
	for _, v1 := range []string{"permit.Client.SyncUser", "api.Users.SyncUser"} {
		ids, _ := g.resolve(v1, "")
		if len(ids) == 0 {
			t.Errorf("the guide maps %s to no entry", v1)
		}
		for _, id := range ids {
			if !strings.Contains(step, "#"+id+")") {
				t.Errorf("the SyncUser step of SKILL.md does not link %s, the entry of %s", id, v1)
			}
		}
	}
}

// guideIDs returns the IDs of the entries of MIGRATION.md, from their anchors, and those that
// tools/migration/ids.txt lists.
func guideIDs(t *testing.T) (anchors, listed map[string]bool) {
	t.Helper()
	guide, err := os.ReadFile(filepath.Join(repositoryRoot, "MIGRATION.md"))
	if err != nil {
		t.Fatal(err)
	}
	anchors = map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^<a id="(M-[^"]+)"></a>$`).FindAllSubmatch(guide,
		-1) {
		anchors[string(m[1])] = true
	}
	ids, err := os.ReadFile(filepath.Join(repositoryRoot, "tools", "migration", "ids.txt"))
	if err != nil {
		t.Fatal(err)
	}
	listed = map[string]bool{}
	lines := bufio.NewScanner(bytes.NewReader(ids))
	for lines.Scan() {
		if line := strings.TrimSpace(lines.Text()); line != "" && !strings.HasPrefix(line, "#") {
			listed[line] = true
		}
	}
	if len(anchors) == 0 || len(listed) == 0 {
		t.Fatalf("MIGRATION.md has %d anchors and ids.txt %d IDs", len(anchors), len(listed))
	}
	return anchors, listed
}

// TestRepositoryGuide runs the scanner with the repository's MIGRATION.md on each fixture: it
// must find the uses of v1 that the fixture annotates, map each of them, but the one of
// testdata/unmapped that no entry names, and print only IDs that MIGRATION.md has as anchors and
// tools/migration/ids.txt lists, which never change.
func TestRepositoryGuide(t *testing.T) {
	t.Parallel()
	guide := filepath.Join(repositoryRoot, "MIGRATION.md")
	anchors, listed := guideIDs(t)
	for _, dir := range fixtureDirs {
		t.Run(dir, func(t *testing.T) {
			t.Parallel()
			code, stdout, stderr := runScanner("-guide", guide, dir)
			var out report
			if err := json.Unmarshal([]byte(stdout), &out); err != nil {
				t.Fatalf("stdout is not a report: %v\n%s\nstderr:\n%s", err, stdout, stderr)
			}
			want := annotations(t, os.DirFS(dir), foundMark)
			if len(out.Hits) != len(want) {
				t.Errorf("%d hits, want the %d uses of v1 that the fixture annotates",
					len(out.Hits), len(want))
			}
			var unmapped []string
			for _, h := range out.Hits {
				if !h.Mapped {
					unmapped = append(unmapped, h.V1)
				}
				for _, id := range h.IDs {
					if !anchors[id] || !listed[id] {
						t.Errorf("%s:%d: %s maps to %s, which MIGRATION.md has as an anchor: "+
							"%v, and tools/migration/ids.txt lists: %v", h.File, h.Line, h.V1, id,
							anchors[id], listed[id])
					}
				}
			}
			wantUnmapped, wantCode := []string(nil), exitPassed
			if dir == "testdata/unmapped" {
				wantUnmapped, wantCode = []string{"models.PtrString"}, exitUnmapped
			}
			if !slices.Equal(unmapped, wantUnmapped) || code != wantCode {
				t.Errorf("exit code %d with the uses without an entry %q, want %d with %q; "+
					"stderr:\n%s", code, unmapped, wantCode, wantUnmapped, stderr)
			}
		})
	}
}
