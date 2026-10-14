package wire

import (
	"encoding/json"
	"math"
	"net/url"
	"slices"
	"strconv"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/permitio/permit-golang/v2/internal/testkit/fixtures"
)

func TestQueryGolden(t *testing.T) {
	for _, tc := range []struct {
		name  string
		build func(q *Query)
		want  string
	}{
		{
			name: "bools",
			build: func(q *Query) {
				q.AddBool("include_resource_instance_roles", new(true))
				q.AddBool("success", new(false))
			},
			want: "include_resource_instance_roles=true&success=false",
		},
		{
			name: "ints",
			build: func(q *Query) {
				q.AddInt("page", new(int64(0)))
				q.AddInt("per_page", new(int64(-1)))
				q.AddInt("timestamp_from", new(fixtures.TwoPow53Plus1))
				q.AddInt("timestamp_until", new(fixtures.MaxInt64))
			},
			want: "page=0&per_page=-1&timestamp_from=9007199254740993&" +
				"timestamp_until=9223372036854775807",
		},
		{
			name: "floats",
			build: func(q *Query) {
				q.AddFloat("a", new(0.1))
				q.AddFloat("b", new(pointThree))
				q.AddFloat("c", new(float64(fixtures.TwoPow53Plus1)))
				q.AddFloat("d", new(123456789.125))
				q.AddFloat("e", new(fixtures.NegativeZero()))
				q.AddFloat("f", new(fixtures.Huge))
				q.AddFloat("g", new(1e-7))
				q.AddFloat("h", new(1e21))
				q.AddFloat("i", new(600.0))
			},
			want: "a=0.1&b=0.30000000000000004&c=9007199254740992&d=123456789.125&e=-0&" +
				"f=1e%2B300&g=1e-7&h=1e%2B21&i=600",
		},
		{
			name: "strings",
			build: func(q *Query) {
				q.AddString("search", new("a b&c=d/é+"+fixtures.HTMLSpecial))
				q.AddString("resource_instance", new(fixtures.TypedKey))
				q.AddString("empty", new(""))
			},
			want: "empty=&resource_instance=document%3A2026-annual-report&" +
				"search=a+b%26c%3Dd%2F%C3%A9%2B%3C%3E%26",
		},
		{
			name: "a repeated list key",
			build: func(q *Query) {
				q.AddStrings("user", []string{"b", "a", "b"})
				q.AddInt("page", new(int64(2)))
				q.AddStrings("role", []string{"viewer"})
			},
			want: "page=2&role=viewer&user=b&user=a&user=b",
		},
		{
			name: "unset filters omitted",
			build: func(q *Query) {
				q.AddBool("detailed_view", nil)
				q.AddInt("page", nil)
				q.AddFloat("wait", nil)
				q.AddString("search", nil)
				q.AddStrings("user", nil)
				q.AddStrings("role", []string{})
			},
			want: "",
		},
		{
			name: "deprecated parameters never sent",
			build: func(q *Query) {
				q.AddBool("include_total_count", new(true))
				q.AddBool("detailed", new(true))
				q.AddBool("return_deleted", new(true))
				q.AddString("detailed", new("true"))
				q.AddStrings("include_total_count", []string{"true"})
				q.AddInt("return_deleted", new(int64(1)))
				q.AddFloat("detailed", new(1.0))
				q.AddInt("per_page", new(int64(100)))
			},
			want: "per_page=100",
		},
		{
			name:  "nothing added",
			build: func(*Query) {},
			want:  "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var q Query
			tc.build(&q)
			if got := q.Encode(); got != tc.want {
				t.Errorf("Encode() = %q, want %q", got, tc.want)
			}
		})
	}
}

// pointThree is 0.1 + 0.2 added at run time, 0.30000000000000004, which a float32 rounds to 0.3.
// A constant expression would be exactly 0.3.
var pointThree = pointOne + 0.2

// pointOne is 0.1 in a variable, so that pointThree is not a constant expression.
var pointOne = 0.1

// floatValues are float64 values whose text a careless formatter changes.
var floatValues = []float64{
	0, 0.1, pointThree, 1.0 / 3, 600, 123456789.125, float64(fixtures.TwoPow53Plus1),
	float64(fixtures.MaxInt64), fixtures.Huge, fixtures.NegativeZero(), -2.5, 1e-6, 9.99e-7,
	1e-7, 1.5e-8, 1e-9, 1e-10, 5e-324, 1e20, 1e21, 1.7976931348623157e308, -1e-7, -1e21,
	3.4028234663852886e38, 16777217,
}

func TestFormatFloatIsJSON(t *testing.T) {
	for _, f := range floatValues {
		want, err := json.Marshal(f)
		if err != nil {
			t.Fatal(err)
		}
		got := FormatFloat(f)
		if got != string(want) {
			t.Errorf("FormatFloat(%v) = %s, want %s, as encoding/json writes it", f, got, want)
		}
		back, err := strconv.ParseFloat(got, 64)
		if err != nil || math.Float64bits(back) != math.Float64bits(f) {
			t.Errorf("FormatFloat(%v) = %s, which reads back as %v, %v", f, got, back, err)
		}
	}
}

func TestFormatFloatNotFinite(t *testing.T) {
	for want, f := range map[string]float64{
		"NaN": math.NaN(), "+Inf": math.Inf(1), "-Inf": math.Inf(-1),
	} {
		if got := FormatFloat(f); got != want {
			t.Errorf("FormatFloat(%v) = %q, want %q", f, got, want)
		}
	}
}

func TestDeprecatedParams(t *testing.T) {
	got := DeprecatedParams()
	want := []string{"detailed", "include_total_count", "return_deleted"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("DeprecatedParams differs (-want +got):\n%s", diff)
	}
	if !slices.IsSorted(got) {
		t.Errorf("DeprecatedParams() = %q, want it sorted", got)
	}
	got[0] = "changed"
	if DeprecatedParams()[0] != "detailed" {
		t.Error("changing what DeprecatedParams returned changed the list")
	}
}

func FuzzQuery(f *testing.F) {
	f.Add("search", "a b&c=d", fixtures.TwoPow53Plus1, 0.1, true)
	f.Add("", "", int64(0), fixtures.NegativeZero(), false)
	f.Add("é;x", fixtures.LineSeparator+fixtures.HTMLSpecial, int64(-1), 1e-7, true)
	f.Fuzz(func(t *testing.T, name, value string, i int64, fl float64, b bool) {
		var q Query
		filter := "x_" + name
		q.AddString(filter, &value)
		q.AddInt("i", &i)
		q.AddFloat("f", &fl)
		q.AddBool("b", &b)
		q.AddStrings("l", []string{value, name})
		q.AddString("unset", nil)
		q.AddBool("include_total_count", &b)

		encoded := q.Encode()
		got, err := url.ParseQuery(encoded)
		if err != nil {
			t.Fatalf("Encode() = %q, which does not parse: %v", encoded, err)
		}
		want := url.Values{
			filter: {value},
			"i":    {strconv.FormatInt(i, 10)},
			"f":    {FormatFloat(fl)},
			"b":    {strconv.FormatBool(b)},
			"l":    {value, name},
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Fatalf("Encode() = %q, which parses to other values (-want +got):\n%s", encoded,
				diff)
		}
		back, err := strconv.ParseFloat(got.Get("f"), 64)
		if err != nil && !math.IsInf(fl, 0) {
			t.Fatalf("the float %v went out as %q, which does not parse: %v", fl, got.Get("f"),
				err)
		}
		if math.Float64bits(back) != math.Float64bits(fl) && !math.IsNaN(fl) {
			t.Fatalf("the float %v went out as %q, which reads back as %v", fl, got.Get("f"), back)
		}
	})
}
