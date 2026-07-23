package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
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

func TestRDSDataExecuteStatementUnavailableAndStubOverride(t *testing.T) {
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

	unavailable := mustJSONTarget(t, handler, "AmazonRDSDataService.ExecuteStatement", "rds-data", map[string]any{
		"resourceArn": inst.DBInstanceARN,
		"secretArn":   inst.MasterUserSecretARN,
		"database":    "postgres",
		"sql":         "SELECT 1",
	}, now)
	if unavailable.Code != http.StatusGatewayTimeout || !strings.Contains(unavailable.Body.String(), "DatabaseUnavailableException") {
		t.Fatalf("want DatabaseUnavailableException, got status=%d body=%q", unavailable.Code, unavailable.Body.String())
	}

	withParams := mustJSONTarget(t, handler, "AmazonRDSDataService.ExecuteStatement", "rds-data", map[string]any{
		"resourceArn": inst.DBInstanceARN,
		"secretArn":   inst.MasterUserSecretARN,
		"database":    "postgres",
		"sql":         "SELECT :id",
		"parameters": []any{
			map[string]any{
				"name": "id",
				"value": map[string]any{
					"longValue": float64(1),
				},
			},
		},
	}, now)
	// No nested container: parameters still fail closed as unavailable (pgx dial + psql).
	if withParams.Code != http.StatusGatewayTimeout || !strings.Contains(withParams.Body.String(), "DatabaseUnavailableException") {
		t.Fatalf("want DatabaseUnavailableException with parameters, got status=%d body=%q", withParams.Code, withParams.Body.String())
	}

	srv.SetRDSDataExecutor(&store.StubRDSDataExecutor{})
	t.Cleanup(func() { srv.SetRDSDataExecutor(nil) })

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
	if begin.Code != http.StatusNotImplemented {
		t.Fatalf("BeginTransaction status=%d want 501 body=%q", begin.Code, begin.Body.String())
	}
	commit := mustJSONTarget(t, handler, "AmazonRDSDataService.CommitTransaction", "rds-data", map[string]any{
		"transactionId": "txn-does-not-exist",
	}, now)
	if commit.Code != http.StatusNotImplemented {
		t.Fatalf("CommitTransaction status=%d want 501 body=%q", commit.Code, commit.Body.String())
	}
	withTxn := mustJSONTarget(t, handler, "AmazonRDSDataService.ExecuteStatement", "rds-data", map[string]any{
		"resourceArn":   inst.DBInstanceARN,
		"secretArn":     inst.MasterUserSecretARN,
		"sql":           "SELECT 1",
		"transactionId": "txn-ignored",
	}, now)
	if withTxn.Code != http.StatusNotImplemented {
		t.Fatalf("Execute with transactionId status=%d want 501 body=%q", withTxn.Code, withTxn.Body.String())
	}
}
