package store_test

import (
	"bytes"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func openDynamoStore(t *testing.T) *store.Store {
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

func TestCreateTableDescribeList(t *testing.T) {
	st := openDynamoStore(t)
	account := "000000000001"

	tbl, err := st.CreateTable(account, "us-east-1", "Music", "Artist", store.KeyTypeString, "SongTitle", store.KeyTypeString, "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if tbl.Status != store.TableStatusActive || tbl.SSEType != store.SSETypeAWSOwned {
		t.Fatalf("table=%+v", tbl)
	}
	if tbl.TableARN != "arn:aws:dynamodb:us-east-1:000000000001:table/Music" {
		t.Fatalf("arn=%q", tbl.TableARN)
	}
	if !tbl.HasRangeKey() {
		t.Fatalf("expected composite key table")
	}

	if _, err := st.CreateTable(account, "us-east-1", "Music", "Artist", store.KeyTypeString, "", "", "", "", nil); !errors.Is(err, store.ErrTableAlreadyExists) {
		t.Fatalf("want ErrTableAlreadyExists, got %v", err)
	}

	got, err := st.DescribeTable(account, "Music")
	if err != nil {
		t.Fatal(err)
	}
	if got.HashKeyName != "Artist" || got.RangeKeyName != "SongTitle" {
		t.Fatalf("describe=%+v", got)
	}

	if _, err := st.GetTable(account, "Missing"); !errors.Is(err, store.ErrNoSuchTable) {
		t.Fatalf("want ErrNoSuchTable, got %v", err)
	}

	tables, err := st.ListTables(account)
	if err != nil {
		t.Fatal(err)
	}
	if len(tables) != 1 || tables[0].TableName != "Music" {
		t.Fatalf("tables=%+v", tables)
	}
}

func TestPutGetDeleteItem(t *testing.T) {
	st := openDynamoStore(t)
	account := "000000000001"
	if _, err := st.CreateTable(account, "us-east-1", "Users", "Id", store.KeyTypeString, "", "", "", "", nil); err != nil {
		t.Fatal(err)
	}

	body := []byte(`{"Id":{"S":"u1"},"Name":{"S":"Ada"}}`)
	if err := st.PutItemBytes(account, "Users", `{"S":"u1"}`, "", "", "", body, false, nil); err != nil {
		t.Fatal(err)
	}

	got, err := st.GetItemBytes(account, "Users", `{"S":"u1"}`, "")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.ItemJSON, body) || got.Sealed {
		t.Fatalf("item=%+v", got)
	}

	if _, err := st.GetItemBytes(account, "Users", `{"S":"missing"}`, ""); !errors.Is(err, store.ErrNoSuchItem) {
		t.Fatalf("want ErrNoSuchItem, got %v", err)
	}

	if err := st.DeleteItem(account, "Users", `{"S":"u1"}`, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetItemBytes(account, "Users", `{"S":"u1"}`, ""); !errors.Is(err, store.ErrNoSuchItem) {
		t.Fatalf("want ErrNoSuchItem after delete, got %v", err)
	}
}

func TestPutItemSealed(t *testing.T) {
	st := openDynamoStore(t)
	account := "000000000001"
	if _, err := st.CreateTable(account, "us-east-1", "Secrets", "Id", store.KeyTypeString, "", "", store.SSETypeKMS, "key-1", nil); err != nil {
		t.Fatal(err)
	}
	cipher := []byte{0x01, 0x02, 0x03}
	dek := []byte{0xaa, 0xbb}
	if err := st.PutItemBytes(account, "Secrets", `{"S":"s1"}`, "", "", "", cipher, true, dek); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetItemBytes(account, "Secrets", `{"S":"s1"}`, "")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Sealed || !bytes.Equal(got.ItemJSON, cipher) || !bytes.Equal(got.SealedDEK, dek) {
		t.Fatalf("sealed item=%+v", got)
	}
}

func TestPutItemUnknownTable(t *testing.T) {
	st := openDynamoStore(t)
	if err := st.PutItemBytes("000000000001", "Ghost", `{"S":"x"}`, "", "", "", []byte("{}"), false, nil); !errors.Is(err, store.ErrNoSuchTable) {
		t.Fatalf("want ErrNoSuchTable, got %v", err)
	}
}

func TestQueryItemsByHash(t *testing.T) {
	st := openDynamoStore(t)
	account := "000000000001"
	if _, err := st.CreateTable(account, "us-east-1", "Music", "Artist", store.KeyTypeString, "SongTitle", store.KeyTypeString, "", "", nil); err != nil {
		t.Fatal(err)
	}
	pk := `{"S":"Radiohead"}`
	songs := []string{`{"S":"Creep"}`, `{"S":"Karma Police"}`, `{"S":"No Surprises"}`}
	for _, sk := range songs {
		if err := st.PutItemBytes(account, "Music", pk, sk, "", "", []byte(`{}`), false, nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.PutItemBytes(account, "Music", `{"S":"Other"}`, `{"S":"Song"}`, "", "", []byte(`{}`), false, nil); err != nil {
		t.Fatal(err)
	}

	page, err := st.QueryItems(account, "Music", pk, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 3 {
		t.Fatalf("want 3 items, got %d", len(page.Items))
	}
	if page.Items[0].ItemSK != `{"S":"Creep"}` {
		t.Fatalf("first sk=%q", page.Items[0].ItemSK)
	}

	first, err := st.QueryItems(account, "Music", pk, 2, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 2 || !first.HasMore {
		t.Fatalf("page1=%+v", first)
	}
	second, err := st.QueryItems(account, "Music", pk, 2, first.LastSK)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 1 || second.HasMore {
		t.Fatalf("page2=%+v", second)
	}
}

func TestScanItems(t *testing.T) {
	st := openDynamoStore(t)
	account := "000000000001"
	if _, err := st.CreateTable(account, "us-east-1", "Music", "Artist", store.KeyTypeString, "SongTitle", store.KeyTypeString, "", "", nil); err != nil {
		t.Fatal(err)
	}
	rows := [][2]string{
		{`{"S":"A"}`, `{"S":"1"}`},
		{`{"S":"A"}`, `{"S":"2"}`},
		{`{"S":"B"}`, `{"S":"1"}`},
	}
	for _, r := range rows {
		if err := st.PutItemBytes(account, "Music", r[0], r[1], "", "", []byte(`{}`), false, nil); err != nil {
			t.Fatal(err)
		}
	}
	full, err := st.ScanItems(account, "Music", 0, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(full.Items) != 3 {
		t.Fatalf("want 3, got %d", len(full.Items))
	}
	page1, err := st.ScanItems(account, "Music", 2, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page1.Items) != 2 || !page1.HasMore {
		t.Fatalf("page1=%+v", page1)
	}
	page2, err := st.ScanItems(account, "Music", 2, page1.LastPK, page1.LastSK)
	if err != nil {
		t.Fatal(err)
	}
	if len(page2.Items) != 1 || page2.HasMore {
		t.Fatalf("page2=%+v", page2)
	}
}

func TestResourcePolicyLifecycle(t *testing.T) {
	st := openDynamoStore(t)
	account := "000000000001"
	if _, err := st.CreateTable(account, "us-east-1", "Music", "Artist", store.KeyTypeString, "", "", "", "", nil); err != nil {
		t.Fatal(err)
	}

	if _, err := st.GetResourcePolicy(account, "Music"); !errors.Is(err, store.ErrNoSuchResourcePolicy) {
		t.Fatalf("want ErrNoSuchResourcePolicy, got %v", err)
	}

	policy := `{"Version":"2012-10-17","Statement":[]}`
	if err := st.PutResourcePolicy(account, "Music", policy); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetResourcePolicy(account, "Music")
	if err != nil {
		t.Fatal(err)
	}
	if got != policy {
		t.Fatalf("policy=%q", got)
	}

	if err := st.DeleteResourcePolicy(account, "Music"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetResourcePolicy(account, "Music"); !errors.Is(err, store.ErrNoSuchResourcePolicy) {
		t.Fatalf("want ErrNoSuchResourcePolicy after delete, got %v", err)
	}
}

func TestUpdateTableSSE(t *testing.T) {
	st := openDynamoStore(t)
	account := "000000000001"
	if _, err := st.CreateTable(account, "us-east-1", "Music", "Artist", store.KeyTypeString, "", "", "", "", nil); err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateTableSSE(account, "Music", store.SSETypeKMS, "key-9"); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetTable(account, "Music")
	if err != nil {
		t.Fatal(err)
	}
	if got.SSEType != store.SSETypeKMS || got.KMSKeyID != "key-9" {
		t.Fatalf("table=%+v", got)
	}
}

func TestDeleteTableRejectsNonEmpty(t *testing.T) {
	st := openDynamoStore(t)
	account := "000000000001"
	if _, err := st.CreateTable(account, "us-east-1", "Music", "Artist", store.KeyTypeString, "", "", "", "", nil); err != nil {
		t.Fatal(err)
	}
	if err := st.PutItemBytes(account, "Music", `{"S":"x"}`, "", "", "", []byte(`{}`), false, nil); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteTable(account, "Music"); !errors.Is(err, store.ErrTableNotEmpty) {
		t.Fatalf("want ErrTableNotEmpty, got %v", err)
	}

	if err := st.DeleteItem(account, "Music", `{"S":"x"}`, ""); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteTable(account, "Music"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetTable(account, "Music"); !errors.Is(err, store.ErrNoSuchTable) {
		t.Fatalf("want ErrNoSuchTable, got %v", err)
	}
}

func TestCreateTableInvalidKeyType(t *testing.T) {
	st := openDynamoStore(t)
	if _, err := st.CreateTable("000000000001", "us-east-1", "Bad", "Id", "X", "", "", "", "", nil); !errors.Is(err, store.ErrInvalidKeyType) {
		t.Fatalf("want ErrInvalidKeyType, got %v", err)
	}
}

func TestCreateTableWithGSI(t *testing.T) {
	st := openDynamoStore(t)
	account := "000000000001"
	gsi := &store.DynamoGSI{
		IndexName:   "GenreIndex",
		HashKeyName: "Genre",
		HashKeyType: store.KeyTypeString,
	}
	tbl, err := st.CreateTable(account, "us-east-1", "Music", "Artist", store.KeyTypeString, "SongTitle", store.KeyTypeString, "", "", gsi)
	if err != nil {
		t.Fatal(err)
	}
	if !tbl.HasGSI() || tbl.GSIName != "GenreIndex" || tbl.GSIHashKeyName != "Genre" {
		t.Fatalf("table=%+v", tbl)
	}
}

func TestQueryGSIItems(t *testing.T) {
	st := openDynamoStore(t)
	account := "000000000001"
	gsi := &store.DynamoGSI{
		IndexName:    "GenreIndex",
		HashKeyName:  "Genre",
		HashKeyType:  store.KeyTypeString,
		RangeKeyName: "Year",
		RangeKeyType: store.KeyTypeNumber,
	}
	if _, err := st.CreateTable(account, "us-east-1", "Music", "Artist", store.KeyTypeString, "SongTitle", store.KeyTypeString, "", "", gsi); err != nil {
		t.Fatal(err)
	}
	pk := `{"S":"Radiohead"}`
	rows := []struct {
		sk, gsiPK, gsiSK string
	}{
		{`{"S":"Creep"}`, `{"S":"Rock"}`, `{"N":"1993"}`},
		{`{"S":"Karma Police"}`, `{"S":"Rock"}`, `{"N":"1997"}`},
		{`{"S":"Jazz Song"}`, `{"S":"Jazz"}`, `{"N":"2001"}`},
	}
	for _, r := range rows {
		if err := st.PutItemBytes(account, "Music", pk, r.sk, r.gsiPK, r.gsiSK, []byte(`{}`), false, nil); err != nil {
			t.Fatal(err)
		}
	}

	rockPK := `{"S":"Rock"}`
	page, err := st.QueryGSIItems(account, "Music", rockPK, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("want 2 rock items, got %d", len(page.Items))
	}
	if page.Items[0].GSISK != `{"N":"1993"}` {
		t.Fatalf("first gsi_sk=%q", page.Items[0].GSISK)
	}
}

func TestUpdateTableGSI(t *testing.T) {
	st := openDynamoStore(t)
	account := "000000000001"
	if _, err := st.CreateTable(account, "us-east-1", "Music", "Artist", store.KeyTypeString, "", "", "", "", nil); err != nil {
		t.Fatal(err)
	}
	gsi := store.DynamoGSI{IndexName: "GenreIndex", HashKeyName: "Genre", HashKeyType: store.KeyTypeString}
	if err := st.UpdateTableGSI(account, "Music", gsi); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetTable(account, "Music")
	if err != nil {
		t.Fatal(err)
	}
	if !got.HasGSI() {
		t.Fatalf("table=%+v", got)
	}
	if err := st.UpdateTableGSI(account, "Music", gsi); !errors.Is(err, store.ErrGSIAlreadyExists) {
		t.Fatalf("want ErrGSIAlreadyExists, got %v", err)
	}
}

func TestUpdateTimeToLive(t *testing.T) {
	st := openDynamoStore(t)
	account := "000000000001"
	if _, err := st.CreateTable(account, "us-east-1", "Sessions", "Id", store.KeyTypeString, "", "", "", "", nil); err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateTimeToLive(account, "Sessions", "expires", true); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetTable(account, "Sessions")
	if err != nil {
		t.Fatal(err)
	}
	if !got.TTLEnabled || got.TTLAttributeName != "expires" {
		t.Fatalf("table=%+v", got)
	}
	if err := st.UpdateTimeToLive(account, "Sessions", "ttl", false); err != nil {
		t.Fatal(err)
	}
	got, err = st.GetTable(account, "Sessions")
	if err != nil {
		t.Fatal(err)
	}
	if got.TTLEnabled || got.TTLAttributeName != "ttl" {
		t.Fatalf("table=%+v", got)
	}
}
