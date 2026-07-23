package dynamodb

import "github.com/Kyaxris-Labs/Noctaxris/internal/store"

func init() {
	// Store cannot import this package (cycle via DynamoTable helpers). Register
	// expression evaluators so TransactWriteItems can reuse the lab subset.
	store.RegisterTransactExprFns(
		func(item store.DynamoAVMap, expr string, names, values map[string]any) error {
			return EvaluateConditionExpression(ItemMap(item), expr, names, values)
		},
		func(item, key store.DynamoAVMap, updateExpr string, names, values map[string]any) (store.DynamoAVMap, error) {
			out, err := ApplyUpdateExpression(ItemMap(item), ItemMap(key), updateExpr, names, values)
			if err != nil {
				return nil, err
			}
			return store.DynamoAVMap(out), nil
		},
	)
}
