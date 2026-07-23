package store_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"

	_ "github.com/Kyaxris-Labs/Noctaxris/internal/services/dynamodb" // register transact expr helpers
)

func TestTransactWriteItemsAtomicPutPair(t *testing.T) {
	st := openDynamoStore(t)
	account := "000000000001"
	if _, err := st.CreateTable(account, "us-east-1", "Orders", "pk", store.KeyTypeString, "", "", "", "", nil); err != nil {
		t.Fatal(err)
	}

	err := st.TransactWriteItems(account, []store.TransactWriteAction{
		{
			Kind: "Put", TableName: "Orders",
			ItemPK: `{"S":"a"}`, ItemJSON: []byte(`{"pk":{"S":"a"},"v":{"S":"1"}}`),
		},
		{
			Kind: "Put", TableName: "Orders",
			ItemPK: `{"S":"b"}`, ItemJSON: []byte(`{"pk":{"S":"b"},"v":{"S":"2"}}`),
		},
	}, "")
	if err != nil {
		t.Fatal(err)
	}

	a, err := st.GetItemBytes(account, "Orders", `{"S":"a"}`, "")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(a.ItemJSON, []byte(`"1"`)) {
		t.Fatalf("item a=%s", a.ItemJSON)
	}
	b, err := st.GetItemBytes(account, "Orders", `{"S":"b"}`, "")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b.ItemJSON, []byte(`"2"`)) {
		t.Fatalf("item b=%s", b.ItemJSON)
	}
}

func TestTransactWriteItemsCancelMissingConditionCheck(t *testing.T) {
	st := openDynamoStore(t)
	account := "000000000001"
	if _, err := st.CreateTable(account, "us-east-1", "Orders", "pk", store.KeyTypeString, "", "", "", "", nil); err != nil {
		t.Fatal(err)
	}

	err := st.TransactWriteItems(account, []store.TransactWriteAction{
		{
			Kind: "Put", TableName: "Orders",
			ItemPK: `{"S":"new"}`, ItemJSON: []byte(`{"pk":{"S":"new"}}`),
		},
		{
			Kind: "ConditionCheck", TableName: "Orders",
			ItemPK: `{"S":"missing"}`, ConditionEmpty: true,
		},
	}, "")
	var canceled *store.TransactionCanceledError
	if !errors.As(err, &canceled) {
		t.Fatalf("want TransactionCanceledError, got %v", err)
	}
	if _, err := st.GetItemBytes(account, "Orders", `{"S":"new"}`, ""); !errors.Is(err, store.ErrNoSuchItem) {
		t.Fatalf("put must roll back, got %v", err)
	}
}

func TestTransactWriteItemsDuplicateKeyCancels(t *testing.T) {
	st := openDynamoStore(t)
	account := "000000000001"
	if _, err := st.CreateTable(account, "us-east-1", "Orders", "pk", store.KeyTypeString, "", "", "", "", nil); err != nil {
		t.Fatal(err)
	}

	err := st.TransactWriteItems(account, []store.TransactWriteAction{
		{
			Kind: "Put", TableName: "Orders",
			ItemPK: `{"S":"dup"}`, ItemJSON: []byte(`{"pk":{"S":"dup"},"v":{"S":"1"}}`),
		},
		{
			Kind: "Delete", TableName: "Orders",
			ItemPK: `{"S":"dup"}`,
		},
	}, "")
	var canceled *store.TransactionCanceledError
	if !errors.As(err, &canceled) {
		t.Fatalf("want TransactionCanceledError, got %v", err)
	}
}

func TestTransactWriteUpdateWithCondition(t *testing.T) {
	st := openDynamoStore(t)
	account := "000000000001"
	if _, err := st.CreateTable(account, "us-east-1", "Orders", "pk", store.KeyTypeString, "", "", "", "", nil); err != nil {
		t.Fatal(err)
	}
	if err := st.PutItemBytes(account, "Orders", `{"S":"acct"}`, "", "", "",
		[]byte(`{"pk":{"S":"acct"},"balance":{"N":"10"}}`), false, nil); err != nil {
		t.Fatal(err)
	}

	err := st.TransactWriteItems(account, []store.TransactWriteAction{
		{
			Kind: "Update", TableName: "Orders",
			ItemPK:           `{"S":"acct"}`,
			KeyJSON:          []byte(`{"pk":{"S":"acct"}}`),
			UpdateExpression: "SET balance = :n",
			ConditionExpression: "balance = :old",
			ExpressionAttributeValues: map[string]any{
				":n":   map[string]any{"N": "20"},
				":old": map[string]any{"N": "10"},
			},
		},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	got, err := st.GetItemBytes(account, "Orders", `{"S":"acct"}`, "")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got.ItemJSON, []byte(`"20"`)) {
		t.Fatalf("balance not updated: %s", got.ItemJSON)
	}

	err = st.TransactWriteItems(account, []store.TransactWriteAction{
		{
			Kind: "Update", TableName: "Orders",
			ItemPK:           `{"S":"acct"}`,
			KeyJSON:          []byte(`{"pk":{"S":"acct"}}`),
			UpdateExpression: "SET balance = :n",
			ConditionExpression: "balance = :old",
			ExpressionAttributeValues: map[string]any{
				":n":   map[string]any{"N": "30"},
				":old": map[string]any{"N": "10"},
			},
		},
	}, "")
	var canceled *store.TransactionCanceledError
	if !errors.As(err, &canceled) {
		t.Fatalf("want TransactionCanceledError, got %v", err)
	}
	if len(canceled.Reasons) != 1 || canceled.Reasons[0].Code != "ConditionalCheckFailed" {
		t.Fatalf("reasons=%+v", canceled.Reasons)
	}
	got, err = st.GetItemBytes(account, "Orders", `{"S":"acct"}`, "")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got.ItemJSON, []byte(`"20"`)) {
		t.Fatalf("balance must stay 20 after cancel: %s", got.ItemJSON)
	}
}

func TestTransactWritePutDeleteConditionExpression(t *testing.T) {
	st := openDynamoStore(t)
	account := "000000000001"
	if _, err := st.CreateTable(account, "us-east-1", "Orders", "pk", store.KeyTypeString, "", "", "", "", nil); err != nil {
		t.Fatal(err)
	}

	err := st.TransactWriteItems(account, []store.TransactWriteAction{
		{
			Kind: "Put", TableName: "Orders",
			ItemPK:   `{"S":"new"}`,
			ItemJSON: []byte(`{"pk":{"S":"new"},"v":{"S":"1"}}`),
			ConditionExpression: "attribute_not_exists(pk)",
		},
	}, "")
	if err != nil {
		t.Fatal(err)
	}

	err = st.TransactWriteItems(account, []store.TransactWriteAction{
		{
			Kind: "Put", TableName: "Orders",
			ItemPK:   `{"S":"new"}`,
			ItemJSON: []byte(`{"pk":{"S":"new"},"v":{"S":"2"}}`),
			ConditionExpression: "attribute_not_exists(pk)",
		},
	}, "")
	var canceled *store.TransactionCanceledError
	if !errors.As(err, &canceled) {
		t.Fatalf("want TransactionCanceledError, got %v", err)
	}

	err = st.TransactWriteItems(account, []store.TransactWriteAction{
		{
			Kind: "Delete", TableName: "Orders",
			ItemPK: `{"S":"new"}`,
			ConditionExpression: "v = :want",
			ExpressionAttributeValues: map[string]any{
				":want": map[string]any{"S": "1"},
			},
		},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetItemBytes(account, "Orders", `{"S":"new"}`, ""); !errors.Is(err, store.ErrNoSuchItem) {
		t.Fatalf("want deleted, got %v", err)
	}
}

func TestTransactWriteAppendsStreamRecords(t *testing.T) {
	st := openDynamoStore(t)
	account := "000000000001"
	table, err := st.CreateTable(account, "us-east-1", "StreamTxn", "pk", store.KeyTypeString, "", "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpdateTableStreamSpec(account, table.TableName, true, store.StreamViewNewAndOldImages); err != nil {
		t.Fatal(err)
	}
	if err := st.TransactWriteItems(account, []store.TransactWriteAction{
		{
			Kind: "Put", TableName: table.TableName,
			ItemPK:   `{"S":"a"}`,
			ItemJSON: []byte(`{"pk":{"S":"a"},"v":{"S":"1"}}`),
		},
	}, ""); err != nil {
		t.Fatal(err)
	}
	if err := st.TransactWriteItems(account, []store.TransactWriteAction{
		{
			Kind: "Put", TableName: table.TableName,
			ItemPK:   `{"S":"a"}`,
			ItemJSON: []byte(`{"pk":{"S":"a"},"v":{"S":"2"}}`),
		},
	}, ""); err != nil {
		t.Fatal(err)
	}
	it, err := st.GetDynamoStreamShardIterator(account, table.TableName, store.LabDynamoStreamShardID, "TRIM_HORIZON", "")
	if err != nil {
		t.Fatal(err)
	}
	recs, _, err := st.GetDynamoStreamRecords(it, 10)
	if err != nil || len(recs) != 2 {
		t.Fatalf("recs=%v err=%v", recs, err)
	}
	if recs[0].EventName != "INSERT" || !bytes.Contains([]byte(recs[0].NewImageJSON), []byte(`"1"`)) {
		t.Fatalf("insert=%+v", recs[0])
	}
	if recs[1].EventName != "MODIFY" ||
		!bytes.Contains([]byte(recs[1].NewImageJSON), []byte(`"2"`)) ||
		!bytes.Contains([]byte(recs[1].OldImageJSON), []byte(`"1"`)) {
		t.Fatalf("modify=%+v", recs[1])
	}
}

func TestTransactWriteClientRequestTokenIdempotent(t *testing.T) {
	st := openDynamoStore(t)
	account := "000000000001"
	if _, err := st.CreateTable(account, "us-east-1", "TokOrders", "pk", store.KeyTypeString, "", "", "", "", nil); err != nil {
		t.Fatal(err)
	}
	actions := []store.TransactWriteAction{{
		Kind: "Put", TableName: "TokOrders",
		ItemPK:   `{"S":"a"}`,
		ItemJSON: []byte(`{"pk":{"S":"a"},"v":{"S":"1"}}`),
	}}
	if err := st.TransactWriteItems(account, actions, "tok-1"); err != nil {
		t.Fatal(err)
	}
	if err := st.TransactWriteItems(account, actions, "tok-1"); err != nil {
		t.Fatalf("replay same token: %v", err)
	}
	got, err := st.GetItemBytes(account, "TokOrders", `{"S":"a"}`, "")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got.ItemJSON, []byte(`"1"`)) {
		t.Fatalf("item=%s", got.ItemJSON)
	}
	mismatch := []store.TransactWriteAction{{
		Kind: "Put", TableName: "TokOrders",
		ItemPK:   `{"S":"a"}`,
		ItemJSON: []byte(`{"pk":{"S":"a"},"v":{"S":"other"}}`),
	}}
	err = st.TransactWriteItems(account, mismatch, "tok-1")
	if !errors.Is(err, store.ErrDynamoTransactIdempotentMismatch) {
		t.Fatalf("want IdempotentParameterMismatch, got %v", err)
	}
}

func TestTransactGetItemsReturnsBoth(t *testing.T) {
	st := openDynamoStore(t)
	account := "000000000001"
	if _, err := st.CreateTable(account, "us-east-1", "Orders", "pk", store.KeyTypeString, "", "", "", "", nil); err != nil {
		t.Fatal(err)
	}
	bodyA := []byte(`{"pk":{"S":"a"},"v":{"S":"1"}}`)
	bodyB := []byte(`{"pk":{"S":"b"},"v":{"S":"2"}}`)
	if err := st.PutItemBytes(account, "Orders", `{"S":"a"}`, "", "", "", bodyA, false, nil); err != nil {
		t.Fatal(err)
	}
	if err := st.PutItemBytes(account, "Orders", `{"S":"b"}`, "", "", "", bodyB, false, nil); err != nil {
		t.Fatal(err)
	}

	got, err := st.TransactGetItems(account, []store.TransactGetKey{
		{TableName: "Orders", ItemPK: `{"S":"a"}`},
		{TableName: "Orders", ItemPK: `{"S":"b"}`},
		{TableName: "Orders", ItemPK: `{"S":"missing"}`},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("len=%d", len(got))
	}
	if !bytes.Equal(got[0], bodyA) || !bytes.Equal(got[1], bodyB) {
		t.Fatalf("got=%q %q", got[0], got[1])
	}
	if got[2] != nil {
		t.Fatalf("missing item want nil, got %q", got[2])
	}
}
