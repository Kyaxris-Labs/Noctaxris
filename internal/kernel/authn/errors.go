package authn

import (
	"errors"
	"fmt"
)

// AWS error codes returned by SigV4 verification failures.
const (
	CodeMissingAuthenticationToken = "MissingAuthenticationToken"
	CodeInvalidClientTokenId       = "InvalidClientTokenId"
	CodeSignatureDoesNotMatch      = "SignatureDoesNotMatch"
	CodeRequestTimeTooSkewed       = "RequestTimeTooSkewed"
)

// Error is a distinguishable authn failure with an AWS error code.
type Error struct {
	code    string
	message string
}

func (e *Error) Error() string {
	if e.message == "" {
		return e.code
	}
	return fmt.Sprintf("%s: %s", e.code, e.message)
}

// Code returns the AWS error code for e.
func (e *Error) Code() string {
	return e.code
}

func newError(code, message string) *Error {
	return &Error{code: code, message: message}
}

// Code extracts the AWS error code from err, or "" if unknown.
func Code(err error) string {
	var ae *Error
	if errors.As(err, &ae) {
		return ae.Code()
	}
	return ""
}
