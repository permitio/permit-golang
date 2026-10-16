package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// The files of the migration check, relative to the repository root.
const (
	surfaceFile   = "tools/migration/v1-surface.txt"
	pendingFile   = "tools/migration/pending.txt"
	idsFile       = "tools/migration/ids.txt"
	migrationFile = "MIGRATION.md"
)

// buckets maps the heading of each bucket section of MIGRATION.md to the bucket.
var buckets = map[string]string{
	"## Compatibility":  "compatibility",
	"## API":            "API",
	"## Wire behaviour": "wire behaviour",
}

// exclusionReasons are the reasons a v2: line may give when v2 has no counterpart for what the
// entry's v1: lines name, after "not exposed in v2: ". "not exposed in v2" alone is allowed too.
var exclusionReasons = []string{
	"account administration",
	"dashboard or CLI helper",
	"Elements back-end",
	"operator route",
	"deprecated upstream",
	"EAP",
}

// notExposed starts every exclusion reason.
const notExposed = "not exposed in v2"

// forbiddenWords may not appear in an entry, from its heading to the next anchor or heading, in
// any case: MIGRATION.md is public, and an entry that names a weakness of the API would disclose
// it before it is fixed.
var forbiddenWords = []string{"secret", "exposes", "vulnerab"}

// fixMark is the line that marks an entry as a fix of a v1 feature that could never work.
const fixMark = "- Bug fix: could never work before"

// fixedSection is the text of the heading, of level 2, of the section of MIGRATION.md that links
// each entry with fixMark.
const fixedSection = "Calls that could never work in v1"

// listedWithoutMark maps the ID of each entry that the section fixedSection links without
// fixMark to the reason. Each is a v1 call that never worked as documented, but that v2 does not
// fix: v1.3.0, the release this guide starts from, fixed it, or v2 drops the feature.
var listedWithoutMark = map[string]string{
	"M-action-groups-6": "v1.3.0 fixed ResourceActionGroups.Delete, which deleted an action " +
		"up to v1.2.8",
	"M-checks-8": "v1.3.0 fixed the checks that sent no key for a resource",
	"M-checks-13": "v2 drops v1's OPA mode, in which BulkCheck and FilterObjects could never " +
		"return decisions",
	"M-checks-23":        "v1.3.0 fixed AllTenantsCheck, which returned the denied tenants too",
	"M-condition-sets-3": "v1.3.0 fixed ConditionSets.List, which always returned an empty list",
	"M-facts-7":          "v1.3.0 fixed the timeout of WaitForSync under one second",
}

const (
	// appendixBegin and appendixEnd enclose the models table that apisurface writes into
	// MIGRATION.md.
	appendixBegin = "<!-- apisurface:models:begin -->"
	appendixEnd   = "<!-- apisurface:models:end -->"
)

var (
	// entryID matches an entry ID, M-<area>-<n>; the area's words start with a letter.
	entryID = regexp.MustCompile(`^M-[a-z][a-z0-9]*(?:-[a-z][a-z0-9]*)*-[1-9][0-9]*$`)
	// anchorLine matches the anchor that starts an entry.
	anchorLine = regexp.MustCompile(`^<a id="([^"]*)"></a>$`)
	// areaName matches the area of a pending line.
	areaName = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)
)

// entry is an entry of MIGRATION.md.
type entry struct {
	id string
	// line is the line number of the entry's anchor.
	line int
	// v1 holds the text after "- v1: " of each v1: line.
	v1 []string
	// v2 holds the text after "- v2: " of each v2: line; an entry has exactly one.
	v2 []string
	// v2Line is the line number of the first v2: line.
	v2Line int
	// fixes is set by the line fixMark.
	fixes bool
	// badFields holds the problems with the entry's field lines, such as a malformed mark.
	badFields []string
}

// where names the entry for a message.
func (e *entry) where() string {
	return fmt.Sprintf("%s:%d (%s)", migrationFile, e.line, e.id)
}

// guide is MIGRATION.md, parsed.
type guide struct {
	lines   []string
	entries []entry
	// beginLine and endLine are the line numbers of the appendix markers, or 0 when the guide
	// has no such marker. endLine is set only after beginLine.
	beginLine, endLine int
}

// parseGuide parses MIGRATION.md and returns it with the problems of its format: an anchor
// outside a bucket section, without its heading, or with a malformed or repeated ID; a heading of
// an entry without its anchor; an entry field outside an entry; a line of an entry, from its
// heading to the next anchor or heading, with one of forbiddenWords; and appendix markers that
// are missing, repeated or out of order.
func parseGuide(content []byte) (guide, []string) {
	g := guide{lines: strings.Split(string(content), "\n")}
	var problems []string
	add := func(n int, format string, args ...any) {
		problems = append(problems, fmt.Sprintf("%s:%d: ", migrationFile, n+1)+
			fmt.Sprintf(format, args...))
	}
	checkWords := func(n int, id string) {
		lower := strings.ToLower(g.lines[n])
		for _, word := range forbiddenWords {
			if strings.Contains(lower, word) {
				add(n, "the entry %s contains %q; %s is public, so an entry must not describe a "+
					"weakness of the API", id, word, migrationFile)
			}
		}
	}
	seen := map[string]int{}
	bucket := ""
	var cur *entry
	for i := 0; i < len(g.lines); i++ {
		line := g.lines[i]
		switch {
		case line == appendixBegin || line == appendixEnd:
			problems = append(problems, g.markAppendix(i, line)...)
			cur = nil
		case strings.HasPrefix(line, "## ") || line == "##":
			bucket = buckets[line]
			cur = nil
		case strings.HasPrefix(line, "### "):
			if strings.HasPrefix(line, "### M-") {
				add(i, "the heading %q has no anchor before it; start the entry with "+
					"<a id=\"<ID>\"></a>", line)
			}
			cur = nil
		case strings.Contains(line, `id="M-`) || anchorLine.MatchString(line):
			e, next, problem := parseAnchor(g.lines, i, bucket)
			if problem != "" {
				add(i, "%s", problem)
				cur = nil
				continue
			}
			if first, ok := seen[e.id]; ok {
				add(i, "the entry ID %s is used on line %d too; give each entry its own ID",
					e.id, first+1)
			}
			seen[e.id] = i
			g.entries = append(g.entries, e)
			cur = &g.entries[len(g.entries)-1]
			i = next
			checkWords(i, e.id)
		case isField(line):
			if cur == nil {
				add(i, "%q is outside an entry; put it under an entry's heading", line)
				continue
			}
			cur.addField(line, i+1)
			checkWords(i, cur.id)
		case cur != nil:
			checkWords(i, cur.id)
		}
	}
	if g.endLine == 0 {
		problems = append(problems, fmt.Sprintf("%s has no models appendix: add a line %s and, "+
			"after it, a line %s, then run apisurface -mode migration -write", migrationFile,
			appendixBegin, appendixEnd))
	}
	for _, e := range g.entries {
		problems = append(problems, e.formatProblems()...)
	}
	return g, problems
}

// markAppendix records an appendix marker at index i and returns the problem with it, if any.
func (g *guide) markAppendix(i int, line string) []string {
	n := i + 1
	problem := func(format string, args ...any) []string {
		return []string{fmt.Sprintf("%s:%d: ", migrationFile, n) + fmt.Sprintf(format, args...)}
	}
	if line == appendixBegin {
		if g.beginLine != 0 {
			return problem("a second %s; keep one models appendix", appendixBegin)
		}
		g.beginLine = n
		return nil
	}
	switch {
	case g.endLine != 0:
		return problem("a second %s; keep one models appendix", appendixEnd)
	case g.beginLine == 0:
		return problem("%s before %s", appendixEnd, appendixBegin)
	}
	g.endLine = n
	return nil
}

// parseAnchor parses the entry whose anchor is lines[i]: the anchor, then its heading
// "### <ID>: <title>" on the next line that is not blank. It returns the entry, the index of the
// heading, and a problem instead when the anchor or heading is malformed or the entry is outside
// a bucket section.
func parseAnchor(lines []string, i int, bucket string) (entry, int, string) {
	m := anchorLine.FindStringSubmatch(lines[i])
	switch {
	case m == nil:
		return entry{}, i, fmt.Sprintf("%q is not an entry anchor, <a id=\"M-<area>-<n>\"></a>, "+
			"alone on its line", lines[i])
	case !entryID.MatchString(m[1]):
		return entry{}, i, fmt.Sprintf("the entry ID %q is not M-<area>-<n>, with an area in "+
			"lower case, such as M-users-1", m[1])
	case bucket == "":
		return entry{}, i, fmt.Sprintf("the entry %s is outside the bucket sections; put it "+
			"under ## Compatibility, ## API or ## Wire behaviour", m[1])
	}
	next := i + 1
	for next < len(lines) && strings.TrimSpace(lines[next]) == "" {
		next++
	}
	prefix := "### " + m[1] + ": "
	if next >= len(lines) || !strings.HasPrefix(lines[next], prefix) ||
		strings.TrimSpace(strings.TrimPrefix(lines[next], prefix)) == "" {
		return entry{}, i, fmt.Sprintf("the anchor of %s is not followed by its heading, "+
			"%q and a title", m[1], prefix)
	}
	return entry{id: m[1], line: i + 1}, next, ""
}

// isField reports whether a line is a field of an entry.
func isField(line string) bool {
	return strings.HasPrefix(line, "- v1:") || strings.HasPrefix(line, "- v2:") ||
		strings.HasPrefix(line, "- Bug fix:")
}

// addField adds a field line, on line n, to the entry.
func (e *entry) addField(line string, n int) {
	switch {
	case strings.HasPrefix(line, "- v1:"):
		e.v1 = append(e.v1, strings.TrimSpace(strings.TrimPrefix(line, "- v1:")))
	case strings.HasPrefix(line, "- v2:"):
		if len(e.v2) == 0 {
			e.v2Line = n
		}
		e.v2 = append(e.v2, strings.TrimSpace(strings.TrimPrefix(line, "- v2:")))
	case line == fixMark && !e.fixes:
		e.fixes = true
	default:
		e.badFields = append(e.badFields, fmt.Sprintf("line %d, %q, is not the bug fix mark, "+
			"which reads exactly %q, once", n, line, fixMark))
	}
}

// formatProblems returns the problems with an entry's fields: no v1: line or an empty one, not
// exactly one v2: line or an empty one, and a malformed or repeated bug fix mark.
func (e *entry) formatProblems() []string {
	var problems []string
	add := func(format string, args ...any) {
		problems = append(problems, e.where()+": "+fmt.Sprintf(format, args...))
	}
	if len(e.v1) == 0 {
		add("the entry has no v1: line; name what it changes in v1")
	}
	if slices.Contains(e.v1, "") {
		add("a v1: line is empty")
	}
	if len(e.v2) != 1 || e.v2[0] == "" {
		add("the entry needs exactly one v2: line, not empty, and has %d", len(e.v2))
	}
	for _, problem := range e.badFields {
		add("%s", problem)
	}
	return problems
}

// unclosed is what reference returns for a line that starts with a backquote that no other
// closes.
const unclosed = "`"

// reference returns the reference that a v1: or v2: line starts with, after an optional
// "replaced by ": the text between its first two backquotes. It returns "" for a line that does
// not start with a backquote, which is prose, and unclosed when the backquotes hold nothing or
// the first is not closed.
func reference(value string) string {
	value = strings.TrimPrefix(value, "replaced by ")
	rest, ok := strings.CutPrefix(value, "`")
	if !ok {
		return ""
	}
	ref, _, ok := strings.Cut(rest, "`")
	if !ok || ref == "" {
		return unclosed
	}
	return ref
}

// unclosedProblem is the problem with a line whose reference is unclosed.
const unclosedProblem = "starts with a backquote that does not enclose a reference; name an " +
	"id in backquotes, or write prose that does not start with a backquote"

// pending is a line of pending.txt: a v1 id or pattern and the v2 area that will write its
// entry.
type pending struct {
	pattern, area string
	line          int
}

// where names the pending line for a message.
func (p pending) where() string {
	return fmt.Sprintf("%s:%d", pendingFile, p.line)
}

// parsePending parses pending.txt and returns its lines and the problems with them.
func parsePending(r io.Reader) ([]pending, []string, error) {
	var lines []pending
	var problems []string
	err := eachLine(r, func(n int, line string) {
		fields := strings.Fields(line)
		switch {
		case len(fields) != 2:
			problems = append(problems, fmt.Sprintf("%s:%d: %q is not <id or pattern> <area>",
				pendingFile, n, line))
		case !validPattern(fields[0]):
			problems = append(problems, fmt.Sprintf("%s:%d: %q is not a valid pattern",
				pendingFile, n, fields[0]))
		case !areaName.MatchString(fields[1]):
			problems = append(problems, fmt.Sprintf("%s:%d: the area %q is not lower-case words "+
				"joined by hyphens, such as users or api-keys", pendingFile, n, fields[1]))
		default:
			lines = append(lines, pending{pattern: fields[0], area: fields[1], line: n})
		}
	})
	return lines, problems, err
}

// eachLine calls fn with each line of r, numbered from 1, but blank lines and lines that start
// with #.
func eachLine(r io.Reader, fn func(n int, line string)) error {
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fn(n, line)
	}
	return sc.Err()
}

// validPattern reports whether s is a pattern that path.Match accepts.
func validPattern(s string) bool {
	_, err := path.Match(s, "")
	return err == nil
}

// parseIDs parses ids.txt: one entry ID per line. It returns the IDs in order and the problems:
// a malformed or repeated ID.
func parseIDs(r io.Reader, name string) ([]string, []string, error) {
	var ids, problems []string
	seen := map[string]bool{}
	err := eachLine(r, func(n int, line string) {
		switch {
		case !entryID.MatchString(line):
			problems = append(problems, fmt.Sprintf("%s:%d: %q is not an entry ID, M-<area>-<n>",
				name, n, line))
		case seen[line]:
			problems = append(problems, fmt.Sprintf("%s:%d: %s is listed twice", name, n, line))
		default:
			seen[line] = true
			ids = append(ids, line)
		}
	})
	return ids, problems, err
}

// migrationInput is what the migration check reads.
type migrationInput struct {
	items []item
	guide guide
	// docs maps MIGRATION.md, CHANGELOG.md and README.md to their headings, anchors and links.
	docs    map[string]markdown
	pending []pending
	ids     []string
	v2      v2Surface
	// problems are those that parsing found.
	problems []string
}

// readMigration reads the files of the migration check under root. It returns an error, for
// exit 2, when a file cannot be read or v1-surface.txt or v2's surface cannot be parsed.
func readMigration(root string) (migrationInput, error) {
	var in migrationInput
	content, err := os.ReadFile(filepath.Join(root, surfaceFile))
	if err != nil {
		return in, err
	}
	if in.items, err = readSurface(bytes.NewReader(content)); err != nil {
		return in, fmt.Errorf("%s: %w", surfaceFile, err)
	}
	content, err = os.ReadFile(filepath.Join(root, migrationFile))
	if err != nil {
		return in, err
	}
	var problems []string
	in.guide, problems = parseGuide(content)
	in.problems = append(in.problems, problems...)
	in.docs = map[string]markdown{migrationFile: parseMarkdown(migrationFile, content)}
	for _, name := range []string{changelogFile, readmeFile} {
		if content, err = os.ReadFile(filepath.Join(root, name)); err != nil {
			return in, err
		}
		in.docs[name] = parseMarkdown(name, content)
	}
	content, err = os.ReadFile(filepath.Join(root, pendingFile))
	if err != nil {
		return in, err
	}
	if in.pending, problems, err = parsePending(bytes.NewReader(content)); err != nil {
		return in, err
	}
	in.problems = append(in.problems, problems...)
	content, err = os.ReadFile(filepath.Join(root, idsFile))
	if err != nil {
		return in, err
	}
	if in.ids, problems, err = parseIDs(bytes.NewReader(content), idsFile); err != nil {
		return in, err
	}
	in.problems = append(in.problems, problems...)
	in.v2, err = readV2Surface(root)
	return in, err
}

// coverage is how each v1 item is covered.
type coverage struct {
	// entries maps an item's id to the entries whose v1: lines match it.
	entries map[string][]entry
	// pending maps an item's id to the pending lines that match it.
	pending map[string][]pending
}

// checkMigration returns the problems of the migration check: every rule that the package
// documentation lists for migration mode.
func checkMigration(in migrationInput) ([]string, coverage) {
	problems := slices.Clone(in.problems)
	cov := coverage{entries: map[string][]entry{}, pending: map[string][]pending{}}
	for _, e := range in.guide.entries {
		covers := false
		for _, v1 := range e.v1 {
			ref := reference(v1)
			switch ref {
			case "":
				continue
			case unclosed:
				problems = append(problems, fmt.Sprintf("%s: the v1: line %q %s", e.where(), v1,
					unclosedProblem))
				continue
			}
			covers = true
			ids, problem := match(in.items, ref)
			if problem != "" {
				problems = append(problems, fmt.Sprintf("%s: the v1: line `%s` %s", e.where(),
					ref, problem))
			}
			for _, id := range ids {
				cov.entries[id] = append(cov.entries[id], e)
			}
		}
		problems = append(problems, checkV2(e, covers, in.v2)...)
	}
	byID := map[string]item{}
	for _, it := range in.items {
		byID[it.id] = it
	}
	for _, p := range in.pending {
		ids, problem := match(in.items, p.pattern)
		if problem != "" {
			problems = append(problems, fmt.Sprintf("%s: %s %s; remove the line", p.where(),
				p.pattern, problem))
		}
		needed := false
		for _, id := range ids {
			cov.pending[id] = append(cov.pending[id], p)
			needed = needed || !sameNameInV2(byID[id], in.v2)
		}
		if needed || len(ids) == 0 {
			continue
		}
		if len(ids) == 1 && ids[0] == p.pattern {
			problems = append(problems, fmt.Sprintf("%s: v2 exports %s, a models type of the same "+
				"name, which needs no pending line; remove the line", p.where(), p.pattern))
			continue
		}
		problems = append(problems, fmt.Sprintf("%s: %s matches only models types that v2 "+
			"exports under the same name, which need no pending line; remove the line", p.where(),
			p.pattern))
	}
	for _, it := range in.items {
		problems = append(problems, checkCovered(it, cov, in.v2)...)
	}
	problems = append(problems, checkIDs(in.guide.entries, in.ids)...)
	problems = append(problems, checkLinks(in.docs)...)
	problems = append(problems, checkFixedList(in.guide, in.docs[migrationFile],
		listedWithoutMark)...)
	return problems, cov
}

// checkFixedList returns the problems with the section fixedSection of MIGRATION.md, md: it is
// missing; it does not link an entry with fixMark, or an entry of listed; it links an entry that
// has no mark and is not in listed; or an entry of listed has the mark. The section ends at the
// next heading of level 2 or less, and links an entry with #<ID>. listed maps the ID of an entry
// that the section links without the mark to the reason; an ID that no entry has is left out.
func checkFixedList(g guide, md markdown, listed map[string]string) []string {
	start := slices.IndexFunc(md.headings, func(h heading) bool {
		return h.level == 2 && h.text == fixedSection
	})
	if start < 0 {
		return []string{fmt.Sprintf("%s has no section \"## %s\"; add it, and link from it each "+
			"entry marked %q", migrationFile, fixedSection, fixMark)}
	}
	end := len(md.headings)
	for i := start + 1; i < len(md.headings); i++ {
		if md.headings[i].level <= 2 {
			end = i
			break
		}
	}
	byID := map[string]entry{}
	for _, e := range g.entries {
		byID[e.id] = e
	}
	var problems []string
	linked := map[string]bool{}
	for _, l := range md.links {
		id, ok := strings.CutPrefix(l.target, "#")
		if !ok || l.heading < start || l.heading >= end || linked[id] {
			continue
		}
		linked[id] = true
		e, ok := byID[id]
		if _, isListed := listed[id]; ok && !e.fixes && !isListed {
			problems = append(problems, fmt.Sprintf("%s:%d: the section %q links %s, which is not "+
				"marked %q; mark the entry when v2 fixes the call, give it and the reason to "+
				"listedWithoutMark in tools/cmd/apisurface when v2 does not, or remove the link",
				migrationFile, l.line, fixedSection, id, fixMark))
		}
	}
	for _, e := range g.entries {
		reason, isListed := listed[e.id]
		switch {
		case e.fixes && isListed:
			problems = append(problems, fmt.Sprintf("%s: the entry is marked %q, and "+
				"listedWithoutMark in tools/cmd/apisurface gives the reason it has no mark: %s; "+
				"remove one of them", e.where(), fixMark, reason))
		case e.fixes && !linked[e.id]:
			problems = append(problems, fmt.Sprintf("%s: the entry is marked %q, and the section "+
				"%q does not link it; add a list item with [%s](#%s)", e.where(), fixMark,
				fixedSection, e.id, e.id))
		case isListed && !linked[e.id]:
			problems = append(problems, fmt.Sprintf("%s: the section %q does not link the entry, "+
				"which listedWithoutMark in tools/cmd/apisurface lists: %s; link it, or remove "+
				"it from listedWithoutMark", e.where(), fixedSection, reason))
		}
	}
	return problems
}

// match returns the ids of the items that a pattern matches, and a problem when the pattern is
// malformed or matches nothing.
func match(items []item, pattern string) ([]string, string) {
	if !validPattern(pattern) {
		return nil, "is not a valid pattern"
	}
	var ids []string
	for _, it := range items {
		if ok, _ := path.Match(pattern, it.id); ok {
			ids = append(ids, it.id)
		}
	}
	if len(ids) == 0 {
		return nil, "matches nothing in " + surfaceFile
	}
	return ids, ""
}

// checkV2 returns the problems with an entry's v2: line. covers says whether its v1: lines name
// v1 items: then the line names a v2 declaration, or gives a reason from exclusionReasons.
func checkV2(e entry, covers bool, v2 v2Surface) []string {
	if len(e.v2) != 1 || e.v2[0] == "" {
		return nil
	}
	value := e.v2[0]
	where := fmt.Sprintf("%s:%d (%s)", migrationFile, e.v2Line, e.id)
	var problems []string
	if ref := reference(value); ref != "" {
		switch {
		case ref == unclosed:
			problems = append(problems, fmt.Sprintf("%s: the v2: line %q %s", where, value,
				unclosedProblem))
		case !v2.names[ref]:
			problems = append(problems, fmt.Sprintf("%s: the v2: line names `%s`, which v2 "+
				"does not export; name a declaration of the permit or models package, such as "+
				"`permit.Client` or `permit.UsersService.Get`", where, ref))
		}
		return problems
	}
	if covers && !isExclusionReason(value) {
		problems = append(problems, fmt.Sprintf("%s: the v2: line %q is not a v2 declaration "+
			"in backquotes, optionally after \"replaced by \", and not a reason from the "+
			"vocabulary: %s", where, value, vocabulary()))
	}
	return problems
}

// isExclusionReason reports whether s is "not exposed in v2", alone or with a reason from
// exclusionReasons.
func isExclusionReason(s string) bool {
	if s == notExposed {
		return true
	}
	reason, ok := strings.CutPrefix(s, notExposed+": ")
	return ok && slices.Contains(exclusionReasons, reason)
}

// vocabulary returns the reasons a v2: line may give, for a message.
func vocabulary() string {
	reasons := []string{strconv.Quote(notExposed)}
	for _, r := range exclusionReasons {
		reasons = append(reasons, strconv.Quote(notExposed+": "+r))
	}
	return strings.Join(reasons, ", ")
}

// sameNameInV2 reports whether an item is a models type of v1 that v2's models package exports
// under the same name, which covers it.
func sameNameInV2(it item, v2 v2Surface) bool {
	return it.block == blockModels && v2.models[it.id]
}

// checkCovered returns the problem with how an item is covered: by nothing, or by more than one
// entry or pending line. A pending line may match a models type that v2 exports under the same
// name, as long as it matches another item too; checkMigration reports one that does not.
func checkCovered(it item, cov coverage, v2 v2Surface) []string {
	entries, pendingLines := cov.entries[it.id], cov.pending[it.id]
	var places []string
	for _, e := range entries {
		places = append(places, e.where())
	}
	for _, p := range pendingLines {
		places = append(places, p.where())
	}
	switch {
	case len(places) > 1:
		return []string{fmt.Sprintf("%s is listed %d times: %s; keep it in one entry or one "+
			"pending line", it.id, len(places), strings.Join(places, ", "))}
	case len(places) == 0 && !sameNameInV2(it, v2):
		return []string{fmt.Sprintf("%s (%s) is in no entry of %s and in no line of %s; write "+
			"its entry, or add a pending line with the area that will", it.id, it.details,
			migrationFile, pendingFile)}
	}
	return nil
}

// checkIDs returns the problems with ids.txt: an entry ID that it does not list, and an ID it
// lists that no entry has.
func checkIDs(entries []entry, ids []string) []string {
	var problems []string
	listed := map[string]bool{}
	for _, id := range ids {
		listed[id] = true
	}
	inGuide := map[string]bool{}
	for _, e := range entries {
		inGuide[e.id] = true
		if !listed[e.id] {
			problems = append(problems, fmt.Sprintf("%s: %s is not in %s; add it at the end",
				e.where(), e.id, idsFile))
		}
	}
	for _, id := range ids {
		if !inGuide[id] {
			problems = append(problems, fmt.Sprintf("%s lists %s, which no entry of %s has; an "+
				"entry ID is never removed or renumbered, so restore the entry", idsFile, id,
				migrationFile))
		}
	}
	return problems
}

// appendix returns the lines of the models appendix: a table of each models type of v1 that an
// entry or a v2 type of the same name covers, and the number of types that are not mapped yet.
func appendix(items []item, cov coverage, v2 v2Surface) []string {
	lines := []string{
		appendixBegin,
		"<!-- apisurface -mode migration -write writes this table; do not edit it. -->", "",
	}
	var rows []string
	total, unmapped := 0, 0
	for _, it := range items {
		if it.block != blockModels {
			continue
		}
		total++
		switch entries := cov.entries[it.id]; {
		case len(entries) > 0:
			e := entries[0]
			rows = append(rows, fmt.Sprintf("| `%s` | %s ([%s](#%s)) |", it.id,
				strings.ReplaceAll(e.v2[0], "|", `\|`), e.id, e.id))
		case sameNameInV2(it, v2):
			rows = append(rows, fmt.Sprintf("| `%s` | `%s` |", it.id, it.id))
		default:
			unmapped++
		}
	}
	if len(rows) > 0 {
		lines = append(lines, "| v1 | v2 |", "| --- | --- |")
		lines = append(lines, rows...)
		lines = append(lines, "")
	}
	if unmapped > 0 {
		lines = append(lines, fmt.Sprintf("Not mapped yet: %d of the %d types of v1's "+
			"pkg/models.", unmapped, total), "")
	}
	return append(lines, appendixEnd)
}

// withAppendix returns MIGRATION.md with its models appendix replaced by lines, and true, or
// nil and false when the appendix holds lines already. The guide must have its appendix
// markers.
func (g *guide) withAppendix(lines []string) ([]byte, bool) {
	old := g.lines[g.beginLine-1 : g.endLine]
	if slices.Equal(old, lines) {
		return nil, false
	}
	out := slices.Concat(g.lines[:g.beginLine-1], lines, g.lines[g.endLine:])
	return []byte(strings.Join(out, "\n")), true
}
