package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func setupCrossAccountPair(t *testing.T) (
	handler http.Handler,
	st *store.Store,
	ownerAccount, ownerAKID, ownerSecret string,
	callerAccount, callerAKID, callerSecret, callerUserARN string,
) {
	t.Helper()
	srv, storeRef, _ := newTestServerStore(t)
	handler = srv.Handler()

	var err error
	_, ownerAccount, err = storeRef.CreateMemberAccount(testAccountID, "owner@example.com", "Owner")
	if err != nil {
		t.Fatal(err)
	}
	ownerAKID = "AKIAOWNERROOT0001"
	ownerSecret = "secret-owner-root"
	if err := storeRef.EnsureRoot(ownerAccount, ownerAKID, ownerSecret); err != nil {
		t.Fatal(err)
	}

	_, callerAccount, err = storeRef.CreateMemberAccount(testAccountID, "caller@example.com", "Caller")
	if err != nil {
		t.Fatal(err)
	}
	const callerRootAKID = "AKIACALLERROOT001"
	const callerRootSecret = "secret-caller-root"
	if err := storeRef.EnsureRoot(callerAccount, callerRootAKID, callerRootSecret); err != nil {
		t.Fatal(err)
	}

	_, callerUserARN, err = storeRef.CreateUser(callerAccount, "xa-user")
	if err != nil {
		t.Fatal(err)
	}
	callerAKID, callerSecret, err = storeRef.CreateUserAccessKey(callerAccount, "xa-user")
	if err != nil {
		t.Fatal(err)
	}

	return handler, storeRef, ownerAccount, ownerAKID, ownerSecret, callerAccount, callerAKID, callerSecret, callerUserARN
}

func mustSQSJSONWithCreds(
	t *testing.T,
	handler http.Handler,
	target string,
	payload map[string]any,
	akid, secret string,
	now time.Time,
) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.0")
	req.Header.Set("X-Amz-Target", "AmazonSQS."+target)
	signHeader(t, req, raw, akid, secret, testRegion, "sqs", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestS3CrossAccountIdentityAndBucketPolicyAllow(t *testing.T) {
	handler, st, _, ownerAKID, ownerSecret, _, callerAKID, callerSecret, callerUserARN := setupCrossAccountPair(t)
	now := time.Now().UTC().Truncate(time.Second)

	const bucket = "xa-s3-bucket"
	mustS3WithCreds(t, handler, http.MethodPut, "http://127.0.0.1:4566/"+bucket, nil, ownerAKID, ownerSecret, "s3", now, nil)
	mustS3WithCreds(t, handler, http.MethodPut, "http://127.0.0.1:4566/"+bucket+"/obj.txt", []byte("cross-account"), ownerAKID, ownerSecret, "s3", now, nil)

	identityAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"}]}`
	if err := st.PutInlinePolicy(callerUserARN, "s3get", identityAllow); err != nil {
		t.Fatal(err)
	}

	bucketPolicy := []byte(fmt.Sprintf(
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"%s"},"Action":"s3:GetObject","Resource":"arn:aws:s3:::%s/*"}]}`,
		callerUserARN, bucket,
	))
	polRec := mustS3WithCreds(t, handler, http.MethodPut, "http://127.0.0.1:4566/"+bucket+"?policy", bucketPolicy, ownerAKID, ownerSecret, "s3", now, map[string]string{
		"Content-Type": "application/json",
	})
	if polRec.Code != http.StatusOK {
		t.Fatalf("PutBucketPolicy status=%d body=%q", polRec.Code, polRec.Body.String())
	}

	getRec := mustS3WithCreds(t, handler, http.MethodGet, "http://127.0.0.1:4566/"+bucket+"/obj.txt", nil, callerAKID, callerSecret, "s3", now, nil)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GetObject status=%d want 200 body=%q", getRec.Code, getRec.Body.String())
	}
	if getRec.Body.String() != "cross-account" {
		t.Fatalf("body=%q", getRec.Body.String())
	}
}

func TestS3CrossAccountIdentityOnlyDeny(t *testing.T) {
	handler, st, _, ownerAKID, ownerSecret, _, callerAKID, callerSecret, callerUserARN := setupCrossAccountPair(t)
	now := time.Now().UTC().Truncate(time.Second)

	const bucket = "xa-s3-id-only"
	mustS3WithCreds(t, handler, http.MethodPut, "http://127.0.0.1:4566/"+bucket, nil, ownerAKID, ownerSecret, "s3", now, nil)
	mustS3WithCreds(t, handler, http.MethodPut, "http://127.0.0.1:4566/"+bucket+"/secret.txt", []byte("nope"), ownerAKID, ownerSecret, "s3", now, nil)

	identityAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"}]}`
	if err := st.PutInlinePolicy(callerUserARN, "s3get", identityAllow); err != nil {
		t.Fatal(err)
	}

	getRec := mustS3WithCreds(t, handler, http.MethodGet, "http://127.0.0.1:4566/"+bucket+"/secret.txt", nil, callerAKID, callerSecret, "s3", now, nil)
	if getRec.Code != http.StatusForbidden {
		t.Fatalf("GetObject status=%d want 403 body=%q", getRec.Code, getRec.Body.String())
	}
}

func TestS3CrossAccountPolicyOnlyDeny(t *testing.T) {
	handler, _, _, ownerAKID, ownerSecret, _, callerAKID, callerSecret, callerUserARN := setupCrossAccountPair(t)
	now := time.Now().UTC().Truncate(time.Second)

	const bucket = "xa-s3-pol-only"
	mustS3WithCreds(t, handler, http.MethodPut, "http://127.0.0.1:4566/"+bucket, nil, ownerAKID, ownerSecret, "s3", now, nil)
	mustS3WithCreds(t, handler, http.MethodPut, "http://127.0.0.1:4566/"+bucket+"/via-policy.txt", []byte("policy"), ownerAKID, ownerSecret, "s3", now, nil)

	bucketPolicy := []byte(fmt.Sprintf(
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"%s"},"Action":"s3:GetObject","Resource":"arn:aws:s3:::%s/*"}]}`,
		callerUserARN, bucket,
	))
	polRec := mustS3WithCreds(t, handler, http.MethodPut, "http://127.0.0.1:4566/"+bucket+"?policy", bucketPolicy, ownerAKID, ownerSecret, "s3", now, map[string]string{
		"Content-Type": "application/json",
	})
	if polRec.Code != http.StatusOK {
		t.Fatalf("PutBucketPolicy status=%d body=%q", polRec.Code, polRec.Body.String())
	}

	getRec := mustS3WithCreds(t, handler, http.MethodGet, "http://127.0.0.1:4566/"+bucket+"/via-policy.txt", nil, callerAKID, callerSecret, "s3", now, nil)
	if getRec.Code != http.StatusForbidden {
		t.Fatalf("GetObject status=%d want 403 body=%q", getRec.Code, getRec.Body.String())
	}
}

func TestSQSCrossAccountIdentityAndQueuePolicyAllow(t *testing.T) {
	handler, st, ownerAccount, ownerAKID, ownerSecret, _, callerAKID, callerSecret, callerUserARN := setupCrossAccountPair(t)
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustSQSJSONWithCreds(t, handler, "CreateQueue", map[string]any{
		"QueueName": "xa-sqs-queue",
	}, ownerAKID, ownerSecret, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateQueue status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	var createOut map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &createOut); err != nil {
		t.Fatal(err)
	}
	queueURL, _ := createOut["QueueUrl"].(string)
	queueARN := store.QueueARN(testRegion, ownerAccount, "xa-sqs-queue")

	identityAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"sqs:ReceiveMessage","Resource":"*"}]}`
	if err := st.PutInlinePolicy(callerUserARN, "sqsrecv", identityAllow); err != nil {
		t.Fatal(err)
	}

	queuePolicy := fmt.Sprintf(
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"%s"},"Action":"sqs:ReceiveMessage","Resource":"%s"}]}`,
		callerUserARN, queueARN,
	)
	setRec := mustSQSJSONWithCreds(t, handler, "SetQueueAttributes", map[string]any{
		"QueueUrl": queueURL,
		"Attributes": map[string]string{
			"Policy": queuePolicy,
		},
	}, ownerAKID, ownerSecret, now)
	if setRec.Code != http.StatusOK {
		t.Fatalf("SetQueueAttributes status=%d body=%q", setRec.Code, setRec.Body.String())
	}

	sendRec := mustSQSJSONWithCreds(t, handler, "SendMessage", map[string]any{
		"QueueUrl":    queueURL,
		"MessageBody": "xa-msg",
	}, ownerAKID, ownerSecret, now)
	if sendRec.Code != http.StatusOK {
		t.Fatalf("SendMessage status=%d body=%q", sendRec.Code, sendRec.Body.String())
	}

	recvRec := mustSQSJSONWithCreds(t, handler, "ReceiveMessage", map[string]any{
		"QueueUrl":            queueURL,
		"MaxNumberOfMessages": 1,
	}, callerAKID, callerSecret, now)
	if recvRec.Code != http.StatusOK {
		t.Fatalf("ReceiveMessage status=%d want 200 body=%q", recvRec.Code, recvRec.Body.String())
	}
	var recvOut map[string]any
	if err := json.Unmarshal(recvRec.Body.Bytes(), &recvOut); err != nil {
		t.Fatal(err)
	}
	msgs, _ := recvOut["Messages"].([]any)
	if len(msgs) != 1 {
		t.Fatalf("Messages len=%d body=%q", len(msgs), recvRec.Body.String())
	}
}

func TestSQSCrossAccountIdentityOnlyDeny(t *testing.T) {
	handler, st, _, ownerAKID, ownerSecret, _, callerAKID, callerSecret, callerUserARN := setupCrossAccountPair(t)
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustSQSJSONWithCreds(t, handler, "CreateQueue", map[string]any{
		"QueueName": "xa-sqs-id-only",
	}, ownerAKID, ownerSecret, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateQueue status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	var createOut map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &createOut); err != nil {
		t.Fatal(err)
	}
	queueURL, _ := createOut["QueueUrl"].(string)

	identityAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"sqs:SendMessage","Resource":"*"}]}`
	if err := st.PutInlinePolicy(callerUserARN, "sqssend", identityAllow); err != nil {
		t.Fatal(err)
	}

	sendRec := mustSQSJSONWithCreds(t, handler, "SendMessage", map[string]any{
		"QueueUrl":    queueURL,
		"MessageBody": "nope",
	}, callerAKID, callerSecret, now)
	if sendRec.Code != http.StatusForbidden {
		t.Fatalf("SendMessage status=%d want 403 body=%q", sendRec.Code, sendRec.Body.String())
	}
	if !strings.Contains(sendRec.Body.String(), "AccessDenied") {
		t.Fatalf("expected AccessDenied in %q", sendRec.Body.String())
	}
}

func TestSQSCrossAccountPolicyOnlyDeny(t *testing.T) {
	handler, _, ownerAccount, ownerAKID, ownerSecret, _, callerAKID, callerSecret, callerUserARN := setupCrossAccountPair(t)
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustSQSJSONWithCreds(t, handler, "CreateQueue", map[string]any{
		"QueueName": "xa-sqs-pol-only",
	}, ownerAKID, ownerSecret, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateQueue status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	var createOut map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &createOut); err != nil {
		t.Fatal(err)
	}
	queueURL, _ := createOut["QueueUrl"].(string)
	queueARN := store.QueueARN(testRegion, ownerAccount, "xa-sqs-pol-only")

	queuePolicy := fmt.Sprintf(
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"%s"},"Action":"sqs:SendMessage","Resource":"%s"}]}`,
		callerUserARN, queueARN,
	)
	setRec := mustSQSJSONWithCreds(t, handler, "SetQueueAttributes", map[string]any{
		"QueueUrl": queueURL,
		"Attributes": map[string]string{
			"Policy": queuePolicy,
		},
	}, ownerAKID, ownerSecret, now)
	if setRec.Code != http.StatusOK {
		t.Fatalf("SetQueueAttributes status=%d body=%q", setRec.Code, setRec.Body.String())
	}

	sendRec := mustSQSJSONWithCreds(t, handler, "SendMessage", map[string]any{
		"QueueUrl":    queueURL,
		"MessageBody": "nope",
	}, callerAKID, callerSecret, now)
	if sendRec.Code != http.StatusForbidden {
		t.Fatalf("SendMessage status=%d want 403 body=%q", sendRec.Code, sendRec.Body.String())
	}
}
