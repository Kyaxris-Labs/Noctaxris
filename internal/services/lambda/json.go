package lambda

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// functionConfiguration maps store metadata to AWS FunctionConfiguration fields.
func functionConfiguration(fn store.LambdaFunction) map[string]any {
	cfg := map[string]any{
		"FunctionName": fn.FunctionName,
		"FunctionArn":  fn.FunctionARN,
		"Role":         fn.RoleARN,
		"Runtime":      fn.Runtime,
		"Handler":      fn.Handler,
		"Timeout":      fn.Timeout,
		"MemorySize":   fn.Memory,
		"CodeSha256":   codeSHA256AWS(fn.CodeSHA256),
		"State":        fn.State,
		"LastModified": fn.LastModified,
		"Description":  fn.Description,
		"Version":      "$LATEST",
		"PackageType":  "Zip",
	}
	if len(fn.Env) > 0 {
		cfg["Environment"] = map[string]any{"Variables": fn.Env}
	}
	return cfg
}

// codeSHA256AWS converts store hex digest to AWS base64 CodeSha256.
func codeSHA256AWS(hexDigest string) string {
	raw, err := hex.DecodeString(hexDigest)
	if err != nil || len(raw) == 0 {
		return hexDigest
	}
	return base64.StdEncoding.EncodeToString(raw)
}

// CreateFunctionJSON builds a CreateFunction / UpdateFunction* success body.
func CreateFunctionJSON(fn store.LambdaFunction) ([]byte, error) {
	return json.Marshal(functionConfiguration(fn))
}

// GetFunctionJSON builds a GetFunction success body.
func GetFunctionJSON(fn store.LambdaFunction) ([]byte, error) {
	return json.Marshal(map[string]any{
		"Configuration": functionConfiguration(fn),
		"Code": map[string]any{
			"RepositoryType": "S3",
			"Location":       "",
		},
	})
}

// ListFunctionsJSON builds a ListFunctions success body.
func ListFunctionsJSON(fns []store.LambdaFunction) ([]byte, error) {
	out := make([]map[string]any, 0, len(fns))
	for _, fn := range fns {
		out = append(out, functionConfiguration(fn))
	}
	return json.Marshal(map[string]any{"Functions": out})
}

// InvokeJSON builds a sync Invoke success body (Payload is base64).
func InvokeJSON(payload []byte, statusCode int) ([]byte, error) {
	if statusCode == 0 {
		statusCode = 200
	}
	return json.Marshal(map[string]any{
		"StatusCode":      statusCode,
		"ExecutedVersion": "$LATEST",
		"Payload":         base64.StdEncoding.EncodeToString(payload),
	})
}

// EmptyOKJSON returns an empty JSON object for DeleteFunction.
func EmptyOKJSON() ([]byte, error) {
	return []byte("{}"), nil
}

// DecodeZipFile decodes a CreateFunction / UpdateFunctionCode ZipFile field.
func DecodeZipFile(v any) ([]byte, error) {
	switch t := v.(type) {
	case string:
		if t == "" {
			return nil, fmt.Errorf("empty ZipFile")
		}
		raw, err := base64.StdEncoding.DecodeString(t)
		if err != nil {
			return nil, fmt.Errorf("decode ZipFile: %w", err)
		}
		return raw, nil
	case []byte:
		if len(t) == 0 {
			return nil, fmt.Errorf("empty ZipFile")
		}
		return t, nil
	default:
		return nil, fmt.Errorf("ZipFile must be base64 string")
	}
}
