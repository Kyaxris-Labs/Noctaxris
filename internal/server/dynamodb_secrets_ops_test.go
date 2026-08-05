package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestDynamoDBTableOpsQueryScanBatch(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustDynamoJSON(t, handler, "CreateTable", map[string]any{
		"TableName": "ops-items",
		"AttributeDefinitions": []map[string]any{
			{"AttributeName": "pk", "AttributeType": "S"},
			{"AttributeName": "sk", "AttributeType": "S"},
		},
		"KeySchema": []map[string]any{
			{"AttributeName": "pk", "KeyType": "HASH"},
			{"AttributeName": "sk", "KeyType": "RANGE"},
		},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateTable %d %s", create.Code, create.Body.String())
	}

	dup := mustDynamoJSON(t, handler, "CreateTable", map[string]any{
		"TableName": "ops-items",
		"AttributeDefinitions": []map[string]any{
			{"AttributeName": "pk", "AttributeType": "S"},
		},
		"KeySchema": []map[string]any{
			{"AttributeName": "pk", "KeyType": "HASH"},
		},
	}, now)
	if dup.Code == http.StatusOK {
		t.Fatalf("duplicate CreateTable should fail: %s", dup.Body.String())
	}

	desc := mustDynamoJSON(t, handler, "DescribeTable", map[string]any{"TableName": "ops-items"}, now)
	if desc.Code != http.StatusOK || !strings.Contains(desc.Body.String(), "ops-items") {
		t.Fatalf("DescribeTable %d %s", desc.Code, desc.Body.String())
	}
	descMissing := mustDynamoJSON(t, handler, "DescribeTable", map[string]any{"TableName": "missing"}, now)
	if descMissing.Code == http.StatusOK {
		t.Fatalf("DescribeTable missing should fail")
	}

	list := mustDynamoJSON(t, handler, "ListTables", map[string]any{}, now)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "ops-items") {
		t.Fatalf("ListTables %d %s", list.Code, list.Body.String())
	}

	for _, sk := range []string{"a", "b", "c"} {
		put := mustDynamoJSON(t, handler, "PutItem", map[string]any{
			"TableName": "ops-items",
			"Item": map[string]any{
				"pk":  map[string]any{"S": "u1"},
				"sk":  map[string]any{"S": sk},
				"val": map[string]any{"S": "v-" + sk},
			},
		}, now)
		if put.Code != http.StatusOK {
			t.Fatalf("PutItem %s: %d %s", sk, put.Code, put.Body.String())
		}
	}

	upd := mustDynamoJSON(t, handler, "UpdateItem", map[string]any{
		"TableName": "ops-items",
		"Key": map[string]any{
			"pk": map[string]any{"S": "u1"},
			"sk": map[string]any{"S": "a"},
		},
		"UpdateExpression": "SET #v = :v",
		"ExpressionAttributeNames": map[string]any{
			"#v": "val",
		},
		"ExpressionAttributeValues": map[string]any{
			":v": map[string]any{"S": "updated"},
		},
	}, now)
	if upd.Code != http.StatusOK {
		t.Fatalf("UpdateItem %d %s", upd.Code, upd.Body.String())
	}

	query := mustDynamoJSON(t, handler, "Query", map[string]any{
		"TableName":              "ops-items",
		"KeyConditionExpression": "pk = :pk",
		"ExpressionAttributeValues": map[string]any{
			":pk": map[string]any{"S": "u1"},
		},
	}, now)
	if query.Code != http.StatusOK {
		t.Fatalf("Query %d %s", query.Code, query.Body.String())
	}

	scan := mustDynamoJSON(t, handler, "Scan", map[string]any{"TableName": "ops-items"}, now)
	if scan.Code != http.StatusOK {
		t.Fatalf("Scan %d %s", scan.Code, scan.Body.String())
	}

	batchGet := mustDynamoJSON(t, handler, "BatchGetItem", map[string]any{
		"RequestItems": map[string]any{
			"ops-items": map[string]any{
				"Keys": []map[string]any{
					{"pk": map[string]any{"S": "u1"}, "sk": map[string]any{"S": "a"}},
					{"pk": map[string]any{"S": "u1"}, "sk": map[string]any{"S": "b"}},
				},
			},
		},
	}, now)
	if batchGet.Code != http.StatusOK {
		t.Fatalf("BatchGetItem %d %s", batchGet.Code, batchGet.Body.String())
	}

	batchWrite := mustDynamoJSON(t, handler, "BatchWriteItem", map[string]any{
		"RequestItems": map[string]any{
			"ops-items": []map[string]any{
				{"PutRequest": map[string]any{
					"Item": map[string]any{
						"pk": map[string]any{"S": "u2"},
						"sk": map[string]any{"S": "z"},
					},
				}},
				{"DeleteRequest": map[string]any{
					"Key": map[string]any{
						"pk": map[string]any{"S": "u1"},
						"sk": map[string]any{"S": "c"},
					},
				}},
			},
		},
	}, now)
	if batchWrite.Code != http.StatusOK {
		t.Fatalf("BatchWriteItem %d %s", batchWrite.Code, batchWrite.Body.String())
	}

	delItem := mustDynamoJSON(t, handler, "DeleteItem", map[string]any{
		"TableName": "ops-items",
		"Key": map[string]any{
			"pk": map[string]any{"S": "u1"},
			"sk": map[string]any{"S": "b"},
		},
	}, now)
	if delItem.Code != http.StatusOK {
		t.Fatalf("DeleteItem %d %s", delItem.Code, delItem.Body.String())
	}

	// clear remaining items then delete table
	scanAll := mustDynamoJSON(t, handler, "Scan", map[string]any{"TableName": "ops-items"}, now)
	var scanOut map[string]any
	_ = json.Unmarshal(scanAll.Body.Bytes(), &scanOut)
	if items, ok := scanOut["Items"].([]any); ok {
		for _, it := range items {
			m, _ := it.(map[string]any)
			pk, _ := m["pk"].(map[string]any)
			sk, _ := m["sk"].(map[string]any)
			mustDynamoJSON(t, handler, "DeleteItem", map[string]any{
				"TableName": "ops-items",
				"Key":       map[string]any{"pk": pk, "sk": sk},
			}, now)
		}
	}

	delTable := mustDynamoJSON(t, handler, "DeleteTable", map[string]any{"TableName": "ops-items"}, now)
	if delTable.Code != http.StatusOK {
		t.Fatalf("DeleteTable %d %s", delTable.Code, delTable.Body.String())
	}
	delGone := mustDynamoJSON(t, handler, "DeleteTable", map[string]any{"TableName": "ops-items"}, now)
	if delGone.Code == http.StatusOK {
		t.Fatalf("DeleteTable missing should fail")
	}
}

func TestSecretsManagerLifecycle(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustJSONTarget(t, handler, "secretsmanager.CreateSecret", "secretsmanager", map[string]any{
		"Name":         "lab/secret-cov",
		"SecretString": "super-secret",
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateSecret %d %s", create.Code, create.Body.String())
	}

	get := mustJSONTarget(t, handler, "secretsmanager.GetSecretValue", "secretsmanager", map[string]any{
		"SecretId": "lab/secret-cov",
	}, now)
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), "super-secret") {
		t.Fatalf("GetSecretValue %d %s", get.Code, get.Body.String())
	}

	list := mustJSONTarget(t, handler, "secretsmanager.ListSecrets", "secretsmanager", map[string]any{}, now)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "lab/secret-cov") {
		t.Fatalf("ListSecrets %d %s", list.Code, list.Body.String())
	}

	put := mustJSONTarget(t, handler, "secretsmanager.PutSecretValue", "secretsmanager", map[string]any{
		"SecretId":     "lab/secret-cov",
		"SecretString": "rotated",
	}, now)
	if put.Code != http.StatusOK {
		t.Fatalf("PutSecretValue %d %s", put.Code, put.Body.String())
	}

	desc := mustJSONTarget(t, handler, "secretsmanager.DescribeSecret", "secretsmanager", map[string]any{
		"SecretId": "lab/secret-cov",
	}, now)
	if desc.Code != http.StatusOK {
		t.Fatalf("DescribeSecret %d %s", desc.Code, desc.Body.String())
	}

	del := mustJSONTarget(t, handler, "secretsmanager.DeleteSecret", "secretsmanager", map[string]any{
		"SecretId":                   "lab/secret-cov",
		"ForceDeleteWithoutRecovery": true,
	}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteSecret %d %s", del.Code, del.Body.String())
	}

	missing := mustJSONTarget(t, handler, "secretsmanager.GetSecretValue", "secretsmanager", map[string]any{
		"SecretId": "lab/secret-cov",
	}, now)
	if missing.Code == http.StatusOK {
		t.Fatalf("GetSecretValue after delete should fail")
	}
}
