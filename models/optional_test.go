package models_test

import (
	"encoding/json"
	"maps"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/permitio/permit-golang/v2/internal/testkit/fixtures"
	"github.com/permitio/permit-golang/v2/internal/wire"
	"github.com/permitio/permit-golang/v2/models"
)

// update is the body of an update with one Optional field, tagged as every Optional field is.
type update[T any] struct {
	Description models.Optional[T] `json:"description,omitzero"`
}

// item is a struct that an Optional holds.
type item struct {
	Key string `json:"key"`
}

// readGolden returns the lines of testdata/golden/<name>. A golden file holds one body on each
// line, and ends with a newline.
func readGolden(t *testing.T, name string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "golden", name))
	if err != nil {
		t.Fatal(err)
	}
	text, ok := strings.CutSuffix(string(data), "\n")
	if !ok {
		t.Fatalf("testdata/golden/%s does not end with a newline", name)
	}
	return strings.Split(text, "\n")
}

// sameOptional reports whether a and b are in the same state and hold equal values.
func sameOptional[T any](a, b models.Optional[T]) bool {
	aValue, aSet := a.Get()
	bValue, bSet := b.Get()
	return a.IsZero() == b.IsZero() && a.IsNull() == b.IsNull() && aSet == bSet &&
		cmp.Equal(aValue, bValue)
}

// checkOptionalGolden checks the bodies of an update[T] that is unset, null, holds empty and
// holds value against the lines of testdata/golden/optional-<kind>.golden: wire.Encode, the
// encoder of the SDK's request bodies, writes each as its line, and each line decodes back into
// its body.
func checkOptionalGolden[T any](t *testing.T, kind string, empty, value T) {
	t.Helper()
	bodies := []update[T]{
		{},
		{Description: models.Null[T]()},
		{Description: models.Some(empty)},
		{Description: models.Some(value)},
	}
	name := "optional-" + kind + ".golden"
	want := readGolden(t, name)
	if len(want) != len(bodies) {
		t.Fatalf("testdata/golden/%s has %d lines, want %d: unset, null, empty and a value",
			name, len(want), len(bodies))
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
		var decoded update[T]
		if err := json.Unmarshal([]byte(want[i]), &decoded); err != nil {
			t.Errorf("line %d of %s does not decode: %v", i+1, name, err)
			continue
		}
		if !sameOptional(decoded.Description, body.Description) {
			t.Errorf("line %d of %s decodes to another state or value", i+1, name)
		}
	}
}

func TestOptionalGolden(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		checkOptionalGolden(t, "string", "", "v")
	})
	t.Run("int", func(t *testing.T) {
		checkOptionalGolden(t, "int", int64(0), fixtures.TwoPow53Plus1)
	})
	t.Run("bool", func(t *testing.T) {
		checkOptionalGolden(t, "bool", false, true)
	})
	t.Run("map", func(t *testing.T) {
		checkOptionalGolden(t, "map", map[string]any{}, map[string]any{"k": "v"})
	})
	t.Run("slice", func(t *testing.T) {
		checkOptionalGolden(t, "slice", []string{}, []string{"v"})
	})
	t.Run("struct", func(t *testing.T) {
		checkOptionalGolden(t, "struct", item{}, item{Key: "v"})
	})
}

func TestOptionalStates(t *testing.T) {
	for _, tc := range []struct {
		name               string
		optional           models.Optional[string]
		zero, null, hasSet bool
		value              string
	}{
		{name: "unset", zero: true},
		{name: "null", optional: models.Null[string](), null: true},
		{name: "empty", optional: models.Some(""), hasSet: true},
		{name: "value", optional: models.Some("v"), hasSet: true, value: "v"},
	} {
		value, ok := tc.optional.Get()
		if value != tc.value || ok != tc.hasSet {
			t.Errorf("%s: Get() = %q, %t, want %q, %t", tc.name, value, ok, tc.value, tc.hasSet)
		}
		if got := tc.optional.IsZero(); got != tc.zero {
			t.Errorf("%s: IsZero() = %t, want %t", tc.name, got, tc.zero)
		}
		if got := tc.optional.IsNull(); got != tc.null {
			t.Errorf("%s: IsNull() = %t, want %t", tc.name, got, tc.null)
		}
	}
}

func TestUnsetOptionalWithoutOmitzeroFailsToEncode(t *testing.T) {
	for name, body := range map[string]any{
		"no omitzero": struct {
			Description models.Optional[string] `json:"description"`
		}{},
		"omitempty": struct {
			//nolint:modernize // The test checks that omitempty does not leave the field out.
			Description models.Optional[string] `json:"description,omitempty"`
		}{},
		"an unset Optional alone": models.Optional[int]{},
	} {
		got, err := wire.Encode(body)
		if err == nil {
			t.Errorf("%s: Encode = %s, nil; want an error, not null in place of leaving the "+
				"field out", name, got)
			continue
		}
		if !strings.Contains(err.Error(), "tag its field omitzero") {
			t.Errorf("%s: Encode returned %q, which does not say to tag the field omitzero",
				name, err)
		}
	}
}

func TestSomeNeverSendsNull(t *testing.T) {
	for name, body := range map[string]any{
		"a nil slice":   update[[]string]{Description: models.Some([]string(nil))},
		"a nil map":     update[map[string]any]{Description: models.Some(map[string]any(nil))},
		"a nil pointer": update[*item]{Description: models.Some[*item](nil)},
		"a nil any":     update[any]{Description: models.Some[any](nil)},
	} {
		got, err := wire.Encode(body)
		if err == nil {
			t.Errorf("Some of %s: Encode = %s, nil; want an error, as only Null sends null",
				name, got)
			continue
		}
		if !strings.Contains(err.Error(), "use Null to send null") {
			t.Errorf("Some of %s: Encode returned %q, which does not say to use Null", name,
				err)
		}
	}
}

func TestOptionalLeavesHTMLEscapingToTheEncoder(t *testing.T) {
	body := update[string]{Description: models.Some(fixtures.HTMLSpecial)}
	sent, err := wire.Encode(body)
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"description":"<>&"}`; string(sent) != want {
		t.Errorf("wire.Encode = %s, want %s", sent, want)
	}
	escaped, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"description":"\u003c\u003e\u0026"}`; string(escaped) != want {
		t.Errorf("json.Marshal = %s, want %s", escaped, want)
	}
}

func TestOptionalDecodesNullOutsideAStruct(t *testing.T) {
	got := models.Some("v")
	if err := json.Unmarshal([]byte("null"), &got); err != nil {
		t.Fatal(err)
	}
	if !got.IsNull() {
		t.Errorf("null decodes to %+v, want a null Optional", got)
	}
}

func TestOptionalDecodeErrors(t *testing.T) {
	for _, body := range []string{
		`{"description":1}`,
		`{"description":true}`,
		`{"description":["v"]}`,
		`{"description":{"key":"v"}}`,
	} {
		var got update[string]
		if err := json.Unmarshal([]byte(body), &got); err == nil {
			t.Errorf("%s decodes into an Optional[string] without an error", body)
		}
	}
	var got update[int64]
	if err := json.Unmarshal([]byte(`{"description":1.5}`), &got); err == nil {
		t.Error(`{"description":1.5} decodes into an Optional[int64] without an error`)
	}
}

// pointerMarshaler encodes through a MarshalJSON on its pointer, as a hand-written union does.
type pointerMarshaler struct{ n int }

func (p *pointerMarshaler) MarshalJSON() ([]byte, error) {
	return []byte(`{"n":` + strconv.Itoa(p.n) + `}`), nil
}

// TestSomeUsesAPointerMarshaler checks that an Optional encodes its value as a field of the same
// type does, also when the type's MarshalJSON is on its pointer.
func TestSomeUsesAPointerMarshaler(t *testing.T) {
	body := update[pointerMarshaler]{Description: models.Some(pointerMarshaler{n: 5})}
	data, err := wire.Encode(body)
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"description":{"n":5}}`; string(data) != want {
		t.Errorf("Encode = %s, want %s", data, want)
	}
}

// TestOptionalDecodeErrorNamesItsType checks that a value that does not decode says what type it
// was for.
func TestOptionalDecodeErrorNamesItsType(t *testing.T) {
	var got update[int64]
	err := json.Unmarshal([]byte(`{"description":"x"}`), &got)
	if err == nil || !strings.Contains(err.Error(), "Optional[int64]") {
		t.Errorf("the error is %v, want one that names Optional[int64]", err)
	}
}

// hostile is an update body with values that a careless encoder or decoder changes.
type hostile struct {
	Big        models.Optional[int64]   `json:"big,omitzero"`
	Zero       models.Optional[float64] `json:"zero,omitzero"`
	Huge       models.Optional[float64] `json:"huge,omitzero"`
	Text       models.Optional[string]  `json:"text,omitzero"`
	Value      models.Optional[any]     `json:"value,omitzero"`
	Attributes models.Attributes        `json:"attributes,omitzero"`
}

func TestHostileValuesSurviveDecodeAndEncode(t *testing.T) {
	numbers := map[string]any{
		"big":  json.Number("9007199254740993"),
		"zero": json.Number("-0"),
		"huge": json.Number("1e300"),
	}
	attributes := models.Attributes{"text": fixtures.HTMLSpecial + fixtures.LineSeparator}
	maps.Copy(attributes, numbers)
	text := fixtures.HTMLSpecial + fixtures.LineSeparator + "é"
	body := hostile{
		Big:        models.Some(fixtures.TwoPow53Plus1),
		Zero:       models.Some(fixtures.NegativeZero()),
		Huge:       models.Some(fixtures.Huge),
		Text:       models.Some(text),
		Value:      models.Some[any](numbers),
		Attributes: attributes,
	}
	want := readGolden(t, "hostile.golden")[0]
	sent, err := wire.Encode(body)
	if err != nil {
		t.Fatal(err)
	}
	if string(sent) != want {
		t.Errorf("Encode = %s, want %s", sent, want)
	}

	var got hostile
	if err = json.Unmarshal([]byte(want), &got); err != nil {
		t.Fatal(err)
	}
	if v, _ := got.Big.Get(); v != fixtures.TwoPow53Plus1 {
		t.Errorf("big decodes to %d, want %d", v, fixtures.TwoPow53Plus1)
	}
	if v, _ := got.Zero.Get(); v != 0 || !math.Signbit(v) {
		t.Errorf("zero decodes to %g, want -0", v)
	}
	if v, _ := got.Huge.Get(); v != fixtures.Huge {
		t.Errorf("huge decodes to %g, want %g", v, fixtures.Huge)
	}
	if v, _ := got.Text.Get(); v != text {
		t.Errorf("text decodes to %q, want %q", v, text)
	}
	value, _ := got.Value.Get()
	if diff := cmp.Diff(any(numbers), value); diff != "" {
		t.Errorf("value decodes to another value (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(attributes, got.Attributes); diff != "" {
		t.Errorf("attributes decode to other values (-want +got):\n%s", diff)
	}

	again, err := wire.Encode(got)
	if err != nil {
		t.Fatal(err)
	}
	if string(again) != want {
		t.Errorf("the decoded body encodes as %s, want %s", again, want)
	}
}
