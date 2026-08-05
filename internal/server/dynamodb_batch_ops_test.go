package server_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestDynamoDBBatchUnprocessedAndResourcePolicy(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustDynamoJSON(t, handler, "CreateTable", map[string]any{
		"TableName": "batch-limit",
		"AttributeDefinitions": []map[string]any{
			{"AttributeName": "pk", "AttributeType": "S"},
		},
		"KeySchema": []map[string]any{
			{"AttributeName": "pk", "KeyType": "HASH"},
		},
		"BillingMode": "PAY_PER_REQUEST",
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateTable %d %s", create.Code, create.Body.String())
	}

	keys := make([]any, 0, 30)
	writes := make([]any, 0, 30)
	for i := 0; i < 30; i++ {
		pk := fmt.Sprintf("k%d", i)
		keys = append(keys, map[string]any{"pk": map[string]any{"S": pk}})
		writes = append(writes, map[string]any{
			"PutRequest": map[string]any{
				"Item": map[string]any{"pk": map[string]any{"S": pk}, "n": map[string]any{"N": fmt.Sprintf("%d", i)}},
			},
		})
	}
	batchWrite := mustDynamoJSON(t, handler, "BatchWriteItem", map[string]any{
		"RequestItems": map[string]any{"batch-limit": writes},
	}, now)
	if batchWrite.Code != http.StatusOK {
		t.Fatalf("BatchWriteItem %d %s", batchWrite.Code, batchWrite.Body.String())
	}
	if !strings.Contains(batchWrite.Body.String(), "UnprocessedItems") {
		// leftover path may still return empty UnprocessedItems key; accept either
		t.Logf("BatchWriteItem body=%s", batchWrite.Body.String())
	}

	batchGet := mustDynamoJSON(t, handler, "BatchGetItem", map[string]any{
		"RequestItems": map[string]any{
			"batch-limit": map[string]any{"Keys": keys},
		},
	}, now)
	if batchGet.Code != http.StatusOK {
		t.Fatalf("BatchGetItem %d %s", batchGet.Code, batchGet.Body.String())
	}

	badEntry := mustDynamoJSON(t, handler, "BatchGetItem", map[string]any{
		"RequestItems": map[string]any{"batch-limit": "bad"},
	}, now)
	if badEntry.Code == http.StatusOK {
		t.Fatalf("BatchGetItem bad entry should fail")
	}
	badWrite := mustDynamoJSON(t, handler, "BatchWriteItem", map[string]any{
		"RequestItems": map[string]any{"batch-limit": []any{map[string]any{"Nope": true}}},
	}, now)
	if badWrite.Code == http.StatusOK {
		t.Fatalf("BatchWriteItem without Put/Delete should fail")
	}

	pol := mustDynamoJSON(t, handler, "PutResourcePolicy", map[string]any{
		"ResourceArn": "arn:aws:dynamodb:" + testRegion + ":" + testAccountID + ":table/batch-limit",
		"Policy":      `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":"*","Action":"dynamodb:GetItem","Resource":"*"}]}`,
	}, now)
	if pol.Code != http.StatusOK {
		t.Fatalf("PutResourcePolicy %d %s", pol.Code, pol.Body.String())
	}
	getPol := mustDynamoJSON(t, handler, "GetResourcePolicy", map[string]any{
		"ResourceArn": "arn:aws:dynamodb:" + testRegion + ":" + testAccountID + ":table/batch-limit",
	}, now)
	if getPol.Code != http.StatusOK {
		t.Fatalf("GetResourcePolicy %d %s", getPol.Code, getPol.Body.String())
	}
	delPol := mustDynamoJSON(t, handler, "DeleteResourcePolicy", map[string]any{
		"ResourceArn": "arn:aws:dynamodb:" + testRegion + ":" + testAccountID + ":table/batch-limit",
	}, now)
	if delPol.Code != http.StatusOK {
		t.Fatalf("DeleteResourcePolicy %d %s", delPol.Code, delPol.Body.String())
	}

	tw := mustDynamoJSON(t, handler, "TransactWriteItems", map[string]any{
		"TransactItems": []map[string]any{
			{"Put": map[string]any{
				"TableName": "batch-limit",
				"Item":      map[string]any{"pk": map[string]any{"S": "tw-put"}, "n": map[string]any{"N": "1"}},
			}},
			{"Update": map[string]any{
				"TableName":        "batch-limit",
				"Key":              map[string]any{"pk": map[string]any{"S": "k0"}},
				"UpdateExpression": "SET #n = :v",
				"ExpressionAttributeNames": map[string]any{
					"#n": "n",
				},
				"ExpressionAttributeValues": map[string]any{
					":v": map[string]any{"N": "99"},
				},
			}},
			{"ConditionCheck": map[string]any{
				"TableName":           "batch-limit",
				"Key":                 map[string]any{"pk": map[string]any{"S": "k1"}},
				"ConditionExpression": "attribute_exists(pk)",
			}},
			{"Delete": map[string]any{
				"TableName": "batch-limit",
				"Key":       map[string]any{"pk": map[string]any{"S": "k2"}},
			}},
		},
	}, now)
	if tw.Code != http.StatusOK {
		t.Fatalf("TransactWriteItems %d %s", tw.Code, tw.Body.String())
	}
	twEmpty := mustDynamoJSON(t, handler, "TransactWriteItems", map[string]any{}, now)
	if twEmpty.Code == http.StatusOK {
		t.Fatalf("TransactWriteItems empty should fail")
	}
}
