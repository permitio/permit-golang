package main

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"
)

// guide is what the scanner reads from MIGRATION.md of v2: which entries name each v1
// identifier, which name v1's module, and what the models appendix maps each v1 models type to.
type guide struct {
	// refs are the references of the entries' v1: lines, in the order of the guide.
	refs []guideRef
	// module holds the IDs of the entries with a v1: line that names v1's module path in
	// backquotes: the entries of an import of v1.
	module []string
	// models holds the rows of the models appendix, by the v1 models type in the first column.
	models map[string]modelsRow
}

// guideRef is the reference of a v1: line, an id or a path.Match pattern of ids, and the ID of
// its entry.
type guideRef struct {
	pattern, id string
}

// modelsRow is what a row of the models appendix maps a v1 models type to: the IDs of the
// entries that its second column links, or, when it links none, the v2 type it names when that
// is the same type.
type modelsRow struct {
	ids []string
	v2  string
}

const (
	// appendixBegin and appendixEnd enclose the models appendix of MIGRATION.md.
	appendixBegin = "<!-- apisurface:models:begin -->"
	appendixEnd   = "<!-- apisurface:models:end -->"
)

var (
	// anchorLine is the anchor that starts an entry, alone on its line.
	anchorLine = regexp.MustCompile(`^<a id="(M-[^"]+)"></a>$`)
	// appendixRow is a row of the models appendix: the v1 type, and the second column.
	appendixRow = regexp.MustCompile("^\\| `(models\\.\\w+)` \\| (.*) \\|$")
	// entryLink is a link to an entry, whose ID it captures.
	entryLink = regexp.MustCompile(`\]\(#(M-[^)]+)\)`)
)

// parseGuide reads the guide from text, the content of MIGRATION.md, by the format that the
// repository's migration check (apisurface -mode migration) holds it to, and not by its prose
// or the order of its sections. An entry starts at its anchor, <a id="M-<area>-<n>"></a>, whose
// heading may follow, and ends at the next anchor, section (##) or other heading (###); a v1:
// line of it that starts with a backquote names a reference, the text up to the next backquote.
// It fails when text has no entry, or a reference is not a valid pattern.
func parseGuide(text string) (guide, error) {
	g := guide{models: map[string]modelsRow{}}
	found, n := false, 0
	// id is the ID of the entry that the line is in, or "" outside an entry, and heading says
	// whether the line may be the entry's heading: the anchor and blank lines were all before it.
	id, heading, appendix := "", false, false
	for line := range strings.Lines(text) {
		n++
		line = strings.TrimRight(line, "\r\n")
		mayBeHeading := heading
		if strings.TrimSpace(line) != "" {
			heading = false
		}
		switch {
		case line == appendixBegin:
			id, appendix = "", true
		case line == appendixEnd:
			appendix = false
		case appendix:
			if m := appendixRow.FindStringSubmatch(line); m != nil {
				g.models[m[1]] = newModelsRow(m[1], m[2])
			}
		case anchorLine.MatchString(line):
			id, heading, found = anchorLine.FindStringSubmatch(line)[1], true, true
		case line == "##" || strings.HasPrefix(line, "## "):
			id = ""
		case strings.HasPrefix(line, "### "):
			if !mayBeHeading {
				id = ""
			}
		case id != "" && strings.HasPrefix(line, "- v1:"):
			value := strings.TrimSpace(strings.TrimPrefix(line, "- v1:"))
			if err := g.addV1(id, value); err != nil {
				return guide{}, fmt.Errorf("line %d: %w", n, err)
			}
		}
	}
	if !found {
		return guide{}, errors.New("the guide has no entry, an anchor such as " +
			"<a id=\"M-module-1\"></a> alone on its line; pass MIGRATION.md of v2")
	}
	return g, nil
}

// addV1 adds value, the text of a v1: line of the entry id, to the guide.
func (g *guide) addV1(id, value string) error {
	if strings.Contains(value, "`"+v1Module+"`") && !slices.Contains(g.module, id) {
		g.module = append(g.module, id)
	}
	rest, ok := strings.CutPrefix(value, "`")
	if !ok {
		return nil
	}
	ref, _, ok := strings.Cut(rest, "`")
	if !ok || ref == "" {
		return nil
	}
	if _, err := path.Match(ref, ""); err != nil {
		return fmt.Errorf("the v1: line of %s names %s, which is not a valid pattern: %w", id,
			ref, err)
	}
	g.refs = append(g.refs, guideRef{pattern: ref, id: id})
	return nil
}

// newModelsRow returns what column, the second column of the appendix row of the v1 models type
// typ, maps it to.
func newModelsRow(typ, column string) modelsRow {
	var row modelsRow
	for _, m := range entryLink.FindAllStringSubmatch(column, -1) {
		if !slices.Contains(row.ids, m[1]) {
			row.ids = append(row.ids, m[1])
		}
	}
	if len(row.ids) == 0 && column == "`"+typ+"`" {
		row.v2 = typ
	}
	return row
}

// resolve returns the IDs of the entries whose v1: lines name v1, a v1 identifier, in the
// order of the guide. When none does, v1 has the entries of typ, the models type that it
// resolves through (use.typ), or v1 itself when typ is "": those whose v1: lines name typ, or
// else those that the row of typ in the models appendix links. When that row links none and
// names the same type in v2, resolve returns that v2 type instead. It returns neither when the
// guide maps v1 to nothing.
func (g *guide) resolve(v1, typ string) (ids []string, v2 string) {
	if ids := g.entries(v1); len(ids) > 0 {
		return ids, ""
	}
	if typ == "" {
		typ = v1
	} else if ids := g.entries(typ); len(ids) > 0 {
		return ids, ""
	}
	row := g.models[typ]
	return row.ids, row.v2
}

// entries returns the IDs of the entries with a reference that matches v1, each once.
func (g *guide) entries(v1 string) []string {
	var ids []string
	for _, ref := range g.refs {
		// parseGuide checked that each pattern is valid, so Match returns no error.
		if matched, _ := path.Match(ref.pattern, v1); matched && !slices.Contains(ids, ref.id) {
			ids = append(ids, ref.id)
		}
	}
	return ids
}
