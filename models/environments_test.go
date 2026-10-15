package models_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/permitio/permit-golang/v2/internal/redact"
	"github.com/permitio/permit-golang/v2/internal/testkit/canary"
	"github.com/permitio/permit-golang/v2/internal/wire"
	"github.com/permitio/permit-golang/v2/models"
)

// statsWith returns the statistics of an environment with one PDP whose client secret is
// clientSecret and whose access token is token, as a reply decodes into them.
func statsWith(clientSecret, token string) models.EnvironmentStats {
	return models.EnvironmentStats{
		EnvironmentRead: models.EnvironmentRead{Key: "production", ID: "e", Name: "Production"},
		PDPConfigs: []models.PDPConfigRead{{
			ID: "c", Name: "Production PDP", OrganizationID: "o", ProjectID: "p",
			EnvironmentID: "e", ClientSecret: models.NewSecret(clientSecret),
			OPALServerAccessToken: models.NewSecret(token),
		}},
		Stats: models.EnvironmentStatistics{Roles: 3, Users: 12},
	}
}

// TestStatsPrintNoCredential checks that fmt with every verb, flag and width prints the marker
// and never the credentials of the PDP configurations of an EnvironmentStats: of the statistics
// by value and by pointer, of a PDPConfigRead, of a list of them and of an exported field that
// holds the statistics; and that fmt prints the statistics in an unexported field, whose
// Secrets it cannot call the methods of, without a marker and without the credentials.
func TestStatsPrintNoCredential(t *testing.T) {
	clientSecret, token := canary.Key("statsclient"), canary.Key("statstoken")
	stats := statsWith(clientSecret, token)
	config := stats.PDPConfigs[0]
	type exported struct{ Stats models.EnvironmentStats }
	type unexported struct{ stats models.EnvironmentStats }
	forms := append(credentialTexts(clientSecret), credentialTexts(token)...)
	scanner := canary.NewScanner(forms...)
	for name, tc := range map[string]struct {
		value any
		// markers is how many markers fmt prints: two for each PDP configuration whose Secrets
		// it calls the methods of, and none for one in an unexported field.
		markers int
	}{
		"an EnvironmentStats":              {stats, 2},
		"a *EnvironmentStats":              {&stats, 2},
		"a PDPConfigRead":                  {config, 2},
		"a []PDPConfigRead":                {stats.PDPConfigs, 2},
		"an exported field":                {exported{stats}, 2},
		"a pointer to an exported field":   {&exported{stats}, 2},
		"an unexported field":              {unexported{stats}, 0},
		"a pointer to an unexported field": {&unexported{stats}, 0},
	} {
		for _, format := range []string{
			"%v", "%+v", "%#v", "%s", "%q", "%x", "%X", "%d", "%10.3s", "%-20v",
		} {
			got := fmt.Sprintf(format, tc.value)
			_, _ = scanner.Write([]byte(got + "\n"))
			if n := strings.Count(got, redact.Marker); n != tc.markers {
				t.Errorf("Sprintf(%q, %s) = %q holds %s %d times, want %d", format, name, got,
					redact.Marker, n, tc.markers)
			}
		}
	}
	scanner.Check(t)
}

// TestStatsInLogsAndJSON checks that slog's text and JSON handlers, and encoding/json, write the
// marker in place of the credentials of the PDP configurations of an EnvironmentStats, and that
// a log line still names the other fields of the statistics.
func TestStatsInLogsAndJSON(t *testing.T) {
	clientSecret, token := canary.Key("statslogclient"), canary.Key("statslogtoken")
	stats := statsWith(clientSecret, token)
	forms := append(credentialTexts(clientSecret), credentialTexts(token)...)
	for name, newHandler := range map[string]func(io.Writer) slog.Handler{
		"text": func(w io.Writer) slog.Handler { return slog.NewTextHandler(w, nil) },
		"JSON": func(w io.Writer) slog.Handler { return slog.NewJSONHandler(w, nil) },
	} {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			slog.New(newHandler(&out)).Info("the statistics", "stats", stats, "pointer", &stats)
			scanner := canary.NewScanner(forms...)
			_, _ = scanner.Write(out.Bytes())
			scanner.Check(t)
			if got := strings.Count(out.String(), redact.Marker); got != 4 ||
				!strings.Contains(out.String(), "Production PDP") {
				t.Errorf("the log holds %s %d times, want 4, and the name of the PDP: %s",
					redact.Marker, got, &out)
			}
		})
	}
	encoded, err := json.Marshal(stats)
	if err != nil {
		t.Fatal(err)
	}
	scanner := canary.NewScanner(forms...)
	_, _ = scanner.Write(encoded)
	scanner.Check(t)
	for _, field := range []string{"client_secret", "opal_server_access_token"} {
		if !bytes.Contains(encoded, []byte(`"`+field+`":"`+redact.Marker+`"`)) {
			t.Errorf("json.Marshal of the statistics = %s, want the marker in place of %s",
				encoded, field)
		}
	}
}

// TestPDPCredentialsEncodeAndCompare checks that a PDPConfigRead encodes an empty client secret
// as "" and leaves out an empty access token, and that go-cmp, which uses the Equal of a Secret,
// finds two statistics equal only when both credentials of their PDP configurations are.
func TestPDPCredentialsEncodeAndCompare(t *testing.T) {
	encoded, err := wire.Encode(models.PDPConfigRead{ID: "c"})
	if err != nil || bytes.Contains(encoded, []byte("opal_server_access_token")) ||
		!bytes.Contains(encoded, []byte(`"client_secret":""`)) {
		t.Errorf("Encode of a configuration without credentials = %s, %v; want an empty "+
			"client_secret and no access token", encoded, err)
	}
	stats := statsWith("opaque-client-value", "opaque-token-value")
	for name, tc := range map[string]struct {
		other models.EnvironmentStats
		want  bool
	}{
		"the same credentials":  {statsWith("opaque-client-value", "opaque-token-value"), true},
		"another client secret": {statsWith("opaque-other-value", "opaque-token-value"), false},
		"another access token":  {statsWith("opaque-client-value", "opaque-other-value"), false},
		"no access token":       {statsWith("opaque-client-value", ""), false},
		"no credential":         {statsWith("", ""), false},
	} {
		if got := cmp.Equal(stats, tc.other); got != tc.want {
			t.Errorf("cmp.Equal of statistics with %s = %t, want %t", name, got, tc.want)
		}
	}
}

// TestPDPCredentialsDecode checks that a PDPConfigRead that the SDK's decoder reads holds the
// client secret and the access token of the reply, and that a client secret that is not a JSON
// string or null is an error that does not hold the value.
func TestPDPCredentialsDecode(t *testing.T) {
	for _, data := range []string{
		`5`, `true`, `{"value":"opaque-nested-value"}`, `["opaque-listed-value"]`, `"unclosed`,
	} {
		var config models.PDPConfigRead
		err := wire.Decode([]byte(`{"id":"c","client_secret":`+data+`}`), &config)
		if err == nil || strings.Contains(err.Error(), "opaque") {
			t.Errorf("Decode of a configuration whose client secret is %s returned %v, want an "+
				"error without the value", data, err)
		}
	}
	var config models.PDPConfigRead
	err := wire.Decode([]byte(`{"id":"c","client_secret":"opaque-client-value",`+
		`"opal_server_access_token":"opaque-token-value"}`), &config)
	if err != nil || config.ClientSecret.Value() != "opaque-client-value" ||
		config.OPALServerAccessToken.Value() != "opaque-token-value" {
		t.Errorf("Decode of a configuration returned the credentials %q and %q and %v",
			config.ClientSecret.Value(), config.OPALServerAccessToken.Value(), err)
	}
}
