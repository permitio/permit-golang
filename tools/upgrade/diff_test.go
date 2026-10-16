package upgrade_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/permitio/permit-golang/v2/tools/upgrade"
)

// decode returns the JSON value of text, as Decode returns it, and fails t when text is not one.
func decode(t *testing.T, text string) any {
	t.Helper()
	v, err := upgrade.Decode([]byte(text))
	if err != nil {
		t.Fatalf("Decode(%s): %v", text, err)
	}
	return v
}

func TestDiff(t *testing.T) {
	for _, tc := range []struct {
		name          string
		written, read string
		unordered     []string
		want          []string
	}{
		{
			name:    "equal values, and members that only v2 read",
			written: `{"key":"k","attributes":{"a":[1,"x",true,null,{}]},"empty":[]}`,
			read: `{"id":"i","key":"k","attributes":{"a":[1,"x",true,null,{"more":1}],` +
				`"b":2},"empty":[]}`,
		},
		{
			name:    "a member that v2 did not read",
			written: `{"key":"k","description":"d"}`, read: `{"key":"k"}`,
			want: []string{`#/description: v1 wrote "d", v2 read nothing`},
		},
		{
			name:    "a nested member, whose name a pointer escapes",
			written: `{"attributes":{"a/b~c":{"d":null}}}`,
			read:    `{"attributes":{"a/b~c":{}}}`,
			want:    []string{`#/attributes/a~1b~0c/d: v1 wrote null, v2 read nothing`},
		},
		{
			name:    "another string, boolean and null",
			written: `{"s":"Zoë","b":true,"n":null,"html":"<a&b>"}`,
			read:    `{"s":"Zoe","b":false,"n":"null","html":"<a&b>"}`,
			want: []string{
				`#/b: v1 wrote true, v2 read false`,
				`#/n: v1 wrote null, v2 read "null"`,
				`#/s: v1 wrote "Zoë", v2 read "Zoe"`,
			},
		},
		{
			name:    "another kind",
			written: `{"o":{"a":1},"a":[1],"n":1,"s":"1"}`,
			read:    `{"o":[1],"a":{"0":1},"n":"1","s":1}`,
			want: []string{
				`#/a: v1 wrote [1], v2 read {"0":1}`,
				`#/n: v1 wrote 1, v2 read "1"`,
				`#/o: v1 wrote {"a":1}, v2 read [1]`,
				`#/s: v1 wrote "1", v2 read 1`,
			},
		},
		{
			name:    "the same decimal values in other texts",
			written: `[1.5e3,0.1,-0,100,1E2]`, read: `[1500.0,0.10,0,1e2,100]`,
		},
		{
			name:    "integers beyond 2^53, which a float64 cannot tell apart",
			written: `{"seats":9007199254740993}`, read: `{"seats":9007199254740992}`,
			want: []string{`#/seats: v1 wrote 9007199254740993, v2 read 9007199254740992`},
		},
		{
			name:    "an array of another length",
			written: `{"tags":["a","b"]}`, read: `{"tags":["a"]}`,
			want: []string{`#/tags: v1 wrote ["a","b"], v2 read ["a"]`},
		},
		{
			name:    "an array in another order",
			written: `{"tags":["a","b"]}`, read: `{"tags":["b","a"]}`,
			want: []string{
				`#/tags/0: v1 wrote "a", v2 read "b"`, `#/tags/1: v1 wrote "b", v2 read "a"`,
			},
		},
		{
			name:    "an unordered array in another order",
			written: `{"permissions":["a:read","b:list"]}`,
			read:    `{"permissions":["b:list","a:read"]}`, unordered: []string{"#/permissions"},
		},
		{
			name:    "an unordered array with another element",
			written: `{"permissions":["a:read","a:read"]}`,
			read:    `{"permissions":["a:read","b:list"]}`, unordered: []string{"#/permissions"},
			want: []string{
				`#/permissions: v1 wrote ["a:read","a:read"] in any order, v2 read ` +
					`["a:read","b:list"]`,
			},
		},
		{
			name:    "an unordered array of objects, in another order and with more members",
			written: `{"rules":[{"url":"a"},{"url":"b","action":"read"}]}`,
			read: `{"rules":[{"url":"b","action":"read","priority":0},` +
				`{"url":"a","action":null}]}`,
			unordered: []string{"#/rules"},
		},
		{
			name: "an unordered array whose first element matches the element that the second " +
				"needs",
			written:   `[{"a":1},{"a":1,"b":2}]`,
			read:      `[{"a":1,"b":2},{"a":1,"c":3}]`,
			unordered: []string{"#"},
		},
		{
			name:      "an unordered array whose elements match one element of read alone",
			written:   `[{"a":1},{"a":1,"b":2}]`,
			read:      `[{"a":1,"b":2},{"c":3}]`,
			unordered: []string{"#"},
			want: []string{
				`#: v1 wrote [{"a":1},{"a":1,"b":2}] in any order, v2 read [{"a":1,"b":2},{"c":3}]`,
			},
		},
		{
			name:    "an unordered array of another length",
			written: `{"extends":["a"]}`, read: `{"extends":["a","b"]}`,
			unordered: []string{"#/extends"},
			want:      []string{`#/extends: v1 wrote ["a"], v2 read ["a","b"]`},
		},
		{
			name:    "an array at another pointer than the unordered one",
			written: `{"roles":{"permissions":["a","b"]}}`,
			read:    `{"roles":{"permissions":["b","a"]}}`, unordered: []string{"#/permissions"},
			want: []string{
				`#/roles/permissions/0: v1 wrote "a", v2 read "b"`,
				`#/roles/permissions/1: v1 wrote "b", v2 read "a"`,
			},
		},
		{
			name:    "the elements of a body that is an array",
			written: `[{"user":"u","role":"r"}]`, read: `[{"user":"u"}]`,
			want: []string{`#/0/role: v1 wrote "r", v2 read nothing`},
		},
		{
			name:    "nothing read",
			written: `{"key":"k"}`, read: `null`,
			want: []string{`#: v1 wrote {"key":"k"}, v2 read null`},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := upgrade.Diff(decode(t, tc.written), decode(t, tc.read), tc.unordered...)
			if !slices.Equal(got, tc.want) {
				t.Errorf("Diff(%s, %s) =\n%s\nwant\n%s", tc.written, tc.read,
					strings.Join(got, "\n"), strings.Join(tc.want, "\n"))
			}
		})
	}
}

// TestDiffDoesNotChangeItsValues checks that Diff leaves the values that it compares as they
// are, so that a step may compare the same body with what v2 read more than once.
func TestDiffDoesNotChangeItsValues(t *testing.T) {
	const text = `{"permissions":["b","a"],"n":1.50}`
	written := decode(t, text)
	upgrade.Diff(written, decode(t, `{"permissions":["a","b"],"n":1.5}`), "#/permissions")
	if data, err := json.Marshal(written); err != nil || string(data) != `{"n":1.50,`+
		`"permissions":["b","a"]}` {
		t.Errorf("after Diff, the written value is %s, %v; want it as Decode read %s", data, err,
			text)
	}
}

func TestDecode(t *testing.T) {
	v := decode(t, `{"seats":9007199254740993,"ratio":1.50}`)
	m, ok := v.(map[string]any)
	if !ok || m["seats"] != json.Number("9007199254740993") || m["ratio"] != json.Number("1.50") {
		t.Errorf("Decode returned %#v, want each number as a json.Number with its text", v)
	}
	for _, text := range []string{``, `{"a":1} {"b":2}`, `{"a":`} {
		if v, err := upgrade.Decode([]byte(text)); err == nil {
			t.Errorf("Decode(%q) = %#v, want an error: it is not one JSON value", text, v)
		}
	}
}

// TestObjects checks that Objects returns the one object of a body that is not an array, or the
// elements of one that is, and fails on another number of bodies than one, on a body that is not
// JSON, and on another number of objects than the step reads.
func TestObjects(t *testing.T) {
	for _, tc := range []struct {
		name    string
		bodies  []string
		n       int
		want    string
		wantErr string
	}{
		{name: "an object", bodies: []string{`{"key":"k"}`}, n: 1, want: `[{"key":"k"}]`},
		{
			name: "the elements of an array", bodies: []string{`[{"a":1.50},{"b":[]}]`}, n: 2,
			want: `[{"a":1.50},{"b":[]}]`,
		},
		{
			name: "the element of an array of one", bodies: []string{`[{"a":1}]`}, n: 1,
			want: `[{"a":1}]`,
		},
		{name: "an empty array", bodies: []string{`[]`}, n: 0, want: `[]`},
		{name: "no body", n: 1, wantErr: "v1 sent 0 request bodies, want 1"},
		{
			name: "two bodies", bodies: []string{`{"key":"a"}`, `{"key":"b"}`}, n: 1,
			wantErr: "v1 sent 2 request bodies, want 1",
		},
		{
			name: "a body that is not JSON", bodies: []string{`{"key":`}, n: 1,
			wantErr: `decoding what v1 wrote, {"key":: `,
		},
		{
			name: "an object, where the step reads two", bodies: []string{`{"key":"k"}`}, n: 2,
			wantErr: `v1 wrote 1 objects, {"key":"k"}, want 2`,
		},
		{
			name: "an array of another length", bodies: []string{`[{},{}]`}, n: 1,
			wantErr: "v1 wrote 2 objects, [{},{}], want 1",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var bodies [][]byte
			for _, body := range tc.bodies {
				bodies = append(bodies, []byte(body))
			}
			objects, err := upgrade.Objects(bodies, tc.n)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("Objects = %#v, %v; want an error with %q", objects, err, tc.wantErr)
				}
				return
			}
			got, jsonErr := json.Marshal(objects)
			if err != nil || jsonErr != nil || string(got) != tc.want {
				t.Errorf("Objects = %s, %v, %v; want %s", got, err, jsonErr, tc.want)
			}
		})
	}
}

// TestWithout checks that Without leaves out the members it names, without changing what it is
// given, returns what it is given when it names none, and fails when what it is given is not an
// object or lacks a member that it names.
func TestWithout(t *testing.T) {
	for _, tc := range []struct {
		name    string
		written string
		members []string
		want    string
		wantErr string
	}{
		{
			name:    "members that v1 wrote",
			written: `{"key":"k","is_role":false,"is_resource":false,"n":1.50}`,
			members: []string{"is_resource", "is_role"}, want: `{"key":"k","n":1.50}`,
		},
		{name: "no members, of a value that is not an object", written: `[1]`, want: `[1]`},
		{
			name: "a member that v1 did not write", written: `{"key":"k","is_role":false}`,
			members: []string{"is_resource", "is_role"},
			wantErr: `v1 did not write ["is_resource"], which the comparison would leave out`,
		},
		{
			name: "an array", written: `[{"is_role":false}]`, members: []string{"is_role"},
			wantErr: `v1 wrote [{"is_role":false}], not an object with the members ["is_role"]`,
		},
		{
			name: "null", written: `null`, members: []string{"is_role"},
			wantErr: `v1 wrote null, not an object`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			written := decode(t, tc.written)
			before, _ := json.Marshal(written)
			got, err := upgrade.Without(written, tc.members...)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("Without = %#v, %v; want an error with %q", got, err, tc.wantErr)
				}
				return
			}
			data, jsonErr := json.Marshal(got)
			if err != nil || jsonErr != nil || string(data) != tc.want {
				t.Errorf("Without = %s, %v, %v; want %s", data, err, jsonErr, tc.want)
			}
			if after, _ := json.Marshal(written); string(after) != string(before) {
				t.Errorf("after Without, what it was given is %s, want %s", after, before)
			}
		})
	}
}

// tenantRead stands in for a model of v2 that decodes a tenant: Key and Attributes, and a
// Description that it drops, as a model with a wrong or missing JSON tag would.
type tenantRead struct {
	Key         string         `json:"key"`
	Description string         `json:"-"`
	Attributes  map[string]any `json:"attributes,omitzero"`
}

// TestViewOfAModelThatDropsAField checks that a field that v1 wrote and that a model of v2 does
// not decode is missing from the model's view, so that Diff reports it.
func TestViewOfAModelThatDropsAField(t *testing.T) {
	written := decode(t, `{"key":"t1","description":"Zoë's","attributes":{"n":9007199254740993}}`)
	model := tenantRead{
		Key: "t1", Description: "Zoë's",
		Attributes: map[string]any{"n": json.Number("9007199254740993")},
	}
	view, err := upgrade.View(model)
	if err != nil {
		t.Fatalf("View: %v", err)
	}

	got := upgrade.Diff(written, view)

	want := []string{`#/description: v1 wrote "Zoë's", v2 read nothing`}
	if !slices.Equal(got, want) {
		t.Errorf("Diff of the view of a model that drops the description = %q, want %q", got,
			want)
	}
}

func TestViewOfAModelThatDoesNotEncode(t *testing.T) {
	if v, err := upgrade.View(map[string]any{"c": make(chan int)}); err == nil ||
		!strings.Contains(err.Error(), "encoding the map[string]interface {} that v2 read") {
		t.Errorf("View of a channel = %#v, %v; want an error that names the model's type", v, err)
	}
}
