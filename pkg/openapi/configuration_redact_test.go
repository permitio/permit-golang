package openapi_test

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/permitio/permit-golang/pkg/openapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var credentialHeaderValues = []string{
	"bearer-value-41d0", "api-key-value-41d0", "cookie-value-41d0",
}

// printVerbs are fmt directives that apply to structs and pointers, and ones that do not: fmt
// prints the fields of a struct with them anyway, and prints in full what an unexported pointer
// field points to.
var printVerbs = []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%X", "%d", "%-12.4v", "%10s",
	"%+q", "%#q", "%#x", "% x", "%08v", "%t", "%e"}

func newConfiguration() *openapi.Configuration {
	cfg := openapi.NewConfiguration()
	cfg.Host = "api.example.test"
	cfg.AddDefaultHeader("Authorization", "Bearer bearer-value-41d0")
	cfg.AddDefaultHeader("X-API-Key", "api-key-value-41d0")
	cfg.AddDefaultHeader("Cookie", "session=cookie-value-41d0")
	cfg.AddDefaultHeader("X-Wait-Timeout", "1.5")
	return cfg
}

// assertNoCredentialHeaderValues fails when output holds a credential header value as text or in
// one of the hexadecimal forms that %x, %X and % x print.
func assertNoCredentialHeaderValues(t *testing.T, output string, name string) {
	t.Helper()
	for _, value := range credentialHeaderValues {
		hexValue := hex.EncodeToString([]byte(value))
		spacedHexValue := strings.TrimSpace(fmt.Sprintf("% x", value))
		for _, form := range []string{value, hexValue, strings.ToUpper(hexValue), spacedHexValue} {
			assert.NotContains(t, output, form, name)
		}
	}
}

func TestPrintingConfigurationHidesCredentialHeaders(t *testing.T) {
	cfg := newConfiguration()
	headers := map[string]string{
		"Authorization":  "Bearer bearer-value-41d0",
		"X-API-Key":      "api-key-value-41d0",
		"Cookie":         "session=cookie-value-41d0",
		"X-Wait-Timeout": "1.5",
	}
	for _, verb := range printVerbs {
		for _, value := range []interface{}{cfg, *cfg} {
			output := fmt.Sprintf(verb, value)

			assertNoCredentialHeaderValues(t, output, verb)
		}
	}
	for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%10s"} {
		for _, value := range []interface{}{cfg, *cfg} {
			assert.Contains(t, fmt.Sprintf(verb, value), "api.example.test", verb)
		}
	}
	assert.Contains(t, fmt.Sprintf("%+v", cfg), "DefaultHeader:map[Authorization:[REDACTED] "+
		"Cookie:[REDACTED] X-API-Key:[REDACTED] X-Wait-Timeout:1.5]")
	assert.True(t, strings.HasPrefix(fmt.Sprintf("%#v", cfg),
		`openapi.Configuration{Host:"api.example.test"`), fmt.Sprintf("%#v", cfg))
	assert.Equal(t, fmt.Sprint(cfg), cfg.String())
	assert.Equal(t, fmt.Sprintf("%#v", cfg), cfg.GoString())
	assert.Equal(t, headers, cfg.DefaultHeader,
		"printing must not change the headers that are sent")

	var nilConfig *openapi.Configuration
	assert.Equal(t, "<nil>", fmt.Sprint(nilConfig))
	assert.Contains(t, fmt.Sprintf("%+v", openapi.Configuration{}), "DefaultHeader:map[]")
}

func TestEncodingConfigurationAsJSONHidesCredentialHeaders(t *testing.T) {
	cfg := newConfiguration()

	for _, value := range []interface{}{cfg, *cfg} {
		encoded, err := json.Marshal(value)

		require.NoError(t, err)
		assertNoCredentialHeaderValues(t, string(encoded), "json.Marshal")
		var decoded map[string]interface{}
		require.NoError(t, json.Unmarshal(encoded, &decoded))
		assert.Equal(t, "api.example.test", decoded["host"])
		assert.Equal(t, map[string]interface{}{"Authorization": "[REDACTED]",
			"X-API-Key": "[REDACTED]", "Cookie": "[REDACTED]", "X-Wait-Timeout": "1.5"},
			decoded["defaultHeader"])
	}

	var logged bytes.Buffer
	encoder := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	logger := zap.New(zapcore.NewCore(encoder, zapcore.AddSync(&logged), zapcore.DebugLevel))
	logger.Info("configuration", zap.Reflect("config", cfg), zap.Any("any", cfg))
	assertNoCredentialHeaderValues(t, logged.String(), "zap")
	assert.Contains(t, logged.String(), `"Authorization":"[REDACTED]"`)

	var nilConfig *openapi.Configuration
	encoded, err := json.Marshal(nilConfig)
	require.NoError(t, err)
	assert.Equal(t, "null", string(encoded))
}

func TestPrintingAPIClientHidesCredentialHeaders(t *testing.T) {
	cfg := newConfiguration()
	client := openapi.NewAPIClient(cfg)

	for _, verb := range printVerbs {
		for _, value := range []interface{}{client, *client, client.APIKeysApi} {
			assertNoCredentialHeaderValues(t, fmt.Sprintf(verb, value), verb)
		}
	}
	assert.Contains(t, fmt.Sprintf("%s", client), "Authorization:[REDACTED]")
	assert.True(t, strings.HasPrefix(fmt.Sprintf("%#v", client), "openapi.APIClient{cfg:"),
		fmt.Sprintf("%#v", client))
	assert.Equal(t, "Bearer bearer-value-41d0", client.GetConfig().DefaultHeader["Authorization"],
		"printing must not change the headers that are sent")

	var nilClient *openapi.APIClient
	assert.Equal(t, "<nil>", fmt.Sprint(nilClient))
	assert.Contains(t, fmt.Sprintf("%+v", openapi.APIClient{}), "cfg:<nil>")
}
