package wire

import (
	"encoding/json"
	"errors"
	"math"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/permitio/permit-golang/v2/internal/testkit/fixtures"
)

// hostileBody holds values that a careless encoder or decoder changes.
type hostileBody struct {
	Big       int64          `json:"big"`
	Max       int64          `json:"max"`
	Huge      float64        `json:"huge"`
	Zero      float64        `json:"zero"`
	HTML      string         `json:"html"`
	Separator string         `json:"separator"`
	Typed     string         `json:"typed"`
	Attrs     map[string]any `json:"attrs"`
}

func TestEncodeKeepsHTMLCharacters(t *testing.T) {
	got, err := Encode(map[string]string{"name": fixtures.HTMLSpecial})
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"name":"<>&"}`; string(got) != want {
		t.Errorf("Encode = %s, want %s: <, > and & as they are, and no newline at the end", got,
			want)
	}
}

func TestEncodeGolden(t *testing.T) {
	body := hostileBody{
		Big:       fixtures.TwoPow53Plus1,
		Max:       fixtures.MaxInt64,
		Huge:      fixtures.Huge,
		Zero:      fixtures.NegativeZero(),
		HTML:      "a<b>&c",
		Separator: fixtures.LineSeparator + fixtures.ParagraphSeparator,
		Typed:     fixtures.TypedKey,
		Attrs:     map[string]any{"n": json.Number("9007199254740993"), "s": "<x>"},
	}
	got, err := Encode(body)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"big":9007199254740993,"max":9223372036854775807,"huge":1e+300,"zero":-0,` +
		`"html":"a<b>&c","separator":"\u2028\u2029","typed":"document:2026-annual-report",` +
		`"attrs":{"n":9007199254740993,"s":"<x>"}}`
	if diff := cmp.Diff(want, string(got)); diff != "" {
		t.Errorf("Encode differs (-want +got):\n%s", diff)
	}
}

func TestEncodeErrors(t *testing.T) {
	for name, v := range map[string]any{
		"a channel": make(chan int),
		"NaN":       math.NaN(),
		"infinity":  math.Inf(1),
	} {
		if got, err := Encode(v); err == nil {
			t.Errorf("Encode(%s) = %s, nil; want an error", name, got)
		}
	}
}

func TestDecodeKeepsNumbers(t *testing.T) {
	var got map[string]any
	err := Decode([]byte(`{"big":9007199254740993,"huge":1e300,"zero":-0,`+
		`"pi":3.14159265358979323846,"list":[18446744073709551616]}`), &got)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"big":  json.Number("9007199254740993"),
		"huge": json.Number("1e300"),
		"zero": json.Number("-0"),
		"pi":   json.Number("3.14159265358979323846"),
		"list": []any{json.Number("18446744073709551616")},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Decode differs (-want +got):\n%s", diff)
	}
}

func TestDecodeRoundTrip(t *testing.T) {
	want := hostileBody{
		Big:       fixtures.TwoPow53Plus1,
		Max:       fixtures.MaxInt64,
		Huge:      fixtures.Huge,
		Zero:      fixtures.NegativeZero(),
		HTML:      fixtures.HTMLSpecial,
		Separator: fixtures.LineSeparator + fixtures.ParagraphSeparator,
		Typed:     fixtures.TypedKey,
		Attrs:     map[string]any{"n": json.Number("9007199254740993"), "s": "<x>"},
	}
	data, err := Encode(want)
	if err != nil {
		t.Fatal(err)
	}
	var got hostileBody
	if err := Decode(data, &got); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("the round trip differs (-want +got):\n%s", diff)
	}
	if !math.Signbit(got.Zero) {
		t.Error("the round trip lost the sign of -0")
	}
}

func TestDecodeIgnoresUnknownFields(t *testing.T) {
	var got struct {
		Key string `json:"key"`
	}
	if err := Decode([]byte(`{"key":"k","added_later":{"a":[1]}}`), &got); err != nil {
		t.Fatalf("Decode of an unknown field returned %v", err)
	}
	if got.Key != "k" {
		t.Errorf("Decode read the key %q, want k", got.Key)
	}
}

func TestDecodeAcceptsWhiteSpaceAround(t *testing.T) {
	var got []int
	if err := Decode([]byte(" \t\r\n[1,2] \n"), &got); err != nil {
		t.Fatalf("Decode returned %v", err)
	}
	if diff := cmp.Diff([]int{1, 2}, got); diff != "" {
		t.Errorf("Decode differs (-want +got):\n%s", diff)
	}
}

func TestDecodeErrors(t *testing.T) {
	for _, tc := range []struct {
		body string
		want error
	}{
		{"", errEmptyBody},
		{" \n\t", errEmptyBody},
		{`{"a":1} {"b":2}`, errTrailingData},
		{`{"a":1}x`, errTrailingData},
		{"[1] ]", errTrailingData},
		{`"s" "t"`, errTrailingData},
	} {
		var v any
		if err := Decode([]byte(tc.body), &v); !errors.Is(err, tc.want) {
			t.Errorf("Decode(%q) returned %v, want %v", tc.body, err, tc.want)
		}
	}
	for _, body := range []string{`{"a":`, "[1,", "nul", "<html></html>", `{"a":1,}`} {
		var v any
		err := Decode([]byte(body), &v)
		if err == nil || errors.Is(err, errEmptyBody) || errors.Is(err, errTrailingData) {
			t.Errorf("Decode(%q) returned %v, want the decoder's syntax error", body, err)
		}
	}
	var n struct {
		N int `json:"n"`
	}
	if err := Decode([]byte(`{"n":"x"}`), &n); err == nil {
		t.Error("Decode of a string into an int returned nil, want an error")
	}
}
