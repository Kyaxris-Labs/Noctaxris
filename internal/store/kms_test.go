package store_test

import (
	"bytes"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

	other := "000000000002"
	aliasARN := "arn:aws:kms:us-east-1:" + accountID + ":alias/lab"
	got, err = st.ResolveKeyID(other, aliasARN)
	if err != nil {
		t.Fatal(err)
	}
	if got != k.KeyID {
		t.Fatalf("resolve alias ARN with caller account %s got %q want %q", other, got, k.KeyID)
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

func TestKMSScheduleAndCancelKeyDeletion(t *testing.T) {
	st := openKMSStore(t)
	accountID := "000000000001"
	k, err := st.CreateKey(accountID, "arn:aws:iam::"+accountID+":root", "")
	if err != nil {
		t.Fatal(err)
	}

	scheduled, err := st.ScheduleKeyDeletion(k.KeyID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if scheduled.KeyState != store.KeyStatePendingDeletion {
		t.Fatalf("state=%q want PendingDeletion", scheduled.KeyState)
	}
	if scheduled.DeletionDate == "" {
		t.Fatal("expected DeletionDate")
	}
	if scheduled.PendingWindowInDays != 30 {
		t.Fatalf("PendingWindowInDays=%d want 30 default", scheduled.PendingWindowInDays)
	}

	got, err := st.GetKey(k.KeyID)
	if err != nil {
		t.Fatal(err)
	}
	if got.KeyState != store.KeyStatePendingDeletion || got.DeletionDate != scheduled.DeletionDate {
		t.Fatalf("GetKey after schedule: %+v", got)
	}

	if _, err := st.ScheduleKeyDeletion(k.KeyID, 7); err == nil {
		t.Fatal("expected error scheduling deletion twice")
	}

	if err := st.CancelKeyDeletion(k.KeyID); err != nil {
		t.Fatal(err)
	}
	got, err = st.GetKey(k.KeyID)
	if err != nil {
		t.Fatal(err)
	}
	if got.KeyState != store.KeyStateDisabled {
		t.Fatalf("after cancel state=%q want Disabled", got.KeyState)
	}
	if got.DeletionDate != "" {
		t.Fatalf("after cancel DeletionDate=%q want empty", got.DeletionDate)
	}

	if err := st.CancelKeyDeletion(k.KeyID); err == nil {
		t.Fatal("expected error canceling when not pending deletion")
	}
}

func TestKMSScheduleKeyDeletionClampsWindow(t *testing.T) {
	st := openKMSStore(t)
	accountID := "000000000001"
	k, err := st.CreateKey(accountID, "arn:aws:iam::"+accountID+":root", "")
	if err != nil {
		t.Fatal(err)
	}

	low, err := st.ScheduleKeyDeletion(k.KeyID, 3)
	if err != nil {
		t.Fatal(err)
	}
	if low.PendingWindowInDays != 7 {
		t.Fatalf("low clamp=%d want 7", low.PendingWindowInDays)
	}
	if err := st.CancelKeyDeletion(k.KeyID); err != nil {
		t.Fatal(err)
	}

	k2, err := st.CreateKey(accountID, "arn:aws:iam::"+accountID+":root", "")
	if err != nil {
		t.Fatal(err)
	}
	high, err := st.ScheduleKeyDeletion(k2.KeyID, 90)
	if err != nil {
		t.Fatal(err)
	}
	if high.PendingWindowInDays != 30 {
		t.Fatalf("high clamp=%d want 30", high.PendingWindowInDays)
	}
}

func TestKMSKeyRotationFlag(t *testing.T) {
	st := openKMSStore(t)
	accountID := "000000000001"
	k, err := st.CreateKey(accountID, "arn:aws:iam::"+accountID+":root", "")
	if err != nil {
		t.Fatal(err)
	}
	enabled, err := st.GetKeyRotationEnabled(k.KeyID)
	if err != nil {
		t.Fatal(err)
	}
	if enabled {
		t.Fatal("rotation should default to false")
	}

	if err := st.SetKeyRotationEnabled(k.KeyID, true); err != nil {
		t.Fatal(err)
	}
	enabled, err = st.GetKeyRotationEnabled(k.KeyID)
	if err != nil {
		t.Fatal(err)
	}
	if !enabled {
		t.Fatal("rotation should be enabled")
	}
	got, err := st.GetKey(k.KeyID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.KeyRotationEnabled {
		t.Fatal("GetKey KeyRotationEnabled=false")
	}

	if err := st.SetKeyRotationEnabled(k.KeyID, false); err != nil {
		t.Fatal(err)
	}
	enabled, err = st.GetKeyRotationEnabled(k.KeyID)
	if err != nil {
		t.Fatal(err)
	}
	if enabled {
		t.Fatal("rotation should be disabled")
	}
}

func TestKMSAWSManagedConvenienceAliases(t *testing.T) {
	st := openKMSStore(t)
	accountID := "000000000001"

	s3ID, err := st.ResolveKeyID(accountID, store.AliasAWSS3)
	if err != nil {
		t.Fatalf("resolve alias/aws/s3: %v", err)
	}
	ddbID, err := st.ResolveKeyID(accountID, store.AliasAWSDynamoDB)
	if err != nil {
		t.Fatalf("resolve alias/aws/dynamodb: %v", err)
	}
	sqsID, err := st.ResolveKeyID(accountID, store.AliasAWSSQS)
	if err != nil {
		t.Fatalf("resolve alias/aws/sqs: %v", err)
	}
	if s3ID != ddbID || ddbID != sqsID {
		t.Fatalf("expected shared lab CMK, got s3=%q ddb=%q sqs=%q", s3ID, ddbID, sqsID)
	}

	again, err := st.EnsureAWSManagedConvenienceAliases(accountID)
	if err != nil {
		t.Fatal(err)
	}
	if again != s3ID {
		t.Fatalf("idempotent ensure got %q want %q", again, s3ID)
	}

	k, err := st.GetKey(s3ID)
	if err != nil {
		t.Fatal(err)
	}
	if k.AccountID != accountID || k.KeyState != store.KeyStateEnabled {
		t.Fatalf("managed key unexpected: %+v", k)
	}
}

func TestKMSCoverageWave2ListPolicyAliasGrants(t *testing.T) {
	st := openKMSStore(t)
	account := "000000000001"
	creator := "arn:aws:iam::" + account + ":root"

	empty, err := st.ListKeys(account)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty keys=%v err=%v", empty, err)
	}
	emptyAliases, err := st.ListAliases(account)
	if err != nil || len(emptyAliases) != 0 {
		t.Fatalf("empty aliases=%v err=%v", emptyAliases, err)
	}

	arn := store.AliasARN("", account, "alias/wave2")
	if !strings.Contains(arn, ":alias/wave2") || !strings.Contains(arn, "us-east-1") {
		t.Fatalf("AliasARN=%q", arn)
	}
	arn2 := store.AliasARN("eu-west-1", account, "wave2-bare")
	if !strings.Contains(arn2, "eu-west-1") || !strings.Contains(arn2, ":alias/wave2-bare") {
		t.Fatalf("AliasARN bare=%q", arn2)
	}

	k, err := st.CreateKey(account, creator, "")
	if err != nil {
		t.Fatal(err)
	}
	k2, err := st.CreateKey(account, creator, "")
	if err != nil {
		t.Fatal(err)
	}
	keys, err := st.ListKeys(account)
	if err != nil || len(keys) < 2 {
		t.Fatalf("keys=%v err=%v", keys, err)
	}

	pol, err := st.GetKeyPolicy(k.KeyID)
	if err != nil || !strings.Contains(pol, "Statement") {
		t.Fatalf("policy=%q err=%v", pol, err)
	}
	if _, err := st.GetKeyPolicy("missing-key-id"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing policy err=%v", err)
	}

	if err := st.SetKeyState(k.KeyID, "Bogus"); err == nil {
		t.Fatal("invalid state must fail")
	}
	if err := st.SetKeyState(k.KeyID, store.KeyStateDisabled); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetKey(k.KeyID)
	if err != nil || got.KeyState != store.KeyStateDisabled {
		t.Fatalf("disabled=%+v err=%v", got, err)
	}
	if err := st.SetKeyState(k.KeyID, store.KeyStateEnabled); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ScheduleKeyDeletion(k.KeyID, 7); err != nil {
		t.Fatal(err)
	}
	if err := st.SetKeyState(k.KeyID, store.KeyStateDisabled); err == nil {
		t.Fatal("pending deletion SetKeyState must fail")
	}
	if err := st.CancelKeyDeletion(k.KeyID); err != nil {
		t.Fatal(err)
	}

	if err := st.CreateAlias(account, "alias/wave2-a", k.KeyID); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateAlias(account, "alias/wave2-b", k2.KeyID); err != nil {
		t.Fatal(err)
	}
	aliases, err := st.ListAliases(account)
	if err != nil || len(aliases) < 2 {
		t.Fatalf("aliases=%v err=%v", aliases, err)
	}
	if err := st.UpdateAlias(account, "alias/wave2-a", "missing"); err == nil {
		t.Fatal("update to missing key must fail")
	}
	if err := st.UpdateAlias(account, "alias/missing", k2.KeyID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("update missing alias err=%v", err)
	}
	if err := st.UpdateAlias(account, "alias/wave2-a", k2.KeyID); err != nil {
		t.Fatal(err)
	}
	resolved, err := st.ResolveKeyID(account, "alias/wave2-a")
	if err != nil || resolved != k2.KeyID {
		t.Fatalf("resolved=%q err=%v", resolved, err)
	}

	grantee := "arn:aws:iam::" + account + ":user/alice"
	retirer := "arn:aws:iam::" + account + ":user/retire"
	grant, err := st.CreateGrant(account, k2.KeyID, grantee, retirer, []string{"Encrypt", "Decrypt"}, "wave2")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.RetireGrant(grant.GrantID, "arn:aws:iam::"+account+":user/other"); err == nil {
		t.Fatal("unauthorized retire must fail")
	}
	if err := st.RetireGrant(grant.GrantID, retirer); err != nil {
		t.Fatal(err)
	}
	if err := st.RevokeGrant(grant.GrantID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("revoke after retire err=%v", err)
	}

	grant2, err := st.CreateGrant(account, k2.KeyID, grantee, "", []string{"Encrypt"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.RevokeGrant(grant2.GrantID); err != nil {
		t.Fatal(err)
	}
	if err := st.RevokeGrant("missing-grant"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("revoke missing err=%v", err)
	}
	if err := st.RetireGrant("missing-grant", grantee); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("retire missing err=%v", err)
	}
}
