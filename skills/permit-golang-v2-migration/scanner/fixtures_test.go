package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strings"
	"testing"
)

// The comments that annotate the uses of v1 in the fixtures, each followed by v1 identifiers or
// import paths separated by spaces, up to the end of the line or the next comment.
const (
	// foundMark lists what the scanner must report on the line.
	foundMark = "// v1: "
	// missedMark lists the methods and fields, on the line, of a value whose v1 type the scanner
	// does not know, such as the result of a v1 API call, which it must not report.
	missedMark = "// v1 missed: "
)

// fixtureDirs are the fixtures under testdata: v1 code whose lines that use v1 each end with the
// comment foundMark, missedMark or both.
var fixtureDirs = []string{
	"testdata/readme/init",
	"testdata/readme/check",
	"testdata/readme/sync",
	"testdata/readme/calls",
	"testdata/provider",
	"testdata/unmapped",
	"testdata/none",
}

// annotations returns what the comments that start with marker, foundMark or missedMark, list
// in the Go files of fsys, each as "<file>:<line> <v1>", sorted.
func annotations(t *testing.T, fsys fs.FS, marker string) []string {
	t.Helper()
	var want []string
	err := fs.WalkDir(fsys, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(name, ".go") {
			return err
		}
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			return err
		}
		lines := bufio.NewScanner(bytes.NewReader(data))
		for n := 1; lines.Scan(); n++ {
			_, listed, ok := strings.Cut(lines.Text(), marker)
			if !ok {
				continue
			}
			listed, _, _ = strings.Cut(listed, "//")
			for v1 := range strings.FieldsSeq(listed) {
				want = append(want, fmt.Sprintf("%s:%d %s", name, n, v1))
			}
		}
		return lines.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(want)
	return want
}

// TestFixtures is the count sentinel: the scanner reports each use of v1 that the fixtures
// annotate as found, none that they annotate as missed, and nothing else.
func TestFixtures(t *testing.T) {
	t.Parallel()
	totalFound, totalMissed := 0, 0
	for _, dir := range fixtureDirs {
		totalFound += len(annotations(t, os.DirFS(dir), foundMark))
		totalMissed += len(annotations(t, os.DirFS(dir), missedMark))
	}
	// The fixtures annotate this many uses of v1 that the scanner finds, and this many that it
	// cannot; a fixture that lost its comments would lower them.
	if totalFound != 124 || totalMissed != 3 {
		t.Errorf("the fixtures annotate %d uses of v1 as found and %d as missed, want 124 and 3",
			totalFound, totalMissed)
	}
	for _, dir := range fixtureDirs {
		t.Run(dir, func(t *testing.T) {
			t.Parallel()
			fsys := os.DirFS(dir)
			want := annotations(t, fsys, foundMark)
			uses, err := scan(fsys, v1Packages(t))
			if err != nil {
				t.Fatalf("scan: %v", err)
			}
			got := make([]string, 0, len(uses))
			for _, u := range uses {
				got = append(got, fmt.Sprintf("%s:%d %s", u.file, u.line, u.v1))
			}
			slices.Sort(got)
			for _, missing := range difference(want, got) {
				t.Errorf("the scanner did not report %s", missing)
			}
			knownMisses := annotations(t, fsys, missedMark)
			for _, extra := range difference(got, want) {
				if slices.Contains(knownMisses, extra) {
					t.Errorf("the scanner reported %s, which a comment lists as missed; move it "+
						"to the comment of what it reports", extra)
					continue
				}
				t.Errorf("the scanner reported %s, which no comment lists", extra)
			}
			t.Logf("%s: %d uses of v1 annotated, %d reported", dir, len(want), len(got))
		})
	}
}

// difference returns the elements of a, sorted, that b does not have, counting repeats.
func difference(a, b []string) []string {
	left := map[string]int{}
	for _, s := range b {
		left[s]++
	}
	var out []string
	for _, s := range a {
		if left[s] > 0 {
			left[s]--
			continue
		}
		out = append(out, s)
	}
	return out
}
