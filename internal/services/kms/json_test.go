package kms_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	kmssvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/kms"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func symKey() store.Key {
	return store.Key{
		KeyID:        "sym-key",
		AccountID:    "000000000001",
		ARN:          "arn:aws:kms:us-east-1:000000000001:key/sym-key",
		KeyState:     store.KeyStateEnabled,
		KeyUsage:     store.KeyUsageEncryptDecrypt,
		KeySpec:      store.KeySpecSymmetricDefault,
		CreationDate: "2024-06-01T12:00:00Z",
	}
}

func asymKey() store.Key {
	return store.Key{
		KeyID:        "asym-key",
		AccountID:    "000000000001",
		ARN:          "arn:aws:kms:us-east-1:000000000001:key/asym-key",
		KeyState:     store.KeyStateEnabled,
		KeyUsage:     store.KeyUsageSignVerify,
		KeySpec:      store.KeySpecRSA2048,
		CreationDate: "2024-06-01T12:00:00Z",
		DeletionDate: "2025-06-01T12:00:00Z",
		PendingWindowInDays: 7,
	}
}

func TestKMSJSONAndCryptoHelpers(t *testing.T) {
	cmk := bytes.Repeat([]byte{0x11}, 32)
	keyID := "key-lab"
	plain := []byte("secret")
	ctx := map[string]string{"tenant": "lab"}

	blob, err := kmssvc.EncryptUnderCMK(cmk, keyID, plain, ctx)
	if err != nil {
		t.Fatal(err)
	}
	gotID, err := kmssvc.KeyIDFromCiphertext(blob)
	if err != nil || gotID != keyID {
		t.Fatalf("key id=%q err=%v", gotID, err)
	}
	dec, err := kmssvc.DecryptUnderCMK(cmk, blob, ctx)
	if err != nil || !bytes.Equal(dec, plain) {
		t.Fatalf("dec=%q err=%v", dec, err)
	}

	nilCtx, err := kmssvc.ParseEncryptionContext(nil)
	if err != nil || nilCtx != nil {
		t.Fatalf("nil ctx=%v err=%v", nilCtx, err)
	}
	parsed, err := kmssvc.ParseEncryptionContext(map[string]any{"k": "v"})
	if err != nil || parsed["k"] != "v" {
		t.Fatalf("parsed=%v err=%v", parsed, err)
	}
	if _, err := kmssvc.ParseEncryptionContext([]any{}); err == nil {
		t.Fatal("expected type error")
	}
	if _, err := kmssvc.ParseEncryptionContext(map[string]any{"": "v"}); err == nil {
		t.Fatal("expected empty key error")
	}
	if _, err := kmssvc.ParseEncryptionContext(map[string]any{"k": 1}); err == nil {
		t.Fatal("expected value type error")
	}

	sk := symKey()
	createRaw, err := kmssvc.CreateKeyJSON(sk)
	if err != nil {
		t.Fatal(err)
	}
	descRaw, err := kmssvc.DescribeKeyJSON(sk)
	if err != nil {
		t.Fatal(err)
	}
	if string(createRaw) != string(descRaw) {
		t.Fatalf("create/desc mismatch")
	}
	listKeys, err := kmssvc.ListKeysJSON([]store.Key{sk})
	if err != nil {
		t.Fatal(err)
	}
	empty, err := kmssvc.EmptyOKJSON()
	if err != nil || string(empty) != `{}` {
		t.Fatal(err)
	}
	tagsRaw, err := kmssvc.ListResourceTagsJSON([]store.ResourceTag{{Key: "a", Value: "b"}})
	if err != nil {
		t.Fatal(err)
	}
	polRaw, err := kmssvc.GetKeyPolicyJSON(`{"Version":"2012-10-17"}`)
	if err != nil {
		t.Fatal(err)
	}
	encJSON, err := kmssvc.EncryptJSON(sk.KeyID, blob)
	if err != nil {
		t.Fatal(err)
	}
	reEnc, err := kmssvc.ReEncryptJSON(sk.ARN, sk.ARN, blob)
	if err != nil {
		t.Fatal(err)
	}
	decJSON, err := kmssvc.DecryptJSON(sk.KeyID, plain)
	if err != nil {
		t.Fatal(err)
	}
	genDK, err := kmssvc.GenerateDataKeyJSON(sk.KeyID, plain, blob, true)
	if err != nil {
		t.Fatal(err)
	}
	genNoPlain, err := kmssvc.GenerateDataKeyJSON(sk.KeyID, plain, blob, false)
	if err != nil {
		t.Fatal(err)
	}
	var genOut map[string]any
	if err := json.Unmarshal(genNoPlain, &genOut); err != nil {
		t.Fatal(err)
	}
	if _, has := genOut["Plaintext"]; has {
		t.Fatalf("unexpected plaintext=%v", genOut)
	}

	grant := store.Grant{
		GrantID:           "grant-1",
		KeyID:             sk.KeyID,
		GranteePrincipal:  "arn:aws:iam::000000000001:role/lab",
		RetiringPrincipal: "arn:aws:iam::000000000001:root",
		Name:              "g",
		Operations:        []string{"Decrypt"},
	}
	createGrant, err := kmssvc.CreateGrantJSON(grant)
	if err != nil {
		t.Fatal(err)
	}
	listGrants, err := kmssvc.ListGrantsJSON([]store.Grant{grant})
	if err != nil {
		t.Fatal(err)
	}
	aliasRaw, err := kmssvc.ListAliasesJSON([]store.Alias{{AliasName: "alias/lab", TargetKeyID: sk.KeyID}}, sk.AccountID)
	if err != nil {
		t.Fatal(err)
	}
	sched, err := kmssvc.ScheduleKeyDeletionJSON(asymKey())
	if err != nil {
		t.Fatal(err)
	}
	cancel, err := kmssvc.CancelKeyDeletionJSON(sk.ARN)
	if err != nil {
		t.Fatal(err)
	}
	rot, err := kmssvc.GetKeyRotationStatusJSON(true)
	if err != nil {
		t.Fatal(err)
	}
	sig, err := kmssvc.SignJSON(sk.KeyID, []byte{1, 2}, kmssvc.SigningAlgorithmPSSSHA256)
	if err != nil {
		t.Fatal(err)
	}
	ver, err := kmssvc.VerifyJSON(sk.KeyID, true, kmssvc.SigningAlgorithmPSSSHA256)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := kmssvc.GetPublicKeyJSON(sk.KeyID, []byte("pem"), store.KeySpecRSA2048, store.KeyUsageSignVerify, kmssvc.LabSigningAlgorithms())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(createGrant), "grant-1") || !strings.Contains(string(listGrants), "Decrypt") {
		t.Fatalf("grant create=%s list=%s", createGrant, listGrants)
	}
	if !strings.Contains(string(aliasRaw), "alias/lab") || !strings.Contains(string(sched), "DeletionDate") {
		t.Fatalf("alias=%s sched=%s", aliasRaw, sched)
	}
	if !strings.Contains(string(cancel), sk.ARN) || !strings.Contains(string(rot), "true") {
		t.Fatalf("cancel=%s rot=%s", cancel, rot)
	}
	if !strings.Contains(string(sig), "Signature") || !strings.Contains(string(ver), "true") {
		t.Fatalf("sig=%s ver=%s", sig, ver)
	}
	if !strings.Contains(string(pub), "SigningAlgorithms") {
		t.Fatalf("pub=%s", pub)
	}

	bin, err := kmssvc.DecodeBinaryField(base64.StdEncoding.EncodeToString([]byte{7}))
	if err != nil || len(bin) != 1 || bin[0] != 7 {
		t.Fatalf("bin=%v err=%v", bin, err)
	}
	binBytes, err := kmssvc.DecodeBinaryField([]byte{8})
	if err != nil || len(binBytes) != 1 {
		t.Fatalf("bin bytes err=%v", err)
	}
	if _, err := kmssvc.DecodeBinaryField(1); err == nil {
		t.Fatal("expected decode type error")
	}

	// Asymmetric metadata includes signing algorithms; bad creation date -> 0 float.
	badDate := sk
	badDate.CreationDate = "bad"
	badRaw, err := kmssvc.DescribeKeyJSON(badDate)
	if err != nil {
		t.Fatal(err)
	}
	var badMeta map[string]any
	if err := json.Unmarshal(badRaw, &badMeta); err != nil {
		t.Fatal(err)
	}
	km, _ := badMeta["KeyMetadata"].(map[string]any)
	if km["CreationDate"] != float64(0) {
		t.Fatalf("bad date meta=%v", km)
	}
	asymRaw, err := kmssvc.DescribeKeyJSON(asymKey())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(asymRaw), "SigningAlgorithms") {
		t.Fatalf("asym=%s", asymRaw)
	}

	_ = listKeys
	_ = tagsRaw
	_ = polRaw
	_ = encJSON
	_ = reEnc
	_ = decJSON
	_ = genDK
}

func TestParseCreationFloatViaSchedule(t *testing.T) {
	k := asymKey()
	k.DeletionDate = time.Unix(1_700_000_000, 0).UTC().Format(time.RFC3339)
	raw, err := kmssvc.ScheduleKeyDeletionJSON(k)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out["DeletionDate"] != float64(1_700_000_000) {
		t.Fatalf("out=%v", out)
	}
}
