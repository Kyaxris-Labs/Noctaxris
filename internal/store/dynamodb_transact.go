package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	maxLabTransactItems = 25
	// labTransactClientTokenTTL matches AWS ClientRequestToken validity (10 minutes).
	labTransactClientTokenTTL = 10 * time.Minute
)

var (
	// ErrDynamoTransactUnsupported is returned for fail-closed TransactWrite ops.
	ErrDynamoTransactUnsupported = errors.New("ValidationException: unsupported TransactWriteItems action")
	// ErrDynamoTransactLimit is returned when TransactItems exceeds the lab cap.
	ErrDynamoTransactLimit = errors.New("ValidationException: TransactItems exceeds lab limit of 25")
	// ErrDynamoTransactEmpty is returned when TransactItems is empty.
	ErrDynamoTransactEmpty = errors.New("ValidationException: TransactItems is required")
	// ErrDynamoTransactSealed is returned when Update/conditions need plaintext of a sealed item.
	ErrDynamoTransactSealed = errors.New("ValidationException: TransactWriteItems Update and ConditionExpression require unsealed items in this lab")
	// ErrDynamoTransactIdempotentMismatch is returned when ClientRequestToken is reused with different parameters.
	ErrDynamoTransactIdempotentMismatch = errors.New("IdempotentParameterMismatchException")
)

const dynamodbTransactTokenSchema = `
CREATE TABLE IF NOT EXISTS dynamodb_transact_tokens (
  account_id TEXT NOT NULL,
  client_request_token TEXT NOT NULL,
  request_hash TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, client_request_token)
);
`

type pendingDynamoStreamAppend struct {
	tableName, eventName       string
	keysJSON, newImage, oldImage []byte
}

// DynamoAVMap is an AttributeValue map used by registered Transact expression helpers.
type DynamoAVMap map[string]map[string]any

type transactExprFns struct {
	evalCondition func(item DynamoAVMap, expr string, names, values map[string]any) error
	applyUpdate   func(item, key DynamoAVMap, updateExpr string, names, values map[string]any) (DynamoAVMap, error)
}

var (
	transactExprMu sync.RWMutex
	transactExprs  *transactExprFns
)

// RegisterTransactExprFns wires lab ConditionExpression / UpdateExpression helpers.
// Called from services/dynamodb init (store cannot import that package).
func RegisterTransactExprFns(
	evalCondition func(item DynamoAVMap, expr string, names, values map[string]any) error,
	applyUpdate func(item, key DynamoAVMap, updateExpr string, names, values map[string]any) (DynamoAVMap, error),
) {
	transactExprMu.Lock()
	defer transactExprMu.Unlock()
	transactExprs = &transactExprFns{evalCondition: evalCondition, applyUpdate: applyUpdate}
}

func getTransactExprFns() (*transactExprFns, error) {
	transactExprMu.RLock()
	defer transactExprMu.RUnlock()
	if transactExprs == nil || transactExprs.evalCondition == nil || transactExprs.applyUpdate == nil {
		return nil, fmt.Errorf("%w: expression helpers not registered", ErrDynamoTransactUnsupported)
	}
	return transactExprs, nil
}

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

// TransactWriteAction is one Put, Delete, ConditionCheck, or Update inside a lab transaction.
type TransactWriteAction struct {
	Kind           string // Put | Delete | ConditionCheck | Update
	TableName      string
	ItemPK, ItemSK string
	ItemJSON       []byte // Put payload (plaintext or sealed)
	GSIPK, GSISK   string
	GSI2PK, GSI2SK string
	Sealed         bool
	SealedDEK      []byte
	ConditionEmpty bool // ConditionCheck: require item existence

	ConditionExpression       string
	ExpressionAttributeNames  map[string]any
	ExpressionAttributeValues map[string]any
	UpdateExpression          string
	KeyJSON                   []byte // Update: Key AttributeValue map JSON
}

// TransactGetKey identifies one item for TransactGetItems.
type TransactGetKey struct {
	TableName      string
	ItemPK, ItemSK string
}

// EnsureDynamoDBTransactSchema creates ClientRequestToken idempotency storage.
func EnsureDynamoDBTransactSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure dynamodb transact schema: db is nil")
	}
	if _, err := db.Exec(dynamodbTransactTokenSchema); err != nil {
		return fmt.Errorf("ensure dynamodb transact schema: %w", err)
	}
	return nil
}

// TransactWriteItems applies Put/Delete/ConditionCheck/Update atomically (SQLite BEGIN).
// Same-account tables only. Duplicate primary keys cancel the entire request.
// Put/Delete/Update honor the lab ConditionExpression subset; unsupported operators
// fail closed with ValidationException before the transaction applies.
// Optional clientRequestToken provides lab idempotency (10-minute window; mismatch → ErrDynamoTransactIdempotentMismatch).
// Successful Put/Delete/Update append stream records when the table stream is enabled.
func (s *Store) TransactWriteItems(accountID string, actions []TransactWriteAction, clientRequestToken string) error {
	if len(actions) == 0 {
		return ErrDynamoTransactEmpty
	}
	if len(actions) > maxLabTransactItems {
		return ErrDynamoTransactLimit
	}
	for _, a := range actions {
		switch strings.TrimSpace(a.Kind) {
		case "Put", "Delete", "ConditionCheck", "Update":
		default:
			return fmt.Errorf("%w: %s", ErrDynamoTransactUnsupported, a.Kind)
		}
		if strings.TrimSpace(a.TableName) == "" {
			return fmt.Errorf("%w: TableName is required", ErrDynamoTransactEmpty)
		}
	}
	if err := validateTransactExpressions(actions); err != nil {
		return err
	}
	if err := validateTransactNoDuplicateKeys(actions); err != nil {
		return err
	}

	token := strings.TrimSpace(clientRequestToken)
	reqHash := ""
	if token != "" {
		reqHash = transactActionsHash(actions)
		replay, err := s.checkTransactClientToken(accountID, token, reqHash)
		if err != nil {
			return err
		}
		if replay {
			return nil
		}
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
	pending := make([]pendingDynamoStreamAppend, 0, len(actions))
	for i, a := range actions {
		streamRec, err := s.applyTransactWriteAction(tx, accountID, a)
		if err != nil {
			if errors.Is(err, ErrDynamoTransactSealed) || errors.Is(err, ErrDynamoTransactUnsupported) ||
				strings.HasPrefix(err.Error(), "ValidationException:") {
				return err
			}
			code, msg := transactCancelFromErr(err)
			reasons[i] = CancellationReason{Code: code, Message: msg}
			return &TransactionCanceledError{Reasons: reasons}
		}
		if streamRec != nil {
			pending = append(pending, *streamRec)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transact write: %w", err)
	}
	for _, rec := range pending {
		if err := s.AppendDynamoStreamRecord(accountID, rec.tableName, rec.eventName, rec.keysJSON, rec.newImage, rec.oldImage); err != nil {
			return fmt.Errorf("transact stream append: %w", err)
		}
	}
	if token != "" {
		if err := s.storeTransactClientToken(accountID, token, reqHash); err != nil {
			return err
		}
	}
	return nil
}

func transactActionsHash(actions []TransactWriteAction) string {
	type wire struct {
		Kind, TableName, ItemPK, ItemSK string
		ItemJSON                        string
		GSIPK, GSISK, GSI2PK, GSI2SK    string
		Sealed                          bool
		ConditionEmpty                  bool
		ConditionExpression             string
		ExpressionAttributeNames        map[string]any
		ExpressionAttributeValues       map[string]any
		UpdateExpression                string
		KeyJSON                         string
	}
	out := make([]wire, 0, len(actions))
	for _, a := range actions {
		out = append(out, wire{
			Kind: a.Kind, TableName: a.TableName, ItemPK: a.ItemPK, ItemSK: a.ItemSK,
			ItemJSON: string(a.ItemJSON), GSIPK: a.GSIPK, GSISK: a.GSISK, GSI2PK: a.GSI2PK, GSI2SK: a.GSI2SK,
			Sealed: a.Sealed, ConditionEmpty: a.ConditionEmpty,
			ConditionExpression: a.ConditionExpression,
			ExpressionAttributeNames: a.ExpressionAttributeNames, ExpressionAttributeValues: a.ExpressionAttributeValues,
			UpdateExpression: a.UpdateExpression, KeyJSON: string(a.KeyJSON),
		})
	}
	raw, _ := json.Marshal(out)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (s *Store) checkTransactClientToken(accountID, token, reqHash string) (replay bool, err error) {
	var storedHash string
	var createdAt int64
	err = s.db.QueryRow(
		`SELECT request_hash, created_at FROM dynamodb_transact_tokens
		 WHERE account_id = ? AND client_request_token = ?`,
		accountID, token,
	).Scan(&storedHash, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check transact client token: %w", err)
	}
	if time.Now().UTC().Unix()-createdAt > int64(labTransactClientTokenTTL.Seconds()) {
		_, _ = s.db.Exec(
			`DELETE FROM dynamodb_transact_tokens WHERE account_id = ? AND client_request_token = ?`,
			accountID, token,
		)
		return false, nil
	}
	if storedHash != reqHash {
		return false, ErrDynamoTransactIdempotentMismatch
	}
	return true, nil
}

func (s *Store) storeTransactClientToken(accountID, token, reqHash string) error {
	now := time.Now().UTC().Unix()
	_, err := s.db.Exec(
		`INSERT INTO dynamodb_transact_tokens (account_id, client_request_token, request_hash, created_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(account_id, client_request_token) DO UPDATE SET
		   request_hash = excluded.request_hash,
		   created_at = excluded.created_at`,
		accountID, token, reqHash, now,
	)
	if err != nil {
		return fmt.Errorf("store transact client token: %w", err)
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

func validateTransactExpressions(actions []TransactWriteAction) error {
	needExpr := false
	for _, a := range actions {
		if strings.TrimSpace(a.ConditionExpression) != "" || a.Kind == "Update" {
			needExpr = true
			break
		}
	}
	if !needExpr {
		return nil
	}
	fns, err := getTransactExprFns()
	if err != nil {
		return err
	}
	for _, a := range actions {
		if a.Kind == "Update" {
			if strings.TrimSpace(a.UpdateExpression) == "" {
				return fmt.Errorf("%w: UpdateExpression is required", ErrDynamoTransactUnsupported)
			}
			if len(a.KeyJSON) == 0 {
				return fmt.Errorf("%w: Update Key is required", ErrDynamoTransactUnsupported)
			}
			key, err := unmarshalDynamoAVMap(a.KeyJSON)
			if err != nil {
				return fmt.Errorf("%w: %v", ErrDynamoTransactUnsupported, err)
			}
			if _, err := fns.applyUpdate(DynamoAVMap{}, key, a.UpdateExpression, a.ExpressionAttributeNames, a.ExpressionAttributeValues); err != nil {
				if !isTransactConditionalErr(err) {
					return fmt.Errorf("ValidationException: %v", err)
				}
			}
		}
		if strings.TrimSpace(a.ConditionExpression) == "" {
			continue
		}
		if err := fns.evalCondition(DynamoAVMap{}, a.ConditionExpression, a.ExpressionAttributeNames, a.ExpressionAttributeValues); err != nil {
			if isTransactConditionalErr(err) {
				continue
			}
			return fmt.Errorf("ValidationException: %v", err)
		}
	}
	return nil
}

func (s *Store) applyTransactWriteAction(tx *sql.Tx, accountID string, a TransactWriteAction) (*pendingDynamoStreamAppend, error) {
	var n int
	err := tx.QueryRow(
		`SELECT COUNT(1) FROM dynamodb_tables WHERE account_id = ? AND table_name = ?`,
		accountID, a.TableName,
	).Scan(&n)
	if err != nil {
		return nil, fmt.Errorf("lookup table: %w", err)
	}
	if n == 0 {
		return nil, ErrNoSuchTable
	}

	switch a.Kind {
	case "ConditionCheck":
		if !a.ConditionEmpty {
			return nil, fmt.Errorf("%w: ConditionCheck requires lab existence check", ErrDynamoTransactUnsupported)
		}
		var exists int
		err := tx.QueryRow(
			`SELECT COUNT(1) FROM dynamodb_items
			 WHERE account_id = ? AND table_name = ? AND item_pk = ? AND item_sk = ?`,
			accountID, a.TableName, a.ItemPK, a.ItemSK,
		).Scan(&exists)
		if err != nil {
			return nil, fmt.Errorf("condition check: %w", err)
		}
		if exists == 0 {
			return nil, errTransactConditionalCheckFailed
		}
		return nil, nil
	case "Delete":
		if strings.TrimSpace(a.ConditionExpression) != "" {
			if err := s.evalTransactConditionOnRow(tx, accountID, a); err != nil {
				return nil, err
			}
		}
		oldRaw, oldSealed, err := loadTransactItemRaw(tx, accountID, a.TableName, a.ItemPK, a.ItemSK)
		if err != nil {
			return nil, err
		}
		_, err = tx.Exec(
			`DELETE FROM dynamodb_items
			 WHERE account_id = ? AND table_name = ? AND item_pk = ? AND item_sk = ?`,
			accountID, a.TableName, a.ItemPK, a.ItemSK,
		)
		if err != nil {
			return nil, fmt.Errorf("transact delete: %w", err)
		}
		return s.transactStreamPending(accountID, a, "REMOVE", nil, oldRaw, oldSealed)
	case "Put":
		if strings.TrimSpace(a.ConditionExpression) != "" {
			if err := s.evalTransactConditionOnRow(tx, accountID, a); err != nil {
				return nil, err
			}
		}
		oldRaw, oldSealed, err := loadTransactItemRaw(tx, accountID, a.TableName, a.ItemPK, a.ItemSK)
		if err != nil {
			return nil, err
		}
		eventName := "INSERT"
		if len(oldRaw) > 0 || oldSealed {
			eventName = "MODIFY"
		}
		sealedFlag := 0
		if a.Sealed {
			sealedFlag = 1
		}
		_, err = tx.Exec(
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
			return nil, fmt.Errorf("transact put: %w", err)
		}
		newImg := a.ItemJSON
		if a.Sealed {
			newImg = nil
		}
		return s.transactStreamPending(accountID, a, eventName, newImg, oldRaw, oldSealed)
	case "Update":
		return s.applyTransactUpdate(tx, accountID, a)
	default:
		return nil, ErrDynamoTransactUnsupported
	}
}

func (s *Store) applyTransactUpdate(tx *sql.Tx, accountID string, a TransactWriteAction) (*pendingDynamoStreamAppend, error) {
	fns, err := getTransactExprFns()
	if err != nil {
		return nil, err
	}
	key, err := unmarshalDynamoAVMap(a.KeyJSON)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDynamoTransactUnsupported, err)
	}
	existing, sealed, err := loadTransactItemMap(tx, accountID, a.TableName, a.ItemPK, a.ItemSK)
	if err != nil {
		return nil, err
	}
	if sealed {
		return nil, ErrDynamoTransactSealed
	}
	hadItem := len(existing) > 0
	var oldRaw []byte
	if hadItem {
		oldRaw, err = json.Marshal(existing)
		if err != nil {
			return nil, fmt.Errorf("marshal old item: %w", err)
		}
	}
	if strings.TrimSpace(a.ConditionExpression) != "" {
		if err := fns.evalCondition(existing, a.ConditionExpression, a.ExpressionAttributeNames, a.ExpressionAttributeValues); err != nil {
			if isTransactConditionalErr(err) {
				return nil, errTransactConditionalCheckFailed
			}
			return nil, fmt.Errorf("ValidationException: %v", err)
		}
	}
	updated, err := fns.applyUpdate(existing, key, a.UpdateExpression, a.ExpressionAttributeNames, a.ExpressionAttributeValues)
	if err != nil {
		return nil, fmt.Errorf("ValidationException: %v", err)
	}
	payload, err := json.Marshal(updated)
	if err != nil {
		return nil, fmt.Errorf("marshal updated item: %w", err)
	}
	table, err := s.GetTable(accountID, a.TableName)
	if err != nil {
		return nil, err
	}
	gsiPK, gsiSK, err := transactGSIKeys(table, updated, 1)
	if err != nil {
		return nil, err
	}
	gsi2PK, gsi2SK, err := transactGSIKeys(table, updated, 2)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(
		`INSERT INTO dynamodb_items
		 (account_id, table_name, item_pk, item_sk, gsi_pk, gsi_sk, gsi2_pk, gsi2_sk, item_json, sealed, sealed_dek)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 0, NULL)
		 ON CONFLICT(account_id, table_name, item_pk, item_sk) DO UPDATE SET
		   gsi_pk = excluded.gsi_pk,
		   gsi_sk = excluded.gsi_sk,
		   gsi2_pk = excluded.gsi2_pk,
		   gsi2_sk = excluded.gsi2_sk,
		   item_json = excluded.item_json,
		   sealed = 0,
		   sealed_dek = NULL`,
		accountID, a.TableName, a.ItemPK, a.ItemSK,
		gsiPK, gsiSK, gsi2PK, gsi2SK, payload,
	)
	if err != nil {
		return nil, fmt.Errorf("transact update: %w", err)
	}
	eventName := "INSERT"
	if hadItem {
		eventName = "MODIFY"
	}
	return s.transactStreamPending(accountID, a, eventName, payload, oldRaw, false)
}

func (s *Store) transactStreamPending(
	accountID string, a TransactWriteAction, eventName string, newImage, oldRaw []byte, oldSealed bool,
) (*pendingDynamoStreamAppend, error) {
	table, err := s.GetTable(accountID, a.TableName)
	if err != nil {
		return nil, err
	}
	if !table.StreamEnabled {
		return nil, nil
	}
	keysJSON, err := DynamoStreamKeysJSON(table, a.ItemPK, a.ItemSK)
	if err != nil {
		return nil, err
	}
	var oldImage []byte
	if !oldSealed && len(oldRaw) > 0 {
		oldImage = oldRaw
	}
	return &pendingDynamoStreamAppend{
		tableName: a.TableName,
		eventName: eventName,
		keysJSON:  keysJSON,
		newImage:  newImage,
		oldImage:  oldImage,
	}, nil
}

func loadTransactItemRaw(tx *sql.Tx, accountID, tableName, itemPK, itemSK string) (raw []byte, sealed bool, err error) {
	var sealedInt int
	err = tx.QueryRow(
		`SELECT item_json, sealed FROM dynamodb_items
		 WHERE account_id = ? AND table_name = ? AND item_pk = ? AND item_sk = ?`,
		accountID, tableName, itemPK, itemSK,
	).Scan(&raw, &sealedInt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("load transact item raw: %w", err)
	}
	return raw, sealedInt == 1, nil
}

func (s *Store) evalTransactConditionOnRow(tx *sql.Tx, accountID string, a TransactWriteAction) error {
	fns, err := getTransactExprFns()
	if err != nil {
		return err
	}
	item, sealed, err := loadTransactItemMap(tx, accountID, a.TableName, a.ItemPK, a.ItemSK)
	if err != nil {
		return err
	}
	if sealed {
		return ErrDynamoTransactSealed
	}
	if err := fns.evalCondition(item, a.ConditionExpression, a.ExpressionAttributeNames, a.ExpressionAttributeValues); err != nil {
		if isTransactConditionalErr(err) {
			return errTransactConditionalCheckFailed
		}
		return fmt.Errorf("ValidationException: %v", err)
	}
	return nil
}

func loadTransactItemMap(tx *sql.Tx, accountID, tableName, itemPK, itemSK string) (DynamoAVMap, bool, error) {
	var (
		raw    []byte
		sealed int
	)
	err := tx.QueryRow(
		`SELECT item_json, sealed FROM dynamodb_items
		 WHERE account_id = ? AND table_name = ? AND item_pk = ? AND item_sk = ?`,
		accountID, tableName, itemPK, itemSK,
	).Scan(&raw, &sealed)
	if errors.Is(err, sql.ErrNoRows) {
		return DynamoAVMap{}, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("load transact item: %w", err)
	}
	if sealed == 1 {
		return nil, true, nil
	}
	item, err := unmarshalDynamoAVMap(raw)
	if err != nil {
		return nil, false, err
	}
	return item, false, nil
}

func unmarshalDynamoAVMap(data []byte) (DynamoAVMap, error) {
	if len(data) == 0 {
		return DynamoAVMap{}, nil
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("decode item json: %w", err)
	}
	out := make(DynamoAVMap, len(raw))
	for name, av := range raw {
		m, ok := av.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("attribute %q must be an AttributeValue object", name)
		}
		out[name] = m
	}
	return out, nil
}

func transactGSIKeys(table DynamoTable, item DynamoAVMap, slot int) (gsiPK, gsiSK string, err error) {
	var hashName, rangeName string
	var hasRange bool
	switch slot {
	case 1:
		if !table.HasGSI() {
			return "", "", nil
		}
		hashName, rangeName, hasRange = table.GSIHashKeyName, table.GSIRangeKeyName, table.GSIHasRangeKey()
	case 2:
		if !table.HasGSI2() {
			return "", "", nil
		}
		hashName, rangeName, hasRange = table.GSI2HashKeyName, table.GSI2RangeKeyName, table.GSI2HasRangeKey()
	default:
		return "", "", fmt.Errorf("invalid GSI slot %d", slot)
	}
	hashAV, ok := item[hashName]
	if !ok {
		return "", "", nil
	}
	gsiPK, err = transactCanonicalAV(hashAV)
	if err != nil {
		return "", "", err
	}
	if !hasRange {
		return gsiPK, "", nil
	}
	rangeAV, ok := item[rangeName]
	if !ok {
		return gsiPK, "", nil
	}
	gsiSK, err = transactCanonicalAV(rangeAV)
	if err != nil {
		return "", "", err
	}
	return gsiPK, gsiSK, nil
}

func transactCanonicalAV(av map[string]any) (string, error) {
	if len(av) == 0 {
		return "", fmt.Errorf("empty AttributeValue")
	}
	b, err := json.Marshal(av)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

var errTransactConditionalCheckFailed = errors.New("ConditionalCheckFailed")

func isTransactConditionalErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errTransactConditionalCheckFailed) {
		return true
	}
	// services/dynamodb.ErrConditionalCheckFailed text / sentinel via Error() match.
	msg := err.Error()
	return strings.Contains(msg, "ConditionalCheckFailedException") ||
		msg == "ConditionalCheckFailedException"
}

func transactCancelFromErr(err error) (code, message string) {
	if errors.Is(err, errTransactConditionalCheckFailed) {
		return "ConditionalCheckFailed", "The conditional request failed"
	}
	if errors.Is(err, ErrNoSuchTable) {
		return "ResourceNotFound", "Requested resource not found"
	}
	if errors.Is(err, ErrDynamoTransactUnsupported) || errors.Is(err, ErrDynamoTransactSealed) {
		return "ValidationError", err.Error()
	}
	if strings.HasPrefix(err.Error(), "ValidationException:") {
		return "ValidationError", err.Error()
	}
	return "ValidationError", err.Error()
}
