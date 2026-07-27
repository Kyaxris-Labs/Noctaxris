package server

import (
	"fmt"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func dynamoAttrTypes(params map[string]any) map[string]string {
	attrTypes := map[string]string{}
	if defs, ok := params["AttributeDefinitions"].([]any); ok {
		for _, raw := range defs {
			m, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			name, _ := m["AttributeName"].(string)
			typ, _ := m["AttributeType"].(string)
			if name != "" && typ != "" {
				attrTypes[name] = typ
			}
		}
	}
	return attrTypes
}

func dynamoParseGSISpec(raw map[string]any, attrTypes map[string]string) (store.DynamoGSI, error) {
	indexName, _ := raw["IndexName"].(string)
	if strings.TrimSpace(indexName) == "" {
		return store.DynamoGSI{}, fmt.Errorf("GSI IndexName is required")
	}
	if proj, ok := raw["Projection"].(map[string]any); ok {
		projType, _ := proj["ProjectionType"].(string)
		if projType != "" && !strings.EqualFold(projType, "ALL") {
			return store.DynamoGSI{}, fmt.Errorf("only ALL projection is supported")
		}
	}
	var hashKey, hashType, rangeKey, rangeType string
	schema, _ := raw["KeySchema"].([]any)
	for _, entry := range schema {
		m, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		name, _ := m["AttributeName"].(string)
		keyType, _ := m["KeyType"].(string)
		switch strings.ToUpper(keyType) {
		case "HASH":
			hashKey = name
			hashType = attrTypes[name]
		case "RANGE":
			rangeKey = name
			rangeType = attrTypes[name]
		}
	}
	if hashKey == "" || hashType == "" {
		return store.DynamoGSI{}, fmt.Errorf("GSI KeySchema must include a HASH key")
	}
	if rangeKey != "" && rangeType == "" {
		return store.DynamoGSI{}, fmt.Errorf("GSI RANGE key AttributeType is required")
	}
	return store.DynamoGSI{
		IndexName:    indexName,
		HashKeyName:  hashKey,
		HashKeyType:  hashType,
		RangeKeyName: rangeKey,
		RangeKeyType: rangeType,
	}, nil
}

func dynamoParseLSISpec(raw map[string]any, attrTypes map[string]string, tableHashKey string) (store.DynamoLSI, error) {
	indexName, _ := raw["IndexName"].(string)
	if strings.TrimSpace(indexName) == "" {
		return store.DynamoLSI{}, fmt.Errorf("LSI IndexName is required")
	}
	if proj, ok := raw["Projection"].(map[string]any); ok {
		projType, _ := proj["ProjectionType"].(string)
		if projType != "" && !strings.EqualFold(projType, "ALL") {
			return store.DynamoLSI{}, fmt.Errorf("only ALL projection is supported")
		}
	}
	var hashKey, rangeKey, rangeType string
	schema, _ := raw["KeySchema"].([]any)
	for _, entry := range schema {
		m, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		name, _ := m["AttributeName"].(string)
		keyType, _ := m["KeyType"].(string)
		switch strings.ToUpper(keyType) {
		case "HASH":
			hashKey = name
		case "RANGE":
			rangeKey = name
			rangeType = attrTypes[name]
		}
	}
	if hashKey == "" || hashKey != tableHashKey {
		return store.DynamoLSI{}, fmt.Errorf("LSI HASH key must match the table HASH key")
	}
	if rangeKey == "" || rangeType == "" {
		return store.DynamoLSI{}, fmt.Errorf("LSI KeySchema must include a RANGE key with AttributeDefinitions")
	}
	return store.DynamoLSI{
		IndexName:    indexName,
		RangeKeyName: rangeKey,
		RangeKeyType: rangeType,
	}, nil
}
