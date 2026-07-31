package store

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

func marshalJSONColumn[T any](v T) (string, error) {
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Map, reflect.Slice:
		if rv.IsNil() || rv.Len() == 0 {
			if rv.Kind() == reflect.Map {
				return "{}", nil
			}
			return "[]", nil
		}
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("marshal json column: %w", err)
	}
	return string(raw), nil
}

func unmarshalJSONColumn[T any](s string, out *T) error {
	if out == nil {
		return fmt.Errorf("unmarshal json column: nil out")
	}
	if strings.TrimSpace(s) == "" {
		var zero T
		*out = zero
		return nil
	}
	if err := json.Unmarshal([]byte(s), out); err != nil {
		return fmt.Errorf("unmarshal json column: %w", err)
	}
	return nil
}
