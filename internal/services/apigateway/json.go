package apigateway

import (
	"encoding/json"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func createdDateUnix(createdAt int64) float64 {
	sec := createdAt
	if createdAt > 1_000_000_000_000 {
		sec = createdAt / 1000
	}
	return float64(sec)
}

// RestApiJSON builds CreateRestApi / GetRestApi response.
func RestApiJSON(a store.RestAPI) ([]byte, error) {
	return json.Marshal(map[string]any{
		"id":             a.APIID,
		"name":           a.Name,
		"description":    a.Description,
		"createdDate":    createdDateUnix(a.CreatedAt),
		"rootResourceId": a.RootResourceID,
	})
}

// RestApisJSON builds GetRestApis response.
func RestApisJSON(apis []store.RestAPI) ([]byte, error) {
	items := make([]map[string]any, 0, len(apis))
	for _, a := range apis {
		items = append(items, map[string]any{
			"id":             a.APIID,
			"name":           a.Name,
			"description":    a.Description,
			"createdDate":    createdDateUnix(a.CreatedAt),
			"rootResourceId": a.RootResourceID,
		})
	}
	return json.Marshal(map[string]any{"item": items})
}

// ResourceJSON builds CreateResource / resource item response.
func ResourceJSON(r store.RestResource) ([]byte, error) {
	m := map[string]any{
		"id":   r.ResourceID,
		"path": r.Path,
	}
	if r.ParentID != "" {
		m["parentId"] = r.ParentID
	}
	if r.PathPart != "" {
		m["pathPart"] = r.PathPart
	}
	return json.Marshal(m)
}

// ResourcesJSON builds GetResources response.
func ResourcesJSON(items []store.RestResource) ([]byte, error) {
	out := make([]map[string]any, 0, len(items))
	for _, r := range items {
		m := map[string]any{
			"id":   r.ResourceID,
			"path": r.Path,
		}
		if r.ParentID != "" {
			m["parentId"] = r.ParentID
		}
		if r.PathPart != "" {
			m["pathPart"] = r.PathPart
		}
		out = append(out, m)
	}
	return json.Marshal(map[string]any{"item": out})
}

// MethodJSON builds PutMethod / GetMethod response.
func MethodJSON(m store.RestMethod) ([]byte, error) {
	return json.Marshal(map[string]any{
		"httpMethod":        m.HTTPMethod,
		"authorizationType": m.AuthorizationType,
		"apiKeyRequired":    m.APIKeyRequired,
		"authorizerId":      m.AuthorizerID,
	})
}

// IntegrationJSON builds PutIntegration / GetIntegration response.
func IntegrationJSON(in store.RestIntegration) ([]byte, error) {
	m := map[string]any{
		"type":                  in.Type,
		"uri":                   in.URI,
		"httpMethod":            in.IntegrationHTTPMethod,
		"passthroughBehavior":   "WHEN_NO_MATCH",
		"timeoutInMillis":       29000,
		"cacheNamespace":        in.ResourceID,
		"cacheKeyParameters":    []string{},
	}
	if in.Credentials != "" {
		m["credentials"] = in.Credentials
	}
	if len(in.RequestTemplates) > 0 {
		m["requestTemplates"] = in.RequestTemplates
	}
	return json.Marshal(m)
}

// DeploymentJSON builds CreateDeployment response.
func DeploymentJSON(d store.RestDeployment) ([]byte, error) {
	return json.Marshal(map[string]any{
		"id":          d.DeploymentID,
		"description": d.Description,
		"createdDate": createdDateUnix(d.CreatedAt),
	})
}

// StageJSON builds CreateStage / GetStage response.
func StageJSON(st store.RestStage) ([]byte, error) {
	return json.Marshal(map[string]any{
		"stageName":    st.StageName,
		"deploymentId": st.DeploymentID,
		"createdDate":  createdDateUnix(st.CreatedAt),
	})
}

// AuthorizerJSON builds CreateAuthorizer / GetAuthorizer response.
func AuthorizerJSON(a store.RestAuthorizer) ([]byte, error) {
	return json.Marshal(map[string]any{
		"id":                    a.AuthorizerID,
		"name":                  a.Name,
		"type":                  a.Type,
		"authorizerUri":         a.AuthorizerURI,
		"identitySource":        a.IdentitySource,
		"authorizerCredentials": a.AuthorizerCredentials,
	})
}

// AuthorizersJSON builds GetAuthorizers response.
func AuthorizersJSON(items []store.RestAuthorizer) ([]byte, error) {
	out := make([]map[string]any, 0, len(items))
	for _, a := range items {
		out = append(out, map[string]any{
			"id":                    a.AuthorizerID,
			"name":                  a.Name,
			"type":                  a.Type,
			"authorizerUri":         a.AuthorizerURI,
			"identitySource":        a.IdentitySource,
			"authorizerCredentials": a.AuthorizerCredentials,
		})
	}
	return json.Marshal(map[string]any{"item": out})
}

// ApiKeyJSON builds CreateApiKey / GetApiKey response. Value is omitted when empty.
func ApiKeyJSON(k store.RestAPIKey) ([]byte, error) {
	m := map[string]any{
		"id":          k.ID,
		"name":        k.Name,
		"enabled":     k.Enabled,
		"createdDate": createdDateUnix(k.CreatedAt),
	}
	if k.Value != "" {
		m["value"] = k.Value
	}
	return json.Marshal(m)
}

// ApiKeysJSON builds GetApiKeys response.
func ApiKeysJSON(keys []store.RestAPIKey) ([]byte, error) {
	items := make([]map[string]any, 0, len(keys))
	for _, k := range keys {
		items = append(items, map[string]any{
			"id":          k.ID,
			"name":        k.Name,
			"enabled":     k.Enabled,
			"createdDate": createdDateUnix(k.CreatedAt),
		})
	}
	return json.Marshal(map[string]any{"item": items})
}

// UsagePlanJSON builds CreateUsagePlan / GetUsagePlan response.
func UsagePlanJSON(p store.RestUsagePlan) ([]byte, error) {
	stages := make([]map[string]any, 0, len(p.APIStages))
	for _, st := range p.APIStages {
		stages = append(stages, map[string]any{"apiId": st.APIID, "stage": st.Stage})
	}
	return json.Marshal(map[string]any{
		"id":          p.ID,
		"name":        p.Name,
		"description": p.Description,
		"apiStages":   stages,
		"createdDate": createdDateUnix(p.CreatedAt),
	})
}

// UsagePlansJSON builds GetUsagePlans response.
func UsagePlansJSON(plans []store.RestUsagePlan) ([]byte, error) {
	items := make([]map[string]any, 0, len(plans))
	for _, p := range plans {
		stages := make([]map[string]any, 0, len(p.APIStages))
		for _, st := range p.APIStages {
			stages = append(stages, map[string]any{"apiId": st.APIID, "stage": st.Stage})
		}
		items = append(items, map[string]any{
			"id":          p.ID,
			"name":        p.Name,
			"description": p.Description,
			"apiStages":   stages,
			"createdDate": createdDateUnix(p.CreatedAt),
		})
	}
	return json.Marshal(map[string]any{"item": items})
}

// UsagePlanKeyJSON builds CreateUsagePlanKey response.
func UsagePlanKeyJSON(k store.RestUsagePlanKey) ([]byte, error) {
	return json.Marshal(map[string]any{
		"id":    k.ID,
		"type":  k.Type,
		"value": k.Value,
	})
}

// UsagePlanKeysJSON builds GetUsagePlanKeys response.
func UsagePlanKeysJSON(keys []store.RestUsagePlanKey) ([]byte, error) {
	items := make([]map[string]any, 0, len(keys))
	for _, k := range keys {
		items = append(items, map[string]any{
			"id":    k.ID,
			"type":  k.Type,
			"value": k.Value,
		})
	}
	return json.Marshal(map[string]any{"item": items})
}

