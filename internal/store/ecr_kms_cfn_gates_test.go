package store_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
	"github.com/Kyaxris-Labs/Noctaxris/internal/validate"
)

func TestECRManifestPathRejectsEscape(t *testing.T) {
	st := openECRStore(t)
	account := "000000000001"
	if _, err := st.CreateRepository(account, "us-east-1", "safe-repo"); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(filepath.Dir(st.DataRoot()), "outside-secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(st.DataRoot(), outside)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutImage(account, "safe-repo", "sha256:"+strings.Repeat("a", 64), nil, rel); err == nil {
		t.Fatal("want PutImage reject path escape")
	}
	if _, err := validate.ResolveUnderRoot(st.DataRoot(), rel); err == nil || !validate.IsInvalid(err) {
		t.Fatalf("want ResolveUnderRoot invalid, got %v", err)
	}
}

func TestKMSRevokeGrantBoundToKey(t *testing.T) {
	st := openKMSStore(t)
	account := "000000000001"
	a, err := st.CreateKey(account, "arn:aws:iam::"+account+":root", "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := st.CreateKey(account, "arn:aws:iam::"+account+":root", "")
	if err != nil {
		t.Fatal(err)
	}
	g, err := st.CreateGrant(account, b.KeyID, "arn:aws:iam::"+account+":user/alice", "", []string{"Decrypt"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.RevokeGrantOnKey(a.KeyID, g.GrantID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("want sql.ErrNoRows on wrong key, got %v", err)
	}
	list, err := st.ListGrants(b.KeyID)
	if err != nil || len(list) != 1 {
		t.Fatalf("grant must remain on key B: %v %#v", err, list)
	}
	if err := st.RevokeGrantOnKey(b.KeyID, g.GrantID); err != nil {
		t.Fatal(err)
	}
}

func TestKMSGrantConstraintsEncryptionContext(t *testing.T) {
	st := openKMSStore(t)
	account := "000000000001"
	k, err := st.CreateKey(account, "arn:aws:iam::"+account+":root", "")
	if err != nil {
		t.Fatal(err)
	}
	grantee := "arn:aws:iam::" + account + ":user/alice"
	_, err = st.CreateGrantWithConstraints(account, k.KeyID, grantee, "", []string{"Decrypt"}, "", store.GrantConstraints{
		EncryptionContextEquals: map[string]string{"purpose": "payments"},
	})
	if err != nil {
		t.Fatal(err)
	}
	ok, err := st.FindMatchingGrantWithContext(k.KeyID, grantee, "Decrypt", map[string]string{"purpose": "payments"})
	if err != nil || !ok {
		t.Fatalf("want match for payments context: ok=%v err=%v", ok, err)
	}
	ok, err = st.FindMatchingGrantWithContext(k.KeyID, grantee, "Decrypt", map[string]string{"purpose": "other"})
	if err != nil || ok {
		t.Fatalf("want deny other context: ok=%v err=%v", ok, err)
	}
	ok, err = st.FindMatchingGrantWithContext(k.KeyID, grantee, "Decrypt", nil)
	if err != nil || ok {
		t.Fatalf("want deny empty context: ok=%v err=%v", ok, err)
	}
}

func TestCFNYAMLAliasCycleRejected(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	_, err := st.CreateCFNStack(account, "us-east-1", "cycle", "&a [*a]\n", "")
	if err == nil {
		t.Fatal("want cyclic YAML rejected")
	}
}

func TestConfigStartRequiresRecorderRole(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	if _, err := st.CreateBucket(account, "cfg-locked"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutConfigRecorder(account, "default", "", "ALL"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutConfigDeliveryChannel(account, "default", "cfg-locked", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := st.StartConfigRecorder(account, "default"); err == nil {
		t.Fatal("want start fail without roleARN")
	}
	roleARN := ensureConfigDeliveryRole(t, st, account, "cfg-ok")
	if _, err := st.PutConfigRecorder(account, "default", roleARN, "ALL"); err != nil {
		t.Fatal(err)
	}
	if err := st.StartConfigRecorder(account, "default"); err != nil {
		t.Fatal(err)
	}
}

func TestCloudControlGetRedactsSecretString(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	if err := st.EnsureCloudControlSchema(); err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureSecretsSchema(); err != nil {
		t.Fatal(err)
	}
	desired := `{"Name":"cc-secret","SecretString":"super-secret"}`
	res, _, err := st.CloudControlCreateResource(account, "AWS::SecretsManager::Secret", desired)
	if err != nil {
		t.Fatal(err)
	}
	got, err := st.CloudControlGetResource(account, "AWS::SecretsManager::Secret", res.Identifier)
	if err != nil {
		t.Fatal(err)
	}
	var props map[string]any
	if err := json.Unmarshal([]byte(got.Properties), &props); err != nil {
		t.Fatal(err)
	}
	if _, ok := props["SecretString"]; ok {
		t.Fatalf("SecretString must be redacted: %s", got.Properties)
	}
}

func TestSESBounceRequiresTopicPolicy(t *testing.T) {
	st := openLambdaStore(t)
	account := "000000000001"
	if err := st.EnsureSESSchema(); err != nil {
		t.Fatal(err)
	}
	topic, err := st.CreateTopic(account, "us-east-1", "ses-no-ses", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.VerifySESEmailIdentity(account, "lab@example.com"); err != nil {
		t.Fatal(err)
	}
	err = st.SetSESIdentityNotificationTopic(account, "lab@example.com", "Bounce", topic.TopicARN)
	if err == nil || !strings.Contains(err.Error(), "ses.amazonaws.com") {
		t.Fatalf("want topic policy reject, got %v", err)
	}
}
