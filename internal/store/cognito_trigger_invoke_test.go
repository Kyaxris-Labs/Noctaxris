package store_test

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/jwtutil"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func openCognitoTriggerStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	key, err := store.LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureCognitoSchema(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestBuildCognitoTriggerEventJSONPostConfirmation(t *testing.T) {
	raw, err := store.BuildCognitoTriggerEventJSON(store.CognitoTriggerEventInput{
		TriggerSource: "PostConfirmation_ConfirmSignUp",
		Region:        "us-east-1",
		UserPoolID:    "us-east-1_abc",
		Username:      "alice",
		ClientID:      "client1",
		UserSub:       "sub-1",
		UserStatus:    "CONFIRMED",
	})
	if err != nil {
		t.Fatal(err)
	}
	var evt map[string]any
	if err := json.Unmarshal([]byte(raw), &evt); err != nil {
		t.Fatal(err)
	}
	if evt["triggerSource"] != "PostConfirmation_ConfirmSignUp" {
		t.Fatalf("triggerSource=%v", evt["triggerSource"])
	}
	req, _ := evt["request"].(map[string]any)
	attrs, _ := req["userAttributes"].(map[string]any)
	if attrs["sub"] != "sub-1" || attrs["cognito:user_status"] != "CONFIRMED" {
		t.Fatalf("attrs=%v", attrs)
	}
}

func TestConfirmSignUpInvokesPostConfirmationTrigger(t *testing.T) {
	st := openCognitoTriggerStore(t)
	account := "000000000001"
	pool, err := st.CreateCognitoUserPool(account, "us-east-1", "trig-pool")
	if err != nil {
		t.Fatal(err)
	}
	client, err := st.CreateCognitoUserPoolClient(account, pool.PoolID, "app")
	if err != nil {
		t.Fatal(err)
	}
	fnARN := "arn:aws:lambda:us-east-1:" + account + ":function:post-confirm"
	if _, err := st.SetCognitoUserPoolTriggers(account, pool.PoolID, "", store.CognitoLambdaConfig{
		PostConfirmation: fnARN,
	}); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var seenARN, seenEvent string
	st.SetCognitoTriggerInvoker(func(functionARN, eventJSON string) ([]byte, error) {
		mu.Lock()
		defer mu.Unlock()
		seenARN = functionARN
		seenEvent = eventJSON
		return []byte(eventJSON), nil
	})

	if _, _, err := st.SignUpCognitoUser(account, client.ClientID, "alice", "Secret1!"); err != nil {
		t.Fatal(err)
	}
	if err := st.ConfirmSignUpCognitoUser(client.ClientID, "alice", "123456"); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if seenARN != fnARN {
		t.Fatalf("invoked ARN=%q want %q", seenARN, fnARN)
	}
	if !strings.Contains(seenEvent, "PostConfirmation_ConfirmSignUp") {
		t.Fatalf("event=%s", seenEvent)
	}
}

func TestConfirmSignUpTriggerFailClosed(t *testing.T) {
	st := openCognitoTriggerStore(t)
	account := "000000000001"
	pool, err := st.CreateCognitoUserPool(account, "us-east-1", "trig-fail")
	if err != nil {
		t.Fatal(err)
	}
	client, err := st.CreateCognitoUserPoolClient(account, pool.PoolID, "app")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetCognitoUserPoolTriggers(account, pool.PoolID, "", store.CognitoLambdaConfig{
		PostConfirmation: "arn:aws:lambda:us-east-1:" + account + ":function:missing",
	}); err != nil {
		t.Fatal(err)
	}
	st.SetCognitoTriggerInvoker(func(_, _ string) ([]byte, error) {
		return nil, errors.New("boom")
	})
	if _, _, err := st.SignUpCognitoUser(account, client.ClientID, "bob", "Secret1!"); err != nil {
		t.Fatal(err)
	}
	err = st.ConfirmSignUpCognitoUser(client.ClientID, "bob", "123456")
	if !errors.Is(err, store.ErrCognitoTriggerFailed) {
		t.Fatalf("err=%v want ErrCognitoTriggerFailed", err)
	}
}

func TestInitiateAuthInvokesPreTokenGeneration(t *testing.T) {
	st := openCognitoTriggerStore(t)
	account := "000000000001"
	pool, err := st.CreateCognitoUserPool(account, "us-east-1", "pretoken-pool")
	if err != nil {
		t.Fatal(err)
	}
	client, err := st.CreateCognitoUserPoolClient(account, pool.PoolID, "app")
	if err != nil {
		t.Fatal(err)
	}
	fnARN := "arn:aws:lambda:us-east-1:" + account + ":function:pre-token"
	if _, err := st.SetCognitoUserPoolTriggers(account, pool.PoolID, "", store.CognitoLambdaConfig{
		PreTokenGeneration: fnARN,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AdminCreateCognitoUser(account, pool.PoolID, "carol", "Secret2!"); err != nil {
		t.Fatal(err)
	}

	var sources []string
	st.SetCognitoTriggerInvoker(func(functionARN, eventJSON string) ([]byte, error) {
		if functionARN != fnARN {
			t.Errorf("arn=%q", functionARN)
		}
		var evt map[string]any
		_ = json.Unmarshal([]byte(eventJSON), &evt)
		sources = append(sources, evt["triggerSource"].(string))
		return []byte(eventJSON), nil
	})

	outcome, err := st.InitiateCognitoAuth(client.ClientID, "carol", "Secret2!")
	if err != nil {
		t.Fatal(err)
	}
	if outcome.AccessToken == "" {
		t.Fatal("missing access token")
	}
	if len(sources) != 1 || sources[0] != "TokenGeneration_Authentication" {
		t.Fatalf("sources=%v", sources)
	}
}

func TestPreTokenGenerationAppliesClaimsToAddOrOverride(t *testing.T) {
	st := openCognitoTriggerStore(t)
	account := "000000000001"
	pool, err := st.CreateCognitoUserPool(account, "us-east-1", "claims-pool")
	if err != nil {
		t.Fatal(err)
	}
	client, err := st.CreateCognitoUserPoolClient(account, pool.PoolID, "app")
	if err != nil {
		t.Fatal(err)
	}
	fnARN := "arn:aws:lambda:us-east-1:" + account + ":function:claims-fn"
	if _, err := st.SetCognitoUserPoolTriggers(account, pool.PoolID, "", store.CognitoLambdaConfig{
		PreTokenGeneration: fnARN,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AdminCreateCognitoUser(account, pool.PoolID, "dave", "Secret3!"); err != nil {
		t.Fatal(err)
	}

	st.SetCognitoTriggerInvoker(func(arn, eventJSON string) ([]byte, error) {
		if arn != fnARN {
			t.Errorf("arn=%q", arn)
		}
		var ev map[string]any
		if err := json.Unmarshal([]byte(eventJSON), &ev); err != nil {
			return nil, err
		}
		resp, _ := ev["response"].(map[string]any)
		if resp == nil {
			resp = map[string]any{}
			ev["response"] = resp
		}
		resp["claimsOverrideDetails"] = map[string]any{
			"claimsToAddOrOverride": map[string]any{"lab_claim": "yes"},
			"claimsToSuppress":      []any{"email_verified"},
		}
		b, err := json.Marshal(ev)
		if err != nil {
			return nil, err
		}
		return b, nil
	})

	out, err := st.InitiateCognitoAuth(client.ClientID, "dave", "Secret3!")
	if err != nil {
		t.Fatal(err)
	}
	jwks, err := st.CognitoJWKSJSON(pool.PoolID)
	if err != nil {
		t.Fatal(err)
	}
	idClaims, err := jwtutil.VerifyCompactRS256(out.IDToken, jwks)
	if err != nil {
		t.Fatal(err)
	}
	if jwtutil.ClaimString(idClaims, "lab_claim") != "yes" {
		t.Fatalf("lab_claim=%v", idClaims["lab_claim"])
	}
	if _, ok := idClaims["email_verified"]; ok {
		t.Fatalf("email_verified should be suppressed: %v", idClaims["email_verified"])
	}
	if jwtutil.ClaimString(idClaims, "iss") == "" || jwtutil.ClaimString(idClaims, "sub") == "" {
		t.Fatal("reserved claims must remain")
	}
}

func TestPreTokenGenerationIgnoresReservedClaimOverrides(t *testing.T) {
	st := openCognitoTriggerStore(t)
	account := "000000000001"
	pool, err := st.CreateCognitoUserPool(account, "us-east-1", "reserved-pool")
	if err != nil {
		t.Fatal(err)
	}
	client, err := st.CreateCognitoUserPoolClient(account, pool.PoolID, "app")
	if err != nil {
		t.Fatal(err)
	}
	fnARN := "arn:aws:lambda:us-east-1:" + account + ":function:reserved-fn"
	if _, err := st.SetCognitoUserPoolTriggers(account, pool.PoolID, "", store.CognitoLambdaConfig{
		PreTokenGeneration: fnARN,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AdminCreateCognitoUser(account, pool.PoolID, "eve", "Secret4!"); err != nil {
		t.Fatal(err)
	}
	st.SetCognitoTriggerInvoker(func(_, eventJSON string) ([]byte, error) {
		var ev map[string]any
		_ = json.Unmarshal([]byte(eventJSON), &ev)
		ev["response"] = map[string]any{
			"claimsOverrideDetails": map[string]any{
				"claimsToAddOrOverride": map[string]any{
					"iss":       "https://evil.example",
					"sub":       "forged",
					"lab_ok":    "1",
					"token_use": "access",
				},
				"claimsToSuppress": []any{"iss", "sub"},
			},
		}
		b, _ := json.Marshal(ev)
		return b, nil
	})
	out, err := st.InitiateCognitoAuth(client.ClientID, "eve", "Secret4!")
	if err != nil {
		t.Fatal(err)
	}
	jwks, err := st.CognitoJWKSJSON(pool.PoolID)
	if err != nil {
		t.Fatal(err)
	}
	idClaims, err := jwtutil.VerifyCompactRS256(out.IDToken, jwks)
	if err != nil {
		t.Fatal(err)
	}
	iss := store.CognitoIssuerURL(pool.Region, pool.PoolID)
	if jwtutil.ClaimString(idClaims, "iss") != iss {
		t.Fatalf("iss overridden: %v", idClaims["iss"])
	}
	if jwtutil.ClaimString(idClaims, "token_use") != "id" {
		t.Fatalf("token_use=%v", idClaims["token_use"])
	}
	if jwtutil.ClaimString(idClaims, "lab_ok") != "1" {
		t.Fatalf("lab_ok=%v", idClaims["lab_ok"])
	}
}

func TestPreTokenGenerationBadLambdaPayloadFailClosed(t *testing.T) {
	st := openCognitoTriggerStore(t)
	account := "000000000001"
	pool, err := st.CreateCognitoUserPool(account, "us-east-1", "bad-payload-pool")
	if err != nil {
		t.Fatal(err)
	}
	client, err := st.CreateCognitoUserPoolClient(account, pool.PoolID, "app")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetCognitoUserPoolTriggers(account, pool.PoolID, "", store.CognitoLambdaConfig{
		PreTokenGeneration: "arn:aws:lambda:us-east-1:" + account + ":function:bad-json",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AdminCreateCognitoUser(account, pool.PoolID, "frank", "Secret5!"); err != nil {
		t.Fatal(err)
	}
	st.SetCognitoTriggerInvoker(func(_, _ string) ([]byte, error) {
		return []byte("not-json"), nil
	})
	_, err = st.InitiateCognitoAuth(client.ClientID, "frank", "Secret5!")
	if !errors.Is(err, store.ErrCognitoTriggerFailed) {
		t.Fatalf("err=%v want ErrCognitoTriggerFailed", err)
	}
}

func TestParseCognitoPreTokenClaimsOverrideAcceptsResponseOnly(t *testing.T) {
	payload := []byte(`{"claimsOverrideDetails":{"claimsToAddOrOverride":{"a":"b"},"claimsToSuppress":["c"]}}`)
	got, err := store.ParseCognitoPreTokenClaimsOverride(payload)
	if err != nil {
		t.Fatal(err)
	}
	if got.AddOrOverride["a"] != "b" {
		t.Fatalf("AddOrOverride=%v", got.AddOrOverride)
	}
	if len(got.Suppress) != 1 || got.Suppress[0] != "c" {
		t.Fatalf("Suppress=%v", got.Suppress)
	}
}

func TestSignUpInvokesCustomMessageTrigger(t *testing.T) {
	st := openCognitoTriggerStore(t)
	account := "000000000001"
	pool, err := st.CreateCognitoUserPool(account, "us-east-1", "custom-msg-pool")
	if err != nil {
		t.Fatal(err)
	}
	client, err := st.CreateCognitoUserPoolClient(account, pool.PoolID, "app")
	if err != nil {
		t.Fatal(err)
	}
	fnARN := "arn:aws:lambda:us-east-1:" + account + ":function:custom-msg"
	if _, err := st.SetCognitoUserPoolTriggers(account, pool.PoolID, "", store.CognitoLambdaConfig{
		CustomMessage: fnARN,
	}); err != nil {
		t.Fatal(err)
	}
	var seenARN, seenEvent string
	st.SetCognitoTriggerInvoker(func(functionARN, eventJSON string) ([]byte, error) {
		seenARN = functionARN
		seenEvent = eventJSON
		return []byte(eventJSON), nil
	})
	if _, _, err := st.SignUpCognitoUser(account, client.ClientID, "gina", "Secret6!"); err != nil {
		t.Fatal(err)
	}
	if seenARN != fnARN {
		t.Fatalf("arn=%q want %q", seenARN, fnARN)
	}
	if !strings.Contains(seenEvent, "CustomMessage_SignUp") {
		t.Fatalf("event=%s", seenEvent)
	}
	if !strings.Contains(seenEvent, `"codeParameter"`) {
		t.Fatalf("want codeParameter in event: %s", seenEvent)
	}
}

func TestSignUpCustomMessageFailClosed(t *testing.T) {
	st := openCognitoTriggerStore(t)
	account := "000000000001"
	pool, err := st.CreateCognitoUserPool(account, "us-east-1", "custom-fail-pool")
	if err != nil {
		t.Fatal(err)
	}
	client, err := st.CreateCognitoUserPoolClient(account, pool.PoolID, "app")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetCognitoUserPoolTriggers(account, pool.PoolID, "", store.CognitoLambdaConfig{
		CustomMessage: "arn:aws:lambda:us-east-1:" + account + ":function:missing-cm",
	}); err != nil {
		t.Fatal(err)
	}
	st.SetCognitoTriggerInvoker(func(_, _ string) ([]byte, error) {
		return nil, errors.New("invoke failed")
	})
	_, _, err = st.SignUpCognitoUser(account, client.ClientID, "hank", "Secret7!")
	if !errors.Is(err, store.ErrCognitoTriggerFailed) {
		t.Fatalf("err=%v want ErrCognitoTriggerFailed", err)
	}
}

func TestUserMigrationCreatesUserFromResponse(t *testing.T) {
	st := openCognitoTriggerStore(t)
	account := "000000000001"
	pool, err := st.CreateCognitoUserPool(account, "us-east-1", "migrate-pool")
	if err != nil {
		t.Fatal(err)
	}
	client, err := st.CreateCognitoUserPoolClient(account, pool.PoolID, "app")
	if err != nil {
		t.Fatal(err)
	}
	fnARN := "arn:aws:lambda:us-east-1:" + account + ":function:migrate-fn"
	if _, err := st.SetCognitoUserPoolTriggers(account, pool.PoolID, "", store.CognitoLambdaConfig{
		UserMigration: fnARN,
	}); err != nil {
		t.Fatal(err)
	}
	st.SetCognitoTriggerInvoker(func(arn, eventJSON string) ([]byte, error) {
		if arn != fnARN {
			t.Errorf("arn=%q", arn)
		}
		var ev map[string]any
		if err := json.Unmarshal([]byte(eventJSON), &ev); err != nil {
			return nil, err
		}
		if ev["triggerSource"] != "UserMigration_Authentication" {
			t.Errorf("triggerSource=%v", ev["triggerSource"])
		}
		req, _ := ev["request"].(map[string]any)
		if req["password"] != "Migrated1!" {
			t.Errorf("password=%v", req["password"])
		}
		ev["response"] = map[string]any{
			"userAttributes": map[string]any{
				"email":          "mig@example.com",
				"email_verified": "true",
			},
			"finalUserStatus": "CONFIRMED",
		}
		b, err := json.Marshal(ev)
		if err != nil {
			return nil, err
		}
		return b, nil
	})
	out, err := st.InitiateCognitoAuth(client.ClientID, "migrated-user", "Migrated1!")
	if err != nil {
		t.Fatal(err)
	}
	if out.AccessToken == "" || out.IDToken == "" {
		t.Fatalf("missing tokens: %+v", out)
	}
	// Second auth should hit the stored user (no migration required for success).
	out2, err := st.InitiateCognitoAuth(client.ClientID, "migrated-user", "Migrated1!")
	if err != nil {
		t.Fatal(err)
	}
	if out2.AccessToken == "" {
		t.Fatal("second auth missing token")
	}
}

func TestUserMigrationWithoutAttributesUserNotFound(t *testing.T) {
	st := openCognitoTriggerStore(t)
	account := "000000000001"
	pool, err := st.CreateCognitoUserPool(account, "us-east-1", "migrate-miss-pool")
	if err != nil {
		t.Fatal(err)
	}
	client, err := st.CreateCognitoUserPoolClient(account, pool.PoolID, "app")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetCognitoUserPoolTriggers(account, pool.PoolID, "", store.CognitoLambdaConfig{
		UserMigration: "arn:aws:lambda:us-east-1:" + account + ":function:migrate-empty",
	}); err != nil {
		t.Fatal(err)
	}
	st.SetCognitoTriggerInvoker(func(_, eventJSON string) ([]byte, error) {
		return []byte(eventJSON), nil
	})
	_, err = st.InitiateCognitoAuth(client.ClientID, "ghost", "Secret8!")
	if !errors.Is(err, store.ErrCognitoUserNotFound) {
		t.Fatalf("err=%v want ErrCognitoUserNotFound", err)
	}
}
