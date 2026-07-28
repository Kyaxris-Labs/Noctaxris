package store_test

import (
	"database/sql"
	"sync"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestAccessKeyCacheWarmSecondLookupUnsealDeltaOne(t *testing.T) {
	st := openTestStore(t)
	const accountID = "000000000001"
	if _, _, err := st.CreateUser(accountID, "cache-user"); err != nil {
		t.Fatal(err)
	}
	keyID, secret, err := st.CreateUserAccessKey(accountID, "cache-user")
	if err != nil {
		t.Fatal(err)
	}

	before := st.AccessKeyUnsealCount()
	ak1, err := st.LookupAccessKeyRecord(keyID)
	if err != nil {
		t.Fatal(err)
	}
	afterFirst := st.AccessKeyUnsealCount()
	if afterFirst-before != 1 {
		t.Fatalf("first Lookup Unseal delta=%d want 1 (before=%d after=%d)", afterFirst-before, before, afterFirst)
	}
	if ak1.Secret != secret {
		t.Fatal("secret mismatch on first Lookup")
	}

	ak2, err := st.LookupAccessKeyRecord(keyID)
	if err != nil {
		t.Fatal(err)
	}
	afterSecond := st.AccessKeyUnsealCount()
	if afterSecond-before != 1 {
		t.Fatalf("warm second Lookup Unseal delta=%d want 1 (before=%d after=%d)", afterSecond-before, before, afterSecond)
	}
	if ak2.Secret != secret || ak2.Status != store.AccessKeyStatusActive {
		t.Fatalf("cached ak=%+v", ak2)
	}
}

func TestAccessKeyCacheDeleteMiss(t *testing.T) {
	st := openTestStore(t)
	const accountID = "000000000001"
	if _, _, err := st.CreateUser(accountID, "del-user"); err != nil {
		t.Fatal(err)
	}
	keyID, _, err := st.CreateUserAccessKey(accountID, "del-user")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.LookupAccessKeyRecord(keyID); err != nil {
		t.Fatal(err)
	}
	before := st.AccessKeyUnsealCount()
	if err := st.DeleteAccessKey(keyID); err != nil {
		t.Fatal(err)
	}
	_, err = st.LookupAccessKeyRecord(keyID)
	if err != sql.ErrNoRows {
		t.Fatalf("err=%v want sql.ErrNoRows", err)
	}
	if st.AccessKeyUnsealCount() != before {
		t.Fatalf("deleted key Lookup must not Unseal; count %d -> %d", before, st.AccessKeyUnsealCount())
	}
}

func TestAccessKeyCacheDeleteInAccountMiss(t *testing.T) {
	st := openTestStore(t)
	const accountID = "000000000002"
	if err := st.EnsureRoot(accountID, "AKIAROOTACCT00002", "secret-acct-2"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.CreateUser(accountID, "del-acct"); err != nil {
		t.Fatal(err)
	}
	keyID, _, err := st.CreateUserAccessKey(accountID, "del-acct")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.LookupAccessKeyRecord(keyID); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteAccessKeyInAccount(accountID, keyID); err != nil {
		t.Fatal(err)
	}
	_, err = st.LookupAccessKeyRecord(keyID)
	if err != sql.ErrNoRows {
		t.Fatalf("err=%v want sql.ErrNoRows", err)
	}
}

func TestAccessKeyCacheInactiveThenReactivate(t *testing.T) {
	st := openTestStore(t)
	const accountID = "000000000001"
	if _, _, err := st.CreateUser(accountID, "status-user"); err != nil {
		t.Fatal(err)
	}
	keyID, _, err := st.CreateUserAccessKey(accountID, "status-user")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.LookupAccessKeyRecord(keyID); err != nil {
		t.Fatal(err)
	}

	if err := st.UpdateAccessKey(keyID, store.AccessKeyStatusInactive); err != nil {
		t.Fatal(err)
	}
	ak, err := st.LookupAccessKeyRecord(keyID)
	if err != nil {
		t.Fatal(err)
	}
	if ak.Status != store.AccessKeyStatusInactive {
		t.Fatalf("status after Inactive=%q", ak.Status)
	}

	if err := st.UpdateAccessKey(keyID, store.AccessKeyStatusActive); err != nil {
		t.Fatal(err)
	}
	ak, err = st.LookupAccessKeyRecord(keyID)
	if err != nil {
		t.Fatal(err)
	}
	if ak.Status != store.AccessKeyStatusActive {
		t.Fatalf("status after Reactivate=%q", ak.Status)
	}
}

func TestAccessKeyCacheFailedDeleteDoesNotClearUnrelated(t *testing.T) {
	st := openTestStore(t)
	const accountID = "000000000001"
	if _, _, err := st.CreateUser(accountID, "keep-user"); err != nil {
		t.Fatal(err)
	}
	keyID, secret, err := st.CreateUserAccessKey(accountID, "keep-user")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.LookupAccessKeyRecord(keyID); err != nil {
		t.Fatal(err)
	}
	before := st.AccessKeyUnsealCount()

	err = st.DeleteAccessKey("AKIADOESNOTEXIST00")
	if err != sql.ErrNoRows {
		t.Fatalf("failed delete err=%v want sql.ErrNoRows", err)
	}

	ak, err := st.LookupAccessKeyRecord(keyID)
	if err != nil {
		t.Fatal(err)
	}
	if st.AccessKeyUnsealCount() != before {
		t.Fatalf("unrelated warm key must stay cached; Unseal count %d -> %d", before, st.AccessKeyUnsealCount())
	}
	if ak.Secret != secret {
		t.Fatal("secret mismatch after failed delete")
	}
}

func TestAccessKeyCacheRaceDeleteLookup(t *testing.T) {
	st := openTestStore(t)
	const accountID = "000000000001"
	if _, _, err := st.CreateUser(accountID, "race-user"); err != nil {
		t.Fatal(err)
	}
	keyID, _, err := st.CreateUserAccessKey(accountID, "race-user")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.LookupAccessKeyRecord(keyID); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	const n = 32
	wg.Add(n + 1)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			<-start
			_, _ = st.LookupAccessKeyRecord(keyID)
		}()
	}
	go func() {
		defer wg.Done()
		<-start
		if err := st.DeleteAccessKey(keyID); err != nil {
			t.Errorf("DeleteAccessKey: %v", err)
		}
	}()
	close(start)
	wg.Wait()

	_, err = st.LookupAccessKeyRecord(keyID)
	if err != sql.ErrNoRows {
		t.Fatalf("after Delete returns, Lookup err=%v want sql.ErrNoRows", err)
	}
}

func TestAccessKeyCacheExpiredTempStillLoadable(t *testing.T) {
	st := openTestStore(t)
	const accountID = "000000000001"
	expires := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)
	keyID, err := st.MintTempCredentialsOpts(store.MintTempOpts{
		AccountID:    accountID,
		UserName:     "expired-user",
		Secret:       "temp-secret",
		SessionToken: "temp-token",
		Expires:      expires,
	})
	if err != nil {
		t.Fatal(err)
	}

	before := st.AccessKeyUnsealCount()
	ak, err := st.LookupAccessKeyRecord(keyID)
	if err != nil {
		t.Fatal(err)
	}
	if after := st.AccessKeyUnsealCount(); after-before != 1 {
		t.Fatalf("first expired Lookup Unseal delta=%d want 1", after-before)
	}
	if ak.SessionToken != "temp-token" || ak.Secret != "temp-secret" {
		t.Fatalf("expired temp ak=%+v", ak)
	}
	if ak.ExpiresAt.IsZero() || !ak.ExpiresAt.Equal(expires) {
		t.Fatalf("ExpiresAt=%v want %v", ak.ExpiresAt, expires)
	}

	ak2, err := st.LookupAccessKeyRecord(keyID)
	if err != nil {
		t.Fatal(err)
	}
	if after := st.AccessKeyUnsealCount(); after-before != 1 {
		t.Fatalf("warm expired Lookup Unseal delta=%d want 1", after-before)
	}
	if !ak2.ExpiresAt.Equal(expires) {
		t.Fatalf("cached ExpiresAt=%v", ak2.ExpiresAt)
	}
}
