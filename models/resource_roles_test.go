package models_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/permitio/permit-golang/v2/models"
)

// TestResourceRoleListDecodes checks that a ResourceRoleList reads the roles of a reply whose
// roles are a JSON array or the paginated object of the API, whose data array holds them, with or
// without its counts and other fields, in the order of the reply, with attributes of a number and
// an object; and that an empty array, an empty page, null and roles left out each read as an
// empty slice, not nil, in place of the roles that the ResourceRoleList held.
func TestResourceRoleListDecodes(t *testing.T) {
	roles := []models.ResourceRoleRead{
		{
			Key: "editor", Resource: "document",
			Attributes: models.Attributes{
				"seats": json.Number("10"), "quota": map[string]any{"max": json.Number("2.5")},
			},
		},
		{Key: "viewer", Resource: "document"},
	}
	const array = `[{"key":"editor","resource":"document",` +
		`"attributes":{"seats":10,"quota":{"max":2.5}}},{"key":"viewer","resource":"document"}]`
	for _, tc := range []struct {
		name string
		data string
		want []models.ResourceRoleRead
	}{
		{"an array", `{"roles":` + array + `}`, roles},
		{"a page", `{"roles":{"data":` + array + `,"total_count":2,"page_count":2}}`, roles},
		{"a page without counts", `{"roles":{"data":` + array + `}}`, roles},
		{"another field", `{"next":1,"roles":` + array + `}`, roles},
		{"an empty array", `{"roles":[]}`, []models.ResourceRoleRead{}},
		{"an empty page", `{"roles":{"data":[],"total_count":0}}`, []models.ResourceRoleRead{}},
		{"null", `{"roles":null}`, []models.ResourceRoleRead{}},
		{"no roles", `{}`, []models.ResourceRoleRead{}},
	} {
		list := models.ResourceRoleList{Roles: []models.ResourceRoleRead{{Key: "old"}}}
		if err := json.Unmarshal([]byte(tc.data), &list); err != nil {
			t.Fatalf("%s: %s does not decode: %v", tc.name, tc.data, err)
		}
		if diff := cmp.Diff(tc.want, list.Roles); diff != "" || list.Roles == nil {
			t.Errorf("%s: %s decodes to roles that differ (-want +got), or nil:\n%s", tc.name,
				tc.data, diff)
		}
	}
}

// TestResourceRoleListDecodeErrors checks that a value that is not a JSON object, and roles that
// are neither a JSON array of roles nor an object whose data array holds them, are errors that
// leave the ResourceRoleList as it is; an error of the roles names them.
func TestResourceRoleListDecodeErrors(t *testing.T) {
	old := models.ResourceRoleList{Roles: []models.ResourceRoleRead{{Key: "old"}}}
	for _, data := range []string{`[]`, `"a"`, `1`, `{"roles":[]} {}`} {
		list := models.ResourceRoleList{Roles: []models.ResourceRoleRead{{Key: "old"}}}
		if err := json.Unmarshal([]byte(data), &list); err == nil || !cmp.Equal(list, old) {
			t.Errorf("%s decodes to %+v and %v, want an error and the ResourceRoleList as it was",
				data, list, err)
		}
	}
	for _, data := range []string{
		`{"roles":"a"}`, `{"roles":1}`, `{"roles":true}`, `{"roles":{}}`,
		`{"roles":{"data":"a"}}`, `{"roles":[1]}`, `{"roles":[{"attributes":[]}]}`,
	} {
		list := models.ResourceRoleList{Roles: []models.ResourceRoleRead{{Key: "old"}}}
		err := json.Unmarshal([]byte(data), &list)
		if err == nil || !strings.Contains(err.Error(), "the roles of a ResourceRoleList") ||
			!cmp.Equal(list, old) {
			t.Errorf("%s decodes to %+v and %v, want an error that names the roles of a "+
				"ResourceRoleList and the ResourceRoleList as it was", data, list, err)
		}
	}
}

// TestResourceRoleListNullLeavesItAsItIs checks that null leaves a ResourceRoleList as it is, as
// encoding/json leaves a struct for null, both on its own and in a field.
func TestResourceRoleListNullLeavesItAsItIs(t *testing.T) {
	list := models.ResourceRoleList{Roles: []models.ResourceRoleRead{{Key: "old"}}}
	if err := list.UnmarshalJSON([]byte("null")); err != nil || len(list.Roles) != 1 {
		t.Errorf("null decodes to %+v and %v, want the ResourceRoleList as it was", list, err)
	}
	var reply struct {
		List models.ResourceRoleList `json:"list"`
	}
	reply.List = list
	if err := json.Unmarshal([]byte(`{"list":null}`), &reply); err != nil ||
		len(reply.List.Roles) != 1 {
		t.Errorf("a field of null decodes to %+v and %v, want the ResourceRoleList as it was",
			reply.List, err)
	}
}
