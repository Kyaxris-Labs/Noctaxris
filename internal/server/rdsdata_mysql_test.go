package server

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/compute"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestExecuteRDSDataMySQLWithConnLive(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("NOCTAXRIS_TEST_MYSQL_DSN"))
	if dsn == "" {
		t.Skip("set NOCTAXRIS_TEST_MYSQL_DSN to run live mysql wire smoke")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	conn, err := rdsDataMySQLConnect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close()
	res, err := executeRDSDataMySQLWithConn(ctx, conn, store.RDSDataExecuteRequest{
		SQL: "SELECT :n AS n",
		Parameters: []store.RDSDataSqlParameter{
			{Name: "n", LongValue: func() *int64 { v := int64(3); return &v }()},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.FormattedRecords != mysqlRDSDataExecutorMarker {
		t.Fatalf("marker=%q", res.FormattedRecords)
	}
	if len(res.Records) != 1 || res.Records[0][0].LongValue == nil || *res.Records[0][0].LongValue != 3 {
		t.Fatalf("records=%v", res.Records)
	}
}

func TestBuildNestedMySQLDSN(t *testing.T) {
	dsn, err := buildNestedMySQLDSN(store.RDSDBInstance{
		EndpointAddress: "noctaxris-data-rds-lab1",
		EndpointPort:    3306,
	}, "root", "s3cret", "appdb")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dsn, "noctaxris-data-rds-lab1:3306") {
		t.Fatalf("host missing: %s", dsn)
	}
	if !strings.Contains(dsn, "appdb") {
		t.Fatalf("db missing: %s", dsn)
	}
	if _, err := buildNestedMySQLDSN(store.RDSDBInstance{
		EndpointAddress: "127.0.0.1",
		EndpointPort:    3306,
	}, "root", "x", "mysql"); err == nil {
		t.Fatal("expected reject loopback DSN")
	}
}

func TestRewriteDataAPIMySQLParams(t *testing.T) {
	sqlText, args, err := rewriteDataAPIMySQLParams(`SELECT :id, :name`, []store.RDSDataSqlParameter{
		{Name: "id", LongValue: func() *int64 { n := int64(9); return &n }()},
		{Name: "name", StringValue: func() *string { s := "x"; return &s }()},
	})
	if err != nil {
		t.Fatal(err)
	}
	if sqlText != `SELECT ?, ?` {
		t.Fatalf("sql=%q", sqlText)
	}
	if len(args) != 2 {
		t.Fatalf("args=%v", args)
	}
}

func TestMapMySQLSQLResultSelect(t *testing.T) {
	res := mapMySQLSQLResult(compute.MySQLSQLResult{
		Columns: []string{"n"},
		Rows:    [][]string{{"1"}},
		Select:  true,
	})
	if res.FormattedRecords != nestedRDSDataMySQLExecutorMarker {
		t.Fatalf("marker=%q", res.FormattedRecords)
	}
	if len(res.Records) != 1 || res.Records[0][0].StringValue == nil || *res.Records[0][0].StringValue != "1" {
		t.Fatalf("records=%v", res.Records)
	}
}