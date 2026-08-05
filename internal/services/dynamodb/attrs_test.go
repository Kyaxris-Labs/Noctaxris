package dynamodb_test

import (
	"encoding/json"
	"strings"
	"testing"

	ddb "github.com/Kyaxris-Labs/Noctaxris/internal/services/dynamodb"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestKeyConditionsLegacyAndIndexKeys(t *testing.T) {
	table := store.DynamoTable{
		HashKeyName:      "Artist",
		HashKeyType:      store.KeyTypeString,
		RangeKeyName:     "SongTitle",
		RangeKeyType:     store.KeyTypeString,
		GSIName:          "GenreIndex",
		GSIHashKeyName:   "Genre",
		GSIHashKeyType:   store.KeyTypeString,
		GSIRangeKeyName:  "Year",
		GSIRangeKeyType:  store.KeyTypeNumber,
		GSI2Name:         "LabelIndex",
		GSI2HashKeyName:  "Label",
		GSI2HashKeyType:  store.KeyTypeString,
		LSIName:          "ByStatus",
		LSIRangeKeyName:  "Status",
		LSIRangeKeyType:  store.KeyTypeString,
		LSI2Name:         "ByAlbum",
		LSI2RangeKeyName: "Album",
		LSI2RangeKeyType: store.KeyTypeString,
	}
	item := ddb.ItemMap{
		"Artist":    map[string]any{"S": "Radiohead"},
		"SongTitle": map[string]any{"S": "Karma Police"},
		"Genre":     map[string]any{"S": "Rock"},
		"Year":      map[string]any{"N": "1997"},
		"Label":     map[string]any{"S": "XL"},
		"Status":    map[string]any{"S": "published"},
		"Album":     map[string]any{"S": "OK Computer"},
	}

	kc, err := ddb.KeyConditionFromQueryIndex(table, "", map[string]any{
		"KeyConditions": map[string]any{
			"Artist": map[string]any{
				"ComparisonOperator": "EQ",
				"AttributeValueList": []any{map[string]any{"S": "Radiohead"}},
			},
			"SongTitle": map[string]any{
				"ComparisonOperator": "BEGINS_WITH",
				"AttributeValueList": []any{map[string]any{"S": "K"}},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ddb.ItemMatchesSortKey(item, "SongTitle", kc.RangeOp, kc.RangeValues) {
		t.Fatalf("kc=%+v", kc)
	}

	hk, err := ddb.HashKeyFromQuery(table, map[string]any{
		"KeyConditions": map[string]any{
			"Artist": map[string]any{
				"ComparisonOperator": "EQ",
				"AttributeValueList": []any{map[string]any{"S": "Radiohead"}},
			},
		},
	})
	if err != nil || hk["S"] != "Radiohead" {
		t.Fatalf("hk=%v err=%v", hk, err)
	}

	gsiKC, err := ddb.KeyConditionFromQueryIndex(table, "GenreIndex", map[string]any{
		"KeyConditionExpression": "Genre = :g AND Year >= :y",
		"ExpressionAttributeValues": map[string]any{
			":g": map[string]any{"S": "Rock"},
			":y": map[string]any{"N": "1990"},
		},
	})
	if err != nil || gsiKC.RangeOp != "GE" {
		t.Fatalf("gsi kc=%+v err=%v", gsiKC, err)
	}

	gpk, gsk, err := ddb.GSIKeyStrings(table, item)
	if err != nil || gpk == "" {
		t.Fatalf("gsi keys pk=%q sk=%q err=%v", gpk, gsk, err)
	}
	gpk2, gsk2, err := ddb.GSI2KeyStrings(table, item)
	if err != nil || gpk2 == "" {
		t.Fatalf("gsi2 keys pk=%q sk=%q err=%v", gpk2, gsk2, err)
	}
	lsiSK, err := ddb.LSIKeyStrings(table, item)
	if err != nil || lsiSK == "" {
		t.Fatalf("lsi sk=%q err=%v", lsiSK, err)
	}
	lsi2SK, err := ddb.LSI2KeyStrings(table, item)
	if err != nil || lsi2SK == "" {
		t.Fatalf("lsi2 sk=%q err=%v", lsi2SK, err)
	}

	beginsKC, err := ddb.KeyConditionFromQueryIndex(table, "", map[string]any{
		"KeyConditionExpression": "Artist = :a AND begins_with(SongTitle, :p)",
		"ExpressionAttributeValues": map[string]any{
			":a": map[string]any{"S": "Radiohead"},
			":p": map[string]any{"S": "K"},
		},
	})
	if err != nil || beginsKC.RangeOp != "BEGINS_WITH" {
		t.Fatalf("begins kc=%+v err=%v", beginsKC, err)
	}
	if _, err := ddb.KeyConditionFromQueryIndex(table, "BadIndex", map[string]any{
		"KeyConditionExpression":    "Artist = :a",
		"ExpressionAttributeValues": map[string]any{":a": map[string]any{"S": "x"}},
	}); err == nil {
		t.Fatal("expected bad index error")
	}
	if _, err := ddb.KeyConditionFromQueryIndex(table, "", map[string]any{
		"KeyConditions": map[string]any{
			"Artist": map[string]any{
				"ComparisonOperator": "LT",
				"AttributeValueList": []any{map[string]any{"S": "x"}},
			},
		},
	}); err == nil {
		t.Fatal("expected hash LT error")
	}
	_, _, err = ddb.PrimaryKeyStrings(table, ddb.ItemMap{})
	if err == nil {
		t.Fatal("expected missing hash key error")
	}
}

func TestConditionComparisonsAndUpdateBranches(t *testing.T) {
	item := ddb.ItemMap{
		"Score": map[string]any{"N": "90"},
		"Name":  map[string]any{"S": "lab"},
	}
	if err := ddb.EvaluateConditionExpression(item, "Score <> :n OR Name = :s", nil, map[string]any{
		":n": map[string]any{"N": "80"},
		":s": map[string]any{"S": "other"},
	}); err != nil {
		t.Fatalf("or cmp err=%v", err)
	}
	if err := ddb.EvaluateConditionExpression(item, "attribute_not_exists(Missing)", nil, nil); err != nil {
		t.Fatalf("not exists err=%v", err)
	}
	if err := ddb.EvaluateConditionExpression(item, "Score < :hi AND Score > :lo", nil, map[string]any{
		":hi": map[string]any{"N": "100"},
		":lo": map[string]any{"N": "50"},
	}); err != nil {
		t.Fatalf("and cmp err=%v", err)
	}

	key := ddb.ItemMap{"id": map[string]any{"S": "1"}}
	out, err := ddb.ApplyUpdateExpression(item, key, "REMOVE Name", nil, nil)
	if err != nil || out["Name"] != nil {
		t.Fatalf("remove=%v err=%v", out, err)
	}
	out2, err := ddb.ApplyUpdateExpression(item, key, "SET #n = :n REMOVE Score", map[string]any{"#n": "Name"}, map[string]any{":n": map[string]any{"S": "new"}})
	if err != nil || out2["Name"]["S"] != "new" {
		t.Fatalf("set+remove=%v err=%v", out2, err)
	}
	if _, err := ddb.ApplyUpdateExpression(item, key, "SET bad", nil, nil); err == nil {
		t.Fatal("expected bad set")
	}
}

func TestItemMatchesSortKeyOps(t *testing.T) {
	item := ddb.ItemMap{"SongTitle": map[string]any{"S": "Karma Police"}}
	if !ddb.ItemMatchesSortKey(item, "SongTitle", "EQ", []map[string]any{{"S": "Karma Police"}}) {
		t.Fatal("eq")
	}
	if !ddb.ItemMatchesSortKey(item, "SongTitle", "GE", []map[string]any{{"S": "A"}}) {
		t.Fatal("ge")
	}
	if ddb.ItemMatchesSortKey(item, "SongTitle", "LT", []map[string]any{{"S": "A"}}) {
		t.Fatal("lt miss")
	}
	if !ddb.ItemMatchesSortKey(item, "", "", nil) {
		t.Fatal("empty op")
	}
}

func TestAttrsRangeOpsAndFilter(t *testing.T) {
	table := store.DynamoTable{
		HashKeyName:  "Artist",
		HashKeyType:  store.KeyTypeString,
		RangeKeyName: "SongTitle",
		RangeKeyType: store.KeyTypeString,
	}
	item := ddb.ItemMap{
		"Artist":    map[string]any{"S": "Radiohead"},
		"SongTitle": map[string]any{"S": "Karma Police"},
		"Score":     map[string]any{"N": "90"},
	}
	params := map[string]any{
		"KeyConditionExpression": "#a = :a AND #s BETWEEN :lo AND :hi",
		"ExpressionAttributeNames": map[string]any{"#a": "Artist", "#s": "SongTitle"},
		"ExpressionAttributeValues": map[string]any{
			":a":  map[string]any{"S": "Radiohead"},
			":lo": map[string]any{"S": "A"},
			":hi": map[string]any{"S": "Z"},
		},
	}
	kc, err := ddb.KeyConditionFromQueryIndex(table, "", params)
	if err != nil {
		t.Fatal(err)
	}
	if kc.RangeOp != "BETWEEN" || !ddb.ItemMatchesSortKey(item, "SongTitle", kc.RangeOp, kc.RangeValues) {
		t.Fatalf("kc=%+v", kc)
	}

	ltParams := map[string]any{
		"KeyConditionExpression": "Artist = :a AND SongTitle < :t",
		"ExpressionAttributeValues": map[string]any{
			":a": map[string]any{"S": "Radiohead"},
			":t": map[string]any{"S": "Z"},
		},
	}
	kcLt, err := ddb.KeyConditionFromQueryIndex(table, "", ltParams)
	if err != nil {
		t.Fatal(err)
	}
	if !ddb.ItemMatchesSortKey(item, "SongTitle", kcLt.RangeOp, kcLt.RangeValues) {
		t.Fatal("expected lt match")
	}

	if err := ddb.EvaluateFilterExpression(item, "Score > :n", nil, map[string]any{":n": map[string]any{"N": "80"}}); err != nil {
		t.Fatalf("filter err=%v", err)
	}
	if err := ddb.EvaluateConditionExpression(item, "attribute_exists(Score)", nil, nil); err != nil {
		t.Fatalf("cond err=%v", err)
	}

	batchRaw, err := ddb.BatchExecuteStatementJSON([]map[string]any{{"Item": item}})
	if err != nil {
		t.Fatal(err)
	}
	execRaw, err := ddb.ExecuteStatementJSON([]ddb.ItemMap{item})
	if err != nil {
		t.Fatal(err)
	}
	var execOut map[string]any
	if err := json.Unmarshal(execRaw, &execOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(batchRaw), "Responses") || execOut["Items"] == nil {
		t.Fatalf("batch=%s exec=%v", batchRaw, execOut)
	}
}
