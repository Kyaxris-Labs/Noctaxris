package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func isRestAPIInvokePath(path string) bool {
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	// /restapis/{apiId}/{stage}/_user_request_/{proxy+}
	return len(parts) >= 4 && parts[0] == "restapis" && parts[1] != "" && parts[2] != "" && parts[3] == "_user_request_"
}

func parseRestAPIInvokePath(path string) (apiID, stage, routePath string, ok bool) {
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) < 4 || parts[0] != "restapis" || parts[3] != "_user_request_" {
		return "", "", "", false
	}
	apiID = parts[1]
	stage = parts[2]
	if apiID == "" || stage == "" {
		return "", "", "", false
	}
	if len(parts) == 4 {
		routePath = "/"
	} else {
		routePath = "/" + strings.Join(parts[4:], "/")
	}
	return apiID, stage, routePath, true
}

// handleRestAPIInvoke serves REST API execute on /restapis/{apiId}/{stage}/_user_request_/{path}.
func (s *Server) handleRestAPIInvoke(w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string, readOnly bool) {
	apiID, stage, routePath, ok := parseRestAPIInvokePath(r.URL.Path)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	accountID, _, err := s.store.GetRestAPIByID(apiID)
	if errors.Is(err, store.ErrAPIGatewayNotFound) {
		http.Error(w, "API not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if _, err := s.store.GetRestStage(accountID, apiID, stage); err != nil {
		if errors.Is(err, store.ErrAPIGatewayNotFound) {
			http.Error(w, "stage not found", http.StatusNotFound)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	region := store.DefaultAPIGatewayRegion
	resource, method, err := s.store.MatchRestAPIRoute(accountID, apiID, r.Method, routePath)
	if errors.Is(err, store.ErrAPIGatewayNotFound) {
		http.Error(w, "method not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	var verified *authn.Verified
	switch method.AuthorizationType {
	case store.APIGatewayRESTAuthNone:
		if !s.cfg.OpenDataPlaneAllowed() {
			http.Error(w, "open data plane disabled (set NOCTAXRIS_ALLOW_OPEN_DATA_PLANE=1)", http.StatusForbidden)
			return
		}
		verified = &authn.Verified{AccountID: accountID, Region: region, Service: "execute-api"}
	case store.APIGatewayRESTAuthIAM:
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
		routeARN := store.RestAPIMethodARN(region, accountID, apiID, stage, r.Method, routePath)
		if !s.authorize(v, catalog.ActionExecuteAPIInvoke, routeARN) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		verified = v
	case store.APIGatewayRESTAuthCUSTOM, store.APIGatewayRESTAuthTOKEN, store.APIGatewayRESTAuthREQUEST:
		authzRow, err := s.store.GetRestAuthorizer(accountID, apiID, method.AuthorizerID)
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		switch method.AuthorizationType {
		case store.APIGatewayRESTAuthTOKEN:
			if authzRow.Type != store.APIGatewayAuthorizerTOKEN {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
		case store.APIGatewayRESTAuthREQUEST:
			if authzRow.Type != store.APIGatewayAuthorizerREQUEST {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
		default: // CUSTOM
			if authzRow.Type != store.APIGatewayAuthorizerTOKEN && authzRow.Type != store.APIGatewayAuthorizerREQUEST {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
		}
		allowed, denyMsg := s.invokeRestAPILambdaAuthorizer(
			r.Context(), r, accountID, apiID, stage, routePath, resource.Path, resource.ResourceID, requestID, authzRow,
		)
		if !allowed {
			switch denyMsg {
			case "compute unavailable":
				http.Error(w, "compute unavailable", http.StatusServiceUnavailable)
			case "Forbidden":
				http.Error(w, "Forbidden", http.StatusForbidden)
			default:
				http.Error(w, "Unauthorized", http.StatusForbidden)
			}
			return
		}
		verified = &authn.Verified{AccountID: accountID, Region: region, Service: "execute-api"}
	default:
		http.Error(w, "unsupported authorization", http.StatusBadRequest)
		return
	}

	requiresKey := method.APIKeyRequired
	if !requiresKey {
		if hasPlan, err := s.store.RestStageHasUsagePlan(accountID, apiID, stage); err == nil && hasPlan {
			requiresKey = true
		}
	}
	if requiresKey {
		if !s.store.ValidateRestAPIKeyForStage(accountID, apiID, stage, r.Header.Get("x-api-key")) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
	}

	in, err := s.store.GetRestIntegration(accountID, apiID, resource.ResourceID, method.HTTPMethod)
	if err != nil {
		http.Error(w, "integration not found", http.StatusNotFound)
		return
	}

	switch in.Type {
	case store.APIGatewayRESTIntegrationMock:
		writeRestAPIMockResponse(w, in)
		s.writeSuccessAudit(r, requestID, eventID, verified, apiGatewayEventSource, "Invoke", readOnly)
		return
	case store.APIGatewayIntegrationHTTPProxy, store.APIGatewayIntegrationVPCLink:
		method := in.IntegrationHTTPMethod
		if method == "" {
			method = r.Method
		}
		status, respBody, respHeader, ferr := store.FetchAPIGatewayHTTPProxy(r.Context(), method, in.URI, body, r.Header)
		if ferr != nil {
			http.Error(w, "HTTP_PROXY fetch failed", http.StatusBadGateway)
			return
		}
		for k, vals := range respHeader {
			if isHTTPAPIStrippedResponseHeader(k, false) {
				continue
			}
			for _, v := range vals {
				w.Header().Add(k, v)
			}
		}
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", "application/octet-stream")
		}
		w.WriteHeader(status)
		_, _ = w.Write(respBody)
		s.writeSuccessAudit(r, requestID, eventID, verified, apiGatewayEventSource, "Invoke", readOnly)
		return
	case store.APIGatewayRESTIntegrationAWSProxy:
		// continue
	default:
		http.Error(w, "unsupported integration", http.StatusBadRequest)
		return
	}

	fnAccount, fnName, ok := store.ParseLambdaARNFromSFNResource(in.URI)
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
	executeAPISourceARN := store.RestAPIMethodARN(region, accountID, apiID, stage, r.Method, routePath)
	if credARN := strings.TrimSpace(in.Credentials); credARN != "" {
		if !s.store.RoleSessionAllows(
			accountID, credARN, catalog.ActionLambdaInvoke, fn.FunctionARN,
			"apigateway-credentials", region,
		) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		if fnAccount != accountID {
			if !s.store.DeliveryTargetResourcePolicyAllows(
				fnAccount, fn.FunctionARN, catalog.ActionLambdaInvoke, authz.ServicePrincipalAPIGateway, executeAPISourceARN,
			) {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
		}
	} else if !s.store.DeliveryTargetResourcePolicyAllows(
		fnAccount, fn.FunctionARN, catalog.ActionLambdaInvoke, authz.ServicePrincipalAPIGateway, executeAPISourceARN,
	) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	qs := queryStringMap(r.URL.Query())
	eventJSON, _ := json.Marshal(map[string]any{
		"resource":                        resource.Path,
		"path":                            routePath,
		"httpMethod":                      r.Method,
		"headers":                         flattenHeaders(r.Header),
		"multiValueHeaders":               multiValueHeaders(r.Header),
		"queryStringParameters":           qs,
		"multiValueQueryStringParameters": multiValueQuery(r.URL.Query()),
		"pathParameters":                  restPathParameters(resource.Path, routePath),
		"stageVariables":                  nil,
		"requestContext": map[string]any{
			"accountId":    accountID,
			"apiId":        apiID,
			"stage":        stage,
			"requestId":    requestID,
			"resourceId":   resource.ResourceID,
			"resourcePath": resource.Path,
			"httpMethod":   r.Method,
			"path":         "/" + stage + routePath,
			"identity":     map[string]any{},
			"protocol":     "HTTP/1.1",
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

func writeRestAPIMockResponse(w http.ResponseWriter, in store.RestIntegration) {
	body := `{"message":"OK"}`
	if tpl, ok := in.RequestTemplates["application/json"]; ok && strings.TrimSpace(tpl) != "" {
		body = tpl
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body))
}

func queryStringMap(q url.Values) map[string]string {
	if len(q) == 0 {
		return nil
	}
	out := make(map[string]string, len(q))
	for k, vals := range q {
		if len(vals) > 0 {
			out[k] = vals[0]
		}
	}
	return out
}

func multiValueQuery(q url.Values) map[string][]string {
	if len(q) == 0 {
		return nil
	}
	out := make(map[string][]string, len(q))
	for k, vals := range q {
		out[k] = vals
	}
	return out
}

func multiValueHeaders(h http.Header) map[string][]string {
	out := make(map[string][]string, len(h))
	for k, vals := range h {
		out[k] = vals
	}
	return out
}

func restPathParameters(resourcePath, requestPath string) map[string]string {
	rParts := splitPathParts(resourcePath)
	qParts := splitPathParts(requestPath)
	out := map[string]string{}
	for i, rp := range rParts {
		if rp == "{proxy+}" {
			if i < len(qParts) {
				out["proxy"] = strings.Join(qParts[i:], "/")
			} else {
				out["proxy"] = ""
			}
			return out
		}
		if strings.HasPrefix(rp, "{") && strings.HasSuffix(rp, "}") && i < len(qParts) {
			name := strings.TrimSuffix(strings.TrimPrefix(rp, "{"), "}")
			out[name] = qParts[i]
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func splitPathParts(p string) []string {
	p = strings.Trim(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}
