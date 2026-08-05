package server

import (
	"encoding/base64"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Kyaxris-Labs/Noctaxris/internal/compute"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/identity"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestRDSDataMySQLLiteralAndSelectHelpers(t *testing.T) {
	t.Parallel()

	if !isMySQLSelectSQL("  select 1") || !isMySQLSelectSQL("WITH cte AS (SELECT 1) SELECT * FROM cte") {
		t.Fatal("select/with should match")
	}
	if isMySQLSelectSQL("INSERT INTO t VALUES (1)") {
		t.Fatal("insert should not match")
	}
	if !isPostgresSelectSQL("SELECT 1") || isPostgresSelectSQL("DELETE FROM t") {
		t.Fatal("postgres select helper")
	}

	if quoteMySQLStringLiteral("a'b") != "'a''b'" {
		t.Fatalf("quote: %q", quoteMySQLStringLiteral("a'b"))
	}

	trueV := true
	falseV := false
	s := "hi"
	long := int64(9)
	dbl := 1.5
	boolT := true
	for _, tc := range []struct {
		name string
		p    store.RDSDataSqlParameter
		want string
	}{
		{"null", store.RDSDataSqlParameter{Name: "n", IsNull: &trueV}, "NULL"},
		{"string", store.RDSDataSqlParameter{Name: "s", StringValue: &s}, "'hi'"},
		{"long", store.RDSDataSqlParameter{Name: "l", LongValue: &long}, "9"},
		{"double", store.RDSDataSqlParameter{Name: "d", DoubleValue: &dbl}, "1.5"},
		{"true", store.RDSDataSqlParameter{Name: "b", BooleanValue: &boolT}, "TRUE"},
		{"false", store.RDSDataSqlParameter{Name: "b", BooleanValue: &falseV}, "FALSE"},
		{"blob", store.RDSDataSqlParameter{Name: "x", BlobValue: []byte{0xab, 0xcd}}, "0xabcd"},
	} {
		got, err := rdsDataMySQLParameterLiteral(tc.p)
		if err != nil || got != tc.want {
			t.Fatalf("%s: got %q err=%v want %q", tc.name, got, err, tc.want)
		}
	}
	if _, err := rdsDataMySQLParameterLiteral(store.RDSDataSqlParameter{Name: "empty"}); err == nil {
		t.Fatal("empty value should error")
	}

	sqlOut, err := applyRDSDataParametersAsMySQLLiterals(
		"SELECT :name, :n, :flag FROM t WHERE id=:id",
		[]store.RDSDataSqlParameter{
			{Name: "name", StringValue: &s},
			{Name: "n", IsNull: &trueV},
			{Name: "flag", BooleanValue: &boolT},
			{Name: "id", LongValue: &long},
		},
	)
	if err != nil || !strings.Contains(sqlOut, "'hi'") || !strings.Contains(sqlOut, "NULL") || !strings.Contains(sqlOut, "TRUE") {
		t.Fatalf("rewrite=%q err=%v", sqlOut, err)
	}
	if _, err := applyRDSDataParametersAsMySQLLiterals("SELECT :missing", nil); err == nil {
		t.Fatal("missing param should error")
	}
	if _, err := applyRDSDataParametersAsMySQLLiterals("SELECT :x", []store.RDSDataSqlParameter{{Name: "", StringValue: &s}}); err == nil {
		t.Fatal("empty param name should error")
	}
}

func TestMapMySQLScanValueAndSQLResult(t *testing.T) {
	t.Parallel()

	if f := mapMySQLScanValue(nil); f.IsNull == nil || !*f.IsNull {
		t.Fatal("nil -> null")
	}
	if f := mapMySQLScanValue(true); f.BooleanValue == nil || !*f.BooleanValue {
		t.Fatal("bool")
	}
	if f := mapMySQLScanValue(int64(3)); f.LongValue == nil || *f.LongValue != 3 {
		t.Fatal("int64")
	}
	if f := mapMySQLScanValue(int32(4)); f.LongValue == nil || *f.LongValue != 4 {
		t.Fatal("int32")
	}
	if f := mapMySQLScanValue(5); f.LongValue == nil || *f.LongValue != 5 {
		t.Fatal("int")
	}
	if f := mapMySQLScanValue(float64(2.5)); f.DoubleValue == nil || *f.DoubleValue != 2.5 {
		t.Fatal("float64")
	}
	if f := mapMySQLScanValue(float32(1.25)); f.DoubleValue == nil {
		t.Fatal("float32")
	}
	if f := mapMySQLScanValue([]byte("ab")); f.BlobValue == nil || *f.BlobValue != base64.StdEncoding.EncodeToString([]byte("ab")) {
		t.Fatal("blob")
	}
	if f := mapMySQLScanValue("s"); f.StringValue == nil || *f.StringValue != "s" {
		t.Fatal("string")
	}
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if f := mapMySQLScanValue(ts); f.StringValue == nil || !strings.Contains(*f.StringValue, "2026") {
		t.Fatal("time")
	}
	if f := mapMySQLScanValue(struct{ X int }{1}); f.StringValue == nil {
		t.Fatal("default")
	}

	res := mapMySQLSQLResult(compute.MySQLSQLResult{Select: false, NumberOfRecordsUpdated: 2})
	if res.NumberOfRecordsUpdated != 2 || res.FormattedRecords == "" {
		t.Fatalf("non-select %#v", res)
	}
	sel := mapMySQLSQLResult(compute.MySQLSQLResult{
		Select:  true,
		Columns: []string{"a", "b"},
		Rows:    [][]string{{"1", "2"}, {"3"}},
	})
	if len(sel.ColumnMetadata) != 2 || len(sel.Records) != 2 || sel.Records[1][1].StringValue == nil {
		t.Fatalf("select %#v", sel)
	}

	if !isRDSDataMySQLDialFailure(store.ErrRDSDataUnavailable) {
		t.Fatal("unavailable")
	}
	if !isRDSDataMySQLDialFailure(errString("connection refused")) {
		t.Fatal("dial msg")
	}
	if isRDSDataMySQLDialFailure(nil) || isRDSDataMySQLDialFailure(errString("syntax error")) {
		t.Fatal("non-dial")
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func TestPostgresOIDMapPgxAndNumericHelpers(t *testing.T) {
	t.Parallel()

	oids := []uint32{
		pgtype.BoolOID, pgtype.Int2OID, pgtype.Int4OID, pgtype.Int8OID, pgtype.OIDOID,
		pgtype.Float4OID, pgtype.Float8OID, pgtype.NumericOID, pgtype.ByteaOID,
		pgtype.JSONOID, pgtype.JSONBOID, pgtype.UUIDOID,
		pgtype.TimestampOID, pgtype.TimestamptzOID, pgtype.DateOID,
		pgtype.TextOID, pgtype.VarcharOID, pgtype.NameOID, pgtype.BPCharOID,
		99999,
	}
	for _, oid := range oids {
		if postgresOIDTypeName(oid) == "" {
			t.Fatalf("empty type for oid %d", oid)
		}
	}

	nullField := mapPgxValue(pgtype.TextOID, nil)
	if nullField.IsNull == nil || !*nullField.IsNull {
		t.Fatal("nil field")
	}
	if f := mapPgxValue(pgtype.BoolOID, true); f.BooleanValue == nil || !*f.BooleanValue {
		t.Fatal("bool oid")
	}
	if f := mapPgxValue(pgtype.Int8OID, int64(7)); f.LongValue == nil || *f.LongValue != 7 {
		t.Fatal("int oid")
	}
	if f := mapPgxValue(pgtype.Float8OID, float64(3.25)); f.DoubleValue == nil {
		t.Fatal("float oid")
	}
	if f := mapPgxValue(pgtype.ByteaOID, []byte{1, 2}); f.BlobValue == nil {
		t.Fatal("bytea oid")
	}
	if f := mapPgxValue(pgtype.TextOID, "x"); f.StringValue == nil || *f.StringValue != "x" {
		t.Fatal("text fallback")
	}
	if f := mapPgxValue(0, int32(2)); f.LongValue == nil || *f.LongValue != 2 {
		t.Fatal("int32 fallback")
	}
	if f := mapPgxValue(0, int16(3)); f.LongValue == nil || *f.LongValue != 3 {
		t.Fatal("int16 fallback")
	}
	if f := mapPgxValue(0, float32(1.5)); f.DoubleValue == nil {
		t.Fatal("float32 fallback")
	}
	var uuid [16]byte
	uuid[0] = 0x12
	if f := mapPgxValue(pgtype.UUIDOID, uuid); f.StringValue == nil || !strings.HasPrefix(*f.StringValue, "12") {
		t.Fatalf("uuid field %#v", f)
	}
	if f := mapPgxValue(0, time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC)); f.StringValue == nil {
		t.Fatal("time fallback")
	}
	if got := formatUUID(uuid); !strings.Contains(got, "-") {
		t.Fatalf("formatUUID %q", got)
	}

	if n, ok := anyToInt64(int64(1)); !ok || n != 1 {
		t.Fatal("i64")
	}
	if n, ok := anyToInt64(int32(2)); !ok || n != 2 {
		t.Fatal("i32")
	}
	if n, ok := anyToInt64(int16(3)); !ok || n != 3 {
		t.Fatal("i16")
	}
	if n, ok := anyToInt64(4); !ok || n != 4 {
		t.Fatal("int")
	}
	if n, ok := anyToInt64(uint32(5)); !ok || n != 5 {
		t.Fatal("u32")
	}
	if n, ok := anyToInt64(float64(6)); !ok || n != 6 {
		t.Fatal("f64")
	}
	if _, ok := anyToInt64("x"); ok {
		t.Fatal("bad int")
	}
	if f, ok := anyToFloat64(float64(1.1)); !ok || f != 1.1 {
		t.Fatal("f64")
	}
	if f, ok := anyToFloat64(float32(2)); !ok || f != 2 {
		t.Fatal("f32")
	}
	if f, ok := anyToFloat64(int64(3)); !ok || f != 3 {
		t.Fatal("i64")
	}
	if f, ok := anyToFloat64(int32(4)); !ok || f != 4 {
		t.Fatal("i32")
	}
	if _, ok := anyToFloat64("no"); ok {
		t.Fatal("bad float")
	}
}

func TestLambdaInvokeHelpersAndPassRoleHelpers(t *testing.T) {
	t.Parallel()

	if s, err := invokeEventJSON(nil); err != nil || s != "{}" {
		t.Fatalf("nil: %q %v", s, err)
	}
	if s, err := invokeEventJSON(""); err != nil || s != "{}" {
		t.Fatalf("empty: %q %v", s, err)
	}
	if s, err := invokeEventJSON(`{"a":1}`); err != nil || s != `{"a":1}` {
		t.Fatalf("json string: %q %v", s, err)
	}
	b64 := base64.StdEncoding.EncodeToString([]byte(`{"b":2}`))
	if s, err := invokeEventJSON(b64); err != nil || s != `{"b":2}` {
		t.Fatalf("b64: %q %v", s, err)
	}
	if _, err := invokeEventJSON(base64.StdEncoding.EncodeToString([]byte("not-json"))); err == nil {
		t.Fatal("bad decoded json")
	}
	if _, err := invokeEventJSON("!!!!"); err == nil {
		t.Fatal("bad b64")
	}
	if s, err := invokeEventJSON(map[string]any{"k": "v"}); err != nil || !strings.Contains(s, `"k"`) {
		t.Fatalf("map: %q %v", s, err)
	}
	if s, err := invokeEventJSON([]any{1, 2}); err != nil || !strings.Contains(s, "1") {
		t.Fatalf("slice: %q %v", s, err)
	}
	if s, err := invokeEventJSON(42); err != nil || s != "42" {
		t.Fatalf("default: %q %v", s, err)
	}

	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.writeLambdaInvokeRESTAccepted(rec, "req-1", "")
	if rec.Code != 202 || rec.Header().Get("X-Amz-Executed-Version") != "$LATEST" {
		t.Fatalf("accepted: %d %v", rec.Code, rec.Header())
	}
	rec = httptest.NewRecorder()
	srv.writeLambdaInvokeREST(rec, "req-2", nil, "")
	if rec.Code != 200 || rec.Body.String() != "null" {
		t.Fatalf("rest empty: %d %q", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.writeLambdaInvokeREST(rec, "req-3", []byte(`{"ok":true}`), "1")
	if rec.Code != 200 || rec.Body.String() != `{"ok":true}` || rec.Header().Get("X-Amz-Executed-Version") != "1" {
		t.Fatalf("rest payload: %d %q", rec.Code, rec.Body.String())
	}

	if batchPullRoleARN(store.BatchJobDefinition{ExecutionRoleARN: "exec", JobRoleARN: "job"}) != "exec" {
		t.Fatal("prefer execution role")
	}
	if batchPullRoleARN(store.BatchJobDefinition{JobRoleARN: " job "}) != "job" {
		t.Fatal("fallback job role")
	}
	if ecsServiceRolesPassRoleMatch(store.ECSService{}, store.ECSTaskDefinition{TaskRoleARN: "a", ExecutionRoleARN: "b"}) {
		t.Fatal("empty passed should false")
	}
	if !ecsServiceRolesPassRoleMatch(
		store.ECSService{PassedTaskRoleARN: "a", PassedExecutionRoleARN: "b"},
		store.ECSTaskDefinition{TaskRoleARN: "a", ExecutionRoleARN: "b"},
	) {
		t.Fatal("match should true")
	}
	if ecsServiceRolesPassRoleMatch(
		store.ECSService{PassedTaskRoleARN: "a", PassedExecutionRoleARN: "b"},
		store.ECSTaskDefinition{TaskRoleARN: "a", ExecutionRoleARN: "x"},
	) {
		t.Fatal("mismatch should false")
	}

	acc := "000000000001"
	if p, ok := principalFromARN(acc, "arn:aws:iam::"+acc+":root"); !ok || p.Kind != identity.KindRoot {
		t.Fatalf("root %#v", p)
	}
	if p, ok := principalFromARN(acc, "arn:aws:iam::"+acc+":user/alice"); !ok || p.UserName != "alice" {
		t.Fatalf("user %#v", p)
	}
	if _, ok := principalFromARN(acc, "arn:aws:iam::"+acc+":user/"); ok {
		t.Fatal("empty user")
	}
	if _, ok := principalFromARN(acc, "arn:aws:iam::"+acc+":user/a/b"); ok {
		t.Fatal("path user")
	}
	if p, ok := principalFromARN(acc, "arn:aws:iam::"+acc+":role/lab"); !ok || p.RoleName != "lab" {
		t.Fatalf("role %#v", p)
	}
	if _, ok := principalFromARN(acc, "arn:aws:iam::"+acc+":role/"); ok {
		t.Fatal("empty role")
	}
	if p, ok := principalFromARN(acc, "arn:aws:sts::"+acc+":assumed-role/lab/sess"); !ok || p.RoleName != "lab" {
		t.Fatalf("assumed %#v", p)
	}
	if _, ok := principalFromARN(acc, "arn:aws:sts::"+acc+":assumed-role/lab"); ok {
		t.Fatal("assumed incomplete")
	}
	if p, ok := principalFromARN(acc, "arn:aws:sts::"+acc+":federated-user/fed"); !ok || p.Kind != identity.KindFederated {
		t.Fatalf("fed %#v", p)
	}
	if _, ok := principalFromARN(acc, "arn:aws:sts::"+acc+":federated-user/"); ok {
		t.Fatal("empty fed")
	}
	if _, ok := principalFromARN(acc, "arn:aws:s3:::bucket"); ok {
		t.Fatal("unknown arn")
	}

	ss := []string{"c", "a", "b"}
	sortStrings(ss)
	if ss[0] != "a" || ss[2] != "c" {
		t.Fatalf("%v", ss)
	}
}