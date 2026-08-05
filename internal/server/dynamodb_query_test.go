package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestDynamoDBQueryScanStreamAndConditions(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustDynamoJSON(t, handler, "CreateTable", map[string]any{
		"TableName": "query-lab",
		"AttributeDefinitions": []map[string]any{
			{"AttributeName": "pk", "AttributeType": "S"},
			{"AttributeName": "sk", "AttributeType": "S"},
			{"AttributeName": "gsi1", "AttributeType": "S"},
		},
		"KeySchema": []map[string]any{
			{"AttributeName": "pk", "KeyType": "HASH"},
			{"AttributeName": "sk", "KeyType": "RANGE"},
		},
		"GlobalSecondaryIndexes": []map[string]any{{
			"IndexName": "GSI1",
			"KeySchema": []map[string]any{
				{"AttributeName": "gsi1", "KeyType": "HASH"},
			},
			"Projection": map[string]any{"ProjectionType": "ALL"},
		}},
		"StreamSpecification": map[string]any{
			"StreamEnabled":  true,
			"StreamViewType": "NEW_AND_OLD_IMAGES",
		},
		"BillingMode": "PAY_PER_REQUEST",
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateTable %d %s", create.Code, create.Body.String())
	}

	tag := mustDynamoJSON(t, handler, "TagResource", map[string]any{
		"ResourceArn": "arn:aws:dynamodb:us-east-1:" + testAccountID + ":table/query-lab",
		"Tags":        []map[string]any{{"Key": "env", "Value": "lab"}},
	}, now)
	if tag.Code != http.StatusOK {
		t.Fatalf("TagResource %d %s", tag.Code, tag.Body.String())
	}
	listTags := mustDynamoJSON(t, handler, "ListTagsOfResource", map[string]any{
		"ResourceArn": "arn:aws:dynamodb:us-east-1:" + testAccountID + ":table/query-lab",
	}, now)
	if listTags.Code != http.StatusOK || !strings.Contains(listTags.Body.String(), "env") {
		t.Fatalf("ListTagsOfResource %d %s", listTags.Code, listTags.Body.String())
	}

	ttl := mustDynamoJSON(t, handler, "UpdateTimeToLive", map[string]any{
		"TableName": "query-lab",
		"TimeToLiveSpecification": map[string]any{
			"Enabled":       true,
			"AttributeName": "expires",
		},
	}, now)
	if ttl.Code != http.StatusOK {
		t.Fatalf("UpdateTimeToLive %d %s", ttl.Code, ttl.Body.String())
	}
	descTTL := mustDynamoJSON(t, handler, "DescribeTimeToLive", map[string]any{"TableName": "query-lab"}, now)
	if descTTL.Code != http.StatusOK {
		t.Fatalf("DescribeTimeToLive %d %s", descTTL.Code, descTTL.Body.String())
	}

	backups := mustDynamoJSON(t, handler, "DescribeContinuousBackups", map[string]any{"TableName": "query-lab"}, now)
	if backups.Code != http.StatusOK {
		t.Fatalf("DescribeContinuousBackups %d %s", backups.Code, backups.Body.String())
	}

	for i, sk := range []string{"01", "02", "03"} {
		put := mustDynamoJSON(t, handler, "PutItem", map[string]any{
			"TableName": "query-lab",
			"Item": map[string]any{
				"pk":   map[string]any{"S": "user"},
				"sk":   map[string]any{"S": sk},
				"gsi1": map[string]any{"S": "g"},
				"kind": map[string]any{"S": "row"},
				"n":    map[string]any{"N": fmt.Sprintf("%d", i)},
			},
		}, now)
		if put.Code != http.StatusOK {
			t.Fatalf("PutItem %s %d %s", sk, put.Code, put.Body.String())
		}
	}

	condFail := mustDynamoJSON(t, handler, "PutItem", map[string]any{
		"TableName":           "query-lab",
		"ConditionExpression": "attribute_not_exists(pk)",
		"Item": map[string]any{
			"pk": map[string]any{"S": "user"},
			"sk": map[string]any{"S": "01"},
		},
	}, now)
	if condFail.Code != http.StatusBadRequest || !strings.Contains(condFail.Body.String(), "ConditionalCheckFailedException") {
		t.Fatalf("PutItem condition want ConditionalCheckFailed status=%d body=%q", condFail.Code, condFail.Body.String())
	}

	query := mustDynamoJSON(t, handler, "Query", map[string]any{
		"TableName":              "query-lab",
		"KeyConditionExpression": "pk = :pk",
		"FilterExpression":       "sk > :sk",
		"ProjectionExpression":   "pk, sk, kind",
		"ExpressionAttributeValues": map[string]any{
			":pk": map[string]any{"S": "user"},
			":sk": map[string]any{"S": "01"},
		},
		"ScanIndexForward": false,
		"Limit":            2,
	}, now)
	if query.Code != http.StatusOK {
		t.Fatalf("Query %d %s", query.Code, query.Body.String())
	}
	var queryOut map[string]any
	if err := json.Unmarshal(query.Body.Bytes(), &queryOut); err != nil {
		t.Fatal(err)
	}
	if queryOut["LastEvaluatedKey"] == nil {
		t.Fatalf("expected LastEvaluatedKey for Limit=2: %v", queryOut)
	}

	queryGSI := mustDynamoJSON(t, handler, "Query", map[string]any{
		"TableName":              "query-lab",
		"IndexName":              "GSI1",
		"KeyConditionExpression": "gsi1 = :g",
		"ExpressionAttributeValues": map[string]any{
			":g": map[string]any{"S": "g"},
		},
	}, now)
	if queryGSI.Code != http.StatusOK {
		t.Fatalf("Query GSI %d %s", queryGSI.Code, queryGSI.Body.String())
	}

	badQuery := mustDynamoJSON(t, handler, "Query", map[string]any{
		"TableName": "query-lab",
	}, now)
	if badQuery.Code != http.StatusBadRequest {
		t.Fatalf("Query missing key condition want 400 status=%d body=%q", badQuery.Code, badQuery.Body.String())
	}

	scan := mustDynamoJSON(t, handler, "Scan", map[string]any{
		"TableName":        "query-lab",
		"FilterExpression": "kind = :k",
		"ExpressionAttributeValues": map[string]any{
			":k": map[string]any{"S": "row"},
		},
		"Limit": 1,
	}, now)
	if scan.Code != http.StatusOK {
		t.Fatalf("Scan %d %s", scan.Code, scan.Body.String())
	}

	get := mustDynamoJSON(t, handler, "GetItem", map[string]any{
		"TableName":            "query-lab",
		"Key":                  map[string]any{"pk": map[string]any{"S": "user"}, "sk": map[string]any{"S": "01"}},
		"ConsistentRead":       true,
		"ProjectionExpression": "pk",
	}, now)
	if get.Code != http.StatusOK {
		t.Fatalf("GetItem %d %s", get.Code, get.Body.String())
	}

	txGet := mustDynamoJSON(t, handler, "TransactGetItems", map[string]any{
		"TransactItems": []map[string]any{
			{"Get": map[string]any{
				"TableName": "query-lab",
				"Key": map[string]any{
					"pk": map[string]any{"S": "user"},
					"sk": map[string]any{"S": "02"},
				},
			}},
		},
	}, now)
	if txGet.Code != http.StatusOK {
		t.Fatalf("TransactGetItems %d %s", txGet.Code, txGet.Body.String())
	}

	delCond := mustDynamoJSON(t, handler, "DeleteItem", map[string]any{
		"TableName":           "query-lab",
		"Key":                 map[string]any{"pk": map[string]any{"S": "user"}, "sk": map[string]any{"S": "03"}},
		"ConditionExpression": "attribute_exists(sk)",
	}, now)
	if delCond.Code != http.StatusOK {
		t.Fatalf("DeleteItem %d %s", delCond.Code, delCond.Body.String())
	}

	delMissing := mustDynamoJSON(t, handler, "DeleteItem", map[string]any{
		"TableName":           "query-lab",
		"Key":                 map[string]any{"pk": map[string]any{"S": "user"}, "sk": map[string]any{"S": "99"}},
		"ConditionExpression": "attribute_exists(sk)",
	}, now)
	if delMissing.Code != http.StatusBadRequest {
		t.Fatalf("DeleteItem missing condition want 400 status=%d body=%q", delMissing.Code, delMissing.Body.String())
	}

	untag := mustDynamoJSON(t, handler, "UntagResource", map[string]any{
		"ResourceArn": "arn:aws:dynamodb:us-east-1:" + testAccountID + ":table/query-lab",
		"TagKeys":     []string{"env"},
	}, now)
	if untag.Code != http.StatusOK {
		t.Fatalf("UntagResource %d %s", untag.Code, untag.Body.String())
	}

	// cleanup items for DeleteTable
	scanAll := mustDynamoJSON(t, handler, "Scan", map[string]any{"TableName": "query-lab"}, now)
	var scanOut map[string]any
	_ = json.Unmarshal(scanAll.Body.Bytes(), &scanOut)
	if items, ok := scanOut["Items"].([]any); ok {
		for _, it := range items {
			m, _ := it.(map[string]any)
			mustDynamoJSON(t, handler, "DeleteItem", map[string]any{
				"TableName": "query-lab",
				"Key": map[string]any{
					"pk": m["pk"],
					"sk": m["sk"],
				},
			}, now)
		}
	}
	delTable := mustDynamoJSON(t, handler, "DeleteTable", map[string]any{"TableName": "query-lab"}, now)
	if delTable.Code != http.StatusOK {
		t.Fatalf("DeleteTable %d %s", delTable.Code, delTable.Body.String())
	}
}
