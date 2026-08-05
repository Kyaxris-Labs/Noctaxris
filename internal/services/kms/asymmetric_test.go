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

func TestNormalizeSigningAlgorithmDefaults(t *testing.T) {
	alg, err := kmssvc.NormalizeSigningAlgorithm("")
	if err != nil || alg != kmssvc.SigningAlgorithmPSSSHA256 {
		t.Fatalf("default alg=%q err=%v", alg, err)
	}
	alg, err = kmssvc.NormalizeSigningAlgorithm(kmssvc.SigningAlgorithmPKCS1SHA256)
	if err != nil || alg != kmssvc.SigningAlgorithmPKCS1SHA256 {
		t.Fatalf("pkcs1 alg=%q err=%v", alg, err)
	}
}

func TestNormalizeMessageTypeAndDigestErrors(t *testing.T) {
	mt, err := kmssvc.NormalizeMessageType("")
	if err != nil || mt != kmssvc.MessageTypeRAW {
		t.Fatalf("mt=%q err=%v", mt, err)
	}
	if _, err := kmssvc.NormalizeMessageType("BAD"); err == nil {
		t.Fatal("expected invalid message type")
	}
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := kmssvc.SignRSA(priv, []byte{1}, kmssvc.SigningAlgorithmPSSSHA256, kmssvc.MessageTypeDIGEST); err == nil {
		t.Fatal("expected digest length error")
	}
}

func TestPublicKeyPEMSPKIAndLabAlgorithms(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pem, err := kmssvc.PublicKeyPEMSPKI(&priv.PublicKey)
	if err != nil || len(pem) == 0 {
		t.Fatalf("pem len=%d err=%v", len(pem), err)
	}
	algs := kmssvc.LabSigningAlgorithms()
	if len(algs) != 2 {
		t.Fatalf("algs=%v", algs)
	}
}
