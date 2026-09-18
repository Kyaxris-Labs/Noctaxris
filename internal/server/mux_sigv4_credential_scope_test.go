package server_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestJSON11TargetRequiresMatchingSigV4Service(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustDynamoJSON(t, handler, "CreateTable", map[string]any{
		"TableName": "mux-scope",
		"AttributeDefinitions": []map[string]any{
			{"AttributeName": "pk", "AttributeType": "S"},
		},
		"KeySchema": []map[string]any{
			{"AttributeName": "pk", "KeyType": "HASH"},
		},
		"BillingMode": "PAY_PER_REQUEST",
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateTable %d %s", create.Code, create.Body.String())
	}

	put := mustDynamoJSON(t, handler, "PutItem", map[string]any{
		"TableName": "mux-scope",
		"Item":      map[string]any{"pk": map[string]any{"S": "row-1"}},
	}, now)
	if put.Code != http.StatusOK {
		t.Fatalf("PutItem %d %s", put.Code, put.Body.String())
	}

	okGet := mustDynamoJSON(t, handler, "GetItem", map[string]any{
		"TableName": "mux-scope",
		"Key":       map[string]any{"pk": map[string]any{"S": "row-1"}},
	}, now)
	if okGet.Code != http.StatusOK || !strings.Contains(okGet.Body.String(), "row-1") {
		t.Fatalf("GetItem dynamodb %d %s", okGet.Code, okGet.Body.String())
	}

	stolen := mustJSONTarget(t, handler, "DynamoDB_20120810.GetItem", "sts", map[string]any{
		"TableName": "mux-scope",
		"Key":       map[string]any{"pk": map[string]any{"S": "row-1"}},
	}, now)
	if stolen.Code == http.StatusOK || strings.Contains(stolen.Body.String(), "row-1") {
		t.Fatalf("sts-scoped GetItem must not run DynamoDB: %d %s", stolen.Code, stolen.Body.String())
	}
	if stolen.Code != http.StatusForbidden ||
		!strings.Contains(stolen.Body.String(), "SignatureDoesNotMatch") ||
		!strings.Contains(stolen.Body.String(), "scoped to correct service") {
		t.Fatalf("sts-scoped GetItem want SignatureDoesNotMatch: %d %s", stolen.Code, stolen.Body.String())
	}

	kmsStolen := mustJSONTarget(t, handler, "TrentService.Decrypt", "sts", map[string]any{
		"CiphertextBlob": "QQ==",
	}, now)
	if kmsStolen.Code != http.StatusForbidden ||
		!strings.Contains(kmsStolen.Body.String(), "SignatureDoesNotMatch") ||
		strings.Contains(kmsStolen.Body.String(), "Ciphertext") {
		t.Fatalf("sts-scoped Decrypt must not run KMS: %d %s", kmsStolen.Code, kmsStolen.Body.String())
	}

	createUser := []byte("Action=CreateUser&Version=2010-05-08&UserName=mux-wrong-scope")
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", createUser)
	signHeader(t, req, createUser, testAccessKey, testSecret, testRegion, "sts", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK || strings.Contains(rec.Body.String(), "CreateUserResponse") {
		t.Fatalf("sts-scoped CreateUser must not run IAM: %d %s", rec.Code, rec.Body.String())
	}
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "scoped to correct service") {
		t.Fatalf("sts-scoped CreateUser want scope mismatch: %d %s", rec.Code, rec.Body.String())
	}

	iamOK := []byte("Action=CreateUser&Version=2010-05-08&UserName=mux-right-scope")
	iamReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", iamOK)
	signHeader(t, iamReq, iamOK, testAccessKey, testSecret, testRegion, "iam", now)
	iamRec := httptest.NewRecorder()
	handler.ServeHTTP(iamRec, iamReq)
	if iamRec.Code != http.StatusOK || !strings.Contains(iamRec.Body.String(), "CreateUserResponse") {
		t.Fatalf("iam-scoped CreateUser %d %s", iamRec.Code, iamRec.Body.String())
	}
}

func TestEmptyActionS3RequiresS3CredentialScope(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/mux-s3-scope", nil, "s3", now, nil)
	payload := []byte("path-style-object")
	put := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/mux-s3-scope/obj.txt", payload, "s3", now, nil)
	if put.Code != http.StatusOK {
		t.Fatalf("PutObject %d %s", put.Code, put.Body.String())
	}

	ok := mustS3(t, handler, http.MethodGet, "http://127.0.0.1:4566/mux-s3-scope/obj.txt", nil, "s3", now, nil)
	if ok.Code != http.StatusOK || ok.Body.String() != string(payload) {
		t.Fatalf("s3 GetObject %d %s", ok.Code, ok.Body.String())
	}

	stolen := mustS3(t, handler, http.MethodGet, "http://127.0.0.1:4566/mux-s3-scope/obj.txt", nil, "sts", now, nil)
	if stolen.Code == http.StatusOK || stolen.Body.String() == string(payload) {
		t.Fatalf("sts-scoped path-style must not hit handleS3: %d %s", stolen.Code, stolen.Body.String())
	}
}

func TestRESTPathMuxBindsVerifiedService(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	lambdaBody := []byte(`{}`)
	lambdaReq := mustNewRequest(t, http.MethodPost,
		"http://127.0.0.1:4566/2015-03-31/functions/mux-fn/invocations", lambdaBody)
	lambdaReq.Header.Set("Content-Type", "application/json")
	signHeader(t, lambdaReq, lambdaBody, testAccessKey, testSecret, testRegion, "s3", now)
	lambdaRec := httptest.NewRecorder()
	handler.ServeHTTP(lambdaRec, lambdaReq)
	if strings.Contains(lambdaRec.Body.String(), "ResourceNotFoundException") ||
		strings.Contains(lambdaRec.Body.String(), "FunctionName") {
		t.Fatalf("s3-scoped Lambda REST must not Invoke: %d %s", lambdaRec.Code, lambdaRec.Body.String())
	}

	eksReq := mustNewRequest(t, http.MethodGet, "http://127.0.0.1:4566/clusters", []byte{})
	eksReq.Header.Set("Content-Type", "application/json")
	signHeader(t, eksReq, []byte{}, testAccessKey, testSecret, testRegion, "s3", now)
	eksRec := httptest.NewRecorder()
	handler.ServeHTTP(eksRec, eksReq)
	if strings.Contains(eksRec.Body.String(), "clusters") && strings.Contains(eksRec.Body.String(), `"cluster"`) {
		t.Fatalf("s3-scoped EKS REST must not ListClusters: %d %s", eksRec.Code, eksRec.Body.String())
	}

	batchBody := []byte(`{"jobName":"x","jobQueue":"q","jobDefinition":"d"}`)
	batchReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/v1/submitjob", batchBody)
	batchReq.Header.Set("Content-Type", "application/json")
	signHeader(t, batchReq, batchBody, testAccessKey, testSecret, testRegion, "s3", now)
	batchRec := httptest.NewRecorder()
	handler.ServeHTTP(batchRec, batchReq)
	if strings.Contains(batchRec.Body.String(), "jobId") || strings.Contains(batchRec.Body.String(), "SubmitJob") {
		t.Fatalf("s3-scoped Batch REST must not SubmitJob: %d %s", batchRec.Code, batchRec.Body.String())
	}

	backupReq := mustNewRequest(t, http.MethodGet, "http://127.0.0.1:4566/backup-vaults", []byte{})
	backupReq.Header.Set("Content-Type", "application/json")
	signHeader(t, backupReq, []byte{}, testAccessKey, testSecret, testRegion, "s3", now)
	backupRec := httptest.NewRecorder()
	handler.ServeHTTP(backupRec, backupReq)
	if strings.Contains(backupRec.Body.String(), "BackupVaultList") ||
		strings.Contains(backupRec.Body.String(), "backupVaultName") {
		t.Fatalf("s3-scoped Backup REST must not ListBackupVaults: %d %s", backupRec.Code, backupRec.Body.String())
	}

	bedrockBody := []byte(`{"messages":[{"role":"user","content":[{"text":"hi"}]}]}`)
	bedrockReq := mustNewRequest(t, http.MethodPost,
		"http://127.0.0.1:4566/model/anthropic.claude-3-haiku-20240307-v1:0/converse", bedrockBody)
	bedrockReq.Header.Set("Content-Type", "application/json")
	signHeader(t, bedrockReq, bedrockBody, testAccessKey, testSecret, testRegion, "s3", now)
	bedrockRec := httptest.NewRecorder()
	handler.ServeHTTP(bedrockRec, bedrockReq)
	if strings.Contains(bedrockRec.Body.String(), `"assistant"`) ||
		strings.Contains(bedrockRec.Body.String(), `"output"`) {
		t.Fatalf("s3-scoped Bedrock REST must not Converse: %d %s", bedrockRec.Code, bedrockRec.Body.String())
	}

	sesBody := []byte(`{"EmailIdentity":"mux@example.com"}`)
	sesReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/v2/email/identities", sesBody)
	sesReq.Header.Set("Content-Type", "application/json")
	signHeader(t, sesReq, sesBody, testAccessKey, testSecret, testRegion, "s3", now)
	sesRec := httptest.NewRecorder()
	handler.ServeHTTP(sesRec, sesReq)
	if strings.Contains(sesRec.Body.String(), "EMAIL_ADDRESS") ||
		strings.Contains(sesRec.Body.String(), "IdentityType") {
		t.Fatalf("s3-scoped SESv2 REST must not CreateEmailIdentity: %d %s", sesRec.Code, sesRec.Body.String())
	}
}
