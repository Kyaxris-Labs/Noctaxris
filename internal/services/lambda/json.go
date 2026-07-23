package lambda

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

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
		"PackageType":  fn.PackageType,
	}
	if fn.PackageType == "" {
		cfg["PackageType"] = store.LambdaPackageTypeZip
	}
	if len(fn.Env) > 0 {
		cfg["Environment"] = map[string]any{"Variables": fn.Env}
	}
	if len(fn.Layers) > 0 {
		cfg["Layers"] = append([]string(nil), fn.Layers...)
	}
	if strings.TrimSpace(fn.DeadLetterTargetArn) != "" {
		cfg["DeadLetterConfig"] = map[string]any{"TargetArn": fn.DeadLetterTargetArn}
	}
	destCfg := map[string]any{}
	if strings.TrimSpace(fn.DestinationOnFailureArn) != "" {
		destCfg["OnFailure"] = map[string]any{"Destination": fn.DestinationOnFailureArn}
	}
	if strings.TrimSpace(fn.DestinationOnSuccessArn) != "" {
		destCfg["OnSuccess"] = map[string]any{"Destination": fn.DestinationOnSuccessArn}
	}
	if len(destCfg) > 0 {
		cfg["DestinationConfig"] = destCfg
	}
	return cfg
}

// EventInvokeConfigJSON builds Put/GetFunctionEventInvokeConfig success body.
func EventInvokeConfigJSON(fn store.LambdaFunction) ([]byte, error) {
	out := map[string]any{
		"FunctionArn": fn.FunctionARN,
	}
	destCfg := map[string]any{}
	if strings.TrimSpace(fn.DestinationOnFailureArn) != "" {
		destCfg["OnFailure"] = map[string]any{"Destination": fn.DestinationOnFailureArn}
	}
	if strings.TrimSpace(fn.DestinationOnSuccessArn) != "" {
		destCfg["OnSuccess"] = map[string]any{"Destination": fn.DestinationOnSuccessArn}
	}
	if len(destCfg) > 0 {
		out["DestinationConfig"] = destCfg
	}
	return json.Marshal(out)
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
	code := map[string]any{
		"RepositoryType": "S3",
		"Location":       "",
	}
	if fn.PackageType == store.LambdaPackageTypeImage {
		code["ImageUri"] = fn.ImageURI
		code["RepositoryType"] = "ECR"
	}
	return json.Marshal(map[string]any{
		"Configuration": functionConfiguration(fn),
		"Code":          code,
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
func InvokeJSON(payload []byte, statusCode int, executedVersion string) ([]byte, error) {
	if statusCode == 0 {
		statusCode = 200
	}
	if executedVersion == "" {
		executedVersion = "$LATEST"
	}
	return json.Marshal(map[string]any{
		"StatusCode":      statusCode,
		"ExecutedVersion": executedVersion,
		"Payload":         base64.StdEncoding.EncodeToString(payload),
	})
}

// InvokeAsyncAcceptedJSON builds an async Invoke (InvocationType=Event) success body.
func InvokeAsyncAcceptedJSON(executedVersion string) ([]byte, error) {
	if executedVersion == "" {
		executedVersion = "$LATEST"
	}
	return json.Marshal(map[string]any{
		"StatusCode":      202,
		"ExecutedVersion": executedVersion,
	})
}

// functionConfigurationWithVersion maps store metadata to AWS FunctionConfiguration fields.
func functionConfigurationWithVersion(fn store.LambdaFunction, version string) map[string]any {
	cfg := functionConfiguration(fn)
	cfg["Version"] = version
	return cfg
}

// PublishVersionJSON builds a PublishVersion success body.
func PublishVersionJSON(v store.LambdaFunctionVersion) ([]byte, error) {
	cfg := functionConfiguration(v.LambdaFunction)
	cfg["Version"] = fmt.Sprintf("%d", v.Version)
	return json.Marshal(cfg)
}

// ListVersionsByFunctionJSON builds a ListVersionsByFunction success body.
// AWS always includes $LATEST first, then published numeric versions.
func ListVersionsByFunctionJSON(latest store.LambdaFunction, versions []store.LambdaFunctionVersion) ([]byte, error) {
	out := make([]map[string]any, 0, len(versions)+1)
	out = append(out, functionConfiguration(latest))
	for _, v := range versions {
		cfg := functionConfiguration(v.LambdaFunction)
		cfg["Version"] = fmt.Sprintf("%d", v.Version)
		out = append(out, cfg)
	}
	return json.Marshal(map[string]any{"Versions": out})
}

// GetFunctionQualifiedJSON builds a GetFunction success body with a version qualifier.
func GetFunctionQualifiedJSON(q store.QualifiedFunction) ([]byte, error) {
	code := map[string]any{
		"RepositoryType": "S3",
		"Location":       "",
	}
	if q.PackageType == store.LambdaPackageTypeImage {
		code["ImageUri"] = q.ImageURI
		code["RepositoryType"] = "ECR"
	}
	return json.Marshal(map[string]any{
		"Configuration": functionConfigurationWithVersion(q.LambdaFunction, q.Version),
		"Code":          code,
	})
}

func aliasConfiguration(a store.LambdaAlias) map[string]any {
	return map[string]any{
		"AliasArn":        a.AliasARN,
		"Name":            a.AliasName,
		"FunctionVersion": fmt.Sprintf("%d", a.FunctionVersion),
		"Description":     a.Description,
		"RevisionId":      a.RevisionID,
	}
}

// CreateAliasJSON builds a CreateAlias / UpdateAlias success body.
func CreateAliasJSON(a store.LambdaAlias) ([]byte, error) {
	return json.Marshal(aliasConfiguration(a))
}

// GetAliasJSON builds a GetAlias success body.
func GetAliasJSON(a store.LambdaAlias) ([]byte, error) {
	return json.Marshal(aliasConfiguration(a))
}

// ListAliasesJSON builds a ListAliases success body.
func ListAliasesJSON(aliases []store.LambdaAlias) ([]byte, error) {
	out := make([]map[string]any, 0, len(aliases))
	for _, a := range aliases {
		out = append(out, aliasConfiguration(a))
	}
	return json.Marshal(map[string]any{"Aliases": out})
}

// EmptyOKJSON returns an empty JSON object for DeleteFunction.
func EmptyOKJSON() ([]byte, error) {
	return []byte("{}"), nil
}

// ListTagsJSON builds a ListTags response (string-to-string Tags map).
func ListTagsJSON(tags map[string]string) ([]byte, error) {
	if tags == nil {
		tags = map[string]string{}
	}
	return json.Marshal(map[string]any{"Tags": tags})
}

// AddPermissionJSON builds an AddPermission success body.
func AddPermissionJSON(statement string) ([]byte, error) {
	return json.Marshal(map[string]string{"Statement": statement})
}

// GetPolicyJSON builds a GetPolicy success body.
func GetPolicyJSON(policy string) ([]byte, error) {
	return json.Marshal(map[string]string{"Policy": policy})
}

func layerVersionConfiguration(layer store.LambdaLayer) map[string]any {
	return map[string]any{
		"LayerArn":               layer.LayerARN,
		"LayerVersionArn":        layer.LayerARN,
		"Description":            layer.Description,
		"CreatedDate":              layer.PublishedAt,
		"Version":                  layer.Version,
		"CompatibleRuntimes":       store.SupportedLambdaRuntimes(),
		"CompatibleArchitectures":  []string{"x86_64"},
		"LicenseInfo":              "",
	}
}

// PublishLayerVersionJSON builds a PublishLayerVersion success body.
func PublishLayerVersionJSON(layer store.LambdaLayer) ([]byte, error) {
	cfg := layerVersionConfiguration(layer)
	cfg["Content"] = map[string]any{
		"CodeSha256": codeSHA256AWS(layer.CodeSHA256),
	}
	return json.Marshal(cfg)
}

// GetLayerVersionJSON builds a GetLayerVersion success body.
func GetLayerVersionJSON(layer store.LambdaLayer) ([]byte, error) {
	cfg := layerVersionConfiguration(layer)
	cfg["Content"] = map[string]any{
		"CodeSha256": codeSHA256AWS(layer.CodeSHA256),
	}
	return json.Marshal(cfg)
}

// ListLayerVersionsJSON builds a ListLayerVersions success body.
func ListLayerVersionsJSON(layers []store.LambdaLayer) ([]byte, error) {
	out := make([]map[string]any, 0, len(layers))
	for _, layer := range layers {
		out = append(out, layerVersionConfiguration(layer))
	}
	return json.Marshal(map[string]any{"LayerVersions": out})
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

func eventSourceMappingConfiguration(m store.LambdaEventSourceMapping) map[string]any {
	out := map[string]any{
		"UUID":           m.UUID,
		"FunctionArn":    m.FunctionARN,
		"EventSourceArn": m.EventSourceARN,
		"BatchSize":      m.BatchSize,
		"State":          m.State,
		"LastModified":   m.LastModified,
	}
	if q := strings.TrimSpace(m.Qualifier); q != "" && q != "$LATEST" {
		out["Qualifier"] = q
	}
	if strings.TrimSpace(m.FilterCriteriaJSON) != "" {
		var fc any
		if err := json.Unmarshal([]byte(m.FilterCriteriaJSON), &fc); err == nil {
			out["FilterCriteria"] = fc
		}
	}
	if strings.TrimSpace(m.FunctionResponseTypesJSON) != "" {
		var rt any
		if err := json.Unmarshal([]byte(m.FunctionResponseTypesJSON), &rt); err == nil {
			out["FunctionResponseTypes"] = rt
		}
	}
	return out
}

// EventSourceMappingJSON builds Create/Get/UpdateEventSourceMapping success body.
func EventSourceMappingJSON(m store.LambdaEventSourceMapping) ([]byte, error) {
	return json.Marshal(eventSourceMappingConfiguration(m))
}

// ListEventSourceMappingsJSON builds ListEventSourceMappings success body.
func ListEventSourceMappingsJSON(mappings []store.LambdaEventSourceMapping) ([]byte, error) {
	out := make([]map[string]any, 0, len(mappings))
	for _, m := range mappings {
		out = append(out, eventSourceMappingConfiguration(m))
	}
	return json.Marshal(map[string]any{"EventSourceMappings": out})
}

func functionURLConfiguration(u store.LambdaFunctionURL) map[string]any {
	m := map[string]any{
		"FunctionUrl":  u.FunctionURL,
		"FunctionArn":  u.FunctionARN,
		"AuthType":     u.AuthType,
		"CreationTime": u.CreationTime,
	}
	if len(u.CorsAllowOrigins) > 0 {
		m["Cors"] = map[string]any{"AllowOrigins": u.CorsAllowOrigins}
	}
	return m
}

// FunctionURLConfigJSON builds Create/GetFunctionUrlConfig success body.
func FunctionURLConfigJSON(u store.LambdaFunctionURL) ([]byte, error) {
	return json.Marshal(functionURLConfiguration(u))
}

// ListFunctionURLConfigsJSON builds ListFunctionUrlConfigs success body.
func ListFunctionURLConfigsJSON(urls []store.LambdaFunctionURL) ([]byte, error) {
	out := make([]map[string]any, 0, len(urls))
	for _, u := range urls {
		out = append(out, functionURLConfiguration(u))
	}
	return json.Marshal(map[string]any{"FunctionUrlConfigs": out})
}
