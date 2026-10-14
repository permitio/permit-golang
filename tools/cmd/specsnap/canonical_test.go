package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// readFixture reads a file under testdata.
func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// mustParse parses a spec or fails the test.
func mustParse(t *testing.T, data []byte) map[string]any {
	t.Helper()
	doc, err := parse(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return doc
}

// mustCanonical returns a spec's canonical form or fails the test.
func mustCanonical(t *testing.T, doc map[string]any) []byte {
	t.Helper()
	data, err := canonical(doc)
	if err != nil {
		t.Fatalf("canonical: %v", err)
	}
	return data
}

// TestCanonical checks the canonical form against a golden file: prose and examples go where
// they are keywords and stay where they are names or data, numbers keep their text, and HTML
// characters are not escaped.
func TestCanonical(t *testing.T) {
	got := mustCanonical(t, mustParse(t, readFixture(t, "canonical-in.json")))
	want := readFixture(t, "canonical-want.json")
	if !bytes.Equal(got, want) {
		t.Errorf("canonical form differs from testdata/canonical-want.json:\n%s", got)
	}
}

// TestCanonicalIsStable checks that the canonical form of a canonical spec is itself, and that
// the order of keys in the input does not matter.
func TestCanonicalIsStable(t *testing.T) {
	want := readFixture(t, "canonical-want.json")
	if got := mustCanonical(t, mustParse(t, want)); !bytes.Equal(got, want) {
		t.Errorf("the canonical form of a canonical spec changed:\n%s", got)
	}
	a := mustCanonical(t, mustParse(t, []byte(`{"b": 1, "a": {"y": [2, 1], "x": null}}`)))
	b := mustCanonical(t, mustParse(t, []byte(`{"a": {"x": null, "y": [2, 1]}, "b": 1}`)))
	wantText := "{\n  \"a\": {\n    \"x\": null,\n    \"y\": [\n      2,\n      1\n    ]\n  },\n" +
		"  \"b\": 1\n}\n"
	if string(a) != wantText || string(b) != wantText {
		t.Errorf("canonical forms:\n%s\n%s\nwant:\n%s", a, b, wantText)
	}
}

// TestCanonicalSecurityAndValues checks the cases the golden file does not: a security
// requirement that is not an array, arrays of names, values that are not objects, and an x- key
// that is a name.
func TestCanonicalSecurityAndValues(t *testing.T) {
	doc := mustParse(t, []byte(`{
		"security": {"title": "kept, though security should be an array"},
		"dependentRequired": {"title": ["summary"]},
		"x-list": [{"description": "kept"}],
		"properties": {
			"a": "not a schema", "b": [{"title": "dropped"}],
			"x-name": {"type": "string", "description": "dropped"}
		}
	}`))
	got := string(mustCanonical(t, doc))
	for _, want := range []string{
		"\"security\": {\n    \"title\": \"kept, though security should be an array\"\n  }",
		"\"dependentRequired\": {\n    \"title\": [\n      \"summary\"\n    ]\n  }",
		"\"x-list\": [\n    {\n      \"description\": \"kept\"\n    }\n  ]",
		`"a": "not a schema"`,
		"\"b\": [\n      {}\n    ]",
		"\"x-name\": {\n      \"type\": \"string\"\n    }",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("canonical form lacks\n%s\nin:\n%s", want, got)
		}
	}
}

func TestParse(t *testing.T) {
	doc := mustParse(t, []byte(`{"n": 1.50, "big": 9007199254740993}`))
	if got := mustCanonical(t, doc); !bytes.Contains(got, []byte("9007199254740993")) ||
		!bytes.Contains(got, []byte("1.50")) {
		t.Errorf("numbers lost their text: %s", got)
	}
	for name, input := range map[string]string{
		"empty":         "",
		"cut short":     `{"paths": {`,
		"not an object": `[1, 2]`,
		"more data":     `{"a": 1} {"b": 2}`,
		"text after":    `{"a": 1} x`,
	} {
		if _, err := parse([]byte(input)); err == nil {
			t.Errorf("%s: parse(%q) succeeded", name, input)
		}
	}
	if _, err := parse([]byte("{\"a\": 1}\n\n")); err != nil {
		t.Errorf("parse with trailing white space: %v", err)
	}
}

func TestCountOperations(t *testing.T) {
	for name, tc := range map[string]struct {
		spec string
		want int
	}{
		"every method": {`{"paths": {"/a": {"get": {}, "put": {}, "post": {}, "delete": {},
			"options": {}, "head": {}, "patch": {}, "trace": {}}}}`, 8},
		"other keys are not operations": {`{"paths": {"/a": {"summary": "x", "parameters": [],
			"servers": [], "get": {}}, "/b": {"post": {}}}}`, 2},
		"no paths":              {`{"openapi": "3.1.0"}`, 0},
		"paths is not a map":    {`{"paths": []}`, 0},
		"a path is not a map":   {`{"paths": {"/a": [], "/b": {"get": {}}}}`, 1},
		"upper case is not one": {`{"paths": {"/a": {"GET": {}}}}`, 0},
	} {
		if got := countOperations(mustParse(t, []byte(tc.spec))); got != tc.want {
			t.Errorf("%s: countOperations = %d, want %d", name, got, tc.want)
		}
	}
}
