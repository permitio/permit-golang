package main

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// fixtureAppendix is the models appendix of fixtureGuide.
const fixtureAppendix = "<!-- apisurface:models:begin -->\n" +
	"<!-- apisurface -mode migration -write writes this table; do not edit it. -->\n" +
	"\n" +
	"| v1 | v2 |\n" +
	"| --- | --- |\n" +
	"| `models.NullableUser` | replaced by `permit.UsersService` ([M-models-1](#M-models-1)) |\n" +
	"| `models.User` | `models.User` |\n" +
	"\n" +
	"Not mapped yet: 1 of the 3 types of v1's pkg/models.\n" +
	"\n" +
	"<!-- apisurface:models:end -->"

// fixtureGuide is a MIGRATION.md that passes with the other fixture files.
const fixtureGuide = `# Migrating

## Staying on v1

Pin v1.

## Compatibility

<a id="M-module-1"></a>
### M-module-1: The module path ends in /v2

- v1: the module path example.com/sdk
- v2: the module path ` + "`example.com/sdk/v2`" + `

Prose about the module.

## API

<a id="M-users-1"></a>
### M-users-1: Users.Create moves to UsersService.Create

- v1: ` + "`api.Users.Create`" + `, now a method of the users service
- v2: ` + "`permit.UsersService.Create`" + `, which returns the user
- Bug fix: could never work before

<a id="M-members-1"></a>
### M-members-1: Members are not in v2

- v1: ` + "`openapi.MembersApiService.*`" + `
- v2: not exposed in v2: account administration

<a id="M-models-1"></a>
### M-models-1: Nullable types

- v1: ` + "`models.Nullable*`" + `
- v2: replaced by ` + "`permit.UsersService`" + `

## Wire behaviour

No entries yet.

## Appendix

` + fixtureAppendix + "\n"

// fixtureMigration returns the files of a repository that migration mode passes: the v1
// surface, MIGRATION.md, pending.txt, ids.txt and the v2 packages.
func fixtureMigration() map[string]string {
	return map[string]string{
		surfaceFile: "# The surface.\n" +
			"a api.Users type\na api.Users.Create method\na api.Users.Delete method\n" +
			"a config.Old const\n" +
			"b openapi.AuthApiService.Login POST /v2/auth/login not-in-spec\n" +
			"b openapi.MembersApiService.ListMembers GET /v2/members in-spec\n" +
			"c models.NullableUser type\nc models.Role type\nc models.User type\n",
		migrationFile: fixtureGuide,
		pendingFile: "# Pending.\n\napi.Users users\napi.Users.Delete users\nconfig.* client\n" +
			"openapi.AuthApiService.Login auth\nmodels.Role models\n",
		idsFile:  "# IDs.\nM-module-1\nM-users-1\nM-members-1\nM-models-1\n",
		"go.mod": "module example.com/sdk/v2\n\ngo 1.26.0\n",
		v2SurfaceFile: "# The surface.\nmodels: type User struct\n" +
			"models: type User struct, Key string\npermit: const Version = \"2.0.0\"\n" +
			"permit: method (*UsersService) Create()\npermit: type UsersService struct\n",
	}
}

// migrationRepo writes the fixture with the changes, where an empty content removes a file, into
// a temporary directory and returns it.
func migrationRepo(t *testing.T, changes map[string]string) string {
	t.Helper()
	files := fixtureMigration()
	maps.Copy(files, changes)
	maps.DeleteFunc(files, func(_, content string) bool { return content == "" })
	dir := t.TempDir()
	writeFiles(t, dir, files)
	return dir
}

// replaced returns s with the first old replaced by with, and fails the test when s does not
// hold old.
func replaced(t *testing.T, s, old, with string) string {
	t.Helper()
	if !strings.Contains(s, old) {
		t.Fatalf("the fixture does not hold %q", old)
	}
	return strings.Replace(s, old, with, 1)
}

func TestMigrationPasses(t *testing.T) {
	dir := migrationRepo(t, nil)
	code, out := runTool(t, system{}, "-mode", "migration", "-root", dir)
	wantRun(t, code, out, exitPassed, "v1 surface: 4 identifiers (a), 2 operations (b), 3 "+
		"models types (c); 3 in entries, 5 pending, 1 by a v2 models type of the same name; 4 "+
		"entries, 5 pending lines\n", "PASS: each of the 9 v1 items is covered once, and every "+
		"entry is well formed\n")
	if got := lastLine(out); !strings.HasPrefix(got, "PASS: ") {
		t.Errorf("the last line is %q, want the verdict", got)
	}
}

func TestMigrationFails(t *testing.T) {
	guide := fixtureGuide
	pendingLines := fixtureMigration()[pendingFile]
	ids := fixtureMigration()[idsFile]
	for name, tc := range map[string]struct {
		changes map[string]string
		want    []string
	}{
		"an identifier removed from pending without an entry": {
			changes: map[string]string{
				pendingFile: replaced(t, pendingLines, "api.Users.Delete users\n", ""),
			},
			want: []string{
				"- api.Users.Delete (method) is in no entry of MIGRATION.md and in no line of " +
					"tools/migration/pending.txt; write its entry, or add a pending line",
				"; 3 in entries, 4 pending, 1 by a v2 models type of the same name, 1 covered " +
					"by nothing; 4 entries, 4 pending lines\n",
			},
		},
		"an identifier in an entry and a pending line": {
			changes: map[string]string{pendingFile: pendingLines + "api.Users.Create users\n"},
			want: []string{"- api.Users.Create is listed 2 times: MIGRATION.md:19 (M-users-1), " +
				"tools/migration/pending.txt:8; keep it in one entry or one pending line"},
		},
		"an identifier in two pending lines": {
			changes: map[string]string{pendingFile: pendingLines + "config.Old client\n"},
			want: []string{"- config.Old is listed 2 times: tools/migration/pending.txt:5, " +
				"tools/migration/pending.txt:8"},
		},
		"an identifier in two entries": {
			changes: map[string]string{migrationFile: replaced(t, guide,
				"- v1: `models.Nullable*`\n",
				"- v1: `models.Nullable*`\n- v1: `models.NullableUser`\n")},
			want: []string{"- models.NullableUser is listed 2 times: MIGRATION.md:32 " +
				"(M-models-1), MIGRATION.md:32 (M-models-1)"},
		},
		"a pending line that matches nothing": {
			changes: map[string]string{pendingFile: pendingLines + "api.Gone users\n"},
			want: []string{"- tools/migration/pending.txt:8: api.Gone matches nothing in " +
				"tools/migration/v1-surface.txt; remove the line"},
		},
		"a v1: reference that matches nothing": {
			changes: map[string]string{migrationFile: replaced(t, guide, "`api.Users.Create`",
				"`api.Users.Gone`")},
			want: []string{
				"- MIGRATION.md:19 (M-users-1): the v1: line `api.Users.Gone` matches " +
					"nothing in tools/migration/v1-surface.txt",
				"- api.Users.Create (method) is in no entry",
			},
		},
		"an exclusion reason that exposes secrets": {
			changes: map[string]string{migrationFile: replaced(t, guide,
				"- v2: not exposed in v2: account administration",
				"- v2: not exposed in v2: exposes secrets")},
			want: []string{
				"- MIGRATION.md:30: the entry M-members-1 contains \"secret\"; MIGRATION.md is " +
					"public, so an entry must not describe a weakness of the API",
				"- MIGRATION.md:30: the entry M-members-1 contains \"exposes\"",
				"- MIGRATION.md:30 (M-members-1): the v2: line \"not exposed in v2: exposes " +
					"secrets\" is not a v2 declaration in backquotes, optionally after " +
					"\"replaced by \", and not a reason from the vocabulary: \"not exposed in " +
					"v2\", \"not exposed in v2: account administration\", \"not exposed in v2: " +
					"dashboard or CLI helper\", \"not exposed in v2: Elements back-end\", \"not " +
					"exposed in v2: operator route\", \"not exposed in v2: deprecated " +
					"upstream\", \"not exposed in v2: EAP\"",
			},
		},
		"a forbidden word in an entry's heading and body": {
			changes: map[string]string{migrationFile: replaced(t, replaced(t, guide,
				"### M-members-1: Members are not in v2", "### M-members-1: Member secrets"),
				"Prose about the module.", "Prose about the module, which exposes a key.")},
			want: []string{
				"- MIGRATION.md:15: the entry M-module-1 contains \"exposes\"; MIGRATION.md is " +
					"public, so an entry must not describe a weakness of the API",
				"- MIGRATION.md:27: the entry M-members-1 contains \"secret\"",
			},
		},
		"a forbidden word on a v1: line": {
			changes: map[string]string{migrationFile: replaced(t, guide,
				"now a method of the users service", "which Exposes the key")},
			want: []string{"- MIGRATION.md:22: the entry M-users-1 contains \"exposes\""},
		},
		"a reason outside the vocabulary": {
			changes: map[string]string{migrationFile: replaced(t, guide,
				"not exposed in v2: account administration", "not exposed in v2: internal only")},
			want: []string{"the v2: line \"not exposed in v2: internal only\" is not a v2 " +
				"declaration in backquotes"},
		},
		"a reason with text after it": {
			changes: map[string]string{migrationFile: replaced(t, guide,
				"not exposed in v2: account administration",
				"not exposed in v2: account administration, for now")},
			want: []string{"is not a v2 declaration in backquotes, optionally after \"replaced " +
				"by \", and not a reason from the vocabulary"},
		},
		"prose on the v2: line of an entry that covers identifiers": {
			changes: map[string]string{migrationFile: replaced(t, guide,
				"- v2: not exposed in v2: account administration", "- v2: use the dashboard")},
			want: []string{"the v2: line \"use the dashboard\" is not a v2 declaration"},
		},
		"a vulnerability in a replacement's prose": {
			changes: map[string]string{migrationFile: replaced(t, guide,
				", which returns the user", ", which fixes a Vulnerability")},
			want: []string{"- MIGRATION.md:23: the entry M-users-1 contains \"vulnerab\""},
		},
		"a forbidden word in a prose entry": {
			changes: map[string]string{migrationFile: replaced(t, guide,
				"- v2: the module path", "- v2: the module path, whose SECRET")},
			want: []string{"- MIGRATION.md:13: the entry M-module-1 contains \"secret\""},
		},
		"a v2 declaration that v2 does not export": {
			changes: map[string]string{migrationFile: replaced(t, guide,
				"`permit.UsersService.Create`", "`permit.UsersService.Gone`")},
			want: []string{"- MIGRATION.md:23 (M-users-1): the v2: line names " +
				"`permit.UsersService.Gone`, which v2 does not export; name a declaration of the " +
				"permit or models package"},
		},
		"a replacement that v2 does not export": {
			changes: map[string]string{migrationFile: replaced(t, guide,
				"replaced by `permit.UsersService`", "replaced by `models.Gone`")},
			want: []string{"the v2: line names `models.Gone`, which v2 does not export"},
		},
		"an unexported v2 declaration in a prose entry": {
			changes: map[string]string{migrationFile: replaced(t, guide,
				"- v2: the module path `example.com/sdk/v2`", "- v2: `permit.Client`")},
			want: []string{"- MIGRATION.md:13 (M-module-1): the v2: line names `permit.Client`"},
		},
		"an entry ID missing from ids.txt": {
			changes: map[string]string{idsFile: replaced(t, ids, "M-members-1\n", "")},
			want: []string{"- MIGRATION.md:26 (M-members-1): M-members-1 is not in " +
				"tools/migration/ids.txt; add it at the end"},
		},
		"an entry renumbered in MIGRATION.md": {
			changes: map[string]string{migrationFile: strings.ReplaceAll(guide, "M-members-1",
				"M-members-2")},
			want: []string{"- tools/migration/ids.txt lists M-members-1, which no entry of " +
				"MIGRATION.md has; an entry ID is never removed or renumbered, so restore the " +
				"entry", "- MIGRATION.md:26 (M-members-2): M-members-2 is not in"},
		},
		"a malformed entry ID": {
			changes: map[string]string{migrationFile: strings.ReplaceAll(guide, "M-members-1",
				"M-Members-1")},
			want: []string{"- MIGRATION.md:26: the entry ID \"M-Members-1\" is not " +
				"M-<area>-<n>, with an area in lower case, such as M-users-1"},
		},
		"an entry ID without a number": {
			changes: map[string]string{migrationFile: strings.ReplaceAll(guide, "M-members-1",
				"M-members")},
			want: []string{"the entry ID \"M-members\" is not M-<area>-<n>"},
		},
		"an entry ID numbered from zero": {
			changes: map[string]string{migrationFile: strings.ReplaceAll(guide, "M-members-1",
				"M-members-0"), idsFile: replaced(t, ids, "M-members-1", "M-members-0")},
			want: []string{
				"the entry ID \"M-members-0\" is not M-<area>-<n>",
				"tools/migration/ids.txt:4: \"M-members-0\" is not an entry ID",
			},
		},
		"an entry outside the bucket sections": {
			changes: map[string]string{migrationFile: replaced(t, guide, "Pin v1.\n",
				"Pin v1.\n\n<a id=\"M-pin-1\"></a>\n### M-pin-1: Pin\n\n- v1: x\n- v2: y\n")},
			want: []string{
				"- MIGRATION.md:7: the entry M-pin-1 is outside the bucket " +
					"sections; put it under ## Compatibility, ## API or ## Wire behaviour",
				"- MIGRATION.md:10: \"- v1: x\" is outside an entry",
			},
		},
		"a heading without its anchor": {
			changes: map[string]string{migrationFile: replaced(t, guide,
				"<a id=\"M-members-1\"></a>\n", "")},
			want: []string{
				"- MIGRATION.md:26: the heading \"### M-members-1: Members are not " +
					"in v2\" has no anchor before it; start the entry with <a id=\"<ID>\"></a>",
				"- MIGRATION.md:28: \"- v1: `openapi.MembersApiService.*`\" is outside an entry; " +
					"put it under an entry's heading",
				"tools/migration/ids.txt lists M-members-1, which no entry",
			},
		},
		"an anchor whose heading names another ID": {
			changes: map[string]string{migrationFile: replaced(t, guide,
				"### M-members-1:", "### M-members-2:")},
			want: []string{"- MIGRATION.md:26: the anchor of M-members-1 is not followed by its " +
				"heading, \"### M-members-1: \" and a title"},
		},
		"an anchor whose heading has no title": {
			changes: map[string]string{migrationFile: replaced(t, guide,
				"### M-members-1: Members are not in v2", "### M-members-1: ")},
			want: []string{"the anchor of M-members-1 is not followed by its heading"},
		},
		"an anchor at the end of the file": {
			changes: map[string]string{migrationFile: guide + "## API\n\n<a id=\"M-end-1\"></a>\n"},
			want:    []string{"the anchor of M-end-1 is not followed by its heading"},
		},
		"a malformed anchor": {
			changes: map[string]string{migrationFile: replaced(t, guide,
				"<a id=\"M-members-1\"></a>", "<a id=\"M-members-1\">")},
			want: []string{"- MIGRATION.md:26: \"<a id=\\\"M-members-1\\\">\" is not an entry " +
				"anchor, <a id=\"M-<area>-<n>\"></a>, alone on its line"},
		},
		"an anchor with another prefix": {
			changes: map[string]string{migrationFile: replaced(t, guide,
				"Prose about the module.", "<a id=\"other\"></a>")},
			want: []string{"the entry ID \"other\" is not M-<area>-<n>"},
		},
		"an entry ID used twice": {
			changes: map[string]string{migrationFile: replaced(t, guide, "M-models-1\"></a>\n"+
				"### M-models-1:", "M-users-1\"></a>\n### M-users-1:")},
			want: []string{"- MIGRATION.md:32: the entry ID M-users-1 is used on line 19 too; " +
				"give each entry its own ID"},
		},
		"an entry without a v1: line": {
			changes: map[string]string{migrationFile: replaced(t, guide,
				"- v1: the module path example.com/sdk\n", "")},
			want: []string{"- MIGRATION.md:9 (M-module-1): the entry has no v1: line; name what " +
				"it changes in v1"},
		},
		"an empty v1: line": {
			changes: map[string]string{migrationFile: replaced(t, guide,
				"- v1: the module path example.com/sdk\n", "- v1: the module\n- v1:\n")},
			want: []string{"- MIGRATION.md:9 (M-module-1): a v1: line is empty"},
		},
		"an entry with two v2: lines": {
			changes: map[string]string{migrationFile: replaced(t, guide,
				"- v2: not exposed in v2: account administration\n",
				"- v2: not exposed in v2: account administration\n- v2: not exposed in v2\n")},
			want: []string{"- MIGRATION.md:26 (M-members-1): the entry needs exactly one v2: " +
				"line, not empty, and has 2"},
		},
		"an entry without a v2: line": {
			changes: map[string]string{migrationFile: replaced(t, guide,
				"- v2: not exposed in v2: account administration\n", "")},
			want: []string{"(M-members-1): the entry needs exactly one v2: line, not empty, and " +
				"has 0"},
		},
		"an empty v2: line": {
			changes: map[string]string{migrationFile: replaced(t, guide,
				"- v2: not exposed in v2: account administration\n", "- v2: \n")},
			want: []string{"(M-members-1): the entry needs exactly one v2: line, not empty, and " +
				"has 1"},
		},
		"a malformed bug fix mark": {
			changes: map[string]string{migrationFile: replaced(t, guide,
				"- Bug fix: could never work before", "- Bug fix: yes")},
			want: []string{"- MIGRATION.md:19 (M-users-1): line 24, \"- Bug fix: yes\", is not " +
				"the bug fix mark, which reads exactly \"- Bug fix: could never work before\", " +
				"once"},
		},
		"a bug fix mark given twice": {
			changes: map[string]string{migrationFile: replaced(t, guide,
				"- Bug fix: could never work before", "- Bug fix: could never work before\n"+
					"- Bug fix: could never work before")},
			want: []string{"line 25, \"- Bug fix: could never work before\", is not the bug fix " +
				"mark"},
		},
		"a pending models type that v2 exports": {
			changes: map[string]string{pendingFile: pendingLines + "models.User models\n"},
			want: []string{"- tools/migration/pending.txt:8: v2 exports models.User, a models " +
				"type of the same name, which needs no pending line; remove the line"},
		},
		"a pending pattern that matches only models types that v2 exports": {
			changes: map[string]string{pendingFile: pendingLines + "models.U* models\n"},
			want: []string{"- tools/migration/pending.txt:8: models.U* matches only models types " +
				"that v2 exports under the same name, which need no pending line; remove the line"},
		},
		"a field line under a section that is not a bucket, after an entry": {
			changes: map[string]string{migrationFile: replaced(t, guide,
				"- v2: replaced by `permit.UsersService`\n",
				"- v2: replaced by `permit.UsersService`\n\n## Notes\n\n- v1: notes\n")},
			want: []string{"- MIGRATION.md:40: \"- v1: notes\" is outside an entry; put it under " +
				"an entry's heading"},
		},
		"a field line right after the appendix": {
			changes: map[string]string{migrationFile: replaced(t, guide,
				"- v2: replaced by `permit.UsersService`\n",
				"- v2: replaced by `permit.UsersService`\n\n"+fixtureAppendix+"\n- v1: notes\n")},
			want: []string{"\"- v1: notes\" is outside an entry"},
		},
		"a field line right after the start of the appendix": {
			changes: map[string]string{migrationFile: replaced(t, guide,
				"- v2: replaced by `permit.UsersService`\n",
				"- v2: replaced by `permit.UsersService`\n\n"+appendixBegin+"\n- v1: notes\n")},
			want: []string{"- MIGRATION.md:39: \"- v1: notes\" is outside an entry"},
		},
		"a models type that v2 no longer exports": {
			changes: map[string]string{v2SurfaceFile: "permit: type UsersService struct\n" +
				"permit: method (*UsersService) Create()\n"},
			want: []string{
				"- models.User (type) is in no entry",
				"- the models appendix of MIGRATION.md is not up to date; run apisurface -mode " +
					"migration -write",
			},
		},
		"an edited appendix": {
			changes: map[string]string{migrationFile: replaced(t, guide, "Not mapped yet: 1",
				"Not mapped yet: 2")},
			want: []string{"- the models appendix of MIGRATION.md is not up to date"},
		},
		"no appendix": {
			changes: map[string]string{migrationFile: replaced(t, guide, fixtureAppendix, "")},
			want: []string{"- MIGRATION.md has no models appendix: add a line " + appendixBegin +
				" and, after it, a line " + appendixEnd + ", then run apisurface -mode " +
				"migration -write"},
		},
		"an appendix without its end": {
			changes: map[string]string{migrationFile: replaced(t, guide, appendixEnd, "")},
			want:    []string{"- MIGRATION.md has no models appendix"},
		},
		"two appendixes": {
			changes: map[string]string{migrationFile: guide + fixtureAppendix + "\n"},
			want: []string{
				"- MIGRATION.md:55: a second <!-- apisurface:models:begin -->; keep one models " +
					"appendix",
				"- MIGRATION.md:65: a second <!-- apisurface:models:end -->; keep one models " +
					"appendix",
			},
		},
		"two appendixes, the first on the first line": {
			changes: map[string]string{migrationFile: fixtureAppendix + "\n" + guide},
			want: []string{
				"- MIGRATION.md:55: a second <!-- apisurface:models:begin -->",
				"- MIGRATION.md:65: a second <!-- apisurface:models:end -->",
			},
		},
		"a stale appendix on the first line": {
			changes: map[string]string{migrationFile: appendixBegin + "\nold\n" + appendixEnd +
				"\n" + replaced(t, guide, fixtureAppendix, "")},
			want: []string{"- the models appendix of MIGRATION.md is not up to date"},
		},
		"an appendix end before its begin": {
			changes: map[string]string{migrationFile: replaced(t, guide, "## Appendix\n",
				"## Appendix\n"+appendixEnd+"\n")},
			want: []string{appendixEnd + " before " + appendixBegin},
		},
		"a pending line without an area": {
			changes: map[string]string{pendingFile: pendingLines + "api.Users\n"},
			want: []string{"- tools/migration/pending.txt:8: \"api.Users\" is not <id or " +
				"pattern> <area>"},
		},
		"a pending line with an area in upper case": {
			changes: map[string]string{pendingFile: replaced(t, pendingLines, "config.* client",
				"config.* Client")},
			want: []string{
				"- tools/migration/pending.txt:5: the area \"Client\" is not " +
					"lower-case words joined by hyphens, such as users or api-keys",
				"- config.Old (const) is in no entry",
			},
		},
		"a malformed pending pattern": {
			changes: map[string]string{pendingFile: pendingLines + "api.[ users\n"},
			want:    []string{"- tools/migration/pending.txt:8: \"api.[\" is not a valid pattern"},
		},
		"an unclosed backquote on a v1: line": {
			changes: map[string]string{migrationFile: replaced(t, guide, "`api.Users.Create`,",
				"`api.Users.Create,")},
			want: []string{"- MIGRATION.md:19 (M-users-1): the v1: line \"`api.Users.Create, now " +
				"a method of the users service\" starts with a backquote that does not enclose a " +
				"reference; name an id in backquotes, or write prose that does not start with a " +
				"backquote", "- api.Users.Create (method) is in no entry"},
		},
		"an empty reference on a v2: line": {
			changes: map[string]string{migrationFile: replaced(t, guide,
				"- v2: `permit.UsersService.Create`", "- v2: ``")},
			want: []string{"- MIGRATION.md:23 (M-users-1): the v2: line \"``, which returns the " +
				"user\" starts with a backquote that does not enclose a reference"},
		},
		"a malformed v1: pattern": {
			changes: map[string]string{migrationFile: replaced(t, guide, "`models.Nullable*`",
				"`models.[`")},
			want: []string{"the v1: line `models.[` is not a valid pattern"},
		},
		"a malformed ID in ids.txt": {
			changes: map[string]string{idsFile: ids + "users-1\n"},
			want: []string{"- tools/migration/ids.txt:6: \"users-1\" is not an entry ID, " +
				"M-<area>-<n>"},
		},
		"an ID listed twice in ids.txt": {
			changes: map[string]string{idsFile: ids + "M-users-1\n"},
			want:    []string{"- tools/migration/ids.txt:6: M-users-1 is listed twice"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := migrationRepo(t, tc.changes)
			code, out := runTool(t, system{}, "-mode", "migration", "-root", dir)
			wantRun(t, code, out, exitFailed, tc.want...)
			if got := lastLine(out); !strings.HasPrefix(got, "FAIL: ") {
				t.Errorf("the last line is %q, want the verdict", got)
			}
		})
	}
}

func TestMigrationAccepts(t *testing.T) {
	pendingLines := fixtureMigration()[pendingFile]
	for name, tc := range map[string]struct {
		changes map[string]string
		want    string
	}{
		"blank lines between an anchor and its heading": {
			changes: map[string]string{migrationFile: replaced(t, fixtureGuide,
				"<a id=\"M-members-1\"></a>\n", "<a id=\"M-members-1\"></a>\n\n \n")},
		},
		"the appendix on the first line": {
			changes: map[string]string{migrationFile: fixtureAppendix + "\n" +
				replaced(t, fixtureGuide, fixtureAppendix, "")},
		},
		"a pending pattern that matches a models type that v2 exports too": {
			changes: map[string]string{pendingFile: replaced(t, pendingLines, "models.Role models",
				"models.[^N]* models")},
			want: "; 3 in entries, 5 pending, 1 by a v2 models type of the same name; 4 entries, " +
				"5 pending lines\n",
		},
		"a forbidden word outside the entries": {
			changes: map[string]string{migrationFile: replaced(t, fixtureGuide, "No entries yet.",
				"No entries yet; no secrets.")},
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := migrationRepo(t, tc.changes)
			code, out := runTool(t, system{}, "-mode", "migration", "-root", dir)
			wantRun(t, code, out, exitPassed, tc.want,
				"PASS: each of the 9 v1 items is covered once")
		})
	}
}

func TestMigrationDidNotRun(t *testing.T) {
	for name, tc := range map[string]struct {
		changes map[string]string
		want    string
	}{
		"no v1 surface":  {map[string]string{surfaceFile: ""}, "v1-surface.txt: no such file"},
		"no guide":       {map[string]string{migrationFile: ""}, "MIGRATION.md: no such file"},
		"no pending.txt": {map[string]string{pendingFile: ""}, "pending.txt: no such file"},
		"no ids.txt":     {map[string]string{idsFile: ""}, "ids.txt: no such file"},
		"a malformed v1 surface": {
			changes: map[string]string{surfaceFile: "a api.Users\n"},
			want: "DID NOT RUN: tools/migration/v1-surface.txt: line 1 is not <block> <id> " +
				"<details>",
		},
		"a v1 surface without models": {
			changes: map[string]string{surfaceFile: "a api.Users type\nb openapi.A.B GET /x " +
				"in-spec\n"},
			want: "DID NOT RUN: tools/migration/v1-surface.txt: block c of the v1 surface is empty",
		},
		"a v2 surface line apisurface does not know": {
			changes: map[string]string{v2SurfaceFile: "permit: type Client struct\n" +
				"permit: interface Client\n"},
			want: "DID NOT RUN: reading v2's surface: testdata/api-surface.txt:2: \"permit: " +
				"interface Client\" is not <package>: <declaration> as internal/surface writes it",
		},
		"no v2 surface": {
			changes: map[string]string{v2SurfaceFile: ""},
			want:    "DID NOT RUN: reading v2's surface: open ",
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := migrationRepo(t, tc.changes)
			code, out := runTool(t, system{}, "-mode", "migration", "-root", dir)
			wantRun(t, code, out, exitDidNotRun, tc.want)
			if got := lastLine(out); !strings.HasPrefix(got, "DID NOT RUN: ") {
				t.Errorf("the last line is %q, want the verdict", got)
			}
		})
	}
}

func TestMigrationWrite(t *testing.T) {
	stale := replaced(t, fixtureGuide, fixtureAppendix, appendixBegin+"\nold\n"+appendixEnd)
	dir := migrationRepo(t, map[string]string{migrationFile: stale})
	code, out := runTool(t, system{}, "-mode", "migration", "-root", dir, "-write")
	wantRun(t, code, out, exitPassed, "wrote the models appendix of MIGRATION.md\n", "PASS: ")
	got, err := os.ReadFile(filepath.Join(dir, migrationFile))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != fixtureGuide {
		t.Errorf("-write wrote\n%s\nwant\n%s", got, fixtureGuide)
	}
	code, out = runTool(t, system{}, "-mode", "migration", "-root", dir, "-write")
	wantRun(t, code, out, exitPassed)
	if strings.Contains(out, "wrote") {
		t.Errorf("-write rewrote an appendix that was up to date:\n%s", out)
	}

	// An appendix on the first line.
	rest := replaced(t, fixtureGuide, fixtureAppendix, "")
	dir = migrationRepo(t, map[string]string{
		migrationFile: appendixBegin + "\nold\n" + appendixEnd + "\n" + rest,
	})
	code, out = runTool(t, system{}, "-mode", "migration", "-root", dir, "-write")
	wantRun(t, code, out, exitPassed, "wrote the models appendix of MIGRATION.md\n")
	got, err = os.ReadFile(filepath.Join(dir, migrationFile))
	if err != nil {
		t.Fatal(err)
	}
	if want := fixtureAppendix + "\n" + rest; string(got) != want {
		t.Errorf("-write wrote\n%s\nwant\n%s", got, want)
	}

	// -write fixes the appendix even when another problem fails the check.
	dir = migrationRepo(t, map[string]string{
		migrationFile: stale, pendingFile: fixtureMigration()[pendingFile] + "api.Gone x\n",
	})
	code, out = runTool(t, system{}, "-mode", "migration", "-root", dir, "-write")
	wantRun(t, code, out, exitFailed, "wrote the models appendix", "api.Gone matches nothing")
	if strings.Contains(out, "not up to date") {
		t.Errorf("-write reported the appendix it wrote as stale:\n%s", out)
	}
}

func TestAppendix(t *testing.T) {
	items := []item{
		{block: blockIdentifiers, id: "api.Users", details: "type"},
		{block: blockModels, id: "models.A", details: "type"},
		{block: blockModels, id: "models.B", details: "type"},
		{block: blockModels, id: "models.C", details: "type"},
	}
	cov := coverage{entries: map[string][]entry{
		"models.A": {{id: "M-models-2", v2: []string{"`models.X` | or `models.Y`"}}},
		"models.C": {{id: "M-models-3", v2: []string{notExposed}}},
	}}
	v2 := v2Surface{models: map[string]bool{"models.A": true, "models.B": true}}
	want := []string{
		appendixBegin,
		"<!-- apisurface -mode migration -write writes this table; do not edit it. -->",
		"",
		"| v1 | v2 |",
		"| --- | --- |",
		"| `models.A` | `models.X` \\| or `models.Y` ([M-models-2](#M-models-2)) |",
		"| `models.B` | `models.B` |",
		"| `models.C` | not exposed in v2 ([M-models-3](#M-models-3)) |",
		"",
		appendixEnd,
	}
	if got := appendix(items, cov, v2); !slices.Equal(got, want) {
		t.Errorf("appendix =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	want = []string{
		appendixBegin,
		"<!-- apisurface -mode migration -write writes this table; do not edit it. -->",
		"",
		"Not mapped yet: 3 of the 3 types of v1's pkg/models.",
		"",
		appendixEnd,
	}
	if got := appendix(items, coverage{}, v2Surface{}); !slices.Equal(got, want) {
		t.Errorf("appendix without mappings =\n%s\nwant\n%s", strings.Join(got, "\n"),
			strings.Join(want, "\n"))
	}
}

func TestReference(t *testing.T) {
	for value, want := range map[string]string{
		"`api.Users.Create`":               "api.Users.Create",
		"`api.Users.*`, and more":          "api.Users.*",
		"replaced by `permit.Client`":      "permit.Client",
		"replaced by `permit.Client` soon": "permit.Client",
		"the module `example.com/sdk`":     "",
		"replaced by the client":           "",
		"not exposed in v2":                "",
		"`unterminated":                    "`",
		"``":                               "`",
		"":                                 "",
	} {
		if got := reference(value); got != want {
			t.Errorf("reference(%q) = %q, want %q", value, got, want)
		}
	}
}

func TestIsExclusionReason(t *testing.T) {
	for _, reason := range append([]string{notExposed}, exclusionReasons...) {
		s := reason
		if reason != notExposed {
			s = notExposed + ": " + reason
		}
		if !isExclusionReason(s) {
			t.Errorf("isExclusionReason(%q) = false, want true", s)
		}
	}
	for _, s := range []string{
		"", "account administration", "not exposed in v2:", "not exposed in v2: ",
		"not exposed in v2:account administration", "Not exposed in v2", "not exposed in v2.",
		"not exposed in v2: EAP.", "not exposed in v2: eap",
	} {
		if isExclusionReason(s) {
			t.Errorf("isExclusionReason(%q) = true, want false", s)
		}
	}
}

func TestIDsGrow(t *testing.T) {
	t.Parallel()
	dir := newRepo(t, fixtureMigration())
	base := gitIn(t, dir, "rev-parse", "HEAD")
	addEntry := func(id string) {
		t.Helper()
		files := map[string]string{}
		content, err := os.ReadFile(filepath.Join(dir, migrationFile))
		if err != nil {
			t.Fatal(err)
		}
		files[migrationFile] = replaced(t, string(content), "## Wire behaviour\n",
			"## Wire behaviour\n\n<a id=\""+id+"\"></a>\n### "+id+": A change\n\n- v1: retries\n"+
				"- v2: retries\n\n")
		ids, err := os.ReadFile(filepath.Join(dir, idsFile))
		if err != nil {
			t.Fatal(err)
		}
		files[idsFile] = string(ids) + id + "\n"
		writeFiles(t, dir, files)
	}
	addEntry("M-retry-1")
	commitAll(t, dir, "add M-retry-1")
	code, out := runTool(t, system{}, "-mode", "migration", "-root", dir, "-ids-base", base)
	wantRun(t, code, out, exitPassed)

	// A commit that renumbers an entry drops its old ID.
	for _, name := range []string{migrationFile, idsFile} {
		content, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		writeFiles(t, dir, map[string]string{
			name: strings.ReplaceAll(string(content), "M-retry-1", "M-retry-2"),
		})
	}
	renumbered := commitAll(t, dir, "renumber M-retry-1")
	code, out = runTool(t, system{}, "-mode", "migration", "-root", dir, "-ids-base", base)
	wantRun(t, code, out, exitFailed, "- commit "+renumbered[:12]+" removes M-retry-1 from "+
		"tools/migration/ids.txt; an entry ID is never removed or renumbered: restore it, and "+
		"give a new entry a new ID")
	code, out = runTool(t, system{}, "-mode", "migration", "-root", dir)
	wantRun(t, code, out, exitPassed)

	// The working tree drops an ID of HEAD.
	gitIn(t, dir, "reset", "--quiet", "--hard", base)
	addEntry("M-retry-1")
	commitAll(t, dir, "add M-retry-1")
	writeFiles(t, dir, map[string]string{
		idsFile: fixtureMigration()[idsFile], migrationFile: fixtureGuide,
	})
	code, out = runTool(t, system{}, "-mode", "migration", "-root", dir, "-ids-base", "HEAD")
	wantRun(t, code, out, exitFailed, "- the working tree removes M-retry-1 from "+
		"tools/migration/ids.txt")

	// A history that starts without ids.txt, as the pull request that adds it does.
	empty := newRepo(t, map[string]string{"README.md": "v1\n"})
	first := gitIn(t, empty, "rev-parse", "HEAD")
	writeFiles(t, empty, fixtureMigration())
	commitAll(t, empty, "add the migration files")
	code, out = runTool(t, system{}, "-mode", "migration", "-root", empty, "-ids-base", first)
	wantRun(t, code, out, exitPassed)

	// A range that holds a root commit, which has no ids.txt before it: the base is on a history
	// of its own.
	gitIn(t, empty, "checkout", "--quiet", "--orphan", "other")
	gitIn(t, empty, "rm", "--quiet", "-r", "--cached", ".")
	commitAll(t, empty, "an unrelated history")
	other := gitIn(t, empty, "rev-parse", "HEAD")
	gitIn(t, empty, "checkout", "--quiet", "--force", "main")
	code, out = runTool(t, system{}, "-mode", "migration", "-root", empty, "-ids-base", other)
	wantRun(t, code, out, exitPassed)

	code, out = runTool(t, system{}, "-mode", "migration", "-root", dir, "-ids-base", "nope")
	wantRun(t, code, out, exitDidNotRun, "DID NOT RUN: git rev-list --reverse nope..HEAD")

	// A commit of the range whose ids.txt cannot be read.
	writeFiles(t, dir, map[string]string{idsFile: "bad id\n"})
	bad := commitAll(t, dir, "break ids.txt")
	writeFiles(t, dir, map[string]string{idsFile: fixtureMigration()[idsFile] + "M-retry-1\n"})
	commitAll(t, dir, "fix ids.txt")
	code, out = runTool(t, system{}, "-mode", "migration", "-root", dir, "-ids-base", base)
	wantRun(t, code, out, exitDidNotRun, "DID NOT RUN: "+bad+":tools/migration/ids.txt:1: "+
		"\"bad id\" is not an entry ID")
}
