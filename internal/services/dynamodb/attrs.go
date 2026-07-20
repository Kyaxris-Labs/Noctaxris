package dynamodb

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// ItemMap is an AWS AttributeValue map (attr name -> typed value object).
type ItemMap map[string]map[string]any

// ParseItemMap parses an Item or Key object from a DynamoDB JSON request.
func ParseItemMap(v any) (ItemMap, error) {
	if v == nil {
		return nil, fmt.Errorf("item is required")
	}
	raw, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("item must be an object")
	}
	out := make(ItemMap, len(raw))
	for name, av := range raw {
		m, ok := av.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("attribute %q must be an AttributeValue object", name)
		}
		out[name] = m
	}
	return out, nil
}

// MarshalItemJSON encodes an ItemMap as AttributeValue JSON bytes.
func MarshalItemJSON(item ItemMap) ([]byte, error) {
	if item == nil {
		item = ItemMap{}
	}
	return json.Marshal(item)
}

// UnmarshalItemJSON decodes AttributeValue map JSON into an ItemMap.
func UnmarshalItemJSON(data []byte) (ItemMap, error) {
	if len(data) == 0 {
		return ItemMap{}, nil
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	return ParseItemMap(raw)
}

// CanonicalAV marshals a single AttributeValue to the store's key string form.
func CanonicalAV(av map[string]any) (string, error) {
	if len(av) == 0 {
		return "", fmt.Errorf("empty AttributeValue")
	}
	b, err := json.Marshal(av)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// PrimaryKeyStrings extracts canonical (itemPK, itemSK) from a Key/Item map
// using table key schema. itemSK is empty for hash-only tables.
func PrimaryKeyStrings(table store.DynamoTable, key ItemMap) (itemPK, itemSK string, err error) {
	if key == nil {
		return "", "", fmt.Errorf("key is required")
	}
	hashAV, ok := key[table.HashKeyName]
	if !ok {
		return "", "", fmt.Errorf("missing hash key %q", table.HashKeyName)
	}
	itemPK, err = CanonicalAV(hashAV)
	if err != nil {
		return "", "", err
	}
	if !table.HasRangeKey() {
		return itemPK, "", nil
	}
	rangeAV, ok := key[table.RangeKeyName]
	if !ok {
		return "", "", fmt.Errorf("missing range key %q", table.RangeKeyName)
	}
	itemSK, err = CanonicalAV(rangeAV)
	if err != nil {
		return "", "", err
	}
	return itemPK, itemSK, nil
}

// KeyFromCanonical rebuilds a Key AttributeValue map from stored PK/SK strings.
func KeyFromCanonical(table store.DynamoTable, itemPK, itemSK string) (ItemMap, error) {
	out := ItemMap{}
	var hashAV map[string]any
	if err := json.Unmarshal([]byte(itemPK), &hashAV); err != nil {
		return nil, fmt.Errorf("decode hash key: %w", err)
	}
	out[table.HashKeyName] = hashAV
	if table.HasRangeKey() && itemSK != "" {
		var rangeAV map[string]any
		if err := json.Unmarshal([]byte(itemSK), &rangeAV); err != nil {
			return nil, fmt.Errorf("decode range key: %w", err)
		}
		out[table.RangeKeyName] = rangeAV
	}
	return out, nil
}

// GSIKeyStrings extracts canonical (gsiPK, gsiSK) from item attributes for the
// first lab GSI. Empty strings are returned when the GSI keys are absent.
func GSIKeyStrings(table store.DynamoTable, item ItemMap) (gsiPK, gsiSK string, err error) {
	if !table.HasGSI() {
		return "", "", nil
	}
	return gsiKeyStringsFor(table.GSIHashKeyName, table.GSIRangeKeyName, table.GSIHasRangeKey(), item)
}

// GSI2KeyStrings extracts canonical keys for the second lab GSI.
func GSI2KeyStrings(table store.DynamoTable, item ItemMap) (gsiPK, gsiSK string, err error) {
	if !table.HasGSI2() {
		return "", "", nil
	}
	return gsiKeyStringsFor(table.GSI2HashKeyName, table.GSI2RangeKeyName, table.GSI2HasRangeKey(), item)
}

func gsiKeyStringsFor(hashName, rangeName string, hasRange bool, item ItemMap) (gsiPK, gsiSK string, err error) {
	hashAV, ok := item[hashName]
	if !ok {
		return "", "", nil
	}
	gsiPK, err = CanonicalAV(hashAV)
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
	gsiSK, err = CanonicalAV(rangeAV)
	if err != nil {
		return "", "", err
	}
	return gsiPK, gsiSK, nil
}

// ItemExpired reports whether a TTL-enabled table should treat the item as expired.
func ItemExpired(table store.DynamoTable, item ItemMap, nowUnix int64) bool {
	if !table.TTLEnabled || table.TTLAttributeName == "" {
		return false
	}
	av, ok := item[table.TTLAttributeName]
	if !ok {
		return false
	}
	n, ok := av["N"].(string)
	if !ok {
		return false
	}
	epoch, err := strconv.ParseInt(n, 10, 64)
	if err != nil || epoch <= 0 {
		return false
	}
	return epoch <= nowUnix
}

// HashKeyFromQueryIndex extracts the hash AttributeValue for a lab Query against
// the base table or a named GSI.
func HashKeyFromQueryIndex(table store.DynamoTable, indexName string, params map[string]any) (map[string]any, error) {
	hashName := table.HashKeyName
	if indexName != "" {
		gsi, _, ok := table.GSIByName(indexName)
		if !ok {
			return nil, fmt.Errorf("index %q not found", indexName)
		}
		hashName = gsi.HashKeyName
	}
	return hashKeyFromQuery(hashName, params)
}

// HashKeyFromQuery extracts the hash AttributeValue for a lab Query on the base table.
func HashKeyFromQuery(table store.DynamoTable, params map[string]any) (map[string]any, error) {
	return HashKeyFromQueryIndex(table, "", params)
}

func hashKeyFromQuery(hashName string, params map[string]any) (map[string]any, error) {
	if kc, ok := params["KeyConditions"].(map[string]any); ok {
		entry, ok := kc[hashName].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("KeyConditions must include hash key %q", hashName)
		}
		op, _ := entry["ComparisonOperator"].(string)
		if op != "" && !strings.EqualFold(op, "EQ") {
			return nil, fmt.Errorf("only EQ ComparisonOperator is supported")
		}
		list, ok := entry["AttributeValueList"].([]any)
		if !ok || len(list) == 0 {
			return nil, fmt.Errorf("AttributeValueList is required")
		}
		av, ok := list[0].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid AttributeValue in KeyConditions")
		}
		return av, nil
	}

	expr, _ := params["KeyConditionExpression"].(string)
	values, _ := params["ExpressionAttributeValues"].(map[string]any)
	names, _ := params["ExpressionAttributeNames"].(map[string]any)
	if strings.TrimSpace(expr) == "" || values == nil {
		return nil, fmt.Errorf("KeyConditions or KeyConditionExpression is required")
	}
	hashNameExpr := hashName
	for k, v := range names {
		if s, ok := v.(string); ok && s == hashName {
			hashNameExpr = k
			break
		}
	}
	// Lab: find ":x" in "Hash = :x" / "#hk = :x".
	parts := strings.Fields(strings.ReplaceAll(expr, "(", " "))
	var placeholder string
	for i, p := range parts {
		token := strings.Trim(p, "()")
		name := token
		if strings.HasPrefix(token, "#") {
			if mapped, ok := names[token].(string); ok {
				name = mapped
			}
		}
		if name == hashName || token == hashNameExpr || token == hashName {
			if i+2 < len(parts) && parts[i+1] == "=" {
				placeholder = strings.Trim(parts[i+2], "()")
				break
			}
		}
	}
	if placeholder == "" || !strings.HasPrefix(placeholder, ":") {
		return nil, fmt.Errorf("unable to parse hash equality from KeyConditionExpression")
	}
	av, ok := values[placeholder].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("ExpressionAttributeValues missing %s", placeholder)
	}
	return av, nil
}

// ApplyUpdateExpression applies lab SET/REMOVE on top-level attributes.
// Key attributes in the Key map are preserved and cannot be removed.
func ApplyUpdateExpression(
	item ItemMap,
	key ItemMap,
	updateExpr string,
	names map[string]any,
	values map[string]any,
) (ItemMap, error) {
	if item == nil {
		item = ItemMap{}
	}
	out := cloneItem(item)
	for k, v := range key {
		out[k] = v
	}
	expr := strings.TrimSpace(updateExpr)
	if expr == "" {
		return nil, fmt.Errorf("UpdateExpression is required")
	}

	upper := strings.ToUpper(expr)
	setIdx := indexKeyword(upper, "SET")
	removeIdx := indexKeyword(upper, "REMOVE")

	var setPart, removePart string
	switch {
	case setIdx >= 0 && removeIdx >= 0:
		if setIdx < removeIdx {
			setPart = expr[setIdx+3 : removeIdx]
			removePart = expr[removeIdx+6:]
		} else {
			removePart = expr[removeIdx+6 : setIdx]
			setPart = expr[setIdx+3:]
		}
	case setIdx >= 0:
		setPart = expr[setIdx+3:]
	case removeIdx >= 0:
		removePart = expr[removeIdx+6:]
	default:
		return nil, fmt.Errorf("UpdateExpression must use SET and/or REMOVE")
	}

	if strings.TrimSpace(setPart) != "" {
		for _, clause := range splitTopLevel(setPart, ',') {
			clause = strings.TrimSpace(clause)
			eq := strings.Index(clause, "=")
			if eq < 0 {
				return nil, fmt.Errorf("invalid SET clause %q", clause)
			}
			left := strings.TrimSpace(clause[:eq])
			right := strings.TrimSpace(clause[eq+1:])
			attr, err := resolveName(left, names)
			if err != nil {
				return nil, err
			}
			if attr == "" || !strings.HasPrefix(right, ":") {
				return nil, fmt.Errorf("invalid SET clause %q", clause)
			}
			av, ok := values[right].(map[string]any)
			if !ok {
				return nil, fmt.Errorf("ExpressionAttributeValues missing %s", right)
			}
			out[attr] = av
		}
	}
	if strings.TrimSpace(removePart) != "" {
		for _, clause := range splitTopLevel(removePart, ',') {
			attr, err := resolveName(strings.TrimSpace(clause), names)
			if err != nil {
				return nil, err
			}
			if _, isKey := key[attr]; isKey {
				continue
			}
			delete(out, attr)
		}
	}
	return out, nil
}

func cloneItem(in ItemMap) ItemMap {
	out := make(ItemMap, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func resolveName(token string, names map[string]any) (string, error) {
	token = strings.TrimSpace(token)
	if strings.HasPrefix(token, "#") {
		if names == nil {
			return "", fmt.Errorf("ExpressionAttributeNames missing %s", token)
		}
		v, ok := names[token].(string)
		if !ok || v == "" {
			return "", fmt.Errorf("ExpressionAttributeNames missing %s", token)
		}
		return v, nil
	}
	return token, nil
}

func indexKeyword(upper, kw string) int {
	idx := 0
	for {
		i := strings.Index(upper[idx:], kw)
		if i < 0 {
			return -1
		}
		i += idx
		beforeOK := i == 0 || isSpace(upper[i-1])
		after := i + len(kw)
		afterOK := after >= len(upper) || isSpace(upper[after])
		if beforeOK && afterOK {
			return i
		}
		idx = i + 1
	}
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

func splitTopLevel(s string, sep rune) []string {
	var out []string
	start := 0
	for i, r := range s {
		if r == sep {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}
