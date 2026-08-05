package dynamodb_test

import (
	"path/filepath"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	key, err := store.LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestTransactExprRegistrationInvoked(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	if _, err := st.CreateTable(account, "us-east-1", "TransactLab", "Id", store.KeyTypeString, "", "", "", "", nil); err != nil {
		t.Fatal(err)
	}
	if err := st.PutItemBytes(account, "TransactLab", `{"S":"1"}`, "", "", "", []byte(`{"Id":{"S":"1"},"Score":{"N":"1"}}`), false, nil); err != nil {
		t.Fatal(err)
	}
	err := st.TransactWriteItems(account, []store.TransactWriteAction{
		{
			Kind:                      "Update",
			TableName:                 "TransactLab",
			ItemPK:                    `{"S":"1"}`,
			KeyJSON:                   []byte(`{"Id":{"S":"1"}}`),
			UpdateExpression:          "SET Score = :s",
			ConditionExpression:       "attribute_exists(Score)",
			ExpressionAttributeValues: map[string]any{":s": map[string]any{"N": "2"}},
		},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	got, err := st.GetItemBytes(account, "TransactLab", `{"S":"1"}`, "")
	if err != nil {
		t.Fatal(err)
	}
	if !containsBytes(got.ItemJSON, `"N":"2"`) {
		t.Fatalf("item=%s", got.ItemJSON)
	}
}

func containsBytes(b []byte, sub string) bool {
	return len(sub) == 0 || string(b) != "" && indexString(string(b), sub) >= 0
}

func indexString(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
