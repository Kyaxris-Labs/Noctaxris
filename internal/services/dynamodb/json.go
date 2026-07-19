package dynamodb

import (
	"encoding/json"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func tableDescription(t store.DynamoTable) map[string]any {
	attrs := []map[string]string{
		{"AttributeName": t.HashKeyName, "AttributeType": t.HashKeyType},
	}
	keySchema := []map[string]string{
		{"AttributeName": t.HashKeyName, "KeyType": "HASH"},
	}
	if t.HasRangeKey() {
		attrs = append(attrs, map[string]string{
			"AttributeName": t.RangeKeyName, "AttributeType": t.RangeKeyType,
		})
		keySchema = append(keySchema, map[string]string{
			"AttributeName": t.RangeKeyName, "KeyType": "RANGE",
		})
	}

	desc := map[string]any{
		"TableName":            t.TableName,
		"TableArn":             t.TableARN,
		"TableStatus":          t.Status,
		"CreationDateTime":     parseCreationFloat(t.CreationDate),
		"AttributeDefinitions": attrs,
		"KeySchema":            keySchema,
		"ItemCount":            0,
		"TableSizeBytes":       0,
		"BillingModeSummary": map[string]any{
			"BillingMode": "PAY_PER_REQUEST",
		},
	}

	sseType := "AES256"
	sse := map[string]any{
		"Status":  "ENABLED",
		"SSEType": sseType,
	}
	if t.SSEType == store.SSETypeKMS {
		sse["SSEType"] = "KMS"
		if t.KMSKeyID != "" {
			sse["KMSMasterKeyArn"] = t.KMSKeyID
		}
	}
	desc["SSEDescription"] = sse
	return desc
}

func parseCreationFloat(rfc3339 string) float64 {
	tm, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		return 0
	}
	return float64(tm.Unix())
}

// CreateTableJSON builds a CreateTable / UpdateTable success body.
func CreateTableJSON(t store.DynamoTable) ([]byte, error) {
	return json.Marshal(map[string]any{"TableDescription": tableDescription(t)})
}

// DescribeTableJSON builds a DescribeTable success body.
func DescribeTableJSON(t store.DynamoTable) ([]byte, error) {
	return json.Marshal(map[string]any{"Table": tableDescription(t)})
}

// ListTablesJSON builds a ListTables success body.
func ListTablesJSON(tables []store.DynamoTable) ([]byte, error) {
	names := make([]string, 0, len(tables))
	for _, t := range tables {
		names = append(names, t.TableName)
	}
	return json.Marshal(map[string]any{"TableNames": names})
}

// EmptyOKJSON returns an empty DynamoDB success object.
func EmptyOKJSON() ([]byte, error) {
	return []byte(`{}`), nil
}

// GetItemJSON builds a GetItem success body. Missing items omit "Item".
func GetItemJSON(item ItemMap, found bool) ([]byte, error) {
	out := map[string]any{}
	if found {
		out["Item"] = item
	}
	return json.Marshal(out)
}

// PutItemJSON builds a PutItem success body (optionally echoing Attributes).
func PutItemJSON(attrs ItemMap) ([]byte, error) {
	if attrs == nil {
		return EmptyOKJSON()
	}
	return json.Marshal(map[string]any{"Attributes": attrs})
}

// DeleteItemJSON builds a DeleteItem success body.
func DeleteItemJSON() ([]byte, error) {
	return EmptyOKJSON()
}

// UpdateItemJSON builds an UpdateItem success body with Attributes.
func UpdateItemJSON(attrs ItemMap) ([]byte, error) {
	return json.Marshal(map[string]any{"Attributes": attrs})
}

// QueryJSON builds a Query success body.
func QueryJSON(items []ItemMap, lastKey ItemMap, hasMore bool) ([]byte, error) {
	out := map[string]any{
		"Items":            items,
		"Count":            len(items),
		"ScannedCount":     len(items),
		"ConsumedCapacity": nil,
	}
	if hasMore && lastKey != nil {
		out["LastEvaluatedKey"] = lastKey
	}
	return json.Marshal(out)
}

// ScanJSON builds a Scan success body.
func ScanJSON(items []ItemMap, lastKey ItemMap, hasMore bool) ([]byte, error) {
	return QueryJSON(items, lastKey, hasMore)
}

// BatchGetItemJSON builds a BatchGetItem success body.
func BatchGetItemJSON(responses map[string][]ItemMap, unprocessed map[string]any) ([]byte, error) {
	out := map[string]any{
		"Responses":            responses,
		"UnprocessedKeys":      unprocessed,
		"ConsumedCapacity":     nil,
	}
	if unprocessed == nil {
		out["UnprocessedKeys"] = map[string]any{}
	}
	return json.Marshal(out)
}

// BatchWriteItemJSON builds a BatchWriteItem success body.
func BatchWriteItemJSON(unprocessed map[string]any) ([]byte, error) {
	if unprocessed == nil {
		unprocessed = map[string]any{}
	}
	return json.Marshal(map[string]any{"UnprocessedItems": unprocessed})
}

// GetResourcePolicyJSON builds a GetResourcePolicy success body.
func GetResourcePolicyJSON(policy string) ([]byte, error) {
	return json.Marshal(map[string]any{"Policy": policy})
}
