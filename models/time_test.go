package models_test

import (
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/permitio/permit-golang/v2/internal/wire"
	"github.com/permitio/permit-golang/v2/models"
)

// decodeTime decodes the JSON string text into a models.Time.
func decodeTime(t *testing.T, text string) models.Time {
	t.Helper()
	var got models.Time
	if err := json.Unmarshal([]byte(strconv.Quote(text)), &got); err != nil {
		t.Fatalf("%q does not decode: %v", text, err)
	}
	return got
}

func TestTimeDecodes(t *testing.T) {
	for _, tc := range []struct {
		text   string
		want   time.Time
		offset int
	}{
		{"2026-10-14T09:30:00Z", time.Date(2026, 10, 14, 9, 30, 0, 0, time.UTC), 0},
		{"2026-10-14T12:30:00+03:00", time.Date(2026, 10, 14, 9, 30, 0, 0, time.UTC), 3 * 3600},
		{"2026-10-14T05:00:00-04:30", time.Date(2026, 10, 14, 9, 30, 0, 0, time.UTC), -16200},
		{
			"2026-10-14T09:30:00.123456789Z",
			time.Date(2026, 10, 14, 9, 30, 0, 123456789, time.UTC), 0,
		},
		{"2026-10-14T09:30:00", time.Date(2026, 10, 14, 9, 30, 0, 0, time.UTC), 0},
		{
			"2019-08-24T14:15:22.123456",
			time.Date(2019, 8, 24, 14, 15, 22, 123456000, time.UTC), 0,
		},
	} {
		got := decodeTime(t, tc.text)
		if !got.Equal(tc.want) {
			t.Errorf("%q decodes to %v, want %v", tc.text, got, tc.want)
		}
		if _, offset := got.Zone(); offset != tc.offset {
			t.Errorf("%q decodes with the offset %ds, want %ds", tc.text, offset, tc.offset)
		}
	}
}

func TestTimeWithoutAZoneIsUTC(t *testing.T) {
	got := decodeTime(t, "2026-10-14T09:30:00.5")
	if got.Location() != time.UTC {
		t.Errorf("a timestamp without a zone decodes in %v, want UTC", got.Location())
	}
}

func TestTimeKeepsTheMethodsOfTimeTime(t *testing.T) {
	zoneless := decodeTime(t, "2026-10-14T09:30:00")
	if !zoneless.Before(decodeTime(t, "2026-10-14T12:30:01+03:00").Time) {
		t.Error("09:30:00 without a zone is not before 12:30:01+03:00, which is 09:30:01 UTC")
	}
	if zoneless.Before(decodeTime(t, "2026-10-14T12:29:59+03:00").Time) {
		t.Error("09:30:00 without a zone is before 12:29:59+03:00, which is 09:29:59 UTC")
	}
	if got, want := zoneless.Format(time.DateOnly), "2026-10-14"; got != want {
		t.Errorf("Format(time.DateOnly) = %q, want %q", got, want)
	}
}

func TestTimeEncodesWithItsZone(t *testing.T) {
	got, err := wire.Encode(decodeTime(t, "2026-10-14T09:30:00.5"))
	if err != nil {
		t.Fatal(err)
	}
	if want := `"2026-10-14T09:30:00.5Z"`; string(got) != want {
		t.Errorf("Encode = %s, want %s", got, want)
	}

	type body struct {
		CreatedAt models.Time `json:"created_at,omitzero"`
	}
	empty, err := wire.Encode(body{})
	if err != nil {
		t.Fatal(err)
	}
	if string(empty) != "{}" {
		t.Errorf("Encode of the zero Time tagged omitzero = %s, want {}", empty)
	}
}

func TestTimeNullLeavesItAsItIs(t *testing.T) {
	got := decodeTime(t, "2026-10-14T09:30:00Z")
	want := got
	if err := json.Unmarshal([]byte("null"), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Equal(want.Time) {
		t.Errorf("null changed the Time to %v, want %v", got, want)
	}
}

func TestTimeDecodeErrors(t *testing.T) {
	for _, body := range []string{
		`"yesterday"`,
		`"2026-10-14"`,
		`"2026-10-14 09:30:00Z"`,
		`"2026-10-14T25:00:00Z"`,
		`"2026-10-14T09:30:00+0300"`,
		`""`,
		`1760434200`,
		`true`,
	} {
		got := decodeTime(t, "2026-10-14T09:30:00Z")
		want := got
		if err := json.Unmarshal([]byte(body), &got); err == nil {
			t.Errorf("%s decodes into a Time without an error", body)
		}
		if !got.Equal(want.Time) {
			t.Errorf("a failed decode of %s changed the Time to %v", body, got)
		}
	}
}

// TestTimeDecodesAsText checks that a decoder that reads text, such as a map key of encoding/json,
// reads a timestamp without a zone as UTC, as UnmarshalJSON does.
func TestTimeDecodesAsText(t *testing.T) {
	var got models.Time
	if err := got.UnmarshalText([]byte("2026-10-14T09:30:00")); err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 10, 14, 9, 30, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("UnmarshalText = %v, want %v", got.Time, want)
	}
	var keyed map[models.Time]int
	if err := json.Unmarshal([]byte(`{"2026-10-14T09:30:00":1}`), &keyed); err != nil {
		t.Fatal(err)
	}
	if len(keyed) != 1 {
		t.Errorf("the map has %d keys, want 1", len(keyed))
	}
	for key := range keyed {
		if key.Location() != time.UTC || key.Hour() != 9 {
			t.Errorf("the map key is %v, want 09:30 UTC", key.Time)
		}
	}
}
