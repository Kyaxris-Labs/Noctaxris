package store

// CreateTableWithGSIs creates a table with up to two lab GSIs.
func (s *Store) CreateTableWithGSIs(
	accountID, region, name, hashKey, hashType, rangeKey, rangeType, sseType, kmsKeyID string,
	gsis []DynamoGSI,
) (DynamoTable, error) {
	return s.CreateTableWithIndexes(accountID, region, name, hashKey, hashType, rangeKey, rangeType, sseType, kmsKeyID, gsis, nil)
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
