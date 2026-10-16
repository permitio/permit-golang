package surface

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// fixture returns the Files of the fixture directory testdata/name.
func fixture(name string) Files {
	dir := filepath.Join("testdata", name)
	return Files{
		OperationMap: filepath.Join(dir, "operation-map.json"),
		LocalOnly:    filepath.Join(dir, "local-only.txt"),
		Advisory:     filepath.Join(dir, "advisory.txt"),
		Pending:      filepath.Join(dir, "pending.txt"),
		Keys:         filepath.Join(dir, "keys.yaml"),
		AllowedSkips: filepath.Join(dir, "allowed-skips.txt"),
	}
}

// passScenarios are the cloud scenarios of the pass fixture. Client.BulkCheck and
// Client.FilterObjects send the operation bulk, and only Client.BulkCheck is covered.
var passScenarios = []Scenario{
	{Name: "scope", Methods: []string{"Client.Scope"}, Sent: []Op{{ID: "scope"}}},
	{Name: "bulk", Methods: []string{"Client.BulkCheck"}, Sent: []Op{{ID: "bulk"}}},
}

func TestCheckPass(t *testing.T) {
	r := Check(fixture("pass"), passScenarios)
	if r.Exit() != 0 || r.Verdict() != "PASS" || r.Problems != nil || r.Broken != nil {
		t.Fatalf("Check of the pass fixture = %d %s, %q, %q", r.Exit(), r.Verdict(), r.Problems,
			r.Broken)
	}
	want := map[string]string{
		"Client.BulkCheck": Cloud, "Client.Check": Advisory, "Client.FilterObjects": Pending,
		"Client.Scope": Cloud, "UsersService.List": LocalOnly,
	}
	if !maps.Equal(r.States, want) {
		t.Errorf("States = %v, want %v", r.States, want)
	}
	if got := r.Summary(); got != "5 methods = 2 cloud + 1 local-only + 1 advisory + 1 pending "+
		"+ 0 unplaced" {
		t.Errorf("Summary = %q", got)
	}
}

// failScenarios are the cloud scenarios of the fail fixture.
var failScenarios = []Scenario{
	{
		Name: "scope", Methods: []string{"Client.Scope", "Client.Nope"},
		Sent: []Op{{ID: "scope"}, {ID: "scope"}},
	},
	{
		Name: "bulk", Methods: []string{"Client.Bulk"},
		Sent: []Op{
			{ID: "list_users", OrganizationLevel: true},
			{ID: "list_users", OrganizationLevel: true},
		},
	},
}

func TestCheckFail(t *testing.T) {
	r := Check(fixture("fail"), failScenarios)
	want := []string{
		"testdata/fail/local-only.txt:2: the reason cloud-only is not one of org-key, " +
			"account-level-write, needs-pdp-id, unverified-response",
		"testdata/fail/local-only.txt:3: Client.Nothing is not a method of the operation map; " +
			"remove the line",
		"testdata/fail/advisory.txt:2: no entry of allowed-skips.txt lets a test of tools/e2e " +
			"skip with the ticket TICKET-2, and the scenario of an advisory method skips in the " +
			"dry run too",
		"the cloud scenario bulk sends list_users, which needs an organization-level API key, " +
			"and keys.yaml gives the cloud lane none: remove the call from the cloud scenarios " +
			"and list each method that sends list_users in local-only.txt with the reason " +
			"org-key, or set organization_key in keys.yaml once the lane has the key",
		"the cloud scenario scope covers Client.Nope, which is not a method of the operation map",
		"Client.Bulk is in no end-to-end state: the cloud scenario bulk covers it, and its dry " +
			"run did not send bulk; cover it in a cloud scenario that calls it, or list it in " +
			"local-only.txt, advisory.txt or pending.txt",
		"Client.Check is in more than one end-to-end state: testdata/fail/advisory.txt:1, " +
			"testdata/fail/pending.txt:1",
		"testdata/fail/local-only.txt:1: local-only is stale: the cloud scenario scope covers " +
			"Client.Scope now, so remove the line",
		"UsersService.List is in no end-to-end state: no cloud scenario covers it; cover it in a " +
			"cloud scenario that calls it, or list it in local-only.txt, advisory.txt or " +
			"pending.txt",
	}
	if !slices.Equal(r.Problems, want) {
		t.Errorf("Problems =\n%s\nwant\n%s", strings.Join(r.Problems, "\n"),
			strings.Join(want, "\n"))
	}
	if r.Exit() != 1 || r.Verdict() != "FAIL" || r.Broken != nil || len(r.States) != 0 {
		t.Errorf("Check of the fail fixture = %d %s, %q, %v", r.Exit(), r.Verdict(), r.Broken,
			r.States)
	}
	if got := r.Summary(); got != "4 methods = 0 cloud + 0 local-only + 0 advisory + 0 pending "+
		"+ 4 unplaced" {
		t.Errorf("Summary = %q", got)
	}
}

func TestCheckDidNotRun(t *testing.T) {
	r := Check(fixture("didnotrun"), passScenarios)
	if r.Exit() != 2 || r.Verdict() != "DID NOT RUN" || len(r.Broken) != 1 ||
		!strings.Contains(r.Broken[0], `parsing the operation map `+
			`testdata/didnotrun/operation-map.json: json: unknown field "extra"`) {
		t.Errorf("Check of the did-not-run fixture = %d %s, %q", r.Exit(), r.Verdict(), r.Broken)
	}
	for _, tc := range []struct {
		name string
		file func(f *Files) *string
		want string
	}{
		{"pending.txt", func(f *Files) *string { return &f.Pending }, "reading the pending list"},
		{"local-only.txt", func(f *Files) *string { return &f.LocalOnly }, "the local-only list"},
		{"advisory.txt", func(f *Files) *string { return &f.Advisory }, "the advisory list"},
		{"keys.yaml", func(f *Files) *string { return &f.Keys }, "the keys of the cloud lane"},
		{"allowed-skips.txt", func(f *Files) *string { return &f.AllowedSkips }, "allowed skips"},
	} {
		files := fixture("pass")
		*tc.file(&files) = filepath.Join("testdata", "none")
		r = Check(files, passScenarios)
		if r.Exit() != 2 || len(r.Broken) != 1 || !strings.Contains(r.Broken[0], tc.want) {
			t.Errorf("Check without %s = %d, %q; want an error with %q", tc.name, r.Exit(),
				r.Broken, tc.want)
		}
		if r.Problems != nil || len(r.States) != 0 || r.Counts["methods"] != 0 {
			t.Errorf("Check without %s placed methods: %q, %v, %v", tc.name, r.Problems, r.States,
				r.Counts)
		}
	}
}

// inputs are the contents of the gate's inputs: methods is the methods of the operation map, and
// keys is organization_key: false when it is empty.
type inputs struct {
	methods, localOnly, advisory, pending, keys, allowedSkips string
}

// write writes the gate's inputs in to a directory of t, and returns their Files.
func write(t *testing.T, in inputs) Files {
	t.Helper()
	dir := t.TempDir()
	files := Files{
		OperationMap: filepath.Join(dir, "operation-map.json"),
		LocalOnly:    filepath.Join(dir, "local-only.txt"),
		Advisory:     filepath.Join(dir, "advisory.txt"),
		Pending:      filepath.Join(dir, "pending.txt"),
		Keys:         filepath.Join(dir, "keys.yaml"),
		AllowedSkips: filepath.Join(dir, "allowed-skips.txt"),
	}
	if in.keys == "" {
		in.keys = "organization_key: false\n"
	}
	for path, content := range map[string]string{
		files.OperationMap: `{"methods": ` + in.methods + `}`, files.LocalOnly: in.localOnly,
		files.Advisory: in.advisory, files.Pending: in.pending, files.Keys: in.keys,
		files.AllowedSkips: in.allowedSkips,
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return files
}

// scopeScenario is a cloud scenario that covers Client.Scope and sends scope and other.
var scopeScenario = Scenario{
	Name: "scope", Methods: []string{"Client.Scope"}, Sent: []Op{{ID: "scope"}, {ID: "other"}},
}

func TestCheckLines(t *testing.T) {
	const methods = `{"Client.Check": ["check"], "Client.Scope": ["scope", "other"]}`
	const skips = "github.com/permitio/permit-golang/v2/tools/e2e TestCloud/check TICKET-1\n"
	for _, tc := range []struct {
		name string
		in   inputs
		want string
	}{
		{
			name: "bad method", in: inputs{localOnly: "check org-key api.x\n"},
			want: "the method check is not <Type>.<Method>, such as Client.Check",
		},
		{
			name: "local-only fields", in: inputs{localOnly: "Client.Check org-key\n"},
			want: "a line is <Method> <reason> <phase>",
		},
		{
			name: "local-only extra field", in: inputs{localOnly: "Client.Check org-key api.x z\n"},
			want: "a line is <Method> <reason> <phase>",
		},
		{
			name: "phase", in: inputs{localOnly: "Client.Check org-key projects\n"},
			want: "the phase projects is not a phase of the local harness",
		},
		{
			name: "advisory fields", in: inputs{advisory: "Client.Check TICKET-1\n"},
			want: "a line is <Method> <ticket> <reason>",
		},
		{
			name: "ticket", in: inputs{advisory: "Client.Check per-1 why\n"},
			want: "the ticket per-1 is not a ticket",
		},
		{
			name: "skip of another ticket", in: inputs{advisory: "Client.Check TICKET-2 why\n"},
			want: "advisory.txt:1: no entry of allowed-skips.txt lets a test of tools/e2e skip " +
				"with the ticket TICKET-2",
		},
		{
			name: "pending marker", in: inputs{pending: "Client.Check pending pdp\n"},
			want: "a line is <Method> e2e-pending <area>",
		},
		{
			name: "pending fields", in: inputs{pending: "Client.Check e2e-pending\n"},
			want: "a line is <Method> e2e-pending <area>",
		},
		{
			name: "area", in: inputs{pending: "Client.Check e2e-pending PDP\n"},
			want: "the area PDP is not lower-case words joined by hyphens",
		},
		{
			name: "area hyphens", in: inputs{pending: "Client.Check e2e-pending pdp--x\n"},
			want: "the area pdp--x",
		},
		{
			name: "twice",
			in:   inputs{pending: "Client.Check e2e-pending pdp\nClient.Check e2e-pending pdp\n"},
			want: "pending.txt:2: Client.Check has a line above already",
		},
		{
			name: "local-only and advisory",
			in: inputs{
				localOnly: "Client.Check org-key api.x\n", advisory: "Client.Check TICKET-1 why\n",
			},
			want: "Client.Check is in more than one end-to-end state",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.in.methods, tc.in.allowedSkips = methods, skips
			r := Check(write(t, tc.in), []Scenario{scopeScenario})
			if r.Exit() != 1 || !slices.ContainsFunc(r.Problems, func(p string) bool {
				return strings.Contains(p, tc.want)
			}) {
				t.Errorf("Check = %d, %q; want a problem with %q", r.Exit(), r.Problems, tc.want)
			}
		})
	}
	for _, line := range []string{
		"Client.Check org-key sdk.config", "Client.Check needs-pdp-id probe.pdp_id",
		"Client.Check unverified-response offline.x",
	} {
		r := Check(write(t, inputs{methods: methods, localOnly: line}), []Scenario{scopeScenario})
		if r.Exit() != 0 || r.States["Client.Check"] != LocalOnly {
			t.Errorf("the local-only line %q: %d, %q", line, r.Exit(), r.Problems)
		}
	}
	r := Check(write(t, inputs{
		methods: methods, advisory: "Client.Check TICKET-1 why\n", allowedSkips: "# a comment\n" +
			"github.com/permitio/permit-golang/v2/tools/e2e\n" + skips,
	}), []Scenario{scopeScenario})
	if r.Exit() != 0 || r.States["Client.Check"] != Advisory {
		t.Errorf("an advisory line whose ticket lets a test of tools/e2e skip: %d, %q", r.Exit(),
			r.Problems)
	}
}

func TestCheckCloud(t *testing.T) {
	files := write(t, inputs{
		methods: `{"Client.Scope": ["scope", "other"], "Client.Nop": [], ` +
			`"Client.Again": ["scope"]}`,
		pending: "Client.Nop e2e-pending nop\nClient.Again e2e-pending again\n",
	})
	half := Scenario{Name: "half", Methods: []string{"Client.Scope"}, Sent: []Op{{ID: "scope"}}}
	r := Check(files, []Scenario{half})
	want := "Client.Scope is in no end-to-end state: the cloud scenario half covers it, and its " +
		"dry run did not send other; cover it"
	if r.Exit() != 1 || len(r.Problems) != 1 || !strings.HasPrefix(r.Problems[0], want) {
		t.Errorf("a method with an operation that its scenario did not send: %d, %q", r.Exit(),
			r.Problems)
	}
	r = Check(files, []Scenario{half, scopeScenario})
	wantStates := map[string]string{
		"Client.Scope": Cloud, "Client.Nop": Pending, "Client.Again": Pending,
	}
	if r.Exit() != 0 || !maps.Equal(r.States, wantStates) {
		t.Errorf("a method whose operations its scenario sent, and one that shares an operation "+
			"with it: %d, %v, %q", r.Exit(), r.States, r.Problems)
	}
	nop := Scenario{Name: "nop", Methods: []string{"Client.Nop"}, Sent: []Op{{ID: "scope"}}}
	r = Check(write(t, inputs{methods: `{"Client.Nop": []}`}), []Scenario{nop})
	want = "Client.Nop is in no end-to-end state: the cloud scenario nop covers it, and the " +
		"operation map gives it no operation to send; cover it"
	if r.Exit() != 1 || len(r.Problems) != 1 || !strings.HasPrefix(r.Problems[0], want) {
		t.Errorf("a method without an operation: %d, %q", r.Exit(), r.Problems)
	}
	r = Check(files, []Scenario{scopeScenario, {
		Name: "again", Methods: []string{"Client.Again"}, Sent: []Op{{ID: "scope"}},
	}})
	want = "pending.txt:2: pending is stale: the cloud scenario again covers Client.Again now, " +
		"so remove the line"
	if r.Exit() != 1 || len(r.Problems) != 1 || !strings.HasSuffix(r.Problems[0], want) ||
		r.States["Client.Scope"] != Cloud {
		t.Errorf("a pending line of a method that its scenario covers: %d, %q", r.Exit(),
			r.Problems)
	}
	r = Check(write(t, inputs{methods: `{}`}), nil)
	if r.Exit() != 2 || len(r.Broken) != 1 || !strings.Contains(r.Broken[0], "has no method") {
		t.Errorf("an empty operation map: %d, %q", r.Exit(), r.Broken)
	}
	r = Check(Files{OperationMap: filepath.Join(t.TempDir(), "none.json")}, nil)
	if r.Exit() != 2 || len(r.Broken) != 1 || !strings.Contains(r.Broken[0],
		"reading the operation map") {
		t.Errorf("a missing operation map: %d, %q", r.Exit(), r.Broken)
	}
}

func TestCheckKeys(t *testing.T) {
	const methods = `{"UsersService.List": ["list_users"], "Client.Scope": ["scope"]}`
	scenarios := []Scenario{
		{Name: "scope", Methods: []string{"Client.Scope"}, Sent: []Op{{ID: "scope"}}},
		{
			Name: "users", Methods: []string{"UsersService.List"},
			Sent: []Op{{ID: "list_users", OrganizationLevel: true}},
		},
	}
	r := Check(write(t, inputs{methods: methods}), scenarios)
	want := []string{
		"the cloud scenario users sends list_users, which needs an organization-level API key, " +
			"and keys.yaml gives the cloud lane none: remove the call from the cloud scenarios " +
			"and list each method that sends list_users in local-only.txt with the reason " +
			"org-key, or set organization_key in keys.yaml once the lane has the key",
	}
	if r.Exit() != 1 || !slices.Equal(r.Problems, want) {
		t.Errorf("an organization-level operation without the key: %d, %q", r.Exit(), r.Problems)
	}
	files := write(t, inputs{methods: methods, keys: "organization_key: true\n"})
	if r = Check(files, scenarios); r.Exit() != 0 || r.States["UsersService.List"] != Cloud {
		t.Errorf("an organization-level operation with the key: %d, %q", r.Exit(), r.Problems)
	}
	files = write(t, inputs{methods: methods, keys: "organization_key: maybe\n"})
	if r = Check(files, scenarios); r.Exit() != 2 || len(r.Broken) != 1 ||
		!strings.Contains(r.Broken[0], "parsing") {
		t.Errorf("a keys.yaml that does not parse: %d, %q", r.Exit(), r.Broken)
	}
}

func TestCountSentinel(t *testing.T) {
	r := Result{Counts: map[string]int{"methods": 3, Cloud: 1, Pending: 1}}
	if got := r.Summary(); got != "3 methods = 1 cloud + 0 local-only + 0 advisory + 1 pending "+
		"+ 0 unplaced" {
		t.Errorf("Summary = %q", got)
	}
	checkSentinel(&r)
	want := "count sentinel: 3 methods = 1 cloud + 0 local-only + 0 advisory + 1 pending + 0 " +
		"unplaced does not add up"
	if r.Exit() != 2 || !slices.Equal(r.Broken, []string{want}) {
		t.Errorf("a sentinel that does not add up: %d, %q; want %q", r.Exit(), r.Broken, want)
	}
	r = Result{Counts: map[string]int{"methods": 3, Advisory: 1, LocalOnly: 1, "unplaced": 1}}
	if checkSentinel(&r); r.Broken != nil {
		t.Errorf("a sentinel that adds up: %q", r.Broken)
	}
	files := write(t, inputs{methods: `{"Client.Scope": ["scope"]}`})
	r = Check(files, passScenarios[:1])
	if r.Exit() != 0 || r.Counts[Cloud] != 1 || r.Counts["unplaced"] != 0 {
		t.Errorf("one cloud method: %d, %v, %q, %q", r.Exit(), r.Counts, r.Problems, r.Broken)
	}
}
