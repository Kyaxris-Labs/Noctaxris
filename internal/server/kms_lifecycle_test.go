package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestKMSScheduleCancelKeyDeletion(t *testing.T) {
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
	if keyID == "" {
		t.Fatal("missing KeyId")
	}

	schedRec := mustKMSJSON(t, handler, "ScheduleKeyDeletion", map[string]any{
		"KeyId":               keyID,
		"PendingWindowInDays": 7,
	}, now)
	if schedRec.Code != http.StatusOK {
		t.Fatalf("ScheduleKeyDeletion status=%d body=%q", schedRec.Code, schedRec.Body.String())
	}
	var schedOut map[string]any
	if err := json.Unmarshal(schedRec.Body.Bytes(), &schedOut); err != nil {
		t.Fatal(err)
	}
	if schedOut["KeyState"] != "PendingDeletion" {
		t.Fatalf("KeyState=%v", schedOut["KeyState"])
	}
	if schedOut["PendingWindowInDays"] != float64(7) {
		t.Fatalf("PendingWindowInDays=%v", schedOut["PendingWindowInDays"])
	}

	plain := "aGVsbG8="
	encRec := mustKMSJSON(t, handler, "Encrypt", map[string]any{
		"KeyId":     keyID,
		"Plaintext": plain,
	}, now)
	if encRec.Code != http.StatusBadRequest {
		t.Fatalf("Encrypt while pending status=%d want 400 body=%q", encRec.Code, encRec.Body.String())
	}
	if !strings.Contains(encRec.Body.String(), "KMSInvalidStateException") {
		t.Fatalf("expected KMSInvalidStateException in %q", encRec.Body.String())
	}

	cancelRec := mustKMSJSON(t, handler, "CancelKeyDeletion", map[string]any{"KeyId": keyID}, now)
	if cancelRec.Code != http.StatusOK {
		t.Fatalf("CancelKeyDeletion status=%d body=%q", cancelRec.Code, cancelRec.Body.String())
	}

	descRec := mustKMSJSON(t, handler, "DescribeKey", map[string]any{"KeyId": keyID}, now)
	if descRec.Code != http.StatusOK {
		t.Fatalf("DescribeKey status=%d body=%q", descRec.Code, descRec.Body.String())
	}
	var descOut map[string]any
	if err := json.Unmarshal(descRec.Body.Bytes(), &descOut); err != nil {
		t.Fatal(err)
	}
	descMeta, _ := descOut["KeyMetadata"].(map[string]any)
	if descMeta["KeyState"] != "Disabled" {
		t.Fatalf("after cancel KeyState=%v want Disabled", descMeta["KeyState"])
	}

	encDisabled := mustKMSJSON(t, handler, "Encrypt", map[string]any{
		"KeyId":     keyID,
		"Plaintext": plain,
	}, now)
	if encDisabled.Code != http.StatusBadRequest {
		t.Fatalf("Encrypt while disabled status=%d want 400 body=%q", encDisabled.Code, encDisabled.Body.String())
	}

	enRec := mustKMSJSON(t, handler, "EnableKey", map[string]any{"KeyId": keyID}, now)
	if enRec.Code != http.StatusOK {
		t.Fatalf("EnableKey status=%d body=%q", enRec.Code, enRec.Body.String())
	}

	encOK := mustKMSJSON(t, handler, "Encrypt", map[string]any{
		"KeyId":     keyID,
		"Plaintext": plain,
	}, now)
	if encOK.Code != http.StatusOK {
		t.Fatalf("Encrypt after EnableKey status=%d body=%q", encOK.Code, encOK.Body.String())
	}
}

func TestKMSKeyRotationStatus(t *testing.T) {
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

	getRec := mustKMSJSON(t, handler, "GetKeyRotationStatus", map[string]any{"KeyId": keyID}, now)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GetKeyRotationStatus status=%d body=%q", getRec.Code, getRec.Body.String())
	}
	var getOut map[string]any
	if err := json.Unmarshal(getRec.Body.Bytes(), &getOut); err != nil {
		t.Fatal(err)
	}
	if getOut["KeyRotationEnabled"] != false {
		t.Fatalf("KeyRotationEnabled=%v want false", getOut["KeyRotationEnabled"])
	}

	enRec := mustKMSJSON(t, handler, "EnableKeyRotation", map[string]any{"KeyId": keyID}, now)
	if enRec.Code != http.StatusOK {
		t.Fatalf("EnableKeyRotation status=%d body=%q", enRec.Code, enRec.Body.String())
	}
	getRec = mustKMSJSON(t, handler, "GetKeyRotationStatus", map[string]any{"KeyId": keyID}, now)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GetKeyRotationStatus status=%d body=%q", getRec.Code, getRec.Body.String())
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &getOut); err != nil {
		t.Fatal(err)
	}
	if getOut["KeyRotationEnabled"] != true {
		t.Fatalf("KeyRotationEnabled=%v want true", getOut["KeyRotationEnabled"])
	}

	disRec := mustKMSJSON(t, handler, "DisableKeyRotation", map[string]any{"KeyId": keyID}, now)
	if disRec.Code != http.StatusOK {
		t.Fatalf("DisableKeyRotation status=%d body=%q", disRec.Code, disRec.Body.String())
	}
	getRec = mustKMSJSON(t, handler, "GetKeyRotationStatus", map[string]any{"KeyId": keyID}, now)
	if err := json.Unmarshal(getRec.Body.Bytes(), &getOut); err != nil {
		t.Fatal(err)
	}
	if getOut["KeyRotationEnabled"] != false {
		t.Fatalf("KeyRotationEnabled=%v want false after disable", getOut["KeyRotationEnabled"])
	}
}

func TestKMSReEncrypt(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	srcRec := mustKMSJSON(t, handler, "CreateKey", map[string]any{}, now)
	if srcRec.Code != http.StatusOK {
		t.Fatalf("CreateKey source status=%d body=%q", srcRec.Code, srcRec.Body.String())
	}
	dstRec := mustKMSJSON(t, handler, "CreateKey", map[string]any{}, now)
	if dstRec.Code != http.StatusOK {
		t.Fatalf("CreateKey dest status=%d body=%q", dstRec.Code, dstRec.Body.String())
	}
	var srcOut, dstOut map[string]any
	if err := json.Unmarshal(srcRec.Body.Bytes(), &srcOut); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(dstRec.Body.Bytes(), &dstOut); err != nil {
		t.Fatal(err)
	}
	srcMeta, _ := srcOut["KeyMetadata"].(map[string]any)
	dstMeta, _ := dstOut["KeyMetadata"].(map[string]any)
	srcID, _ := srcMeta["KeyId"].(string)
	dstID, _ := dstMeta["KeyId"].(string)
	if srcID == "" || dstID == "" {
		t.Fatal("missing KeyId")
	}

	plain := "aGVsbG8=" // "hello"
	encRec := mustKMSJSON(t, handler, "Encrypt", map[string]any{
		"KeyId":     srcID,
		"Plaintext": plain,
	}, now)
	if encRec.Code != http.StatusOK {
		t.Fatalf("Encrypt status=%d body=%q", encRec.Code, encRec.Body.String())
	}
	var encOut map[string]any
	if err := json.Unmarshal(encRec.Body.Bytes(), &encOut); err != nil {
		t.Fatal(err)
	}
	blob, _ := encOut["CiphertextBlob"].(string)

	reRec := mustKMSJSON(t, handler, "ReEncrypt", map[string]any{
		"CiphertextBlob":   blob,
		"DestinationKeyId": dstID,
	}, now)
	if reRec.Code != http.StatusOK {
		t.Fatalf("ReEncrypt status=%d body=%q", reRec.Code, reRec.Body.String())
	}
	var reOut map[string]any
	if err := json.Unmarshal(reRec.Body.Bytes(), &reOut); err != nil {
		t.Fatal(err)
	}
	newBlob, _ := reOut["CiphertextBlob"].(string)
	if newBlob == "" || newBlob == blob {
		t.Fatalf("expected new ciphertext, got %q", newBlob)
	}
	srcARN, _ := reOut["SourceKeyId"].(string)
	dstARN, _ := reOut["KeyId"].(string)
	if !strings.Contains(srcARN, srcID) || !strings.Contains(dstARN, dstID) {
		t.Fatalf("SourceKeyId=%q KeyId=%q", srcARN, dstARN)
	}

	decRec := mustKMSJSON(t, handler, "Decrypt", map[string]any{
		"CiphertextBlob": newBlob,
		"KeyId":          dstID,
	}, now)
	if decRec.Code != http.StatusOK {
		t.Fatalf("Decrypt reencrypted status=%d body=%q", decRec.Code, decRec.Body.String())
	}
	var decOut map[string]any
	if err := json.Unmarshal(decRec.Body.Bytes(), &decOut); err != nil {
		t.Fatal(err)
	}
	if decOut["Plaintext"] != plain {
		t.Fatalf("Plaintext=%v want %q", decOut["Plaintext"], plain)
	}
}

func TestKMSReEncryptRejectsDisabledSource(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	srcRec := mustKMSJSON(t, handler, "CreateKey", map[string]any{}, now)
	dstRec := mustKMSJSON(t, handler, "CreateKey", map[string]any{}, now)
	var srcOut, dstOut map[string]any
	_ = json.Unmarshal(srcRec.Body.Bytes(), &srcOut)
	_ = json.Unmarshal(dstRec.Body.Bytes(), &dstOut)
	srcID := srcOut["KeyMetadata"].(map[string]any)["KeyId"].(string)
	dstID := dstOut["KeyMetadata"].(map[string]any)["KeyId"].(string)

	encRec := mustKMSJSON(t, handler, "Encrypt", map[string]any{
		"KeyId":     srcID,
		"Plaintext": "aGVsbG8=",
	}, now)
	var encOut map[string]any
	_ = json.Unmarshal(encRec.Body.Bytes(), &encOut)

	disRec := mustKMSJSON(t, handler, "DisableKey", map[string]any{"KeyId": srcID}, now)
	if disRec.Code != http.StatusOK {
		t.Fatalf("DisableKey status=%d", disRec.Code)
	}

	reRec := mustKMSJSON(t, handler, "ReEncrypt", map[string]any{
		"CiphertextBlob":   encOut["CiphertextBlob"],
		"DestinationKeyId": dstID,
		"SourceKeyId":      srcID,
	}, now)
	if reRec.Code != http.StatusBadRequest {
		t.Fatalf("ReEncrypt disabled source status=%d want 400 body=%q", reRec.Code, reRec.Body.String())
	}
	if !strings.Contains(reRec.Body.String(), "DisabledException") {
		t.Fatalf("expected DisabledException in %q", reRec.Body.String())
	}
}

func TestKMSAWSManagedAliasResolve(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	plain := "YWxpYXM="
	encRec := mustKMSJSON(t, handler, "Encrypt", map[string]any{
		"KeyId":     "alias/aws/s3",
		"Plaintext": plain,
	}, now)
	if encRec.Code != http.StatusOK {
		t.Fatalf("Encrypt via alias/aws/s3 status=%d body=%q", encRec.Code, encRec.Body.String())
	}
	var encOut map[string]any
	if err := json.Unmarshal(encRec.Body.Bytes(), &encOut); err != nil {
		t.Fatal(err)
	}
	decRec := mustKMSJSON(t, handler, "Decrypt", map[string]any{
		"CiphertextBlob": encOut["CiphertextBlob"],
		"KeyId":          "alias/aws/dynamodb",
	}, now)
	if decRec.Code != http.StatusOK {
		t.Fatalf("Decrypt via alias/aws/dynamodb status=%d body=%q", decRec.Code, decRec.Body.String())
	}
	var decOut map[string]any
	if err := json.Unmarshal(decRec.Body.Bytes(), &decOut); err != nil {
		t.Fatal(err)
	}
	if decOut["Plaintext"] != plain {
		t.Fatalf("Plaintext=%v want %q", decOut["Plaintext"], plain)
	}
}
