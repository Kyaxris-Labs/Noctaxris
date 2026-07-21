package store_test

import (
	"bytes"
	"database/sql"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestKMSSweepExpiredPendingKeys(t *testing.T) {
	st := openKMSStore(t)
	accountID := "000000000001"
	k, err := st.CreateKey(accountID, "arn:aws:iam::"+accountID+":root", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.CreateAlias(accountID, "alias/sweep-me", k.KeyID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateGrant(accountID, k.KeyID, "arn:aws:iam::"+accountID+":user/alice", "", []string{"Encrypt"}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ScheduleKeyDeletion(k.KeyID, 7); err != nil {
		t.Fatal(err)
	}
	past := time.Now().UTC().Add(-time.Hour)
	if err := st.SetPendingDeletionDate(k.KeyID, past); err != nil {
		t.Fatal(err)
	}

	n, err := st.SweepExpiredPendingKeys(time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("swept=%d want 1", n)
	}
	if _, err := st.GetKey(k.KeyID); !errorsIsNoRows(err) {
		t.Fatalf("GetKey after sweep err=%v want no rows", err)
	}
	if _, err := st.ResolveKeyID(accountID, "alias/sweep-me"); !errorsIsNoRows(err) {
		t.Fatalf("alias should be gone: %v", err)
	}
	grants, err := st.ListGrants(k.KeyID)
	if err != nil {
		t.Fatal(err)
	}
	if len(grants) != 0 {
		t.Fatalf("grants=%d want 0", len(grants))
	}
}

func errorsIsNoRows(err error) bool {
	return err != nil && (err == sql.ErrNoRows || bytes.Contains([]byte(err.Error()), []byte("no rows")))
}

func TestKMSGetKeySweepsExpired(t *testing.T) {
	st := openKMSStore(t)
	accountID := "000000000001"
	k, err := st.CreateKey(accountID, "arn:aws:iam::"+accountID+":root", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.ScheduleKeyDeletion(k.KeyID, 7); err != nil {
		t.Fatal(err)
	}
	if err := st.SetPendingDeletionDate(k.KeyID, time.Now().UTC().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetKey(k.KeyID); !errorsIsNoRows(err) {
		t.Fatalf("GetKey should sweep: err=%v", err)
	}
}

func TestKMSRotateKeyMaterialKeepsDecrypt(t *testing.T) {
	st := openKMSStore(t)
	accountID := "000000000001"
	k, err := st.CreateKey(accountID, "arn:aws:iam::"+accountID+":root", "")
	if err != nil {
		t.Fatal(err)
	}
	before, err := st.UnsealKeyMaterial(k.KeyID)
	if err != nil {
		t.Fatal(err)
	}
	blob, err := store.EncryptUnderCMK(before, k.KeyID, []byte("hello-rotate"), nil)
	if err != nil {
		t.Fatal(err)
	}

	if err := st.RotateKeyMaterial(k.KeyID); err != nil {
		t.Fatal(err)
	}
	after, err := st.UnsealKeyMaterial(k.KeyID)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(before, after) {
		t.Fatal("expected new key material after rotation")
	}
	if _, err := store.DecryptUnderCMK(after, blob, nil); err == nil {
		t.Fatal("current material alone should not open pre-rotation ciphertext")
	}
	plain, err := st.DecryptBlobWithKey(k.KeyID, blob)
	if err != nil {
		t.Fatal(err)
	}
	if string(plain) != "hello-rotate" {
		t.Fatalf("plain=%q", plain)
	}
}

func TestKMSEnableRotationRotatesMaterial(t *testing.T) {
	st := openKMSStore(t)
	accountID := "000000000001"
	k, err := st.CreateKey(accountID, "arn:aws:iam::"+accountID+":root", "")
	if err != nil {
		t.Fatal(err)
	}
	before, err := st.UnsealKeyMaterial(k.KeyID)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetKeyRotationEnabled(k.KeyID, true); err != nil {
		t.Fatal(err)
	}
	after, err := st.UnsealKeyMaterial(k.KeyID)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(before, after) {
		t.Fatal("EnableKeyRotation should rotate material")
	}
	enabled, err := st.GetKeyRotationEnabled(k.KeyID)
	if err != nil || !enabled {
		t.Fatalf("enabled=%v err=%v", enabled, err)
	}
}

func TestKMSMaybeAutoRotate(t *testing.T) {
	st := openKMSStore(t)
	accountID := "000000000001"
	k, err := st.CreateKey(accountID, "arn:aws:iam::"+accountID+":root", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetKeyRotationEnabled(k.KeyID, true); err != nil {
		t.Fatal(err)
	}
	mid, err := st.UnsealKeyMaterial(k.KeyID)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetRotationPeriodDays(k.KeyID, 1); err != nil {
		t.Fatal(err)
	}
	if err := st.SetLastRotationDate(k.KeyID, time.Now().UTC().Add(-48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := st.MaybeAutoRotate(k.KeyID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	after, err := st.UnsealKeyMaterial(k.KeyID)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(mid, after) {
		t.Fatal("MaybeAutoRotate should rotate when period elapsed")
	}
}
