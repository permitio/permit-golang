package main

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"maps"
	"slices"
	"strconv"
)

// candidate is a line in scope whose mutants would run: gremlins' dry run found
// mutants on it that a test covers.
type candidate struct {
	// file is relative to the repository root.
	file    string
	line    int
	mutants int
}

// fileLine names a line of a file relative to the repository root.
type fileLine struct {
	file string
	line int
}

// chooseSample returns the candidates to test so that at most budget mutants run,
// or the first line's mutants when that line alone holds more: the first line is
// always taken. It goes round-robin over the files, each round taking the next
// line of every file, and skips a line whose mutants would take the total past the
// budget. The files, and the lines of each file, are in an order fixed by seed,
// the commit HEAD names, so a commit always gets the same sample and the next one
// usually another. While there are no more files than the budget, every file gets
// a line before any file gets a second; with more, the order of the files decides
// which of them the sample covers.
func chooseSample(candidates []candidate, seed string, budget int) map[fileLine]bool {
	byFile := map[string][]candidate{}
	for _, c := range candidates {
		byFile[c.file] = append(byFile[c.file], c)
	}
	files := slices.SortedFunc(maps.Keys(byFile), func(a, b string) int {
		return cmp.Or(cmp.Compare(sampleKey(seed, a), sampleKey(seed, b)), cmp.Compare(a, b))
	})
	for _, file := range files {
		slices.SortFunc(byFile[file], func(a, b candidate) int {
			return cmp.Or(cmp.Compare(sampleKey(seed, file, strconv.Itoa(a.line)),
				sampleKey(seed, file, strconv.Itoa(b.line))), cmp.Compare(a.line, b.line))
		})
	}
	chosen := map[fileLine]bool{}
	total := 0
	for round := 0; ; round++ {
		more := false
		for _, file := range files {
			lines := byFile[file]
			if round >= len(lines) {
				continue
			}
			more = true
			c := lines[round]
			if total > 0 && total+c.mutants > budget {
				continue
			}
			chosen[fileLine{c.file, c.line}] = true
			total += c.mutants
		}
		if !more {
			return chosen
		}
	}
}

// sampleKey orders the files and lines of a sample: the SHA-256 of the seed and
// parts.
func sampleKey(seed string, parts ...string) string {
	h := sha256.New()
	h.Write([]byte(seed))
	for _, part := range parts {
		h.Write([]byte{0})
		h.Write([]byte(part))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// sampleShare is how much of what would run a sample covers, in one module or in
// all of them.
type sampleShare struct {
	mutants, lines, files                   int
	chosenMutants, chosenLines, chosenFiles int
}

// add counts the candidates and the chosen ones among them.
func (s *sampleShare) add(candidates []candidate, chosen map[fileLine]bool) {
	files := map[string]bool{}
	for _, c := range candidates {
		s.mutants += c.mutants
		s.lines++
		if _, seen := files[c.file]; !seen {
			files[c.file] = false
		}
		if chosen[fileLine{c.file, c.line}] {
			s.chosenMutants += c.mutants
			s.chosenLines++
			files[c.file] = true
		}
	}
	for _, covered := range files {
		s.files++
		if covered {
			s.chosenFiles++
		}
	}
}
