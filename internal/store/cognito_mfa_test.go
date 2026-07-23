package store_test

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestCognitoSoftwareTokenMFAChallenge(t *testing.T) {
	dir := t.TempDir()
	key, err := store.LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	const acct = "000000000001"
	pool, err := st.CreateCognitoUserPool(acct, "us-east-1", "mfa-pool")
	if err != nil {
		t.Fatal(err)
	}
	client, err := st.CreateCognitoUserPoolClient(acct, pool.PoolID, "mfa-app")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AdminCreateCognitoUser(acct, pool.PoolID, "alice", "Secret1!"); err != nil {
		t.Fatal(err)
	}

	setupAuth, err := st.InitiateCognitoAuth(client.ClientID, "alice", "Secret1!")
	if err != nil {
		t.Fatal(err)
	}
	if setupAuth.ChallengeName != "" {
		t.Fatalf("want tokens before MFA enroll, got challenge %q", setupAuth.ChallengeName)
	}
	if setupAuth.AccessToken == "" {
		t.Fatal("missing access token for MFA enroll")
	}

	secretCode, assocSession, err := st.AssociateSoftwareTokenMFA("", "", "", setupAuth.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if secretCode == "" || len(assocSession) < 20 {
		t.Fatalf("AssociateSoftwareTokenMFA secret=%q session=%q", secretCode, assocSession)
	}

	code, err := store.GenerateCognitoTOTP(secretCode, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.VerifySoftwareTokenMFA("", "", "", assocSession, code); err != nil {
		t.Fatal(err)
	}

	challenged, err := st.InitiateCognitoAuth(client.ClientID, "alice", "Secret1!")
	if err != nil {
		t.Fatal(err)
	}
	if challenged.ChallengeName != "SOFTWARE_TOKEN_MFA" {
		t.Fatalf("ChallengeName=%q want SOFTWARE_TOKEN_MFA", challenged.ChallengeName)
	}
	if challenged.Session == "" {
		t.Fatal("missing SOFTWARE_TOKEN_MFA session")
	}
	if challenged.AccessToken != "" {
		t.Fatal("must not issue tokens before MFA challenge response")
	}

	totp, err := store.GenerateCognitoTOTP(secretCode, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	authed, err := st.RespondToSOFTWARETokenMFAChallenge(client.ClientID, "alice", challenged.Session, totp)
	if err != nil {
		t.Fatal(err)
	}
	if authed.AccessToken == "" || authed.IDToken == "" {
		t.Fatalf("missing tokens after MFA: %+v", authed)
	}

	if _, err := st.RespondToSOFTWARETokenMFAChallenge(client.ClientID, "alice", challenged.Session, totp); err == nil {
		t.Fatal("want fail after session consumed")
	}

	challenged2, err := st.InitiateCognitoAuth(client.ClientID, "alice", "Secret1!")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.RespondToSOFTWARETokenMFAChallenge(client.ClientID, "alice", challenged2.Session, "000000"); err == nil {
		t.Fatal("want CodeMismatch for wrong TOTP")
	} else if !errors.Is(err, store.ErrCognitoCodeMismatch) {
		t.Fatalf("want CodeMismatchException, got %v", err)
	}
}
