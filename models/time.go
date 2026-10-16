package models

import (
	"encoding/json"
	"fmt"
	"time"
)

// zonelessLayout is the layout of an RFC 3339 timestamp without its zone. time.Parse also reads
// a fraction of a second after the seconds, as for every layout.
const zonelessLayout = "2006-01-02T15:04:05"

// Time is a timestamp that the API returns, such as the time an object was created. It embeds
// [time.Time], so a Time has its methods, such as Before, Format and Unix; pass t.Time where a
// time.Time is needed.
//
// It decodes an RFC 3339 timestamp with a zone, such as 2026-10-14T09:30:00Z or
// 2026-10-14T12:30:00+03:00, and one without a zone, such as 2026-10-14T09:30:00.123456, which
// it reads as UTC. It encodes as time.Time does: in RFC 3339, with the zone.
type Time struct {
	// Time is the timestamp.
	time.Time
}

// UnmarshalJSON sets t to the timestamp in the JSON string data, as [Time.UnmarshalText] reads
// it. null leaves t as it is, as for a time.Time.
func (t *Time) UnmarshalJSON(data []byte) error {
	if string(data) == jsonNull {
		return nil
	}
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return fmt.Errorf("models: a Time is a JSON string that holds an RFC 3339 timestamp: %w",
			err)
	}
	return t.UnmarshalText([]byte(text))
}

// UnmarshalText sets t to the timestamp in text, an RFC 3339 timestamp with or without a zone:
// one without a zone is in UTC. A decoder that reads text, such as a map key of encoding/json,
// reads a Time this way.
func (t *Time) UnmarshalText(text []byte) error {
	parsed, err := time.Parse(time.RFC3339, string(text))
	if err != nil {
		parsed, err = time.Parse(zonelessLayout, string(text))
	}
	if err != nil {
		return fmt.Errorf("models: %q is not an RFC 3339 timestamp with or without a zone, such "+
			"as 2006-01-02T15:04:05Z or 2006-01-02T15:04:05", text)
	}
	t.Time = parsed
	return nil
}
