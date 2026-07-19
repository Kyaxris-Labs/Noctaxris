package server_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"
)

func TestDynamoDBGSIQuery(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustDynamoJSON(t, handler, "CreateTable", map[string]any{
		"TableName": "gsi-music",
		"AttributeDefinitions": []map[string]any{
			{"AttributeName": "Artist", "AttributeType": "S"},
			{"AttributeName": "SongTitle", "AttributeType": "S"},
			{"AttributeName": "Genre", "AttributeType": "S"},
		},
		"KeySchema": []map[string]any{
			{"AttributeName": "Artist", "KeyType": "HASH"},
			{"AttributeName": "SongTitle", "KeyType": "RANGE"},
		},
		"GlobalSecondaryIndexes": []map[string]any{{
			"IndexName": "GenreIndex",
			"KeySchema": []map[string]any{
				{"AttributeName": "Genre", "KeyType": "HASH"},
			},
			"Projection": map[string]any{"ProjectionType": "ALL"},
		}},
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateTable status=%d body=%q", createRec.Code, createRec.Body.String())
	}

	putRec := mustDynamoJSON(t, handler, "PutItem", map[string]any{
		"TableName": "gsi-music",
		"Item": map[string]any{
			"Artist":    map[string]any{"S": "Radiohead"},
			"SongTitle": map[string]any{"S": "Creep"},
			"Genre":     map[string]any{"S": "Rock"},
		},
	}, now)
	if putRec.Code != http.StatusOK {
		t.Fatalf("PutItem status=%d body=%q", putRec.Code, putRec.Body.String())
	}
	putRec2 := mustDynamoJSON(t, handler, "PutItem", map[string]any{
		"TableName": "gsi-music",
		"Item": map[string]any{
			"Artist":    map[string]any{"S": "Miles Davis"},
			"SongTitle": map[string]any{"S": "So What"},
			"Genre":     map[string]any{"S": "Jazz"},
		},
	}, now)
	if putRec2.Code != http.StatusOK {
		t.Fatalf("PutItem2 status=%d body=%q", putRec2.Code, putRec2.Body.String())
	}

	queryRec := mustDynamoJSON(t, handler, "Query", map[string]any{
		"TableName":              "gsi-music",
		"IndexName":              "GenreIndex",
		"KeyConditionExpression": "Genre = :g",
		"ExpressionAttributeValues": map[string]any{
			":g": map[string]any{"S": "Rock"},
		},
	}, now)
	if queryRec.Code != http.StatusOK {
		t.Fatalf("Query status=%d body=%q", queryRec.Code, queryRec.Body.String())
	}
	var queryOut map[string]any
	if err := json.Unmarshal(queryRec.Body.Bytes(), &queryOut); err != nil {
		t.Fatal(err)
	}
	items, _ := queryOut["Items"].([]any)
	if len(items) != 1 {
		t.Fatalf("want 1 item, got %d body=%q", len(items), queryRec.Body.String())
	}
}

func TestDynamoDBTTLDescribeUpdateAndLazyExpiry(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustDynamoJSON(t, handler, "CreateTable", map[string]any{
		"TableName": "ttl-sessions",
		"AttributeDefinitions": []map[string]any{
			{"AttributeName": "Id", "AttributeType": "S"},
		},
		"KeySchema": []map[string]any{
			{"AttributeName": "Id", "KeyType": "HASH"},
		},
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateTable status=%d body=%q", createRec.Code, createRec.Body.String())
	}

	updateTTL := mustDynamoJSON(t, handler, "UpdateTimeToLive", map[string]any{
		"TableName": "ttl-sessions",
		"TimeToLiveSpecification": map[string]any{
			"AttributeName": "expires",
			"Enabled":       true,
		},
	}, now)
	if updateTTL.Code != http.StatusOK {
		t.Fatalf("UpdateTimeToLive status=%d body=%q", updateTTL.Code, updateTTL.Body.String())
	}

	descTTL := mustDynamoJSON(t, handler, "DescribeTimeToLive", map[string]any{
		"TableName": "ttl-sessions",
	}, now)
	if descTTL.Code != http.StatusOK {
		t.Fatalf("DescribeTimeToLive status=%d body=%q", descTTL.Code, descTTL.Body.String())
	}
	var ttlOut map[string]any
	if err := json.Unmarshal(descTTL.Body.Bytes(), &ttlOut); err != nil {
		t.Fatal(err)
	}
	desc, _ := ttlOut["TimeToLiveDescription"].(map[string]any)
	if desc["TimeToLiveStatus"] != "ENABLED" || desc["AttributeName"] != "expires" {
		t.Fatalf("ttl description=%v", desc)
	}

	past := strconv.FormatInt(now.Add(-time.Hour).Unix(), 10)
	putRec := mustDynamoJSON(t, handler, "PutItem", map[string]any{
		"TableName": "ttl-sessions",
		"Item": map[string]any{
			"Id":      map[string]any{"S": "expired-row"},
			"expires": map[string]any{"N": past},
		},
	}, now)
	if putRec.Code != http.StatusOK {
		t.Fatalf("PutItem status=%d body=%q", putRec.Code, putRec.Body.String())
	}

	getRec := mustDynamoJSON(t, handler, "GetItem", map[string]any{
		"TableName": "ttl-sessions",
		"Key": map[string]any{
			"Id": map[string]any{"S": "expired-row"},
		},
	}, now)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GetItem status=%d body=%q", getRec.Code, getRec.Body.String())
	}
	var getOut map[string]any
	if err := json.Unmarshal(getRec.Body.Bytes(), &getOut); err != nil {
		t.Fatal(err)
	}
	if _, ok := getOut["Item"]; ok {
		t.Fatalf("expired item should be omitted, body=%q", getRec.Body.String())
	}
}
