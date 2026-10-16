package models_test

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/permitio/permit-golang/v2/internal/wire"
	"github.com/permitio/permit-golang/v2/models"
)

// attributesUpdate is the body of an update with a field of type Attributes, which the API does
// not accept null for, tagged omitzero.
type attributesUpdate struct {
	Attributes models.Attributes `json:"attributes,omitzero"`
}

func TestAttributesKeepNumbers(t *testing.T) {
	var got models.Attributes
	err := json.Unmarshal([]byte(`{"big":9007199254740993,"zero":-0,"huge":1e300,`+
		`"pi":3.14159265358979323846,"nested":{"list":[18446744073709551616,{"z":-0.0}]}}`), &got)
	if err != nil {
		t.Fatal(err)
	}
	want := models.Attributes{
		"big":  json.Number("9007199254740993"),
		"zero": json.Number("-0"),
		"huge": json.Number("1e300"),
		"pi":   json.Number("3.14159265358979323846"),
		"nested": map[string]any{
			"list": []any{json.Number("18446744073709551616"), map[string]any{
				"z": json.Number("-0.0"),
			}},
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("json.Unmarshal into Attributes differs (-want +got):\n%s", diff)
	}
}

func TestAttributesNumberMethods(t *testing.T) {
	var got models.Attributes
	if err := json.Unmarshal([]byte(`{"age":9007199254740993,"score":0.5}`), &got); err != nil {
		t.Fatal(err)
	}
	age, ok := got["age"].(json.Number)
	if !ok {
		t.Fatalf("age decodes to %T, want json.Number", got["age"])
	}
	if n, err := age.Int64(); err != nil || n != 9007199254740993 {
		t.Errorf("Int64() = %d, %v, want 9007199254740993", n, err)
	}
	score, ok := got["score"].(json.Number)
	if !ok {
		t.Fatalf("score decodes to %T, want json.Number", got["score"])
	}
	if f, err := score.Float64(); err != nil || f != 0.5 {
		t.Errorf("Float64() = %g, %v, want 0.5", f, err)
	}
}

func TestAttributesDecodeReplacesTheMap(t *testing.T) {
	got := models.Attributes{"old": "kept?"}
	if err := json.Unmarshal([]byte(`{"new":1}`), &got); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(models.Attributes{"new": json.Number("1")}, got); diff != "" {
		t.Errorf("Attributes differ (-want +got):\n%s", diff)
	}

	if err := json.Unmarshal([]byte("null"), &got); err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("null decodes to %v, want nil", got)
	}
}

func TestAttributesDecodeErrors(t *testing.T) {
	for _, body := range []string{`[1]`, `"x"`, `1`, `true`} {
		got := models.Attributes{"k": "v"}
		if err := json.Unmarshal([]byte(body), &got); err == nil {
			t.Errorf("%s decodes into Attributes without an error", body)
		}
		if diff := cmp.Diff(models.Attributes{"k": "v"}, got); diff != "" {
			t.Errorf("a failed decode of %s changed the Attributes (-want +got):\n%s", body, diff)
		}
	}
}

func TestAttributesGolden(t *testing.T) {
	bodies := []attributesUpdate{
		{},
		{Attributes: models.Attributes{}},
		{Attributes: models.Attributes{"k": "v"}},
	}
	const name = "attributes.golden"
	want := readGolden(t, name)
	if len(want) != len(bodies) {
		t.Fatalf("testdata/golden/%s has %d lines, want %d: nil, empty and a value", name,
			len(want), len(bodies))
	}
	for i, body := range bodies {
		got, err := wire.Encode(body)
		if err != nil {
			t.Errorf("Encode of the body on line %d of %s: %v", i+1, name, err)
			continue
		}
		if string(got) != want[i] {
			t.Errorf("Encode = %s, want %s, line %d of %s", got, want[i], i+1, name)
		}
		var decoded attributesUpdate
		if err := json.Unmarshal([]byte(want[i]), &decoded); err != nil {
			t.Errorf("line %d of %s does not decode: %v", i+1, name, err)
			continue
		}
		// cmp.Equal tells a nil map from an empty one.
		if !cmp.Equal(decoded.Attributes, body.Attributes) {
			t.Errorf("line %d of %s decodes to %#v, want %#v", i+1, name, decoded.Attributes,
				body.Attributes)
		}
	}
}
