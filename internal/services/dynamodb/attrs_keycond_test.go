package dynamodb_test

import (
	"testing"

	ddb "github.com/Kyaxris-Labs/Noctaxris/internal/services/dynamodb"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestKeyConditionFromQueryIndexBeginsWith(t *testing.T) {
	table := store.DynamoTable{
		HashKeyName:  "Artist",
		RangeKeyName: "SongTitle",
	}
	kc, err := ddb.KeyConditionFromQueryIndex(table, "", map[string]any{
		"KeyConditionExpression": "Artist = :a AND begins_with(SongTitle, :p)",
		"ExpressionAttributeValues": map[string]any{
			":a": map[string]any{"S": "Radiohead"},
			":p": map[string]any{"S": "K"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if kc.RangeOp != "BEGINS_WITH" || len(kc.RangeValues) != 1 {
		t.Fatalf("kc=%+v", kc)
	}
	item := ddb.ItemMap{
		"Artist":    map[string]any{"S": "Radiohead"},
		"SongTitle": map[string]any{"S": "Karma Police"},
	}
	if !ddb.ItemMatchesSortKey(item, "SongTitle", kc.RangeOp, kc.RangeValues) {
		t.Fatal("expected match")
	}
	miss := ddb.ItemMap{
		"Artist":    map[string]any{"S": "Radiohead"},
		"SongTitle": map[string]any{"S": "Creep"},
	}
	if ddb.ItemMatchesSortKey(miss, "SongTitle", kc.RangeOp, kc.RangeValues) {
		t.Fatal("expected miss")
	}
}
