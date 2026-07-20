package store

import (
	"fmt"
	"strings"
)

// CreateTableWithGSIs creates a table with up to two lab GSIs.
func (s *Store) CreateTableWithGSIs(
	accountID, region, name, hashKey, hashType, rangeKey, rangeType, sseType, kmsKeyID string,
	gsis []DynamoGSI,
) (DynamoTable, error) {
	if len(gsis) > maxLabGSIs {
		return DynamoTable{}, ErrTooManyGSIs
	}
	for i := range gsis {
		if err := validateGSI(&gsis[i]); err != nil {
			return DynamoTable{}, err
		}
	}
	names := map[string]struct{}{}
	for _, g := range gsis {
		if _, ok := names[g.IndexName]; ok {
			return DynamoTable{}, fmt.Errorf("duplicate GSI IndexName %q", g.IndexName)
		}
		names[g.IndexName] = struct{}{}
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
	arn := TableARN(accountID, region, name)
	created := nowRFC3339()
	_, err := s.db.Exec(
		`INSERT INTO dynamodb_tables
		 (account_id, table_name, table_arn, status, hash_key_name, hash_key_type,
		  range_key_name, range_key_type, resource_policy, sse_type, kms_key_id, creation_date,
		  gsi_name, gsi_hash_key_name, gsi_hash_key_type, gsi_range_key_name, gsi_range_key_type,
		  gsi2_name, gsi2_hash_key_name, gsi2_hash_key_type, gsi2_range_key_name, gsi2_range_key_type,
		  ttl_attribute_name, ttl_enabled)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', 0)`,
		accountID, name, arn, TableStatusActive, hashKey, hashType,
		rangeKey, rangeType, sseType, kmsKeyID, created,
		n1, h1, ht1, r1, rt1,
		n2, h2, ht2, r2, rt2,
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
	}, nil
}

// gsiCount returns how many GSIs are configured on the table.
func (t DynamoTable) gsiCount() int {
	n := 0
	if t.GSIName != "" {
		n++
	}
	if t.GSI2Name != "" {
		n++
	}
	return n
}
