package permit_test

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/permitio/permit-golang/v2/internal/validate"
)

// rulesFile lists the validation rules, which apisurface's diff mode compares with the base of a
// pull request.
const rulesFile = "testdata/validation-rules.txt"

// rulesHeader starts rulesFile.
const rulesHeader = "" +
	"# The validation rules of the SDK: each check that rejects a configuration or an\n" +
	"# argument before a request is sent, one per line as <ID> <message>, sorted by ID.\n" +
	"# TestValidationRules (validation_rules_test.go) fails when internal/validate differs.\n" +
	"# After an intended change, rewrite this file with\n" +
	"# go test -run TestValidationRules -update . and review the diff. apisurface's diff mode\n" +
	"# counts an ID that is new here as a breaking change.\n"

// ruleID matches a rule ID: lower-case words, each joined to the next by a dot.
var ruleID = regexp.MustCompile(`^[a-z][a-z0-9-]*(?:\.[a-z][a-z0-9-]*)+$`)

// rulesText returns the content of rulesFile for the rules.
func rulesText(rules []validate.Rule) string {
	var b strings.Builder
	b.WriteString(rulesHeader)
	for _, r := range rules {
		b.WriteString(r.ID + " " + r.Message + "\n")
	}
	return b.String()
}

// ruleProblems returns a problem for each rule whose ID is malformed, repeated or out of order,
// and for each whose message is empty or has more than one line.
func ruleProblems(rules []validate.Rule) []string {
	var problems []string
	for i, r := range rules {
		switch {
		case !ruleID.MatchString(r.ID):
			problems = append(problems, fmt.Sprintf("rule %d: the ID %q is not lower-case words "+
				"joined by dots, such as config.api-key.missing", i, r.ID))
		case i > 0 && r.ID <= rules[i-1].ID:
			problems = append(problems, fmt.Sprintf("rule %d: %s is not after %s; sort the "+
				"rules by ID, each ID once", i, r.ID, rules[i-1].ID))
		}
		if strings.TrimSpace(r.Message) == "" || strings.ContainsAny(r.Message, "\r\n") {
			problems = append(problems, fmt.Sprintf("rule %d (%s): the message must be one line "+
				"of text", i, r.ID))
		}
	}
	return problems
}

func TestValidationRules(t *testing.T) {
	rules := validate.Rules()
	for _, problem := range ruleProblems(rules) {
		t.Error(problem)
	}
	got := rulesText(rules)
	if *update {
		if err := os.WriteFile(rulesFile, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s", rulesFile)
	}
	want, err := os.ReadFile(rulesFile)
	if err != nil {
		t.Fatalf("%v; write the file with go test -run TestValidationRules -update .", err)
	}
	if diff := snapshotDiff(string(want), got); diff != "" {
		t.Errorf("the validation rules differ from %s (-file +internal/validate):\n%s\nWhen the "+
			"change is intended, rewrite the file with go test -run TestValidationRules -update "+
			". and review its diff.", rulesFile, diff)
	}
}

func TestRulesText(t *testing.T) {
	got := rulesText([]validate.Rule{
		{ID: "config.api-key.missing", Message: "give an API key"},
		{ID: "path.segment.empty", Message: "a path parameter is empty"},
	})
	want := rulesHeader + "config.api-key.missing give an API key\n" +
		"path.segment.empty a path parameter is empty\n"
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("rulesText differs (-want +got):\n%s", diff)
	}
	if got := rulesText(nil); got != rulesHeader {
		t.Errorf("rulesText(nil) = %q, want the header alone", got)
	}
}

func TestRuleProblems(t *testing.T) {
	valid := []validate.Rule{
		{ID: "config.api-key.missing", Message: "give an API key"},
		{ID: "config.url.scheme", Message: "the URL must use http or https"},
	}
	if problems := ruleProblems(valid); len(problems) != 0 {
		t.Errorf("ruleProblems of valid rules = %q, want none", problems)
	}
	for name, tc := range map[string]struct {
		rules []validate.Rule
		want  string
	}{
		"an ID without a dot": {
			rules: []validate.Rule{{ID: "config", Message: "m"}},
			want:  `rule 0: the ID "config" is not lower-case words joined by dots`,
		},
		"an ID in upper case": {
			rules: []validate.Rule{{ID: "Config.key", Message: "m"}},
			want:  `rule 0: the ID "Config.key" is not lower-case words`,
		},
		"an ID that ends in a dot": {
			rules: []validate.Rule{{ID: "config.", Message: "m"}},
			want:  `rule 0: the ID "config." is not lower-case words`,
		},
		"a repeated ID": {
			rules: []validate.Rule{{ID: "a.b", Message: "m"}, {ID: "a.b", Message: "m"}},
			want:  "rule 1: a.b is not after a.b; sort the rules by ID, each ID once",
		},
		"IDs out of order": {
			rules: []validate.Rule{{ID: "b.a", Message: "m"}, {ID: "a.b", Message: "m"}},
			want:  "rule 1: a.b is not after b.a",
		},
		"an empty message": {
			rules: []validate.Rule{{ID: "a.b", Message: " "}},
			want:  "rule 0 (a.b): the message must be one line of text",
		},
		"a message of two lines": {
			rules: []validate.Rule{{ID: "a.b", Message: "one\ntwo"}},
			want:  "rule 0 (a.b): the message must be one line of text",
		},
	} {
		problems := ruleProblems(tc.rules)
		if len(problems) != 1 || !strings.Contains(problems[0], tc.want) {
			t.Errorf("%s: ruleProblems = %q, want one problem that contains %q", name, problems,
				tc.want)
		}
	}
}
