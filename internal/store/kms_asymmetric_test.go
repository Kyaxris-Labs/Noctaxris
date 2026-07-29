package store_test

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	kmssvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/kms"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestKMSCreateKeyRSA2048SignVerify(t *testing.T) {
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

	accountID := "000000000001"
	k, err := st.CreateKeyWithParams(store.CreateKeyParams{
		AccountID:  accountID,
		CreatorARN: "arn:aws:iam::" + accountID + ":root",
		KeySpec:    store.KeySpecRSA2048,
	})
	if err != nil {
		t.Fatal(err)
	}
	if k.KeyUsage != store.KeyUsageSignVerify {
		t.Fatalf("KeyUsage=%q want SIGN_VERIFY", k.KeyUsage)
	}
	if k.KeySpec != store.KeySpecRSA2048 {
		t.Fatalf("KeySpec=%q want RSA_2048", k.KeySpec)
	}

	got, err := st.GetKey(k.KeyID)
	if err != nil {
		t.Fatal(err)
	}
	if got.KeySpec != store.KeySpecRSA2048 || got.KeyUsage != store.KeyUsageSignVerify {
		t.Fatalf("GetKey usage/spec = %s/%s", got.KeyUsage, got.KeySpec)
	}

	priv, err := st.UnsealRSAPrivateKey(k.KeyID)
	if err != nil {
		t.Fatal(err)
	}
	if priv.N.BitLen() != 2048 {
		t.Fatalf("rsa bits=%d want 2048", priv.N.BitLen())
	}

	material, err := st.UnsealKeyMaterial(k.KeyID)
	if err != nil {
		t.Fatal(err)
	}
	dbBytes, err := os.ReadFile(filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(dbBytes, material) {
		t.Fatal("plaintext PKCS8 material found in db file")
	}
}

func TestKMSCreateKeyCustomerMasterKeySpecRSA2048(t *testing.T) {
	st := openKMSStore(t)
	k, err := st.CreateKeyWithParams(store.CreateKeyParams{
		AccountID:  "000000000001",
		CreatorARN: "arn:aws:iam::000000000001:root",
		KeyUsage:   store.KeyUsageSignVerify,
		KeySpec:    store.KeySpecRSA2048,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !store.IsAsymmetricSignVerify(k) {
		t.Fatalf("expected asymmetric sign/verify key: %+v", k)
	}
}

func TestKMSUnsealRSAPrivateKeyRejectsSymmetric(t *testing.T) {
	st := openKMSStore(t)
	k, err := st.CreateKey("000000000001", "arn:aws:iam::000000000001:root", "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.UnsealRSAPrivateKey(k.KeyID)
	if err == nil {
		t.Fatal("expected error for symmetric key")
	}
}

func TestKMSSignVerifyRoundTripStore(t *testing.T) {
	st := openKMSStore(t)
	k, err := st.CreateKeyWithParams(store.CreateKeyParams{
		AccountID:  "000000000001",
		CreatorARN: "arn:aws:iam::000000000001:root",
		KeySpec:    store.KeySpecRSA2048,
	})
	if err != nil {
		t.Fatal(err)
	}
	priv, err := st.UnsealRSAPrivateKey(k.KeyID)
	if err != nil {
		t.Fatal(err)
	}
	msg := []byte("lab-sign-message")
	sig, err := kmssvc.SignRSA(priv, msg, kmssvc.SigningAlgorithmPSSSHA256, kmssvc.MessageTypeRAW)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := kmssvc.VerifyRSA(&priv.PublicKey, msg, sig, kmssvc.SigningAlgorithmPSSSHA256, kmssvc.MessageTypeRAW)
	if err != nil || !ok {
		t.Fatalf("verify pss: ok=%v err=%v", ok, err)
	}
	sig2, err := kmssvc.SignRSA(priv, msg, kmssvc.SigningAlgorithmPKCS1SHA256, "")
	if err != nil {
		t.Fatal(err)
	}
	ok, err = kmssvc.VerifyRSA(&priv.PublicKey, msg, sig2, kmssvc.SigningAlgorithmPKCS1SHA256, "")
	if err != nil || !ok {
		t.Fatalf("verify pkcs1: ok=%v err=%v", ok, err)
	}
	pemSPKI, err := kmssvc.PublicKeyPEMSPKI(&priv.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(pemSPKI)
	if block == nil || block.Type != "PUBLIC KEY" {
		t.Fatalf("expected PEM PUBLIC KEY, got %q", pemSPKI)
	}
	if _, err := x509.ParsePKIXPublicKey(block.Bytes); err != nil {
		t.Fatal(err)
	}
}
