package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// invokeHTTPAPILambdaAuthorizer calls the REQUEST authorizer Lambda and returns
// whether the request is allowed. Failures and Deny short-circuit with false.
func (s *Server) invokeHTTPAPILambdaAuthorizer(
	ctx context.Context,
	r *http.Request,
	accountID, apiID, stage, routePath, requestID string,
	route store.APIGatewayRoute,
	authzRow store.APIGatewayAuthorizer,
) (allowed bool, errMsg string) {
	region := store.DefaultAPIGatewayRegion
	fnAccount, fnName, ok := store.ParseLambdaARNFromSFNResource(authzRow.AuthorizerURI)
	if !ok || fnName == "" {
		return false, "invalid authorizer"
	}
	if fnAccount == "" {
		fnAccount = accountID
	}
	fn, executedVersion, err := s.store.ResolveFunction(fnAccount, fnName, "$LATEST")
	if err != nil {
		return false, "authorizer lambda not found"
	}
	executeAPISourceARN := store.APIGatewayRouteARN(region, accountID, apiID, stage, r.Method, routePath)
	if credARN := strings.TrimSpace(authzRow.AuthorizerCredentialsArn); credARN != "" {
		if !s.store.RoleSessionAllows(
			accountID, credARN, catalog.ActionLambdaInvoke, fn.FunctionARN,
			"apigateway-authorizer-credentials", region,
		) {
			return false, "Forbidden"
		}
	} else if !s.store.DeliveryTargetResourcePolicyAllows(
		fnAccount, fn.FunctionARN, catalog.ActionLambdaInvoke, authz.ServicePrincipalAPIGateway, executeAPISourceARN,
	) {
		return false, "Forbidden"
	}

	eventJSON, err := buildHTTPAPIAuthorizerEvent(r, accountID, apiID, stage, routePath, requestID, route, authzRow, executeAPISourceARN)
	if err != nil {
		return false, "authorizer event"
	}
	result, err := s.executeLambdaInvoke(ctx, fnAccount, fnName, fn, executedVersion, string(eventJSON))
	if err != nil {
		if strings.Contains(err.Error(), "compute unavailable") {
			return false, "compute unavailable"
		}
		return false, "authorizer invoke failed"
	}
	if parseHTTPAPIAuthorizerResponse(result, authzRow.EnableSimpleResponses) {
		return true, ""
	}
	return false, "Unauthorized"
}

func buildHTTPAPIAuthorizerEvent(
	r *http.Request,
	accountID, apiID, stage, routePath, requestID string,
	route store.APIGatewayRoute,
	authzRow store.APIGatewayAuthorizer,
	routeARN string,
) ([]byte, error) {
	headers := flattenHeaders(r.Header)
	identityVals := resolveAPIGatewayIdentitySources(r, route.RouteKey, authzRow.IdentitySource)
	payloadVer := authzRow.AuthorizerPayloadFormatVersion
	if payloadVer == "" {
		payloadVer = store.APIGatewayAuthorizerPayload20
	}
	if payloadVer == store.APIGatewayAuthorizerPayload10 {
		return json.Marshal(map[string]any{
			"version":            "1.0",
			"type":               "REQUEST",
			"methodArn":          routeARN,
			"identitySource":     strings.Join(identityVals, ","),
			"authorizationToken": strings.Join(identityVals, ","),
			"resource":           routePath,
			"path":               routePath,
			"httpMethod":         r.Method,
			"headers":            headers,
			"queryStringParameters": flattenQuery(r),
			"requestContext": map[string]any{
				"accountId":  accountID,
				"apiId":      apiID,
				"stage":      stage,
				"requestId":  requestID,
				"httpMethod": r.Method,
				"path":       routePath,
			},
		})
	}
	return json.Marshal(map[string]any{
		"version":               "2.0",
		"type":                  "REQUEST",
		"routeArn":              routeARN,
		"identitySource":        identityVals,
		"routeKey":              route.RouteKey,
		"rawPath":               routePath,
		"rawQueryString":        r.URL.RawQuery,
		"headers":               headers,
		"queryStringParameters": flattenQuery(r),
		"requestContext": map[string]any{
			"accountId": accountID,
			"apiId":     apiID,
			"stage":     stage,
			"requestId": requestID,
			"routeKey":  route.RouteKey,
			"http":      map[string]any{"method": r.Method, "path": routePath},
			"timeEpoch": 0,
		},
	})
}

func resolveAPIGatewayIdentitySources(r *http.Request, routeKey, identitySource string) []string {
	src := strings.TrimSpace(identitySource)
	switch {
	case strings.HasPrefix(src, "$request.header."):
		name := strings.TrimPrefix(src, "$request.header.")
		return []string{r.Header.Get(name)}
	case strings.HasPrefix(src, "$request.querystring."):
		name := strings.TrimPrefix(src, "$request.querystring.")
		return []string{r.URL.Query().Get(name)}
	case src == "$context.routeKey":
		return []string{routeKey}
	default:
		return []string{r.Header.Get("Authorization")}
	}
}

func flattenQuery(r *http.Request) map[string]string {
	out := map[string]string{}
	for k, vals := range r.URL.Query() {
		if len(vals) > 0 {
			out[k] = vals[0]
		}
	}
	return out
}

// parseHTTPAPIAuthorizerResponse honors simple isAuthorized or IAM policy Effect.
// Unsupported / malformed responses fail closed (deny).
func parseHTTPAPIAuthorizerResponse(result []byte, preferSimple bool) bool {
	_ = preferSimple
	var raw map[string]any
	if err := json.Unmarshal(result, &raw); err != nil {
		return false
	}
	if v, ok := raw["isAuthorized"]; ok {
		switch t := v.(type) {
		case bool:
			return t
		case string:
			return strings.EqualFold(t, "true")
		default:
			return false
		}
	}
	policy, _ := raw["policyDocument"].(map[string]any)
	if policy == nil {
		return false
	}
	stmts, _ := policy["Statement"].([]any)
	if len(stmts) == 0 {
		return false
	}
	// Lab: allow only when every statement is Allow (Deny anywhere fails closed).
	sawAllow := false
	for _, s := range stmts {
		st, ok := s.(map[string]any)
		if !ok {
			return false
		}
		effect, _ := st["Effect"].(string)
		switch strings.ToLower(strings.TrimSpace(effect)) {
		case "allow":
			sawAllow = true
		case "deny":
			return false
		default:
			return false
		}
	}
	return sawAllow
}
