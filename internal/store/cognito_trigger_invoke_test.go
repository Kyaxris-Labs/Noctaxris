package store_test

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

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
	st.SetCognitoTriggerInvoker(func(functionARN, eventJSON string) error {
		mu.Lock()
		defer mu.Unlock()
		seenARN = functionARN
		seenEvent = eventJSON
		return nil
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
	st.SetCognitoTriggerInvoker(func(_, _ string) error {
		return errors.New("boom")
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
	st.SetCognitoTriggerInvoker(func(functionARN, eventJSON string) error {
		if functionARN != fnARN {
			t.Errorf("arn=%q", functionARN)
		}
		var evt map[string]any
		_ = json.Unmarshal([]byte(eventJSON), &evt)
		sources = append(sources, evt["triggerSource"].(string))
		return nil
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
