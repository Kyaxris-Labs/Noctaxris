package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRDSDataExecuteRequiresGetSecretValue(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustQuery(t, handler, http.MethodPost, "/", map[string]string{
		"Action":               "CreateDBInstance",
		"Version":              "2014-10-31",
		"DBInstanceIdentifier": "rds-data-secret-authz",
		"DBInstanceClass":      "db.t3.micro",
		"Engine":               "postgres",
		"MasterUsername":       "postgres",
		"MasterUserPassword":   "lab-password-1",
		"AllocatedStorage":     "20",
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateDBInstance status=%d body=%q", create.Code, create.Body.String())
	}
	inst, err := st.DescribeRDSDBInstance(testAccountID, "rds-data-secret-authz")
	if err != nil {
		t.Fatal(err)
	}
	secretARN := strings.TrimSpace(inst.MasterUserSecretARN)
	if secretARN == "" {
		t.Fatal("expected MasterUserSecretARN")
	}

	userName := "rds-exec-no-secret"
	if _, _, err := st.CreateUser(testAccountID, userName); err != nil {
		t.Fatal(err)
	}
	akid, secret, err := st.CreateUserAccessKey(testAccountID, userName)
	if err != nil {
		t.Fatal(err)
	}
	allowRDS := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"rds-data:*","Resource":"*"}]}`
	polARN, err := st.CreateManagedPolicy(testAccountID, "RDSExecOnly", allowRDS)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachUserPolicy(testAccountID, userName, polARN); err != nil {
		t.Fatal(err)
	}

	raw, _ := json.Marshal(map[string]any{
		"resourceArn": inst.DBInstanceARN,
		"secretArn":   secretARN,
		"database":    "postgres",
		"sql":         "SELECT 1",
	})
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "AmazonRDSDataService.ExecuteStatement")
	signHeader(t, req, raw, akid, secret, testRegion, "rds-data", now)
	denyRec := httptest.NewRecorder()
	handler.ServeHTTP(denyRec, req)
	if denyRec.Code != http.StatusForbidden || !strings.Contains(denyRec.Body.String(), "secretsmanager:GetSecretValue") {
		t.Fatalf("ExecuteStatement without GetSecretValue status=%d body=%q", denyRec.Code, denyRec.Body.String())
	}

	allowSecret := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"secretsmanager:GetSecretValue","Resource":"*"}]}`
	secPol, err := st.CreateManagedPolicy(testAccountID, "RDSExecSecret", allowSecret)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachUserPolicy(testAccountID, userName, secPol); err != nil {
		t.Fatal(err)
	}
	reqOK := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	reqOK.Header.Set("Content-Type", "application/x-amz-json-1.1")
	reqOK.Header.Set("X-Amz-Target", "AmazonRDSDataService.ExecuteStatement")
	signHeader(t, reqOK, raw, akid, secret, testRegion, "rds-data", now)
	okRec := httptest.NewRecorder()
	handler.ServeHTTP(okRec, reqOK)
	// Nested compute is usually unavailable in unit tests; after GetSecretValue Allow
	// the call may proceed to DatabaseUnavailableException rather than AccessDenied.
	if okRec.Code == http.StatusForbidden && strings.Contains(okRec.Body.String(), "secretsmanager:GetSecretValue") {
		t.Fatalf("unexpected GetSecretValue deny after Allow: %q", okRec.Body.String())
	}
}
