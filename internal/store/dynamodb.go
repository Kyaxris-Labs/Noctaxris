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
	ErrGSIAlreadyExists     = errors.New("ResourceInUseException: GSI already exists")
	ErrNoSuchIndex          = errors.New("ResourceNotFoundException: index not found")
	ErrTooManyGSIs          = errors.New("ValidationException: lab supports at most two GSIs per table")
)

const maxLabGSIs = 2

const (
	// DefaultDynamoRegion is the lab region embedded in DynamoDB ARNs.
	DefaultDynamoRegion = "us-east-1"

	TableStatusActive = "ACTIVE"

	SSETypeAWSOwned = "AWS_OWNED"
	SSETypeKMS      = "KMS"

	KeyTypeString = "S"
	KeyTypeNumber = "N"
	KeyTypeBinary = "B"

	TTLStatusEnabled  = "ENABLED"
	TTLStatusDisabled = "DISABLED"
)

// DynamoGSI is lab metadata for a single global secondary index.
type DynamoGSI struct {
	IndexName    string
	HashKeyName  string
	HashKeyType  string
	RangeKeyName string
	RangeKeyType string
}

// HasRangeKey reports whether the GSI uses a composite key.
func (g DynamoGSI) HasRangeKey() bool {
	return g.RangeKeyName != ""
}

// DynamoTable is a DynamoDB table metadata row.
type DynamoTable struct {
	AccountID        string
	TableName        string
	TableARN         string
	Status           string
	HashKeyName      string
	HashKeyType      string
	RangeKeyName     string
	RangeKeyType     string
	ResourcePolicy   string
	SSEType          string
	KMSKeyID         string
	CreationDate     string
	GSIName          string
	GSIHashKeyName   string
	GSIHashKeyType   string
	GSIRangeKeyName  string
	GSIRangeKeyType  string
	GSI2Name         string
	GSI2HashKeyName  string
	GSI2HashKeyType  string
	GSI2RangeKeyName string
	GSI2RangeKeyType string
	TTLAttributeName string
	TTLEnabled       bool
	StreamEnabled    bool
	StreamViewType   string
	StreamLabel      string
}

// HasRangeKey reports whether the table uses a composite primary key.
func (t DynamoTable) HasRangeKey() bool {
	return t.RangeKeyName != ""
}

// HasGSI reports whether the table has at least one lab global secondary index.
func (t DynamoTable) HasGSI() bool {
	return t.GSIName != ""
}

// HasGSI2 reports whether the table has a second lab GSI.
func (t DynamoTable) HasGSI2() bool {
	return t.GSI2Name != ""
}

// GSIs returns configured lab GSIs in order (up to two).
func (t DynamoTable) GSIs() []DynamoGSI {
	var out []DynamoGSI
	if t.GSIName != "" {
		out = append(out, DynamoGSI{
			IndexName:    t.GSIName,
			HashKeyName:  t.GSIHashKeyName,
			HashKeyType:  t.GSIHashKeyType,
			RangeKeyName: t.GSIRangeKeyName,
			RangeKeyType: t.GSIRangeKeyType,
		})
	}
	if t.GSI2Name != "" {
		out = append(out, DynamoGSI{
			IndexName:    t.GSI2Name,
			HashKeyName:  t.GSI2HashKeyName,
			HashKeyType:  t.GSI2HashKeyType,
			RangeKeyName: t.GSI2RangeKeyName,
			RangeKeyType: t.GSI2RangeKeyType,
		})
	}
	return out
}

// GSIByName returns the GSI definition and slot (1 or 2).
func (t DynamoTable) GSIByName(name string) (DynamoGSI, int, bool) {
	if name == "" {
		return DynamoGSI{}, 0, false
	}
	if t.GSIName == name {
		return DynamoGSI{
			IndexName: t.GSIName, HashKeyName: t.GSIHashKeyName, HashKeyType: t.GSIHashKeyType,
			RangeKeyName: t.GSIRangeKeyName, RangeKeyType: t.GSIRangeKeyType,
		}, 1, true
	}
	if t.GSI2Name == name {
		return DynamoGSI{
			IndexName: t.GSI2Name, HashKeyName: t.GSI2HashKeyName, HashKeyType: t.GSI2HashKeyType,
			RangeKeyName: t.GSI2RangeKeyName, RangeKeyType: t.GSI2RangeKeyType,
		}, 2, true
	}
	return DynamoGSI{}, 0, false
}

// GSIHasRangeKey reports whether the first table GSI uses a composite key.
func (t DynamoTable) GSIHasRangeKey() bool {
	return t.GSIRangeKeyName != ""
}

// GSI2HasRangeKey reports whether the second table GSI uses a composite key.
func (t DynamoTable) GSI2HasRangeKey() bool {
	return t.GSI2RangeKeyName != ""
}

// DynamoStoredItem is a stored item row. ItemJSON holds plaintext AttributeValue
// map JSON when Sealed is false, or ciphertext bytes when Sealed is true.
type DynamoStoredItem struct {
	ItemPK    string
	ItemSK    string
	GSIPK     string
	GSISK     string
	GSI2PK    string
	GSI2SK    string
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

// ParseTableARN extracts account id and table name from a DynamoDB table ARN.
func ParseTableARN(arn string) (accountID, tableName string, ok bool) {
	arn = strings.TrimSpace(arn)
	const prefix = "arn:aws:dynamodb:"
	if !strings.HasPrefix(arn, prefix) {
		return "", "", false
	}
	rest := strings.TrimPrefix(arn, prefix)
	parts := strings.SplitN(rest, ":", 3)
	if len(parts) < 3 || parts[1] == "" {
		return "", "", false
	}
	resource := parts[2]
	if !strings.HasPrefix(resource, "table/") {
		return "", "", false
	}
	name := strings.TrimPrefix(resource, "table/")
	if name == "" || strings.Contains(name, "/") {
		return "", "", false
	}
	return parts[1], name, true
}

func validKeyType(t string) bool {
	switch t {
	case KeyTypeString, KeyTypeNumber, KeyTypeBinary:
		return true
	default:
		return false
	}
}

const dynamoTableSelect = `account_id, table_name, table_arn, status, hash_key_name, hash_key_type,
		        range_key_name, range_key_type, resource_policy, sse_type, kms_key_id, creation_date,
		        gsi_name, gsi_hash_key_name, gsi_hash_key_type, gsi_range_key_name, gsi_range_key_type,
		        COALESCE(gsi2_name, ''), COALESCE(gsi2_hash_key_name, ''), COALESCE(gsi2_hash_key_type, ''),
		        COALESCE(gsi2_range_key_name, ''), COALESCE(gsi2_range_key_type, ''),
		        ttl_attribute_name, ttl_enabled,
		        COALESCE(stream_enabled, 0), COALESCE(stream_view_type, ''), COALESCE(stream_label, '')`

func scanDynamoTable(scanner interface {
	Scan(dest ...any) error
}) (DynamoTable, error) {
	var t DynamoTable
	var ttlEnabled int
	var streamEnabled int
	err := scanner.Scan(
		&t.AccountID, &t.TableName, &t.TableARN, &t.Status,
		&t.HashKeyName, &t.HashKeyType, &t.RangeKeyName, &t.RangeKeyType,
		&t.ResourcePolicy, &t.SSEType, &t.KMSKeyID, &t.CreationDate,
		&t.GSIName, &t.GSIHashKeyName, &t.GSIHashKeyType, &t.GSIRangeKeyName, &t.GSIRangeKeyType,
		&t.GSI2Name, &t.GSI2HashKeyName, &t.GSI2HashKeyType, &t.GSI2RangeKeyName, &t.GSI2RangeKeyType,
		&t.TTLAttributeName, &ttlEnabled,
		&streamEnabled, &t.StreamViewType, &t.StreamLabel,
	)
	if err != nil {
		return DynamoTable{}, err
	}
	t.TTLEnabled = ttlEnabled == 1
	t.StreamEnabled = streamEnabled == 1
	return t, nil
}

// StreamARN returns the DynamoDB Streams ARN when the table stream is enabled.
func (t DynamoTable) StreamARN(region string) string {
	if !t.StreamEnabled || t.StreamLabel == "" {
		return ""
	}
	return DynamoStreamARN(region, t.AccountID, t.TableName, t.StreamLabel)
}

func validateGSI(gsi *DynamoGSI) error {
	if gsi == nil {
		return nil
	}
	if strings.TrimSpace(gsi.IndexName) == "" {
		return fmt.Errorf("GSI IndexName is required")
	}
	if strings.TrimSpace(gsi.HashKeyName) == "" || !validKeyType(gsi.HashKeyType) {
		return ErrInvalidKeyType
	}
	if gsi.RangeKeyName != "" && !validKeyType(gsi.RangeKeyType) {
		return ErrInvalidKeyType
	}
	return nil
}

func gsiColumns(gsi *DynamoGSI) (name, hashName, hashType, rangeName, rangeType string) {
	if gsi == nil {
		return "", "", "", "", ""
	}
	return gsi.IndexName, gsi.HashKeyName, gsi.HashKeyType, gsi.RangeKeyName, gsi.RangeKeyType
}

// CreateTable inserts a new table. rangeKey and rangeType are empty for
// hash-only tables. sseType defaults to AWS_OWNED when empty. gsi is optional
// (legacy single-GSI helper). Prefer CreateTableWithGSIs for multiple GSIs.
func (s *Store) CreateTable(
	accountID, region, name, hashKey, hashType, rangeKey, rangeType, sseType, kmsKeyID string,
	gsi *DynamoGSI,
) (DynamoTable, error) {
	var gsis []DynamoGSI
	if gsi != nil {
		gsis = []DynamoGSI{*gsi}
	}
	return s.CreateTableWithGSIs(accountID, region, name, hashKey, hashType, rangeKey, rangeType, sseType, kmsKeyID, gsis)
}

// GetTable returns table metadata or ErrNoSuchTable.
func (s *Store) GetTable(accountID, name string) (DynamoTable, error) {
	row := s.db.QueryRow(
		`SELECT `+dynamoTableSelect+` FROM dynamodb_tables WHERE account_id = ? AND table_name = ?`,
		accountID, name,
	)
	t, err := scanDynamoTable(row)
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
		`SELECT `+dynamoTableSelect+` FROM dynamodb_tables WHERE account_id = ? ORDER BY table_name`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list tables: %w", err)
	}
	defer rows.Close()
	out := []DynamoTable{}
	for rows.Next() {
		t, err := scanDynamoTable(rows)
		if err != nil {
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

// UpdateTableGSI adds a lab GSI to the next free slot (up to two GSIs).
func (s *Store) UpdateTableGSI(accountID, name string, gsi DynamoGSI) error {
	if err := validateGSI(&gsi); err != nil {
		return err
	}
	table, err := s.GetTable(accountID, name)
	if err != nil {
		return err
	}
	if table.GSIName == gsi.IndexName || table.GSI2Name == gsi.IndexName {
		return ErrGSIAlreadyExists
	}
	switch table.gsiCount() {
	case 0:
		res, err := s.db.Exec(
			`UPDATE dynamodb_tables SET
			   gsi_name = ?, gsi_hash_key_name = ?, gsi_hash_key_type = ?,
			   gsi_range_key_name = ?, gsi_range_key_type = ?
			 WHERE account_id = ? AND table_name = ?`,
			gsi.IndexName, gsi.HashKeyName, gsi.HashKeyType,
			gsi.RangeKeyName, gsi.RangeKeyType,
			accountID, name,
		)
		if err != nil {
			return fmt.Errorf("update table gsi: %w", err)
		}
		affected, _ := res.RowsAffected()
		if affected == 0 {
			return ErrNoSuchTable
		}
		return nil
	case 1:
		res, err := s.db.Exec(
			`UPDATE dynamodb_tables SET
			   gsi2_name = ?, gsi2_hash_key_name = ?, gsi2_hash_key_type = ?,
			   gsi2_range_key_name = ?, gsi2_range_key_type = ?
			 WHERE account_id = ? AND table_name = ?`,
			gsi.IndexName, gsi.HashKeyName, gsi.HashKeyType,
			gsi.RangeKeyName, gsi.RangeKeyType,
			accountID, name,
		)
		if err != nil {
			return fmt.Errorf("update table gsi2: %w", err)
		}
		affected, _ := res.RowsAffected()
		if affected == 0 {
			return ErrNoSuchTable
		}
		return nil
	default:
		return ErrTooManyGSIs
	}
}

// UpdateTimeToLive sets the TTL attribute name and enabled flag on a table.
func (s *Store) UpdateTimeToLive(accountID, name, attributeName string, enabled bool) error {
	if strings.TrimSpace(attributeName) == "" {
		return fmt.Errorf("update ttl: attribute name is required")
	}
	enabledFlag := 0
	if enabled {
		enabledFlag = 1
	}
	res, err := s.db.Exec(
		`UPDATE dynamodb_tables SET ttl_attribute_name = ?, ttl_enabled = ?
		 WHERE account_id = ? AND table_name = ?`,
		attributeName, enabledFlag, accountID, name,
	)
	if err != nil {
		return fmt.Errorf("update ttl: %w", err)
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
// KMS tables (with the sealed flag set and optional sealedDEK). gsiPK/gsiSK and
// gsi2PK/gsi2SK index the item for lab GSI queries when non-empty.
func (s *Store) PutItemBytes(
	accountID, table, itemPK, itemSK, gsiPK, gsiSK string,
	itemJSON []byte, sealed bool, sealedDEK []byte,
) error {
	return s.PutItemBytesMultiGSI(accountID, table, itemPK, itemSK, gsiPK, gsiSK, "", "", itemJSON, sealed, sealedDEK)
}

// PutItemBytesMultiGSI upserts an item with up to two GSI key pairs.
func (s *Store) PutItemBytesMultiGSI(
	accountID, table, itemPK, itemSK, gsiPK, gsiSK, gsi2PK, gsi2SK string,
	itemJSON []byte, sealed bool, sealedDEK []byte,
) error {
	if _, err := s.GetTable(accountID, table); err != nil {
		return err
	}
	sealedFlag := 0
	if sealed {
		sealedFlag = 1
	}
	_, err := s.db.Exec(
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
		accountID, table, itemPK, itemSK, gsiPK, gsiSK, gsi2PK, gsi2SK, itemJSON, sealedFlag, sealedDEK,
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
		`SELECT item_pk, item_sk, gsi_pk, gsi_sk, COALESCE(gsi2_pk, ''), COALESCE(gsi2_sk, ''), item_json, sealed, sealed_dek
		 FROM dynamodb_items
		 WHERE account_id = ? AND table_name = ? AND item_pk = ? AND item_sk = ?`,
		accountID, table, itemPK, itemSK,
	).Scan(&it.ItemPK, &it.ItemSK, &it.GSIPK, &it.GSISK, &it.GSI2PK, &it.GSI2SK, &it.ItemJSON, &sealed, &sealedDEK)
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

// GetItemGSIKeys returns stored GSI key columns for an item.
func (s *Store) GetItemGSIKeys(accountID, table, itemPK, itemSK string) (gsiPK, gsiSK string, err error) {
	err = s.db.QueryRow(
		`SELECT gsi_pk, gsi_sk FROM dynamodb_items
		 WHERE account_id = ? AND table_name = ? AND item_pk = ? AND item_sk = ?`,
		accountID, table, itemPK, itemSK,
	).Scan(&gsiPK, &gsiSK)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", ErrNoSuchItem
	}
	if err != nil {
		return "", "", fmt.Errorf("get item gsi keys: %w", err)
	}
	return gsiPK, gsiSK, nil
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
	query := `SELECT item_pk, item_sk, gsi_pk, gsi_sk, COALESCE(gsi2_pk, ''), COALESCE(gsi2_sk, ''), item_json, sealed, sealed_dek
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

// QueryGSIItems returns items whose first-slot gsi_pk equals hashPK, ordered by gsi_sk.
func (s *Store) QueryGSIItems(accountID, table, gsiPK string, limit int, startGsiSK string) (ItemPage, error) {
	return s.QueryGSISlotItems(accountID, table, 1, gsiPK, limit, startGsiSK)
}

// QueryGSISlotItems queries GSI slot 1 or 2 by hash key value.
func (s *Store) QueryGSISlotItems(accountID, table string, slot int, gsiPK string, limit int, startGsiSK string) (ItemPage, error) {
	pkCol, skCol := "gsi_pk", "gsi_sk"
	if slot == 2 {
		pkCol, skCol = "gsi2_pk", "gsi2_sk"
	}
	query := fmt.Sprintf(`SELECT item_pk, item_sk, gsi_pk, gsi_sk, COALESCE(gsi2_pk, ''), COALESCE(gsi2_sk, ''), item_json, sealed, sealed_dek
	          FROM dynamodb_items
	          WHERE account_id = ? AND table_name = ? AND %s = ?`, pkCol)
	args := []any{accountID, table, gsiPK}
	if startGsiSK != "" {
		query += fmt.Sprintf(` AND %s > ?`, skCol)
		args = append(args, startGsiSK)
	}
	query += fmt.Sprintf(` ORDER BY %s`, skCol)
	fetch := limit
	if limit > 0 {
		fetch = limit + 1
		query += ` LIMIT ?`
		args = append(args, fetch)
	}
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return ItemPage{}, fmt.Errorf("query gsi items: %w", err)
	}
	defer rows.Close()
	return scanItemPage(rows, limit)
}

// ScanItems returns items across a table ordered by (item_pk, item_sk). startPK
// and startSK form an exclusive continuation key (both empty for the first page).
func (s *Store) ScanItems(accountID, table string, limit int, startPK, startSK string) (ItemPage, error) {
	query := `SELECT item_pk, item_sk, gsi_pk, gsi_sk, COALESCE(gsi2_pk, ''), COALESCE(gsi2_sk, ''), item_json, sealed, sealed_dek
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
		if err := rows.Scan(&it.ItemPK, &it.ItemSK, &it.GSIPK, &it.GSISK, &it.GSI2PK, &it.GSI2SK, &it.ItemJSON, &sealed, &sealedDEK); err != nil {
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
