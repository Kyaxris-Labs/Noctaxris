package server_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func accessKeyCacheGCI(t *testing.T, handler http.Handler, akid, secret, sessionToken string, when time.Time) *httptest.ResponseRecorder {
	t.Helper()
	body := []byte("Action=GetCallerIdentity&Version=2011-06-15")
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", body)
	signHeader(t, req, body, akid, secret, testRegion, testSvc, when)
	if sessionToken != "" {
		req.Header.Set("X-Amz-Security-Token", sessionToken)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestAccessKeyCacheGateDeleteThenSigV4Reject(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	if _, _, err := st.CreateUser(testAccountID, "gate-del"); err != nil {
		t.Fatal(err)
	}
	keyID, secret, err := st.CreateUserAccessKey(testAccountID, "gate-del")
	if err != nil {
		t.Fatal(err)
	}

	rec := accessKeyCacheGCI(t, handler, keyID, secret, "", now)
	if rec.Code != http.StatusOK {
		t.Fatalf("warm SigV4: %d %s", rec.Code, rec.Body.String())
	}

	if err := st.DeleteAccessKey(keyID); err != nil {
		t.Fatal(err)
	}
	rec = accessKeyCacheGCI(t, handler, keyID, secret, "", now)
	if rec.Code == http.StatusOK {
		t.Fatalf("after DeleteAccessKey SigV4 must reject, got OK: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "InvalidClientTokenId") {
		t.Fatalf("expected InvalidClientTokenId, got: %s", rec.Body.String())
	}
}

func TestAccessKeyCacheGateInactiveThenReactivate(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	if _, _, err := st.CreateUser(testAccountID, "gate-status"); err != nil {
		t.Fatal(err)
	}
	keyID, secret, err := st.CreateUserAccessKey(testAccountID, "gate-status")
	if err != nil {
		t.Fatal(err)
	}
	rec := accessKeyCacheGCI(t, handler, keyID, secret, "", now)
	if rec.Code != http.StatusOK {
		t.Fatalf("warm: %d %s", rec.Code, rec.Body.String())
	}

	if err := st.UpdateAccessKey(keyID, store.AccessKeyStatusInactive); err != nil {
		t.Fatal(err)
	}
	rec = accessKeyCacheGCI(t, handler, keyID, secret, "", now)
	if rec.Code == http.StatusOK {
		t.Fatalf("Inactive must reject, got OK: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "InvalidClientTokenId") {
		t.Fatalf("expected InvalidClientTokenId for Inactive: %s", rec.Body.String())
	}

	if err := st.UpdateAccessKey(keyID, store.AccessKeyStatusActive); err != nil {
		t.Fatal(err)
	}
	rec = accessKeyCacheGCI(t, handler, keyID, secret, "", now)
	if rec.Code != http.StatusOK {
		t.Fatalf("Reactivate must allow: %d %s", rec.Code, rec.Body.String())
	}
}

func TestAccessKeyCacheGateUpdateInAccountInactiveReject(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	if _, _, err := st.CreateUser(testAccountID, "gate-upd-acct"); err != nil {
		t.Fatal(err)
	}
	keyID, secret, err := st.CreateUserAccessKey(testAccountID, "gate-upd-acct")
	if err != nil {
		t.Fatal(err)
	}
	rec := accessKeyCacheGCI(t, handler, keyID, secret, "", now)
	if rec.Code != http.StatusOK {
		t.Fatalf("warm: %d %s", rec.Code, rec.Body.String())
	}

	if err := st.UpdateAccessKeyInAccount(testAccountID, keyID, store.AccessKeyStatusInactive); err != nil {
		t.Fatal(err)
	}
	rec = accessKeyCacheGCI(t, handler, keyID, secret, "", now)
	if rec.Code == http.StatusOK {
		t.Fatalf("UpdateAccessKeyInAccount Inactive must reject, got OK: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "InvalidClientTokenId") {
		t.Fatalf("expected InvalidClientTokenId: %s", rec.Body.String())
	}
}

func TestAccessKeyCacheGateExpiredTempReject(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)
	expires := now.Add(-time.Hour)

	keyID, err := st.MintTempCredentialsOpts(store.MintTempOpts{
		AccountID:    testAccountID,
		UserName:     "gate-expired",
		Secret:       "temp-secret",
		SessionToken: "temp-token",
		Expires:      expires,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Warm store cache; expired rows remain loadable at store layer.
	if _, err := st.LookupAccessKeyRecord(keyID); err != nil {
		t.Fatal(err)
	}

	rec := accessKeyCacheGCI(t, handler, keyID, "temp-secret", "temp-token", now)
	if rec.Code == http.StatusOK {
		t.Fatalf("expired temp must reject, got OK: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "InvalidClientTokenId") {
		t.Fatalf("expected InvalidClientTokenId for expired: %s", rec.Body.String())
	}
}

func TestAccessKeyCacheGateWrongSessionTokenReject(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	keyID, err := st.MintTempCredentialsOpts(store.MintTempOpts{
		AccountID:    testAccountID,
		UserName:     "gate-token",
		Secret:       "temp-secret",
		SessionToken: "correct-token",
		Expires:      now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	rec := accessKeyCacheGCI(t, handler, keyID, "temp-secret", "correct-token", now)
	if rec.Code != http.StatusOK {
		t.Fatalf("warm with correct token: %d %s", rec.Code, rec.Body.String())
	}

	rec = accessKeyCacheGCI(t, handler, keyID, "temp-secret", "wrong-token", now)
	if rec.Code == http.StatusOK {
		t.Fatalf("wrong session token must reject, got OK: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "InvalidClientTokenId") {
		t.Fatalf("expected InvalidClientTokenId for wrong token: %s", rec.Body.String())
	}
}
