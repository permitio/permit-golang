package main

import (
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"
)

func runNotes(args []string, stdout, stderr io.Writer) int {
	var root, tag, surface string
	flags := newFlags("notes", stderr, &root)
	flags.StringVar(&tag, "tag", "", "the release's `tag`, such as v2.1.0")
	flags.StringVar(&surface, "surface", "", "the output `file` of apisurface -mode release")
	if err := parse(flags, args, map[string]*string{"tag": &tag}); err != nil {
		return abort(stdout, err)
	}
	m := tagPattern.FindStringSubmatch(tag)
	if m == nil {
		return abort(stdout, fmt.Errorf("the tag %q is not a v2 release tag, such as v2.1.0", tag))
	}
	lines, err := readLines(filepath.Join(root, changelogFile), false)
	if err != nil {
		return abort(stdout, err)
	}
	version := strings.TrimSuffix(tag, m[3])
	section, err := changelogSection(lines, version)
	if err != nil {
		return abort(stdout, err)
	}
	var incompatible []string
	base := ""
	if surface != "" {
		out, err := readLines(surface, false)
		if err != nil {
			return abort(stdout, err)
		}
		base, incompatible = gorelease(out)
	}
	fmt.Fprintln(stdout, strings.Join(section, "\n"))
	if len(incompatible) > 0 {
		fmt.Fprintf(stdout, "\n### Incompatible changes since %s\n\n", base)
		for _, change := range incompatible {
			fmt.Fprintf(stdout, "- %s\n", change)
		}
	}
	return exitPassed
}

// changelogSection returns the lines of the section of CHANGELOG.md for version, such as v2.1.0,
// without its heading and the blank lines around it.
func changelogSection(lines []string, version string) ([]string, error) {
	heading := "## " + version
	start := slices.IndexFunc(lines, func(line string) bool {
		return line == heading || strings.HasPrefix(line, heading+" (")
	})
	if start < 0 {
		return nil, fmt.Errorf("%s has no section %q", changelogFile, heading)
	}
	start++
	end := len(lines)
	for i := start; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "## ") {
			end = i
			break
		}
	}
	section := lines[start:end]
	for len(section) > 0 && strings.TrimSpace(section[0]) == "" {
		section = section[1:]
	}
	for len(section) > 0 && strings.TrimSpace(section[len(section)-1]) == "" {
		section = section[:len(section)-1]
	}
	if len(section) == 0 {
		return nil, fmt.Errorf("the section %q of %s is empty", heading, changelogFile)
	}
	return section, nil
}

// gorelease returns the base and the incompatible changes that apisurface -mode release printed:
// "gorelease -base=<base> ..." and an "  incompatible: <change>" line for each change.
func gorelease(lines []string) (string, []string) {
	base := ""
	var changes []string
	for _, line := range lines {
		if rest, ok := strings.CutPrefix(line, "gorelease -base="); ok {
			base, _, _ = strings.Cut(rest, " ")
		}
		if change, ok := strings.CutPrefix(line, "  incompatible: "); ok {
			changes = append(changes, change)
		}
	}
	return base, changes
}
