package store_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestSignUpRendersAndStoresCustomMessage(t *testing.T) {
	st := openCognitoTriggerStore(t)
	account := "000000000001"
	pool, err := st.CreateCognitoUserPool(account, "us-east-1", "cm-render-pool")
	if err != nil {
		t.Fatal(err)
	}
	client, err := st.CreateCognitoUserPoolClient(account, pool.PoolID, "app")
	if err != nil {
		t.Fatal(err)
	}
	fnARN := "arn:aws:lambda:us-east-1:" + account + ":function:cm-render"
	if _, err := st.SetCognitoUserPoolTriggers(account, pool.PoolID, "", store.CognitoLambdaConfig{
		CustomMessage: fnARN,
	}); err != nil {
		t.Fatal(err)
	}
	st.SetCognitoTriggerInvoker(func(_, eventJSON string) ([]byte, error) {
		var ev map[string]any
		if err := json.Unmarshal([]byte(eventJSON), &ev); err != nil {
			return nil, err
		}
		ev["response"] = map[string]any{
			"smsMessage":   "Your code is {####}",
			"emailMessage": "Hello {username}, code {####}",
			"emailSubject": "Welcome",
		}
		b, err := json.Marshal(ev)
		if err != nil {
			return nil, err
		}
		return b, nil
	})
	if _, _, err := st.SignUpCognitoUser(account, client.ClientID, "iris", "Secret9!"); err != nil {
		t.Fatal(err)
	}
	code, err := st.PeekCognitoConfirmationCode(account, pool.PoolID, "iris", store.CognitoConfirmPurposeSignUp)
	if err != nil {
		t.Fatal(err)
	}
	msg, err := st.GetLastCognitoCustomMessage(account, pool.PoolID, "iris")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg.SMSMessage, code) {
		t.Fatalf("sms=%q want code %q", msg.SMSMessage, code)
	}
	if !strings.Contains(msg.EmailMessage, "iris") || !strings.Contains(msg.EmailMessage, code) {
		t.Fatalf("email=%q want code %q", msg.EmailMessage, code)
	}
	if msg.EmailSubject != "Welcome" {
		t.Fatalf("subject=%q", msg.EmailSubject)
	}
}

func TestSignUpCustomMessageRejectsMissingCodeParameter(t *testing.T) {
	st := openCognitoTriggerStore(t)
	account := "000000000001"
	pool, err := st.CreateCognitoUserPool(account, "us-east-1", "cm-bad-pool")
	if err != nil {
		t.Fatal(err)
	}
	client, err := st.CreateCognitoUserPoolClient(account, pool.PoolID, "app")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetCognitoUserPoolTriggers(account, pool.PoolID, "", store.CognitoLambdaConfig{
		CustomMessage: "arn:aws:lambda:us-east-1:" + account + ":function:cm-bad",
	}); err != nil {
		t.Fatal(err)
	}
	st.SetCognitoTriggerInvoker(func(_, eventJSON string) ([]byte, error) {
		var ev map[string]any
		_ = json.Unmarshal([]byte(eventJSON), &ev)
		ev["response"] = map[string]any{"emailMessage": "no code here"}
		b, _ := json.Marshal(ev)
		return b, nil
	})
	_, _, err = st.SignUpCognitoUser(account, client.ClientID, "jade", "Secret0!")
	if !errors.Is(err, store.ErrCognitoInvalidLambdaResponse) {
		t.Fatalf("err=%v want InvalidLambdaResponse", err)
	}
}

func TestAdminCreateRendersAndStoresCustomMessage(t *testing.T) {
	st := openCognitoTriggerStore(t)
	account := "000000000001"
	pool, err := st.CreateCognitoUserPool(account, "us-east-1", "cm-admin-render-pool")
	if err != nil {
		t.Fatal(err)
	}
	fnARN := "arn:aws:lambda:us-east-1:" + account + ":function:cm-admin-render"
	if _, err := st.SetCognitoUserPoolTriggers(account, pool.PoolID, "", store.CognitoLambdaConfig{
		CustomMessage: fnARN,
	}); err != nil {
		t.Fatal(err)
	}
	var seenEvent string
	st.SetCognitoTriggerInvoker(func(_, eventJSON string) ([]byte, error) {
		seenEvent = eventJSON
		var ev map[string]any
		if err := json.Unmarshal([]byte(eventJSON), &ev); err != nil {
			return nil, err
		}
		ev["response"] = map[string]any{
			"smsMessage":   "Temp password {####}",
			"emailMessage": "Hello {username}, code {####}",
			"emailSubject": "Admin welcome",
		}
		b, err := json.Marshal(ev)
		if err != nil {
			return nil, err
		}
		return b, nil
	})
	if _, err := st.AdminCreateCognitoUser(account, pool.PoolID, "nova", "Secret9!"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(seenEvent, "CustomMessage_AdminCreateUser") {
		t.Fatalf("event=%s", seenEvent)
	}
	if !strings.Contains(seenEvent, `"codeParameter"`) {
		t.Fatalf("want codeParameter in event: %s", seenEvent)
	}
	msg, err := st.GetLastCognitoCustomMessage(account, pool.PoolID, "nova")
	if err != nil {
		t.Fatal(err)
	}
	if msg.TriggerSource != "CustomMessage_AdminCreateUser" {
		t.Fatalf("trigger=%q", msg.TriggerSource)
	}
	if !strings.Contains(msg.SMSMessage, "123456") {
		t.Fatalf("sms=%q", msg.SMSMessage)
	}
	if !strings.Contains(msg.EmailMessage, "nova") || !strings.Contains(msg.EmailMessage, "123456") {
		t.Fatalf("email=%q", msg.EmailMessage)
	}
	if msg.EmailSubject != "Admin welcome" {
		t.Fatalf("subject=%q", msg.EmailSubject)
	}
}

func TestAdminCreateCustomMessageRejectsMissingCodeParameter(t *testing.T) {
	st := openCognitoTriggerStore(t)
	account := "000000000001"
	pool, err := st.CreateCognitoUserPool(account, "us-east-1", "cm-admin-bad-pool")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetCognitoUserPoolTriggers(account, pool.PoolID, "", store.CognitoLambdaConfig{
		CustomMessage: "arn:aws:lambda:us-east-1:" + account + ":function:cm-admin-bad",
	}); err != nil {
		t.Fatal(err)
	}
	st.SetCognitoTriggerInvoker(func(_, eventJSON string) ([]byte, error) {
		var ev map[string]any
		_ = json.Unmarshal([]byte(eventJSON), &ev)
		ev["response"] = map[string]any{"emailMessage": "no code here"}
		b, _ := json.Marshal(ev)
		return b, nil
	})
	_, err = st.AdminCreateCognitoUser(account, pool.PoolID, "jade-admin", "Secret0!")
	if !errors.Is(err, store.ErrCognitoInvalidLambdaResponse) {
		t.Fatalf("err=%v want InvalidLambdaResponse", err)
	}
}

func TestAdminCreateCustomMessageFailClosed(t *testing.T) {
	st := openCognitoTriggerStore(t)
	account := "000000000001"
	pool, err := st.CreateCognitoUserPool(account, "us-east-1", "cm-admin-fail-pool")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetCognitoUserPoolTriggers(account, pool.PoolID, "", store.CognitoLambdaConfig{
		CustomMessage: "arn:aws:lambda:us-east-1:" + account + ":function:missing-cm-admin",
	}); err != nil {
		t.Fatal(err)
	}
	st.SetCognitoTriggerInvoker(func(_, _ string) ([]byte, error) {
		return nil, errors.New("invoke failed")
	})
	_, err = st.AdminCreateCognitoUser(account, pool.PoolID, "hank-admin", "Secret7!")
	if !errors.Is(err, store.ErrCognitoTriggerFailed) {
		t.Fatalf("err=%v want ErrCognitoTriggerFailed", err)
	}
}

func TestCustomAuthChallengeRoundTrip(t *testing.T) {
	st := openCognitoTriggerStore(t)
	account := "000000000001"
	pool, err := st.CreateCognitoUserPool(account, "us-east-1", "custom-auth-pool")
	if err != nil {
		t.Fatal(err)
	}
	client, err := st.CreateCognitoUserPoolClient(account, pool.PoolID, "app")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AdminCreateCognitoUser(account, pool.PoolID, "kara", "Secret1!"); err != nil {
		t.Fatal(err)
	}
	defineARN := "arn:aws:lambda:us-east-1:" + account + ":function:define"
	createARN := "arn:aws:lambda:us-east-1:" + account + ":function:create"
	verifyARN := "arn:aws:lambda:us-east-1:" + account + ":function:verify"
	if _, err := st.SetCognitoUserPoolTriggers(account, pool.PoolID, "", store.CognitoLambdaConfig{
		DefineAuthChallenge:         defineARN,
		CreateAuthChallenge:         createARN,
		VerifyAuthChallengeResponse: verifyARN,
	}); err != nil {
		t.Fatal(err)
	}
	st.SetCognitoTriggerInvoker(func(arn, eventJSON string) ([]byte, error) {
		var ev map[string]any
		if err := json.Unmarshal([]byte(eventJSON), &ev); err != nil {
			return nil, err
		}
		req, _ := ev["request"].(map[string]any)
		switch arn {
		case defineARN:
			session, _ := req["session"].([]any)
			if len(session) == 0 {
				ev["response"] = map[string]any{
					"challengeName":      "CUSTOM_CHALLENGE",
					"issueTokens":        false,
					"failAuthentication": false,
				}
			} else {
				entry, _ := session[0].(map[string]any)
				ok := entry["challengeResult"] == true
				ev["response"] = map[string]any{
					"issueTokens":        ok,
					"failAuthentication": !ok,
				}
			}
		case createARN:
			ev["response"] = map[string]any{
				"publicChallengeParameters":  map[string]any{"prompt": "favorite color?"},
				"privateChallengeParameters": map[string]any{"answer": "blue"},
				"challengeMetadata":          "color",
			}
		case verifyARN:
			priv, _ := req["privateChallengeParameters"].(map[string]any)
			ans, _ := req["challengeAnswer"].(string)
			ev["response"] = map[string]any{
				"answerCorrect": priv["answer"] == ans,
			}
		default:
			t.Fatalf("unexpected arn %s", arn)
		}
		b, err := json.Marshal(ev)
		if err != nil {
			return nil, err
		}
		return b, nil
	})

	chal, err := st.InitiateCognitoCustomAuth(client.ClientID, "kara", "")
	if err != nil {
		t.Fatal(err)
	}
	if chal.ChallengeName != "CUSTOM_CHALLENGE" || chal.Session == "" {
		t.Fatalf("challenge=%+v", chal)
	}
	if chal.ChallengeParameters["prompt"] != "favorite color?" {
		t.Fatalf("params=%v", chal.ChallengeParameters)
	}

	out, err := st.RespondToCognitoCUSTOMChallenge(client.ClientID, chal.Session, map[string]string{
		"USERNAME": "kara",
		"ANSWER":   "blue",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.AccessToken == "" || out.IDToken == "" {
		t.Fatalf("want tokens: %+v", out)
	}
}

func TestCustomAuthWrongAnswerFails(t *testing.T) {
	st := openCognitoTriggerStore(t)
	account := "000000000001"
	pool, err := st.CreateCognitoUserPool(account, "us-east-1", "custom-auth-fail")
	if err != nil {
		t.Fatal(err)
	}
	client, err := st.CreateCognitoUserPoolClient(account, pool.PoolID, "app")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AdminCreateCognitoUser(account, pool.PoolID, "liam", "Secret2!"); err != nil {
		t.Fatal(err)
	}
	defineARN := "arn:aws:lambda:us-east-1:" + account + ":function:define2"
	createARN := "arn:aws:lambda:us-east-1:" + account + ":function:create2"
	verifyARN := "arn:aws:lambda:us-east-1:" + account + ":function:verify2"
	if _, err := st.SetCognitoUserPoolTriggers(account, pool.PoolID, "", store.CognitoLambdaConfig{
		DefineAuthChallenge:         defineARN,
		CreateAuthChallenge:         createARN,
		VerifyAuthChallengeResponse: verifyARN,
	}); err != nil {
		t.Fatal(err)
	}
	st.SetCognitoTriggerInvoker(func(arn, eventJSON string) ([]byte, error) {
		var ev map[string]any
		_ = json.Unmarshal([]byte(eventJSON), &ev)
		req, _ := ev["request"].(map[string]any)
		switch arn {
		case defineARN:
			session, _ := req["session"].([]any)
			if len(session) == 0 {
				ev["response"] = map[string]any{
					"challengeName": "CUSTOM_CHALLENGE", "issueTokens": false, "failAuthentication": false,
				}
			} else {
				ev["response"] = map[string]any{"issueTokens": false, "failAuthentication": true}
			}
		case createARN:
			ev["response"] = map[string]any{
				"publicChallengeParameters":  map[string]any{},
				"privateChallengeParameters": map[string]any{"answer": "yes"},
			}
		case verifyARN:
			ev["response"] = map[string]any{"answerCorrect": false}
		}
		b, _ := json.Marshal(ev)
		return b, nil
	})
	chal, err := st.InitiateCognitoCustomAuth(client.ClientID, "liam", "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.RespondToCognitoCUSTOMChallenge(client.ClientID, chal.Session, map[string]string{
		"USERNAME": "liam",
		"ANSWER":   "no",
	})
	if !errors.Is(err, store.ErrCognitoUnauthorized) {
		t.Fatalf("err=%v want unauthorized", err)
	}
}

func TestForgotPasswordRendersAndStoresCustomMessage(t *testing.T) {
	st := openCognitoTriggerStore(t)
	account := "000000000001"
	pool, err := st.CreateCognitoUserPool(account, "us-east-1", "cm-forgot-pool")
	if err != nil {
		t.Fatal(err)
	}
	client, err := st.CreateCognitoUserPoolClient(account, pool.PoolID, "app")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AdminCreateCognitoUser(account, pool.PoolID, "forgot-user", "Secret9!"); err != nil {
		t.Fatal(err)
	}
	fnARN := "arn:aws:lambda:us-east-1:" + account + ":function:cm-forgot"
	if _, err := st.SetCognitoUserPoolTriggers(account, pool.PoolID, "", store.CognitoLambdaConfig{
		CustomMessage: fnARN,
	}); err != nil {
		t.Fatal(err)
	}
	var seenEvent string
	st.SetCognitoTriggerInvoker(func(_, eventJSON string) ([]byte, error) {
		seenEvent = eventJSON
		var ev map[string]any
		if err := json.Unmarshal([]byte(eventJSON), &ev); err != nil {
			return nil, err
		}
		ev["response"] = map[string]any{
			"smsMessage":   "Reset code {####}",
			"emailMessage": "Hello {username}, reset {####}",
			"emailSubject": "Reset",
		}
		b, err := json.Marshal(ev)
		if err != nil {
			return nil, err
		}
		return b, nil
	})
	details, err := st.ForgotPasswordCognitoUser(client.ClientID, "forgot-user")
	if err != nil {
		t.Fatal(err)
	}
	if details.DeliveryMedium != "EMAIL" {
		t.Fatalf("details=%+v", details)
	}
	if !strings.Contains(seenEvent, "CustomMessage_ForgotPassword") {
		t.Fatalf("event=%s", seenEvent)
	}
	code, err := st.PeekCognitoConfirmationCode(account, pool.PoolID, "forgot-user", store.CognitoConfirmPurposeForgotPassword)
	if err != nil {
		t.Fatal(err)
	}
	msg, err := st.GetLastCognitoCustomMessage(account, pool.PoolID, "forgot-user")
	if err != nil {
		t.Fatal(err)
	}
	if msg.TriggerSource != "CustomMessage_ForgotPassword" {
		t.Fatalf("trigger=%q", msg.TriggerSource)
	}
	if !strings.Contains(msg.SMSMessage, code) {
		t.Fatalf("sms=%q want code %q", msg.SMSMessage, code)
	}
	if !strings.Contains(msg.EmailMessage, "forgot-user") || !strings.Contains(msg.EmailMessage, code) {
		t.Fatalf("email=%q want code %q", msg.EmailMessage, code)
	}
}

func TestForgotPasswordCustomMessageRejectsMissingCodeParameter(t *testing.T) {
	st := openCognitoTriggerStore(t)
	account := "000000000001"
	pool, err := st.CreateCognitoUserPool(account, "us-east-1", "cm-forgot-bad")
	if err != nil {
		t.Fatal(err)
	}
	client, err := st.CreateCognitoUserPoolClient(account, pool.PoolID, "app")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AdminCreateCognitoUser(account, pool.PoolID, "forgot-bad", "Secret0!"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetCognitoUserPoolTriggers(account, pool.PoolID, "", store.CognitoLambdaConfig{
		CustomMessage: "arn:aws:lambda:us-east-1:" + account + ":function:cm-forgot-bad",
	}); err != nil {
		t.Fatal(err)
	}
	st.SetCognitoTriggerInvoker(func(_, eventJSON string) ([]byte, error) {
		var ev map[string]any
		_ = json.Unmarshal([]byte(eventJSON), &ev)
		ev["response"] = map[string]any{"emailMessage": "no code here"}
		b, _ := json.Marshal(ev)
		return b, nil
	})
	_, err = st.ForgotPasswordCognitoUser(client.ClientID, "forgot-bad")
	if !errors.Is(err, store.ErrCognitoInvalidLambdaResponse) {
		t.Fatalf("err=%v want InvalidLambdaResponse", err)
	}
}

func TestForgotPasswordCustomMessageFailClosed(t *testing.T) {
	st := openCognitoTriggerStore(t)
	account := "000000000001"
	pool, err := st.CreateCognitoUserPool(account, "us-east-1", "cm-forgot-fail")
	if err != nil {
		t.Fatal(err)
	}
	client, err := st.CreateCognitoUserPoolClient(account, pool.PoolID, "app")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AdminCreateCognitoUser(account, pool.PoolID, "forgot-fail", "Secret7!"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetCognitoUserPoolTriggers(account, pool.PoolID, "", store.CognitoLambdaConfig{
		CustomMessage: "arn:aws:lambda:us-east-1:" + account + ":function:missing-forgot",
	}); err != nil {
		t.Fatal(err)
	}
	st.SetCognitoTriggerInvoker(func(_, _ string) ([]byte, error) {
		return nil, errors.New("invoke failed")
	})
	_, err = st.ForgotPasswordCognitoUser(client.ClientID, "forgot-fail")
	if !errors.Is(err, store.ErrCognitoTriggerFailed) {
		t.Fatalf("err=%v want ErrCognitoTriggerFailed", err)
	}
}

func TestResendConfirmationCodeRendersAndStoresCustomMessage(t *testing.T) {
	st := openCognitoTriggerStore(t)
	account := "000000000001"
	pool, err := st.CreateCognitoUserPool(account, "us-east-1", "cm-resend-pool")
	if err != nil {
		t.Fatal(err)
	}
	client, err := st.CreateCognitoUserPoolClient(account, pool.PoolID, "app")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetCognitoUserPoolTriggers(account, pool.PoolID, "", store.CognitoLambdaConfig{
		CustomMessage: "arn:aws:lambda:us-east-1:" + account + ":function:cm-resend",
	}); err != nil {
		t.Fatal(err)
	}
	st.SetCognitoTriggerInvoker(func(_, eventJSON string) ([]byte, error) {
		var ev map[string]any
		if err := json.Unmarshal([]byte(eventJSON), &ev); err != nil {
			return nil, err
		}
		src, _ := ev["triggerSource"].(string)
		ev["response"] = map[string]any{
			"smsMessage":   "Code {####}",
			"emailMessage": "Hello {username}, code {####}",
			"emailSubject": src,
		}
		b, err := json.Marshal(ev)
		if err != nil {
			return nil, err
		}
		return b, nil
	})
	if _, _, err := st.SignUpCognitoUser(account, client.ClientID, "resend-user", "Secret9!"); err != nil {
		t.Fatal(err)
	}
	details, err := st.ResendConfirmationCodeCognitoUser(client.ClientID, "resend-user")
	if err != nil {
		t.Fatal(err)
	}
	if details.AttributeName != "email" {
		t.Fatalf("details=%+v", details)
	}
	code, err := st.PeekCognitoConfirmationCode(account, pool.PoolID, "resend-user", store.CognitoConfirmPurposeSignUp)
	if err != nil {
		t.Fatal(err)
	}
	msg, err := st.GetLastCognitoCustomMessage(account, pool.PoolID, "resend-user")
	if err != nil {
		t.Fatal(err)
	}
	if msg.TriggerSource != "CustomMessage_ResendCode" {
		t.Fatalf("trigger=%q", msg.TriggerSource)
	}
	if !strings.Contains(msg.EmailMessage, code) || !strings.Contains(msg.EmailMessage, "resend-user") {
		t.Fatalf("email=%q want code %q", msg.EmailMessage, code)
	}
}

func TestResendConfirmationCodeCustomMessageRejectsMissingCodeParameter(t *testing.T) {
	st := openCognitoTriggerStore(t)
	account := "000000000001"
	pool, err := st.CreateCognitoUserPool(account, "us-east-1", "cm-resend-bad")
	if err != nil {
		t.Fatal(err)
	}
	client, err := st.CreateCognitoUserPoolClient(account, pool.PoolID, "app")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetCognitoUserPoolTriggers(account, pool.PoolID, "", store.CognitoLambdaConfig{
		CustomMessage: "arn:aws:lambda:us-east-1:" + account + ":function:cm-resend-bad",
	}); err != nil {
		t.Fatal(err)
	}
	call := 0
	st.SetCognitoTriggerInvoker(func(_, eventJSON string) ([]byte, error) {
		call++
		var ev map[string]any
		_ = json.Unmarshal([]byte(eventJSON), &ev)
		if call == 1 {
			// SignUp CustomMessage OK
			ev["response"] = map[string]any{"emailMessage": "code {####}"}
		} else {
			ev["response"] = map[string]any{"emailMessage": "no code"}
		}
		b, _ := json.Marshal(ev)
		return b, nil
	})
	if _, _, err := st.SignUpCognitoUser(account, client.ClientID, "resend-bad", "Secret0!"); err != nil {
		t.Fatal(err)
	}
	_, err = st.ResendConfirmationCodeCognitoUser(client.ClientID, "resend-bad")
	if !errors.Is(err, store.ErrCognitoInvalidLambdaResponse) {
		t.Fatalf("err=%v want InvalidLambdaResponse", err)
	}
}

func TestCustomAuthSRPNestingThenCustomChallenge(t *testing.T) {
	st := openCognitoTriggerStore(t)
	account := "000000000001"
	pool, err := st.CreateCognitoUserPool(account, "us-east-1", "custom-srp-nest")
	if err != nil {
		t.Fatal(err)
	}
	client, err := st.CreateCognitoUserPoolClient(account, pool.PoolID, "app")
	if err != nil {
		t.Fatal(err)
	}
	password := "SecretNest1!"
	if _, err := st.AdminCreateCognitoUser(account, pool.PoolID, "nest-user", password); err != nil {
		t.Fatal(err)
	}
	defineARN := "arn:aws:lambda:us-east-1:" + account + ":function:define-nest"
	createARN := "arn:aws:lambda:us-east-1:" + account + ":function:create-nest"
	verifyARN := "arn:aws:lambda:us-east-1:" + account + ":function:verify-nest"
	if _, err := st.SetCognitoUserPoolTriggers(account, pool.PoolID, "", store.CognitoLambdaConfig{
		DefineAuthChallenge:         defineARN,
		CreateAuthChallenge:         createARN,
		VerifyAuthChallengeResponse: verifyARN,
	}); err != nil {
		t.Fatal(err)
	}
	st.SetCognitoTriggerInvoker(func(arn, eventJSON string) ([]byte, error) {
		var ev map[string]any
		if err := json.Unmarshal([]byte(eventJSON), &ev); err != nil {
			return nil, err
		}
		req, _ := ev["request"].(map[string]any)
		switch arn {
		case defineARN:
			session, _ := req["session"].([]any)
			switch len(session) {
			case 1:
				entry, _ := session[0].(map[string]any)
				if entry["challengeName"] != "SRP_A" {
					t.Fatalf("want SRP_A session entry: %+v", entry)
				}
				ev["response"] = map[string]any{
					"challengeName": "PASSWORD_VERIFIER", "issueTokens": false, "failAuthentication": false,
				}
			case 2:
				entry, _ := session[1].(map[string]any)
				if entry["challengeName"] != "PASSWORD_VERIFIER" || entry["challengeResult"] != true {
					t.Fatalf("want PASSWORD_VERIFIER true: %+v", entry)
				}
				ev["response"] = map[string]any{
					"challengeName": "CUSTOM_CHALLENGE", "issueTokens": false, "failAuthentication": false,
				}
			case 3:
				entry, _ := session[2].(map[string]any)
				ok := entry["challengeResult"] == true
				ev["response"] = map[string]any{"issueTokens": ok, "failAuthentication": !ok}
			default:
				t.Fatalf("unexpected session len=%d", len(session))
			}
		case createARN:
			ev["response"] = map[string]any{
				"publicChallengeParameters":  map[string]any{"prompt": "pin?"},
				"privateChallengeParameters": map[string]any{"answer": "42"},
			}
		case verifyARN:
			priv, _ := req["privateChallengeParameters"].(map[string]any)
			ans, _ := req["challengeAnswer"].(string)
			ev["response"] = map[string]any{"answerCorrect": priv["answer"] == ans}
		default:
			t.Fatalf("unexpected arn %s", arn)
		}
		b, err := json.Marshal(ev)
		if err != nil {
			return nil, err
		}
		return b, nil
	})

	srpClient, err := store.NewCognitoSRPClient(pool.PoolID, "nest-user", password)
	if err != nil {
		t.Fatal(err)
	}
	chal, err := st.InitiateCognitoCustomAuth(client.ClientID, "nest-user", srpClient.SRPAHex())
	if err != nil {
		t.Fatal(err)
	}
	if chal.ChallengeName != "PASSWORD_VERIFIER" || chal.Session == "" {
		t.Fatalf("challenge=%+v", chal)
	}
	if chal.ChallengeParameters["SALT"] == "" || chal.ChallengeParameters["SRP_B"] == "" {
		t.Fatalf("missing SRP params: %+v", chal.ChallengeParameters)
	}

	responses, err := srpClient.PasswordVerifierChallengeResponses(chal.ChallengeParameters, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	afterSRP, err := st.RespondToCognitoPASSWORDVerifierChallenge(client.ClientID, chal.Session, responses)
	if err != nil {
		t.Fatal(err)
	}
	if afterSRP.ChallengeName != "CUSTOM_CHALLENGE" || afterSRP.Session == "" {
		t.Fatalf("want CUSTOM_CHALLENGE after SRP: %+v", afterSRP)
	}
	if afterSRP.ChallengeParameters["prompt"] != "pin?" {
		t.Fatalf("params=%v", afterSRP.ChallengeParameters)
	}

	out, err := st.RespondToCognitoCUSTOMChallenge(client.ClientID, afterSRP.Session, map[string]string{
		"USERNAME": "nest-user",
		"ANSWER":   "42",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.AccessToken == "" || out.IDToken == "" {
		t.Fatalf("want tokens: %+v", out)
	}
}
