package dynamodb_test

import (
	"errors"
	"testing"

	ddb "github.com/Kyaxris-Labs/Noctaxris/internal/services/dynamodb"
)

func TestEvaluateConditionAttributeNotExists(t *testing.T) {
	err := ddb.EvaluateConditionExpression(nil, "attribute_not_exists(pk)", nil, nil)
	if err != nil {
		t.Fatalf("empty item: %v", err)
	}
	err = ddb.EvaluateConditionExpression(ddb.ItemMap{
		"pk": {"S": "a"},
	}, "attribute_not_exists(pk)", nil, nil)
	if !errors.Is(err, ddb.ErrConditionalCheckFailed) {
		t.Fatalf("existing item: got %v, want ConditionalCheckFailed", err)
	}
}

func TestEvaluateConditionComparison(t *testing.T) {
	item := ddb.ItemMap{"version": {"N": "3"}}
	values := map[string]any{":v": map[string]any{"N": "3"}}
	if err := ddb.EvaluateConditionExpression(item, "version = :v", nil, values); err != nil {
		t.Fatalf("equal: %v", err)
	}
	values[":v"] = map[string]any{"N": "4"}
	if err := ddb.EvaluateConditionExpression(item, "version = :v", nil, values); !errors.Is(err, ddb.ErrConditionalCheckFailed) {
		t.Fatalf("not equal: got %v", err)
	}
	if err := ddb.EvaluateConditionExpression(item, "version < :v", nil, values); err != nil {
		t.Fatalf("less than: %v", err)
	}
}

func TestEvaluateConditionANDOR(t *testing.T) {
	item := ddb.ItemMap{
		"pk":  {"S": "a"},
		"ver": {"N": "1"},
	}
	names := map[string]any{"#v": "ver"}
	values := map[string]any{":one": map[string]any{"N": "1"}}
	err := ddb.EvaluateConditionExpression(item, "attribute_exists(pk) AND #v = :one", names, values)
	if err != nil {
		t.Fatalf("and: %v", err)
	}
	err = ddb.EvaluateConditionExpression(item, "attribute_not_exists(pk) OR #v = :one", names, values)
	if err != nil {
		t.Fatalf("or: %v", err)
	}
}

func TestEvaluateConditionUnsupportedHardFails(t *testing.T) {
	err := ddb.EvaluateConditionExpression(nil, "size(tags) > :n", nil, map[string]any{
		":n": map[string]any{"N": "1"},
	})
	if err == nil || errors.Is(err, ddb.ErrConditionalCheckFailed) {
		t.Fatalf("unsupported must Validation-fail, got %v", err)
	}
}
