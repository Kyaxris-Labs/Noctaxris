package store_test

import (
	"errors"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestDynamoDBTwoGSIs(t *testing.T) {
	st := openKMSStore(t)
	account := "000000000001"
	gsis := []store.DynamoGSI{
		{IndexName: "Gsi1", HashKeyName: "gsi1pk", HashKeyType: store.KeyTypeString},
		{IndexName: "Gsi2", HashKeyName: "gsi2pk", HashKeyType: store.KeyTypeString},
	}
	tbl, err := st.CreateTableWithGSIs(account, "us-east-1", "Multi", "pk", store.KeyTypeString, "", "", "", "", gsis)
	if err != nil {
		t.Fatal(err)
	}
	if !tbl.HasGSI() || !tbl.HasGSI2() {
		t.Fatalf("expected two GSIs: %+v", tbl)
	}
	if len(tbl.GSIs()) != 2 {
		t.Fatalf("GSIs=%d", len(tbl.GSIs()))
	}

	if err := st.PutItemBytesMultiGSI(account, "Multi", "a", "", "g1", "", "g2", "", []byte(`{"pk":{"S":"a"}}`), false, nil); err != nil {
		t.Fatal(err)
	}
	page1, err := st.QueryGSISlotItems(account, "Multi", 1, "g1", 0, "")
	if err != nil || len(page1.Items) != 1 {
		t.Fatalf("gsi1 page=%+v err=%v", page1, err)
	}
	page2, err := st.QueryGSISlotItems(account, "Multi", 2, "g2", 0, "")
	if err != nil || len(page2.Items) != 1 {
		t.Fatalf("gsi2 page=%+v err=%v", page2, err)
	}
}

func TestDynamoDBUpdateTableSecondGSI(t *testing.T) {
	st := openKMSStore(t)
	account := "000000000001"
	if _, err := st.CreateTable(account, "us-east-1", "T", "pk", store.KeyTypeString, "", "", "", "", nil); err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateTableGSI(account, "T", store.DynamoGSI{IndexName: "A", HashKeyName: "a", HashKeyType: store.KeyTypeString}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateTableGSI(account, "T", store.DynamoGSI{IndexName: "B", HashKeyName: "b", HashKeyType: store.KeyTypeString}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateTableGSI(account, "T", store.DynamoGSI{IndexName: "C", HashKeyName: "c", HashKeyType: store.KeyTypeString}); !errors.Is(err, store.ErrTooManyGSIs) {
		t.Fatalf("third GSI err=%v want ErrTooManyGSIs", err)
	}
}

func TestS3VersioningLite(t *testing.T) {
	st := openKMSStore(t)
	account := "000000000001"
	bucket := "ver-bucket"
	if _, err := st.CreateBucket(account, bucket); err != nil {
		t.Fatal(err)
	}
	if err := st.SetBucketVersioning(account, bucket, store.VersioningEnabled); err != nil {
		t.Fatal(err)
	}
	status, err := st.GetBucketVersioning(account, bucket)
	if err != nil || status != store.VersioningEnabled {
		t.Fatalf("status=%q err=%v", status, err)
	}
	v1, id1, err := st.PutObjectVersioned(account, bucket, "k.txt", store.PutObjectMeta{Data: []byte("one"), PlainSize: 3, ETag: "e1"})
	if err != nil || id1 == "" {
		t.Fatalf("v1=%+v id=%q err=%v", v1, id1, err)
	}
	_, id2, err := st.PutObjectVersioned(account, bucket, "k.txt", store.PutObjectMeta{Data: []byte("two"), PlainSize: 3, ETag: "e2"})
	if err != nil || id2 == "" || id2 == id1 {
		t.Fatalf("id2=%q id1=%q err=%v", id2, id1, err)
	}
	meta, vid, data, err := st.GetObjectVersion(account, bucket, "k.txt", "")
	if err != nil || vid != id2 || string(data) != "two" {
		t.Fatalf("latest meta=%+v vid=%q data=%q err=%v", meta, vid, data, err)
	}
	_, _, old, err := st.GetObjectVersion(account, bucket, "k.txt", id1)
	if err != nil || string(old) != "one" {
		t.Fatalf("old=%q err=%v", old, err)
	}
	list, err := st.ListObjectVersions(account, bucket, "k")
	if err != nil || len(list) != 2 {
		t.Fatalf("list=%+v err=%v", list, err)
	}
}
