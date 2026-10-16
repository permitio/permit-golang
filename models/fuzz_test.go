package models_test

import (
	"bytes"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/permitio/permit-golang/v2/internal/wire"
	"github.com/permitio/permit-golang/v2/models"
)

// FuzzOptional decodes data, a body with an Optional field that holds any JSON value, with
// json.Unmarshal, which keeps no number as a json.Number on its own, encodes the body with
// wire.Encode, and decodes the result: the value holds no float64, the field keeps its state and
// its value, and the second encoding is the first byte for byte. go test runs the seeds in
// testdata/fuzz/FuzzOptional.
func FuzzOptional(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		var first update[any]
		if json.Unmarshal(data, &first) != nil {
			return
		}
		value, _ := first.Description.Get()
		if path, ok := findFloat(value, "description"); ok {
			t.Fatalf("%s decodes with the float64 %s; a number must be a json.Number", data,
				path)
		}
		encoded, err := wire.Encode(first)
		if err != nil {
			t.Fatalf("%s decodes, and its body does not encode: %v", data, err)
		}
		var second update[any]
		if err = json.Unmarshal(encoded, &second); err != nil {
			t.Fatalf("%s encodes as %s, which does not decode: %v", data, encoded, err)
		}
		if !sameOptional(first.Description, second.Description) {
			firstValue, _ := first.Description.Get()
			secondValue, _ := second.Description.Get()
			t.Fatalf("%s encodes as %s, which decodes to another state or value "+
				"(-first +second):\n%s", data, encoded, cmp.Diff(firstValue, secondValue))
		}
		again, err := wire.Encode(second)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(again, encoded) {
			t.Fatalf("%s encodes as %s, and then as %s", data, encoded, again)
		}
	})
}

// FuzzAttributes decodes data into Attributes with json.Unmarshal, which keeps no number as a
// json.Number on its own, encodes the Attributes with wire.Encode, and decodes the result: the
// Attributes hold no float64 and keep every value, and the second encoding is the first byte for
// byte. go test runs the seeds in testdata/fuzz/FuzzAttributes.
func FuzzAttributes(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		var first models.Attributes
		if json.Unmarshal(data, &first) != nil {
			return
		}
		if path, ok := findFloat(first, "attributes"); ok {
			t.Fatalf("%s decodes with the float64 %s; a number must be a json.Number", data,
				path)
		}
		encoded, err := wire.Encode(first)
		if err != nil {
			t.Fatalf("%s decodes, and does not encode: %v", data, err)
		}
		var second models.Attributes
		if err = json.Unmarshal(encoded, &second); err != nil {
			t.Fatalf("%s encodes as %s, which does not decode: %v", data, encoded, err)
		}
		if diff := cmp.Diff(first, second); diff != "" {
			t.Fatalf("%s encodes as %s, which decodes to other values (-first +second):\n%s",
				data, encoded, diff)
		}
		again, err := wire.Encode(second)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(again, encoded) {
			t.Fatalf("%s encodes as %s, and then as %s", data, encoded, again)
		}
	})
}

// findFloat returns the path of the first float64 in value, which is at path, and true, or ""
// and false when value holds none.
func findFloat(value any, path string) (string, bool) {
	switch v := value.(type) {
	case float64:
		return path, true
	case models.Attributes:
		return findFloat(map[string]any(v), path)
	case map[string]any:
		for key, element := range v {
			if found, ok := findFloat(element, path+"."+key); ok {
				return found, true
			}
		}
	case []any:
		for i, element := range v {
			if found, ok := findFloat(element, path+"["+strconv.Itoa(i)+"]"); ok {
				return found, true
			}
		}
	}
	return "", false
}
