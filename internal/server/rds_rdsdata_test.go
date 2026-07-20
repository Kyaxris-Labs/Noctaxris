package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func mustRDSForm(t *testing.T, handler http.Handler, values url.Values, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	body := []byte(values.Encode())
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	signHeader(t, req, body, testAccessKey, testSecret, testRegion, "rds", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestRDSCreateDescribeDelete(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustRDSForm(t, handler, url.Values{
		"Action":               {"CreateDBInstance"},
		"Version":              {"2014-10-31"},
		"DBInstanceIdentifier": {"labpg1"},
		"Engine":               {"postgres"},
		"DBInstanceClass":      {"db.t3.micro"},
		"MasterUsername":       {"postgres"},
		"MasterUserPassword":   {"lab-password-1"},
		"AllocatedStorage":     {"20"},
		"DBName":               {"appdb"},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateDBInstance status=%d body=%q", create.Code, create.Body.String())
	}
	body := create.Body.String()
	if !strings.Contains(body, "labpg1") || !strings.Contains(body, "SecretArn") {
		t.Fatalf("missing fields: %s", body)
	}
	if !strings.Contains(body, "creating") && !strings.Contains(body, "available") {
		t.Fatalf("unexpected status in %s", body)
	}

	desc := mustRDSForm(t, handler, url.Values{
		"Action":               {"DescribeDBInstances"},
		"Version":              {"2014-10-31"},
		"DBInstanceIdentifier": {"labpg1"},
	}, now)
	if desc.Code != http.StatusOK || !strings.Contains(desc.Body.String(), "labpg1") {
		t.Fatalf("DescribeDBInstances status=%d body=%q", desc.Code, desc.Body.String())
	}

	list := mustRDSForm(t, handler, url.Values{
		"Action":  {"DescribeDBInstances"},
		"Version": {"2014-10-31"},
	}, now)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "labpg1") {
		t.Fatalf("list status=%d body=%q", list.Code, list.Body.String())
	}

	del := mustRDSForm(t, handler, url.Values{
		"Action":               {"DeleteDBInstance"},
		"Version":              {"2014-10-31"},
		"DBInstanceIdentifier": {"labpg1"},
	}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteDBInstance status=%d body=%q", del.Code, del.Body.String())
	}
}

func TestRDSRejectsMySQL(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)
	rec := mustRDSForm(t, handler, url.Values{
		"Action":               {"CreateDBInstance"},
		"Version":              {"2014-10-31"},
		"DBInstanceIdentifier": {"labmysql"},
		"Engine":               {"mysql"},
		"DBInstanceClass":      {"db.t3.micro"},
		"MasterUsername":       {"root"},
		"MasterUserPassword":   {"lab-password-1"},
	}, now)
	if rec.Code == http.StatusOK {
		t.Fatalf("expected rejection, got %s", rec.Body.String())
	}
}

func TestRDSDataExecuteStatementStub(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustRDSForm(t, handler, url.Values{
		"Action":               {"CreateDBInstance"},
		"Version":              {"2014-10-31"},
		"DBInstanceIdentifier": {"dataapi1"},
		"Engine":               {"postgres"},
		"DBInstanceClass":      {"db.t3.micro"},
		"MasterUsername":       {"postgres"},
		"MasterUserPassword":   {"lab-password-1"},
		"DBName":               {"postgres"},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%q", create.Code, create.Body.String())
	}
	inst, err := st.DescribeRDSDBInstance(testAccountID, "dataapi1")
	if err != nil {
		t.Fatal(err)
	}

	exec := mustJSONTarget(t, handler, "AmazonRDSDataService.ExecuteStatement", "rds-data", map[string]any{
		"resourceArn": inst.DBInstanceARN,
		"secretArn":   inst.MasterUserSecretARN,
		"database":    "postgres",
		"sql":         "SELECT 1",
	}, now)
	if exec.Code != http.StatusOK {
		t.Fatalf("ExecuteStatement status=%d body=%q", exec.Code, exec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(exec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if _, ok := out["records"]; !ok {
		t.Fatalf("missing records: %s", exec.Body.String())
	}

	badSecret := mustJSONTarget(t, handler, "AmazonRDSDataService.ExecuteStatement", "rds-data", map[string]any{
		"resourceArn": inst.DBInstanceARN,
		"secretArn":   "arn:aws:secretsmanager:us-east-1:" + testAccountID + ":secret:wrong-abcdef",
		"sql":         "SELECT 1",
	}, now)
	if badSecret.Code == http.StatusOK {
		t.Fatalf("expected secret failure, got %s", badSecret.Body.String())
	}

	begin := mustJSONTarget(t, handler, "AmazonRDSDataService.BeginTransaction", "rds-data", map[string]any{
		"resourceArn": inst.DBInstanceARN,
		"secretArn":   inst.MasterUserSecretARN,
		"database":    "postgres",
	}, now)
	if begin.Code != http.StatusOK {
		t.Fatalf("BeginTransaction status=%d body=%q", begin.Code, begin.Body.String())
	}
	var beginOut map[string]any
	_ = json.Unmarshal(begin.Body.Bytes(), &beginOut)
	txnID, _ := beginOut["transactionId"].(string)
	if txnID == "" {
		t.Fatalf("missing transactionId: %s", begin.Body.String())
	}
	commit := mustJSONTarget(t, handler, "AmazonRDSDataService.CommitTransaction", "rds-data", map[string]any{
		"resourceArn":   inst.DBInstanceARN,
		"secretArn":     inst.MasterUserSecretARN,
		"transactionId": txnID,
	}, now)
	if commit.Code != http.StatusOK {
		t.Fatalf("CommitTransaction status=%d body=%q", commit.Code, commit.Body.String())
	}
}
