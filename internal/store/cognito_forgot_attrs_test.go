package store_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestConfirmForgotPasswordHappyPath(t *testing.T) {
	st := openCognitoTriggerStore(t)
	account := "000000000001"
	pool, err := st.CreateCognitoUserPool(account, "us-east-1", "cfp-pool")
	if err != nil {
		t.Fatal(err)
	}
	client, err := st.CreateCognitoUserPoolClient(account, pool.PoolID, "app")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AdminCreateCognitoUser(account, pool.PoolID, "cfp-user", "OldPass1!"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ForgotPasswordCognitoUser(client.ClientID, "cfp-user"); err != nil {
		t.Fatal(err)
	}
	if err := st.ConfirmForgotPasswordCognitoUser(client.ClientID, "cfp-user", store.CognitoLabConfirmationCode, "NewPass9!"); err != nil {
		t.Fatal(err)
	}
	outcome, err := st.InitiateCognitoAuth(client.ClientID, "cfp-user", "NewPass9!")
	if err != nil {
		t.Fatal(err)
	}
	if outcome.AccessToken == "" {
		t.Fatal("expected access token after password reset")
	}
	_, err = st.InitiateCognitoAuth(client.ClientID, "cfp-user", "OldPass1!")
	if !errors.Is(err, store.ErrCognitoUnauthorized) {
		t.Fatalf("old password err=%v want unauthorized", err)
	}
}

func TestConfirmForgotPasswordWrongCode(t *testing.T) {
	st := openCognitoTriggerStore(t)
	account := "000000000001"
	pool, err := st.CreateCognitoUserPool(account, "us-east-1", "cfp-bad")
	if err != nil {
		t.Fatal(err)
	}
	client, err := st.CreateCognitoUserPoolClient(account, pool.PoolID, "app")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AdminCreateCognitoUser(account, pool.PoolID, "cfp-bad", "Secret1!"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ForgotPasswordCognitoUser(client.ClientID, "cfp-bad"); err != nil {
		t.Fatal(err)
	}
	err = st.ConfirmForgotPasswordCognitoUser(client.ClientID, "cfp-bad", "000000", "NewPass9!")
	if !errors.Is(err, store.ErrCognitoCodeMismatch) {
		t.Fatalf("err=%v want CodeMismatch", err)
	}
}

func TestConfirmForgotPasswordWithoutForgot(t *testing.T) {
	st := openCognitoTriggerStore(t)
	account := "000000000001"
	pool, err := st.CreateCognitoUserPool(account, "us-east-1", "cfp-none")
	if err != nil {
		t.Fatal(err)
	}
	client, err := st.CreateCognitoUserPoolClient(account, pool.PoolID, "app")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AdminCreateCognitoUser(account, pool.PoolID, "cfp-none", "Secret1!"); err != nil {
		t.Fatal(err)
	}
	err = st.ConfirmForgotPasswordCognitoUser(client.ClientID, "cfp-none", store.CognitoLabConfirmationCode, "NewPass9!")
	if !errors.Is(err, store.ErrCognitoCodeMismatch) {
		t.Fatalf("err=%v want CodeMismatch", err)
	}
}

func TestUpdateUserAttributesCustomMessageAndVerify(t *testing.T) {
	st := openCognitoTriggerStore(t)
	account := "000000000001"
	pool, err := st.CreateCognitoUserPool(account, "us-east-1", "attr-pool")
	if err != nil {
		t.Fatal(err)
	}
	client, err := st.CreateCognitoUserPoolClient(account, pool.PoolID, "app")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AdminCreateCognitoUser(account, pool.PoolID, "attr-user", "Secret1!"); err != nil {
		t.Fatal(err)
	}
	fnARN := "arn:aws:lambda:us-east-1:" + account + ":function:cm-attr"
	if _, err := st.SetCognitoUserPoolTriggers(account, pool.PoolID, "", store.CognitoLambdaConfig{
		CustomMessage: fnARN,
	}); err != nil {
		t.Fatal(err)
	}
	var seenSources []string
	st.SetCognitoTriggerInvoker(func(_, eventJSON string) ([]byte, error) {
		var ev map[string]any
		if err := json.Unmarshal([]byte(eventJSON), &ev); err != nil {
			return nil, err
		}
		src, _ := ev["triggerSource"].(string)
		seenSources = append(seenSources, src)
		ev["response"] = map[string]any{
			"smsMessage":   "Code {####}",
			"emailMessage": "Hello {username}, code {####}",
			"emailSubject": "Verify",
		}
		b, err := json.Marshal(ev)
		if err != nil {
			return nil, err
		}
		return b, nil
	})
	auth, err := st.InitiateCognitoAuth(client.ClientID, "attr-user", "Secret1!")
	if err != nil {
		t.Fatal(err)
	}
	details, err := st.UpdateUserAttributesCognitoUser(auth.AccessToken, map[string]string{
		"email": "iris@example.com",
		"name":  "Iris",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(details) != 1 || details[0].AttributeName != "email" {
		t.Fatalf("details=%+v", details)
	}
	foundUpdate := false
	for _, src := range seenSources {
		if src == "CustomMessage_UpdateUserAttribute" {
			foundUpdate = true
			break
		}
	}
	if !foundUpdate {
		t.Fatalf("sources=%v", seenSources)
	}
	msg, err := st.GetLastCognitoCustomMessage(account, pool.PoolID, "attr-user")
	if err != nil {
		t.Fatal(err)
	}
	if msg.TriggerSource != "CustomMessage_UpdateUserAttribute" {
		t.Fatalf("trigger=%q", msg.TriggerSource)
	}
	if !strings.Contains(msg.EmailMessage, "123456") {
		t.Fatalf("email=%q", msg.EmailMessage)
	}
	attr, err := st.GetCognitoUserAttribute(account, pool.PoolID, "attr-user", "email")
	if err != nil {
		t.Fatal(err)
	}
	if attr.Value != "iris@example.com" || attr.Verified {
		t.Fatalf("attr=%+v", attr)
	}
	nameAttr, err := st.GetCognitoUserAttribute(account, pool.PoolID, "attr-user", "name")
	if err != nil {
		t.Fatal(err)
	}
	if !nameAttr.Verified {
		t.Fatalf("name should be verified without CustomMessage: %+v", nameAttr)
	}

	seenSources = nil
	codeDetails, err := st.GetUserAttributeVerificationCodeCognitoUser(auth.AccessToken, "email")
	if err != nil {
		t.Fatal(err)
	}
	if codeDetails.AttributeName != "email" {
		t.Fatalf("codeDetails=%+v", codeDetails)
	}
	foundVerify := false
	for _, src := range seenSources {
		if src == "CustomMessage_VerifyUserAttribute" {
			foundVerify = true
			break
		}
	}
	if !foundVerify {
		t.Fatalf("sources=%v want VerifyUserAttribute", seenSources)
	}
	err = st.VerifyUserAttributeCognitoUser(auth.AccessToken, "email", "000000")
	if !errors.Is(err, store.ErrCognitoCodeMismatch) {
		t.Fatalf("wrong code err=%v", err)
	}
	if err := st.VerifyUserAttributeCognitoUser(auth.AccessToken, "email", store.CognitoLabConfirmationCode); err != nil {
		t.Fatal(err)
	}
	attr, err = st.GetCognitoUserAttribute(account, pool.PoolID, "attr-user", "email")
	if err != nil {
		t.Fatal(err)
	}
	if !attr.Verified {
		t.Fatalf("email should be verified: %+v", attr)
	}
}

func TestUpdateUserAttributesWithoutCustomMessage(t *testing.T) {
	st := openCognitoTriggerStore(t)
	account := "000000000001"
	pool, err := st.CreateCognitoUserPool(account, "us-east-1", "attr-plain")
	if err != nil {
		t.Fatal(err)
	}
	client, err := st.CreateCognitoUserPoolClient(account, pool.PoolID, "app")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AdminCreateCognitoUser(account, pool.PoolID, "plain-user", "Secret1!"); err != nil {
		t.Fatal(err)
	}
	auth, err := st.InitiateCognitoAuth(client.ClientID, "plain-user", "Secret1!")
	if err != nil {
		t.Fatal(err)
	}
	details, err := st.UpdateUserAttributesCognitoUser(auth.AccessToken, map[string]string{
		"email": "plain@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(details) != 1 {
		t.Fatalf("details=%+v", details)
	}
	if err := st.VerifyUserAttributeCognitoUser(auth.AccessToken, "email", store.CognitoLabConfirmationCode); err != nil {
		t.Fatal(err)
	}
}
