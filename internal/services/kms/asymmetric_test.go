package kms_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"testing"

	kmssvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/kms"
)

func TestSignVerifyPSSAndPKCS1(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	msg := []byte("unit-test-message")

	sig, err := kmssvc.SignRSA(priv, msg, "", "")
	if err != nil {
		t.Fatal(err)
	}
	ok, err := kmssvc.VerifyRSA(&priv.PublicKey, msg, sig, kmssvc.SigningAlgorithmPSSSHA256, kmssvc.MessageTypeRAW)
	if err != nil || !ok {
		t.Fatalf("default PSS verify ok=%v err=%v", ok, err)
	}

	digest := sha256.Sum256(msg)
	sigDig, err := kmssvc.SignRSA(priv, digest[:], kmssvc.SigningAlgorithmPKCS1SHA256, kmssvc.MessageTypeDIGEST)
	if err != nil {
		t.Fatal(err)
	}
	ok, err = kmssvc.VerifyRSA(&priv.PublicKey, digest[:], sigDig, kmssvc.SigningAlgorithmPKCS1SHA256, kmssvc.MessageTypeDIGEST)
	if err != nil || !ok {
		t.Fatalf("digest PKCS1 verify ok=%v err=%v", ok, err)
	}

	ok, err = kmssvc.VerifyRSA(&priv.PublicKey, []byte("tampered"), sig, kmssvc.SigningAlgorithmPSSSHA256, "")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected verify failure for tampered message")
	}
}

func TestNormalizeSigningAlgorithmRejectsUnknown(t *testing.T) {
	_, err := kmssvc.NormalizeSigningAlgorithm("ECDSA_SHA_256")
	if err == nil {
		t.Fatal("expected unsupported algorithm error")
	}
}
