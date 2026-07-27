package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

const maxLabLSIs = 2

var (
	ErrTooManyLSIs     = errors.New("ValidationException: lab supports at most two LSIs per table")
	ErrLSIRequiresRange = errors.New("ValidationException: LocalSecondaryIndexes require a table RANGE key")
)

// DynamoLSI is lab metadata for a local secondary index (same hash as table, alternate range).
type DynamoLSI struct {
	IndexName    string
	RangeKeyName string
	RangeKeyType string
}

// HasLSI reports whether the table has at least one lab LSI.
func (t DynamoTable) HasLSI() bool {
	return t.LSIName != ""
}

// HasLSI2 reports whether the table has a second lab LSI.
func (t DynamoTable) HasLSI2() bool {
	return t.LSI2Name != ""
}

// LSIs returns configured lab LSIs in order (up to two).
func (t DynamoTable) LSIs() []DynamoLSI {
	var out []DynamoLSI
	if t.LSIName != "" {
		out = append(out, DynamoLSI{
			IndexName: t.LSIName, RangeKeyName: t.LSIRangeKeyName, RangeKeyType: t.LSIRangeKeyType,
		})
	}
	if t.LSI2Name != "" {
		out = append(out, DynamoLSI{
			IndexName: t.LSI2Name, RangeKeyName: t.LSI2RangeKeyName, RangeKeyType: t.LSI2RangeKeyType,
		})
	}
	return out
}

// LSIByName returns the LSI definition and slot (1 or 2).
func (t DynamoTable) LSIByName(name string) (DynamoLSI, int, bool) {
	if name == "" {
		return DynamoLSI{}, 0, false
	}
	if t.LSIName == name {
		return DynamoLSI{
			IndexName: t.LSIName, RangeKeyName: t.LSIRangeKeyName, RangeKeyType: t.LSIRangeKeyType,
		}, 1, true
	}
	if t.LSI2Name == name {
		return DynamoLSI{
			IndexName: t.LSI2Name, RangeKeyName: t.LSI2RangeKeyName, RangeKeyType: t.LSI2RangeKeyType,
		}, 2, true
	}
	return DynamoLSI{}, 0, false
}

// IndexByName resolves a GSI or LSI by IndexName. kind is "GSI" or "LSI".
func (t DynamoTable) IndexByName(name string) (kind string, gsi DynamoGSI, lsi DynamoLSI, slot int, ok bool) {
	if g, s, found := t.GSIByName(name); found {
		return "GSI", g, DynamoLSI{}, s, true
	}
	if l, s, found := t.LSIByName(name); found {
		return "LSI", DynamoGSI{}, l, s, true
	}
	return "", DynamoGSI{}, DynamoLSI{}, 0, false
}

func validateLSI(lsi *DynamoLSI) error {
	if lsi == nil {
		return nil
	}
	if strings.TrimSpace(lsi.IndexName) == "" {
		return fmt.Errorf("LSI IndexName is required")
	}
	if strings.TrimSpace(lsi.RangeKeyName) == "" || !validKeyType(lsi.RangeKeyType) {
		return fmt.Errorf("LSI RANGE key AttributeName and AttributeType are required")
	}
	return nil
}

func lsiColumns(lsi *DynamoLSI) (name, rangeName, rangeType string) {
	if lsi == nil {
		return "", "", ""
	}
	return lsi.IndexName, lsi.RangeKeyName, lsi.RangeKeyType
}

// CreateTableWithIndexes creates a table with up to two lab GSIs and two lab LSIs.
func (s *Store) CreateTableWithIndexes(
	accountID, region, name, hashKey, hashType, rangeKey, rangeType, sseType, kmsKeyID string,
	gsis []DynamoGSI,
	lsis []DynamoLSI,
) (DynamoTable, error) {
	if len(gsis) > maxLabGSIs {
		return DynamoTable{}, ErrTooManyGSIs
	}
	if len(lsis) > maxLabLSIs {
		return DynamoTable{}, ErrTooManyLSIs
	}
	if len(lsis) > 0 && strings.TrimSpace(rangeKey) == "" {
		return DynamoTable{}, ErrLSIRequiresRange
	}
	for i := range gsis {
		if err := validateGSI(&gsis[i]); err != nil {
			return DynamoTable{}, err
		}
	}
	for i := range lsis {
		if err := validateLSI(&lsis[i]); err != nil {
			return DynamoTable{}, err
		}
	}
	names := map[string]struct{}{}
	for _, g := range gsis {
		if _, ok := names[g.IndexName]; ok {
			return DynamoTable{}, fmt.Errorf("duplicate IndexName %q", g.IndexName)
		}
		names[g.IndexName] = struct{}{}
	}
	for _, l := range lsis {
		if _, ok := names[l.IndexName]; ok {
			return DynamoTable{}, fmt.Errorf("duplicate IndexName %q", l.IndexName)
		}
		names[l.IndexName] = struct{}{}
		if l.RangeKeyName == rangeKey {
			return DynamoTable{}, fmt.Errorf("LSI range key must differ from table RANGE key")
		}
	}

	var gsi1, gsi2 *DynamoGSI
	if len(gsis) > 0 {
		g := gsis[0]
		gsi1 = &g
	}
	if len(gsis) > 1 {
		g := gsis[1]
		gsi2 = &g
	}
	var lsi1, lsi2 *DynamoLSI
	if len(lsis) > 0 {
		l := lsis[0]
		lsi1 = &l
	}
	if len(lsis) > 1 {
		l := lsis[1]
		lsi2 = &l
	}

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

	n1, h1, ht1, r1, rt1 := gsiColumns(gsi1)
	n2, h2, ht2, r2, rt2 := gsiColumns(gsi2)
	ln1, lr1, lrt1 := lsiColumns(lsi1)
	ln2, lr2, lrt2 := lsiColumns(lsi2)
	arn := TableARN(accountID, region, name)
	created := nowRFC3339()
	_, err := s.db.Exec(
		`INSERT INTO dynamodb_tables
		 (account_id, table_name, table_arn, status, hash_key_name, hash_key_type,
		  range_key_name, range_key_type, resource_policy, sse_type, kms_key_id, creation_date,
		  gsi_name, gsi_hash_key_name, gsi_hash_key_type, gsi_range_key_name, gsi_range_key_type,
		  gsi2_name, gsi2_hash_key_name, gsi2_hash_key_type, gsi2_range_key_name, gsi2_range_key_type,
		  lsi_name, lsi_range_key_name, lsi_range_key_type,
		  lsi2_name, lsi2_range_key_name, lsi2_range_key_type,
		  ttl_attribute_name, ttl_enabled)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', 0)`,
		accountID, name, arn, TableStatusActive, hashKey, hashType,
		rangeKey, rangeType, sseType, kmsKeyID, created,
		n1, h1, ht1, r1, rt1,
		n2, h2, ht2, r2, rt2,
		ln1, lr1, lrt1,
		ln2, lr2, lrt2,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return DynamoTable{}, ErrTableAlreadyExists
		}
		return DynamoTable{}, fmt.Errorf("create table: %w", err)
	}
	return DynamoTable{
		AccountID:        accountID,
		TableName:        name,
		TableARN:         arn,
		Status:           TableStatusActive,
		HashKeyName:      hashKey,
		HashKeyType:      hashType,
		RangeKeyName:     rangeKey,
		RangeKeyType:     rangeType,
		SSEType:          sseType,
		KMSKeyID:         kmsKeyID,
		CreationDate:     created,
		GSIName:          n1,
		GSIHashKeyName:   h1,
		GSIHashKeyType:   ht1,
		GSIRangeKeyName:  r1,
		GSIRangeKeyType:  rt1,
		GSI2Name:         n2,
		GSI2HashKeyName:  h2,
		GSI2HashKeyType:  ht2,
		GSI2RangeKeyName: r2,
		GSI2RangeKeyType: rt2,
		LSIName:          ln1,
		LSIRangeKeyName:  lr1,
		LSIRangeKeyType:  lrt1,
		LSI2Name:         ln2,
		LSI2RangeKeyName: lr2,
		LSI2RangeKeyType: lrt2,
	}, nil
}

// QueryLSISlotItems returns items for LSI slot 1 or 2 matching the table hash PK, ordered by LSI range.
func (s *Store) QueryLSISlotItems(accountID, table string, slot int, hashPK string, limit int, startLsiSK string) (ItemPage, error) {
	skCol := "lsi_sk"
	if slot == 2 {
		skCol = "lsi2_sk"
	}
	query := fmt.Sprintf(`SELECT item_pk, item_sk, gsi_pk, gsi_sk, COALESCE(gsi2_pk, ''), COALESCE(gsi2_sk, ''), item_json, sealed, sealed_dek
		 FROM dynamodb_items
		 WHERE account_id = ? AND table_name = ? AND item_pk = ?`)
	args := []any{accountID, table, hashPK}
	if startLsiSK != "" {
		query += fmt.Sprintf(` AND %s > ?`, skCol)
		args = append(args, startLsiSK)
	}
	query += fmt.Sprintf(` ORDER BY %s ASC, item_sk ASC`, skCol)
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit+1)
	}
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return ItemPage{}, fmt.Errorf("query lsi items: %w", err)
	}
	defer rows.Close()
	return scanItemPage(rows, limit)
}

// GetItemLSISlotKeys returns stored LSI range key for slot 1 or 2.
func (s *Store) GetItemLSISlotKeys(accountID, table, itemPK, itemSK string, slot int) (lsiSK string, err error) {
	skCol := "COALESCE(lsi_sk, '')"
	if slot == 2 {
		skCol = "COALESCE(lsi2_sk, '')"
	}
	query := fmt.Sprintf(`SELECT %s FROM dynamodb_items
		 WHERE account_id = ? AND table_name = ? AND item_pk = ? AND item_sk = ?`, skCol)
	err = s.db.QueryRow(query, accountID, table, itemPK, itemSK).Scan(&lsiSK)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNoSuchItem
	}
	if err != nil {
		return "", fmt.Errorf("get item lsi keys: %w", err)
	}
	return lsiSK, nil
}
