package store_test

import (
	"errors"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestForgotPasswordSecureCodeRejectsStubAndIsSingleUse(t *testing.T) {
	st := openCognitoTriggerStore(t)
	account := "000000000001"
	pool, err := st.CreateCognitoUserPool(account, "us-east-1", "secure-cfp")
	if err != nil {
		t.Fatal(err)
	}
	client, err := st.CreateCognitoUserPoolClient(account, pool.PoolID, "app")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AdminCreateCognitoUser(account, pool.PoolID, "secure-cfp-user", "OldPass1!"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ForgotPasswordCognitoUser(client.ClientID, "secure-cfp-user"); err != nil {
		t.Fatal(err)
	}
	code, err := st.PeekCognitoConfirmationCode(account, pool.PoolID, "secure-cfp-user", store.CognitoConfirmPurposeForgotPassword)
	if err != nil {
		t.Fatal(err)
	}
	if code == "" || code == store.CognitoLabConfirmationCode {
		t.Fatalf("stored code=%q want non-empty non-stub", code)
	}
	if len(code) < 8 {
		t.Fatalf("stored code=%q want length >= 8", code)
	}
	err = st.ConfirmForgotPasswordCognitoUser(client.ClientID, "secure-cfp-user", store.CognitoLabConfirmationCode, "NewPass9!")
	if !errors.Is(err, store.ErrCognitoCodeMismatch) {
		t.Fatalf("stub code err=%v want CodeMismatch", err)
	}
	if err := st.ConfirmForgotPasswordCognitoUser(client.ClientID, "secure-cfp-user", code, "NewPass9!"); err != nil {
		t.Fatal(err)
	}
	err = st.ConfirmForgotPasswordCognitoUser(client.ClientID, "secure-cfp-user", code, "OtherPass9!")
	if !errors.Is(err, store.ErrCognitoCodeMismatch) {
		t.Fatalf("reuse err=%v want CodeMismatch", err)
	}
}

func TestConfirmSignUpRequiresStoredCode(t *testing.T) {
	st := openCognitoTriggerStore(t)
	account := "000000000001"
	pool, err := st.CreateCognitoUserPool(account, "us-east-1", "secure-signup")
	if err != nil {
		t.Fatal(err)
	}
	client, err := st.CreateCognitoUserPoolClient(account, pool.PoolID, "app")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.SignUpCognitoUser(account, client.ClientID, "secure-bob", "Secret2!"); err != nil {
		t.Fatal(err)
	}
	code, err := st.PeekCognitoConfirmationCode(account, pool.PoolID, "secure-bob", store.CognitoConfirmPurposeSignUp)
	if err != nil {
		t.Fatal(err)
	}
	if code == "" || code == store.CognitoLabConfirmationCode {
		t.Fatalf("signup code=%q want non-empty non-stub", code)
	}
	err = st.ConfirmSignUpCognitoUser(client.ClientID, "secure-bob", store.CognitoLabConfirmationCode)
	if !errors.Is(err, store.ErrCognitoCodeMismatch) {
		t.Fatalf("stub code err=%v want CodeMismatch", err)
	}
	err = st.ConfirmSignUpCognitoUser(client.ClientID, "secure-bob", "wrong-code")
	if !errors.Is(err, store.ErrCognitoCodeMismatch) {
		t.Fatalf("wrong code err=%v want CodeMismatch", err)
	}
	if err := st.ConfirmSignUpCognitoUser(client.ClientID, "secure-bob", code); err != nil {
		t.Fatal(err)
	}
	err = st.ConfirmSignUpCognitoUser(client.ClientID, "secure-bob", code)
	if !errors.Is(err, store.ErrCognitoCodeMismatch) {
		t.Fatalf("reuse err=%v want CodeMismatch", err)
	}
}

func TestCognitoInsecureCodesRestoresStubs(t *testing.T) {
	st := openCognitoTriggerStore(t)
	st.SetCognitoInsecureCodes(true)
	account := "000000000001"
	pool, err := st.CreateCognitoUserPool(account, "us-east-1", "insecure-pool")
	if err != nil {
		t.Fatal(err)
	}
	client, err := st.CreateCognitoUserPoolClient(account, pool.PoolID, "app")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AdminCreateCognitoUser(account, pool.PoolID, "insecure-cfp", "OldPass1!"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ForgotPasswordCognitoUser(client.ClientID, "insecure-cfp"); err != nil {
		t.Fatal(err)
	}
	code, err := st.PeekCognitoConfirmationCode(account, pool.PoolID, "insecure-cfp", store.CognitoConfirmPurposeForgotPassword)
	if err != nil {
		t.Fatal(err)
	}
	if code != store.CognitoLabConfirmationCode {
		t.Fatalf("forgot code=%q want stub", code)
	}
	if err := st.ConfirmForgotPasswordCognitoUser(client.ClientID, "insecure-cfp", store.CognitoLabConfirmationCode, "NewPass9!"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.SignUpCognitoUser(account, client.ClientID, "insecure-bob", "Secret2!"); err != nil {
		t.Fatal(err)
	}
	if err := st.ConfirmSignUpCognitoUser(client.ClientID, "insecure-bob", "any-non-empty"); err != nil {
		t.Fatal(err)
	}
}
