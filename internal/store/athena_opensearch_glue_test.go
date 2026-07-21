package store_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestAthenaSelectCSVFromGlue(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	if _, err := st.CreateBucket(account, "athena-lab"); err != nil {
		t.Fatal(err)
	}
	csv := "id,name\n1,alice\n2,bob\n"
	if _, err := st.PutObject(account, "athena-lab", "data/t1.csv", store.PutObjectMeta{
		Data: []byte(csv), PlainSize: int64(len(csv)), ContentType: "text/csv",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateGlueDatabase(account, "labdb", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateGlueTable(account, store.GlueTableCreate{
		DatabaseName:    "labdb",
		Name:            "people",
		StorageLocation: "s3://athena-lab/data/",
		Columns: []store.GlueColumn{
			{Name: "id", Type: "string"},
			{Name: "name", Type: "string"},
		},
		SerDeInfo: store.GlueSerDeInfo{
			SerializationLibrary: "org.apache.hadoop.hive.serde2.lazy.LazySimpleSerDe",
			Parameters:           map[string]string{"field.delim": ","},
		},
	}); err != nil {
		t.Fatal(err)
	}

	exec, err := st.StartAthenaQueryExecution(account, store.AthenaStartInput{
		QueryString: "SELECT id, name FROM labdb.people LIMIT 10",
		Database:    "labdb",
	})
	if err != nil {
		t.Fatal(err)
	}
	if exec.State != "SUCCEEDED" {
		t.Fatalf("state=%s reason=%s", exec.State, exec.StateChangeReason)
	}
	got, err := st.GetAthenaQueryResults(account, exec.QueryExecutionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.ResultRows) < 3 {
		t.Fatalf("expected header+2 rows, got %#v", got.ResultRows)
	}
	if got.ResultRows[0][0] != "id" || got.ResultRows[1][1] != "alice" {
		t.Fatalf("rows=%#v", got.ResultRows)
	}
}

func TestAthenaUnsupportedSQLFails(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	exec, err := st.StartAthenaQueryExecution(account, store.AthenaStartInput{
		QueryString: "DROP TABLE labdb.people",
	})
	if err != nil {
		t.Fatal(err)
	}
	if exec.State != "FAILED" {
		t.Fatalf("expected FAILED, got %s", exec.State)
	}
	if !strings.Contains(exec.ErrorMessage, "unsupported") {
		t.Fatalf("error=%q", exec.ErrorMessage)
	}
}

func TestAthenaMissingTableFails(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	if _, err := st.CreateGlueDatabase(account, "labdb", ""); err != nil {
		t.Fatal(err)
	}
	exec, err := st.StartAthenaQueryExecution(account, store.AthenaStartInput{
		QueryString: "SELECT * FROM labdb.missing LIMIT 1",
		Database:    "labdb",
	})
	if err != nil {
		t.Fatal(err)
	}
	if exec.State != "FAILED" || !strings.Contains(exec.StateChangeReason, "TABLE_NOT_FOUND") {
		t.Fatalf("exec=%#v", exec)
	}
}

func TestAthenaJSONLines(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	if _, err := st.CreateBucket(account, "athena-json"); err != nil {
		t.Fatal(err)
	}
	payload := `{"id":"9","name":"zoe"}` + "\n" + `{"id":"8","name":"yan"}` + "\n"
	if _, err := st.PutObject(account, "athena-json", "j/rows.json", store.PutObjectMeta{
		Data: []byte(payload), PlainSize: int64(len(payload)),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateGlueDatabase(account, "jdb", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateGlueTable(account, store.GlueTableCreate{
		DatabaseName:    "jdb",
		Name:            "rows",
		StorageLocation: "s3://athena-json/j/",
		Columns:         []store.GlueColumn{{Name: "id", Type: "string"}, {Name: "name", Type: "string"}},
		InputFormat:     "org.apache.hive.hcatalog.data.JsonSerDe",
		SerDeInfo:       store.GlueSerDeInfo{SerializationLibrary: "org.openx.data.jsonserde.JsonSerDe"},
	}); err != nil {
		t.Fatal(err)
	}
	exec, err := st.StartAthenaQueryExecution(account, store.AthenaStartInput{
		QueryString: "SELECT * FROM jdb.rows LIMIT 5",
	})
	if err != nil || exec.State != "SUCCEEDED" {
		t.Fatalf("exec=%v %#v", err, exec)
	}
	if len(exec.ResultRows) != 3 {
		t.Fatalf("rows=%#v", exec.ResultRows)
	}
}

func TestOpenSearchDomainCRUD(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	d, err := st.CreateOpenSearchDomain(account, "us-east-1", "lab-domain", "OpenSearch_2.11")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(d.StubEndpoint, "stub://127.0.0.1/opensearch/") {
		t.Fatalf("endpoint=%s", d.StubEndpoint)
	}
	got, err := st.DescribeOpenSearchDomain(account, "lab-domain")
	if err != nil || got.DomainName != "lab-domain" {
		t.Fatalf("describe: %v %#v", err, got)
	}
	list, err := st.ListOpenSearchDomainNames(account)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v %#v", err, list)
	}
	if _, err := st.CreateOpenSearchDomain(account, "us-east-1", "lab-domain", ""); err != store.ErrOpenSearchDomainExists {
		t.Fatalf("dup: %v", err)
	}
	if err := st.DeleteOpenSearchDomain(account, "lab-domain"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DescribeOpenSearchDomain(account, "lab-domain"); err != store.ErrOpenSearchDomainNotFound {
		t.Fatalf("after delete: %v", err)
	}
}

func TestGluePartitionKeysAndSerDeRoundTrip(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	if _, err := st.CreateGlueDatabase(account, "pkdb", ""); err != nil {
		t.Fatal(err)
	}
	tbl, err := st.CreateGlueTable(account, store.GlueTableCreate{
		DatabaseName:    "pkdb",
		Name:            "t",
		StorageLocation: "s3://b/p",
		Columns:         []store.GlueColumn{{Name: "id", Type: "string"}},
		PartitionKeys:   []store.GlueColumn{{Name: "year", Type: "string"}},
		InputFormat:     "org.apache.hadoop.mapred.TextInputFormat",
		OutputFormat:    "org.apache.hadoop.hive.ql.io.HiveIgnoreKeyTextOutputFormat",
		SerDeInfo: store.GlueSerDeInfo{
			Name:                 "serde",
			SerializationLibrary: "org.apache.hadoop.hive.serde2.lazy.LazySimpleSerDe",
			Parameters:           map[string]string{"field.delim": ","},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(tbl.SerDeInfo.Parameters)
	if string(raw) == "" || tbl.PartitionKeys[0].Name != "year" {
		t.Fatalf("%#v", tbl)
	}
}
