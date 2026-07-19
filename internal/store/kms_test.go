package store_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func openKMSStore(t *testing.T) *store.Store {
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
	t.Cleanup(func() {
		_ = st.Close()
	})
	return st
}

func TestKMSCreateKey(t *testing.T) {
	st := openKMSStore(t)
	accountID := "000000000001"
	creator := "arn:aws:iam::" + accountID + ":user/alice"

	k, err := st.CreateKey(accountID, creator, "")
	if err != nil {
		t.Fatal(err)
	}
	if k.KeyID == "" || k.ARN == "" {
		t.Fatalf("empty key: %+v", k)
	}
	if k.KeyState != store.KeyStateEnabled {
		t.Fatalf("state=%q", k.KeyState)
	}
	if !bytes.Contains([]byte(k.KeyPolicy), []byte("kms:*")) {
		t.Fatalf("default policy missing kms:*: %s", k.KeyPolicy)
	}
	if !bytes.Contains([]byte(k.KeyPolicy), []byte(creator)) {
		t.Fatalf("default policy missing creator: %s", k.KeyPolicy)
	}

	material, err := st.UnsealKeyMaterial(k.KeyID)
	if err != nil {
		t.Fatal(err)
	}
	if len(material) != 32 {
		t.Fatalf("material len=%d want 32", len(material))
	}

	got, err := st.GetKey(k.KeyID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ARN != k.ARN {
		t.Fatalf("GetKey ARN=%q want %q", got.ARN, k.ARN)
	}
}

func TestKMSCreateKeyMaterialNotPlaintextInDB(t *testing.T) {
	dir := t.TempDir()
	key, err := store.LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	k, err := st.CreateKey("000000000001", "arn:aws:iam::000000000001:root", "")
	if err != nil {
		t.Fatal(err)
	}
	material, err := st.UnsealKeyMaterial(k.KeyID)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, material) {
		t.Fatal("plaintext CMK material found in db file")
	}
}

func TestKMSAliasResolve(t *testing.T) {
	st := openKMSStore(t)
	accountID := "000000000001"
	k, err := st.CreateKey(accountID, "arn:aws:iam::"+accountID+":root", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.CreateAlias(accountID, "alias/lab", k.KeyID); err != nil {
		t.Fatal(err)
	}
	got, err := st.ResolveKeyID(accountID, "alias/lab")
	if err != nil {
		t.Fatal(err)
	}
	if got != k.KeyID {
		t.Fatalf("resolve alias got %q want %q", got, k.KeyID)
	}
	got, err = st.ResolveKeyID(accountID, k.ARN)
	if err != nil {
		t.Fatal(err)
	}
	if got != k.KeyID {
		t.Fatalf("resolve ARN got %q want %q", got, k.KeyID)
	}
}

func TestKMSGrantList(t *testing.T) {
	st := openKMSStore(t)
	accountID := "000000000001"
	k, err := st.CreateKey(accountID, "arn:aws:iam::"+accountID+":root", "")
	if err != nil {
		t.Fatal(err)
	}
	grantee := "arn:aws:iam::" + accountID + ":user/alice"
	g, err := st.CreateGrant(accountID, k.KeyID, grantee, "", []string{"Encrypt", "Decrypt"}, "lab")
	if err != nil {
		t.Fatal(err)
	}
	if g.GrantID == "" {
		t.Fatal("empty grant id")
	}
	list, err := st.ListGrants(k.KeyID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].GrantID != g.GrantID {
		t.Fatalf("list=%+v", list)
	}
	ok, err := st.FindMatchingGrant(k.KeyID, grantee, "kms:Encrypt")
	if err != nil || !ok {
		t.Fatalf("FindMatchingGrant ok=%v err=%v", ok, err)
	}
}
