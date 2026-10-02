package federation

import (
	"testing"
)

func TestVerifySAMLAssertionWrapperAndLabCognitoIssuer(t *testing.T) {
	t.Parallel()
	err := VerifySAMLAssertion("", "")
	if err == nil {
		t.Fatal("empty metadata should fail")
	}
	if fe, ok := err.(*Error); !ok || fe.Code() != CodeIdPNotConfigured {
		t.Fatalf("err=%v", err)
	}
	if fe := err.(*Error); fe.Error() == "" {
		t.Fatal("Error() empty")
	}
	if fe := (&Error{code: CodeAccessDenied}); fe.Error() != CodeAccessDenied {
		t.Fatalf("code-only Error()=%q", fe.Error())
	}

	_, err = VerifyWebIdentityJWTClients(
		"header.payload.sig",
		"https://cognito-idp.us-east-1.amazonaws.com/us-east-1_LabPool",
		[]string{"client"},
		nil,
	)
	if err == nil {
		t.Fatal("lab Cognito issuer should fail closed for remote JWKS path")
	}
	if fe, ok := err.(*Error); !ok || fe.Code() != CodeInvalidIdentityToken {
		t.Fatalf("err=%v", err)
	}
}
