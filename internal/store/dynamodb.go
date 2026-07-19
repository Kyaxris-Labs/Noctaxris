package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrTableAlreadyExists   = errors.New("ResourceInUseException")
	ErrNoSuchTable          = errors.New("ResourceNotFoundException")
	ErrTableNotEmpty        = errors.New("ValidationException")
	ErrNoSuchItem           = errors.New("NoSuchItem")
	ErrNoSuchResourcePolicy = errors.New("PolicyNotFoundException")
	ErrInvalidKeyType       = errors.New("ValidationException: invalid key type")
)

const (
	// DefaultDynamoRegion is the lab region embedded in DynamoDB ARNs.
	DefaultDynamoRegion = "us-east-1"

	TableStatusActive = "ACTIVE"

	SSETypeAWSOwned = "AWS_OWNED"
	SSETypeKMS      = "KMS"

	KeyTypeString = "S"
	KeyTypeNumber = "N"
	KeyTypeBinary = "B"
)

// DynamoTable is a DynamoDB table metadata row.
type DynamoTable struct {
	AccountID      string
	TableName      string
	TableARN       string
	Status         string
	HashKeyName    string
	HashKeyType    string
	RangeKeyName   string
	RangeKeyType   string
	ResourcePolicy string
	SSEType        string
	KMSKeyID       string
	CreationDate   string
}

// HasRangeKey reports whether the table uses a composite primary key.
func (t DynamoTable) HasRangeKey() bool {
	return t.RangeKeyName != ""
}

// DynamoStoredItem is a stored item row. ItemJSON holds plaintext AttributeValue
// map JSON when Sealed is false, or ciphertext bytes when Sealed is true.
type DynamoStoredItem struct {
	ItemPK    string
	ItemSK    string
	ItemJSON  []byte
	Sealed    bool
	SealedDEK []byte
}

// ItemPage is a page of items with an optional exclusive continuation key.
type ItemPage struct {
	Items   []DynamoStoredItem
	LastPK  string
	LastSK  string
	HasMore bool
}

// TableARN builds arn:aws:dynamodb:REGION:ACCOUNT:table/NAME.
func TableARN(accountID, region, tableName string) string {
	if region == "" {
		region = DefaultDynamoRegion
	}
	return fmt.Sprintf("arn:aws:dynamodb:%s:%s:table/%s", region, accountID, tableName)
}

func validKeyType(t string) bool {
	switch t {
	case KeyTypeString, KeyTypeNumber, KeyTypeBinary:
		return true
	default:
		return false
	}
}

// CreateTable inserts a new table. rangeKey and rangeType are empty for
// hash-only tables. sseType defaults to AWS_OWNED when empty.
func (s *Store) CreateTable(accountID, region, name, hashKey, hashType, rangeKey, rangeType, sseType, kmsKeyID string) (DynamoTable, error) {
	if strings.TrimSpace(name) == "" {
		return DynamoTable{}, fmt.Errorf("create table: name is required")
	}
	if strings.TrimSpace(hashKey) == "" || !validKeyType(hashType) {
		return DynamoTable{}, ErrInvalidKeyType
	}
	if rangeKey != "" && !validKeyType(rangeType) {
		return DynamoTable{}, ErrInvalidKeyType
	}
	if sseType == "" {
		sseType = SSETypeAWSOwned
	}
	if sseType != SSETypeAWSOwned && sseType != SSETypeKMS {
		return DynamoTable{}, fmt.Errorf("create table: invalid sse type %q", sseType)
	}
	arn := TableARN(accountID, region, name)
	created := nowRFC3339()
	_, err := s.db.Exec(
		`INSERT INTO dynamodb_tables
		 (account_id, table_name, table_arn, status, hash_key_name, hash_key_type,
		  range_key_name, range_key_type, resource_policy, sse_type, kms_key_id, creation_date)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?, ?)`,
		accountID, name, arn, TableStatusActive, hashKey, hashType,
		rangeKey, rangeType, sseType, kmsKeyID, created,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return DynamoTable{}, ErrTableAlreadyExists
		}
		return DynamoTable{}, fmt.Errorf("create table: %w", err)
	}
	return DynamoTable{
		AccountID:    accountID,
		TableName:    name,
		TableARN:     arn,
		Status:       TableStatusActive,
		HashKeyName:  hashKey,
		HashKeyType:  hashType,
		RangeKeyName: rangeKey,
		RangeKeyType: rangeType,
		SSEType:      sseType,
		KMSKeyID:     kmsKeyID,
		CreationDate: created,
	}, nil
}

// GetTable returns table metadata or ErrNoSuchTable.
func (s *Store) GetTable(accountID, name string) (DynamoTable, error) {
	var t DynamoTable
	err := s.db.QueryRow(
		`SELECT account_id, table_name, table_arn, status, hash_key_name, hash_key_type,
		        range_key_name, range_key_type, resource_policy, sse_type, kms_key_id, creation_date
		 FROM dynamodb_tables WHERE account_id = ? AND table_name = ?`,
		accountID, name,
	).Scan(&t.AccountID, &t.TableName, &t.TableARN, &t.Status, &t.HashKeyName, &t.HashKeyType,
		&t.RangeKeyName, &t.RangeKeyType, &t.ResourcePolicy, &t.SSEType, &t.KMSKeyID, &t.CreationDate)
	if errors.Is(err, sql.ErrNoRows) {
		return DynamoTable{}, ErrNoSuchTable
	}
	if err != nil {
		return DynamoTable{}, fmt.Errorf("get table: %w", err)
	}
	return t, nil
}

// DescribeTable is an alias for GetTable.
func (s *Store) DescribeTable(accountID, name string) (DynamoTable, error) {
	return s.GetTable(accountID, name)
}

// ListTables returns tables for an account ordered by name.
func (s *Store) ListTables(accountID string) ([]DynamoTable, error) {
	rows, err := s.db.Query(
		`SELECT account_id, table_name, table_arn, status, hash_key_name, hash_key_type,
		        range_key_name, range_key_type, resource_policy, sse_type, kms_key_id, creation_date
		 FROM dynamodb_tables WHERE account_id = ? ORDER BY table_name`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list tables: %w", err)
	}
	defer rows.Close()
	out := []DynamoTable{}
	for rows.Next() {
		var t DynamoTable
		if err := rows.Scan(&t.AccountID, &t.TableName, &t.TableARN, &t.Status, &t.HashKeyName, &t.HashKeyType,
			&t.RangeKeyName, &t.RangeKeyType, &t.ResourcePolicy, &t.SSEType, &t.KMSKeyID, &t.CreationDate); err != nil {
			return nil, fmt.Errorf("list tables: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// DeleteTable removes an empty table. Returns ErrTableNotEmpty if items exist.
func (s *Store) DeleteTable(accountID, name string) error {
	if _, err := s.GetTable(accountID, name); err != nil {
		return err
	}
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM dynamodb_items WHERE account_id = ? AND table_name = ?`,
		accountID, name,
	).Scan(&n)
	if err != nil {
		return fmt.Errorf("delete table: count items: %w", err)
	}
	if n > 0 {
		return ErrTableNotEmpty
	}
	_, err = s.db.Exec(
		`DELETE FROM dynamodb_tables WHERE account_id = ? AND table_name = ?`,
		accountID, name,
	)
	if err != nil {
		return fmt.Errorf("delete table: %w", err)
	}
	return nil
}

// UpdateTableSSE changes the server-side encryption settings on a table.
func (s *Store) UpdateTableSSE(accountID, name, sseType, kmsKeyID string) error {
	if sseType != SSETypeAWSOwned && sseType != SSETypeKMS {
		return fmt.Errorf("update table sse: invalid sse type %q", sseType)
	}
	res, err := s.db.Exec(
		`UPDATE dynamodb_tables SET sse_type = ?, kms_key_id = ? WHERE account_id = ? AND table_name = ?`,
		sseType, kmsKeyID, accountID, name,
	)
	if err != nil {
		return fmt.Errorf("update table sse: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrNoSuchTable
	}
	return nil
}

// PutResourcePolicy replaces the table resource policy document.
func (s *Store) PutResourcePolicy(accountID, name, policy string) error {
	res, err := s.db.Exec(
		`UPDATE dynamodb_tables SET resource_policy = ? WHERE account_id = ? AND table_name = ?`,
		policy, accountID, name,
	)
	if err != nil {
		return fmt.Errorf("put resource policy: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrNoSuchTable
	}
	return nil
}

// GetResourcePolicy returns the stored policy or ErrNoSuchResourcePolicy when empty.
func (s *Store) GetResourcePolicy(accountID, name string) (string, error) {
	t, err := s.GetTable(accountID, name)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(t.ResourcePolicy) == "" {
		return "", ErrNoSuchResourcePolicy
	}
	return t.ResourcePolicy, nil
}

// DeleteResourcePolicy clears the table resource policy.
func (s *Store) DeleteResourcePolicy(accountID, name string) error {
	res, err := s.db.Exec(
		`UPDATE dynamodb_tables SET resource_policy = '' WHERE account_id = ? AND table_name = ?`,
		accountID, name,
	)
	if err != nil {
		return fmt.Errorf("delete resource policy: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrNoSuchTable
	}
	return nil
}

// PutItemBytes upserts an item under the canonical (itemPK, itemSK) key. itemJSON
// is plaintext AttributeValue map JSON for AWS_OWNED tables, or ciphertext for
// KMS tables (with the sealed flag set and optional sealedDEK).
func (s *Store) PutItemBytes(accountID, table, itemPK, itemSK string, itemJSON []byte, sealed bool, sealedDEK []byte) error {
	if _, err := s.GetTable(accountID, table); err != nil {
		return err
	}
	sealedFlag := 0
	if sealed {
		sealedFlag = 1
	}
	_, err := s.db.Exec(
		`INSERT INTO dynamodb_items
		 (account_id, table_name, item_pk, item_sk, item_json, sealed, sealed_dek)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(account_id, table_name, item_pk, item_sk) DO UPDATE SET
		   item_json = excluded.item_json,
		   sealed = excluded.sealed,
		   sealed_dek = excluded.sealed_dek`,
		accountID, table, itemPK, itemSK, itemJSON, sealedFlag, sealedDEK,
	)
	if err != nil {
		return fmt.Errorf("put item: %w", err)
	}
	return nil
}

// GetItemBytes returns the stored item or ErrNoSuchItem.
func (s *Store) GetItemBytes(accountID, table, itemPK, itemSK string) (DynamoStoredItem, error) {
	var (
		it        DynamoStoredItem
		sealed    int
		sealedDEK []byte
	)
	err := s.db.QueryRow(
		`SELECT item_pk, item_sk, item_json, sealed, sealed_dek
		 FROM dynamodb_items
		 WHERE account_id = ? AND table_name = ? AND item_pk = ? AND item_sk = ?`,
		accountID, table, itemPK, itemSK,
	).Scan(&it.ItemPK, &it.ItemSK, &it.ItemJSON, &sealed, &sealedDEK)
	if errors.Is(err, sql.ErrNoRows) {
		return DynamoStoredItem{}, ErrNoSuchItem
	}
	if err != nil {
		return DynamoStoredItem{}, fmt.Errorf("get item: %w", err)
	}
	it.Sealed = sealed == 1
	it.SealedDEK = sealedDEK
	return it, nil
}

// DeleteItem removes an item by canonical key. Missing items are a no-op.
func (s *Store) DeleteItem(accountID, table, itemPK, itemSK string) error {
	_, err := s.db.Exec(
		`DELETE FROM dynamodb_items
		 WHERE account_id = ? AND table_name = ? AND item_pk = ? AND item_sk = ?`,
		accountID, table, itemPK, itemSK,
	)
	if err != nil {
		return fmt.Errorf("delete item: %w", err)
	}
	return nil
}

// QueryItems returns items whose hash key equals hashPK, ordered by item_sk. When
// limit is greater than zero the page is capped and HasMore is set. startSK is an
// exclusive continuation key (empty for the first page).
func (s *Store) QueryItems(accountID, table, hashPK string, limit int, startSK string) (ItemPage, error) {
	query := `SELECT item_pk, item_sk, item_json, sealed, sealed_dek
	          FROM dynamodb_items
	          WHERE account_id = ? AND table_name = ? AND item_pk = ?`
	args := []any{accountID, table, hashPK}
	if startSK != "" {
		query += ` AND item_sk > ?`
		args = append(args, startSK)
	}
	query += ` ORDER BY item_sk`
	fetch := limit
	if limit > 0 {
		fetch = limit + 1
		query += ` LIMIT ?`
		args = append(args, fetch)
	}
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return ItemPage{}, fmt.Errorf("query items: %w", err)
	}
	defer rows.Close()
	return scanItemPage(rows, limit)
}

// ScanItems returns items across a table ordered by (item_pk, item_sk). startPK
// and startSK form an exclusive continuation key (both empty for the first page).
func (s *Store) ScanItems(accountID, table string, limit int, startPK, startSK string) (ItemPage, error) {
	query := `SELECT item_pk, item_sk, item_json, sealed, sealed_dek
	          FROM dynamodb_items
	          WHERE account_id = ? AND table_name = ?`
	args := []any{accountID, table}
	if startPK != "" || startSK != "" {
		query += ` AND (item_pk > ? OR (item_pk = ? AND item_sk > ?))`
		args = append(args, startPK, startPK, startSK)
	}
	query += ` ORDER BY item_pk, item_sk`
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit+1)
	}
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return ItemPage{}, fmt.Errorf("scan items: %w", err)
	}
	defer rows.Close()
	return scanItemPage(rows, limit)
}

func scanItemPage(rows *sql.Rows, limit int) (ItemPage, error) {
	var page ItemPage
	page.Items = []DynamoStoredItem{}
	for rows.Next() {
		var (
			it        DynamoStoredItem
			sealed    int
			sealedDEK []byte
		)
		if err := rows.Scan(&it.ItemPK, &it.ItemSK, &it.ItemJSON, &sealed, &sealedDEK); err != nil {
			return ItemPage{}, fmt.Errorf("scan item page: %w", err)
		}
		it.Sealed = sealed == 1
		it.SealedDEK = sealedDEK
		page.Items = append(page.Items, it)
	}
	if err := rows.Err(); err != nil {
		return ItemPage{}, fmt.Errorf("scan item page: %w", err)
	}
	if limit > 0 && len(page.Items) > limit {
		page.Items = page.Items[:limit]
		last := page.Items[limit-1]
		page.HasMore = true
		page.LastPK = last.ItemPK
		page.LastSK = last.ItemSK
	}
	return page, nil
}
