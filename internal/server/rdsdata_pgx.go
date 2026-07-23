package server

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const pgxRDSDataExecutorMarker = `{"noctaxrisExecutor":"pgx"}`

// dataAPINamedParamRE rewrites AWS Data API :name placeholders to pgx @name.
// It leaves PostgreSQL casts (::type) untouched.
var dataAPINamedParamRE = regexp.MustCompile(`(^|[^:]):([A-Za-z_][A-Za-z0-9_]*)`)

// rdsDataPgxConnect is the dial entrypoint (overridable in tests).
var rdsDataPgxConnect = func(ctx context.Context, dsn string) (*pgx.Conn, error) {
	return pgx.Connect(ctx, dsn)
}

// rdsDataPgxEnabled reports whether the wire-protocol executor may be preferred.
// Buy-in is granted: default on. Set NOCTAXRIS_RDS_DATA_PGX=0 to force nested-psql only.
func rdsDataPgxEnabled() bool {
	v := strings.TrimSpace(os.Getenv(EnvRDSDataPgx))
	if v == "0" || strings.EqualFold(v, "false") || strings.EqualFold(v, "off") {
		return false
	}
	return true
}

// executeRDSDataPgx connects to the nested Postgres endpoint only (no host-published
// ports, no operator-supplied DSN) and runs ExecuteStatement with typed fields + binds.
func (s *Server) executeRDSDataPgx(
	ctx context.Context,
	accountID string,
	inst store.RDSDBInstance,
	req store.RDSDataExecuteRequest,
) (store.RDSDataExecuteResult, error) {
	if strings.TrimSpace(inst.ContainerID) == "" || inst.DBInstanceStatus != "available" {
		return store.RDSDataExecuteResult{}, store.ErrRDSDataUnavailable
	}
	user, password, err := s.rdsDataMasterCreds(accountID, inst, req.SecretARN)
	if err != nil {
		return store.RDSDataExecuteResult{}, err
	}
	db := strings.TrimSpace(req.Database)
	if db == "" {
		db = inst.DBName
	}
	if db == "" {
		db = "postgres"
	}
	dsn, err := buildNestedPostgresDSN(inst, user, password, db)
	if err != nil {
		return store.RDSDataExecuteResult{}, err
	}
	runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	conn, err := rdsDataPgxConnect(runCtx, dsn)
	if err != nil {
		return store.RDSDataExecuteResult{}, fmt.Errorf("%w: nested pgx dial: %v", store.ErrRDSDataUnavailable, err)
	}
	defer conn.Close(context.Background())
	return executeRDSDataPgxWithConn(runCtx, conn, req)
}

func executeRDSDataPgxWithConn(
	ctx context.Context,
	conn *pgx.Conn,
	req store.RDSDataExecuteRequest,
) (store.RDSDataExecuteResult, error) {
	sqlText := rewriteDataAPINamedParams(req.SQL)
	args := buildRDSDataNamedArgs(req.Parameters)
	selectLike := isPostgresSelectSQL(sqlText)
	returning := hasPostgresReturning(sqlText)
	if selectLike || returning {
		rows, err := conn.Query(ctx, sqlText, args)
		if err != nil {
			return store.RDSDataExecuteResult{}, err
		}
		defer rows.Close()
		res, err := mapPgxRows(rows)
		if err != nil {
			return store.RDSDataExecuteResult{}, err
		}
		if returning && !selectLike {
			res.NumberOfRecordsUpdated = int64(len(res.Records))
			if len(res.Records) > 0 {
				res.GeneratedFields = append([]store.RDSDataField(nil), res.Records[0]...)
			}
		}
		return res, nil
	}
	tag, err := conn.Exec(ctx, sqlText, args)
	if err != nil {
		return store.RDSDataExecuteResult{}, err
	}
	return store.RDSDataExecuteResult{
		NumberOfRecordsUpdated: tag.RowsAffected(),
		FormattedRecords:       pgxRDSDataExecutorMarker,
	}, nil
}

func buildNestedPostgresDSN(inst store.RDSDBInstance, user, password, database string) (string, error) {
	host := strings.TrimSpace(inst.EndpointAddress)
	if err := validateNestedPostgresHost(host); err != nil {
		return "", err
	}
	port := inst.EndpointPort
	if port <= 0 {
		port = 5432
	}
	if port > 65535 {
		return "", fmt.Errorf("%w: invalid nested postgres port", store.ErrRDSDataBadRequest)
	}
	user = strings.TrimSpace(user)
	if user == "" {
		user = "postgres"
	}
	database = strings.TrimSpace(database)
	if database == "" {
		database = "postgres"
	}
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, password),
		Host:   net.JoinHostPort(host, strconv.Itoa(port)),
		Path:   "/" + database,
	}
	q := u.Query()
	q.Set("sslmode", "disable")
	q.Set("connect_timeout", "2")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// validateNestedPostgresHost allows only DinD nested data-plane hostnames.
// Loopback, wildcards, and raw IPs are rejected so the Data API never dials an
// operator-published or open network endpoint.
func validateNestedPostgresHost(host string) error {
	host = strings.TrimSpace(host)
	if host == "" {
		return fmt.Errorf("%w: nested postgres endpoint is empty", store.ErrRDSDataUnavailable)
	}
	lower := strings.ToLower(host)
	switch lower {
	case "localhost", "127.0.0.1", "::1", "0.0.0.0", "*", "host.docker.internal":
		return fmt.Errorf("%w: refusing non-nested postgres host %q", store.ErrRDSDataBadRequest, host)
	}
	if strings.Contains(host, "/") || strings.Contains(host, "\\") {
		return fmt.Errorf("%w: invalid nested postgres host", store.ErrRDSDataBadRequest)
	}
	if ip := net.ParseIP(host); ip != nil {
		return fmt.Errorf("%w: refusing IP postgres host %q (nested container DNS name required)", store.ErrRDSDataBadRequest, host)
	}
	if !strings.HasPrefix(lower, "noctaxris-data-rds-") {
		return fmt.Errorf("%w: nested postgres host %q is not a data-plane endpoint", store.ErrRDSDataBadRequest, host)
	}
	return nil
}

func rewriteDataAPINamedParams(sqlText string) string {
	return dataAPINamedParamRE.ReplaceAllString(sqlText, "${1}@${2}")
}

func buildRDSDataNamedArgs(params []store.RDSDataSqlParameter) pgx.NamedArgs {
	if len(params) == 0 {
		return nil
	}
	out := make(pgx.NamedArgs, len(params))
	for _, p := range params {
		name := strings.TrimSpace(p.Name)
		if name == "" {
			continue
		}
		if p.IsNull != nil && *p.IsNull {
			out[name] = nil
			continue
		}
		switch {
		case p.StringValue != nil:
			out[name] = *p.StringValue
		case p.LongValue != nil:
			out[name] = *p.LongValue
		case p.DoubleValue != nil:
			out[name] = *p.DoubleValue
		case p.BooleanValue != nil:
			out[name] = *p.BooleanValue
		case len(p.BlobValue) > 0:
			out[name] = append([]byte(nil), p.BlobValue...)
		default:
			out[name] = nil
		}
	}
	return out
}

func mapPgxRows(rows pgx.Rows) (store.RDSDataExecuteResult, error) {
	fds := rows.FieldDescriptions()
	meta := make([]store.RDSDataColumnMeta, 0, len(fds))
	for _, fd := range fds {
		name := fd.Name
		meta = append(meta, store.RDSDataColumnMeta{
			Name:     name,
			TypeName: postgresOIDTypeName(fd.DataTypeOID),
			Label:    name,
		})
	}
	records := make([][]store.RDSDataField, 0)
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return store.RDSDataExecuteResult{}, err
		}
		fields := make([]store.RDSDataField, len(meta))
		for i := range meta {
			var oid uint32
			if i < len(fds) {
				oid = fds[i].DataTypeOID
			}
			var v any
			if i < len(vals) {
				v = vals[i]
			}
			fields[i] = mapPgxValue(oid, v)
		}
		records = append(records, fields)
	}
	if err := rows.Err(); err != nil {
		return store.RDSDataExecuteResult{}, err
	}
	return store.RDSDataExecuteResult{
		ColumnMetadata:   meta,
		Records:          records,
		FormattedRecords: pgxRDSDataExecutorMarker,
	}, nil
}

func postgresOIDTypeName(oid uint32) string {
	switch oid {
	case pgtype.BoolOID:
		return "BOOLEAN"
	case pgtype.Int2OID, pgtype.Int4OID, pgtype.Int8OID, pgtype.OIDOID:
		return "BIGINT"
	case pgtype.Float4OID, pgtype.Float8OID, pgtype.NumericOID:
		return "DOUBLE"
	case pgtype.ByteaOID:
		return "BLOB"
	case pgtype.JSONOID, pgtype.JSONBOID:
		return "JSON"
	case pgtype.UUIDOID:
		return "UUID"
	case pgtype.TimestampOID, pgtype.TimestamptzOID, pgtype.DateOID:
		return "TIMESTAMP"
	case pgtype.TextOID, pgtype.VarcharOID, pgtype.NameOID, pgtype.BPCharOID:
		return "VARCHAR"
	default:
		return "VARCHAR"
	}
}

func mapPgxValue(oid uint32, v any) store.RDSDataField {
	if v == nil {
		t := true
		return store.RDSDataField{IsNull: &t}
	}
	null := false
	switch oid {
	case pgtype.BoolOID:
		if b, ok := v.(bool); ok {
			return store.RDSDataField{BooleanValue: &b, IsNull: &null}
		}
	case pgtype.Int2OID, pgtype.Int4OID, pgtype.Int8OID, pgtype.OIDOID:
		if n, ok := anyToInt64(v); ok {
			return store.RDSDataField{LongValue: &n, IsNull: &null}
		}
	case pgtype.Float4OID, pgtype.Float8OID, pgtype.NumericOID:
		if f, ok := anyToFloat64(v); ok {
			return store.RDSDataField{DoubleValue: &f, IsNull: &null}
		}
	case pgtype.ByteaOID:
		if b, ok := v.([]byte); ok {
			enc := base64.StdEncoding.EncodeToString(b)
			return store.RDSDataField{BlobValue: &enc, IsNull: &null}
		}
	}
	switch t := v.(type) {
	case bool:
		return store.RDSDataField{BooleanValue: &t, IsNull: &null}
	case int64:
		return store.RDSDataField{LongValue: &t, IsNull: &null}
	case int32:
		n := int64(t)
		return store.RDSDataField{LongValue: &n, IsNull: &null}
	case int16:
		n := int64(t)
		return store.RDSDataField{LongValue: &n, IsNull: &null}
	case int:
		n := int64(t)
		return store.RDSDataField{LongValue: &n, IsNull: &null}
	case float64:
		return store.RDSDataField{DoubleValue: &t, IsNull: &null}
	case float32:
		f := float64(t)
		return store.RDSDataField{DoubleValue: &f, IsNull: &null}
	case []byte:
		enc := base64.StdEncoding.EncodeToString(t)
		return store.RDSDataField{BlobValue: &enc, IsNull: &null}
	case [16]byte:
		s := formatUUID(t)
		return store.RDSDataField{StringValue: &s, IsNull: &null}
	case time.Time:
		s := t.UTC().Format(time.RFC3339Nano)
		return store.RDSDataField{StringValue: &s, IsNull: &null}
	default:
		s := fmt.Sprint(v)
		return store.RDSDataField{StringValue: &s, IsNull: &null}
	}
}

func formatUUID(b [16]byte) string {
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func anyToInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case int32:
		return int64(n), true
	case int16:
		return int64(n), true
	case int:
		return int64(n), true
	case uint32:
		return int64(n), true
	case float64:
		return int64(n), true
	default:
		return 0, false
	}
}

func anyToFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int64:
		return float64(n), true
	case int32:
		return float64(n), true
	case pgtype.Numeric:
		f, err := n.Float64Value()
		if err != nil || !f.Valid {
			return 0, false
		}
		return f.Float64, true
	default:
		return 0, false
	}
}

func isPostgresSelectSQL(sqlText string) bool {
	upper := strings.ToUpper(strings.TrimSpace(sqlText))
	return strings.HasPrefix(upper, "SELECT") || strings.HasPrefix(upper, "WITH")
}

// hasPostgresReturning detects a RETURNING clause so DML can return generatedFields/records.
func hasPostgresReturning(sqlText string) bool {
	upper := strings.ToUpper(sqlText)
	const token = "RETURNING"
	for i := 0; i < len(upper); {
		rel := strings.Index(upper[i:], token)
		if rel < 0 {
			return false
		}
		idx := i + rel
		if idx > 0 {
			prev := upper[idx-1]
			if isSQLIdentChar(prev) {
				i = idx + 1
				continue
			}
		}
		end := idx + len(token)
		if end < len(upper) && isSQLIdentChar(upper[end]) {
			i = idx + 1
			continue
		}
		return true
	}
	return false
}

func isSQLIdentChar(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
}

// isRDSDataPgxDialFailure reports whether err means the nested wire endpoint was unreachable
// so ExecuteStatement may fall back to nested-psql.
func isRDSDataPgxDialFailure(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, store.ErrRDSDataUnavailable) {
		return true
	}
	var connectErr *pgconn.ConnectError
	if errors.As(err, &connectErr) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "nested pgx dial") ||
		strings.Contains(msg, "dial") ||
		strings.Contains(msg, "connect:") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "i/o timeout")
}
