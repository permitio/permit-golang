package fixtures

import (
	"bytes"
	"encoding/json"
	"math"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestTwoPow53Plus1(t *testing.T) {
	if TwoPow53Plus1 != 9007199254740993 {
		t.Fatalf("TwoPow53Plus1 = %d, want 9007199254740993", TwoPow53Plus1)
	}
	if int64(float64(TwoPow53Plus1)) == TwoPow53Plus1 {
		t.Errorf("float64 holds %d exactly, want it rounded", TwoPow53Plus1)
	}
	text := strconv.FormatInt(TwoPow53Plus1, 10)
	var lossy any
	if err := json.Unmarshal([]byte(text), &lossy); err != nil {
		t.Fatal(err)
	}
	if lossy != float64(9007199254740992) {
		t.Errorf("decoding %s into an any = %v, want the rounded 9007199254740992", text, lossy)
	}
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.UseNumber()
	var exact any
	if err := decoder.Decode(&exact); err != nil {
		t.Fatal(err)
	}
	if exact != json.Number(text) {
		t.Errorf("decoding %s with UseNumber = %#v, want json.Number(%q)", text, exact, text)
	}
}

func TestMaxInt64(t *testing.T) {
	if MaxInt64 != 9223372036854775807 {
		t.Fatalf("MaxInt64 = %d, want 9223372036854775807", MaxInt64)
	}
	if float64(MaxInt64) != math.Pow(2, 63) {
		t.Errorf("float64(MaxInt64) = %g, want it rounded to 2^63", float64(MaxInt64))
	}
	encoded, err := json.Marshal(MaxInt64)
	if err != nil || string(encoded) != "9223372036854775807" {
		t.Errorf("json.Marshal(MaxInt64) = %s, %v, want 9223372036854775807", encoded, err)
	}
}

func TestHuge(t *testing.T) {
	encoded, err := json.Marshal(Huge)
	if err != nil || string(encoded) != "1e+300" {
		t.Errorf("json.Marshal(Huge) = %s, %v, want 1e+300", encoded, err)
	}
	if Huge <= math.MaxInt64 {
		t.Errorf("Huge = %g fits in an int64", Huge)
	}
}

func TestNegativeZero(t *testing.T) {
	zero := NegativeZero()
	if zero != 0 || !math.Signbit(zero) {
		t.Errorf("NegativeZero() = %g with sign bit %t, want 0 with the sign bit set", zero,
			math.Signbit(zero))
	}
	encoded, err := json.Marshal(zero)
	if err != nil || string(encoded) != "-0" {
		t.Errorf("json.Marshal(NegativeZero()) = %s, %v, want -0", encoded, err)
	}
}

func TestEscapedStrings(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "LineSeparator", value: LineSeparator, want: `"\u2028"`},
		{name: "ParagraphSeparator", value: ParagraphSeparator, want: `"\u2029"`},
		{name: "HTMLSpecial", value: HTMLSpecial, want: `"\u003c\u003e\u0026"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := json.Marshal(tt.value)
			if err != nil || string(encoded) != tt.want {
				t.Errorf("json.Marshal(%q) = %s, %v, want %s", tt.value, encoded, err, tt.want)
			}
			var decoded string
			if err := json.Unmarshal(encoded, &decoded); err != nil || decoded != tt.value {
				t.Errorf("decoding %s = %q, %v, want %q", encoded, decoded, err, tt.value)
			}
		})
	}
	if LineSeparator != "\u2028" || ParagraphSeparator != "\u2029" {
		t.Errorf("LineSeparator, ParagraphSeparator = %q, %q, want U+2028, U+2029",
			LineSeparator, ParagraphSeparator)
	}
}

func TestHTMLSpecialWithoutHTMLEscaping(t *testing.T) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(HTMLSpecial); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != "\"<>&\"\n" {
		t.Errorf("encoding HTMLSpecial without HTML escaping = %q, want %q", got, "\"<>&\"\n")
	}
}

func TestLoneSurrogate(t *testing.T) {
	if !json.Valid([]byte(LoneSurrogateJSON)) {
		t.Fatalf("LoneSurrogateJSON %s is not valid JSON", LoneSurrogateJSON)
	}
	var decoded string
	if err := json.Unmarshal([]byte(LoneSurrogateJSON), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded != LoneSurrogateDecoded || LoneSurrogateDecoded != "\ufffd" {
		t.Errorf("decoding %s = %q, LoneSurrogateDecoded = %q, want U+FFFD for both",
			LoneSurrogateJSON, decoded, LoneSurrogateDecoded)
	}
}

func TestKeys(t *testing.T) {
	uuid := regexp.MustCompile(
		`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if !uuid.MatchString(UUIDKey) {
		t.Errorf("UUIDKey = %q, want the shape of a version 4 UUID", UUIDKey)
	}
	kind, key, ok := strings.Cut(TypedKey, ":")
	if !ok || kind == "" || key == "" || strings.Contains(key, ":") {
		t.Errorf(`TypedKey = %q, want "type:key"`, TypedKey)
	}
}
