package server

import (
	"context"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/compute"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const nestedRDSDataExecutorMarker = `{"noctaxrisExecutor":"nested-psql"}`

// EnvRDSDataPgx controls the wire-protocol Data API executor for Postgres and
// MySQL/MariaDB. Default (unset): prefer wire dial when a nested data-plane DSN
// can be built; fall back to nested CLI on dial failure. Set to 0/false/off to
// force nested CLI for auto-commit Execute/Batch.
const EnvRDSDataPgx = "NOCTAXRIS_RDS_DATA_PGX"

// preferNestedRDSDataExecute selects a Data API executor:
//  1. test override (SetRDSDataExecutor)
//  2. mysql/mariadb: go-sql-driver/mysql or nested mysql CLI
//  3. postgres: pgx when enabled and nested DSN is available (typed fields + binds)
//  4. nested DinD psql when the instance has a container (VARCHAR cells; literal params)
//  5. DatabaseUnavailableException
//
// Production never fakes SELECT success without a nested engine container.
// Wire dials only the nested data-plane hostname (never host-published ports).
func (s *Server) preferNestedRDSDataExecute(
	ctx context.Context,
	accountID string,
	inst store.RDSDBInstance,
	req store.RDSDataExecuteRequest,
) (store.RDSDataExecuteResult, error) {
	if override := getRDSDataExecutorOverride(); override != nil {
		return override.Execute(req)
	}
	switch store.NormalizeRDSEngine(inst.Engine) {
	case "mysql", "mariadb":
		return s.preferNestedRDSDataExecuteMySQL(ctx, accountID, inst, req)
	case "postgres":
	default:
		return store.RDSDataExecuteResult{}, store.ErrRDSDataUnavailable
	}
	if rdsDataPgxEnabled() {
		res, err := s.executeRDSDataPgx(ctx, accountID, inst, req)
		if err == nil {
			return res, nil
		}
		// Dial / DSN-unavailable: fall back to nested-psql. SQL errors from a
		// successful wire session are not dial failures and must surface.
		if !isRDSDataPgxDialFailure(err) {
			return store.RDSDataExecuteResult{}, err
		}
	}
	return s.executeRDSDataNestedPsql(ctx, accountID, inst, req)
}

func (s *Server) executeRDSDataNestedPsql(
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
	db := strings.TrimSpace(req.Database)
	if db == "" {
		db = inst.DBName
	}
	if db == "" {
		db = "postgres"
	}
	sqlText := req.SQL
	if len(req.Parameters) > 0 {
		rewritten, rewriteErr := applyRDSDataParametersAsLiterals(sqlText, req.Parameters)
		if rewriteErr != nil {
			return store.RDSDataExecuteResult{}, rewriteErr
		}
		sqlText = rewritten
	}
	runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	pgRes, err := cli.ExecPostgresSQL(runCtx, compute.PostgresSQLOpts{
		ContainerID: inst.ContainerID,
		Username:    user,
		Password:    password,
		Database:    db,
		SQL:         sqlText,
	})
	if err != nil {
		return store.RDSDataExecuteResult{}, err
	}
	return mapPostgresSQLResult(pgRes), nil
}

func (s *Server) rdsDataMasterCreds(accountID string, inst store.RDSDBInstance, secretARN string) (user, password string, err error) {
	user = inst.MasterUsername
	arn := strings.TrimSpace(secretARN)
	if arn == "" {
		arn = inst.MasterUserSecretARN
	}
	if arn == "" {
		return "", "", fmt.Errorf("%w: secretArn is required for nested SQL", store.ErrRDSDataInvalidSecret)
	}
	sec, err := s.store.GetSecretValue(accountID, arn)
	if err != nil {
		return "", "", fmt.Errorf("%w: %v", store.ErrRDSDataSecretsError, err)
	}
	u, p, err := store.ParseRDSMasterSecret(sec.SecretString)
	if err != nil {
		return "", "", fmt.Errorf("%w: %v", store.ErrRDSDataInvalidSecret, err)
	}
	if u != "" {
		user = u
	}
	if p == "" {
		return "", "", fmt.Errorf("%w: secret has empty password", store.ErrRDSDataInvalidSecret)
	}
	if user == "" {
		user = store.DefaultRDSMasterUsername(inst.Engine)
	}
	return user, p, nil
}

func mapPostgresSQLResult(pg compute.PostgresSQLResult) store.RDSDataExecuteResult {
	if !pg.Select {
		return store.RDSDataExecuteResult{
			NumberOfRecordsUpdated: pg.NumberOfRecordsUpdated,
			FormattedRecords:       nestedRDSDataExecutorMarker,
		}
	}
	meta := make([]store.RDSDataColumnMeta, 0, len(pg.Columns))
	for _, name := range pg.Columns {
		meta = append(meta, store.RDSDataColumnMeta{
			Name:     name,
			TypeName: "VARCHAR",
			Label:    name,
		})
	}
	records := make([][]store.RDSDataField, 0, len(pg.Rows))
	for _, row := range pg.Rows {
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
		FormattedRecords: nestedRDSDataExecutorMarker,
	}
}

// applyRDSDataParametersAsLiterals rewrites :name placeholders to typed SQL
// literals for the nested-psql fallback (no string-concatenated identifiers).
func applyRDSDataParametersAsLiterals(sqlText string, params []store.RDSDataSqlParameter) (string, error) {
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
		lit, err := rdsDataParameterLiteral(p)
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

func rdsDataParameterLiteral(p store.RDSDataSqlParameter) (string, error) {
	if p.IsNull != nil && *p.IsNull {
		return "NULL", nil
	}
	switch {
	case p.StringValue != nil:
		return quotePostgresStringLiteral(*p.StringValue), nil
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
		return `'\x` + hex.EncodeToString(p.BlobValue) + `'::bytea`, nil
	default:
		return "", fmt.Errorf("%w: parameter %q has no supported value", store.ErrRDSDataBadRequest, p.Name)
	}
}

func quotePostgresStringLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
