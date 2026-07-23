package apigatewayv2

import (
	"encoding/json"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// CreateApiJSON builds CreateApi response.
func CreateApiJSON(a store.APIGatewayAPI) ([]byte, error) {
	return json.Marshal(map[string]any{
		"ApiId":        a.APIID,
		"Name":         a.Name,
		"ProtocolType": a.ProtocolType,
		"ApiEndpoint":  a.APIEndpoint,
		"CreatedDate":  a.CreatedAt / 1000,
	})
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
			"ApiId":        a.APIID,
			"Name":         a.Name,
			"ProtocolType": a.ProtocolType,
			"ApiEndpoint":  a.APIEndpoint,
		})
	}
	return json.Marshal(map[string]any{"Items": items})
}

// CreateIntegrationJSON builds CreateIntegration response.
func CreateIntegrationJSON(in store.APIGatewayIntegration) ([]byte, error) {
	m := map[string]any{
		"IntegrationId":        in.IntegrationID,
		"IntegrationType":      in.IntegrationType,
		"IntegrationUri":       in.IntegrationURI,
		"PayloadFormatVersion": in.PayloadFormatVersion,
		"ApiId":                in.APIID,
	}
	if in.CredentialsArn != "" {
		m["CredentialsArn"] = in.CredentialsArn
	}
	return json.Marshal(m)
}

// CreateAuthorizerJSON builds CreateAuthorizer response.
func CreateAuthorizerJSON(a store.APIGatewayAuthorizer) ([]byte, error) {
	m := map[string]any{
		"AuthorizerId":   a.AuthorizerID,
		"Name":           a.Name,
		"AuthorizerType": a.AuthorizerType,
		"IdentitySource": []string{a.IdentitySource},
		"ApiId":          a.APIID,
	}
	switch a.AuthorizerType {
	case store.APIGatewayAuthorizerJWT:
		m["JwtConfiguration"] = map[string]any{
			"Issuer":   a.JWTIssuer,
			"Audience": a.JWTAudience,
		}
	case store.APIGatewayAuthorizerREQUEST:
		m["AuthorizerUri"] = a.AuthorizerURI
		m["AuthorizerPayloadFormatVersion"] = a.AuthorizerPayloadFormatVersion
		m["EnableSimpleResponses"] = a.EnableSimpleResponses
		if a.AuthorizerCredentialsArn != "" {
			m["AuthorizerCredentialsArn"] = a.AuthorizerCredentialsArn
		}
	}
	return json.Marshal(m)
}

// CreateRouteJSON builds CreateRoute response.
func CreateRouteJSON(r store.APIGatewayRoute) ([]byte, error) {
	m := map[string]any{
		"RouteId":           r.RouteID,
		"RouteKey":          r.RouteKey,
		"Target":            r.Target,
		"AuthorizationType": r.AuthorizationType,
		"ApiId":             r.APIID,
	}
	if r.AuthorizerID != "" {
		m["AuthorizerId"] = r.AuthorizerID
	}
	return json.Marshal(m)
}

// CreateStageJSON builds CreateStage response.
func CreateStageJSON(st store.APIGatewayStage) ([]byte, error) {
	return json.Marshal(map[string]any{
		"StageName":  st.StageName,
		"ApiId":      st.APIID,
		"AutoDeploy": st.AutoDeploy,
	})
}
