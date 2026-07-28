package store_test

import (
	"bytes"
	"compress/gzip"
	"errors"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

var errDuckUnavailable = errors.New("duck down")

func seedAthenaCloudTrailJSON(t *testing.T, st *store.Store, account, bucket, prefix string) {
	t.Helper()
	if _, err := st.CreateBucket(account, bucket); err != nil {
		t.Fatal(err)
	}
	payload := `{"Records":[{"eventName":"AssumeRole","eventID":"e1"},{"eventName":"PutObject","eventID":"e2"},{"eventName":"AssumeRoleWithSAML","eventID":"e3"}]}`
	key := prefix + "delivery.json"
	if _, err := st.PutObject(account, bucket, key, store.PutObjectMeta{
		Data: []byte(payload), PlainSize: int64(len(payload)),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateGlueDatabase(account, "ctdb", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateGlueTable(account, store.GlueTableCreate{
		DatabaseName:    "ctdb",
		Name:            "events",
		StorageLocation: "s3://" + bucket + "/" + prefix,
		Columns: []store.GlueColumn{
			{Name: "eventName", Type: "string"},
			{Name: "eventID", Type: "string"},
		},
		InputFormat: "org.apache.hive.hcatalog.data.JsonSerDe",
		SerDeInfo:   store.GlueSerDeInfo{SerializationLibrary: "org.openx.data.jsonserde.JsonSerDe"},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAthenaCloudTrailRecordsUnwrap(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	seedAthenaCloudTrailJSON(t, st, account, "athena-ct", "trail/")

	exec, err := st.StartAthenaQueryExecution(account, store.AthenaStartInput{
		QueryString: "SELECT eventName, eventID FROM ctdb.events ORDER BY eventID",
	})
	if err != nil || exec.State != "SUCCEEDED" {
		t.Fatalf("exec err=%v state=%s reason=%s", err, exec.State, exec.StateChangeReason)
	}
	if len(exec.ResultRows) != 4 {
		t.Fatalf("rows=%#v", exec.ResultRows)
	}
	if exec.ResultRows[1][0] != "AssumeRole" || exec.ResultRows[3][0] != "AssumeRoleWithSAML" {
		t.Fatalf("unwrap rows=%#v", exec.ResultRows)
	}
}

func TestAthenaGzipCloudTrailObject(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	bucket := "athena-ct-gz"
	prefix := "gzip/"
	if _, err := st.CreateBucket(account, bucket); err != nil {
		t.Fatal(err)
	}
	plain := `{"Records":[{"eventName":"RunInstances","eventID":"g1"}]}`
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	if _, err := gw.Write([]byte(plain)); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}
	key := prefix + "log.json.gz"
	if _, err := st.PutObject(account, bucket, key, store.PutObjectMeta{
		Data: buf.Bytes(), PlainSize: int64(buf.Len()),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateGlueDatabase(account, "gzdb", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateGlueTable(account, store.GlueTableCreate{
		DatabaseName:    "gzdb",
		Name:            "events",
		StorageLocation: "s3://" + bucket + "/" + prefix,
		Columns:         []store.GlueColumn{{Name: "eventName", Type: "string"}, {Name: "eventID", Type: "string"}},
		InputFormat:     "org.apache.hive.hcatalog.data.JsonSerDe",
		SerDeInfo:       store.GlueSerDeInfo{SerializationLibrary: "org.openx.data.jsonserde.JsonSerDe"},
	}); err != nil {
		t.Fatal(err)
	}
	exec, err := st.StartAthenaQueryExecution(account, store.AthenaStartInput{
		QueryString: "SELECT eventName FROM gzdb.events",
	})
	if err != nil || exec.State != "SUCCEEDED" {
		t.Fatalf("exec err=%v state=%s reason=%s", err, exec.State, exec.StateChangeReason)
	}
	if len(exec.ResultRows) != 2 || exec.ResultRows[1][0] != "RunInstances" {
		t.Fatalf("rows=%#v", exec.ResultRows)
	}
}

func TestAthenaWhereLike(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	seedAthenaCloudTrailJSON(t, st, account, "athena-like", "data/")

	exec, err := st.StartAthenaQueryExecution(account, store.AthenaStartInput{
		QueryString: "SELECT eventName FROM ctdb.events WHERE eventName LIKE 'Assume%'",
	})
	if err != nil || exec.State != "SUCCEEDED" {
		t.Fatalf("exec err=%v state=%s reason=%s", err, exec.State, exec.StateChangeReason)
	}
	if len(exec.ResultRows) != 3 {
		t.Fatalf("expected header+2 rows, got %#v", exec.ResultRows)
	}
	names := []string{exec.ResultRows[1][0], exec.ResultRows[2][0]}
	if !strings.Contains(names[0]+names[1], "AssumeRole") || !strings.Contains(names[0]+names[1], "AssumeRoleWithSAML") {
		t.Fatalf("like filter names=%v", names)
	}
}

func TestAthenaWhereJSONExtract(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	bucket := "athena-jx"
	if _, err := st.CreateBucket(account, bucket); err != nil {
		t.Fatal(err)
	}
	payload := `{"Records":[{"eventName":"ConsoleLogin","userIdentity":{"type":"IAMUser","userName":"alice"}}]}`
	if _, err := st.PutObject(account, bucket, "j/one.json", store.PutObjectMeta{
		Data: []byte(payload), PlainSize: int64(len(payload)),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateGlueDatabase(account, "jxdb", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateGlueTable(account, store.GlueTableCreate{
		DatabaseName:    "jxdb",
		Name:            "events",
		StorageLocation: "s3://" + bucket + "/j/",
		Columns: []store.GlueColumn{
			{Name: "eventName", Type: "string"},
			{Name: "userIdentity", Type: "string"},
		},
		InputFormat: "org.apache.hive.hcatalog.data.JsonSerDe",
		SerDeInfo:   store.GlueSerDeInfo{SerializationLibrary: "org.openx.data.jsonserde.JsonSerDe"},
	}); err != nil {
		t.Fatal(err)
	}
	exec, err := st.StartAthenaQueryExecution(account, store.AthenaStartInput{
		QueryString: "SELECT eventName FROM jxdb.events WHERE json_extract(userIdentity, '$.type') = 'IAMUser'",
	})
	if err != nil || exec.State != "SUCCEEDED" {
		t.Fatalf("exec err=%v state=%s reason=%s", err, exec.State, exec.StateChangeReason)
	}
	if len(exec.ResultRows) != 2 || exec.ResultRows[1][0] != "ConsoleLogin" {
		t.Fatalf("rows=%#v", exec.ResultRows)
	}
}

func TestAthenaWhereLikeUnderscore(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	seedAthenaCloudTrailJSON(t, st, account, "athena-us", "u/")

	exec, err := st.StartAthenaQueryExecution(account, store.AthenaStartInput{
		QueryString: "SELECT eventID FROM ctdb.events WHERE eventID LIKE 'e_'",
	})
	if err != nil || exec.State != "SUCCEEDED" {
		t.Fatalf("exec err=%v state=%s reason=%s", err, exec.State, exec.StateChangeReason)
	}
	if len(exec.ResultRows) != 4 {
		t.Fatalf("expected 3 data rows, got %#v", exec.ResultRows)
	}
}

func TestAthenaWhereIn(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	seedAthenaCloudTrailJSON(t, st, account, "athena-in", "in/")

	exec, err := st.StartAthenaQueryExecution(account, store.AthenaStartInput{
		QueryString: "SELECT eventName FROM ctdb.events WHERE eventName IN ('AssumeRole', 'PutObject') ORDER BY eventName",
	})
	if err != nil || exec.State != "SUCCEEDED" {
		t.Fatalf("exec err=%v state=%s reason=%s", err, exec.State, exec.StateChangeReason)
	}
	if len(exec.ResultRows) != 3 {
		t.Fatalf("expected header+2 rows, got %#v", exec.ResultRows)
	}
	if exec.ResultRows[1][0] != "AssumeRole" || exec.ResultRows[2][0] != "PutObject" {
		t.Fatalf("rows=%#v", exec.ResultRows)
	}
}

func TestAthenaWhereNeq(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	seedAthenaCloudTrailJSON(t, st, account, "athena-neq", "neq/")

	exec, err := st.StartAthenaQueryExecution(account, store.AthenaStartInput{
		QueryString: "SELECT eventName, eventID FROM ctdb.events WHERE eventName != 'PutObject' ORDER BY eventID",
	})
	if err != nil || exec.State != "SUCCEEDED" {
		t.Fatalf("exec err=%v state=%s reason=%s", err, exec.State, exec.StateChangeReason)
	}
	if len(exec.ResultRows) != 3 {
		t.Fatalf("expected header+2 rows, got %#v", exec.ResultRows)
	}
	if exec.ResultRows[1][0] != "AssumeRole" || exec.ResultRows[2][0] != "AssumeRoleWithSAML" {
		t.Fatalf("rows=%#v", exec.ResultRows)
	}

	exec2, err := st.StartAthenaQueryExecution(account, store.AthenaStartInput{
		QueryString: "SELECT eventID FROM ctdb.events WHERE eventID <> 'e2' ORDER BY eventID",
	})
	if err != nil || exec2.State != "SUCCEEDED" {
		t.Fatalf("<> exec err=%v state=%s reason=%s", err, exec2.State, exec2.StateChangeReason)
	}
	if len(exec2.ResultRows) != 3 {
		t.Fatalf("expected header+2 rows for <>, got %#v", exec2.ResultRows)
	}
	if exec2.ResultRows[1][0] != "e1" || exec2.ResultRows[2][0] != "e3" {
		t.Fatalf("<> rows=%#v", exec2.ResultRows)
	}
}

func TestAthenaWhereBetweenAndRange(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	seedAthenaPeopleCSV(t, st, account)

	exec, err := st.StartAthenaQueryExecution(account, store.AthenaStartInput{
		QueryString: "SELECT id, name FROM labdb.people WHERE id BETWEEN '1' AND '1'",
	})
	if err != nil || exec.State != "SUCCEEDED" {
		t.Fatalf("BETWEEN err=%v state=%s reason=%s", err, exec.State, exec.StateChangeReason)
	}
	if len(exec.ResultRows) != 2 || exec.ResultRows[1][1] != "alice" {
		t.Fatalf("BETWEEN rows=%#v", exec.ResultRows)
	}

	gt, err := st.StartAthenaQueryExecution(account, store.AthenaStartInput{
		QueryString: "SELECT name FROM labdb.people WHERE id > '1'",
	})
	if err != nil || gt.State != "SUCCEEDED" {
		t.Fatalf("> err=%v state=%s reason=%s", err, gt.State, gt.StateChangeReason)
	}
	if len(gt.ResultRows) != 2 || gt.ResultRows[1][0] != "bob" {
		t.Fatalf("> rows=%#v", gt.ResultRows)
	}

	le, err := st.StartAthenaQueryExecution(account, store.AthenaStartInput{
		QueryString: "SELECT name FROM labdb.people WHERE id <= '1'",
	})
	if err != nil || le.State != "SUCCEEDED" {
		t.Fatalf("<= err=%v state=%s reason=%s", err, le.State, le.StateChangeReason)
	}
	if len(le.ResultRows) != 2 || le.ResultRows[1][0] != "alice" {
		t.Fatalf("<= rows=%#v", le.ResultRows)
	}
}

func TestAthenaParquetInProcessFailsClosed(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	if _, err := st.CreateBucket(account, "athena-pq"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutObject(account, "athena-pq", "data/x.parquet", store.PutObjectMeta{
		Data: []byte("not-real-parquet"), PlainSize: 16,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateGlueDatabase(account, "pqdb", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateGlueTable(account, store.GlueTableCreate{
		DatabaseName:    "pqdb",
		Name:            "events",
		StorageLocation: "s3://athena-pq/data/",
		Columns:         []store.GlueColumn{{Name: "id", Type: "string"}},
		InputFormat:     "org.apache.hadoop.hive.ql.io.parquet.MapredParquetInputFormat",
		SerDeInfo:       store.GlueSerDeInfo{SerializationLibrary: "org.apache.hadoop.hive.ql.io.parquet.serde.ParquetHiveSerDe"},
	}); err != nil {
		t.Fatal(err)
	}
	exec, err := st.StartAthenaQueryExecution(account, store.AthenaStartInput{
		QueryString: "SELECT id FROM pqdb.events",
	})
	if err != nil {
		t.Fatal(err)
	}
	if exec.State != "FAILED" || !strings.Contains(exec.StateChangeReason, "DuckDB") {
		t.Fatalf("state=%s reason=%s", exec.State, exec.StateChangeReason)
	}
}

func TestAthenaDuckQueryExecution(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	seedAthenaPeopleCSV(t, st, account)

	exec, err := st.StartAthenaDuckQueryExecution(account, store.AthenaStartInput{
		QueryString: "SELECT name FROM people WHERE name = 'alice'",
		Database:    "labdb",
	}, func(querySQL, setupSQL string) ([]store.AthenaColumnInfo, [][]string, error) {
		if !strings.Contains(setupSQL, "CREATE OR REPLACE VIEW") || !strings.Contains(setupSQL, "read_csv_auto") {
			t.Fatalf("setup=%q", setupSQL)
		}
		if !strings.Contains(querySQL, "alice") {
			t.Fatalf("query=%q", querySQL)
		}
		return []store.AthenaColumnInfo{{Name: "name", Type: "varchar"}}, [][]string{{"alice"}}, nil
	})
	if err != nil || exec.State != "SUCCEEDED" {
		t.Fatalf("err=%v state=%s reason=%s", err, exec.State, exec.StateChangeReason)
	}
	if len(exec.ResultRows) != 2 || exec.ResultRows[1][0] != "alice" {
		t.Fatalf("rows=%#v", exec.ResultRows)
	}
}

func TestAthenaDuckRunnerFailClosed(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	seedAthenaPeopleCSV(t, st, account)
	exec, err := st.StartAthenaDuckQueryExecution(account, store.AthenaStartInput{
		QueryString: "SELECT * FROM people",
		Database:    "labdb",
	}, func(_, _ string) ([]store.AthenaColumnInfo, [][]string, error) {
		return nil, nil, errDuckUnavailable
	})
	if err != nil {
		t.Fatal(err)
	}
	if exec.State != "FAILED" {
		t.Fatalf("state=%s", exec.State)
	}
}


