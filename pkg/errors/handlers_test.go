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
