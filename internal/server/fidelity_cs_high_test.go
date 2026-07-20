package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestGetSessionTokenRejectsTemporaryCredentials(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	roleARN := "arn:aws:iam::" + testAccountID + ":role/gst-deny"
	trust := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"*"},"Action":"sts:AssumeRole"}]}`
	if _, err := st.CreateRole(testAccountID, "gst-deny", trust); err != nil {
		t.Fatal(err)
	}

	assumeBody := []byte("Action=AssumeRole&Version=2011-06-15&RoleArn=" + roleARN + "&RoleSessionName=lab")
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", assumeBody)
	signHeader(t, req, assumeBody, testAccessKey, testSecret, testRegion, "sts", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("AssumeRole status=%d body=%q", rec.Code, rec.Body.String())
	}
	tempAKID := xmlTag(t, rec.Body.String(), "AccessKeyId")
	tempSecret := xmlTag(t, rec.Body.String(), "SecretAccessKey")
	tempToken := xmlTag(t, rec.Body.String(), "SessionToken")

	gstBody := []byte("Action=GetSessionToken&Version=2011-06-15")
	req = mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", gstBody)
	signHeader(t, req, gstBody, tempAKID, tempSecret, testRegion, "sts", now)
	req.Header.Set("X-Amz-Security-Token", tempToken)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "AccessDenied") {
		t.Fatalf("GetSessionToken with temp creds: status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestGetSessionTokenSessionCannotCallIAMWithoutMFA(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	gstBody := []byte("Action=GetSessionToken&Version=2011-06-15")
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", gstBody)
	signHeader(t, req, gstBody, testAccessKey, testSecret, testRegion, "sts", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GetSessionToken status=%d body=%q", rec.Code, rec.Body.String())
	}
	akid := xmlTag(t, rec.Body.String(), "AccessKeyId")
	secret := xmlTag(t, rec.Body.String(), "SecretAccessKey")
	token := xmlTag(t, rec.Body.String(), "SessionToken")

	listBody := []byte("Action=ListUsers&Version=2010-05-08")
	req = mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", listBody)
	signHeader(t, req, listBody, akid, secret, testRegion, "iam", now)
	req.Header.Set("X-Amz-Security-Token", token)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "AccessDenied") {
		t.Fatalf("ListUsers without MFA session: status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestDynamoDBBatchGetUnprocessedKeys(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustDynamoJSON(t, handler, "CreateTable", map[string]any{
		"TableName": "batch-overflow",
		"AttributeDefinitions": []map[string]any{
			{"AttributeName": "pk", "AttributeType": "S"},
		},
		"KeySchema": []map[string]any{
			{"AttributeName": "pk", "KeyType": "HASH"},
		},
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateTable status=%d body=%q", createRec.Code, createRec.Body.String())
	}

	keys := make([]any, 0, 30)
	for i := 0; i < 30; i++ {
		pk := fmt.Sprintf("k-%02d", i)
		putRec := mustDynamoJSON(t, handler, "PutItem", map[string]any{
			"TableName": "batch-overflow",
			"Item":      map[string]any{"pk": map[string]any{"S": pk}},
		}, now)
		if putRec.Code != http.StatusOK {
			t.Fatalf("PutItem %d status=%d", i, putRec.Code)
		}
		keys = append(keys, map[string]any{"pk": map[string]any{"S": pk}})
	}

	batchRec := mustDynamoJSON(t, handler, "BatchGetItem", map[string]any{
		"RequestItems": map[string]any{
			"batch-overflow": map[string]any{"Keys": keys},
		},
	}, now)
	if batchRec.Code != http.StatusOK {
		t.Fatalf("BatchGetItem status=%d body=%q", batchRec.Code, batchRec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(batchRec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	resp, _ := out["Responses"].(map[string]any)
	items, _ := resp["batch-overflow"].([]any)
	if len(items) != 25 {
		t.Fatalf("Responses len=%d want 25", len(items))
	}
	unproc, _ := out["UnprocessedKeys"].(map[string]any)
	entry, _ := unproc["batch-overflow"].(map[string]any)
	left, _ := entry["Keys"].([]any)
	if len(left) != 5 {
		t.Fatalf("UnprocessedKeys len=%d want 5 body=%q", len(left), batchRec.Body.String())
	}
}

func TestDynamoDBQuerySortKeyBeginsWith(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustDynamoJSON(t, handler, "CreateTable", map[string]any{
		"TableName": "sk-music",
		"AttributeDefinitions": []map[string]any{
			{"AttributeName": "Artist", "AttributeType": "S"},
			{"AttributeName": "SongTitle", "AttributeType": "S"},
		},
		"KeySchema": []map[string]any{
			{"AttributeName": "Artist", "KeyType": "HASH"},
			{"AttributeName": "SongTitle", "KeyType": "RANGE"},
		},
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateTable status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	for _, title := range []string{"Creep", "Karma Police", "No Surprises"} {
		putRec := mustDynamoJSON(t, handler, "PutItem", map[string]any{
			"TableName": "sk-music",
			"Item": map[string]any{
				"Artist":    map[string]any{"S": "Radiohead"},
				"SongTitle": map[string]any{"S": title},
			},
		}, now)
		if putRec.Code != http.StatusOK {
			t.Fatalf("PutItem %s status=%d", title, putRec.Code)
		}
	}

	queryRec := mustDynamoJSON(t, handler, "Query", map[string]any{
		"TableName":              "sk-music",
		"KeyConditionExpression": "Artist = :a AND begins_with(SongTitle, :p)",
		"ExpressionAttributeValues": map[string]any{
			":a": map[string]any{"S": "Radiohead"},
			":p": map[string]any{"S": "K"},
		},
	}, now)
	if queryRec.Code != http.StatusOK {
		t.Fatalf("Query status=%d body=%q", queryRec.Code, queryRec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(queryRec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	items, _ := out["Items"].([]any)
	if len(items) != 1 {
		t.Fatalf("want 1 item, got %d body=%q", len(items), queryRec.Body.String())
	}
}

func TestDynamoDBGSI2PaginationUsesSlot2Keys(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustDynamoJSON(t, handler, "CreateTable", map[string]any{
		"TableName": "gsi2-page",
		"AttributeDefinitions": []map[string]any{
			{"AttributeName": "pk", "AttributeType": "S"},
			{"AttributeName": "sk", "AttributeType": "S"},
			{"AttributeName": "g1", "AttributeType": "S"},
			{"AttributeName": "g2", "AttributeType": "S"},
			{"AttributeName": "g2sk", "AttributeType": "S"},
		},
		"KeySchema": []map[string]any{
			{"AttributeName": "pk", "KeyType": "HASH"},
			{"AttributeName": "sk", "KeyType": "RANGE"},
		},
		"GlobalSecondaryIndexes": []map[string]any{
			{
				"IndexName": "Gsi1",
				"KeySchema": []map[string]any{
					{"AttributeName": "g1", "KeyType": "HASH"},
				},
				"Projection": map[string]any{"ProjectionType": "ALL"},
			},
			{
				"IndexName": "Gsi2",
				"KeySchema": []map[string]any{
					{"AttributeName": "g2", "KeyType": "HASH"},
					{"AttributeName": "g2sk", "KeyType": "RANGE"},
				},
				"Projection": map[string]any{"ProjectionType": "ALL"},
			},
		},
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateTable status=%d body=%q", createRec.Code, createRec.Body.String())
	}

	for i, g2sk := range []string{"a", "b", "c"} {
		putRec := mustDynamoJSON(t, handler, "PutItem", map[string]any{
			"TableName": "gsi2-page",
			"Item": map[string]any{
				"pk":   map[string]any{"S": fmt.Sprintf("p%d", i)},
				"sk":   map[string]any{"S": fmt.Sprintf("s%d", i)},
				"g1":   map[string]any{"S": "other"},
				"g2":   map[string]any{"S": "group"},
				"g2sk": map[string]any{"S": g2sk},
			},
		}, now)
		if putRec.Code != http.StatusOK {
			t.Fatalf("PutItem %d status=%d body=%q", i, putRec.Code, putRec.Body.String())
		}
	}

	first := mustDynamoJSON(t, handler, "Query", map[string]any{
		"TableName":              "gsi2-page",
		"IndexName":              "Gsi2",
		"KeyConditionExpression": "g2 = :g",
		"ExpressionAttributeValues": map[string]any{
			":g": map[string]any{"S": "group"},
		},
		"Limit": 2,
	}, now)
	if first.Code != http.StatusOK {
		t.Fatalf("Query page1 status=%d body=%q", first.Code, first.Body.String())
	}
	var page1 map[string]any
	if err := json.Unmarshal(first.Body.Bytes(), &page1); err != nil {
		t.Fatal(err)
	}
	items1, _ := page1["Items"].([]any)
	lek, _ := page1["LastEvaluatedKey"].(map[string]any)
	if len(items1) != 2 || lek == nil {
		t.Fatalf("page1 items=%d lek=%v body=%q", len(items1), lek, first.Body.String())
	}

	second := mustDynamoJSON(t, handler, "Query", map[string]any{
		"TableName":              "gsi2-page",
		"IndexName":              "Gsi2",
		"KeyConditionExpression": "g2 = :g",
		"ExpressionAttributeValues": map[string]any{
			":g": map[string]any{"S": "group"},
		},
		"ExclusiveStartKey": lek,
		"Limit":             2,
	}, now)
	if second.Code != http.StatusOK {
		t.Fatalf("Query page2 status=%d body=%q", second.Code, second.Body.String())
	}
	var page2 map[string]any
	if err := json.Unmarshal(second.Body.Bytes(), &page2); err != nil {
		t.Fatal(err)
	}
	items2, _ := page2["Items"].([]any)
	if len(items2) != 1 {
		t.Fatalf("page2 want 1 item, got %d body=%q", len(items2), second.Body.String())
	}
}

func TestDynamoDBCrossAccountSSEKMSGetItem(t *testing.T) {
	handler, st, ownerAccount, ownerAKID, ownerSecret, _, callerAKID, callerSecret, callerUserARN := setupCrossAccountPair(t)
	now := time.Now().UTC().Truncate(time.Second)

	createKeyRec := mustKMSJSONWithCreds(t, handler, "CreateKey", map[string]any{}, ownerAKID, ownerSecret, now)
	if createKeyRec.Code != http.StatusOK {
		t.Fatalf("CreateKey status=%d body=%q", createKeyRec.Code, createKeyRec.Body.String())
	}
	var createKeyOut map[string]any
	if err := json.Unmarshal(createKeyRec.Body.Bytes(), &createKeyOut); err != nil {
		t.Fatal(err)
	}
	meta, _ := createKeyOut["KeyMetadata"].(map[string]any)
	keyID, _ := meta["KeyId"].(string)

	createRec := mustDynamoJSONWithCreds(t, handler, "CreateTable", map[string]any{
		"TableName":            "xa-sse",
		"AttributeDefinitions": []map[string]string{{"AttributeName": "pk", "AttributeType": "S"}},
		"KeySchema":            []map[string]string{{"AttributeName": "pk", "KeyType": "HASH"}},
		"BillingMode":          "PAY_PER_REQUEST",
		"SSESpecification": map[string]any{
			"Enabled":        true,
			"SSEType":        "KMS",
			"KMSMasterKeyId": keyID,
		},
	}, ownerAKID, ownerSecret, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateTable status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	tableARN := store.TableARN(ownerAccount, testRegion, "xa-sse")

	putItem := mustDynamoJSONWithCreds(t, handler, "PutItem", map[string]any{
		"TableName": "xa-sse",
		"Item":      map[string]any{"pk": map[string]string{"S": "1"}, "v": map[string]string{"S": "secret"}},
	}, ownerAKID, ownerSecret, now)
	if putItem.Code != http.StatusOK {
		t.Fatalf("PutItem status=%d body=%q", putItem.Code, putItem.Body.String())
	}

	identityAllow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["dynamodb:GetItem","kms:Decrypt"],"Resource":"*"}]}`
	if err := st.PutInlinePolicy(callerUserARN, "xa-sse", identityAllow); err != nil {
		t.Fatal(err)
	}
	tablePolicy := fmt.Sprintf(
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"%s"},"Action":"dynamodb:GetItem","Resource":"*"}]}`,
		callerUserARN,
	)
	putPol := mustDynamoJSONWithCreds(t, handler, "PutResourcePolicy", map[string]any{
		"ResourceArn": tableARN,
		"Policy":      tablePolicy,
	}, ownerAKID, ownerSecret, now)
	if putPol.Code != http.StatusOK {
		t.Fatalf("PutResourcePolicy status=%d body=%q", putPol.Code, putPol.Body.String())
	}
	keyPolicy := fmt.Sprintf(
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"%s"},"Action":"kms:Decrypt","Resource":"*"},{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::%s:root"},"Action":"kms:*","Resource":"*"}]}`,
		callerUserARN, ownerAccount,
	)
	putKeyPol := mustKMSJSONWithCreds(t, handler, "PutKeyPolicy", map[string]any{
		"KeyId":      keyID,
		"PolicyName": "default",
		"Policy":     keyPolicy,
	}, ownerAKID, ownerSecret, now)
	if putKeyPol.Code != http.StatusOK {
		t.Fatalf("PutKeyPolicy status=%d body=%q", putKeyPol.Code, putKeyPol.Body.String())
	}

	getRec := mustDynamoJSONWithCreds(t, handler, "GetItem", map[string]any{
		"TableName": tableARN,
		"Key":       map[string]any{"pk": map[string]string{"S": "1"}},
	}, callerAKID, callerSecret, now)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GetItem status=%d want 200 body=%q", getRec.Code, getRec.Body.String())
	}
	var getOut map[string]any
	if err := json.Unmarshal(getRec.Body.Bytes(), &getOut); err != nil {
		t.Fatal(err)
	}
	item, _ := getOut["Item"].(map[string]any)
	val, _ := item["v"].(map[string]any)
	if val["S"] != "secret" {
		t.Fatalf("item=%v", item)
	}
}
