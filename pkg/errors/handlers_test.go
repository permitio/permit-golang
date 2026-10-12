package errors_test

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	permitErrors "github.com/permitio/permit-golang/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHttpErrorHandleMapsNon2xxStatusWithNilError(t *testing.T) {
	const body = `{"detail":"rejected"}`
	tests := []struct {
		status   int
		wantCode permitErrors.ErrorCode
	}{
		{status: 0, wantCode: permitErrors.UnexpectedError},
		{status: http.StatusSwitchingProtocols, wantCode: permitErrors.UnexpectedError},
		{status: http.StatusOK},
		{status: http.StatusCreated},
		{status: http.StatusNoContent},
		{status: http.StatusFound, wantCode: permitErrors.UnexpectedError},
		{status: http.StatusBadRequest, wantCode: permitErrors.UnexpectedError},
		{status: http.StatusUnauthorized, wantCode: permitErrors.Unauthorized},
		{status: http.StatusMethodNotAllowed, wantCode: permitErrors.UnexpectedError},
		{status: http.StatusMisdirectedRequest, wantCode: permitErrors.UnexpectedError},
		{status: http.StatusTooManyRequests, wantCode: permitErrors.UnexpectedError},
		{status: http.StatusBadGateway, wantCode: permitErrors.UnexpectedError},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.status), func(t *testing.T) {
			response := &http.Response{
				StatusCode: tt.status,
				Status:     fmt.Sprintf("%d %s", tt.status, http.StatusText(tt.status)),
				Body:       io.NopCloser(strings.NewReader(body)),
			}

			err := permitErrors.HttpErrorHandle(nil, response)

			if tt.wantCode == "" {
				assert.NoError(t, err)
				return
			}
			var permitErr permitErrors.PermitError
			require.True(t, errors.As(err, &permitErr), "want a PermitError, got %T: %v", err, err)
			assert.Equal(t, tt.wantCode, permitErr.ErrorCode)
			assert.Equal(t, tt.status, permitErr.StatusCode)
			assert.Equal(t, body, permitErr.ResponseBody)
		})
	}
}

func TestHttpErrorHandleNamesStatusCodeWhenStatusIsEmpty(t *testing.T) {
	for _, status := range []int{0, http.StatusSwitchingProtocols, http.StatusTeapot} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			response := &http.Response{StatusCode: status, Body: http.NoBody}

			err := permitErrors.HttpErrorHandle(nil, response)

			require.Error(t, err)
			want := fmt.Sprintf("Message: unexpected HTTP status %d", status)
			assert.True(t, strings.HasSuffix(err.Error(), want), err.Error())
		})
	}
}

// jsonEscape starts a JSON unicode escape: followed by "u" and four hexadecimal digits, a JSON
// decoder reads it as one character.
const jsonEscape = `\`

func TestPermitErrorRemovesCredentials(t *testing.T) {
	const apiKey = "configured-key-9c41e7"
	secrets := []string{
		apiKey,
		"permit_key_FAKEechoed12345",
		"proxy-secret-value",
		"bearer-token-value",
		"nested-password-value",
		"header-secret-value",
		"8675309",
	}
	tests := []struct {
		name         string
		status       int
		err          error
		body         string
		wantBody     string
		wantContains []string
	}{
		{
			name:   "422 JSON body",
			status: http.StatusUnprocessableEntity,
			err: errors.New("422 Unprocessable Entity (invalid key " + apiKey +
				" or permit_key_FAKEechoed12345)"),
			body: `{"detail":"invalid key ` + apiKey + ` or permit_key_FAKEechoed12345",` +
				`"secret":"proxy-secret-value","Token":"bearer-token-value",` +
				`"auth":{"password":"nested-password-value","pin":{"api_key":8675309}},` +
				`"items":[{"access_token":"bearer-token-value"}],` +
				`"empty_token":"","flag_secret":true}`,
			wantBody: `{"Token":"[REDACTED]",` +
				`"auth":{"password":"[REDACTED]","pin":{"api_key":"[REDACTED]"}},` +
				`"detail":"invalid key [REDACTED] or [REDACTED]",` +
				`"empty_token":"","flag_secret":true,` +
				`"items":[{"access_token":"[REDACTED]"}],"secret":"[REDACTED]"}`,
			wantContains: []string{
				"422 Unprocessable Entity (invalid key [REDACTED] or [REDACTED])",
			},
		},
		{
			name:     "500 JSON body without credential keys keeps its bytes",
			status:   http.StatusInternalServerError,
			body:     "{\n  \"detail\": \"failed for " + apiKey + " <b>\"\n}",
			wantBody: "{\n  \"detail\": \"failed for [REDACTED] <b>\"\n}",
		},
		{
			name:   "keys written with JSON escapes",
			status: http.StatusBadRequest,
			body: `{"detail":"bad ` + strings.ReplaceAll(apiKey, "-", jsonEscape+"u002d") +
				` and permit` + jsonEscape + `u005fkey_FAKEechoed12345"}`,
			wantBody: `{"detail":"bad [REDACTED] and [REDACTED]"}`,
		},
		{
			name:   "200 body that did not decode",
			status: http.StatusOK,
			err: errors.New("json: cannot unmarshal object into Go struct field " +
				"ProxyConfigRead.secret of type string"),
			body:     `{"key":"pc","secret":{"X-Api":"header-secret-value"},"name":"n"}`,
			wantBody: `{"key":"pc","name":"n","secret":{"X-Api":"[REDACTED]"}}`,
		},
		{
			name:     "text body",
			status:   http.StatusBadGateway,
			body:     "upstream rejected Bearer " + apiKey + " and permit_key_FAKEechoed12345",
			wantBody: "upstream rejected Bearer [REDACTED] and [REDACTED]",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request, err := http.NewRequest(http.MethodGet, "https://api.example.test/v2/x", nil)
			require.NoError(t, err)
			request.Header.Set("Authorization", "Bearer "+apiKey)
			response := &http.Response{
				StatusCode: tt.status,
				Status:     fmt.Sprintf("%d %s", tt.status, http.StatusText(tt.status)),
				Body:       io.NopCloser(strings.NewReader(tt.body)),
				Request:    request,
			}

			err = permitErrors.HttpErrorHandle(tt.err, response)

			var permitErr permitErrors.PermitError
			require.True(t, errors.As(err, &permitErr), "want a PermitError, got %T: %v", err, err)
			assert.Equal(t, tt.status, permitErr.StatusCode)
			assert.Equal(t, tt.wantBody, permitErr.ResponseBody)
			for _, secret := range secrets {
				assert.NotContains(t, permitErr.ResponseBody, secret)
				assert.NotContains(t, permitErr.Error(), secret)
			}
			for _, want := range tt.wantContains {
				assert.Contains(t, permitErr.Error(), want)
			}
		})
	}
}

func TestPermitErrorWithoutRequestRemovesPermitKeys(t *testing.T) {
	body := `{"detail":"bad permit_key_FAKEechoed12345","token":"t-1"}`
	response := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Status:     "500 Internal Server Error",
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	cause := errors.New("failed with permit_key_FAKEechoed12345")
	err := permitErrors.NewPermitUnexpectedError(cause, response)

	assert.Equal(t, `{"detail":"bad [REDACTED]","token":"[REDACTED]"}`, err.ResponseBody)
	assert.Equal(t,
		"ErrorCode: UnexpectedError, ErrorType: general_error, Message: failed with [REDACTED]",
		err.Error())
}
