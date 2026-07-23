package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestDynamoDBTransactWriteAndGet(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustDynamoJSON(t, handler, "CreateTable", map[string]any{
		"TableName": "txn-items",
		"AttributeDefinitions": []map[string]any{
			{"AttributeName": "pk", "AttributeType": "S"},
		},
		"KeySchema": []map[string]any{
			{"AttributeName": "pk", "KeyType": "HASH"},
		},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateTable status=%d body=%q", create.Code, create.Body.String())
	}

	write := mustDynamoJSON(t, handler, "TransactWriteItems", map[string]any{
		"TransactItems": []any{
			map[string]any{
				"Put": map[string]any{
					"TableName": "txn-items",
					"Item": map[string]any{
						"pk":  map[string]any{"S": "a"},
						"val": map[string]any{"S": "one"},
					},
				},
			},
			map[string]any{
				"Put": map[string]any{
					"TableName": "txn-items",
					"Item": map[string]any{
						"pk":  map[string]any{"S": "b"},
						"val": map[string]any{"S": "two"},
					},
				},
			},
		},
	}, now)
	if write.Code != http.StatusOK {
		t.Fatalf("TransactWriteItems status=%d body=%q", write.Code, write.Body.String())
	}

	get := mustDynamoJSON(t, handler, "TransactGetItems", map[string]any{
		"TransactItems": []any{
			map[string]any{
				"Get": map[string]any{
					"TableName": "txn-items",
					"Key":       map[string]any{"pk": map[string]any{"S": "a"}},
				},
			},
			map[string]any{
				"Get": map[string]any{
					"TableName": "txn-items",
					"Key":       map[string]any{"pk": map[string]any{"S": "b"}},
				},
			},
		},
	}, now)
	if get.Code != http.StatusOK {
		t.Fatalf("TransactGetItems status=%d body=%q", get.Code, get.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(get.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	responses, _ := out["Responses"].([]any)
	if len(responses) != 2 {
		t.Fatalf("Responses=%v", out)
	}
}

func TestDynamoDBTransactWriteConditionCheckCancels(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustDynamoJSON(t, handler, "CreateTable", map[string]any{
		"TableName": "txn-cancel",
		"AttributeDefinitions": []map[string]any{
			{"AttributeName": "pk", "AttributeType": "S"},
		},
		"KeySchema": []map[string]any{
			{"AttributeName": "pk", "KeyType": "HASH"},
		},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateTable status=%d body=%q", create.Code, create.Body.String())
	}

	write := mustDynamoJSON(t, handler, "TransactWriteItems", map[string]any{
		"TransactItems": []any{
			map[string]any{
				"Put": map[string]any{
					"TableName": "txn-cancel",
					"Item":      map[string]any{"pk": map[string]any{"S": "new"}},
				},
			},
			map[string]any{
				"ConditionCheck": map[string]any{
					"TableName":           "txn-cancel",
					"Key":                 map[string]any{"pk": map[string]any{"S": "missing"}},
					"ConditionExpression": "attribute_exists(pk)",
				},
			},
		},
	}, now)
	if write.Code != http.StatusBadRequest || !strings.Contains(write.Body.String(), "TransactionCanceledException") {
		t.Fatalf("want TransactionCanceledException, status=%d body=%q", write.Code, write.Body.String())
	}

	get := mustDynamoJSON(t, handler, "GetItem", map[string]any{
		"TableName": "txn-cancel",
		"Key":       map[string]any{"pk": map[string]any{"S": "new"}},
	}, now)
	if get.Code != http.StatusOK {
		t.Fatalf("GetItem status=%d body=%q", get.Code, get.Body.String())
	}
	if strings.Contains(get.Body.String(), `"Item"`) {
		t.Fatalf("put must roll back, body=%q", get.Body.String())
	}
}

func TestDynamoDBTransactWriteUpdateWithCondition(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustDynamoJSON(t, handler, "CreateTable", map[string]any{
		"TableName": "txn-update",
		"AttributeDefinitions": []map[string]any{
			{"AttributeName": "pk", "AttributeType": "S"},
		},
		"KeySchema": []map[string]any{
			{"AttributeName": "pk", "KeyType": "HASH"},
		},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateTable status=%d body=%q", create.Code, create.Body.String())
	}

	put := mustDynamoJSON(t, handler, "PutItem", map[string]any{
		"TableName": "txn-update",
		"Item": map[string]any{
			"pk":      map[string]any{"S": "acct"},
			"balance": map[string]any{"N": "10"},
		},
	}, now)
	if put.Code != http.StatusOK {
		t.Fatalf("PutItem status=%d body=%q", put.Code, put.Body.String())
	}

	write := mustDynamoJSON(t, handler, "TransactWriteItems", map[string]any{
		"TransactItems": []any{
			map[string]any{
				"Update": map[string]any{
					"TableName":           "txn-update",
					"Key":                 map[string]any{"pk": map[string]any{"S": "acct"}},
					"UpdateExpression":    "SET balance = :n",
					"ConditionExpression": "balance = :old",
					"ExpressionAttributeValues": map[string]any{
						":n":   map[string]any{"N": "20"},
						":old": map[string]any{"N": "10"},
					},
				},
			},
		},
	}, now)
	if write.Code != http.StatusOK {
		t.Fatalf("TransactWriteItems Update status=%d body=%q", write.Code, write.Body.String())
	}

	get := mustDynamoJSON(t, handler, "GetItem", map[string]any{
		"TableName": "txn-update",
		"Key":       map[string]any{"pk": map[string]any{"S": "acct"}},
	}, now)
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), `"20"`) {
		t.Fatalf("want balance 20, status=%d body=%q", get.Code, get.Body.String())
	}

	cancel := mustDynamoJSON(t, handler, "TransactWriteItems", map[string]any{
		"TransactItems": []any{
			map[string]any{
				"Update": map[string]any{
					"TableName":           "txn-update",
					"Key":                 map[string]any{"pk": map[string]any{"S": "acct"}},
					"UpdateExpression":    "SET balance = :n",
					"ConditionExpression": "balance = :old",
					"ExpressionAttributeValues": map[string]any{
						":n":   map[string]any{"N": "30"},
						":old": map[string]any{"N": "10"},
					},
				},
			},
		},
	}, now)
	if cancel.Code != http.StatusBadRequest || !strings.Contains(cancel.Body.String(), "TransactionCanceledException") {
		t.Fatalf("want TransactionCanceledException, status=%d body=%q", cancel.Code, cancel.Body.String())
	}
}
