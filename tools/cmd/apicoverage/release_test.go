package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// decideEveryRow rewrites the fixture's coverage.yaml without a planned row: each planned row of
// a route of a spec is excluded, and the planned row of the SDK-only route, with the
// sdk_only_params entry, is gone.
func decideEveryRow(t *testing.T, f *fixture) {
	t.Helper()
	file := filepath.Join(f.specs, "coverage.yaml")
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	decided := strings.NewReplacer(
		"stage: GA, status: planned}", "stage: GA, status: excluded, reason: operator}",
		"    status: planned\n    stage_override:",
		"    status: excluded\n    reason: operator\n    stage_override:",
		"  - op: elements_login_as\n    route: \"POST /v2/auth/elements_login_as\"\n"+
			"    stage: GA\n    status: planned\n"+
			"    sdk_only: \"The login-as route is not in the spec.\"\n", "",
		"  - op: forward_remaining_requests\n    route: \"ANY /facts/{full_path}\"\n"+
			"    stage: GA\n    status: planned\n"+
			"    sdk_only: \"The catch-all of the facts routes.\"\n",
		"  - {route: \"ANY /facts/{full_path}\", stage: GA, status: excluded, reason: operator}\n",
	).Replace(string(data))
	decided, _, _ = strings.Cut(decided, "sdk_only_params:")
	if strings.Contains(decided, "planned") {
		t.Fatalf("coverage.yaml still has a planned row:\n%s", decided)
	}
	f.write(file, decided)
}

func TestRelease(t *testing.T) {
	live := strings.Replace(getUser, `"wire"`, `"live"`, 1)
	dryRun := strings.Replace(getUser, `"wire"`, `"dryrun"`, 1)
	tests := []struct {
		name string
		// planned keeps the planned rows of the fixture.
		planned bool
		// requests are the requests of a record next to the wire record; localOnly and advisory
		// are the lines of the lists.
		requests            []string
		localOnly, advisory string
		wantExit            int
		want                []string
	}{
		{
			name:     "every covered row in the cloud or advisory",
			requests: []string{live}, advisory: "Client.Check the PDP image may lag\n",
			wantExit: exitPassed,
			want: []string{
				"e2e of the 2 covered rows: 1 cloud, 0 local, 0 local-only, 1 advisory, 0 dry " +
					"run, 0 none\n",
				"PASS: 2 of 17 rows covered",
			},
		},
		{
			name:      "a local-only method",
			requests:  []string{dryRun},
			localOnly: "# The local stack.\n\nUsersService.Get org-key api.users\n",
			advisory:  "Client.Check the PDP image may lag\n",
			wantExit:  exitPassed,
			want:      []string{"(get_user): UsersService.Get; 2 requests; e2e local-only\n"},
		},
		{
			name:    "a remaining planned row",
			planned: true, requests: []string{live}, advisory: "Client.Check lags\n",
			wantExit: exitFailed,
			want: []string{
				"problem: control-plane GET /v2/api-key/scope: planned: a release covers every " +
					"operation it plans; cover it, or exclude or defer it with a reason\n",
				"problem: pdp ANY /facts/{full_path}: planned: ",
				"FAIL: 7 problems with coverage.yaml",
			},
		},
		{
			name:     "a covered row that only a dry run reached",
			requests: []string{dryRun}, advisory: "Client.Check lags\n",
			wantExit: exitFailed,
			want: []string{
				"problem: control-plane GET /v2/facts/{proj_id}/{env_id}/users/{user_id}: e2e " +
					"dry run: a release needs each covered row proven end to end: by a live " +
					"request of the e2e job, by the run against a full local stack, or by a " +
					"method on the -local-only or -advisory list\n",
				"FAIL: 1 problem with coverage.yaml",
			},
		},
		{
			name:     "a covered row that no scenario reached",
			requests: []string{live},
			wantExit: exitFailed,
			want: []string{
				"problem: pdp POST /allowed: e2e none: a release needs each covered row",
				"FAIL: 1 problem with coverage.yaml",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			if !tt.planned {
				decideEveryRow(t, f)
			}
			args := []string{"-release"}
			if tt.localOnly != "" {
				args = append(args, "-local-only", f.raw("local-only.txt", tt.localOnly))
			}
			if tt.advisory != "" {
				args = append(args, "-advisory", f.raw("advisory.txt", tt.advisory))
			}
			args = append(args, f.wire, f.record("e2e.jsonl", tt.requests...))

			code, out := f.run(args...)

			if code != tt.wantExit {
				t.Errorf("apicoverage exited %d, want %d:\n%s", code, tt.wantExit, out)
			}
			for _, want := range tt.want {
				if !strings.Contains(out, want) {
					t.Errorf("apicoverage printed\n%s\nwant %q", out, want)
				}
			}
		})
	}
}

// Without -release, neither a planned row nor a covered row without end-to-end evidence fails.
func TestPlannedRowsPassWithoutRelease(t *testing.T) {
	f := newFixture(t)

	code, out := f.run(f.wire)

	if code != exitPassed || strings.Contains(out, "a release") {
		t.Errorf("apicoverage exited %d, want 0 and no release problem:\n%s", code, out)
	}
}

func TestLocalOnlyListDidNotRun(t *testing.T) {
	tests := []struct{ name, content, want string }{
		{
			"a line without a method", "users org-key api.users\n",
			"local-only.txt:1: \"users org-key api.users\" does not start with a method",
		},
		{"a line without a reason", "UsersService.Get\n", "UsersService.Get has no reason\n"},
		{
			"a method listed twice", "UsersService.Get a p\nUsersService.Get b p\n",
			"local-only.txt:2: UsersService.Get is listed twice\n",
		},
		{"a list that is not there", "", "missing.txt: no such file or directory\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			list := filepath.Join(f.dir, "missing.txt")
			if tt.content != "" {
				list = f.raw("local-only.txt", tt.content)
			}

			code, out := f.run("-release", "-local-only", list, f.wire)

			if code != exitDidNotRun || !strings.HasPrefix(out, "DID NOT RUN: ") ||
				!strings.Contains(out, tt.want) {
				t.Errorf("apicoverage exited %d, want 2 with the verdict DID NOT RUN and %q:\n%s",
					code, tt.want, out)
			}
		})
	}
}
