package store_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestCognitoUSER_SRP_AUTHRoundTrip(t *testing.T) {
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
	pool, err := st.CreateCognitoUserPool(acct, "us-east-1", "srp-pool")
	if err != nil {
		t.Fatal(err)
	}
	client, err := st.CreateCognitoUserPoolClient(acct, pool.PoolID, "srp-app")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AdminCreateCognitoUser(acct, pool.PoolID, "srp-user", "Secret1!"); err != nil {
		t.Fatal(err)
	}

	srpClient, err := store.NewCognitoSRPClient(pool.PoolID, "srp-user", "Secret1!")
	if err != nil {
		t.Fatal(err)
	}
	chal, err := st.InitiateCognitoSRPAuth(client.ClientID, "srp-user", srpClient.SRPAHex())
	if err != nil {
		t.Fatal(err)
	}
	if chal.ChallengeName != "PASSWORD_VERIFIER" {
		t.Fatalf("ChallengeName=%q", chal.ChallengeName)
	}
	if chal.Session == "" || chal.ChallengeParameters["SRP_B"] == "" || chal.ChallengeParameters["SALT"] == "" {
		t.Fatalf("incomplete challenge: %+v", chal)
	}

	responses, err := srpClient.PasswordVerifierChallengeResponses(chal.ChallengeParameters, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	out, err := st.RespondToCognitoPASSWORDVerifierChallenge(client.ClientID, chal.Session, responses)
	if err != nil {
		t.Fatal(err)
	}
	if out.AccessToken == "" || out.IDToken == "" || out.RefreshToken == "" {
		t.Fatalf("missing tokens: %+v", out)
	}

	badClient, err := store.NewCognitoSRPClient(pool.PoolID, "srp-user", "WrongPass!")
	if err != nil {
		t.Fatal(err)
	}
	badChal, err := st.InitiateCognitoSRPAuth(client.ClientID, "srp-user", badClient.SRPAHex())
	if err != nil {
		t.Fatal(err)
	}
	badResp, err := badClient.PasswordVerifierChallengeResponses(badChal.ChallengeParameters, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.RespondToCognitoPASSWORDVerifierChallenge(client.ClientID, badChal.Session, badResp); err == nil {
		t.Fatal("expected wrong password reject")
	}
}

func TestCognitoUSER_SRP_AUTHThenMFA(t *testing.T) {
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
	pool, err := st.CreateCognitoUserPool(acct, "us-east-1", "srp-mfa-pool")
	if err != nil {
		t.Fatal(err)
	}
	client, err := st.CreateCognitoUserPoolClient(acct, pool.PoolID, "srp-mfa-app")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AdminCreateCognitoUser(acct, pool.PoolID, "mfa-user", "Secret1!"); err != nil {
		t.Fatal(err)
	}
	pwAuth, err := st.InitiateCognitoAuth(client.ClientID, "mfa-user", "Secret1!")
	if err != nil {
		t.Fatal(err)
	}
	secret, assocSession, err := st.AssociateSoftwareTokenMFA(acct, pool.PoolID, "mfa-user", pwAuth.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	code, err := store.GenerateCognitoTOTP(secret, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.VerifySoftwareTokenMFA(acct, pool.PoolID, "mfa-user", assocSession, code); err != nil {
		t.Fatal(err)
	}

	srpClient, err := store.NewCognitoSRPClient(pool.PoolID, "mfa-user", "Secret1!")
	if err != nil {
		t.Fatal(err)
	}
	chal, err := st.InitiateCognitoSRPAuth(client.ClientID, "mfa-user", srpClient.SRPAHex())
	if err != nil {
		t.Fatal(err)
	}
	responses, err := srpClient.PasswordVerifierChallengeResponses(chal.ChallengeParameters, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	out, err := st.RespondToCognitoPASSWORDVerifierChallenge(client.ClientID, chal.Session, responses)
	if err != nil {
		t.Fatal(err)
	}
	if out.ChallengeName != "SOFTWARE_TOKEN_MFA" || out.Session == "" {
		t.Fatalf("want SOFTWARE_TOKEN_MFA after SRP, got %+v", out)
	}
}
