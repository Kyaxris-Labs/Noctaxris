package server

import (
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
)

func TestDynamoLimit(t *testing.T) {
	t.Parallel()
	if dynamoLimit(map[string]any{"Limit": float64(5)}) != 5 {
		t.Fatal("float64")
	}
	if dynamoLimit(map[string]any{"Limit": 3}) != 3 {
		t.Fatal("int")
	}
	if dynamoLimit(map[string]any{"Limit": float64(0)}) != 0 {
		t.Fatal("zero")
	}
	if dynamoLimit(map[string]any{}) != 0 {
		t.Fatal("missing")
	}
}

func TestDynamoLabAttributeExistsOnly(t *testing.T) {
	t.Parallel()
	if !dynamoLabAttributeExistsOnly("attribute_exists(pk)") {
		t.Fatal("simple")
	}
	if dynamoLabAttributeExistsOnly("attribute_not_exists(pk)") {
		t.Fatal("neg")
	}
	if dynamoLabAttributeExistsOnly("attribute_exists(pk") {
		t.Fatal("no close")
	}
}

func TestDynamoActionMapping(t *testing.T) {
	t.Parallel()
	if dynamoAction("PutItem") != catalog.ActionDynamoDBPutItem {
		t.Fatal("PutItem")
	}
	if dynamoAction("dynamodb:Scan") != "dynamodb:Scan" {
		t.Fatal("passthrough")
	}
	if dynamoAction("NoSuch") != "NoSuch" {
		t.Fatal("unknown passthrough")
	}
}

func TestDynamoActionKnownOps(t *testing.T) {
	t.Parallel()
	ops := []string{
		"CreateTable", "BatchGetItem", "TransactWriteItems", "ExecuteStatement",
	}
	for _, op := range ops {
		if dynamoAction(op) == "" {
			t.Fatalf("missing mapping for %s", op)
		}
	}
	if !strings.Contains(dynamoAction("Query"), "Query") {
		t.Fatal("Query")
	}
}
