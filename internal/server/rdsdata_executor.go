package server

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/compute"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const nestedRDSDataExecutorMarker = `{"noctaxrisExecutor":"nested-psql"}`

// EnvRDSDataPgx enables the optional wire-protocol Data API executor after
// github.com/jackc/pgx is approved and linked. Without that dependency this
// flag never activates a live path (rdsDataPgxEnabled stays false).
const EnvRDSDataPgx = "NOCTAXRIS_RDS_DATA_PGX"

// preferNestedRDSDataExecute selects a Data API executor:
//  1. test override (SetRDSDataExecutor)
//  2. optional pgx wire path when buy-in lands and rdsDataPgxEnabled()
//  3. nested DinD psql when the instance has a container
//  4. DatabaseUnavailableException
//
// Production never fakes SELECT success without a nested Postgres container.
func (s *Server) preferNestedRDSDataExecute(
	ctx context.Context,
	accountID string,
	inst store.RDSDBInstance,
	req store.RDSDataExecuteRequest,
) (store.RDSDataExecuteResult, error) {
	if override := getRDSDataExecutorOverride(); override != nil {
		return override.Execute(req)
	}
	if rdsDataPgxEnabled() {
		return s.executeRDSDataPgx(ctx, accountID, inst, req)
	}
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
	runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	pgRes, err := cli.ExecPostgresSQL(runCtx, compute.PostgresSQLOpts{
		ContainerID: inst.ContainerID,
		Username:    user,
		Password:    password,
		Database:    db,
		SQL:         req.SQL,
	})
	if err != nil {
		return store.RDSDataExecuteResult{}, err
	}
	return mapPostgresSQLResult(pgRes), nil
}

// rdsDataPgxEnabled reports whether the optional wire-protocol executor should
// run. Always false until pgx is approved in go.mod and a real implementation
// replaces executeRDSDataPgx. EnvRDSDataPgx alone does not enable anything.
func rdsDataPgxEnabled() bool {
	return false
}

// executeRDSDataPgx is the reserved wire-protocol path. Without a linked pgx
// driver this always fails closed so callers never see a fake typed result.
func (s *Server) executeRDSDataPgx(
	_ context.Context,
	_ string,
	_ store.RDSDBInstance,
	_ store.RDSDataExecuteRequest,
) (store.RDSDataExecuteResult, error) {
	return store.RDSDataExecuteResult{}, store.ErrRDSDataUnavailable
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
		user = "postgres"
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
