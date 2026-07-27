package store_test

import (
	"errors"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestDynamoDBLSICreateAndQuery(t *testing.T) {
	st := openKMSStore(t)
	account := "000000000001"
	lsis := []store.DynamoLSI{
		{IndexName: "ByStatus", RangeKeyName: "status", RangeKeyType: store.KeyTypeString},
	}
	tbl, err := st.CreateTableWithIndexes(account, "us-east-1", "Orders",
		"pk", store.KeyTypeString, "sk", store.KeyTypeString, "", "", nil, lsis)
	if err != nil {
		t.Fatal(err)
	}
	if !tbl.HasLSI() || tbl.LSIName != "ByStatus" {
		t.Fatalf("LSI missing: %+v", tbl)
	}
	got, err := st.DescribeTable(account, "Orders")
	if err != nil || !got.HasLSI() {
		t.Fatalf("describe LSI=%+v err=%v", got, err)
	}

	if err := st.PutItemBytesIndexed(account, "Orders",
		`{"S":"u1"}`, `{"S":"o1"}`, "", "", "", "", `{"S":"OPEN"}`, "",
		[]byte(`{"pk":{"S":"u1"},"sk":{"S":"o1"},"status":{"S":"OPEN"}}`), false, nil); err != nil {
		t.Fatal(err)
	}
	if err := st.PutItemBytesIndexed(account, "Orders",
		`{"S":"u1"}`, `{"S":"o2"}`, "", "", "", "", `{"S":"CLOSED"}`, "",
		[]byte(`{"pk":{"S":"u1"},"sk":{"S":"o2"},"status":{"S":"CLOSED"}}`), false, nil); err != nil {
		t.Fatal(err)
	}

	page, err := st.QueryLSISlotItems(account, "Orders", 1, `{"S":"u1"}`, 0, "")
	if err != nil || len(page.Items) != 2 {
		t.Fatalf("page=%+v err=%v", page, err)
	}

	_, err = st.CreateTableWithIndexes(account, "us-east-1", "NoRange",
		"pk", store.KeyTypeString, "", "", "", "", nil, lsis)
	if !errors.Is(err, store.ErrLSIRequiresRange) {
		t.Fatalf("want ErrLSIRequiresRange got %v", err)
	}
}

func TestDynamoDBLSIMaxTwo(t *testing.T) {
	st := openKMSStore(t)
	lsis := []store.DynamoLSI{
		{IndexName: "A", RangeKeyName: "a", RangeKeyType: store.KeyTypeString},
		{IndexName: "B", RangeKeyName: "b", RangeKeyType: store.KeyTypeString},
		{IndexName: "C", RangeKeyName: "c", RangeKeyType: store.KeyTypeString},
	}
	_, err := st.CreateTableWithIndexes("000000000001", "us-east-1", "T",
		"pk", store.KeyTypeString, "sk", store.KeyTypeString, "", "", nil, lsis)
	if !errors.Is(err, store.ErrTooManyLSIs) {
		t.Fatalf("want ErrTooManyLSIs got %v", err)
	}
}
