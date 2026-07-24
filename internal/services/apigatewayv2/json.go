package apigatewayv2

import (
	"encoding/json"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// CreateApiJSON builds CreateApi / UpdateApi / GetApi response (restJson1 camelCase).
func CreateApiJSON(a store.APIGatewayAPI) ([]byte, error) {
	m := map[string]any{
		"apiId":        a.APIID,
		"name":         a.Name,
		"protocolType": a.ProtocolType,
		"apiEndpoint":  a.APIEndpoint,
		"createdDate":  createdDateISO8601(a.CreatedAt),
	}
	if a.CORS.HasCORS() {
		m["corsConfiguration"] = corsConfigurationMap(a.CORS)
	}
	return json.Marshal(m)
}

func createdDateISO8601(createdAt int64) string {
	// Store uses Unix millis; fall back if already seconds-sized.
	sec := createdAt
	if createdAt > 1_000_000_000_000 {
		sec = createdAt / 1000
	}
	return time.Unix(sec, 0).UTC().Format(time.RFC3339)
}

func corsConfigurationMap(c store.APIGatewayCORS) map[string]any {
	m := map[string]any{}
	if len(c.AllowOrigins) > 0 {
		m["allowOrigins"] = c.AllowOrigins
	}
	if len(c.AllowMethods) > 0 {
		m["allowMethods"] = c.AllowMethods
	}
	if len(c.AllowHeaders) > 0 {
		m["allowHeaders"] = c.AllowHeaders
	}
	if len(c.ExposeHeaders) > 0 {
		m["exposeHeaders"] = c.ExposeHeaders
	}
	if c.MaxAge > 0 {
		m["maxAge"] = c.MaxAge
	}
	if c.AllowCredentials {
		m["allowCredentials"] = true
	}
	return m
}

// GetApiJSON builds GetApi response.
func GetApiJSON(a store.APIGatewayAPI) ([]byte, error) {
	return CreateApiJSON(a)
}

// DeleteApiJSON is an empty OK body.
func DeleteApiJSON() ([]byte, error) { return []byte(`{}`), nil }

// GetApisJSON builds GetApis / ListApis response.
func GetApisJSON(apis []store.APIGatewayAPI) ([]byte, error) {
	items := make([]map[string]any, 0, len(apis))
	for _, a := range apis {
		items = append(items, map[string]any{
			"apiId":        a.APIID,
			"name":         a.Name,
			"protocolType": a.ProtocolType,
			"apiEndpoint":  a.APIEndpoint,
		})
	}
	return json.Marshal(map[string]any{"items": items})
}

// CreateIntegrationJSON builds CreateIntegration response.
func CreateIntegrationJSON(in store.APIGatewayIntegration) ([]byte, error) {
	m := map[string]any{
		"integrationId":        in.IntegrationID,
		"integrationType":      in.IntegrationType,
		"integrationUri":       in.IntegrationURI,
		"payloadFormatVersion": in.PayloadFormatVersion,
		"apiId":                in.APIID,
	}
	if in.CredentialsArn != "" {
		m["credentialsArn"] = in.CredentialsArn
	}
	return json.Marshal(m)
}

// CreateAuthorizerJSON builds CreateAuthorizer response.
func CreateAuthorizerJSON(a store.APIGatewayAuthorizer) ([]byte, error) {
	m := map[string]any{
		"authorizerId":   a.AuthorizerID,
		"name":           a.Name,
		"authorizerType": a.AuthorizerType,
		"identitySource": []string{a.IdentitySource},
		"apiId":          a.APIID,
	}
	switch a.AuthorizerType {
	case store.APIGatewayAuthorizerJWT:
		m["jwtConfiguration"] = map[string]any{
			"issuer":   a.JWTIssuer,
			"audience": a.JWTAudience,
		}
	case store.APIGatewayAuthorizerREQUEST:
		m["authorizerUri"] = a.AuthorizerURI
		m["authorizerPayloadFormatVersion"] = a.AuthorizerPayloadFormatVersion
		m["enableSimpleResponses"] = a.EnableSimpleResponses
		if a.AuthorizerCredentialsArn != "" {
			m["authorizerCredentialsArn"] = a.AuthorizerCredentialsArn
		}
	}
	return json.Marshal(m)
}

// CreateRouteJSON builds CreateRoute response.
func CreateRouteJSON(r store.APIGatewayRoute) ([]byte, error) {
	m := map[string]any{
		"routeId":           r.RouteID,
		"routeKey":          r.RouteKey,
		"target":            r.Target,
		"authorizationType": r.AuthorizationType,
		"apiId":             r.APIID,
	}
	if r.AuthorizerID != "" {
		m["authorizerId"] = r.AuthorizerID
	}
	return json.Marshal(m)
}

// CreateStageJSON builds CreateStage response.
func CreateStageJSON(st store.APIGatewayStage) ([]byte, error) {
	return json.Marshal(map[string]any{
		"stageName":  st.StageName,
		"apiId":      st.APIID,
		"autoDeploy": st.AutoDeploy,
	})
}

// GetIntegrationsJSON builds GetIntegrations response.
func GetIntegrationsJSON(items []store.APIGatewayIntegration) ([]byte, error) {
	out := make([]map[string]any, 0, len(items))
	for _, in := range items {
		m := map[string]any{
			"integrationId":        in.IntegrationID,
			"integrationType":      in.IntegrationType,
			"integrationUri":       in.IntegrationURI,
			"payloadFormatVersion": in.PayloadFormatVersion,
			"apiId":                in.APIID,
		}
		if in.CredentialsArn != "" {
			m["credentialsArn"] = in.CredentialsArn
		}
		out = append(out, m)
	}
	return json.Marshal(map[string]any{"items": out})
}

// GetRoutesJSON builds GetRoutes response.
func GetRoutesJSON(items []store.APIGatewayRoute) ([]byte, error) {
	out := make([]map[string]any, 0, len(items))
	for _, r := range items {
		m := map[string]any{
			"routeId":           r.RouteID,
			"routeKey":          r.RouteKey,
			"target":            r.Target,
			"authorizationType": r.AuthorizationType,
			"apiId":             r.APIID,
		}
		if r.AuthorizerID != "" {
			m["authorizerId"] = r.AuthorizerID
		}
		out = append(out, m)
	}
	return json.Marshal(map[string]any{"items": out})
}

// GetAuthorizersJSON builds GetAuthorizers response.
func GetAuthorizersJSON(items []store.APIGatewayAuthorizer) ([]byte, error) {
	out := make([]map[string]any, 0, len(items))
	for _, a := range items {
		raw, err := CreateAuthorizerJSON(a)
		if err != nil {
			return nil, err
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return json.Marshal(map[string]any{"items": out})
}
