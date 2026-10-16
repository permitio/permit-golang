package main

import (
	"os"
	"slices"
	"strings"
	"testing"
)

// testGuide parses testdata/guide.md.
func testGuide(t *testing.T) guide {
	t.Helper()
	text, err := os.ReadFile("testdata/guide.md")
	if err != nil {
		t.Fatal(err)
	}
	g, err := parseGuide(string(text))
	if err != nil {
		t.Fatalf("parseGuide(testdata/guide.md): %v", err)
	}
	return g
}

func TestResolve(t *testing.T) {
	t.Parallel()
	g := testGuide(t)
	for _, test := range []struct {
		v1, typ string
		ids     []string
		v2      string
	}{
		{v1: "permit.New", ids: []string{"M-client-1"}},
		{v1: "permit.NewPermit", ids: []string{"M-client-1"}},
		{v1: "config.PermitConfig.Build", ids: []string{"M-client-1"}},
		{v1: "config.PermitConfig.WithApiUrl", ids: []string{"M-client-1"}},
		{v1: "config.PermitBuilder"},
		{v1: "permit.Client.SyncUser", ids: []string{"M-users-1"}},
		{v1: "api.Users.SyncUser", ids: []string{"M-users-1", "M-users-2"}},
		{v1: "api.Users.Create", ids: []string{"M-users-2"}},
		{v1: "errors.NotFound", ids: []string{"M-errors-1"}},
		{v1: "errors.PermitError", ids: []string{"M-errors-1"}},
		{v1: "errors.Conflict"},
		{v1: "models.ResourceCreate", ids: []string{"M-models-1"}},
		{v1: "models.ResourceCreate", typ: "models.ResourceCreate", ids: []string{"M-models-1"}},
		{v1: "models.NewResourceCreate", typ: "models.ResourceCreate", ids: []string{"M-models-1"}},
		{v1: "models.TenantCreate", v2: "models.TenantCreate"},
		{v1: "models.NewTenantCreate", typ: "models.TenantCreate", v2: "models.TenantCreate"},
		{v1: "models.UserCreate.SetEmail", typ: "models.UserCreate", v2: "models.UserCreate"},
		{v1: "models.STRING", typ: "models.AttributeType", v2: "models.AttributeType"},
		// The appendix row links the entry that names the type only in its text.
		{v1: "models.ResourceId", ids: []string{"M-models-1"}},
		{v1: "models.ResourceId.Get", typ: "models.ResourceId", ids: []string{"M-models-1"}},
		// Only the type that scan found counts, not the name.
		{v1: "models.NewTenantCreate"},
		{v1: "models.TenantCreate.Key", typ: "models.UserRead"},
		{v1: "models.PtrString"},
		// The appendix row after the end marker is not read.
		{v1: "models.RoleCreate"},
		// Fields outside an entry, prose, and text after an entry's fields map nothing.
		{v1: "api.Users.Delete"},
		{v1: "api.Users.Get"},
		{v1: "api.Tenants.Create"},
		{v1: "api.Roles.Create"},
		{v1: "go 1.19"},
	} {
		ids, v2 := g.resolve(test.v1, test.typ)
		if !slices.Equal(ids, test.ids) || v2 != test.v2 {
			t.Errorf("resolve(%q, %q) = %q, %q; want %q, %q", test.v1, test.typ, ids, v2,
				test.ids, test.v2)
		}
	}
	if !slices.Equal(g.module, []string{"M-module-1"}) {
		t.Errorf("the entries of the module path are %q, want [M-module-1]", g.module)
	}
}

func TestParseGuide(t *testing.T) {
	t.Parallel()
	for name, test := range map[string]struct {
		text   string
		v1     string
		ids    []string
		v2     string
		module []string
	}{
		"an entry ends at the next section": {
			text: "## API\n\n<a id=\"M-users-1\"></a>\n### M-users-1: Users\n\n" +
				"- v1: `api.Users.Get`\n\n## Notes\n\n- v1: `api.Users.Create`\n",
			v1: "api.Users.Create",
		},
		"an entry ends at a bare section mark": {
			text: "<a id=\"M-users-1\"></a>\n- v1: `api.Users.Get`\n##\n- v1: `api.Users.Create`\n",
			v1:   "api.Users.Create",
		},
		"an entry ends at the appendix": {
			text: "<a id=\"M-users-1\"></a>\n- v1: `api.Users.Get`\n" +
				"<!-- apisurface:models:begin -->\n<!-- apisurface:models:end -->\n" +
				"- v1: `api.Users.Create`\n",
			v1: "api.Users.Create",
		},
		"a guide with CRLF line ends": {
			text: "<a id=\"M-users-1\"></a>\r\n### M-users-1: Users\r\n\r\n" +
				"- v1: `api.Users.Get`\r\n",
			v1:  "api.Users.Get",
			ids: []string{"M-users-1"},
		},
		"an entry ends at a heading that is not its own": {
			text: "<a id=\"M-users-1\"></a>\n\n### M-users-1: Users\n### Other\n" +
				"- v1: `api.Users.Get`\n",
			v1: "api.Users.Get",
		},
		"the heading may follow the anchor after blank lines": {
			text: "<a id=\"M-users-1\"></a>\n\n\n### M-users-1: Users\n- v1: `api.Users.Get`\n",
			v1:   "api.Users.Get",
			ids:  []string{"M-users-1"},
		},
		"an entry without a heading takes its fields": {
			text: "<a id=\"M-users-1\"></a>\n- v1: `api.Users.Get`\n",
			v1:   "api.Users.Get",
			ids:  []string{"M-users-1"},
		},
		"the next anchor starts the next entry": {
			text: "<a id=\"M-users-1\"></a>\n### M-users-1: A\n- v1: `api.Users.Get`\n" +
				"<a id=\"M-users-2\"></a>\n### M-users-2: B\n- v1: `api.Users.Get`\n",
			v1:  "api.Users.Get",
			ids: []string{"M-users-1", "M-users-2"},
		},
		"an entry that names an identifier twice lists itself once": {
			text: "<a id=\"M-users-1\"></a>\n- v1: `api.Users.*`\n- v1: `api.Users.Get`\n",
			v1:   "api.Users.Get",
			ids:  []string{"M-users-1"},
		},
		"an anchor that is not alone on its line is not an entry": {
			text: "<a id=\"M-users-1\"></a> Users\n- v1: `api.Users.Get`\n" +
				"<a id=\"M-users-2\"></a>\n",
			v1: "api.Users.Get",
		},
		"an anchor whose ID does not start with M- is not an entry": {
			text: "<a id=\"users-1\"></a>\n- v1: `api.Users.Get`\n<a id=\"M-users-2\"></a>\n",
			v1:   "api.Users.Get",
		},
		"a reference is the text between the first two backquotes": {
			text: "<a id=\"M-users-1\"></a>\n- v1:   `api.Users.Get`, now `api.Users.Read`\n",
			v1:   "api.Users.Get",
			ids:  []string{"M-users-1"},
		},
		"a line that does not start with a backquote is prose": {
			text: "<a id=\"M-users-1\"></a>\n- v1: the method `api.Users.Get`\n",
			v1:   "api.Users.Get",
		},
		"an unclosed backquote names nothing": {
			text: "<a id=\"M-users-1\"></a>\n- v1: `api.Users.Get\n",
			v1:   "api.Users.Get",
		},
		"empty backquotes name nothing": {
			text: "<a id=\"M-users-1\"></a>\n- v1: ``\n",
			v1:   "",
		},
		"a v1: line that names the module path maps the imports": {
			text: "<a id=\"M-module-1\"></a>\n" +
				"- v1: the module `github.com/permitio/permit-golang`\n" +
				"<a id=\"M-module-3\"></a>\n- v1: `github.com/permitio/permit-golang`\n" +
				"- v1: the module `github.com/permitio/permit-golang`, again\n" +
				"<a id=\"M-module-4\"></a>\n" +
				"- v1: the module `github.com/permitio/permit-golang/v2`\n" +
				"- v2: the module `github.com/permitio/permit-golang`\n",
			module: []string{"M-module-1", "M-module-3"},
		},
		"a models type that v2 keeps is a row of the appendix": {
			text: "<a id=\"M-users-1\"></a>\n<!-- apisurface:models:begin -->\n" +
				"| `models.UserRead` | `models.UserRead` |\n" +
				"| `models.UserCreate` | `models.UserCreateV2` |\n" +
				"| `models.UserUpdate` | `models.UserUpdate` ([M-users-1](#M-users-1)) |\n" +
				"<!-- apisurface:models:end -->\n",
			v1: "models.UserRead",
			v2: "models.UserRead",
		},
		"a row maps to each entry that it links, once": {
			text: "<a id=\"M-users-1\"></a>\n<!-- apisurface:models:begin -->\n" +
				"| `models.UserRead` | `models.UserRead` ([M-users-1](#M-users-1), " +
				"[M-users-2](#M-users-2), [M-users-1](#M-users-1)) |\n" +
				"<!-- apisurface:models:end -->\n",
			v1:  "models.UserRead",
			ids: []string{"M-users-1", "M-users-2"},
		},
		"a row that renames the type is not kept": {
			text: "<a id=\"M-users-1\"></a>\n<!-- apisurface:models:begin -->\n" +
				"| `models.UserCreate` | `models.UserCreateV2` |\n<!-- apisurface:models:end -->\n",
			v1: "models.UserCreate",
		},
		"a row before the appendix is not read": {
			text: "<a id=\"M-users-1\"></a>\n| `models.UserRead` | `models.UserRead` |\n" +
				"<!-- apisurface:models:begin -->\n<!-- apisurface:models:end -->\n",
			v1: "models.UserRead",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			g, err := parseGuide(test.text)
			if err != nil {
				t.Fatalf("parseGuide: %v", err)
			}
			if test.v1 != "" {
				ids, v2 := g.resolve(test.v1, "")
				if !slices.Equal(ids, test.ids) || v2 != test.v2 {
					t.Errorf("resolve(%q) = %q, %q; want %q, %q", test.v1, ids, v2, test.ids,
						test.v2)
				}
			}
			if !slices.Equal(g.module, test.module) {
				t.Errorf("the entries of the module path are %q, want %q", g.module, test.module)
			}
		})
	}
}

func TestParseGuideRejects(t *testing.T) {
	t.Parallel()
	for name, test := range map[string]struct {
		text, want string
	}{
		"no entry": {
			text: "# Migrating\n\n- v1: `api.Users.Get`\n",
			want: "has no entry, an anchor such as <a id=\"M-module-1\"></a>",
		},
		"an empty file": {
			text: "",
			want: "has no entry",
		},
		"a malformed pattern": {
			text: "<a id=\"M-users-1\"></a>\n\n- v1: `api.Users.[Get`\n",
			want: "line 3: the v1: line of M-users-1 names api.Users.[Get, which is not a " +
				"valid pattern",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := parseGuide(test.text)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Errorf("parseGuide = %v, want an error with %q", err, test.want)
			}
		})
	}
}
