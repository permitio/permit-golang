package main

import (
	"cmp"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestFenceAfter(t *testing.T) {
	for _, tc := range []struct {
		open, line string
		want       string
		wantFenced bool
	}{
		{"", "```go", "```", true},
		{"", "  ```sh", "```", true},
		{"", "~~~", "~~~", true},
		{"", "````markdown", "````", true},
		{"", "``code`` and text", "", false},
		{"", "```x``` is inline code", "", false},
		{"", "~~~ a`b", "~~~", true},
		{"", "text ```", "", false},
		{"", "", "", false},
		{"```", "```", "", true},
		{"```", "  ````  ", "", true},
		{"````", "```", "````", true},
		{"```", "```go", "```", true},
		{"```", "~~~", "```", true},
		{"~~~", "~~~~", "", true},
		{"```", "", "```", true},
		{"```", "[x](#y)", "```", true},
	} {
		got, fenced := fenceAfter(tc.open, tc.line)
		if got != tc.want || fenced != tc.wantFenced {
			t.Errorf("fenceAfter(%q, %q) = %q, %t, want %q, %t", tc.open, tc.line, got, fenced,
				tc.want, tc.wantFenced)
		}
	}
}

// The anchors that GitHub's rule gives these headings, in the order of a file; the first rows are
// headings of MIGRATION.md and CHANGELOG.md.
func TestSluggerAnchor(t *testing.T) {
	s := slugger{}
	for _, tc := range []struct{ text, want string }{
		{"Calls that could never work in v1", "calls-that-could-never-work-in-v1"},
		{"M-client-4: WithApiUrl becomes WithAPIURL", "m-client-4-withapiurl-becomes-withapiurl"},
		{"v1.3.0 (2026-10-13)", "v130-2026-10-13"},
		{"Appendix: v1 models types", "appendix-v1-models-types"},
		{
			text: "M-logging-1: WithLogger takes a *slog.Logger",
			want: "m-logging-1-withlogger-takes-a-sloglogger",
		},
		{
			text: "M-wire-1: Request bodies keep `<`, `>` and `&` as they are",
			want: "m-wire-1-request-bodies-keep---and--as-they-are",
		},
		{"snake_case Ünïcode cafe\u0301 ½ ² 3", "snake_case-ünïcode-cafe\u0301---3"},
		{"Fixes", "fixes"},
		{"Fixes", "fixes-1"},
		{"fixes", "fixes-2"},
		{"", ""},
	} {
		if got := s.anchor(tc.text); got != tc.want {
			t.Errorf("anchor(%q) = %q, want %q", tc.text, got, tc.want)
		}
	}
}

func TestParseMarkdown(t *testing.T) {
	md := parseMarkdown(migrationFile, []byte(`# Guide

[top](#guide) and [list](CHANGELOG.md#fixes "Fixes")[x](#a)
<a id="M-users-1"></a><a id="users-1"></a>
### M-users-1: Users
#hashtag
####### seven

`+"```markdown\n## Fenced\n<a id=\"fenced\"></a>\n[in a fence](#fenced)\n```\n"+`
## Guide
[after](#guide-1)`))
	wantHeadings := []heading{{1, "Guide"}, {3, "M-users-1: Users"}, {2, "Guide"}}
	if !slices.Equal(md.headings, wantHeadings) {
		t.Errorf("headings = %v, want %v", md.headings, wantHeadings)
	}
	wantAnchors := []string{"M-users-1", "guide", "guide-1", "m-users-1-users", "users-1"}
	if got := slices.Sorted(maps.Keys(md.anchors)); !slices.Equal(got, wantAnchors) {
		t.Errorf("anchors = %v, want %v", got, wantAnchors)
	}
	wantLinks := []link{
		{target: "#guide", line: 3, heading: 0},
		{target: "CHANGELOG.md#fixes", line: 3, heading: 0},
		{target: "#a", line: 3, heading: 0},
		{target: "#guide-1", line: 16, heading: 2},
	}
	if !slices.Equal(md.links, wantLinks) {
		t.Errorf("links = %v, want %v", md.links, wantLinks)
	}
	if md.name != migrationFile {
		t.Errorf("name = %q, want %q", md.name, migrationFile)
	}
	if got := parseMarkdown(readmeFile, []byte("[x](#y)\n# Title\n")).links; !slices.Equal(got,
		[]link{{target: "#y", line: 1, heading: -1}}) {
		t.Errorf("a link before the first heading = %v, want its heading -1", got)
	}
	headings := parseMarkdown(readmeFile,
		[]byte("## Retries ##  \n   ### Errors\n    # Code\n## C#\n## ##\n")).headings
	if want := []heading{{2, "Retries"}, {3, "Errors"}, {2, "C#"}, {2, ""}}; !slices.Equal(
		headings, want) {
		t.Errorf("headings with a closing sequence or an indent = %v, want %v", headings, want)
	}
}

// linksGuide, linksChangelog and linksReadme are files whose links resolve.
const (
	linksGuide = `# Guide

## Calls that could never work in v1

- [M-users-1](#M-users-1), as [v1.3.0](CHANGELOG.md#v130-2026-10-13) fixed it.

## API

<a id="M-users-1"></a>
### M-users-1: Users.Create moves
`
	linksChangelog = `# Changelog

## v1.3.0 (2026-10-13)

[M-users-1](MIGRATION.md#M-users-1) is in
[the list](MIGRATION.md#calls-that-could-never-work-in-v1), as [above](#changelog) says.

### Fixes

### Fixes

[The second](#fixes-1).
`
	linksReadme = `# SDK

[The entry](MIGRATION.md#m-users-1-userscreate-moves) and [Retries](#retries).

## Retries
`
)

// linkProblem is the advice after each problem of checkLinks.
const linkProblem = "; link the id of an <a id=\"...\"> tag, such as #M-users-1, or the anchor " +
	"that GitHub gives a heading: its text in lower case, without punctuation other than - and " +
	"_, and with a hyphen for each space"

func TestCheckLinks(t *testing.T) {
	for name, tc := range map[string]struct {
		guide, changelog, readme string
		want                     []string
	}{
		"every link resolves": {},
		"a link to an entry ID that the guide does not have": {
			guide: replaced(t, linksGuide, "(#M-users-1)", "(#M-users-2)"),
			want:  []string{"MIGRATION.md:5: the link #M-users-2 names no anchor of MIGRATION.md"},
		},
		"a link to a heading that the guide does not have": {
			changelog: replaced(t, linksChangelog, "#calls-that-could-never-work-in-v1",
				"#calls-that-never-worked"),
			want: []string{"CHANGELOG.md:6: the link MIGRATION.md#calls-that-never-worked names " +
				"no anchor of MIGRATION.md"},
		},
		"a link of CHANGELOG.md to an entry ID that the guide does not have": {
			changelog: replaced(t, linksChangelog, "MIGRATION.md#M-users-1",
				"MIGRATION.md#M-users-21"),
			want: []string{"CHANGELOG.md:5: the link MIGRATION.md#M-users-21 names no anchor of " +
				"MIGRATION.md"},
		},
		"a link to an entry ID in another case": {
			readme: replaced(t, linksReadme, "MIGRATION.md#m-users-1-userscreate-moves",
				"MIGRATION.md#m-users-1"),
			want: []string{"README.md:3: the link MIGRATION.md#m-users-1 names no anchor of " +
				"MIGRATION.md"},
		},
		"a link to a heading of CHANGELOG.md that it does not have": {
			guide: replaced(t, linksGuide, "#v130-2026-10-13", "#v1.3.0"),
			want: []string{"MIGRATION.md:5: the link CHANGELOG.md#v1.3.0 names no anchor of " +
				"CHANGELOG.md"},
		},
		"a link to an anchor of the file itself that it does not have": {
			changelog: replaced(t, linksChangelog, "(#fixes-1)", "(#fixes-2)"),
			readme:    replaced(t, linksReadme, "(#retries)", "(#retry)"),
			want: []string{
				"CHANGELOG.md:12: the link #fixes-2 names no anchor of CHANGELOG.md",
				"README.md:3: the link #retry names no anchor of README.md",
			},
		},
		"a link with ./ before the file name": {
			readme: linksReadme + "\n[Gone](./MIGRATION.md#M-users-3)\n",
			want:   []string{"README.md:7: the link ./MIGRATION.md#M-users-3 names no anchor"},
		},
		"links in a fenced code block, and one after it": {
			guide: linksGuide + "\n```markdown\n[x](#M-users-9)\n<a id=\"M-users-8\"></a>\n" +
				"### M-users-7: Fenced\n```\n\n~~~\n[y](#M-users-6)\n~~~\n" +
				"[z](#M-users-8) [w](#m-users-7-fenced)\n",
			want: []string{
				"MIGRATION.md:21: the link #M-users-8 names no anchor of MIGRATION.md",
				"MIGRATION.md:21: the link #m-users-7-fenced names no anchor of MIGRATION.md",
			},
		},
		"links that the check does not resolve": {
			readme: linksReadme + "\n[Security](SECURITY.md#missing), " +
				"[a URL](https://example.com/MIGRATION.md#missing), [the guide](MIGRATION.md), " +
				"[the top](MIGRATION.md#) and [code](err)\n",
			want: []string{"README.md:7: the link MIGRATION.md# names no anchor of MIGRATION.md"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			files := map[string]markdown{}
			for file, content := range map[string]string{
				migrationFile: cmp.Or(tc.guide, linksGuide),
				changelogFile: cmp.Or(tc.changelog, linksChangelog),
				readmeFile:    cmp.Or(tc.readme, linksReadme),
			} {
				files[file] = parseMarkdown(file, []byte(content))
			}
			got := checkLinks(files)
			if len(got) != len(tc.want) {
				t.Fatalf("checkLinks returned %d problems, want %d:\n%s", len(got), len(tc.want),
					strings.Join(got, "\n"))
			}
			for i, want := range tc.want {
				if !strings.HasPrefix(got[i], want) || !strings.HasSuffix(got[i], linkProblem) {
					t.Errorf("problem %d is\n%s\nwant\n%s%s", i, got[i], want, linkProblem)
				}
			}
		})
	}
}

// fixedListGuide is a MIGRATION.md whose section of the calls that could never work, its first
// heading, links each entry with the mark and the entry of fixedListed.
const fixedListGuide = `## Calls that could never work in v1

- [M-users-1](#M-users-1) and [M-old-1](#M-old-1).

### Details

- [M-roles-1](#M-roles-1), as [Check these first](#check-these-first) says.
- [M-gone-1](#M-gone-1) has no entry.

## API

<a id="M-users-1"></a>
### M-users-1: Users

- v1: x
- v2: y
- Bug fix: could never work before

<a id="M-roles-1"></a>
### M-roles-1: Roles

- v1: x
- v2: y
- Bug fix: could never work before

<a id="M-old-1"></a>
### M-old-1: Old

- v1: x
- v2: y

<a id="M-plain-1"></a>
### M-plain-1: Plain

- v1: x
- v2: y
`

// fixedListed is the listedWithoutMark of fixedListGuide; no entry has M-gone-1.
var fixedListed = map[string]string{
	"M-old-1":  "v1.3.0 fixed it",
	"M-gone-1": "an ID that no entry has",
}

const (
	// markedNotLinked follows the entry in the problem with a marked entry that the section does
	// not link.
	markedNotLinked = ": the entry is marked \"- Bug fix: could never work before\", and the " +
		"section \"Calls that could never work in v1\" does not link it; add a list item with "
	// sectionLinks starts the problem with a link of the section to an entry without the mark.
	sectionLinks = "the section \"Calls that could never work in v1\" links "
	// notMarked follows the entry in that problem.
	notMarked = ", which is not marked \"- Bug fix: could never work before\"; mark the entry " +
		"when v2 fixes the call, give it and the reason to listedWithoutMark in " +
		"tools/cmd/apisurface when v2 does not, or remove the link"
)

func TestCheckFixedList(t *testing.T) {
	g := fixedListGuide
	rolesNotLinked := markedNotLinked + "[M-roles-1](#M-roles-1)"
	for name, tc := range map[string]struct {
		guide string
		want  []string
	}{
		"the section links each marked entry and each listed entry": {guide: g},
		"no section": {
			guide: replaced(t, g, "## Calls that could never work in v1", "## Calls that failed"),
			want: []string{"MIGRATION.md has no section \"## Calls that could never work in " +
				"v1\"; add it, and link from it each entry marked \"- Bug fix: could never " +
				"work before\""},
		},
		"the section's heading at level 3": {
			guide: replaced(t, g, "## Calls", "### Calls"),
			want:  []string{"MIGRATION.md has no section"},
		},
		"the section after another section": {
			guide: "# Guide\n\n## Check these first\n\n" + g,
		},
		"the section at the end of the guide": {
			guide: g[strings.Index(g, "## API"):] + "\n" + g[:strings.Index(g, "## API")],
		},
		"a marked entry that the section does not link": {
			guide: replaced(t, g, "[M-roles-1](#M-roles-1)", "M-roles-1"),
			want:  []string{"MIGRATION.md:19 (M-roles-1)" + rolesNotLinked},
		},
		"a marked entry that only the next section links": {
			guide: replaced(t, replaced(t, g, "[M-roles-1](#M-roles-1)", "M-roles-1"), "## API\n",
				"## API\n\n[M-roles-1](#M-roles-1)\n"),
			want: []string{"MIGRATION.md:21 (M-roles-1)" + rolesNotLinked},
		},
		"a link to an entry without the mark": {
			guide: replaced(t, g, " has no entry.", " has no entry, and [M-plain-1](#M-plain-1) "+
				"and [M-plain-1 again](#M-plain-1) have no mark."),
			want: []string{"MIGRATION.md:8: " + sectionLinks + "M-plain-1" + notMarked},
		},
		"the mark removed from a linked entry": {
			guide: replaced(t, g, "Roles\n\n- v1: x\n- v2: y\n- Bug fix: could never work before\n",
				"Roles\n\n- v1: x\n- v2: y\n"),
			want: []string{"MIGRATION.md:7: " + sectionLinks + "M-roles-1" + notMarked},
		},
		"a listed entry that the section does not link": {
			guide: replaced(t, g, " and [M-old-1](#M-old-1)", ""),
			want: []string{"MIGRATION.md:26 (M-old-1): the section \"Calls that could never " +
				"work in v1\" does not link the entry, which listedWithoutMark in " +
				"tools/cmd/apisurface lists: v1.3.0 fixed it; link it, or remove it from " +
				"listedWithoutMark"},
		},
		"a listed entry with the mark": {
			guide: replaced(t, g, "### M-old-1: Old\n\n- v1: x\n- v2: y\n",
				"### M-old-1: Old\n\n- v1: x\n- v2: y\n- Bug fix: could never work before\n"),
			want: []string{"MIGRATION.md:26 (M-old-1): the entry is marked \"- Bug fix: could " +
				"never work before\", and listedWithoutMark in tools/cmd/apisurface gives the " +
				"reason it has no mark: v1.3.0 fixed it; remove one of them"},
		},
		"a link to an entry that the guide does not have": {
			guide: replaced(t, g, " has no entry.", " has no entry, nor [M-users-9](#M-users-9)."),
		},
	} {
		t.Run(name, func(t *testing.T) {
			content := []byte(tc.guide)
			guide, _ := parseGuide(content)
			got := checkFixedList(guide, parseMarkdown(migrationFile, content), fixedListed)
			if len(got) != len(tc.want) {
				t.Fatalf("checkFixedList returned %d problems, want %d:\n%s", len(got),
					len(tc.want), strings.Join(got, "\n"))
			}
			for i, want := range tc.want {
				if !strings.HasPrefix(got[i], want) {
					t.Errorf("problem %d is\n%s\nwant it to start with\n%s", i, got[i], want)
				}
			}
		})
	}
}

// TestRepositoryListedWithoutMark checks that each ID of listedWithoutMark is an entry of
// MIGRATION.md, as migration mode leaves out an ID that no entry has.
func TestRepositoryListedWithoutMark(t *testing.T) {
	content, err := os.ReadFile(filepath.Join(repoRoot, migrationFile))
	if err != nil {
		t.Fatal(err)
	}
	g, _ := parseGuide(content)
	ids := map[string]bool{}
	for _, e := range g.entries {
		ids[e.id] = true
	}
	for _, id := range slices.Sorted(maps.Keys(listedWithoutMark)) {
		if !ids[id] {
			t.Errorf("listedWithoutMark lists %s, which no entry of %s has", id, migrationFile)
		}
	}
}
