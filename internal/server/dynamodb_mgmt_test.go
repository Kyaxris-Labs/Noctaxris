package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDynamoDBListDescribeDeleteUpdateAndItems(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustDynamoJSON(t, handler, "CreateTable", map[string]any{
		"TableName": "mgmt-items",
		"AttributeDefinitions": []map[string]any{
			{"AttributeName": "pk", "AttributeType": "S"},
			{"AttributeName": "sk", "AttributeType": "S"},
			{"AttributeName": "gsi1", "AttributeType": "S"},
		},
		"KeySchema": []map[string]any{
			{"AttributeName": "pk", "KeyType": "HASH"},
			{"AttributeName": "sk", "KeyType": "RANGE"},
		},
		"BillingMode": "PAY_PER_REQUEST",
		"StreamSpecification": map[string]any{
			"StreamEnabled":  true,
			"StreamViewType": "NEW_AND_OLD_IMAGES",
		},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateTable status=%d body=%q", create.Code, create.Body.String())
	}

	list := mustDynamoJSON(t, handler, "ListTables", map[string]any{}, now)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "mgmt-items") {
		t.Fatalf("ListTables status=%d body=%q", list.Code, list.Body.String())
	}

	desc := mustDynamoJSON(t, handler, "DescribeTable", map[string]any{"TableName": "mgmt-items"}, now)
	if desc.Code != http.StatusOK || !strings.Contains(desc.Body.String(), "mgmt-items") {
		t.Fatalf("DescribeTable status=%d body=%q", desc.Code, desc.Body.String())
	}
	missingDesc := mustDynamoJSON(t, handler, "DescribeTable", map[string]any{"TableName": "no-such"}, now)
	if missingDesc.Code != http.StatusBadRequest || !strings.Contains(missingDesc.Body.String(), "ResourceNotFoundException") {
		t.Fatalf("DescribeTable missing want ResourceNotFound status=%d body=%q", missingDesc.Code, missingDesc.Body.String())
	}

	upd := mustDynamoJSON(t, handler, "UpdateTable", map[string]any{
		"TableName": "mgmt-items",
		"AttributeDefinitions": []map[string]any{
			{"AttributeName": "pk", "AttributeType": "S"},
			{"AttributeName": "gsi1", "AttributeType": "S"},
		},
		"GlobalSecondaryIndexUpdates": []map[string]any{{
			"Create": map[string]any{
				"IndexName": "GSI1",
				"KeySchema": []map[string]any{
					{"AttributeName": "gsi1", "KeyType": "HASH"},
				},
				"Projection": map[string]any{"ProjectionType": "ALL"},
			},
		}},
	}, now)
	if upd.Code != http.StatusOK {
		t.Fatalf("UpdateTable status=%d body=%q", upd.Code, upd.Body.String())
	}
	badUpd := mustDynamoJSON(t, handler, "UpdateTable", map[string]any{
		"TableName": "mgmt-items",
		"GlobalSecondaryIndexUpdates": []map[string]any{{
			"Delete": map[string]any{"IndexName": "GSI1"},
		}},
	}, now)
	if badUpd.Code != http.StatusBadRequest {
		t.Fatalf("UpdateTable Delete-only want 400 status=%d body=%q", badUpd.Code, badUpd.Body.String())
	}

	put := mustDynamoJSON(t, handler, "PutItem", map[string]any{
		"TableName": "mgmt-items",
		"Item": map[string]any{
			"pk":   map[string]any{"S": "u1"},
			"sk":   map[string]any{"S": "a"},
			"gsi1": map[string]any{"S": "g1"},
			"n":    map[string]any{"N": "1"},
		},
	}, now)
	if put.Code != http.StatusOK {
		t.Fatalf("PutItem status=%d body=%q", put.Code, put.Body.String())
	}

	updateItem := mustDynamoJSON(t, handler, "UpdateItem", map[string]any{
		"TableName": "mgmt-items",
		"Key": map[string]any{
			"pk": map[string]any{"S": "u1"},
			"sk": map[string]any{"S": "a"},
		},
		"UpdateExpression": "SET #n = :n",
		"ExpressionAttributeNames": map[string]any{
			"#n": "n",
		},
		"ExpressionAttributeValues": map[string]any{
			":n": map[string]any{"N": "2"},
		},
	}, now)
	if updateItem.Code != http.StatusOK {
		t.Fatalf("UpdateItem status=%d body=%q", updateItem.Code, updateItem.Body.String())
	}
	badUpdate := mustDynamoJSON(t, handler, "UpdateItem", map[string]any{
		"TableName": "mgmt-items",
		"Key": map[string]any{
			"pk": map[string]any{"S": "u1"},
			"sk": map[string]any{"S": "a"},
		},
		"UpdateExpression": "INVALID EXPR",
	}, now)
	if badUpdate.Code != http.StatusBadRequest {
		t.Fatalf("UpdateItem bad expr want 400 status=%d body=%q", badUpdate.Code, badUpdate.Body.String())
	}

	batchWrite := mustDynamoJSON(t, handler, "BatchWriteItem", map[string]any{
		"RequestItems": map[string]any{
			"mgmt-items": []map[string]any{
				{"PutRequest": map[string]any{"Item": map[string]any{
					"pk": map[string]any{"S": "u2"},
					"sk": map[string]any{"S": "b"},
				}}},
				{"DeleteRequest": map[string]any{"Key": map[string]any{
					"pk": map[string]any{"S": "missing"},
					"sk": map[string]any{"S": "x"},
				}}},
			},
		},
	}, now)
	if batchWrite.Code != http.StatusOK {
		t.Fatalf("BatchWriteItem status=%d body=%q", batchWrite.Code, batchWrite.Body.String())
	}
	emptyBatch := mustDynamoJSON(t, handler, "BatchWriteItem", map[string]any{}, now)
	if emptyBatch.Code != http.StatusBadRequest {
		t.Fatalf("BatchWriteItem empty want 400 status=%d body=%q", emptyBatch.Code, emptyBatch.Body.String())
	}

	delItem := mustDynamoJSON(t, handler, "DeleteItem", map[string]any{
		"TableName": "mgmt-items",
		"Key": map[string]any{
			"pk": map[string]any{"S": "u1"},
			"sk": map[string]any{"S": "a"},
		},
	}, now)
	if delItem.Code != http.StatusOK {
		t.Fatalf("DeleteItem status=%d body=%q", delItem.Code, delItem.Body.String())
	}

	txDel := mustDynamoJSON(t, handler, "TransactWriteItems", map[string]any{
		"TransactItems": []map[string]any{{
			"Delete": map[string]any{
				"TableName": "mgmt-items",
				"Key": map[string]any{
					"pk": map[string]any{"S": "u2"},
					"sk": map[string]any{"S": "b"},
				},
			},
		}},
	}, now)
	if txDel.Code != http.StatusOK {
		t.Fatalf("TransactWriteItems Delete status=%d body=%q", txDel.Code, txDel.Body.String())
	}

	delTableBusy := mustDynamoJSON(t, handler, "DeleteTable", map[string]any{"TableName": "mgmt-items"}, now)
	// empty table should delete; if items remain, ResourceInUse is fine too
	if delTableBusy.Code != http.StatusOK && !strings.Contains(delTableBusy.Body.String(), "ResourceInUseException") {
		t.Fatalf("DeleteTable status=%d body=%q", delTableBusy.Code, delTableBusy.Body.String())
	}
	if delTableBusy.Code != http.StatusOK {
		_ = st
		scan := mustDynamoJSON(t, handler, "Scan", map[string]any{"TableName": "mgmt-items"}, now)
		if scan.Code == http.StatusOK {
			var scanOut map[string]any
			_ = json.Unmarshal(scan.Body.Bytes(), &scanOut)
			if items, _ := scanOut["Items"].([]any); len(items) > 0 {
				for _, raw := range items {
					item, _ := raw.(map[string]any)
					key := map[string]any{}
					if v, ok := item["pk"]; ok {
						key["pk"] = v
					}
					if v, ok := item["sk"]; ok {
						key["sk"] = v
					}
					_ = mustDynamoJSON(t, handler, "DeleteItem", map[string]any{"TableName": "mgmt-items", "Key": key}, now)
				}
			}
		}
		del := mustDynamoJSON(t, handler, "DeleteTable", map[string]any{"TableName": "mgmt-items"}, now)
		if del.Code != http.StatusOK {
			t.Fatalf("DeleteTable retry status=%d body=%q", del.Code, del.Body.String())
		}
	}
	gone := mustDynamoJSON(t, handler, "DeleteTable", map[string]any{"TableName": "mgmt-items"}, now)
	if gone.Code != http.StatusBadRequest || !strings.Contains(gone.Body.String(), "ResourceNotFoundException") {
		t.Fatalf("DeleteTable missing want ResourceNotFound status=%d body=%q", gone.Code, gone.Body.String())
	}

	_, denyARN, err := st.CreateUser(testAccountID, "ddb-list-deny")
	if err != nil {
		t.Fatal(err)
	}
	ak, secret, err := st.CreateUserAccessKey(testAccountID, "ddb-list-deny")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutInlinePolicy(denyARN, "deny", `{
		"Version":"2012-10-17",
		"Statement":[{"Effect":"Deny","Action":"dynamodb:ListTables","Resource":"*"}]
	}`); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{})
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.0")
	req.Header.Set("X-Amz-Target", "DynamoDB_20120810.ListTables")
	signHeader(t, req, raw, ak, secret, testRegion, "dynamodb", now)
	denyRec := httptest.NewRecorder()
	handler.ServeHTTP(denyRec, req)
	if denyRec.Code != http.StatusForbidden || !strings.Contains(denyRec.Body.String(), "AccessDeniedException") {
		t.Fatalf("ListTables deny status=%d body=%q", denyRec.Code, denyRec.Body.String())
	}
}

func TestDynamoDBResourcePolicyGetDeleteAndPartiQL(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustDynamoJSON(t, handler, "CreateTable", map[string]any{
		"TableName": "partiql-lab",
		"AttributeDefinitions": []map[string]any{
			{"AttributeName": "pk", "AttributeType": "S"},
		},
		"KeySchema": []map[string]any{
			{"AttributeName": "pk", "KeyType": "HASH"},
		},
		"BillingMode": "PAY_PER_REQUEST",
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateTable status=%d body=%q", create.Code, create.Body.String())
	}

	noPol := mustDynamoJSON(t, handler, "GetResourcePolicy", map[string]any{
		"ResourceArn": "arn:aws:dynamodb:us-east-1:" + testAccountID + ":table/partiql-lab",
	}, now)
	if noPol.Code != http.StatusBadRequest || !strings.Contains(noPol.Body.String(), "PolicyNotFoundException") {
		t.Fatalf("GetResourcePolicy missing want PolicyNotFound status=%d body=%q", noPol.Code, noPol.Body.String())
	}

	putPol := mustDynamoJSON(t, handler, "PutResourcePolicy", map[string]any{
		"ResourceArn": "arn:aws:dynamodb:us-east-1:" + testAccountID + ":table/partiql-lab",
		"Policy":      `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":"*","Action":"dynamodb:*","Resource":"*"}]}`,
	}, now)
	if putPol.Code != http.StatusOK {
		t.Fatalf("PutResourcePolicy status=%d body=%q", putPol.Code, putPol.Body.String())
	}
	getPol := mustDynamoJSON(t, handler, "GetResourcePolicy", map[string]any{
		"TableName": "partiql-lab",
	}, now)
	if getPol.Code != http.StatusOK || !strings.Contains(getPol.Body.String(), "Version") {
		t.Fatalf("GetResourcePolicy status=%d body=%q", getPol.Code, getPol.Body.String())
	}
	delPol := mustDynamoJSON(t, handler, "DeleteResourcePolicy", map[string]any{
		"ResourceArn": "arn:aws:dynamodb:us-east-1:" + testAccountID + ":table/partiql-lab",
	}, now)
	if delPol.Code != http.StatusOK {
		t.Fatalf("DeleteResourcePolicy status=%d body=%q", delPol.Code, delPol.Body.String())
	}

	ins := mustDynamoJSON(t, handler, "ExecuteStatement", map[string]any{
		"Statement": `INSERT INTO "partiql-lab" VALUE {'pk':?}`,
		"Parameters": []any{
			map[string]any{"S": "row1"},
		},
	}, now)
	if ins.Code != http.StatusOK {
		t.Fatalf("ExecuteStatement INSERT status=%d body=%q", ins.Code, ins.Body.String())
	}
	sel := mustDynamoJSON(t, handler, "ExecuteStatement", map[string]any{
		"Statement": `SELECT * FROM "partiql-lab" WHERE pk=?`,
		"Parameters": []any{
			map[string]any{"S": "row1"},
		},
	}, now)
	if sel.Code != http.StatusOK || !strings.Contains(sel.Body.String(), "row1") {
		t.Fatalf("ExecuteStatement SELECT status=%d body=%q", sel.Code, sel.Body.String())
	}
	badSQL := mustDynamoJSON(t, handler, "ExecuteStatement", map[string]any{
		"Statement": "DROP TABLE partiql-lab",
	}, now)
	if badSQL.Code != http.StatusBadRequest {
		t.Fatalf("ExecuteStatement bad want 400 status=%d body=%q", badSQL.Code, badSQL.Body.String())
	}

	batch := mustDynamoJSON(t, handler, "BatchExecuteStatement", map[string]any{
		"Statements": []map[string]any{
			{
				"Statement": `INSERT INTO "partiql-lab" VALUE {'pk':?}`,
				"Parameters": []any{
					map[string]any{"S": "row2"},
				},
			},
			{
				"Statement": "NOT A STATEMENT",
			},
			{
				"Statement": `SELECT * FROM "partiql-lab" WHERE pk=?`,
				"Parameters": []any{
					map[string]any{"S": "row2"},
				},
			},
		},
	}, now)
	if batch.Code != http.StatusOK {
		t.Fatalf("BatchExecuteStatement status=%d body=%q", batch.Code, batch.Body.String())
	}
	emptyBatch := mustDynamoJSON(t, handler, "BatchExecuteStatement", map[string]any{}, now)
	if emptyBatch.Code != http.StatusBadRequest {
		t.Fatalf("BatchExecuteStatement empty want 400 status=%d body=%q", emptyBatch.Code, emptyBatch.Body.String())
	}

	del := mustDynamoJSON(t, handler, "ExecuteStatement", map[string]any{
		"Statement": `DELETE FROM "partiql-lab" WHERE pk=?`,
		"Parameters": []any{
			map[string]any{"S": "row1"},
		},
	}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("ExecuteStatement DELETE status=%d body=%q", del.Code, del.Body.String())
	}
}

func TestDynamoDBLSICreateTableCoverage(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustDynamoJSON(t, handler, "CreateTable", map[string]any{
		"TableName": "lsi-lab",
		"AttributeDefinitions": []map[string]any{
			{"AttributeName": "pk", "AttributeType": "S"},
			{"AttributeName": "sk", "AttributeType": "S"},
			{"AttributeName": "status", "AttributeType": "S"},
		},
		"KeySchema": []map[string]any{
			{"AttributeName": "pk", "KeyType": "HASH"},
			{"AttributeName": "sk", "KeyType": "RANGE"},
		},
		"LocalSecondaryIndexes": []map[string]any{{
			"IndexName": "ByStatus",
			"KeySchema": []map[string]any{
				{"AttributeName": "pk", "KeyType": "HASH"},
				{"AttributeName": "status", "KeyType": "RANGE"},
			},
			"Projection": map[string]any{"ProjectionType": "ALL"},
		}},
		"BillingMode": "PAY_PER_REQUEST",
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateTable LSI status=%d body=%q", create.Code, create.Body.String())
	}
	desc := mustDynamoJSON(t, handler, "DescribeTable", map[string]any{"TableName": "lsi-lab"}, now)
	if desc.Code != http.StatusOK || !strings.Contains(desc.Body.String(), "ByStatus") {
		t.Fatalf("DescribeTable LSI status=%d body=%q", desc.Code, desc.Body.String())
	}
	del := mustDynamoJSON(t, handler, "DeleteTable", map[string]any{"TableName": "lsi-lab"}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteTable status=%d body=%q", del.Code, del.Body.String())
	}
}
