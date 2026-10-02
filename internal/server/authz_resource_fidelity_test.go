package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func mustJSONTargetCreds(
	t *testing.T,
	handler http.Handler,
	target, service string,
	payload map[string]any,
	now time.Time,
	akid, secret string,
) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", target)
	signHeader(t, req, raw, akid, secret, testRegion, service, now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestCognitoDescribeRequiresPoolResource(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	pool := cognitoMustOK(t, handler, "CreateUserPool", map[string]any{"PoolName": "res-pool"}, now)
	up, _ := pool["UserPool"].(map[string]any)
	poolID, _ := up["Id"].(string)
	poolARN := store.CognitoPoolARN(testRegion, testAccountID, poolID)

	userName := "cognito-pool-scoped"
	if _, _, err := st.CreateUser(testAccountID, userName); err != nil {
		t.Fatal(err)
	}
	akid, secret, err := st.CreateUserAccessKey(testAccountID, userName)
	if err != nil {
		t.Fatal(err)
	}
	allowOther := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"cognito-idp:DescribeUserPool","Resource":"arn:aws:cognito-idp:` + testRegion + `:` + testAccountID + `:userpool/other-pool"}]}`
	polARN, err := st.CreateManagedPolicy(testAccountID, "CognitoOtherPoolOnly", allowOther)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachUserPolicy(testAccountID, userName, polARN); err != nil {
		t.Fatal(err)
	}

	deny := mustJSONTargetCreds(t, handler, "AWSCognitoIdentityProviderService.DescribeUserPool", "cognito-idp", map[string]any{
		"UserPoolId": poolID,
	}, now, akid, secret)
	if deny.Code != http.StatusForbidden {
		t.Fatalf("Describe wrong pool resource status=%d body=%q", deny.Code, deny.Body.String())
	}

	allowExact := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"cognito-idp:DescribeUserPool","Resource":"` + poolARN + `"}]}`
	exactARN, err := st.CreateManagedPolicy(testAccountID, "CognitoExactPool", allowExact)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachUserPolicy(testAccountID, userName, exactARN); err != nil {
		t.Fatal(err)
	}
	ok := mustJSONTargetCreds(t, handler, "AWSCognitoIdentityProviderService.DescribeUserPool", "cognito-idp", map[string]any{
		"UserPoolId": poolID,
	}, now, akid, secret)
	if ok.Code != http.StatusForbidden && ok.Code != http.StatusOK {
		t.Fatalf("Describe exact pool unexpected status=%d body=%q", ok.Code, ok.Body.String())
	}
	// Multiple attached managed policies OR together: exact Allow should succeed.
	if ok.Code != http.StatusOK {
		t.Fatalf("Describe exact pool status=%d body=%q", ok.Code, ok.Body.String())
	}
}

func TestS3PutObjectAclAndRetentionRequireActions(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	userName := "s3-acl-ret"
	if _, _, err := st.CreateUser(testAccountID, userName); err != nil {
		t.Fatal(err)
	}
	akid, secret, err := st.CreateUserAccessKey(testAccountID, userName)
	if err != nil {
		t.Fatal(err)
	}
	allowPutOnly := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:CreateBucket","s3:PutObject"],"Resource":"*"}]}`
	polARN, err := st.CreateManagedPolicy(testAccountID, "S3PutOnly", allowPutOnly)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachUserPolicy(testAccountID, userName, polARN); err != nil {
		t.Fatal(err)
	}
	bucket := "acl-ret-bucket"
	if _, err := st.CreateBucketWithOptions(testAccountID, bucket, store.CreateBucketOptions{ObjectLockEnabled: true}); err != nil {
		t.Fatal(err)
	}

	body := []byte("hello")
	aclReq := mustNewRequest(t, http.MethodPut, "http://127.0.0.1:4566/"+bucket+"/acl.txt", body)
	aclReq.Header.Set("Content-Type", "text/plain")
	aclReq.Header.Set("x-amz-acl", "public-read")
	signS3Header(t, aclReq, body, akid, secret, testRegion, "s3", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, aclReq)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("PutObject ACL without PutObjectAcl status=%d body=%q", rec.Code, rec.Body.String())
	}

	retain := now.Add(24 * time.Hour).UTC().Format(time.RFC3339)
	lockReq := mustNewRequest(t, http.MethodPut, "http://127.0.0.1:4566/"+bucket+"/lock.txt", body)
	lockReq.Header.Set("Content-Type", "text/plain")
	lockReq.Header.Set("x-amz-object-lock-mode", "GOVERNANCE")
	lockReq.Header.Set("x-amz-object-lock-retain-until-date", retain)
	signS3Header(t, lockReq, body, akid, secret, testRegion, "s3", now)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, lockReq)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("PutObject retention without PutObjectRetention status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestS3SelectObjectContentRequiresGetObject(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	bucket := "select-authz"
	if _, err := st.CreateBucket(testAccountID, bucket); err != nil {
		t.Fatal(err)
	}
	csv := []byte("a,b\n1,2\n")
	if _, err := st.PutObject(testAccountID, bucket, "rows.csv", store.PutObjectMeta{
		Data: csv, PlainSize: int64(len(csv)), ContentType: "text/csv",
	}); err != nil {
		t.Fatal(err)
	}

	userName := "s3-select-only"
	if _, _, err := st.CreateUser(testAccountID, userName); err != nil {
		t.Fatal(err)
	}
	akid, secret, err := st.CreateUserAccessKey(testAccountID, userName)
	if err != nil {
		t.Fatal(err)
	}
	allowSelect := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:SelectObjectContent","Resource":"*"}]}`
	polARN, err := st.CreateManagedPolicy(testAccountID, "S3SelectOnly", allowSelect)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachUserPolicy(testAccountID, userName, polARN); err != nil {
		t.Fatal(err)
	}

	selectBody := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<SelectObjectContentRequest>
  <Expression>SELECT * FROM S3Object</Expression>
  <ExpressionType>SQL</ExpressionType>
  <InputSerialization><CSV><FileHeaderInfo>USE</FileHeaderInfo></CSV></InputSerialization>
  <OutputSerialization><CSV></CSV></OutputSerialization>
</SelectObjectContentRequest>`)
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/"+bucket+"/rows.csv?select&select-type=2", selectBody)
	req.Header.Set("Content-Type", "application/xml")
	signS3Header(t, req, selectBody, akid, secret, testRegion, "s3", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("Select without GetObject status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestDynamoPartiQLRequiresExecuteStatement(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	tableName := "partiql-authz"
	mustDynamoJSON(t, handler, "CreateTable", map[string]any{
		"TableName": tableName,
		"AttributeDefinitions": []map[string]any{
			{"AttributeName": "pk", "AttributeType": "S"},
		},
		"KeySchema": []map[string]any{
			{"AttributeName": "pk", "KeyType": "HASH"},
		},
		"BillingMode": "PAY_PER_REQUEST",
	}, now)

	userName := "ddb-put-only"
	if _, _, err := st.CreateUser(testAccountID, userName); err != nil {
		t.Fatal(err)
	}
	akid, secret, err := st.CreateUserAccessKey(testAccountID, userName)
	if err != nil {
		t.Fatal(err)
	}
	allowPut := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["dynamodb:PutItem","dynamodb:DescribeTable"],"Resource":"*"}]}`
	polARN, err := st.CreateManagedPolicy(testAccountID, "DDBPutOnly", allowPut)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachUserPolicy(testAccountID, userName, polARN); err != nil {
		t.Fatal(err)
	}

	ins := mustDynamoJSONWithCreds(t, handler, "ExecuteStatement", map[string]any{
		"Statement": `INSERT INTO "` + tableName + `" VALUE {'pk': 'a'}`,
	}, akid, secret, now)
	if ins.Code != http.StatusForbidden {
		t.Fatalf("PartiQL INSERT with PutItem-only status=%d body=%q", ins.Code, ins.Body.String())
	}
}

func TestFirehosePutRecordUsesStreamARN(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	if _, err := st.CreateBucket(testAccountID, "fh-dest"); err != nil {
		t.Fatal(err)
	}
	policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"firehose.amazonaws.com"},"Action":"s3:PutObject","Resource":"*"}]}`
	if err := st.PutBucketPolicy(testAccountID, "fh-dest", policy); err != nil {
		t.Fatal(err)
	}
	roleARN, err := st.CreateRole(testAccountID, "fh-role", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"firehose.amazonaws.com"},"Action":"sts:AssumeRole"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateFirehoseStream(testAccountID, testRegion, "fh-res", roleARN, "S3", "fh-dest", "", ""); err != nil {
		t.Fatal(err)
	}
	streamARN := store.FirehoseStreamARN(testRegion, testAccountID, "fh-res")

	userName := "fh-other-stream"
	if _, _, err := st.CreateUser(testAccountID, userName); err != nil {
		t.Fatal(err)
	}
	akid, secret, err := st.CreateUserAccessKey(testAccountID, userName)
	if err != nil {
		t.Fatal(err)
	}
	allowOther := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"firehose:PutRecord","Resource":"arn:aws:firehose:` + testRegion + `:` + testAccountID + `:deliverystream/other"}]}`
	polARN, err := st.CreateManagedPolicy(testAccountID, "FHOther", allowOther)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachUserPolicy(testAccountID, userName, polARN); err != nil {
		t.Fatal(err)
	}
	deny := mustJSONTargetCreds(t, handler, "Firehose_20150804.PutRecord", "firehose", map[string]any{
		"DeliveryStreamName": "fh-res",
		"Record":             map[string]any{"Data": "aGVsbG8="},
	}, now, akid, secret)
	if deny.Code != http.StatusForbidden {
		t.Fatalf("PutRecord wrong stream status=%d body=%q", deny.Code, deny.Body.String())
	}

	allowExact := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"firehose:PutRecord","Resource":"` + streamARN + `"}]}`
	exactARN, err := st.CreateManagedPolicy(testAccountID, "FHExact", allowExact)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachUserPolicy(testAccountID, userName, exactARN); err != nil {
		t.Fatal(err)
	}
	ok := mustJSONTargetCreds(t, handler, "Firehose_20150804.PutRecord", "firehose", map[string]any{
		"DeliveryStreamName": "fh-res",
		"Record":             map[string]any{"Data": "aGVsbG8="},
	}, now, akid, secret)
	if ok.Code == http.StatusForbidden {
		t.Fatalf("PutRecord exact stream must not be AccessDenied status=%d body=%q", ok.Code, ok.Body.String())
	}
}

func TestElastiCacheCreateRequiresSecretsManager(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	userName := "cache-no-secrets"
	if _, _, err := st.CreateUser(testAccountID, userName); err != nil {
		t.Fatal(err)
	}
	akid, secret, err := st.CreateUserAccessKey(testAccountID, userName)
	if err != nil {
		t.Fatal(err)
	}
	allowCache := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"elasticache:CreateCacheCluster","Resource":"*"}]}`
	polARN, err := st.CreateManagedPolicy(testAccountID, "CacheOnly", allowCache)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachUserPolicy(testAccountID, userName, polARN); err != nil {
		t.Fatal(err)
	}

	body := strings.Join([]string{
		"Action=CreateCacheCluster",
		"Version=2015-02-02",
		"CacheClusterId=no-secret",
		"Engine=redis",
	}, "&")
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(body))
	signHeader(t, req, []byte(body), akid, secret, testRegion, "elasticache", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "secretsmanager") {
		t.Fatalf("CreateCacheCluster without secrets IAM status=%d body=%q", rec.Code, rec.Body.String())
	}
}
