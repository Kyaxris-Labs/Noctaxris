package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const policyContextClientIP = "203.0.113.10"

func mustSignedJSONRemote(
	t *testing.T,
	handler http.Handler,
	service, amzTarget, contentType string,
	payload map[string]any,
	akid, secret, remoteAddr string,
	now time.Time,
) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.RemoteAddr = remoteAddr
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("X-Amz-Target", amzTarget)
	signHeader(t, req, raw, akid, secret, testRegion, service, now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

// TestRequestContextSourceIpAndResourceTagPopulated proves two previously empty
// catalog keys (aws:SourceIp, aws:ResourceTag/env) are populated on a dataplane
// authorize path and can Allow a conditioned identity policy.
func TestRequestContextSourceIpAndResourceTagPopulated(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	_, userARN, err := st.CreateUser(testAccountID, "ctx-user")
	if err != nil {
		t.Fatal(err)
	}
	userAKID, userSecret, err := st.CreateUserAccessKey(testAccountID, "ctx-user")
	if err != nil {
		t.Fatal(err)
	}

	createRec := mustSQSJSON(t, handler, "CreateQueue", map[string]any{
		"QueueName": "ctx-policy-q",
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateQueue status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	var createOut map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &createOut); err != nil {
		t.Fatal(err)
	}
	queueURL, _ := createOut["QueueUrl"].(string)
	if queueURL == "" {
		t.Fatalf("missing QueueUrl in %q", createRec.Body.String())
	}
	queueARN := fmt.Sprintf("arn:aws:sqs:%s:%s:ctx-policy-q", testRegion, testAccountID)

	if _, err := st.TagResources(testAccountID, []string{queueARN}, map[string]string{"env": "lab"}); err != nil {
		t.Fatal(err)
	}

	policy := `{
		"Version":"2012-10-17",
		"Statement":[{
			"Effect":"Allow",
			"Action":"sqs:SendMessage",
			"Resource":"*",
			"Condition":{"StringEquals":{
				"aws:SourceIp":"203.0.113.10",
				"aws:ResourceTag/env":"lab"
			}}
		}]
	}`
	if err := st.PutInlinePolicy(userARN, "ctx-send", policy); err != nil {
		t.Fatal(err)
	}

	allowRec := mustSignedJSONRemote(t, handler, "sqs", "AmazonSQS.SendMessage", "application/x-amz-json-1.0",
		map[string]any{"QueueUrl": queueURL, "MessageBody": "ok"},
		userAKID, userSecret, policyContextClientIP+":54321", now)
	if allowRec.Code != http.StatusOK {
		t.Fatalf("SendMessage allow status=%d body=%q", allowRec.Code, allowRec.Body.String())
	}

	denyIP := mustSignedJSONRemote(t, handler, "sqs", "AmazonSQS.SendMessage", "application/x-amz-json-1.0",
		map[string]any{"QueueUrl": queueURL, "MessageBody": "bad-ip"},
		userAKID, userSecret, "198.51.100.9:54321", now)
	if denyIP.Code != http.StatusForbidden {
		t.Fatalf("SendMessage wrong IP status=%d want 403 body=%q", denyIP.Code, denyIP.Body.String())
	}
}

func TestRequestContextECRResourceTagPopulated(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	_, userARN, err := st.CreateUser(testAccountID, "ecr-ctx-user")
	if err != nil {
		t.Fatal(err)
	}
	userAKID, userSecret, err := st.CreateUserAccessKey(testAccountID, "ecr-ctx-user")
	if err != nil {
		t.Fatal(err)
	}

	createRec := mustECRJSON(t, handler, "CreateRepository", map[string]any{
		"repositoryName": "ctx-tagged-repo",
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateRepository status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	repoARN := "arn:aws:ecr:us-east-1:000000000001:repository/ctx-tagged-repo"
	if _, err := st.TagResources(testAccountID, []string{repoARN}, map[string]string{"env": "lab"}); err != nil {
		t.Fatal(err)
	}

	policy := `{
		"Version":"2012-10-17",
		"Statement":[{
			"Effect":"Allow",
			"Action":"ecr:DescribeRepositories",
			"Resource":"*",
			"Condition":{"StringEquals":{"ecr:ResourceTag/env":"lab"}}
		}]
	}`
	if err := st.PutInlinePolicy(userARN, "ecr-ctx", policy); err != nil {
		t.Fatal(err)
	}

	allowRec := mustECRJSONWithCreds(t, handler, "DescribeRepositories", map[string]any{
		"repositoryNames": []string{"ctx-tagged-repo"},
	}, userAKID, userSecret, now)
	if allowRec.Code != http.StatusOK {
		t.Fatalf("DescribeRepositories allow status=%d body=%q", allowRec.Code, allowRec.Body.String())
	}

	if _, err := st.UntagResources(testAccountID, []string{repoARN}, []string{"env"}); err != nil {
		t.Fatal(err)
	}
	denyRec := mustECRJSONWithCreds(t, handler, "DescribeRepositories", map[string]any{
		"repositoryNames": []string{"ctx-tagged-repo"},
	}, userAKID, userSecret, now)
	if denyRec.Code != http.StatusForbidden {
		t.Fatalf("DescribeRepositories untagged status=%d want 403 body=%q", denyRec.Code, denyRec.Body.String())
	}
}
