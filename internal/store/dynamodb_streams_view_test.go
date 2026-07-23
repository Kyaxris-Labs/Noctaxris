package store_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestDynamoStreamViewOldImageAndNewAndOld(t *testing.T) {
	st := openDynamoStore(t)
	account := "000000000001"

	table, err := st.CreateTable(account, "us-east-1", "ViewOld", "pk", store.KeyTypeString, "", "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpdateTableStreamSpec(account, table.TableName, true, store.StreamViewOldImage); err != nil {
		t.Fatal(err)
	}
	oldItem, _ := json.Marshal(map[string]any{"pk": map[string]string{"S": "1"}, "v": map[string]string{"S": "old"}})
	newItem, _ := json.Marshal(map[string]any{"pk": map[string]string{"S": "1"}, "v": map[string]string{"S": "new"}})
	keys, _ := store.DynamoStreamKeysJSON(table, `{"S":"1"}`, "")
	if err := st.AppendDynamoStreamRecord(account, table.TableName, "MODIFY", keys, newItem, oldItem); err != nil {
		t.Fatal(err)
	}
	it, err := st.GetDynamoStreamShardIterator(account, table.TableName, store.LabDynamoStreamShardID, "TRIM_HORIZON", "")
	if err != nil {
		t.Fatal(err)
	}
	recs, _, err := st.GetDynamoStreamRecords(it, 10)
	if err != nil || len(recs) != 1 {
		t.Fatalf("recs=%v err=%v", recs, err)
	}
	if recs[0].NewImageJSON != "" {
		t.Fatalf("OLD_IMAGE must omit NewImage, got %q", recs[0].NewImageJSON)
	}
	if !strings.Contains(recs[0].OldImageJSON, `"old"`) {
		t.Fatalf("OldImage=%q", recs[0].OldImageJSON)
	}
	if recs[0].StreamViewType != store.StreamViewOldImage {
		t.Fatalf("StreamViewType=%q", recs[0].StreamViewType)
	}

	table2, err := st.CreateTable(account, "us-east-1", "ViewBoth", "pk", store.KeyTypeString, "", "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpdateTableStreamSpec(account, table2.TableName, true, store.StreamViewNewAndOldImages); err != nil {
		t.Fatal(err)
	}
	keys2, _ := store.DynamoStreamKeysJSON(table2, `{"S":"2"}`, "")
	if err := st.AppendDynamoStreamRecord(account, table2.TableName, "MODIFY", keys2, newItem, oldItem); err != nil {
		t.Fatal(err)
	}
	it2, err := st.GetDynamoStreamShardIterator(account, table2.TableName, store.LabDynamoStreamShardID, "TRIM_HORIZON", "")
	if err != nil {
		t.Fatal(err)
	}
	recs2, _, err := st.GetDynamoStreamRecords(it2, 10)
	if err != nil || len(recs2) != 1 {
		t.Fatalf("recs2=%v err=%v", recs2, err)
	}
	if !strings.Contains(recs2[0].NewImageJSON, `"new"`) || !strings.Contains(recs2[0].OldImageJSON, `"old"`) {
		t.Fatalf("NEW_AND_OLD_IMAGES record=%+v", recs2[0])
	}
}

func TestDynamoStreamViewRejectUnknown(t *testing.T) {
	st := openDynamoStore(t)
	account := "000000000001"
	if _, err := st.CreateTable(account, "us-east-1", "BadView", "pk", store.KeyTypeString, "", "", "", "", nil); err != nil {
		t.Fatal(err)
	}
	_, err := st.UpdateTableStreamSpec(account, "BadView", true, "KEYS_AND_NEW")
	if err != store.ErrDynamoStreamBadView {
		t.Fatalf("want ErrDynamoStreamBadView got %v", err)
	}
}
