package models_test

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/permitio/permit-golang/v2/internal/wire"
	"github.com/permitio/permit-golang/v2/models"
)

// IDs of a scope, mostly zeros so that the secret scan does not take them for credentials.
const (
	organizationID = "00000000-0000-4000-8000-000000000001"
	projectID      = "00000000-0000-4000-8000-000000000002"
	environmentID  = "00000000-0000-4000-8000-000000000003"
)

func TestAPIKeyScopeRead(t *testing.T) {
	for _, tc := range []struct {
		level string
		reply string
		want  models.APIKeyScopeRead
	}{
		{
			level: "organization",
			reply: `{"organization_id":"` + organizationID + `"}`,
			want:  models.APIKeyScopeRead{OrganizationID: organizationID},
		},
		{
			level: "project",
			reply: `{"organization_id":"` + organizationID + `","project_id":"` + projectID +
				`"}`,
			want: models.APIKeyScopeRead{OrganizationID: organizationID, ProjectID: projectID},
		},
		{
			level: "environment",
			reply: `{"organization_id":"` + organizationID + `","project_id":"` + projectID +
				`","environment_id":"` + environmentID + `"}`,
			want: models.APIKeyScopeRead{
				OrganizationID: organizationID, ProjectID: projectID,
				EnvironmentID: environmentID,
			},
		},
	} {
		var got models.APIKeyScopeRead
		if err := wire.Decode([]byte(tc.reply), &got); err != nil {
			t.Fatalf("the scope of an %s-level key: %v", tc.level, err)
		}
		if diff := cmp.Diff(tc.want, got); diff != "" {
			t.Errorf("the scope of an %s-level key differs (-want +got):\n%s", tc.level, diff)
		}
		encoded, err := wire.Encode(got)
		if err != nil {
			t.Fatal(err)
		}
		if string(encoded) != tc.reply {
			t.Errorf("the scope of an %s-level key encodes as %s, want %s", tc.level, encoded,
				tc.reply)
		}
	}
	// organization_id is required, so the zero scope still writes it.
	encoded, err := wire.Encode(models.APIKeyScopeRead{})
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"organization_id":""}`; string(encoded) != want {
		t.Errorf("the zero scope encodes as %s, want %s", encoded, want)
	}
}

func TestAPIKeyScopeReadIgnoresUnknownFields(t *testing.T) {
	var got models.APIKeyScopeRead
	reply := `{"organization_id":"` + organizationID + `","added_later":{"a":[1]}}`
	if err := json.Unmarshal([]byte(reply), &got); err != nil {
		t.Fatalf("a scope with an unknown field does not decode: %v", err)
	}
	if want := (models.APIKeyScopeRead{OrganizationID: organizationID}); got != want {
		t.Errorf("the scope decodes to %+v, want %+v", got, want)
	}
}
