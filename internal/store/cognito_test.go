package store_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/jwtutil"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestCognitoPoolClientJWKSAndAuth(t *testing.T) {
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
	pool, err := st.CreateCognitoUserPool(acct, "us-east-1", "lab-pool")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(pool.PoolID, "us-east-1_") {
		t.Fatalf("pool id = %q", pool.PoolID)
	}
	iss := store.CognitoIssuerURL(pool.Region, pool.PoolID)
	if !strings.Contains(iss, pool.PoolID) {
		t.Fatalf("issuer = %q", iss)
	}

	jwks, err := st.CognitoJWKSJSON(pool.PoolID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(jwks), `"kty":"RSA"`) && !strings.Contains(string(jwks), `"kty": "RSA"`) {
		t.Fatalf("jwks missing RSA: %s", jwks)
	}

	client, err := st.CreateCognitoUserPoolClient(acct, pool.PoolID, "app")
	if err != nil {
		t.Fatal(err)
	}
	user, err := st.AdminCreateCognitoUser(acct, pool.PoolID, "alice", "Secret1!")
	if err != nil {
		t.Fatal(err)
	}
	if user.UserStatus != "CONFIRMED" {
		t.Fatalf("status=%q", user.UserStatus)
	}

	auth, err := st.AdminInitiateCognitoAuth(acct, pool.PoolID, client.ClientID, "alice", "Secret1!")
	if err != nil {
		t.Fatal(err)
	}
	if auth.AccessToken == "" || auth.IDToken == "" || auth.RefreshToken == "" {
		t.Fatalf("missing tokens: %+v", auth)
	}

	idClaims, err := jwtutil.VerifyCompactRS256(auth.IDToken, jwks)
	if err != nil {
		t.Fatal(err)
	}
	if jwtutil.ClaimString(idClaims, "token_use") != "id" {
		t.Fatalf("id token_use=%v", idClaims["token_use"])
	}
	if jwtutil.ClaimString(idClaims, "aud") != client.ClientID {
		t.Fatalf("aud=%v", idClaims["aud"])
	}
	if jwtutil.ClaimString(idClaims, "iss") != iss {
		t.Fatalf("iss=%v want %s", idClaims["iss"], iss)
	}
	if jwtutil.ClaimExpired(idClaims, time.Now()) {
		t.Fatal("id token expired")
	}

	accessClaims, err := jwtutil.VerifyCompactRS256(auth.AccessToken, jwks)
	if err != nil {
		t.Fatal(err)
	}
	if jwtutil.ClaimString(accessClaims, "token_use") != "access" {
		t.Fatalf("access token_use=%v", accessClaims["token_use"])
	}
	if jwtutil.ClaimString(accessClaims, "client_id") != client.ClientID {
		t.Fatalf("client_id=%v", accessClaims["client_id"])
	}

	if _, err := st.AdminInitiateCognitoAuth(acct, pool.PoolID, client.ClientID, "alice", "wrong"); err == nil {
		t.Fatal("expected bad password reject")
	}

	viaClient, err := st.InitiateCognitoAuth(client.ClientID, "alice", "Secret1!")
	if err != nil {
		t.Fatal(err)
	}
	if viaClient.IDToken == "" {
		t.Fatal("InitiateAuth missing IdToken")
	}

	listed, err := st.ListCognitoUserPools(acct)
	if err != nil || len(listed) != 1 {
		t.Fatalf("list pools: %v len=%d", err, len(listed))
	}
	clients, err := st.ListCognitoUserPoolClients(acct, pool.PoolID)
	if err != nil || len(clients) != 1 {
		t.Fatalf("list clients: %v len=%d", err, len(clients))
	}
	if err := st.DeleteCognitoUserPoolClient(acct, pool.PoolID, client.ClientID); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteCognitoUserPool(acct, pool.PoolID); err != nil {
		t.Fatal(err)
	}
}

func TestCognitoSignUpConfirm(t *testing.T) {
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
	pool, err := st.CreateCognitoUserPool(acct, "us-east-1", "signup-pool")
	if err != nil {
		t.Fatal(err)
	}
	client, err := st.CreateCognitoUserPoolClient(acct, pool.PoolID, "web")
	if err != nil {
		t.Fatal(err)
	}
	user, _, err := st.SignUpCognitoUser(acct, client.ClientID, "bob", "Secret2!")
	if err != nil {
		t.Fatal(err)
	}
	if user.UserStatus != "UNCONFIRMED" {
		t.Fatalf("status=%q", user.UserStatus)
	}
	if _, err := st.InitiateCognitoAuth(client.ClientID, "bob", "Secret2!"); err == nil {
		t.Fatal("unconfirmed user must not auth")
	}
	if err := st.ConfirmSignUpCognitoUser(client.ClientID, "bob", "123456"); err != nil {
		t.Fatal(err)
	}
	auth, err := st.InitiateCognitoAuth(client.ClientID, "bob", "Secret2!")
	if err != nil {
		t.Fatal(err)
	}
	if auth.AccessToken == "" {
		t.Fatal("missing access token after confirm")
	}
}
