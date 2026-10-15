package models_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/permitio/permit-golang/v2/models"
)

// TestRoleListDecodes checks that a RoleList reads the roles of a reply whose roles are a JSON
// array or the paginated object of the API, whose data array holds them, with or without its
// counts and other fields, in the order of the reply; and that an empty array, an empty page,
// null and roles left out each read as an empty slice, not nil, in place of the roles that the
// RoleList held.
func TestRoleListDecodes(t *testing.T) {
	roles := []models.RoleRead{
		{Key: "editor", Name: "Editor", Attributes: models.Attributes{"seats": json.Number("10")}},
		{Key: "viewer", Name: "Viewer"},
	}
	const array = `[{"key":"editor","name":"Editor","attributes":{"seats":10}},` +
		`{"key":"viewer","name":"Viewer"}]`
	for _, tc := range []struct {
		name string
		data string
		want []models.RoleRead
	}{
		{"an array", `{"roles":` + array + `}`, roles},
		{"a page", `{"roles":{"data":` + array + `,"total_count":2,"page_count":1}}`, roles},
		{"a page without counts", `{"roles":{"data":` + array + `}}`, roles},
		{"another field", `{"next":1,"roles":` + array + `}`, roles},
		{"an empty array", `{"roles":[]}`, []models.RoleRead{}},
		{"an empty page", `{"roles":{"data":[],"total_count":0}}`, []models.RoleRead{}},
		{"null", `{"roles":null}`, []models.RoleRead{}},
		{"no roles", `{}`, []models.RoleRead{}},
	} {
		list := models.RoleList{Roles: []models.RoleRead{{Key: "old"}}}
		if err := json.Unmarshal([]byte(tc.data), &list); err != nil {
			t.Fatalf("%s: %s does not decode: %v", tc.name, tc.data, err)
		}
		if diff := cmp.Diff(tc.want, list.Roles); diff != "" || list.Roles == nil {
			t.Errorf("%s: %s decodes to roles that differ (-want +got), or nil:\n%s", tc.name,
				tc.data, diff)
		}
	}
}

// TestRoleListDecodeErrors checks that a value that is not a JSON object, and roles that are
// neither a JSON array of roles nor an object whose data array holds them, are errors that leave
// the RoleList as it is; an error of the roles names them.
func TestRoleListDecodeErrors(t *testing.T) {
	for _, data := range []string{`[]`, `"a"`, `1`, `{"roles":[]} {}`} {
		list := models.RoleList{Roles: []models.RoleRead{{Key: "old"}}}
		if err := json.Unmarshal([]byte(data), &list); err == nil ||
			!cmp.Equal(list, models.RoleList{Roles: []models.RoleRead{{Key: "old"}}}) {
			t.Errorf("%s decodes to %+v and %v, want an error and the RoleList as it was", data,
				list, err)
		}
	}
	for _, data := range []string{
		`{"roles":"a"}`, `{"roles":1}`, `{"roles":true}`, `{"roles":{}}`,
		`{"roles":{"data":"a"}}`, `{"roles":[1]}`, `{"roles":[{"attributes":[]}]}`,
	} {
		list := models.RoleList{Roles: []models.RoleRead{{Key: "old"}}}
		err := json.Unmarshal([]byte(data), &list)
		if err == nil || !strings.Contains(err.Error(), "the roles of a RoleList") ||
			!cmp.Equal(list, models.RoleList{Roles: []models.RoleRead{{Key: "old"}}}) {
			t.Errorf("%s decodes to %+v and %v, want an error that names the roles of a "+
				"RoleList and the RoleList as it was", data, list, err)
		}
	}
}

// TestRoleListNullLeavesItAsItIs checks that null leaves a RoleList as it is, as encoding/json
// leaves a struct for null, both on its own and in a field.
func TestRoleListNullLeavesItAsItIs(t *testing.T) {
	list := models.RoleList{Roles: []models.RoleRead{{Key: "old"}}}
	if err := list.UnmarshalJSON([]byte("null")); err != nil || len(list.Roles) != 1 {
		t.Errorf("null decodes to %+v and %v, want the RoleList as it was", list, err)
	}
	var reply struct {
		List models.RoleList `json:"list"`
	}
	reply.List = list
	if err := json.Unmarshal([]byte(`{"list":null}`), &reply); err != nil ||
		len(reply.List.Roles) != 1 {
		t.Errorf("a field of null decodes to %+v and %v, want the RoleList as it was",
			reply.List, err)
	}
}
