package main

import (
	"fmt"
	"maps"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// The Markdown files besides MIGRATION.md whose links migration mode resolves, relative to the
// repository root.
const (
	changelogFile = "CHANGELOG.md"
	readmeFile    = "README.md"
)

var (
	// headingLine matches an ATX heading, indented by up to three spaces, and captures its level
	// and its text.
	headingLine = regexp.MustCompile(`^ {0,3}(#{1,6})(?:[ \t]+(.*))?$`)
	// closingSequence matches the closing sequence of an ATX heading's text, as in "## Retries ##".
	closingSequence = regexp.MustCompile(`(?:^|[ \t]+)#+[ \t]*$`)
	// anchorTag matches an HTML anchor, <a id="...">, and captures its id.
	anchorTag = regexp.MustCompile(`<a id="([^"]*)"`)
	// linkTarget matches the target of an inline link, [text](target) or [text](target "title"):
	// what follows "](" up to a space or the closing parenthesis.
	linkTarget = regexp.MustCompile(`\]\(([^)\s]*)`)
)

// markdown is a Markdown file, read for its headings, anchors and links. A fenced code block
// holds none of them.
type markdown struct {
	name     string
	headings []heading
	// anchors holds each fragment that names a place in the file: the id of each <a id="...">
	// tag, and the anchor that GitHub gives each heading.
	anchors map[string]bool
	links   []link
}

// heading is an ATX heading of a Markdown file, such as "## Compatibility".
type heading struct {
	level int
	text  string
}

// link is the target of an inline link of a Markdown file.
type link struct {
	target string
	line   int
	// heading is the index of the last heading before the link, or -1 when none is.
	heading int
}

// parseMarkdown reads the headings, anchors and links of the Markdown file name.
func parseMarkdown(name string, content []byte) markdown {
	md := markdown{name: name, anchors: map[string]bool{}}
	anchors := slugger{}
	fence := ""
	for i, line := range strings.Split(string(content), "\n") {
		var fenced bool
		if fence, fenced = fenceAfter(fence, line); fenced {
			continue
		}
		if m := headingLine.FindStringSubmatch(line); m != nil {
			text := closingSequence.ReplaceAllString(strings.TrimSpace(m[2]), "")
			h := heading{level: len(m[1]), text: text}
			md.headings = append(md.headings, h)
			md.anchors[anchors.anchor(h.text)] = true
		}
		for _, m := range anchorTag.FindAllStringSubmatch(line, -1) {
			md.anchors[m[1]] = true
		}
		for _, m := range linkTarget.FindAllStringSubmatch(line, -1) {
			l := link{target: m[1], line: i + 1, heading: len(md.headings) - 1}
			md.links = append(md.links, l)
		}
	}
	return md
}

// fenceAfter returns the fence that is open after line, given the one open before it, or "" for
// none, and whether line is part of a fenced code block: its opening fence, its code or its
// closing fence. A fence is three or more backquotes or tildes at the start of a line, after any
// indentation, and a fence of backquotes has no backquote after them on its line; the closing
// fence is the same character, at least as many times, alone on its line.
func fenceAfter(open, line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if open != "" {
		if len(trimmed) >= len(open) && strings.Trim(trimmed, open[:1]) == "" {
			return "", true
		}
		return open, true
	}
	for _, char := range []string{"`", "~"} {
		n := len(trimmed) - len(strings.TrimLeft(trimmed, char))
		if n >= 3 && (char == "~" || !strings.Contains(trimmed[n:], "`")) {
			return trimmed[:n], true
		}
	}
	return "", false
}

// slugger gives the headings of a file their anchors, as GitHub does. It maps an anchor to the
// number of headings before that have it.
type slugger map[string]int

// anchor returns the anchor of the next heading, whose text is text: the text in lower case,
// with each space a hyphen, and without each character but letters, marks, decimal digits,
// connector punctuation such as _ and hyphens; then, when a heading before has the same anchor,
// -1 for the second such heading, -2 for the third, and so on. GitHub reads the text as it
// renders it, which is the Markdown text for a heading without links, emphasis or HTML.
func (s slugger) anchor(text string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(text) {
		switch {
		case r == ' ':
			b.WriteByte('-')
		case r == '-' || unicode.IsLetter(r) || unicode.IsMark(r) || unicode.IsDigit(r) ||
			unicode.Is(unicode.Pc, r):
			b.WriteRune(r)
		}
	}
	anchor := b.String()
	n := s[anchor]
	s[anchor]++
	if n > 0 {
		return anchor + "-" + strconv.Itoa(n)
	}
	return anchor
}

// checkLinks returns a problem for each link of files, MIGRATION.md, CHANGELOG.md and
// README.md by name, to an anchor of one of them that the file does not have: #<anchor>, in the
// same file, or <file>#<anchor>. It does not check the other links, such as those to other files
// or to URLs.
func checkLinks(files map[string]markdown) []string {
	var problems []string
	for _, name := range slices.Sorted(maps.Keys(files)) {
		for _, l := range files[name].links {
			file, anchor, ok := strings.Cut(l.target, "#")
			if !ok {
				continue
			}
			if file == "" {
				file = name
			}
			to, ok := files[path.Clean(file)]
			if !ok || to.anchors[anchor] {
				continue
			}
			problems = append(problems, fmt.Sprintf("%s:%d: the link %s names no anchor of %s; "+
				"link the id of an <a id=\"...\"> tag, such as #M-users-1, or the anchor that "+
				"GitHub gives a heading: its text in lower case, without punctuation other than "+
				"- and _, and with a hyphen for each space", name, l.line, l.target, to.name))
		}
	}
	return problems
}
