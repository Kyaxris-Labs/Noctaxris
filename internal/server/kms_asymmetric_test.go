package server_test

import (
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"strings"
	"testing"
	"time"

	kmssvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/kms"
)

func TestKMSSignVerifyGetPublicKey(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustKMSJSON(t, handler, "CreateKey", map[string]any{
		"KeyUsage": "SIGN_VERIFY",
		"KeySpec":  "RSA_2048",
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateKey status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	var createOut map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &createOut); err != nil {
		t.Fatal(err)
	}
	meta, _ := createOut["KeyMetadata"].(map[string]any)
	keyID, _ := meta["KeyId"].(string)
	if keyID == "" {
		t.Fatal("missing KeyId")
	}
	if meta["KeyUsage"] != "SIGN_VERIFY" || meta["KeySpec"] != "RSA_2048" {
		t.Fatalf("metadata usage/spec = %v/%v", meta["KeyUsage"], meta["KeySpec"])
	}

	msgB64 := base64.StdEncoding.EncodeToString([]byte("hello-sign"))
	signRec := mustKMSJSON(t, handler, "Sign", map[string]any{
		"KeyId":            keyID,
		"Message":          msgB64,
		"SigningAlgorithm": kmssvc.SigningAlgorithmPSSSHA256,
	}, now)
	if signRec.Code != http.StatusOK {
		t.Fatalf("Sign status=%d body=%q", signRec.Code, signRec.Body.String())
	}
	var signOut map[string]any
	if err := json.Unmarshal(signRec.Body.Bytes(), &signOut); err != nil {
		t.Fatal(err)
	}
	sig, _ := signOut["Signature"].(string)
	if sig == "" {
		t.Fatal("missing Signature")
	}
	if signOut["SigningAlgorithm"] != kmssvc.SigningAlgorithmPSSSHA256 {
		t.Fatalf("SigningAlgorithm=%v", signOut["SigningAlgorithm"])
	}

	verifyRec := mustKMSJSON(t, handler, "Verify", map[string]any{
		"KeyId":            keyID,
		"Message":          msgB64,
		"Signature":        sig,
		"SigningAlgorithm": kmssvc.SigningAlgorithmPSSSHA256,
	}, now)
	if verifyRec.Code != http.StatusOK {
		t.Fatalf("Verify status=%d body=%q", verifyRec.Code, verifyRec.Body.String())
	}
	var verifyOut map[string]any
	if err := json.Unmarshal(verifyRec.Body.Bytes(), &verifyOut); err != nil {
		t.Fatal(err)
	}
	if verifyOut["SignatureValid"] != true {
		t.Fatalf("SignatureValid=%v", verifyOut["SignatureValid"])
	}

	pkcsSign := mustKMSJSON(t, handler, "Sign", map[string]any{
		"KeyId":            keyID,
		"Message":          msgB64,
		"SigningAlgorithm": kmssvc.SigningAlgorithmPKCS1SHA256,
	}, now)
	if pkcsSign.Code != http.StatusOK {
		t.Fatalf("Sign PKCS1 status=%d body=%q", pkcsSign.Code, pkcsSign.Body.String())
	}

	pubRec := mustKMSJSON(t, handler, "GetPublicKey", map[string]any{"KeyId": keyID}, now)
	if pubRec.Code != http.StatusOK {
		t.Fatalf("GetPublicKey status=%d body=%q", pubRec.Code, pubRec.Body.String())
	}
	var pubOut map[string]any
	if err := json.Unmarshal(pubRec.Body.Bytes(), &pubOut); err != nil {
		t.Fatal(err)
	}
	pubB64, _ := pubOut["PublicKey"].(string)
	pemBytes, err := base64.StdEncoding.DecodeString(pubB64)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(pemBytes)
	if block == nil || block.Type != "PUBLIC KEY" {
		t.Fatalf("PublicKey is not PEM SPKI: %q", pemBytes)
	}
	if pubOut["KeySpec"] != "RSA_2048" || pubOut["KeyUsage"] != "SIGN_VERIFY" {
		t.Fatalf("GetPublicKey KeySpec/KeyUsage = %v/%v", pubOut["KeySpec"], pubOut["KeyUsage"])
	}

	encRec := mustKMSJSON(t, handler, "Encrypt", map[string]any{
		"KeyId":     keyID,
		"Plaintext": base64.StdEncoding.EncodeToString([]byte("nope")),
	}, now)
	if encRec.Code != http.StatusBadRequest || !strings.Contains(encRec.Body.String(), "InvalidKeyUsageException") {
		t.Fatalf("Encrypt on SIGN_VERIFY status=%d body=%q", encRec.Code, encRec.Body.String())
	}
}

func TestKMSSignRejectsSymmetricKey(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustKMSJSON(t, handler, "CreateKey", map[string]any{}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateKey status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	var createOut map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &createOut); err != nil {
		t.Fatal(err)
	}
	meta, _ := createOut["KeyMetadata"].(map[string]any)
	keyID, _ := meta["KeyId"].(string)

	signRec := mustKMSJSON(t, handler, "Sign", map[string]any{
		"KeyId":   keyID,
		"Message": base64.StdEncoding.EncodeToString([]byte("hello")),
	}, now)
	if signRec.Code != http.StatusBadRequest || !strings.Contains(signRec.Body.String(), "InvalidKeyUsageException") {
		t.Fatalf("Sign on symmetric status=%d body=%q", signRec.Code, signRec.Body.String())
	}

	verifyRec := mustKMSJSON(t, handler, "Verify", map[string]any{
		"KeyId":     keyID,
		"Message":   base64.StdEncoding.EncodeToString([]byte("hello")),
		"Signature": base64.StdEncoding.EncodeToString([]byte("sig")),
	}, now)
	if verifyRec.Code != http.StatusBadRequest || !strings.Contains(verifyRec.Body.String(), "InvalidKeyUsageException") {
		t.Fatalf("Verify on symmetric status=%d body=%q", verifyRec.Code, verifyRec.Body.String())
	}

	pubRec := mustKMSJSON(t, handler, "GetPublicKey", map[string]any{"KeyId": keyID}, now)
	if pubRec.Code != http.StatusBadRequest || !strings.Contains(pubRec.Body.String(), "UnsupportedOperationException") {
		t.Fatalf("GetPublicKey on symmetric status=%d body=%q", pubRec.Code, pubRec.Body.String())
	}
}

func TestKMSCreateKeyCustomerMasterKeySpec(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustKMSJSON(t, handler, "CreateKey", map[string]any{
		"CustomerMasterKeySpec": "RSA_2048",
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateKey status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	var createOut map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &createOut); err != nil {
		t.Fatal(err)
	}
	meta, _ := createOut["KeyMetadata"].(map[string]any)
	if meta["KeyUsage"] != "SIGN_VERIFY" || meta["CustomerMasterKeySpec"] != "RSA_2048" {
		t.Fatalf("metadata=%v", meta)
	}
}
