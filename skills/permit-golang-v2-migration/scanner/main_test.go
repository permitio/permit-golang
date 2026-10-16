package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// The import paths of the v1 packages that the tests import.
const (
	models    = "github.com/permitio/permit-golang/pkg/models"
	permitPkg = "github.com/permitio/permit-golang/pkg/permit"
)

// writeFiles writes each file of files, a map from a slash path to its content, under dir.
func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// runScanner runs the command with args and returns its exit code, standard output and standard
// error.
func runScanner(args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := run(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestRun(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"mapped/main.go": `package main

import (
	"context"

	"github.com/permitio/permit-golang/pkg/models"
	"github.com/permitio/permit-golang/pkg/permit"
)

func main() {
	client := permit.New(nil)
	client.SyncUser(context.Background(), *models.NewUserCreate("alice"))
}
`,
		"unmapped/a/main.go": `package main

import "github.com/permitio/permit-golang/pkg/permit"

func main() {
	client := permit.New(nil)
	client.Check(nil, "read", nil)
}
`,
		"unmapped/b.go": "package b\n",
		"models/main.go": `package main

import "github.com/permitio/permit-golang/pkg/models"

func main() {
	user := models.NewUserCreate("alice")
	user.SetEmail("alice@example.com")
	_ = models.ResourceId{}.Get()
	_ = models.STRING
	_ = models.GET
}
`,
		"none/main.go": "package main\n\nimport \"github.com/permitio/permit-golang/v2\"\n\n" +
			"var _ = permit.New\n",
		"empty/README": "no Go files\n",
	})
	guide, err := filepath.Abs("testdata/guide.md")
	if err != nil {
		t.Fatal(err)
	}
	module := []string{"M-module-1"}
	for name, test := range map[string]struct {
		dir        string
		code       int
		hits       []hit
		unmapped   int
		lastStderr string
	}{
		"every use has an entry": {
			dir:  "mapped",
			code: exitPassed,
			hits: []hit{
				{"main.go", 6, 2, kindImport, models, true, module, ""},
				{"main.go", 7, 2, kindImport, permitPkg, true, module, ""},
				{"main.go", 11, 19, kindIdentifier, "permit.New", true, []string{"M-client-1"}, ""},
				{
					"main.go", 12, 9, kindIdentifier, "permit.Client.SyncUser", true,
					[]string{"M-users-1"},
					"",
				},
				{
					"main.go", 12, 48, kindIdentifier, "models.NewUserCreate", true,
					[]string{},
					"models.UserCreate",
				},
			},
			lastStderr: "PASS: 5 uses of v1, each with an entry of the guide or a v2 type",
		},
		"a use without an entry": {
			dir:  "unmapped",
			code: exitUnmapped,
			hits: []hit{
				{"a/main.go", 3, 8, kindImport, permitPkg, true, module, ""},
				{
					"a/main.go", 6, 19, kindIdentifier, "permit.New", true,
					[]string{"M-client-1"},
					"",
				},
				{"a/main.go", 7, 9, kindIdentifier, "permit.Client.Check", false, []string{}, ""},
			},
			unmapped: 1,
			lastStderr: "FAIL: 1 use of v1 without an entry in the guide, of 3: " +
				"permit.Client.Check",
		},
		"the uses of models resolve through their types": {
			dir:  "models",
			code: exitUnmapped,
			hits: []hit{
				{"main.go", 3, 8, kindImport, models, true, module, ""},
				{
					"main.go", 6, 17, kindIdentifier, "models.NewUserCreate", true,
					[]string{},
					"models.UserCreate",
				},
				{
					"main.go", 7, 7, kindIdentifier, "models.UserCreate.SetEmail", true,
					[]string{},
					"models.UserCreate",
				},
				{
					"main.go", 8, 13, kindIdentifier, "models.ResourceId", true,
					[]string{"M-models-1"},
					"",
				},
				{
					"main.go", 8, 26, kindIdentifier, "models.ResourceId.Get", true,
					[]string{"M-models-1"},
					"",
				},
				{
					"main.go", 9, 13, kindIdentifier, "models.STRING", true,
					[]string{},
					"models.AttributeType",
				},
				// The guide has no row of models.Methods.
				{"main.go", 10, 13, kindIdentifier, "models.GET", false, []string{}, ""},
			},
			unmapped:   1,
			lastStderr: "FAIL: 1 use of v1 without an entry in the guide, of 7: models.GET",
		},
		"no use of v1": {
			dir:        "none",
			code:       exitPassed,
			hits:       []hit{},
			lastStderr: "PASS: no use of v1",
		},
		"no Go file": {
			dir:        "empty",
			code:       exitPassed,
			hits:       []hit{},
			lastStderr: "PASS: no use of v1",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			code, stdout, stderr := runScanner("-guide", guide, filepath.Join(dir, test.dir))
			if code != test.code {
				t.Errorf("exit code %d, want %d; stderr:\n%s", code, test.code, stderr)
			}
			var got report
			if err := json.Unmarshal([]byte(stdout), &got); err != nil {
				t.Fatalf("stdout is not a report: %v\n%s", err, stdout)
			}
			want := report{Hits: test.hits, Unmapped: test.unmapped}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("report:\n%+v\nwant:\n%+v", got, want)
			}
			if last := lastLine(stderr); last != test.lastStderr {
				t.Errorf("the last line of stderr is %q, want %q", last, test.lastStderr)
			}
		})
	}
}

// TestRunOutput checks the text of the report: indented JSON with each field named, an empty
// list of IDs as [], and no v2 when there is none.
func TestRunOutput(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"main.go": "package main\n\nimport " +
		"\"github.com/permitio/permit-golang/pkg/permit\"\n\nvar _ permit.Client\n"})
	_, stdout, _ := runScanner("-guide", "testdata/guide.md", dir)
	want := "{\n" +
		"  \"hits\": [\n" +
		"    {\n" +
		"      \"file\": \"main.go\",\n" +
		"      \"line\": 3,\n" +
		"      \"column\": 8,\n" +
		"      \"kind\": \"import\",\n" +
		"      \"v1\": \"github.com/permitio/permit-golang/pkg/permit\",\n" +
		"      \"mapped\": true,\n" +
		"      \"ids\": [\n" +
		"        \"M-module-1\"\n" +
		"      ]\n" +
		"    },\n" +
		"    {\n" +
		"      \"file\": \"main.go\",\n" +
		"      \"line\": 5,\n" +
		"      \"column\": 14,\n" +
		"      \"kind\": \"identifier\",\n" +
		"      \"v1\": \"permit.Client\",\n" +
		"      \"mapped\": false,\n" +
		"      \"ids\": []\n" +
		"    }\n" +
		"  ],\n" +
		"  \"unmapped\": 1\n" +
		"}\n"
	if stdout != want {
		t.Errorf("stdout:\n%s\nwant:\n%s", stdout, want)
	}
}

func TestRunUnmappedCount(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"main.go": `package main

import "github.com/permitio/permit-golang/pkg/models"

var _ = models.PtrString("a")
var _ = models.PtrBool(true)
var _ = models.PtrString("b")
`})
	code, stdout, stderr := runScanner("-guide", "testdata/guide.md", dir)
	if code != exitUnmapped {
		t.Errorf("exit code %d, want %d", code, exitUnmapped)
	}
	if !strings.Contains(stdout, "\"unmapped\": 3\n") {
		t.Errorf("stdout does not count 3 uses without an entry:\n%s", stdout)
	}
	// The FAIL line names each identifier once.
	want := "FAIL: 3 uses of v1 without an entry in the guide, of 4: models.PtrBool, " +
		"models.PtrString"
	if last := lastLine(stderr); last != want {
		t.Errorf("the last line of stderr is %q, want %q", last, want)
	}
}

func TestRunCurrentDirectory(t *testing.T) {
	guide, err := filepath.Abs("testdata/guide.md")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"sub/main.go": "package main\n\nimport _ " +
		"\"github.com/permitio/permit-golang/pkg/permit\"\n"})
	t.Chdir(dir)
	code, stdout, stderr := runScanner("-guide", guide)
	if code != exitPassed {
		t.Errorf("exit code %d, want %d; stderr:\n%s", code, exitPassed, stderr)
	}
	if !strings.Contains(stdout, "\"file\": \"sub/main.go\"") {
		t.Errorf("stdout does not name sub/main.go:\n%s", stdout)
	}
}

func TestRunDidNotRun(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"code/main.go":   "package main\n",
		"broken/main.go": "package main\n\nfunc main() {\n",
		"guide.md":       "# Migrating from v1 to v2\n\nNo entries.\n",
	})
	code := filepath.Join(dir, "code")
	for name, test := range map[string]struct {
		args []string
		want string
	}{
		"no -guide": {
			args: []string{code},
			want: "DID NOT RUN: -guide is missing; pass the path of MIGRATION.md of v2",
		},
		"two directories": {
			args: []string{"-guide", "testdata/guide.md", code, code},
			want: "DID NOT RUN: scanner takes one directory, and was given 2",
		},
		"an unknown flag": {
			args: []string{"-guide", "testdata/guide.md", "-x", code},
			want: "DID NOT RUN: flag provided but not defined: -x",
		},
		"a guide that cannot be read": {
			args: []string{"-guide", filepath.Join(dir, "missing.md"), code},
			want: "DID NOT RUN: read the guide: open " + filepath.Join(dir, "missing.md") +
				": no such file or directory",
		},
		"a guide that is not MIGRATION.md": {
			args: []string{"-guide", filepath.Join(dir, "guide.md"), code},
			want: "DID NOT RUN: read the guide " + filepath.Join(dir, "guide.md") + ": the guide " +
				"has no entry, an anchor such as <a id=\"M-module-1\"></a> alone on its line; " +
				"pass MIGRATION.md of v2",
		},
		"a directory that does not exist": {
			args: []string{"-guide", "testdata/guide.md", filepath.Join(dir, "missing")},
			want: "DID NOT RUN: stat " + filepath.Join(dir, "missing") + ": no such file or " +
				"directory",
		},
		"a file in place of a directory": {
			args: []string{"-guide", "testdata/guide.md", filepath.Join(code, "main.go")},
			want: "DID NOT RUN: " + filepath.Join(code, "main.go") + " is not a directory; pass " +
				"the directory of the code to scan",
		},
		"a Go file that does not parse": {
			args: []string{"-guide", "testdata/guide.md", filepath.Join(dir, "broken")},
			want: "DID NOT RUN: scan " + filepath.Join(dir, "broken") + ": parse main.go: " +
				"main.go:3:15: expected '}', found 'EOF'",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			exit, stdout, stderr := runScanner(test.args...)
			if exit != exitDidNotRun {
				t.Errorf("exit code %d, want %d", exit, exitDidNotRun)
			}
			if stdout != "" {
				t.Errorf("stdout is not empty:\n%s", stdout)
			}
			if last := lastLine(stderr); !strings.HasPrefix(last, test.want) {
				t.Errorf("the last line of stderr is %q, want it to start with %q", last,
					test.want)
			}
		})
	}
}

func TestRunHelp(t *testing.T) {
	t.Parallel()
	code, stdout, stderr := runScanner("-h")
	if code != exitPassed {
		t.Errorf("exit code %d, want %d", code, exitPassed)
	}
	if stdout != "" {
		t.Errorf("stdout is not empty:\n%s", stdout)
	}
	for _, want := range []string{"Usage: scanner -guide MIGRATION.md [dir]", "-guide"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr does not contain %q:\n%s", want, stderr)
		}
	}
}

// lastLine returns the last line of s, without its newline.
func lastLine(s string) string {
	lines := strings.Split(strings.TrimSuffix(s, "\n"), "\n")
	return lines[len(lines)-1]
}
