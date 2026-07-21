package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func isHTTPAPIInvokePath(path string) bool {
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	// /http-api/{apiId}/{stage}/...
	return len(parts) >= 3 && parts[0] == "http-api" && parts[1] != "" && parts[2] != ""
}

func parseHTTPAPIInvokePath(path string) (apiID, stage, routePath string, ok bool) {
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) < 3 || parts[0] != "http-api" {
		return "", "", "", false
	}
	apiID = parts[1]
	stage = parts[2]
	if apiID == "" || stage == "" {
		return "", "", "", false
	}
	if len(parts) == 3 {
		routePath = "/"
	} else {
		routePath = "/" + strings.Join(parts[3:], "/")
	}
	return apiID, stage, routePath, true
}

// handleHTTPAPIInvoke serves HTTP API route invoke on /http-api/{apiId}/{stage}/{path}.
// Auth is route-specific: NONE, JWT (Bearer), or AWS_IAM (SigV4 execute-api).
func (s *Server) handleHTTPAPIInvoke(w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string, readOnly bool) {
	apiID, stage, routePath, ok := parseHTTPAPIInvokePath(r.URL.Path)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	accountID, _, err := s.store.GetAPIGatewayAPIByID(apiID)
	if errors.Is(err, store.ErrAPIGatewayNotFound) {
		http.Error(w, "API not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if _, err := s.store.GetAPIGatewayStage(accountID, apiID, stage); err != nil {
		if errors.Is(err, store.ErrAPIGatewayNotFound) {
			http.Error(w, "stage not found", http.StatusNotFound)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	region := store.DefaultAPIGatewayRegion
	if !s.enforceAssociatedWAF(w, accountID, httpAPIWAFCandidateARNs(region, accountID, apiID, stage)) {
		return
	}

	route, err := s.store.MatchAPIGatewayRoute(accountID, apiID, r.Method, routePath)
	if errors.Is(err, store.ErrAPIGatewayNotFound) {
		http.Error(w, "route not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	var verified *authn.Verified
	switch route.AuthorizationType {
	case store.APIGatewayAuthNone:
		if !s.cfg.OpenDataPlaneAllowed() {
			http.Error(w, "open data plane disabled (set NOCTAXRIS_ALLOW_OPEN_DATA_PLANE=1)", http.StatusForbidden)
			return
		}
		verified = &authn.Verified{AccountID: accountID, Region: region, Service: "execute-api"}
	case store.APIGatewayAuthJWT:
		authz, err := s.store.GetAPIGatewayAuthorizer(accountID, apiID, route.AuthorizerID)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		token := bearerTokenFromAuthorization(r.Header.Get("Authorization"))
		if token == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		if err := s.verifyAPIGatewayJWT(token, authz.JWTIssuer, authz.JWTAudience, s.now(), "access"); err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		verified = &authn.Verified{AccountID: accountID, Region: region, Service: "execute-api"}
	case store.APIGatewayAuthIAM:
		v, err := authn.Verify(r, body, s.now(), sigv4Skew, s.lookupKey)
		if err != nil {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		if !strings.EqualFold(v.Service, "execute-api") {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		if v.AccountID != accountID {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		routeARN := store.APIGatewayRouteARN(region, accountID, apiID, stage, r.Method, routePath)
		if !s.authorize(v, catalog.ActionExecuteAPIInvoke, routeARN) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		verified = v
	default:
		http.Error(w, "unsupported authorization", http.StatusBadRequest)
		return
	}

	integrationID := store.IntegrationIDFromTarget(route.Target)
	in, err := s.store.GetAPIGatewayIntegration(accountID, apiID, integrationID)
	if err != nil {
		http.Error(w, "integration not found", http.StatusNotFound)
		return
	}
	fnAccount, fnName, ok := store.ParseLambdaARNFromSFNResource(in.IntegrationURI)
	if !ok {
		http.Error(w, "invalid integration", http.StatusBadRequest)
		return
	}
	if fnAccount == "" {
		fnAccount = accountID
	}
	fn, executedVersion, err := s.store.ResolveFunction(fnAccount, fnName, "$LATEST")
	if err != nil {
		http.Error(w, "lambda not found", http.StatusBadRequest)
		return
	}
	executeAPISourceARN := store.APIGatewayRouteARN(region, accountID, apiID, stage, r.Method, routePath)
	if credARN := strings.TrimSpace(in.CredentialsArn); credARN != "" {
		if !s.store.RoleSessionAllows(
			accountID, credARN, catalog.ActionLambdaInvoke, fn.FunctionARN,
			"apigateway-credentials", region,
		) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
	} else if !s.store.DeliveryTargetResourcePolicyAllows(
		fnAccount, fn.FunctionARN, catalog.ActionLambdaInvoke, authz.ServicePrincipalAPIGateway, executeAPISourceARN,
	) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	eventJSON, _ := json.Marshal(map[string]any{
		"version":        "2.0",
		"routeKey":       route.RouteKey,
		"rawPath":        routePath,
		"rawQueryString": r.URL.RawQuery,
		"headers":        flattenHeaders(r.Header),
		"requestContext": map[string]any{
			"accountId":  accountID,
			"apiId":      apiID,
			"stage":      stage,
			"http":       map[string]any{"method": r.Method, "path": routePath},
			"requestId":  requestID,
			"timeEpoch":  s.now().UnixMilli(),
		},
		"body":            string(body),
		"isBase64Encoded": false,
	})

	result, err := s.executeLambdaInvoke(r.Context(), fnAccount, fnName, fn, executedVersion, string(eventJSON))
	if err != nil {
		if strings.Contains(err.Error(), "compute unavailable") {
			http.Error(w, "compute unavailable", http.StatusServiceUnavailable)
			return
		}
		http.Error(w, "invoke failed", http.StatusInternalServerError)
		return
	}
	writeHTTPAPIProxyResponse(w, result)
	s.writeSuccessAudit(r, requestID, eventID, verified, apiGatewayEventSource, "Invoke", readOnly)
}

func bearerTokenFromAuthorization(h string) string {
	h = strings.TrimSpace(h)
	if h == "" {
		return ""
	}
	const prefix = "Bearer "
	if len(h) > len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
		return strings.TrimSpace(h[len(prefix):])
	}
	return ""
}

func flattenHeaders(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for k, vals := range h {
		if len(vals) > 0 {
			out[k] = vals[0]
		}
	}
	return out
}

// writeHTTPAPIProxyResponse maps Lambda proxy 2.0 response shape to HTTP, or raw body.
func writeHTTPAPIProxyResponse(w http.ResponseWriter, result []byte) {
	var proxy struct {
		StatusCode int               `json:"statusCode"`
		Headers    map[string]string `json:"headers"`
		Body       string            `json:"body"`
	}
	if json.Unmarshal(result, &proxy) == nil && proxy.StatusCode > 0 {
		for k, v := range proxy.Headers {
			w.Header().Set(k, v)
		}
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", "application/json")
		}
		w.WriteHeader(proxy.StatusCode)
		_, _ = w.Write([]byte(proxy.Body))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if len(result) == 0 {
		_, _ = w.Write([]byte("null"))
		return
	}
	_, _ = w.Write(result)
}