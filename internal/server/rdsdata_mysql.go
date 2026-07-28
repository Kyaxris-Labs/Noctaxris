package server

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/Kyaxris-Labs/Noctaxris/internal/compute"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	mysqlRDSDataExecutorMarker       = `{"noctaxrisExecutor":"mysql"}`
	nestedRDSDataMySQLExecutorMarker = `{"noctaxrisExecutor":"nested-mysql"}`
)

// rdsDataMySQLConnect opens a single connection (overridable in tests).
var rdsDataMySQLConnect = func(ctx context.Context, dsn string) (*sql.Conn, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	return db.Conn(ctx)
}

func (s *Server) preferNestedRDSDataExecuteMySQL(
	ctx context.Context,
	accountID string,
	inst store.RDSDBInstance,
	req store.RDSDataExecuteRequest,
) (store.RDSDataExecuteResult, error) {
	if rdsDataPgxEnabled() {
		res, err := s.executeRDSDataMySQL(ctx, accountID, inst, req)
		if err == nil {
			return res, nil
		}
		if !isRDSDataMySQLDialFailure(err) {
			return store.RDSDataExecuteResult{}, err
		}
	}
	return s.executeRDSDataNestedMySQL(ctx, accountID, inst, req)
}

func (s *Server) executeRDSDataMySQL(
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
	db := rdsDataDatabaseName(inst, req.Database, "mysql")
	dsn, err := buildNestedMySQLDSN(inst, user, password, db)
	if err != nil {
		return store.RDSDataExecuteResult{}, err
	}
	runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	conn, err := rdsDataMySQLConnect(runCtx, dsn)
	if err != nil {
		return store.RDSDataExecuteResult{}, fmt.Errorf("%w: nested mysql dial: %v", store.ErrRDSDataUnavailable, err)
	}
	defer conn.Close()
	return executeRDSDataMySQLWithConn(runCtx, conn, req)
}

func executeRDSDataMySQLWithConn(
	ctx context.Context,
	conn *sql.Conn,
	req store.RDSDataExecuteRequest,
) (store.RDSDataExecuteResult, error) {
	sqlText, args, err := rewriteDataAPIMySQLParams(req.SQL, req.Parameters)
	if err != nil {
		return store.RDSDataExecuteResult{}, err
	}
	if isMySQLSelectSQL(sqlText) {
		rows, err := conn.QueryContext(ctx, sqlText, args...)
		if err != nil {
			return store.RDSDataExecuteResult{}, err
		}
		defer rows.Close()
		return mapMySQLRows(rows)
	}
	res, err := conn.ExecContext(ctx, sqlText, args...)
	if err != nil {
		return store.RDSDataExecuteResult{}, err
	}
	n, _ := res.RowsAffected()
	return store.RDSDataExecuteResult{
		NumberOfRecordsUpdated: n,
		FormattedRecords:       mysqlRDSDataExecutorMarker,
	}, nil
}

func executeRDSDataMySQLWithTx(
	ctx context.Context,
	tx *sql.Tx,
	req store.RDSDataExecuteRequest,
) (store.RDSDataExecuteResult, error) {
	sqlText, args, err := rewriteDataAPIMySQLParams(req.SQL, req.Parameters)
	if err != nil {
		return store.RDSDataExecuteResult{}, err
	}
	if isMySQLSelectSQL(sqlText) {
		rows, err := tx.QueryContext(ctx, sqlText, args...)
		if err != nil {
			return store.RDSDataExecuteResult{}, err
		}
		defer rows.Close()
		return mapMySQLRows(rows)
	}
	res, err := tx.ExecContext(ctx, sqlText, args...)
	if err != nil {
		return store.RDSDataExecuteResult{}, err
	}
	n, _ := res.RowsAffected()
	return store.RDSDataExecuteResult{
		NumberOfRecordsUpdated: n,
		FormattedRecords:       mysqlRDSDataExecutorMarker,
	}, nil
}

func (s *Server) executeRDSDataNestedMySQL(
	ctx context.Context,
	accountID string,
	inst store.RDSDBInstance,
	req store.RDSDataExecuteRequest,
) (store.RDSDataExecuteResult, error) {
	if strings.TrimSpace(inst.ContainerID) == "" || inst.DBInstanceStatus != "available" {
		return store.RDSDataExecuteResult{}, store.ErrRDSDataUnavailable
	}
	cli, err := s.computeClient()
	if err != nil || cli == nil {
		return store.RDSDataExecuteResult{}, store.ErrRDSDataUnavailable
	}
	user, password, err := s.rdsDataMasterCreds(accountID, inst, req.SecretARN)
	if err != nil {
		return store.RDSDataExecuteResult{}, err
	}
	db := rdsDataDatabaseName(inst, req.Database, "mysql")
	sqlText := req.SQL
	if len(req.Parameters) > 0 {
		rewritten, rewriteErr := applyRDSDataParametersAsMySQLLiterals(sqlText, req.Parameters)
		if rewriteErr != nil {
			return store.RDSDataExecuteResult{}, rewriteErr
		}
		sqlText = rewritten
	}
	runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	myRes, err := cli.ExecMySQLSQL(runCtx, compute.MySQLSQLOpts{
		ContainerID: inst.ContainerID,
		Username:    user,
		Password:    password,
		Database:    db,
		SQL:         sqlText,
	})
	if err != nil {
		return store.RDSDataExecuteResult{}, err
	}
	return mapMySQLSQLResult(myRes), nil
}

func buildNestedMySQLDSN(inst store.RDSDBInstance, user, password, database string) (string, error) {
	host := strings.TrimSpace(inst.EndpointAddress)
	if err := validateNestedRDSDataHost(host); err != nil {
		return "", err
	}
	port := inst.EndpointPort
	if port <= 0 {
		port = 3306
	}
	if port > 65535 {
		return "", fmt.Errorf("%w: invalid nested mysql port", store.ErrRDSDataBadRequest)
	}
	user = strings.TrimSpace(user)
	if user == "" {
		user = "root"
	}
	database = strings.TrimSpace(database)
	if database == "" {
		database = "mysql"
	}
	cfg := mysql.Config{
		User:                 user,
		Passwd:               password,
		Net:                  "tcp",
		Addr:                 net.JoinHostPort(host, strconv.Itoa(port)),
		DBName:               database,
		AllowNativePasswords: true,
		ParseTime:            true,
		Timeout:              2 * time.Second,
	}
	return cfg.FormatDSN(), nil
}

func rewriteDataAPIMySQLParams(sqlText string, params []store.RDSDataSqlParameter) (string, []any, error) {
	if len(params) == 0 {
		return sqlText, nil, nil
	}
	byName := make(map[string]store.RDSDataSqlParameter, len(params))
	for _, p := range params {
		name := strings.TrimSpace(p.Name)
		if name == "" {
			return "", nil, fmt.Errorf("%w: parameter name is required", store.ErrRDSDataBadRequest)
		}
		byName[name] = p
	}
	var args []any
	var rewriteErr error
	out := dataAPINamedParamRE.ReplaceAllStringFunc(sqlText, func(match string) string {
		if rewriteErr != nil {
			return match
		}
		parts := dataAPINamedParamRE.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}
		prefix, name := parts[1], parts[2]
		p, ok := byName[name]
		if !ok {
			rewriteErr = fmt.Errorf("%w: missing value for parameter %q", store.ErrRDSDataBadRequest, name)
			return match
		}
		arg, err := rdsDataMySQLArgValue(p)
		if err != nil {
			rewriteErr = err
			return match
		}
		args = append(args, arg)
		return prefix + "?"
	})
	if rewriteErr != nil {
		return "", nil, rewriteErr
	}
	return out, args, nil
}

func rdsDataMySQLArgValue(p store.RDSDataSqlParameter) (any, error) {
	if p.IsNull != nil && *p.IsNull {
		return nil, nil
	}
	switch {
	case p.StringValue != nil:
		return *p.StringValue, nil
	case p.LongValue != nil:
		return *p.LongValue, nil
	case p.DoubleValue != nil:
		return *p.DoubleValue, nil
	case p.BooleanValue != nil:
		return *p.BooleanValue, nil
	case len(p.BlobValue) > 0:
		return append([]byte(nil), p.BlobValue...), nil
	default:
		return nil, fmt.Errorf("%w: parameter %q has no supported value", store.ErrRDSDataBadRequest, p.Name)
	}
}

func applyRDSDataParametersAsMySQLLiterals(sqlText string, params []store.RDSDataSqlParameter) (string, error) {
	byName := make(map[string]store.RDSDataSqlParameter, len(params))
	for _, p := range params {
		name := strings.TrimSpace(p.Name)
		if name == "" {
			return "", fmt.Errorf("%w: parameter name is required", store.ErrRDSDataBadRequest)
		}
		byName[name] = p
	}
	var rewriteErr error
	out := dataAPINamedParamRE.ReplaceAllStringFunc(sqlText, func(match string) string {
		if rewriteErr != nil {
			return match
		}
		parts := dataAPINamedParamRE.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}
		prefix, name := parts[1], parts[2]
		p, ok := byName[name]
		if !ok {
			rewriteErr = fmt.Errorf("%w: missing value for parameter %q", store.ErrRDSDataBadRequest, name)
			return match
		}
		lit, err := rdsDataMySQLParameterLiteral(p)
		if err != nil {
			rewriteErr = err
			return match
		}
		return prefix + lit
	})
	if rewriteErr != nil {
		return "", rewriteErr
	}
	return out, nil
}

func rdsDataMySQLParameterLiteral(p store.RDSDataSqlParameter) (string, error) {
	if p.IsNull != nil && *p.IsNull {
		return "NULL", nil
	}
	switch {
	case p.StringValue != nil:
		return quoteMySQLStringLiteral(*p.StringValue), nil
	case p.LongValue != nil:
		return strconv.FormatInt(*p.LongValue, 10), nil
	case p.DoubleValue != nil:
		return strconv.FormatFloat(*p.DoubleValue, 'g', -1, 64), nil
	case p.BooleanValue != nil:
		if *p.BooleanValue {
			return "TRUE", nil
		}
		return "FALSE", nil
	case len(p.BlobValue) > 0:
		return `0x` + fmt.Sprintf("%x", p.BlobValue), nil
	default:
		return "", fmt.Errorf("%w: parameter %q has no supported value", store.ErrRDSDataBadRequest, p.Name)
	}
}

func quoteMySQLStringLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func mapMySQLRows(rows *sql.Rows) (store.RDSDataExecuteResult, error) {
	cols, err := rows.Columns()
	if err != nil {
		return store.RDSDataExecuteResult{}, err
	}
	meta := make([]store.RDSDataColumnMeta, 0, len(cols))
	for _, name := range cols {
		meta = append(meta, store.RDSDataColumnMeta{
			Name:     name,
			TypeName: "VARCHAR",
			Label:    name,
		})
	}
	records := make([][]store.RDSDataField, 0)
	for rows.Next() {
		dest := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range dest {
			ptrs[i] = &dest[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return store.RDSDataExecuteResult{}, err
		}
		fields := make([]store.RDSDataField, len(cols))
		for i, v := range dest {
			fields[i] = mapMySQLScanValue(v)
		}
		records = append(records, fields)
	}
	if err := rows.Err(); err != nil {
		return store.RDSDataExecuteResult{}, err
	}
	return store.RDSDataExecuteResult{
		ColumnMetadata:   meta,
		Records:          records,
		FormattedRecords: mysqlRDSDataExecutorMarker,
	}, nil
}

func mapMySQLScanValue(v any) store.RDSDataField {
	if v == nil {
		t := true
		return store.RDSDataField{IsNull: &t}
	}
	null := false
	switch t := v.(type) {
	case bool:
		return store.RDSDataField{BooleanValue: &t, IsNull: &null}
	case int64:
		return store.RDSDataField{LongValue: &t, IsNull: &null}
	case int32:
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
	case string:
		s := t
		return store.RDSDataField{StringValue: &s, IsNull: &null}
	case time.Time:
		s := t.UTC().Format(time.RFC3339Nano)
		return store.RDSDataField{StringValue: &s, IsNull: &null}
	default:
		s := fmt.Sprint(v)
		return store.RDSDataField{StringValue: &s, IsNull: &null}
	}
}

func mapMySQLSQLResult(my compute.MySQLSQLResult) store.RDSDataExecuteResult {
	if !my.Select {
		return store.RDSDataExecuteResult{
			NumberOfRecordsUpdated: my.NumberOfRecordsUpdated,
			FormattedRecords:       nestedRDSDataMySQLExecutorMarker,
		}
	}
	meta := make([]store.RDSDataColumnMeta, 0, len(my.Columns))
	for _, name := range my.Columns {
		meta = append(meta, store.RDSDataColumnMeta{
			Name:     name,
			TypeName: "VARCHAR",
			Label:    name,
		})
	}
	records := make([][]store.RDSDataField, 0, len(my.Rows))
	for _, row := range my.Rows {
		fields := make([]store.RDSDataField, len(meta))
		for i := range meta {
			null := false
			var val string
			if i < len(row) {
				val = row[i]
			}
			v := val
			fields[i] = store.RDSDataField{StringValue: &v, IsNull: &null}
		}
		records = append(records, fields)
	}
	return store.RDSDataExecuteResult{
		ColumnMetadata:   meta,
		Records:          records,
		FormattedRecords: nestedRDSDataMySQLExecutorMarker,
	}
}

func isMySQLSelectSQL(sqlText string) bool {
	upper := strings.ToUpper(strings.TrimSpace(sqlText))
	return strings.HasPrefix(upper, "SELECT") || strings.HasPrefix(upper, "WITH")
}

func isRDSDataMySQLDialFailure(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, store.ErrRDSDataUnavailable) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "nested mysql dial") ||
		strings.Contains(msg, "dial") ||
		strings.Contains(msg, "connect") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "i/o timeout")
}

func rdsDataDatabaseName(inst store.RDSDBInstance, reqDB, defaultEngineDB string) string {
	db := strings.TrimSpace(reqDB)
	if db == "" {
		db = inst.DBName
	}
	if db == "" {
		db = defaultEngineDB
	}
	return db
}

// validateNestedRDSDataHost allows only DinD nested data-plane hostnames (all RDS SQL engines).
func validateNestedRDSDataHost(host string) error {
	host = strings.TrimSpace(host)
	if host == "" {
		return fmt.Errorf("%w: nested data-plane endpoint is empty", store.ErrRDSDataUnavailable)
	}
	lower := strings.ToLower(host)
	switch lower {
	case "localhost", "127.0.0.1", "::1", "0.0.0.0", "*", "host.docker.internal":
		return fmt.Errorf("%w: refusing non-nested host %q", store.ErrRDSDataBadRequest, host)
	}
	if strings.Contains(host, "/") || strings.Contains(host, "\\") {
		return fmt.Errorf("%w: invalid nested data-plane host", store.ErrRDSDataBadRequest)
	}
	if ip := net.ParseIP(host); ip != nil {
		return fmt.Errorf("%w: refusing IP host %q (nested container DNS name required)", store.ErrRDSDataBadRequest, host)
	}
	if !strings.HasPrefix(lower, "noctaxris-data-rds-") {
		return fmt.Errorf("%w: nested host %q is not a data-plane endpoint", store.ErrRDSDataBadRequest, host)
	}
	return nil
}
