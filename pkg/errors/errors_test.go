package errors_test

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	permitErrors "github.com/permitio/permit-golang/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestNewPermitContextErrorWithCauseKeepsCause(t *testing.T) {
	const message permitErrors.ErrorMessage = "the scope request failed"
	const body = `{"detail":"slow down"}`
	plainCause := errors.New("connection reset")
	statusCause := permitErrors.NewPermitUnexpectedError(errors.New("429 Too Many Requests"),
		&http.Response{
			StatusCode: http.StatusTooManyRequests,
			Body:       io.NopCloser(strings.NewReader(body)),
		})
	contextText := permitErrors.NewPermitContextError(message).Error()
	tests := []struct {
		name       string
		cause      error
		wantText   string
		wantStatus int
		wantBody   string
	}{
		{name: "nil cause", cause: nil, wantText: contextText},
		{name: "plain cause", cause: plainCause, wantText: contextText + ": connection reset"},
		{
			name:       "PermitError cause with a status",
			cause:      statusCause,
			wantText:   contextText + " (status 429): " + statusCause.Error(),
			wantStatus: http.StatusTooManyRequests,
			wantBody:   body,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := permitErrors.NewPermitContextErrorWithCause(message, tt.cause)

			assert.Equal(t, permitErrors.ContextError, err.ErrorCode)
			assert.Equal(t, tt.wantText, err.Error())
			assert.Equal(t, tt.wantStatus, err.StatusCode)
			assert.Equal(t, tt.wantBody, err.ResponseBody)
			assert.Equal(t, tt.cause, errors.Unwrap(err))
		})
	}
}

func TestPermitErrorWithoutCauseUnwrapsToNil(t *testing.T) {
	response := &http.Response{StatusCode: http.StatusBadRequest, Body: http.NoBody}
	tests := []struct {
		name string
		err  permitErrors.PermitError
	}{
		{
			name: "NewPermitError",
			err: permitErrors.NewPermitError(permitErrors.UnexpectedErrorMessage,
				permitErrors.UnexpectedError, permitErrors.GENERAL_ERROR, response),
		},
		{
			name: "NewPermitUnexpectedError",
			err:  permitErrors.NewPermitUnexpectedError(errors.New("bad request"), response),
		},
		{
			name: "NewPermitNotFoundError",
			err:  permitErrors.NewPermitNotFoundError(errors.New("not found"), response),
		},
		{name: "NewPermitContextError", err: permitErrors.NewPermitContextError("no scope")},
		{
			name: "NewPermitConnectionError",
			err:  permitErrors.NewPermitConnectionError(errors.New("connection refused")),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Nil(t, errors.Unwrap(tt.err))
		})
	}
}
