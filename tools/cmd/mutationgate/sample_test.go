package main

import (
	"fmt"
	"maps"
	"math/rand/v2"
	"slices"
	"testing"
)

const (
	seedA = "4f1c1b8e2d7a9c0b3e5f6a7b8c9d0e1f2a3b4c5d"
	seedB = "9a8b7c6d5e4f3a2b1c0d9e8f7a6b5c4d3e2f1a0b"
)

// manyCandidates returns lines 1 to perFile of each of files files, each line with
// mutants mutants.
func manyCandidates(files, perFile, mutants int) []candidate {
	var out []candidate
	for f := range files {
		for line := 1; line <= perFile; line++ {
			out = append(out, candidate{
				file: fmt.Sprintf("pkg%d/f.go", f), line: line,
				mutants: mutants,
			})
		}
	}
	return out
}

func sampledMutants(cands []candidate, chosen map[fileLine]bool) int {
	n := 0
	for _, c := range cands {
		if chosen[fileLine{c.file, c.line}] {
			n += c.mutants
		}
	}
	return n
}

func sampledFiles(chosen map[fileLine]bool) map[string]int {
	files := map[string]int{}
	for fl := range chosen {
		files[fl.file]++
	}
	return files
}

// The sample depends on the commit only: the same candidates, in any order, give
// the same sample for one commit.
func TestChooseSampleIsTheSameForACommit(t *testing.T) {
	cands := manyCandidates(7, 30, 3)
	want := chooseSample(cands, seedA, 100)
	rng := rand.New(rand.NewPCG(1, 2))
	for range 20 {
		shuffled := slices.Clone(cands)
		rng.Shuffle(
			len(shuffled),
			func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] },
		)
		if got := chooseSample(shuffled, seedA, 100); !maps.Equal(got, want) {
			t.Fatalf("a shuffled input gives another sample:\n%v\nwant\n%v", got, want)
		}
	}
	if got := chooseSample(cands, seedB, 100); maps.Equal(got, want) {
		t.Errorf("another commit gives the same sample of %d lines", len(got))
	}
}

func TestChooseSampleKeepsToTheBudget(t *testing.T) {
	tests := []struct {
		name        string
		cands       []candidate
		budget      int
		wantMutants int
		wantFiles   int
		// wantPerFile is the most lines any file gets.
		wantPerFile int
	}{
		{
			name: "fewer mutants than the budget", cands: manyCandidates(3, 4, 2), budget: 400,
			wantMutants: 24, wantFiles: 3, wantPerFile: 4,
		},
		{
			name: "every file gets a line first", cands: manyCandidates(5, 10, 1), budget: 7,
			wantMutants: 7, wantFiles: 5, wantPerFile: 2,
		},
		{
			name: "more files than the budget", cands: manyCandidates(50, 2, 1), budget: 20,
			wantMutants: 20, wantFiles: 20, wantPerFile: 1,
		},
		{
			name: "a line that would pass the budget is left out", budget: 10,
			cands: []candidate{
				{file: "a.go", line: 1, mutants: 6},
				{file: "b.go", line: 1, mutants: 6},
				{file: "c.go", line: 1, mutants: 4},
			},
			wantMutants: 10, wantFiles: 2, wantPerFile: 1,
		},
		{
			name: "the first line is taken even past the budget", budget: 2,
			cands:       []candidate{{file: "a.go", line: 1, mutants: 5}},
			wantMutants: 5, wantFiles: 1, wantPerFile: 1,
		},
		{name: "nothing would run", budget: 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chosen := chooseSample(tt.cands, seedA, tt.budget)
			files := sampledFiles(chosen)
			perFile := 0
			for _, n := range files {
				perFile = max(perFile, n)
			}
			if got := sampledMutants(tt.cands, chosen); got != tt.wantMutants ||
				len(files) != tt.wantFiles || perFile != tt.wantPerFile {
				t.Errorf("%d mutants in %d files, at most %d lines a file; want %d, %d and %d",
					got, len(files), perFile, tt.wantMutants, tt.wantFiles, tt.wantPerFile)
			}
		})
	}
}

// Every line the sample leaves out would take it past the budget: a line that does
// not fit is skipped, and the lines after it, in its round and in later ones, are
// still considered.
func TestChooseSampleSkipsOnlyWhatDoesNotFit(t *testing.T) {
	var twos, mixed []candidate
	for i := range 10 {
		twos = append(twos, candidate{file: fmt.Sprintf("f%d.go", i), line: 1, mutants: 2})
	}
	twos = append(twos, candidate{file: "small.go", line: 1, mutants: 1})
	for i := range 12 {
		for line := 1; line <= 3; line++ {
			mixed = append(mixed, candidate{
				file: fmt.Sprintf("m%d.go", i), line: line, mutants: (7*i+line)%4 + 1,
			})
		}
	}
	for name, cands := range map[string][]candidate{"twos": twos, "mixed": mixed} {
		for _, seed := range []string{seedA, seedB, "c", "d", "e", "f"} {
			for _, budget := range []int{5, 9, 20} {
				chosen := chooseSample(cands, seed, budget)
				total := sampledMutants(cands, chosen)
				for _, c := range cands {
					if !chosen[fileLine{c.file, c.line}] && total+c.mutants <= budget {
						t.Errorf("%s, seed %s, budget %d: %s:%d fits (%d + %d) but was left out",
							name, seed, budget, c.file, c.line, total, c.mutants)
					}
				}
			}
		}
	}
}

// With more files than the budget, the commit decides which files the sample
// covers: another commit covers others.
func TestChooseSampleSamplesFilesByCommit(t *testing.T) {
	cands := manyCandidates(40, 1, 1)
	a := sampledFiles(chooseSample(cands, seedA, 10))
	b := sampledFiles(chooseSample(cands, seedB, 10))
	if len(a) != 10 || len(b) != 10 || maps.Equal(a, b) {
		t.Errorf("files sampled for two commits:\n%v\n%v", a, b)
	}
}

// A file counts as covered when the sample holds any of its lines, before or after
// the lines it leaves out.
func TestSampleShare(t *testing.T) {
	cands := []candidate{
		{file: "a.go", line: 1, mutants: 2},
		{file: "a.go", line: 5, mutants: 1},
		{file: "b.go", line: 3, mutants: 4},
	}
	tests := []struct {
		name   string
		chosen map[fileLine]bool
		want   sampleShare
	}{
		{
			name:   "a chosen line after one left out",
			chosen: map[fileLine]bool{{"a.go", 5}: true, {"z.go", 1}: true},
			want: sampleShare{
				mutants: 7, lines: 3, files: 2, chosenMutants: 1, chosenLines: 1,
				chosenFiles: 1,
			},
		},
		{
			name:   "a chosen line before one left out",
			chosen: map[fileLine]bool{{"a.go", 1}: true},
			want: sampleShare{
				mutants: 7, lines: 3, files: 2, chosenMutants: 2, chosenLines: 1,
				chosenFiles: 1,
			},
		},
	}
	for _, tt := range tests {
		var s sampleShare
		s.add(cands, tt.chosen)
		if s != tt.want {
			t.Errorf("%s: got %+v, want %+v", tt.name, s, tt.want)
		}
	}
	if got := percent(1, 3); got != "33%" {
		t.Errorf("percent(1, 3) = %s", got)
	}
	if got := percent(0, 0); got != "0%" {
		t.Errorf("percent(0, 0) = %s", got)
	}
}

// The candidates are the lines with mutants that would run, each with how many: a
// line's mutants that no test covers, or that gremlins skipped, do not count.
func TestDryRunCandidates(t *testing.T) {
	got := dryRunCandidates([]mutation{
		m("a.go", 1, statusRunnable), m("a.go", 1, statusRunnable),
		m("a.go", 1, statusNotCovered), m("a.go", 2, statusNotCovered),
		m("a.go", 4, statusSkipped), m("b.go", 3, statusRunnable),
	})
	want := []candidate{{file: "a.go", line: 1, mutants: 2}, {file: "b.go", line: 3, mutants: 1}}
	if !slices.Equal(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}
