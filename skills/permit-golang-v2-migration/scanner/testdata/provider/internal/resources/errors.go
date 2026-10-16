package resources

import (
	"errors"

	permitErrors "github.com/permitio/permit-golang/pkg/errors" // v1: github.com/permitio/permit-golang/pkg/errors
)

// isNotFound reports whether err is v1's error for an object that does not exist.
func isNotFound(err error) bool {
	var permitError permitErrors.PermitError                                              // v1: errors.PermitError
	return errors.As(err, &permitError) && permitError.ErrorCode == permitErrors.NotFound // v1: errors.NotFound
}
