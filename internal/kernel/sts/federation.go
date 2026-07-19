package sts

import (
	"errors"
	"fmt"
)

// AWS STS error codes for federation / web-identity fail-closed paths.
const (
	CodeInvalidIdentityToken                 = "InvalidIdentityToken"
	CodeNotSupported                         = "NotSupported"
	CodeAccessDeniedException                = "AccessDeniedException"
	CodeInvalidAuthorizationMessageException = "InvalidAuthorizationMessageException"
)

// Error is a distinguishable STS failure with an AWS error code.
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

// CodeOf extracts the AWS error code from err, or "" if unknown.
func CodeOf(err error) string {
	var ae *Error
	if errors.As(err, &ae) {
		return ae.Code()
	}
	return ""
}

// GetWebIdentityToken fails closed until an OIDC issuer is configured for the account.
func GetWebIdentityToken(oidcConfigured bool, audience string) error {
	_ = audience
	if !oidcConfigured {
		return newError(CodeNotSupported, "web identity token issuance is not supported; OIDC issuer is not configured")
	}
	return newError(CodeAccessDeniedException, "GetWebIdentityToken is not supported in this lab configuration")
}

// GetDelegatedAccessToken always fails closed; delegated access is not configured.
func GetDelegatedAccessToken() error {
	return newError(CodeAccessDeniedException, "delegated access is not configured")
}
