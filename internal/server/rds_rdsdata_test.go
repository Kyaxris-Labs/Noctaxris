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

func TestRDSAcceptsMySQLAndMariaDBRejectsUnknown(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mysql := mustRDSForm(t, handler, url.Values{
		"Action":               {"CreateDBInstance"},
		"Version":              {"2014-10-31"},
		"DBInstanceIdentifier": {"labmysql"},
		"Engine":               {"mysql"},
		"DBInstanceClass":      {"db.t3.micro"},
		"MasterUsername":       {"root"},
		"MasterUserPassword":   {"lab-password-1"},
	}, now)
	if mysql.Code != http.StatusOK {
		t.Fatalf("expected mysql accept, got %d %s", mysql.Code, mysql.Body.String())
	}
	if !strings.Contains(mysql.Body.String(), "mysql") {
		t.Fatalf("missing engine in %s", mysql.Body.String())
	}

	maria := mustRDSForm(t, handler, url.Values{
		"Action":               {"CreateDBInstance"},
		"Version":              {"2014-10-31"},
		"DBInstanceIdentifier": {"labmaria"},
		"Engine":               {"mariadb"},
		"DBInstanceClass":      {"db.t3.micro"},
		"MasterUsername":       {"root"},
		"MasterUserPassword":   {"lab-password-1"},
	}, now)
	if maria.Code != http.StatusOK {
		t.Fatalf("expected mariadb accept, got %d %s", maria.Code, maria.Body.String())
	}

	reject := mustRDSForm(t, handler, url.Values{
		"Action":               {"CreateDBInstance"},
		"Version":              {"2014-10-31"},
		"DBInstanceIdentifier": {"laboracle"},
		"Engine":               {"oracle-ee"},
		"DBInstanceClass":      {"db.t3.micro"},
		"MasterUsername":       {"admin"},
		"MasterUserPassword":   {"lab-password-1"},
	}, now)
	if reject.Code == http.StatusOK {
		t.Fatalf("expected rejection, got %s", reject.Body.String())
	}

	for _, id := range []string{"labmysql", "labmaria"} {
		del := mustRDSForm(t, handler, url.Values{
			"Action":               {"DeleteDBInstance"},
			"Version":              {"2014-10-31"},
			"DBInstanceIdentifier": {id},
		}, now)
		if del.Code != http.StatusOK {
			t.Fatalf("DeleteDBInstance %s status=%d body=%q", id, del.Code, del.Body.String())
		}
	}
}

func TestRDSDataMySQLEngineUnavailableWithoutNested(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustRDSForm(t, handler, url.Values{
		"Action":               {"CreateDBInstance"},
		"Version":              {"2014-10-31"},
		"DBInstanceIdentifier": {"dataapi-mysql"},
		"Engine":               {"mysql"},
		"DBInstanceClass":      {"db.t3.micro"},
		"MasterUsername":       {"root"},
		"MasterUserPassword":   {"lab-password-1"},
		"DBName":               {"appdb"},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%q", create.Code, create.Body.String())
	}
	inst, err := st.DescribeRDSDBInstance(testAccountID, "dataapi-mysql")
	if err != nil {
		t.Fatal(err)
	}
	_ = st.UpdateRDSDBInstanceRuntime(testAccountID, "dataapi-mysql", "available", "cid-mysql", "noctaxris-data-rds-dataapi-mysql", 3306)

	exec := mustJSONTarget(t, handler, "AmazonRDSDataService.ExecuteStatement", "rds-data", map[string]any{
		"resourceArn": inst.DBInstanceARN,
		"secretArn":   inst.MasterUserSecretARN,
		"database":    "appdb",
		"sql":         "SELECT 1",
	}, now)
	if exec.Code != http.StatusGatewayTimeout || !strings.Contains(exec.Body.String(), "DatabaseUnavailableException") {
		t.Fatalf("want DatabaseUnavailableException for mysql without reachable nested engine, got status=%d body=%s", exec.Code, exec.Body.String())
	}

	begin := mustJSONTarget(t, handler, "AmazonRDSDataService.BeginTransaction", "rds-data", map[string]any{
		"resourceArn": inst.DBInstanceARN,
		"secretArn":   inst.MasterUserSecretARN,
		"database":    "appdb",
	}, now)
	if begin.Code != http.StatusGatewayTimeout && begin.Code != http.StatusBadRequest {
		t.Fatalf("BeginTransaction mysql status=%d body=%s", begin.Code, begin.Body.String())
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
	if begin.Code == http.StatusNotImplemented {
		t.Fatalf("Begin must not stay 501; got %d body=%s", begin.Code, begin.Body.String())
	}
	if begin.Code != http.StatusGatewayTimeout && begin.Code != http.StatusBadRequest {
		t.Fatalf("BeginTransaction status=%d body=%s", begin.Code, begin.Body.String())
	}
	if !strings.Contains(begin.Body.String(), "DatabaseUnavailableException") &&
		!strings.Contains(begin.Body.String(), "Unavailable") {
		t.Fatalf("BeginTransaction body=%s", begin.Body.String())
	}
	commit := mustJSONTarget(t, handler, "AmazonRDSDataService.CommitTransaction", "rds-data", map[string]any{
		"resourceArn":   inst.DBInstanceARN,
		"secretArn":     inst.MasterUserSecretARN,
		"transactionId": "txn-does-not-exist",
	}, now)
	if commit.Code == http.StatusNotImplemented {
		t.Fatalf("CommitTransaction must not stay 501; body=%q", commit.Body.String())
	}
	if commit.Code != http.StatusNotFound || !strings.Contains(commit.Body.String(), "TransactionNotFoundException") {
		t.Fatalf("CommitTransaction status=%d body=%q", commit.Code, commit.Body.String())
	}
	withTxn := mustJSONTarget(t, handler, "AmazonRDSDataService.ExecuteStatement", "rds-data", map[string]any{
		"resourceArn":   inst.DBInstanceARN,
		"secretArn":     inst.MasterUserSecretARN,
		"sql":           "SELECT 1",
		"transactionId": "txn-ignored",
	}, now)
	if withTxn.Code == http.StatusNotImplemented {
		t.Fatalf("Execute with transactionId must not stay 501; body=%q", withTxn.Body.String())
	}
	if withTxn.Code != http.StatusNotFound || !strings.Contains(withTxn.Body.String(), "TransactionNotFoundException") {
		t.Fatalf("Execute with transactionId status=%d body=%q", withTxn.Code, withTxn.Body.String())
	}
}

func TestRDSDataBeginUnavailableNot501(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustRDSForm(t, handler, url.Values{
		"Action":               {"CreateDBInstance"},
		"Version":              {"2014-10-31"},
		"DBInstanceIdentifier": {"dataapi-begin"},
		"Engine":               {"postgres"},
		"DBInstanceClass":      {"db.t3.micro"},
		"MasterUsername":       {"postgres"},
		"MasterUserPassword":   {"lab-password-1"},
		"DBName":               {"postgres"},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%q", create.Code, create.Body.String())
	}
	inst, err := st.DescribeRDSDBInstance(testAccountID, "dataapi-begin")
	if err != nil {
		t.Fatal(err)
	}

	begin := mustJSONTarget(t, handler, "AmazonRDSDataService.BeginTransaction", "rds-data", map[string]any{
		"resourceArn": inst.DBInstanceARN,
		"secretArn":   inst.MasterUserSecretARN,
		"database":    "postgres",
	}, now)
	if begin.Code == http.StatusNotImplemented {
		t.Fatalf("Begin must not stay 501; got %d body=%s", begin.Code, begin.Body.String())
	}
	if begin.Code != http.StatusGatewayTimeout && begin.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", begin.Code, begin.Body.String())
	}
	if !strings.Contains(begin.Body.String(), "DatabaseUnavailableException") &&
		!strings.Contains(begin.Body.String(), "Unavailable") {
		t.Fatalf("body=%s", begin.Body.String())
	}
}

func TestRDSDataBatchExecuteUnavailable(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustRDSForm(t, handler, url.Values{
		"Action":               {"CreateDBInstance"},
		"Version":              {"2014-10-31"},
		"DBInstanceIdentifier": {"dataapi-batch"},
		"Engine":               {"postgres"},
		"DBInstanceClass":      {"db.t3.micro"},
		"MasterUsername":       {"postgres"},
		"MasterUserPassword":   {"lab-password-1"},
		"DBName":               {"postgres"},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%q", create.Code, create.Body.String())
	}
	inst, err := st.DescribeRDSDBInstance(testAccountID, "dataapi-batch")
	if err != nil {
		t.Fatal(err)
	}

	batch := mustJSONTarget(t, handler, "AmazonRDSDataService.BatchExecuteStatement", "rds-data", map[string]any{
		"resourceArn":   inst.DBInstanceARN,
		"secretArn":     inst.MasterUserSecretARN,
		"database":      "postgres",
		"sql":           "SELECT 1",
		"parameterSets": []any{[]any{}, []any{}},
	}, now)
	if batch.Code == http.StatusNotImplemented {
		t.Fatal("BatchExecuteStatement must be wired")
	}
	if batch.Code != http.StatusGatewayTimeout || !strings.Contains(batch.Body.String(), "DatabaseUnavailableException") {
		t.Fatalf("want DatabaseUnavailableException, got status=%d body=%q", batch.Code, batch.Body.String())
	}

	empty := mustJSONTarget(t, handler, "AmazonRDSDataService.BatchExecuteStatement", "rds-data", map[string]any{
		"resourceArn":   inst.DBInstanceARN,
		"secretArn":     inst.MasterUserSecretARN,
		"database":      "postgres",
		"sql":           "SELECT 1",
		"parameterSets": []any{},
	}, now)
	if empty.Code != http.StatusBadRequest || !strings.Contains(empty.Body.String(), "BadRequestException") {
		t.Fatalf("empty parameterSets status=%d body=%q", empty.Code, empty.Body.String())
	}
}

func TestRDSDataExecuteStatementRPCPath(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustRDSForm(t, handler, url.Values{
		"Action":               {"CreateDBInstance"},
		"Version":              {"2014-10-31"},
		"DBInstanceIdentifier": {"dataapi-rpc"},
		"Engine":               {"postgres"},
		"DBInstanceClass":      {"db.t3.micro"},
		"MasterUsername":       {"postgres"},
		"MasterUserPassword":   {"lab-password-1"},
		"DBName":               {"postgres"},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%q", create.Code, create.Body.String())
	}
	inst, err := st.DescribeRDSDBInstance(testAccountID, "dataapi-rpc")
	if err != nil {
		t.Fatal(err)
	}

	payload := map[string]any{
		"resourceArn": inst.DBInstanceARN,
		"secretArn":   inst.MasterUserSecretARN,
		"database":    "postgres",
		"sql":         "SELECT 1",
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/Execute", raw)
	req.Header.Set("Content-Type", "application/json")
	signHeader(t, req, raw, testAccessKey, testSecret, testRegion, "rds-data", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code == http.StatusNotImplemented || strings.Contains(rec.Body.String(), "not implemented") {
		t.Fatalf("RPC /Execute must resolve ExecuteStatement, got status=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Code != http.StatusGatewayTimeout || !strings.Contains(rec.Body.String(), "DatabaseUnavailableException") {
		t.Fatalf("want DatabaseUnavailableException, got status=%d body=%q", rec.Code, rec.Body.String())
	}
}
