package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

var (
	ErrRDSDataBadRequest         = errors.New("BadRequestException")
	ErrRDSDataNotFound           = errors.New("DatabaseNotFoundException")
	ErrRDSDataInvalidSecret      = errors.New("InvalidSecretException")
	ErrRDSDataUnavailable        = errors.New("DatabaseUnavailableException")
	ErrRDSDataTxnNotFound        = errors.New("TransactionNotFoundException")
	ErrRDSDataSecretsError       = errors.New("SecretsErrorException")
)

const rdsDataSchema = `
CREATE TABLE IF NOT EXISTS rds_data_statements (
  account_id TEXT NOT NULL,
  statement_id TEXT NOT NULL,
  resource_arn TEXT NOT NULL,
  secret_arn TEXT NOT NULL,
  database_name TEXT NOT NULL DEFAULT '',
  sql_text TEXT NOT NULL,
  transaction_id TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, statement_id)
);
CREATE TABLE IF NOT EXISTS rds_data_transactions (
  account_id TEXT NOT NULL,
  transaction_id TEXT NOT NULL,
  resource_arn TEXT NOT NULL,
  secret_arn TEXT NOT NULL,
  database_name TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, transaction_id)
);
`

// RDSDataSqlParameter is one ExecuteStatement bind parameter (lab subset).
// pgx binds these when the nested wire DSN is reachable; nested-psql falls back
// to type-safe literal substitution for the same shapes.
type RDSDataSqlParameter struct {
	Name         string
	StringValue  *string
	LongValue    *int64
	DoubleValue  *float64
	BooleanValue *bool
	BlobValue    []byte
	IsNull       *bool
}

// RDSDataExecuteRequest is the Data API ExecuteStatement input (lab subset).
type RDSDataExecuteRequest struct {
	ResourceARN   string
	SecretARN     string
	Database      string
	SQL           string
	TransactionID string
	Parameters    []RDSDataSqlParameter
}

// RDSDataField is one cell in an AWS Data API records array.
type RDSDataField struct {
	StringValue  *string
	LongValue    *int64
	DoubleValue  *float64
	BooleanValue *bool
	// BlobValue is standard base64 (Data API blobValue).
	BlobValue *string
	IsNull    *bool
}

// RDSDataColumnMeta is column metadata returned by ExecuteStatement.
type RDSDataColumnMeta struct {
	Name     string
	TypeName string
	Label    string
}

// RDSDataExecuteResult is the ExecuteStatement result shape.
type RDSDataExecuteResult struct {
	ColumnMetadata         []RDSDataColumnMeta
	Records                [][]RDSDataField
	NumberOfRecordsUpdated int64
	FormattedRecords       string
}

// RDSDataExecutor runs SQL against a nested engine (or a stub).
// Implementations return typed column metadata, records, and NumberOfRecordsUpdated
// when they can; nested-psql maps SELECT cells as VARCHAR strings only; pgx maps OIDs.
type RDSDataExecutor interface {
	Execute(req RDSDataExecuteRequest) (RDSDataExecuteResult, error)
}

// StubRDSDataExecutor records statements and returns canned SELECT-shaped results.
// Only used when tests inject SetRDSDataExecutor. Production ExecuteStatement fails
// closed with DatabaseUnavailableException when no nested Postgres container exists.
// Live SQL prefers pgx against the nested data-plane DSN, else DinD exec + psql.
type StubRDSDataExecutor struct {
	mu         sync.Mutex
	Statements []RDSDataExecuteRequest
}

// Execute records the statement and returns a deterministic stub result.
func (e *StubRDSDataExecutor) Execute(req RDSDataExecuteRequest) (RDSDataExecuteResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.Statements = append(e.Statements, req)
	sqlUpper := strings.ToUpper(strings.TrimSpace(req.SQL))
	if strings.HasPrefix(sqlUpper, "SELECT") {
		label := "stub"
		val := "ok"
		null := false
		return RDSDataExecuteResult{
			ColumnMetadata: []RDSDataColumnMeta{
				{Name: label, TypeName: "VARCHAR", Label: label},
			},
			Records: [][]RDSDataField{
				{{StringValue: &val, IsNull: &null}},
			},
			FormattedRecords: `{"noctaxrisExecutor":"stub"}`,
		}, nil
	}
	return RDSDataExecuteResult{
		NumberOfRecordsUpdated: 1,
		FormattedRecords:       `{"noctaxrisExecutor":"stub"}`,
	}, nil
}

// EnsureRDSDataSchema creates RDS Data API tables if missing.
func EnsureRDSDataSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure rds-data schema: db is nil")
	}
	if _, err := db.Exec(rdsDataSchema); err != nil {
		return fmt.Errorf("ensure rds-data schema: %w", err)
	}
	return nil
}

// EnsureRDSDataSchema ensures RDS Data API tables on an open store.
func (s *Store) EnsureRDSDataSchema() error {
	return EnsureRDSDataSchema(s.db)
}

// ResolveRDSDataResource validates resourceArn + secretArn against the RDS instance.
func (s *Store) ResolveRDSDataResource(accountID, resourceARN, secretARN string) (RDSDBInstance, error) {
	resourceARN = strings.TrimSpace(resourceARN)
	secretARN = strings.TrimSpace(secretARN)
	if resourceARN == "" || secretARN == "" {
		return RDSDBInstance{}, fmt.Errorf("%w: resourceArn and secretArn are required", ErrRDSDataBadRequest)
	}
	inst, err := s.DescribeRDSDBInstanceByARN(accountID, resourceARN)
	if err != nil {
		if errors.Is(err, ErrRDSInstanceNotFound) {
			return RDSDBInstance{}, ErrRDSDataNotFound
		}
		return RDSDBInstance{}, err
	}
	if _, err := s.ResolveDataPlaneSecretARN(accountID, secretARN); err != nil {
		if errors.Is(err, ErrSecretNotFound) || errors.Is(err, ErrSecretScheduledDeletion) {
			return RDSDBInstance{}, ErrRDSDataSecretsError
		}
		return RDSDBInstance{}, fmt.Errorf("%w: %v", ErrRDSDataInvalidSecret, err)
	}
	if inst.MasterUserSecretARN != "" && !secretARNsMatch(inst.MasterUserSecretARN, secretARN) {
		return RDSDBInstance{}, ErrRDSDataInvalidSecret
	}
	return inst, nil
}

func secretARNsMatch(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == b {
		return true
	}
	// Secrets Manager ARNs include a random suffix; compare by name segment when needed.
	return rdsDataSecretNameFromARN(a) != "" && rdsDataSecretNameFromARN(a) == rdsDataSecretNameFromARN(b)
}

func rdsDataSecretNameFromARN(arn string) string {
	const marker = ":secret:"
	i := strings.Index(arn, marker)
	if i < 0 {
		return ""
	}
	name := arn[i+len(marker):]
	if j := strings.LastIndex(name, "-"); j > 0 {
		// strip 6-char random suffix when present
		if len(name)-j-1 == 6 {
			return name[:j]
		}
	}
	return name
}

// RecordRDSDataStatement persists an executed statement for lab audit/tests.
func (s *Store) RecordRDSDataStatement(accountID string, req RDSDataExecuteRequest) error {
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO rds_data_statements
		 (account_id, statement_id, resource_arn, secret_arn, database_name, sql_text, transaction_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		accountID, uuid.NewString(), req.ResourceARN, req.SecretARN, req.Database, req.SQL, req.TransactionID, now,
	)
	if err != nil {
		return fmt.Errorf("record rds-data statement: %w", err)
	}
	return nil
}

// BeginRDSDataTransaction creates an in-memory/sqlite transaction id.
func (s *Store) BeginRDSDataTransaction(accountID, resourceARN, secretARN, database string) (string, error) {
	if _, err := s.ResolveRDSDataResource(accountID, resourceARN, secretARN); err != nil {
		return "", err
	}
	txnID := uuid.NewString()
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO rds_data_transactions
		 (account_id, transaction_id, resource_arn, secret_arn, database_name, status, created_at)
		 VALUES (?, ?, ?, ?, ?, 'active', ?)`,
		accountID, txnID, resourceARN, secretARN, database, now,
	)
	if err != nil {
		return "", fmt.Errorf("begin rds-data txn: %w", err)
	}
	return txnID, nil
}

// FinishRDSDataTransaction commits or rolls back a lab transaction id.
func (s *Store) FinishRDSDataTransaction(accountID, transactionID, status string) error {
	transactionID = strings.TrimSpace(transactionID)
	if transactionID == "" {
		return fmt.Errorf("%w: transactionId is required", ErrRDSDataBadRequest)
	}
	var cur string
	err := s.db.QueryRow(
		`SELECT status FROM rds_data_transactions WHERE account_id = ? AND transaction_id = ?`,
		accountID, transactionID,
	).Scan(&cur)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrRDSDataTxnNotFound
	}
	if err != nil {
		return fmt.Errorf("finish rds-data txn: %w", err)
	}
	if cur != "active" {
		return fmt.Errorf("%w: transaction is not active", ErrRDSDataBadRequest)
	}
	_, err = s.db.Exec(
		`UPDATE rds_data_transactions SET status = ? WHERE account_id = ? AND transaction_id = ?`,
		status, accountID, transactionID,
	)
	if err != nil {
		return fmt.Errorf("finish rds-data txn: update: %w", err)
	}
	return nil
}
