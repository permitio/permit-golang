package models_test

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/permitio/permit-golang/v2/models"
)

// The three states of an Optional field in the body of an update: unset leaves the field out, so
// the API keeps what it stores, Null sends null, and Some sends a value.
func ExampleOptional() {
	type update struct {
		Description models.Optional[string] `json:"description,omitzero"`
	}
	for _, body := range []update{
		{},
		{Description: models.Null[string]()},
		{Description: models.Some("v")},
	} {
		data, err := json.Marshal(body)
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Println(string(data))
	}
	// Output:
	// {}
	// {"description":null}
	// {"description":"v"}
}

// Get, IsNull and IsZero tell the three states of a decoded Optional apart.
func ExampleOptional_Get() {
	type update struct {
		Description models.Optional[string] `json:"description,omitzero"`
	}
	for _, body := range []string{`{}`, `{"description":null}`, `{"description":"v"}`} {
		var u update
		if err := json.Unmarshal([]byte(body), &u); err != nil {
			fmt.Println(err)
			return
		}
		value, ok := u.Description.Get()
		fmt.Printf("%s: unset %t, null %t, value %q %t\n", body, u.Description.IsZero(),
			u.Description.IsNull(), value, ok)
	}
	// Output:
	// {}: unset true, null false, value "" false
	// {"description":null}: unset false, null true, value "" false
	// {"description":"v"}: unset false, null false, value "v" true
}

// A number in Attributes is a json.Number with every digit, which a float64 would round.
func ExampleAttributes() {
	reply := []byte(`{"id":9007199254740993,"score":0.5}`)
	var attributes models.Attributes
	if err := json.Unmarshal(reply, &attributes); err != nil {
		fmt.Println(err)
		return
	}
	id, ok := attributes["id"].(json.Number)
	if !ok {
		fmt.Printf("id is a %T\n", attributes["id"])
		return
	}
	n, err := id.Int64()
	fmt.Println(n, err)
	score, ok := attributes["score"].(json.Number)
	if !ok {
		fmt.Printf("score is a %T\n", attributes["score"])
		return
	}
	f, err := score.Float64()
	fmt.Println(f, err)
	// Output:
	// 9007199254740993 <nil>
	// 0.5 <nil>
}

// A value that this version of the SDK does not know decodes as it is, and is not Known.
func ExampleAttributeType_Known() {
	for _, body := range []string{`"string"`, `"geo_point"`} {
		var t models.AttributeType
		if err := json.Unmarshal([]byte(body), &t); err != nil {
			fmt.Println(err)
			return
		}
		fmt.Printf("%s: known %t\n", t, t.Known())
	}
	// Output:
	// string: known true
	// geo_point: known false
}

// A timestamp without a zone decodes as UTC, and a Time has the methods of time.Time.
func ExampleTime() {
	var created, updated models.Time
	if err := json.Unmarshal([]byte(`"2026-10-14T09:30:00.5"`), &created); err != nil {
		fmt.Println(err)
		return
	}
	if err := json.Unmarshal([]byte(`"2026-10-14T12:31:00+03:00"`), &updated); err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(created.Format(time.RFC3339Nano), created.Location())
	fmt.Println(created.Before(updated.Time))
	// Output:
	// 2026-10-14T09:30:00.5Z UTC
	// true
}
