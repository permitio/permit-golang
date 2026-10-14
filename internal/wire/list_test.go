package wire

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// listItem is an item of the list fixtures in testdata/list.
type listItem struct {
	Key        string         `json:"key"`
	Attributes map[string]any `json:"attributes"`
}

// fixtureItems are the items that both list fixtures hold.
var fixtureItems = []listItem{
	{Key: "a/b", Attributes: map[string]any{
		"count": json.Number("9007199254740993"), "note": "<>&",
	}},
	{Key: "00000000-0000-4000-8000-000000000001", Attributes: map[string]any{}},
	{Key: "document:2026-annual-report", Attributes: map[string]any{"ratio": json.Number("0.1")}},
}

// readFixture returns the content of testdata/list/name.
func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "list", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestDecodeListBothShapes(t *testing.T) {
	for _, tc := range []struct {
		fixture string
		want    List[listItem]
	}{
		{"bare-array.json", List[listItem]{Data: fixtureItems}},
		{"envelope.json", List[listItem]{
			Data: fixtureItems, TotalCount: new(3), PageCount: new(1),
		}},
	} {
		t.Run(tc.fixture, func(t *testing.T) {
			got, err := DecodeList[listItem](readFixture(t, tc.fixture))
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("DecodeList differs (-want +got):\n%s", diff)
			}
		})
	}
}

func TestDecodeListEmptyAndPartial(t *testing.T) {
	for _, tc := range []struct {
		body string
		want List[int]
	}{
		{"[]", List[int]{Data: []int{}}},
		{" \r\n\t[1, 2]", List[int]{Data: []int{1, 2}}},
		{`{"data": [], "total_count": 0}`, List[int]{Data: []int{}, TotalCount: new(0)}},
		{`{"data": [7]}`, List[int]{Data: []int{7}}},
		{
			`{"page_count": 4, "data": [7], "total_count": 91, "extra": true}`,
			List[int]{Data: []int{7}, TotalCount: new(91), PageCount: new(4)},
		},
	} {
		got, err := DecodeList[int]([]byte(tc.body))
		if err != nil {
			t.Errorf("DecodeList(%q) returned %v", tc.body, err)
			continue
		}
		if diff := cmp.Diff(tc.want, got); diff != "" {
			t.Errorf("DecodeList(%q) differs (-want +got):\n%s", tc.body, diff)
		}
		if got.Data == nil {
			t.Errorf("DecodeList(%q) returned nil Data, want an empty list", tc.body)
		}
	}
}

func TestDecodeListRejectsOtherBodies(t *testing.T) {
	for body, want := range map[string]error{
		"":                  errNotAList,
		" \n":               errNotAList,
		"null":              errNotAList,
		`"text"`:            errNotAList,
		"42":                errNotAList,
		"-1":                errNotAList,
		"true":              errNotAList,
		"<html></html>":     errNotAList,
		"{}":                errNoData,
		`{"data":null}`:     errNoData,
		`{"key":"a"}`:       errNoData,
		`{"total_count":1}`: errNoData,
		"[] []":             errTrailingData,
		`{"data":[]} x`:     errTrailingData,
	} {
		got, err := DecodeList[int]([]byte(body))
		if !errors.Is(err, want) {
			t.Errorf("DecodeList(%q) = %+v, %v; want the error %q", body, got, err, want)
		}
	}
	for _, body := range []string{`{"data":{}}`, `[1,`, `{"data":["x"]}`, `["x"]`, `{"data":[1],`} {
		if got, err := DecodeList[int]([]byte(body)); err == nil {
			t.Errorf("DecodeList(%q) = %+v, nil; want the decoder's error", body, got)
		}
	}
}
