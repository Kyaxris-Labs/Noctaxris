package server

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestRDSDataPgxEnabledDefaultOn(t *testing.T) {
	t.Setenv(EnvRDSDataPgx, "")
	if !rdsDataPgxEnabled() {
		t.Fatal("pgx path should prefer-on by default after buy-in")
	}
	t.Setenv(EnvRDSDataPgx, "1")
	if !rdsDataPgxEnabled() {
		t.Fatal("NOCTAXRIS_RDS_DATA_PGX=1 must enable pgx")
	}
	t.Setenv(EnvRDSDataPgx, "0")
	if rdsDataPgxEnabled() {
		t.Fatal("NOCTAXRIS_RDS_DATA_PGX=0 must disable pgx")
	}
}

func TestValidateNestedPostgresHost(t *testing.T) {
	if err := validateNestedPostgresHost("noctaxris-data-rds-lab1"); err != nil {
		t.Fatalf("nested host: %v", err)
	}
	for _, host := range []string{"localhost", "127.0.0.1", "0.0.0.0", "8.8.8.8", "db.example.com", ""} {
		if err := validateNestedPostgresHost(host); err == nil {
			t.Fatalf("expected reject for host %q", host)
		}
	}
}

func TestBuildNestedPostgresDSN(t *testing.T) {
	dsn, err := buildNestedPostgresDSN(store.RDSDBInstance{
		EndpointAddress: "noctaxris-data-rds-lab1",
		EndpointPort:    5432,
	}, "postgres", "s3cret", "appdb")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dsn, "noctaxris-data-rds-lab1:5432") {
		t.Fatalf("host missing: %s", dsn)
	}
	if !strings.Contains(dsn, "sslmode=disable") {
		t.Fatalf("sslmode missing: %s", dsn)
	}
	if strings.Contains(dsn, "s3cret") && !strings.Contains(dsn, "s3cret") {
		t.Fatal("password should be URL-encoded in userinfo")
	}
	if _, err := buildNestedPostgresDSN(store.RDSDBInstance{
		EndpointAddress: "127.0.0.1",
		EndpointPort:    5432,
	}, "postgres", "x", "postgres"); err == nil {
		t.Fatal("expected reject loopback DSN")
	}
}

func TestRewriteDataAPINamedParams(t *testing.T) {
	got := rewriteDataAPINamedParams(`SELECT :id::int, :name`)
	if got != `SELECT @id::int, @name` {
		t.Fatalf("rewrite=%q", got)
	}
}

func TestBuildRDSDataNamedArgsAndLiterals(t *testing.T) {
	s := "hi"
	var n int64 = 7
	b := true
	args := buildRDSDataNamedArgs([]store.RDSDataSqlParameter{
		{Name: "s", StringValue: &s},
		{Name: "n", LongValue: &n},
		{Name: "b", BooleanValue: &b},
		{Name: "z", IsNull: boolPtr(true)},
	})
	if args["s"] != "hi" || args["n"] != int64(7) || args["b"] != true || args["z"] != nil {
		t.Fatalf("args=%v", args)
	}
	sql, err := applyRDSDataParametersAsLiterals(
		`SELECT :s, :n, :b, :z`,
		[]store.RDSDataSqlParameter{
			{Name: "s", StringValue: &s},
			{Name: "n", LongValue: &n},
			{Name: "b", BooleanValue: &b},
			{Name: "z", IsNull: boolPtr(true)},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := `SELECT 'hi', 7, TRUE, NULL`
	if sql != want {
		t.Fatalf("literals=%q want %q", sql, want)
	}
}

func TestMapPgxValueTyped(t *testing.T) {
	null := false
	field := mapPgxValue(pgtype.Int8OID, int64(42))
	if field.LongValue == nil || *field.LongValue != 42 || field.IsNull == nil || *field.IsNull != null {
		t.Fatalf("long=%v", field)
	}
	field = mapPgxValue(pgtype.BoolOID, true)
	if field.BooleanValue == nil || !*field.BooleanValue {
		t.Fatalf("bool=%v", field)
	}
	field = mapPgxValue(pgtype.ByteaOID, []byte{0x01, 0x02})
	if field.BlobValue == nil || *field.BlobValue != base64.StdEncoding.EncodeToString([]byte{0x01, 0x02}) {
		t.Fatalf("blob=%v", field)
	}
	field = mapPgxValue(pgtype.TextOID, nil)
	if field.IsNull == nil || !*field.IsNull {
		t.Fatalf("null=%v", field)
	}
	if postgresOIDTypeName(pgtype.Int4OID) != "BIGINT" {
		t.Fatalf("type=%s", postgresOIDTypeName(pgtype.Int4OID))
	}
}

func TestHasPostgresReturning(t *testing.T) {
	if !hasPostgresReturning(`INSERT INTO t(x) VALUES (1) RETURNING id`) {
		t.Fatal("expected RETURNING")
	}
	if hasPostgresReturning(`SELECT returning_col FROM t`) {
		t.Fatal("column name must not match")
	}
	if hasPostgresReturning(`UPDATE t SET x=1`) {
		t.Fatal("no RETURNING")
	}
}

func TestIsRDSDataPgxDialFailure(t *testing.T) {
	if !isRDSDataPgxDialFailure(store.ErrRDSDataUnavailable) {
		t.Fatal("unavailable should be dial failure")
	}
	if !isRDSDataPgxDialFailure(errors.New("nested pgx dial: no such host")) {
		t.Fatal("dial message should match")
	}
	if isRDSDataPgxDialFailure(errors.New("ERROR: syntax error at or near")) {
		t.Fatal("SQL errors must not look like dial failures")
	}
}

func TestExecuteRDSDataPgxWithConnLive(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("NOCTAXRIS_TEST_PGX_DSN"))
	if dsn == "" {
		t.Skip("set NOCTAXRIS_TEST_PGX_DSN to run live pgx bind/typed smoke")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(context.Background())

	var id int64 = 9
	res, err := executeRDSDataPgxWithConn(ctx, conn, store.RDSDataExecuteRequest{
		SQL: "SELECT :id::bigint AS n, true AS flag",
		Parameters: []store.RDSDataSqlParameter{
			{Name: "id", LongValue: &id},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.FormattedRecords != pgxRDSDataExecutorMarker {
		t.Fatalf("marker=%q", res.FormattedRecords)
	}
	if len(res.Records) != 1 || res.Records[0][0].LongValue == nil || *res.Records[0][0].LongValue != 9 {
		t.Fatalf("records=%v", res.Records)
	}
	if res.Records[0][1].BooleanValue == nil || !*res.Records[0][1].BooleanValue {
		t.Fatalf("bool cell=%v", res.Records[0][1])
	}
}

func TestExecuteRDSDataPgxReturningGeneratedFieldsLive(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("NOCTAXRIS_TEST_PGX_DSN"))
	if dsn == "" {
		t.Skip("set NOCTAXRIS_TEST_PGX_DSN to run live pgx RETURNING smoke")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(context.Background())

	tbl := "noctaxris_rdsdata_ret_" + strings.ReplaceAll(t.Name(), "/", "_")
	if _, err := conn.Exec(ctx, `CREATE TEMP TABLE `+tbl+` (id BIGSERIAL PRIMARY KEY, name TEXT)`); err != nil {
		t.Fatal(err)
	}
	name := "alice"
	res, err := executeRDSDataPgxWithConn(ctx, conn, store.RDSDataExecuteRequest{
		SQL: "INSERT INTO " + tbl + " (name) VALUES (:name) RETURNING id",
		Parameters: []store.RDSDataSqlParameter{
			{Name: "name", StringValue: &name},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.GeneratedFields) != 1 || res.GeneratedFields[0].LongValue == nil {
		t.Fatalf("generatedFields=%v", res.GeneratedFields)
	}
	if res.NumberOfRecordsUpdated != 1 {
		t.Fatalf("updated=%d", res.NumberOfRecordsUpdated)
	}
}

func boolPtr(v bool) *bool { return &v }
