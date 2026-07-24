package store_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

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
	msg, err := st.GetLastCognitoCustomMessage(account, pool.PoolID, "iris")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg.SMSMessage, "123456") {
		t.Fatalf("sms=%q", msg.SMSMessage)
	}
	if !strings.Contains(msg.EmailMessage, "iris") || !strings.Contains(msg.EmailMessage, "123456") {
		t.Fatalf("email=%q", msg.EmailMessage)
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

	chal, err := st.InitiateCognitoCustomAuth(client.ClientID, "kara")
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
	chal, err := st.InitiateCognitoCustomAuth(client.ClientID, "liam")
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
