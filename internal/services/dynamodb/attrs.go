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
	kc, err := KeyConditionFromQueryIndex(table, indexName, params)
	if err != nil {
		return nil, err
	}
	return kc.HashAV, nil
}

// KeyCondition holds parsed hash equality and optional sort-key range from a Query.
type KeyCondition struct {
	HashAV      map[string]any
	RangeOp     string // "", EQ, LT, LE, GT, GE, BETWEEN, BEGINS_WITH
	RangeValues []map[string]any
}

// KeyConditionFromQueryIndex parses KeyConditions or KeyConditionExpression for
// the base table or a named GSI, including lab sort-key operators.
func KeyConditionFromQueryIndex(table store.DynamoTable, indexName string, params map[string]any) (KeyCondition, error) {
	hashName := table.HashKeyName
	rangeName := table.RangeKeyName
	if indexName != "" {
		gsi, _, ok := table.GSIByName(indexName)
		if !ok {
			return KeyCondition{}, fmt.Errorf("index %q not found", indexName)
		}
		hashName = gsi.HashKeyName
		rangeName = gsi.RangeKeyName
	}
	return keyConditionFromQuery(hashName, rangeName, params)
}

// HashKeyFromQuery extracts the hash AttributeValue for a lab Query on the base table.
func HashKeyFromQuery(table store.DynamoTable, params map[string]any) (map[string]any, error) {
	return HashKeyFromQueryIndex(table, "", params)
}

func keyConditionFromQuery(hashName, rangeName string, params map[string]any) (KeyCondition, error) {
	if kc, ok := params["KeyConditions"].(map[string]any); ok {
		entry, ok := kc[hashName].(map[string]any)
		if !ok {
			return KeyCondition{}, fmt.Errorf("KeyConditions must include hash key %q", hashName)
		}
		op, _ := entry["ComparisonOperator"].(string)
		if op != "" && !strings.EqualFold(op, "EQ") {
			return KeyCondition{}, fmt.Errorf("only EQ ComparisonOperator is supported for hash key")
		}
		list, ok := entry["AttributeValueList"].([]any)
		if !ok || len(list) == 0 {
			return KeyCondition{}, fmt.Errorf("AttributeValueList is required")
		}
		av, ok := list[0].(map[string]any)
		if !ok {
			return KeyCondition{}, fmt.Errorf("invalid AttributeValue in KeyConditions")
		}
		out := KeyCondition{HashAV: av}
		if rangeName != "" {
			if re, ok := kc[rangeName].(map[string]any); ok {
				rop, _ := re["ComparisonOperator"].(string)
				rlist, _ := re["AttributeValueList"].([]any)
				vals, err := avListToMaps(rlist)
				if err != nil {
					return KeyCondition{}, err
				}
				out.RangeOp = strings.ToUpper(strings.TrimSpace(rop))
				out.RangeValues = vals
				if err := validateRangeOp(out.RangeOp, len(out.RangeValues)); err != nil {
					return KeyCondition{}, err
				}
			}
		}
		return out, nil
	}

	expr, _ := params["KeyConditionExpression"].(string)
	values, _ := params["ExpressionAttributeValues"].(map[string]any)
	names, _ := params["ExpressionAttributeNames"].(map[string]any)
	if strings.TrimSpace(expr) == "" || values == nil {
		return KeyCondition{}, fmt.Errorf("KeyConditions or KeyConditionExpression is required")
	}
	hashAV, err := hashKeyFromExpression(hashName, expr, names, values)
	if err != nil {
		return KeyCondition{}, err
	}
	out := KeyCondition{HashAV: hashAV}
	if rangeName != "" {
		rop, rvals, rerr := rangeKeyFromExpression(rangeName, expr, names, values)
		if rerr != nil {
			return KeyCondition{}, rerr
		}
		out.RangeOp = rop
		out.RangeValues = rvals
	}
	return out, nil
}

func hashKeyFromQuery(hashName string, params map[string]any) (map[string]any, error) {
	kc, err := keyConditionFromQuery(hashName, "", params)
	if err != nil {
		return nil, err
	}
	return kc.HashAV, nil
}

func hashKeyFromExpression(hashName, expr string, names, values map[string]any) (map[string]any, error) {
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
				placeholder = strings.Trim(parts[i+2], "(),")
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

func rangeKeyFromExpression(rangeName, expr string, names, values map[string]any) (op string, vals []map[string]any, err error) {
	rangeNameExpr := rangeName
	for k, v := range names {
		if s, ok := v.(string); ok && s == rangeName {
			rangeNameExpr = k
			break
		}
	}
	normalized := strings.ReplaceAll(expr, "(", " ")
	normalized = strings.ReplaceAll(normalized, ")", " ")
	normalized = strings.ReplaceAll(normalized, ",", " ")
	parts := strings.Fields(normalized)
	for i, p := range parts {
		token := strings.Trim(p, ",")
		name := resolveExprName(token, names)
		if name != rangeName && token != rangeNameExpr && token != rangeName {
			// begins_with(#sk, :p) form
			if strings.EqualFold(token, "begins_with") && i+2 < len(parts) {
				attrTok := strings.Trim(parts[i+1], ",")
				attrName := resolveExprName(attrTok, names)
				if attrName != rangeName && attrTok != rangeNameExpr && attrTok != rangeName {
					continue
				}
				ph := strings.Trim(parts[i+2], ",")
				if !strings.HasPrefix(ph, ":") {
					return "", nil, fmt.Errorf("begins_with requires a value placeholder")
				}
				av, ok := values[ph].(map[string]any)
				if !ok {
					return "", nil, fmt.Errorf("ExpressionAttributeValues missing %s", ph)
				}
				return "BEGINS_WITH", []map[string]any{av}, nil
			}
			continue
		}
		if i+1 >= len(parts) {
			break
		}
		cmp := strings.ToUpper(strings.Trim(parts[i+1], ","))
		switch cmp {
		case "=", "EQ":
			if i+2 >= len(parts) {
				return "", nil, fmt.Errorf("incomplete sort key EQ in KeyConditionExpression")
			}
			ph := strings.Trim(parts[i+2], ",")
			av, ok := values[ph].(map[string]any)
			if !ok {
				return "", nil, fmt.Errorf("ExpressionAttributeValues missing %s", ph)
			}
			return "EQ", []map[string]any{av}, nil
		case "<", "LT":
			ph := strings.Trim(parts[i+2], ",")
			av, ok := values[ph].(map[string]any)
			if !ok {
				return "", nil, fmt.Errorf("ExpressionAttributeValues missing %s", ph)
			}
			return "LT", []map[string]any{av}, nil
		case "<=", "LE":
			ph := strings.Trim(parts[i+2], ",")
			av, ok := values[ph].(map[string]any)
			if !ok {
				return "", nil, fmt.Errorf("ExpressionAttributeValues missing %s", ph)
			}
			return "LE", []map[string]any{av}, nil
		case ">", "GT":
			ph := strings.Trim(parts[i+2], ",")
			av, ok := values[ph].(map[string]any)
			if !ok {
				return "", nil, fmt.Errorf("ExpressionAttributeValues missing %s", ph)
			}
			return "GT", []map[string]any{av}, nil
		case ">=", "GE":
			ph := strings.Trim(parts[i+2], ",")
			av, ok := values[ph].(map[string]any)
			if !ok {
				return "", nil, fmt.Errorf("ExpressionAttributeValues missing %s", ph)
			}
			return "GE", []map[string]any{av}, nil
		case "BETWEEN":
			if i+4 >= len(parts) || !strings.EqualFold(parts[i+3], "AND") {
				return "", nil, fmt.Errorf("invalid BETWEEN in KeyConditionExpression")
			}
			ph1 := strings.Trim(parts[i+2], ",")
			ph2 := strings.Trim(parts[i+4], ",")
			av1, ok1 := values[ph1].(map[string]any)
			av2, ok2 := values[ph2].(map[string]any)
			if !ok1 || !ok2 {
				return "", nil, fmt.Errorf("ExpressionAttributeValues missing BETWEEN placeholders")
			}
			return "BETWEEN", []map[string]any{av1, av2}, nil
		}
	}
	return "", nil, nil
}

func resolveExprName(token string, names map[string]any) string {
	token = strings.TrimSpace(token)
	if strings.HasPrefix(token, "#") {
		if mapped, ok := names[token].(string); ok {
			return mapped
		}
	}
	return token
}

func avListToMaps(list []any) ([]map[string]any, error) {
	out := make([]map[string]any, 0, len(list))
	for _, raw := range list {
		av, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid AttributeValue in KeyConditions")
		}
		out = append(out, av)
	}
	return out, nil
}

func validateRangeOp(op string, n int) error {
	switch op {
	case "EQ", "LT", "LE", "GT", "GE", "BEGINS_WITH":
		if n != 1 {
			return fmt.Errorf("sort key %s requires one value", op)
		}
	case "BETWEEN":
		if n != 2 {
			return fmt.Errorf("sort key BETWEEN requires two values")
		}
	case "":
		return nil
	default:
		return fmt.Errorf("unsupported sort key ComparisonOperator %q", op)
	}
	return nil
}

// ItemMatchesSortKey reports whether item's range attribute satisfies the condition.
func ItemMatchesSortKey(item ItemMap, rangeName string, op string, values []map[string]any) bool {
	if op == "" || rangeName == "" {
		return true
	}
	av, ok := item[rangeName]
	if !ok {
		return false
	}
	itemCanon, err := CanonicalAV(av)
	if err != nil {
		return false
	}
	switch op {
	case "EQ":
		want, err := CanonicalAV(values[0])
		return err == nil && itemCanon == want
	case "LT":
		want, err := CanonicalAV(values[0])
		return err == nil && itemCanon < want
	case "LE":
		want, err := CanonicalAV(values[0])
		return err == nil && itemCanon <= want
	case "GT":
		want, err := CanonicalAV(values[0])
		return err == nil && itemCanon > want
	case "GE":
		want, err := CanonicalAV(values[0])
		return err == nil && itemCanon >= want
	case "BETWEEN":
		lo, err1 := CanonicalAV(values[0])
		hi, err2 := CanonicalAV(values[1])
		return err1 == nil && err2 == nil && itemCanon >= lo && itemCanon <= hi
	case "BEGINS_WITH":
		prefix := scalarString(values[0])
		got := scalarString(av)
		return prefix != "" && strings.HasPrefix(got, prefix)
	default:
		return false
	}
}

func scalarString(av map[string]any) string {
	if s, ok := av["S"].(string); ok {
		return s
	}
	if n, ok := av["N"].(string); ok {
		return n
	}
	return ""
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
