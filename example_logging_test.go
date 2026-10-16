package permit_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"slices"

	"github.com/permitio/permit-golang/v2"
)

// Give the client a *slog.Logger, here with an attribute that tells the client's lines apart.
// New warns once about an API URL that uses http:// to a host that is not loopback. The line has
// a marker in place of the API key, which this URL holds by mistake. This Example prints the
// fields of the JSON line one per line.
func ExampleWithLogger() {
	var out bytes.Buffer
	handler := slog.NewJSONHandler(&out, &slog.HandlerOptions{
		// Leave the time out, so that the line is the same on every run.
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey && len(groups) == 0 {
				return slog.Attr{}
			}
			return a
		},
	})
	logger := slog.New(handler).With("component", "permit")

	apiKey := "permit_key_FAKEexample"
	client, err := permit.New(
		permit.WithAPIKey(apiKey),
		permit.WithAPIURL("http://api.example.com/"+apiKey),
		permit.WithLogger(logger),
	)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer client.Close()

	var line map[string]any
	if err := json.Unmarshal(out.Bytes(), &line); err != nil {
		fmt.Println(err)
		return
	}
	for _, key := range slices.Sorted(maps.Keys(line)) {
		fmt.Printf("%s: %v\n", key, line[key])
	}
	// Output:
	// component: permit
	// level: WARN
	// msg: permit: the API key is sent over http:// to a host that is not loopback; use https://
	// url: http://api.example.com/[REDACTED]
}
