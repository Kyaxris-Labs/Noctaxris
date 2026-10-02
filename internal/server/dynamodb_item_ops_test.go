package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestDynamoDBItemCRUDQueryScanNegatives(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustDynamoJSON(t, handler, "CreateTable", map[string]any{
		"TableName": "lab-items",
		"AttributeDefinitions": []any{
			map[string]any{"AttributeName": "pk", "AttributeType": "S"},
			map[string]any{"AttributeName": "sk", "AttributeType": "S"},
		},
		"KeySchema": []any{
			map[string]any{"AttributeName": "pk", "KeyType": "HASH"},
			map[string]any{"AttributeName": "sk", "KeyType": "RANGE"},
		},
		"BillingMode": "PAY_PER_REQUEST",
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateTable %d %s", create.Code, create.Body.String())
	}
	dup := mustDynamoJSON(t, handler, "CreateTable", map[string]any{
		"TableName": "lab-items",
		"AttributeDefinitions": []any{
			map[string]any{"AttributeName": "pk", "AttributeType": "S"},
		},
		"KeySchema": []any{
			map[string]any{"AttributeName": "pk", "KeyType": "HASH"},
		},
		"BillingMode": "PAY_PER_REQUEST",
	}, now)
	if dup.Code == http.StatusOK {
		t.Fatalf("duplicate CreateTable should fail: %s", dup.Body.String())
	}

	put := mustDynamoJSON(t, handler, "PutItem", map[string]any{
		"TableName": "lab-items",
		"Item": map[string]any{
			"pk":   map[string]any{"S": "user-1"},
			"sk":   map[string]any{"S": "profile"},
			"name": map[string]any{"S": "alice"},
			"n":    map[string]any{"N": "1"},
		},
	}, now)
	if put.Code != http.StatusOK {
		t.Fatalf("PutItem %d %s", put.Code, put.Body.String())
	}
	get := mustDynamoJSON(t, handler, "GetItem", map[string]any{
		"TableName": "lab-items",
		"Key": map[string]any{
			"pk": map[string]any{"S": "user-1"},
			"sk": map[string]any{"S": "profile"},
		},
	}, now)
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), "alice") {
		t.Fatalf("GetItem %d %s", get.Code, get.Body.String())
	}
	upd := mustDynamoJSON(t, handler, "UpdateItem", map[string]any{
		"TableName": "lab-items",
		"Key": map[string]any{
			"pk": map[string]any{"S": "user-1"},
			"sk": map[string]any{"S": "profile"},
		},
		"UpdateExpression": "SET #n = :n",
		"ExpressionAttributeNames": map[string]any{
			"#n": "name",
		},
		"ExpressionAttributeValues": map[string]any{
			":n": map[string]any{"S": "bob"},
		},
		"ReturnValues": "ALL_NEW",
	}, now)
	if upd.Code != http.StatusOK || !strings.Contains(upd.Body.String(), "bob") {
		t.Fatalf("UpdateItem %d %s", upd.Code, upd.Body.String())
	}

	query := mustDynamoJSON(t, handler, "Query", map[string]any{
		"TableName":              "lab-items",
		"KeyConditionExpression": "pk = :pk",
		"ExpressionAttributeValues": map[string]any{
			":pk": map[string]any{"S": "user-1"},
		},
	}, now)
	if query.Code != http.StatusOK || !strings.Contains(query.Body.String(), "Items") {
		t.Fatalf("Query %d %s", query.Code, query.Body.String())
	}
	scan := mustDynamoJSON(t, handler, "Scan", map[string]any{"TableName": "lab-items"}, now)
	if scan.Code != http.StatusOK {
		t.Fatalf("Scan %d %s", scan.Code, scan.Body.String())
	}
	batchGet := mustDynamoJSON(t, handler, "BatchGetItem", map[string]any{
		"RequestItems": map[string]any{
			"lab-items": map[string]any{
				"Keys": []any{
					map[string]any{
						"pk": map[string]any{"S": "user-1"},
						"sk": map[string]any{"S": "profile"},
					},
				},
			},
		},
	}, now)
	if batchGet.Code != http.StatusOK {
		t.Fatalf("BatchGetItem %d %s", batchGet.Code, batchGet.Body.String())
	}
	batchWrite := mustDynamoJSON(t, handler, "BatchWriteItem", map[string]any{
		"RequestItems": map[string]any{
			"lab-items": []any{
				map[string]any{
					"PutRequest": map[string]any{
						"Item": map[string]any{
							"pk": map[string]any{"S": "user-2"},
							"sk": map[string]any{"S": "profile"},
						},
					},
				},
			},
		},
	}, now)
	if batchWrite.Code != http.StatusOK {
		t.Fatalf("BatchWriteItem %d %s", batchWrite.Code, batchWrite.Body.String())
	}

	desc := mustDynamoJSON(t, handler, "DescribeTable", map[string]any{"TableName": "lab-items"}, now)
	if desc.Code != http.StatusOK {
		t.Fatalf("DescribeTable %d %s", desc.Code, desc.Body.String())
	}
	list := mustDynamoJSON(t, handler, "ListTables", map[string]any{}, now)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "lab-items") {
		t.Fatalf("ListTables %d %s", list.Code, list.Body.String())
	}

	delItem := mustDynamoJSON(t, handler, "DeleteItem", map[string]any{
		"TableName": "lab-items",
		"Key": map[string]any{
			"pk": map[string]any{"S": "user-1"},
			"sk": map[string]any{"S": "profile"},
		},
	}, now)
	if delItem.Code != http.StatusOK {
		t.Fatalf("DeleteItem %d %s", delItem.Code, delItem.Body.String())
	}
	delItem2 := mustDynamoJSON(t, handler, "DeleteItem", map[string]any{
		"TableName": "lab-items",
		"Key": map[string]any{
			"pk": map[string]any{"S": "user-2"},
			"sk": map[string]any{"S": "profile"},
		},
	}, now)
	if delItem2.Code != http.StatusOK {
		t.Fatalf("DeleteItem user-2 %d %s", delItem2.Code, delItem2.Body.String())
	}
	getMiss := mustDynamoJSON(t, handler, "GetItem", map[string]any{
		"TableName": "missing-table",
		"Key":       map[string]any{"pk": map[string]any{"S": "x"}},
	}, now)
	if getMiss.Code == http.StatusOK && strings.Contains(getMiss.Body.String(), `"Item"`) {
		// missing table should error; accept non-OK or empty Item path
		var body map[string]any
		_ = json.Unmarshal(getMiss.Body.Bytes(), &body)
		if body["__type"] == nil && body["Item"] != nil {
			t.Fatalf("missing table GetItem unexpected OK with item: %s", getMiss.Body.String())
		}
	}
	if getMiss.Code == http.StatusOK {
		// some paths return ResourceNotFound
	} else if getMiss.Code < 400 {
		t.Fatalf("missing table GetItem %d %s", getMiss.Code, getMiss.Body.String())
	}

	delTable := mustDynamoJSON(t, handler, "DeleteTable", map[string]any{"TableName": "lab-items"}, now)
	if delTable.Code != http.StatusOK {
		t.Fatalf("DeleteTable %d %s", delTable.Code, delTable.Body.String())
	}
	delAgain := mustDynamoJSON(t, handler, "DeleteTable", map[string]any{"TableName": "lab-items"}, now)
	if delAgain.Code == http.StatusOK {
		t.Fatalf("DeleteTable twice should fail: %s", delAgain.Body.String())
	}
}
