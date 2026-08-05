package dynamodb_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	ddb "github.com/Kyaxris-Labs/Noctaxris/internal/services/dynamodb"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func richTable() store.DynamoTable {
	return store.DynamoTable{
		AccountID:        "000000000001",
		TableName:        "Music",
		TableARN:         "arn:aws:dynamodb:us-east-1:000000000001:table/Music",
		Status:           store.TableStatusActive,
		HashKeyName:      "Artist",
		HashKeyType:      store.KeyTypeString,
		RangeKeyName:     "SongTitle",
		RangeKeyType:     store.KeyTypeString,
		SSEType:          store.SSETypeKMS,
		KMSKeyID:         "arn:aws:kms:us-east-1:000000000001:key/sym",
		CreationDate:     "2024-06-01T12:00:00Z",
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
		TTLAttributeName: "expires",
		TTLEnabled:       true,
		StreamEnabled:    true,
		StreamViewType:   "NEW_AND_OLD_IMAGES",
		StreamLabel:      "2024-06-01T12:00:00.000",
	}
}

func TestDynamoDBJSONFormatters(t *testing.T) {
	tbl := richTable()
	createRaw, err := ddb.CreateTableJSON(tbl)
	if err != nil {
		t.Fatal(err)
	}
	descRaw, err := ddb.DescribeTableJSON(tbl)
	if err != nil {
		t.Fatal(err)
	}
	listRaw, err := ddb.ListTablesJSON([]store.DynamoTable{tbl})
	if err != nil {
		t.Fatal(err)
	}
	empty, err := ddb.EmptyOKJSON()
	if err != nil || string(empty) != `{}` {
		t.Fatal(err)
	}
	item := ddb.ItemMap{
		"Artist":    map[string]any{"S": "Radiohead"},
		"SongTitle": map[string]any{"S": "Creep"},
	}
	getFound, err := ddb.GetItemJSON(item, true)
	if err != nil {
		t.Fatal(err)
	}
	getMiss, err := ddb.GetItemJSON(nil, false)
	if err != nil {
		t.Fatal(err)
	}
	putRaw, err := ddb.PutItemJSON(item)
	if err != nil {
		t.Fatal(err)
	}
	putNil, err := ddb.PutItemJSON(nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(putNil) != `{}` {
		t.Fatalf("put nil=%s", putNil)
	}
	delRaw, err := ddb.DeleteItemJSON()
	if err != nil {
		t.Fatal(err)
	}
	updRaw, err := ddb.UpdateItemJSON(item)
	if err != nil {
		t.Fatal(err)
	}
	lastKey := ddb.ItemMap{"Artist": map[string]any{"S": "Radiohead"}}
	queryRaw, err := ddb.QueryJSON([]ddb.ItemMap{item}, lastKey, true)
	if err != nil {
		t.Fatal(err)
	}
	scanRaw, err := ddb.ScanJSON([]ddb.ItemMap{item}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	batchGet, err := ddb.BatchGetItemJSON(map[string][]ddb.ItemMap{"Music": {item}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	batchWrite, err := ddb.BatchWriteItemJSON(nil)
	if err != nil {
		t.Fatal(err)
	}
	txWrite, err := ddb.TransactWriteItemsJSON()
	if err != nil {
		t.Fatal(err)
	}
	txGet, err := ddb.TransactGetItemsJSON([]ddb.ItemMap{item, nil})
	if err != nil {
		t.Fatal(err)
	}
	txCancel, err := ddb.TransactionCanceledJSON("", []store.CancellationReason{{Code: "ConditionalCheckFailed", Message: "m"}})
	if err != nil {
		t.Fatal(err)
	}
	pol, err := ddb.GetResourcePolicyJSON(`{"Version":"2012-10-17"}`)
	if err != nil {
		t.Fatal(err)
	}
	ttlDesc, err := ddb.DescribeTimeToLiveJSON(tbl)
	if err != nil {
		t.Fatal(err)
	}
	ttlUpd, err := ddb.UpdateTimeToLiveJSON(tbl)
	if err != nil {
		t.Fatal(err)
	}
	cont, err := ddb.DescribeContinuousBackupsJSON()
	if err != nil {
		t.Fatal(err)
	}
	tags, err := ddb.ListTagsOfResourceJSON([]store.ResourceTag{{Key: "k", Value: "v"}})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(createRaw), "GlobalSecondaryIndexes") ||
		!strings.Contains(string(descRaw), "LocalSecondaryIndexes") ||
		!strings.Contains(string(listRaw), "Music") {
		t.Fatalf("table json create=%s desc=%s", createRaw, descRaw)
	}
	if !strings.Contains(string(getFound), "Item") || strings.Contains(string(getMiss), "Item") {
		t.Fatalf("get found=%s miss=%s", getFound, getMiss)
	}
	if !strings.Contains(string(putRaw), "Attributes") || !strings.Contains(string(delRaw), `{}`) {
		t.Fatalf("put=%s del=%s", putRaw, delRaw)
	}
	if !strings.Contains(string(updRaw), "Attributes") || !strings.Contains(string(queryRaw), "LastEvaluatedKey") {
		t.Fatalf("upd=%s query=%s", updRaw, queryRaw)
	}
	if !strings.Contains(string(scanRaw), "Items") || !strings.Contains(string(batchGet), "Responses") {
		t.Fatalf("scan=%s batch=%s", scanRaw, batchGet)
	}
	if !strings.Contains(string(batchWrite), "UnprocessedItems") || string(txWrite) != `{}` {
		t.Fatalf("batchWrite=%s txWrite=%s", batchWrite, txWrite)
	}
	if !strings.Contains(string(txGet), "Responses") || !strings.Contains(string(txCancel), "CancellationReasons") {
		t.Fatalf("txGet=%s cancel=%s", txGet, txCancel)
	}
	if !strings.Contains(string(pol), "Policy") || !strings.Contains(string(ttlDesc), "ENABLED") {
		t.Fatalf("pol=%s ttl=%s", pol, ttlDesc)
	}
	if string(ttlDesc) != string(ttlUpd) || !strings.Contains(string(cont), "ContinuousBackupsStatus") {
		t.Fatalf("ttl upd=%s cont=%s", ttlUpd, cont)
	}
	if !strings.Contains(string(tags), "Tags") {
		t.Fatalf("tags=%s", tags)
	}

	// Hash-only table branch (no range/GSI).
	simple := store.DynamoTable{
		TableName:    "Simple",
		HashKeyName:  "Id",
		HashKeyType:  store.KeyTypeString,
		CreationDate: "bad-date",
	}
	simpleRaw, err := ddb.DescribeTableJSON(simple)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(simpleRaw), "Simple") {
		t.Fatalf("simple=%s", simpleRaw)
	}
}

func TestDynamoDBAttrsAndUpdate(t *testing.T) {
	table := store.DynamoTable{
		HashKeyName:  "Artist",
		HashKeyType:  store.KeyTypeString,
		RangeKeyName: "SongTitle",
		RangeKeyType: store.KeyTypeString,
		GSIName:      "GenreIndex",
		GSIHashKeyName: "Genre",
		GSIHashKeyType: store.KeyTypeString,
		GSIRangeKeyName: "Year",
		GSIRangeKeyType: store.KeyTypeNumber,
		LSIName:        "ByStatus",
		LSIRangeKeyName: "Status",
		LSIRangeKeyType: store.KeyTypeString,
		TTLAttributeName: "expires",
		TTLEnabled:       true,
	}
	rawItem := map[string]any{
		"Artist":    map[string]any{"S": "Radiohead"},
		"SongTitle": map[string]any{"S": "Creep"},
		"Genre":     map[string]any{"S": "Rock"},
		"Year":      map[string]any{"N": "1993"},
		"Status":    map[string]any{"S": "published"},
	}
	item, err := ddb.ParseItemMap(rawItem)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ddb.ParseItemMap(nil); err == nil {
		t.Fatal("expected nil item error")
	}
	if _, err := ddb.ParseItemMap(map[string]any{"a": 1}); err == nil {
		t.Fatal("expected bad av error")
	}

	marshaled, err := ddb.MarshalItemJSON(item)
	if err != nil {
		t.Fatal(err)
	}
	round, err := ddb.UnmarshalItemJSON(marshaled)
	if err != nil {
		t.Fatal(err)
	}
	emptyRound, err := ddb.UnmarshalItemJSON(nil)
	if err != nil || len(emptyRound) != 0 {
		t.Fatalf("empty round err=%v len=%d", err, len(emptyRound))
	}
	canonical, err := ddb.CanonicalAV(item["Artist"])
	if err != nil || canonical == "" {
		t.Fatalf("canonical=%q err=%v", canonical, err)
	}
	if _, err := ddb.CanonicalAV(map[string]any{}); err == nil {
		t.Fatal("expected empty av error")
	}

	pk, sk, err := ddb.PrimaryKeyStrings(table, item)
	if err != nil || pk == "" || sk == "" {
		t.Fatalf("pk=%q sk=%q err=%v", pk, sk, err)
	}
	keyFromCanon, err := ddb.KeyFromCanonical(table, pk, sk)
	if err != nil {
		t.Fatal(err)
	}
	if keyFromCanon["Artist"]["S"] != "Radiohead" {
		t.Fatalf("key=%v", keyFromCanon)
	}
	gpk, gsk, err := ddb.GSIKeyStrings(table, item)
	if err != nil || gpk == "" {
		t.Fatalf("gsi pk=%q sk=%q err=%v", gpk, gsk, err)
	}
	lsiSK, err := ddb.LSIKeyStrings(table, item)
	if err != nil || lsiSK == "" {
		t.Fatalf("lsi sk=%q err=%v", lsiSK, err)
	}

	exp := time.Now().Unix() - 10
	expiredItem := ddb.ItemMap{
		"expires": map[string]any{"N": fmt.Sprintf("%d", exp)},
	}
	if !ddb.ItemExpired(table, expiredItem, time.Now().Unix()) {
		t.Fatal("expected expired")
	}
	if ddb.ItemExpired(store.DynamoTable{}, item, time.Now().Unix()) {
		t.Fatal("ttl disabled should not expire")
	}

	hashOnly := map[string]any{"Artist": map[string]any{"S": "A"}}
	hk, err := ddb.HashKeyFromQuery(table, map[string]any{"KeyConditionExpression": "#a = :a", "ExpressionAttributeNames": map[string]any{"#a": "Artist"}, "ExpressionAttributeValues": map[string]any{":a": map[string]any{"S": "A"}}})
	if err != nil {
		t.Fatal(err)
	}
	if hk["S"] != "A" {
		t.Fatalf("hk=%v", hk)
	}
	_, err = ddb.HashKeyFromQueryIndex(table, "GenreIndex", map[string]any{"KeyConditionExpression": "Genre = :g", "ExpressionAttributeValues": map[string]any{":g": map[string]any{"S": "Rock"}}})
	if err != nil {
		t.Fatal(err)
	}

	key := ddb.ItemMap{
		"Artist":    item["Artist"],
		"SongTitle": item["SongTitle"],
	}
	updated, err := ddb.ApplyUpdateExpression(item, key, "SET #s = :s REMOVE Genre", map[string]any{"#s": "Status"}, map[string]any{":s": map[string]any{"S": "draft"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := updated["Genre"]; ok {
		t.Fatalf("genre should be removed: %v", updated)
	}
	if updated["Status"]["S"] != "draft" {
		t.Fatalf("status=%v", updated["Status"])
	}
	if _, err := ddb.ApplyUpdateExpression(item, key, "BAD", nil, nil); err == nil {
		t.Fatal("expected bad update expr")
	}

	_ = round
	_ = hashOnly
}

func TestDynamoDBCryptoRoundTrip(t *testing.T) {
	cmk := bytes.Repeat([]byte{0x42}, 32)
	keyID := "ddb-key"
	table := richTable()
	plain := []byte(`{"Artist":{"S":"A"}}`)
	ctx := ddb.EncryptionContext(table)
	if ctx["aws:dynamodb:tableName"] != "Music" {
		t.Fatalf("ctx=%v", ctx)
	}

	sealed, err := ddb.SealItemJSON(cmk, keyID, plain, ctx)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := ddb.OpenItemJSON(cmk, sealed, ctx)
	if err != nil || !bytes.Equal(opened, plain) {
		t.Fatalf("open=%q err=%v", opened, err)
	}
	openedAny, err := ddb.OpenItemJSONAny([][]byte{cmk, bytes.Repeat([]byte{1}, 32)}, sealed, ctx)
	if err != nil || !bytes.Equal(openedAny, plain) {
		t.Fatalf("openAny=%q err=%v", openedAny, err)
	}

	dek := bytes.Repeat([]byte{0x33}, 32)
	ct, err := ddb.EncryptAES256GCM(dek, plain)
	if err != nil {
		t.Fatal(err)
	}
	pt, err := ddb.DecryptAES256GCM(dek, ct)
	if err != nil || !bytes.Equal(pt, plain) {
		t.Fatalf("gcm=%q err=%v", pt, err)
	}
	if _, err := ddb.EncryptAES256GCM([]byte{1}, plain); err == nil {
		t.Fatal("expected dek length error")
	}
	if _, err := ddb.DecryptAES256GCM(dek, []byte{1}); err == nil {
		t.Fatal("expected short ciphertext error")
	}

	data, sealedFlag, sealedDEK, err := ddb.StoragePayload(table, plain, cmk, keyID)
	if err != nil || !sealedFlag || len(sealedDEK) == 0 {
		t.Fatalf("storage sealed=%v dek=%d err=%v", sealedFlag, len(sealedDEK), err)
	}
	owned := table
	owned.SSEType = store.SSETypeAWSOwned
	dataOwned, sealedOwned, _, err := ddb.StoragePayload(owned, plain, nil, "")
	if err != nil || sealedOwned || !bytes.Equal(dataOwned, plain) {
		t.Fatalf("owned data=%q sealed=%v err=%v", dataOwned, sealedOwned, err)
	}
	if _, _, _, err := ddb.StoragePayload(table, plain, nil, ""); err == nil {
		t.Fatal("expected kms material required")
	}

	stored := store.DynamoStoredItem{ItemJSON: data, Sealed: true, SealedDEK: sealedDEK}
	loaded, err := ddb.LoadItemJSON(table, stored, cmk)
	if err != nil || !bytes.Equal(loaded, plain) {
		t.Fatalf("load=%q err=%v", loaded, err)
	}
	loadedAny, err := ddb.LoadItemJSONAny(table, stored, [][]byte{cmk})
	if err != nil || !bytes.Equal(loadedAny, plain) {
		t.Fatalf("loadAny=%q err=%v", loadedAny, err)
	}
	unsealed := store.DynamoStoredItem{ItemJSON: plain, Sealed: false}
	plainLoad, err := ddb.LoadItemJSON(owned, unsealed, nil)
	if err != nil || !bytes.Equal(plainLoad, plain) {
		t.Fatalf("plain load=%q err=%v", plainLoad, err)
	}
	sealedRow := store.DynamoStoredItem{ItemJSON: []byte{1}, Sealed: true}
	if _, err := ddb.LoadItemJSONAny(owned, sealedRow, nil); err == nil {
		t.Fatal("expected sealed item on owned table error")
	}
}
