package models_test

import (
	"bytes"
	"encoding/json"
	"slices"
	"testing"

	"github.com/permitio/permit-golang/v2/internal/wire"
	"github.com/permitio/permit-golang/v2/models"
)

// jwksBody is the body of a JwksConfig with only the fields of a StringOrList.
type jwksBody struct {
	Audience models.StringOrList `json:"audience,omitzero"`
	Issuer   models.StringOrList `json:"issuer,omitzero"`
}

// TestStringOrListEncodes checks the JSON of each form of a StringOrList in a body: one string as
// a JSON string, the empty string and text that an encoder may escape too; a list as a JSON
// array, of one string and of none too; and unset left out of the body.
func TestStringOrListEncodes(t *testing.T) {
	for _, tc := range []struct {
		body jwksBody
		want string
	}{
		{jwksBody{}, `{}`},
		{jwksBody{Audience: models.OneString("app")}, `{"audience":"app"}`},
		{jwksBody{Audience: models.OneString("")}, `{"audience":""}`},
		{jwksBody{Issuer: models.OneString("<a> & b")}, `{"issuer":"<a> & b"}`},
		{jwksBody{Audience: models.StringList()}, `{"audience":[]}`},
		{jwksBody{Audience: models.StringList("app")}, `{"audience":["app"]}`},
		{
			jwksBody{Audience: models.StringList("a", "b"), Issuer: models.OneString("i")},
			`{"audience":["a","b"],"issuer":"i"}`,
		},
	} {
		got, err := wire.Encode(tc.body)
		if err != nil || string(got) != tc.want {
			t.Errorf("Encode = %s, %v; want %s", got, err, tc.want)
		}
	}
}

// TestUnsetStringOrListDoesNotEncode checks that an unset StringOrList in a field without
// omitzero fails to encode, rather than send a value that the caller did not set.
func TestUnsetStringOrListDoesNotEncode(t *testing.T) {
	body := struct {
		Audience models.StringOrList `json:"audience"`
	}{}
	if got, err := wire.Encode(body); err == nil {
		t.Errorf("Encode of an unset StringOrList without omitzero = %s, want an error", got)
	}
}

// TestStringOrListDecodes checks what each JSON value decodes to: a string as one string, an
// array of strings as a list, of one string and of none too, and null as unset, also with
// whitespace around the value, which encoding/json does not pass to UnmarshalJSON; and that it
// encodes back as it came.
func TestStringOrListDecodes(t *testing.T) {
	for _, tc := range []struct {
		data string
		want models.StringOrList
	}{
		{`"app"`, models.OneString("app")},
		{`""`, models.OneString("")},
		{`" <&>"`, models.OneString(" <&>")},
		{`["app"]`, models.StringList("app")},
		{`[ "a" , "b" ]`, models.StringList("a", "b")},
		{`[]`, models.StringList()},
		{`null`, models.StringOrList{}},
		{" \"app\"\n", models.OneString("app")},
		{"\t[ \"a\" ] ", models.StringList("a")},
		{" null ", models.StringOrList{}},
	} {
		var got models.StringOrList
		if err := json.Unmarshal([]byte(tc.data), &got); err != nil || !got.Equal(tc.want) {
			t.Errorf("Unmarshal(%s) = %v, %v; want %v", tc.data, got.Values(), err,
				tc.want.Values())
			continue
		}
		if got.IsZero() {
			continue
		}
		encoded, err := wire.Encode(got)
		var again models.StringOrList
		if err != nil || json.Unmarshal(encoded, &again) != nil || !again.Equal(got) {
			t.Errorf("%s decodes and encodes as %s, %v, which does not decode to the same "+
				"value", tc.data, encoded, err)
		}
	}
}

// TestStringOrListNullUnsets checks that null makes a value that is set unset: given to
// UnmarshalJSON, and in a body that json.Unmarshal decodes into a value whose fields are set.
func TestStringOrListNullUnsets(t *testing.T) {
	for _, set := range []models.StringOrList{models.OneString("kept"), models.StringList("a")} {
		got := set
		if err := got.UnmarshalJSON([]byte("null")); err != nil || !got.IsZero() {
			t.Errorf("UnmarshalJSON(null) of %q (list %t) = %v and the value %q, want nil and "+
				"unset", set.Values(), set.IsList(), err, got.Values())
		}
	}
	body := jwksBody{Audience: models.OneString("kept"), Issuer: models.StringList("a")}
	err := json.Unmarshal([]byte(`{"audience": null, "issuer": null}`), &body)
	if err != nil || !body.Audience.IsZero() || !body.Issuer.IsZero() {
		t.Errorf("Unmarshal of a null audience and issuer = %v, the audience %q and the issuer "+
			"%q; want both unset", err, body.Audience.Values(), body.Issuer.Values())
	}
}

// TestStringOrListRejects checks that a JSON value that is neither a string nor an array of
// strings is an error, and leaves the value as it was.
func TestStringOrListRejects(t *testing.T) {
	for _, data := range []string{
		`1`, `true`, `{}`, `{"a":"b"}`, `[1]`, `[null]`, `["a",2]`, `[["a"]]`, `"a`, `["a"`,
	} {
		got := models.OneString("kept")
		err := got.UnmarshalJSON([]byte(data))
		if err == nil || !got.Equal(models.OneString("kept")) {
			t.Errorf("UnmarshalJSON(%s) = %v and the value %v, want an error and the value kept",
				data, err, got.Values())
		}
	}
	var body jwksBody
	if err := json.Unmarshal([]byte(`{"audience":7}`), &body); err == nil {
		t.Errorf("Unmarshal of an audience that is a number = %v, want an error", body)
	}
}

// TestStringOrListAccessors checks Values, IsList, IsZero and Equal in each form, and that
// Values and StringList copy the strings, so that a change to either slice changes no value.
func TestStringOrListAccessors(t *testing.T) {
	for _, tc := range []struct {
		name         string
		v            models.StringOrList
		values       []string
		list, isZero bool
	}{
		{name: "unset", v: models.StringOrList{}, isZero: true},
		{name: "one", v: models.OneString("a"), values: []string{"a"}},
		{name: "list", v: models.StringList("a", "b"), values: []string{"a", "b"}, list: true},
		{name: "empty list", v: models.StringList(), values: []string{}, list: true},
	} {
		got := tc.v.Values()
		if !slices.Equal(got, tc.values) || (got == nil) != (tc.values == nil) ||
			tc.v.IsList() != tc.list || tc.v.IsZero() != tc.isZero || !tc.v.Equal(tc.v) {
			t.Errorf("%s: Values %q, IsList %t, IsZero %t; want %q, %t, %t", tc.name, got,
				tc.v.IsList(), tc.v.IsZero(), tc.values, tc.list, tc.isZero)
		}
	}
	for _, pair := range [][2]models.StringOrList{
		{models.OneString("a"), models.StringList("a")},
		{models.StringOrList{}, models.StringList()},
		{models.StringOrList{}, models.OneString("")},
		{models.StringList("a", "b"), models.StringList("b", "a")},
		{models.StringList("a"), models.StringList("a", "a")},
	} {
		if pair[0].Equal(pair[1]) || pair[1].Equal(pair[0]) {
			t.Errorf("%v (list %t) and %v (list %t) are Equal, want them to differ",
				pair[0].Values(), pair[0].IsList(), pair[1].Values(), pair[1].IsList())
		}
	}
	given := []string{"a", "b"}
	v := models.StringList(given...)
	given[0] = "changed"
	v.Values()[1] = "changed"
	if !v.Equal(models.StringList("a", "b")) {
		t.Errorf("a change to the slice given to StringList or returned by Values changed the "+
			"value to %q", v.Values())
	}
}

// TestStringOrListErrors checks that the errors of a StringOrList say what it takes.
func TestStringOrListErrors(t *testing.T) {
	var v models.StringOrList
	err := v.UnmarshalJSON([]byte(`1`))
	if err == nil || err.Error() != "models: a StringOrList is a JSON string or a JSON array of "+
		"strings" {
		t.Errorf("UnmarshalJSON(1) = %v, want the error that says what it takes", err)
	}
	_, err = models.StringOrList{}.MarshalJSON()
	if err == nil || err.Error() != "models: an unset StringOrList has no JSON value; tag its "+
		"field omitzero, so that encoding/json leaves the field out" {
		t.Errorf("MarshalJSON of an unset StringOrList = %v, want the error that says to tag "+
			"its field omitzero", err)
	}
}

// FuzzStringOrList decodes data into a StringOrList with json.Unmarshal, encodes it with
// wire.Encode and decodes the result: the value keeps its form and its strings, and the second
// encoding is the first byte for byte.
func FuzzStringOrList(f *testing.F) {
	for _, seed := range []string{
		`"a"`, `""`, `[]`, `["a"]`, `["a","b"]`, `null`, `1`, `[1]`, `" "`, `["<&>"]`,
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		var first models.StringOrList
		if json.Unmarshal(data, &first) != nil || first.IsZero() {
			return
		}
		encoded, err := wire.Encode(first)
		if err != nil {
			t.Fatalf("%s decodes, and does not encode: %v", data, err)
		}
		var second models.StringOrList
		if err = json.Unmarshal(encoded, &second); err != nil || !second.Equal(first) {
			t.Fatalf("%s encodes as %s, which decodes to %q (list %t), %v; want %q (list %t)",
				data, encoded, second.Values(), second.IsList(), err, first.Values(),
				first.IsList())
		}
		again, err := wire.Encode(second)
		if err != nil || !bytes.Equal(again, encoded) {
			t.Fatalf("%s encodes as %s, and then as %s, %v", data, encoded, again, err)
		}
	})
}
