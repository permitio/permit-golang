package main

import (
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/permitio/permit-golang/v2/tools/internal/execfile"
)

// fixtureSDK returns the files of a v2 module that diff mode compares.
func fixtureSDK() map[string]string {
	return map[string]string{
		"go.mod": "module example.com/sdk/v2\n\ngo 1.26.0\n",
		"doc.go": "// Package sdk is a fixture.\npackage sdk\n\n// F does nothing.\n" +
			"func F(a int) {}\n",
		"models/models.go": "// Package models is a fixture.\npackage models\n\n" +
			"// User is a user.\ntype User struct{ Key string }\n",
		"internal/x/x.go": "// Package x is a fixture.\npackage x\n\n// I does nothing.\n" +
			"func I() {}\n",
		rulesFile: "# The rules.\nconfig.api-key.missing give an API key\n",
		idsFile:   "# IDs.\nM-module-1\n",
	}
}

// modelMapYAML is a model map in which models.User has the fields key, required, and email,
// optional.
const modelMapYAML = "# The map.\n" +
	"models:\n" +
	"  User:\n" +
	"    schema: UserRead\n" +
	"    fields:\n" +
	"      key: {required: true, null_policy: rejects}\n" +
	"      email: {required: false, null_policy: accepts}\n"

// diffCase is a change to the working tree of a repository whose base is fixtureSDK with base.
type diffCase struct {
	// base is added to fixtureSDK in the base commit; an empty content removes a file.
	base map[string]string
	// head is written into the working tree; an empty content removes a file.
	head  map[string]string
	label bool
}

// runDiffCase commits fixtureSDK with tc.base, applies tc.head to the working tree and runs diff
// mode with the apidiff binary.
func runDiffCase(t *testing.T, apidiff string, tc diffCase) (int, string) {
	t.Helper()
	files := fixtureSDK()
	maps.Copy(files, tc.base)
	maps.DeleteFunc(files, func(_, content string) bool { return content == "" })
	dir := newRepo(t, files)
	base := gitIn(t, dir, "rev-parse", "HEAD")
	writeFiles(t, dir, tc.head)
	args := []string{"-mode", "diff", "-root", dir, "-base", base, "-apidiff", apidiff}
	if tc.label {
		args = append(args, "-breaking-label")
	}
	return runTool(t, system{}, args...)
}

func TestDiff(t *testing.T) {
	t.Parallel()
	apidiff := buildTool(t, "golang.org/x/exp/cmd/apidiff")
	changedSignature := map[string]string{
		"doc.go": strings.Replace(fixtureSDK()["doc.go"], "F(a int)", "F(a string)", 1),
	}
	for name, tc := range map[string]struct {
		diffCase
		code int
		want []string
	}{
		"no change": {
			code: exitPassed,
			want: []string{
				"no tools/drift/model-map.yaml yet: no field can turn required\n",
				"PASS: no breaking change since ",
			},
		},
		"a changed signature without the label": {
			diffCase: diffCase{head: changedSignature},
			code:     exitFailed,
			want: []string{
				"  apidiff: F: changed from func(int) to func(string)\n",
				"- 1 breaking change, and the pull request does not have the breaking label: " +
					"add the label when the change is intended, and describe it in CHANGELOG.md\n",
				"FAIL: 1 problem\n",
			},
		},
		"a changed signature with the label": {
			diffCase: diffCase{head: changedSignature, label: true},
			code:     exitPassed,
			want: []string{
				"  apidiff: F: changed from func(int) to func(string)\n",
				"PASS: 1 breaking change, and the pull request has the breaking label\n",
			},
		},
		"a removed models type": {
			diffCase: diffCase{head: map[string]string{
				"models/models.go": "// Package models is a fixture.\npackage models\n",
			}},
			code: exitFailed,
			want: []string{"  apidiff: ./models.User: removed\n"},
		},
		"compatible additions": {
			diffCase: diffCase{head: map[string]string{
				"doc.go": fixtureSDK()["doc.go"] + "\n// G does nothing.\nfunc G() {}\n",
				"models/models.go": strings.Replace(fixtureSDK()["models/models.go"],
					"Key string", "Key, Email string", 1),
			}},
			code: exitPassed,
			want: []string{"PASS: no breaking change since "},
		},
		"a change to an internal package": {
			diffCase: diffCase{head: map[string]string{
				"internal/x/x.go": "// Package x is a fixture.\npackage x\n\n// I is changed.\n" +
					"func I(a int) {}\n",
			}},
			code: exitPassed,
			want: []string{"PASS: no breaking change since "},
		},
		"an added validation rule": {
			diffCase: diffCase{head: map[string]string{
				rulesFile: fixtureSDK()[rulesFile] +
					"path.segment.empty a path parameter is empty\n",
			}},
			code: exitFailed,
			want: []string{
				"  testdata/validation-rules.txt: validation rule path.segment.empty: " +
					"added; it rejects input that was accepted\n",
				"- 1 breaking change, and the pull request does not have the breaking label",
			},
		},
		"a removed validation rule": {
			diffCase: diffCase{head: map[string]string{rulesFile: "# No rules.\n"}},
			code:     exitPassed,
			want:     []string{"PASS: no breaking change since "},
		},
		"a field turned required": {
			diffCase: diffCase{
				base: map[string]string{modelMapFile: modelMapYAML},
				head: map[string]string{modelMapFile: strings.Replace(modelMapYAML,
					"email: {required: false", "email: {required: true", 1)},
			},
			code: exitFailed,
			want: []string{"  tools/drift/model-map.yaml: models.User field email: turned " +
				"required\n", "- 1 breaking change, and the pull request does not have the " +
				"breaking label"},
		},
		"a field without a flag turned required": {
			diffCase: diffCase{
				base: map[string]string{modelMapFile: strings.Replace(modelMapYAML,
					"email: {required: false, ", "email: {", 1)},
				head: map[string]string{modelMapFile: strings.Replace(modelMapYAML,
					"email: {required: false", "email: {required: true", 1)},
			},
			code: exitFailed,
			want: []string{"models.User field email: turned required"},
		},
		"every kind of breaking change with the label": {
			diffCase: diffCase{
				base: map[string]string{modelMapFile: modelMapYAML},
				head: map[string]string{
					"doc.go": changedSignature["doc.go"],
					rulesFile: fixtureSDK()[rulesFile] +
						"path.segment.empty a path parameter is empty\n",
					modelMapFile: strings.Replace(modelMapYAML, "email: {required: false",
						"email: {required: true", 1),
				},
				label: true,
			},
			code: exitPassed,
			want: []string{"PASS: 3 breaking changes, and the pull request has the breaking " +
				"label\n"},
		},
		"a new required field and a field made optional": {
			diffCase: diffCase{
				base: map[string]string{modelMapFile: modelMapYAML},
				head: map[string]string{modelMapFile: strings.Replace(modelMapYAML,
					"key: {required: true", "key: {required: false", 1) +
					"  Role:\n    fields:\n      key: {required: true}\n" +
					"      name: {required: true}\n"},
			},
			code: exitPassed,
			want: []string{"PASS: no breaking change since "},
		},
		"a new required field on an existing model": {
			diffCase: diffCase{
				base: map[string]string{modelMapFile: modelMapYAML},
				head: map[string]string{modelMapFile: modelMapYAML +
					"      phone: {required: true}\n"},
			},
			code: exitFailed,
			want: []string{"  tools/drift/model-map.yaml: models.User field phone: added as " +
				"required\n", "- 1 breaking change, and the pull request does not have the " +
				"breaking label"},
		},
		"a new optional field on an existing model": {
			diffCase: diffCase{
				base: map[string]string{modelMapFile: modelMapYAML},
				head: map[string]string{modelMapFile: modelMapYAML +
					"      phone: {required: false}\n"},
			},
			code: exitPassed,
			want: []string{"PASS: no breaking change since "},
		},
		"a model map that is new": {
			diffCase: diffCase{head: map[string]string{modelMapFile: modelMapYAML}},
			code:     exitPassed,
			want: []string{
				"tools/drift/model-map.yaml is new: no field turned required\n",
				"PASS: ",
			},
		},
		"an entry ID dropped by the working tree": {
			diffCase: diffCase{head: map[string]string{idsFile: "# IDs.\n"}},
			code:     exitFailed,
			want: []string{
				"- the working tree removes M-module-1 from tools/migration/ids.txt",
			},
		},
		"a malformed line in ids.txt": {
			diffCase: diffCase{head: map[string]string{idsFile: "# IDs.\nM-module-1\nbad\n"}},
			code:     exitFailed,
			want:     []string{"- tools/migration/ids.txt:3: \"bad\" is not an entry ID"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			code, out := runDiffCase(t, apidiff, tc.diffCase)
			wantRun(t, code, out, tc.code, tc.want...)
		})
	}
}

func TestDiffDidNotRun(t *testing.T) {
	t.Parallel()
	apidiff := buildTool(t, "golang.org/x/exp/cmd/apidiff")
	for name, tc := range map[string]struct {
		diffCase
		want string
	}{
		"a working tree that does not build": {
			diffCase: diffCase{head: map[string]string{"doc.go": "package sdk\n\nfunc F( {}\n"}},
			want:     "DID NOT RUN: apidiff could not compare example.com/sdk/v2 with ",
		},
		"a base that does not build": {
			diffCase: diffCase{base: map[string]string{"doc.go": "package sdk\n\nfunc F( {}\n"}},
			want:     "DID NOT RUN: apidiff could not load example.com/sdk/v2 at ",
		},
		"a base without packages": {
			diffCase: diffCase{base: map[string]string{
				"doc.go": "", "models/models.go": "", "internal/x/x.go": "",
			}},
			want: "DID NOT RUN: apidiff could not load example.com/sdk/v2 at ",
		},
		"a base without validation rules": {
			diffCase: diffCase{base: map[string]string{rulesFile: ""}},
			want:     " has no testdata/validation-rules.txt",
		},
		"a working tree without validation rules": {
			diffCase: diffCase{head: map[string]string{rulesFile: ""}},
			want:     "DID NOT RUN: open ",
		},
		"a rule without a message": {
			diffCase: diffCase{head: map[string]string{rulesFile: "config.api-key.missing\n"}},
			want: "DID NOT RUN: testdata/validation-rules.txt: line 1, " +
				"\"config.api-key.missing\", is not <ID> <message>",
		},
		"a rule ID listed twice": {
			diffCase: diffCase{head: map[string]string{
				rulesFile: fixtureSDK()[rulesFile] + "config.api-key.missing again\n",
			}},
			want: "line 3 repeats the rule ID config.api-key.missing",
		},
		"a base with a malformed rule": {
			diffCase: diffCase{base: map[string]string{rulesFile: "x\n"}},
			want:     ":testdata/validation-rules.txt: line 1, \"x\", is not <ID> <message>",
		},
		"a model map that the working tree removes": {
			diffCase: diffCase{
				base: map[string]string{modelMapFile: modelMapYAML},
				head: map[string]string{modelMapFile: ""},
			},
			want: "DID NOT RUN: tools/drift/model-map.yaml is in ",
		},
		"a model map without models": {
			diffCase: diffCase{
				base: map[string]string{modelMapFile: modelMapYAML},
				head: map[string]string{modelMapFile: "models: {}\n"},
			},
			want: "DID NOT RUN: tools/drift/model-map.yaml: its models key maps no models type",
		},
		"a base model map that is not YAML": {
			diffCase: diffCase{
				base: map[string]string{modelMapFile: "models: [\n"},
				head: map[string]string{modelMapFile: modelMapYAML},
			},
			want: ":tools/drift/model-map.yaml: yaml:",
		},
		"no ids.txt": {
			diffCase: diffCase{head: map[string]string{idsFile: ""}},
			want:     "DID NOT RUN: open ",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			code, out := runDiffCase(t, apidiff, tc.diffCase)
			wantRun(t, code, out, exitDidNotRun, tc.want)
			if got := lastLine(out); !strings.HasPrefix(got, "DID NOT RUN: ") {
				t.Errorf("the last line is %q, want the verdict", got)
			}
		})
	}
	t.Run("an unknown base", func(t *testing.T) {
		t.Parallel()
		dir := newRepo(t, fixtureSDK())
		code, out := runTool(t, system{}, "-mode", "diff", "-root", dir, "-base", "nope",
			"-apidiff", apidiff)
		wantRun(t, code, out, exitDidNotRun, "DID NOT RUN: git archive --format=tar nope:")
	})
	for name, tc := range map[string]struct {
		script, want string
	}{
		"an apidiff that writes no export data": {
			script: "#!/bin/sh\nexit 0\n",
			want:   "DID NOT RUN: apidiff wrote no export data for example.com/sdk/v2 at HEAD",
		},
		"an apidiff that prints a line apisurface does not know": {
			script: "#!/bin/sh\nif [ \"$2\" = -w ]; then echo data >\"$3\"; exit 0; fi\n" +
				"echo 'Breaking changes:'\n",
			want: "DID NOT RUN: apidiff printed a line apisurface does not know: " +
				"\"Breaking changes:\"",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := newRepo(t, fixtureSDK())
			fake := filepath.Join(t.TempDir(), "apidiff")
			if err := execfile.WriteFile(fake, []byte(tc.script), 0o700); err != nil {
				t.Fatal(err)
			}
			code, out := runTool(t, system{}, "-mode", "diff", "-root", dir, "-base", "HEAD",
				"-apidiff", fake)
			wantRun(t, code, out, exitDidNotRun, tc.want)
		})
	}
	t.Run("no apidiff", func(t *testing.T) {
		t.Parallel()
		dir := newRepo(t, fixtureSDK())
		code, out := runTool(t, system{}, "-mode", "diff", "-root", dir, "-base", "HEAD",
			"-apidiff", dir+"/missing")
		wantRun(t, code, out, exitDidNotRun, "DID NOT RUN: apidiff could not load")
	})
}

func TestParseAPIDiff(t *testing.T) {
	got, err := parseAPIDiff([]byte("Ignoring internal package example.com/m/internal/x\n" +
		"Incompatible changes:\n- F: changed from func(int) to func(string)\n" +
		"- ./models.U: removed\nCompatible changes:\n- G: added\n"))
	want := []string{"F: changed from func(int) to func(string)", "./models.U: removed"}
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("parseAPIDiff = %q, %v, want %q", got, err, want)
	}
	got, err = parseAPIDiff([]byte("Compatible changes:\n- G: added\n"))
	if err != nil || len(got) != 0 {
		t.Errorf("parseAPIDiff of compatible changes = %q, %v, want none", got, err)
	}
	got, err = parseAPIDiff(nil)
	if err != nil || len(got) != 0 {
		t.Errorf("parseAPIDiff of no output = %q, %v, want none", got, err)
	}
	for _, out := range []string{
		"- F: changed before any section\n",
		"Incompatible changes:\n* F: changed\n",
		"Incompatible changes:\n\n",
		"Breaking changes:\n- F: changed\n",
	} {
		if _, err := parseAPIDiff([]byte(out)); err == nil ||
			!strings.Contains(err.Error(), "apidiff printed a line apisurface does not know") {
			t.Errorf("parseAPIDiff(%q) error = %v, want an unknown line", out, err)
		}
	}
}

func TestRuleIDs(t *testing.T) {
	ids, err := ruleIDs([]byte("# c\n\na.b one\nc.d two words\n  e.f  indented\n"))
	if want := []string{"a.b", "c.d", "e.f"}; err != nil || !slices.Equal(ids, want) {
		t.Errorf("ruleIDs = %q, %v, want %q", ids, err, want)
	}
	ids, err = ruleIDs([]byte("# no rules\n"))
	if err != nil || len(ids) != 0 {
		t.Errorf("ruleIDs of no rules = %q, %v", ids, err)
	}
	for content, want := range map[string]string{
		"a.b\n":                   `line 1, "a.b", is not <ID> <message>`,
		"a.b one\na.b two\n":      "line 2 repeats the rule ID a.b",
		"a.b one\nc.d\na.b two\n": `line 2, "c.d", is not <ID> <message>`,
	} {
		if _, err := ruleIDs([]byte(content)); err == nil || err.Error() != want {
			t.Errorf("ruleIDs(%q) error = %v, want %q", content, err, want)
		}
	}
}
