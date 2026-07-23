package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

const maxLabTransactItems = 25

var (
	// ErrDynamoTransactUnsupported is returned for Update and other fail-closed ops.
	ErrDynamoTransactUnsupported = errors.New("ValidationException: unsupported TransactWriteItems action")
	// ErrDynamoTransactLimit is returned when TransactItems exceeds the lab cap.
	ErrDynamoTransactLimit = errors.New("ValidationException: TransactItems exceeds lab limit of 25")
	// ErrDynamoTransactEmpty is returned when TransactItems is empty.
	ErrDynamoTransactEmpty = errors.New("ValidationException: TransactItems is required")
)

// CancellationReason is one entry in a TransactionCanceledException payload.
type CancellationReason struct {
	Code    string
	Message string
}

// TransactionCanceledError is the lab equivalent of TransactionCanceledException.
type TransactionCanceledError struct {
	Reasons []CancellationReason
}

func (e *TransactionCanceledError) Error() string {
	if e == nil {
		return "TransactionCanceledException"
	}
	return "TransactionCanceledException"
}

// TransactWriteAction is one Put, Delete, or ConditionCheck inside a lab transaction.
type TransactWriteAction struct {
	Kind           string // Put | Delete | ConditionCheck | Update (fail closed)
	TableName      string
	ItemPK, ItemSK string
	ItemJSON       []byte // Put payload (plaintext or sealed)
	GSIPK, GSISK   string
	GSI2PK, GSI2SK string
	Sealed         bool
	SealedDEK      []byte
	ConditionEmpty bool // ConditionCheck: require item existence
}

// TransactGetKey identifies one item for TransactGetItems.
type TransactGetKey struct {
	TableName      string
	ItemPK, ItemSK string
}

// TransactWriteItems applies Put/Delete/ConditionCheck atomically (SQLite BEGIN IMMEDIATE).
// Same-account tables only. Duplicate primary keys cancel the entire request.
// Update and other unsupported kinds fail closed.
func (s *Store) TransactWriteItems(accountID string, actions []TransactWriteAction) error {
	if len(actions) == 0 {
		return ErrDynamoTransactEmpty
	}
	if len(actions) > maxLabTransactItems {
		return ErrDynamoTransactLimit
	}
	for _, a := range actions {
		switch strings.TrimSpace(a.Kind) {
		case "Put", "Delete", "ConditionCheck":
		case "Update":
			return ErrDynamoTransactUnsupported
		default:
			return fmt.Errorf("%w: %s", ErrDynamoTransactUnsupported, a.Kind)
		}
		if strings.TrimSpace(a.TableName) == "" {
			return fmt.Errorf("%w: TableName is required", ErrDynamoTransactEmpty)
		}
	}
	if err := validateTransactNoDuplicateKeys(actions); err != nil {
		return err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transact write: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	reasons := make([]CancellationReason, len(actions))
	for i := range reasons {
		reasons[i] = CancellationReason{Code: "None"}
	}
	for i, a := range actions {
		if err := s.applyTransactWriteAction(tx, accountID, a); err != nil {
			code, msg := transactCancelFromErr(err)
			reasons[i] = CancellationReason{Code: code, Message: msg}
			return &TransactionCanceledError{Reasons: reasons}
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transact write: %w", err)
	}
	return nil
}

// TransactGetItems returns item JSON bytes (or nil when missing) in request order.
func (s *Store) TransactGetItems(accountID string, keys []TransactGetKey) ([][]byte, error) {
	if len(keys) == 0 {
		return nil, ErrDynamoTransactEmpty
	}
	if len(keys) > maxLabTransactItems {
		return nil, ErrDynamoTransactLimit
	}
	out := make([][]byte, len(keys))
	for i, k := range keys {
		if strings.TrimSpace(k.TableName) == "" {
			return nil, fmt.Errorf("%w: TableName is required", ErrDynamoTransactEmpty)
		}
		if _, err := s.GetTable(accountID, k.TableName); err != nil {
			return nil, err
		}
		it, err := s.GetItemBytes(accountID, k.TableName, k.ItemPK, k.ItemSK)
		if errors.Is(err, ErrNoSuchItem) {
			out[i] = nil
			continue
		}
		if err != nil {
			return nil, err
		}
		out[i] = it.ItemJSON
	}
	return out, nil
}

func validateTransactNoDuplicateKeys(actions []TransactWriteAction) error {
	seen := map[string]int{}
	reasons := make([]CancellationReason, len(actions))
	for i := range reasons {
		reasons[i] = CancellationReason{Code: "None"}
	}
	dup := false
	for i, a := range actions {
		key := a.TableName + "\x00" + a.ItemPK + "\x00" + a.ItemSK
		if prev, ok := seen[key]; ok {
			dup = true
			reasons[prev] = CancellationReason{
				Code:    "ValidationError",
				Message: "Transaction request cannot include multiple operations on one item",
			}
			reasons[i] = CancellationReason{
				Code:    "ValidationError",
				Message: "Transaction request cannot include multiple operations on one item",
			}
			continue
		}
		seen[key] = i
	}
	if dup {
		return &TransactionCanceledError{Reasons: reasons}
	}
	return nil
}

func (s *Store) applyTransactWriteAction(tx *sql.Tx, accountID string, a TransactWriteAction) error {
	var n int
	err := tx.QueryRow(
		`SELECT COUNT(1) FROM dynamodb_tables WHERE account_id = ? AND table_name = ?`,
		accountID, a.TableName,
	).Scan(&n)
	if err != nil {
		return fmt.Errorf("lookup table: %w", err)
	}
	if n == 0 {
		return ErrNoSuchTable
	}

	switch a.Kind {
	case "ConditionCheck":
		if !a.ConditionEmpty {
			return fmt.Errorf("%w: ConditionCheck requires lab existence check", ErrDynamoTransactUnsupported)
		}
		var exists int
		err := tx.QueryRow(
			`SELECT COUNT(1) FROM dynamodb_items
			 WHERE account_id = ? AND table_name = ? AND item_pk = ? AND item_sk = ?`,
			accountID, a.TableName, a.ItemPK, a.ItemSK,
		).Scan(&exists)
		if err != nil {
			return fmt.Errorf("condition check: %w", err)
		}
		if exists == 0 {
			return errTransactConditionalCheckFailed
		}
		return nil
	case "Delete":
		_, err := tx.Exec(
			`DELETE FROM dynamodb_items
			 WHERE account_id = ? AND table_name = ? AND item_pk = ? AND item_sk = ?`,
			accountID, a.TableName, a.ItemPK, a.ItemSK,
		)
		if err != nil {
			return fmt.Errorf("transact delete: %w", err)
		}
		return nil
	case "Put":
		sealedFlag := 0
		if a.Sealed {
			sealedFlag = 1
		}
		_, err := tx.Exec(
			`INSERT INTO dynamodb_items
			 (account_id, table_name, item_pk, item_sk, gsi_pk, gsi_sk, gsi2_pk, gsi2_sk, item_json, sealed, sealed_dek)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(account_id, table_name, item_pk, item_sk) DO UPDATE SET
			   gsi_pk = excluded.gsi_pk,
			   gsi_sk = excluded.gsi_sk,
			   gsi2_pk = excluded.gsi2_pk,
			   gsi2_sk = excluded.gsi2_sk,
			   item_json = excluded.item_json,
			   sealed = excluded.sealed,
			   sealed_dek = excluded.sealed_dek`,
			accountID, a.TableName, a.ItemPK, a.ItemSK,
			a.GSIPK, a.GSISK, a.GSI2PK, a.GSI2SK,
			a.ItemJSON, sealedFlag, a.SealedDEK,
		)
		if err != nil {
			return fmt.Errorf("transact put: %w", err)
		}
		return nil
	default:
		return ErrDynamoTransactUnsupported
	}
}

var errTransactConditionalCheckFailed = errors.New("ConditionalCheckFailed")

func transactCancelFromErr(err error) (code, message string) {
	if errors.Is(err, errTransactConditionalCheckFailed) {
		return "ConditionalCheckFailed", "The conditional request failed"
	}
	if errors.Is(err, ErrNoSuchTable) {
		return "ResourceNotFound", "Requested resource not found"
	}
	if errors.Is(err, ErrDynamoTransactUnsupported) {
		return "ValidationError", err.Error()
	}
	return "ValidationError", err.Error()
}
