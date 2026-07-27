package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func isWebSocketAPILabPath(path string) bool {
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	// /ws-api/{apiId}/{stage}/$connect|$disconnect|$default
	return len(parts) == 4 && parts[0] == "ws-api" && parts[1] != "" && parts[2] != "" &&
		store.IsAPIGatewayWebSocketRouteKey(parts[3])
}

func isAPIGatewayConnectionsPath(path string) bool {
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	// /execute-api/{apiId}/{stage}/@connections/{connectionId}
	return len(parts) == 5 && parts[0] == "execute-api" && parts[3] == "@connections" &&
		parts[1] != "" && parts[2] != "" && parts[4] != ""
}

func parseWebSocketAPILabPath(path string) (apiID, stage, routeKey string, ok bool) {
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) != 4 || parts[0] != "ws-api" {
		return "", "", "", false
	}
	if !store.IsAPIGatewayWebSocketRouteKey(parts[3]) {
		return "", "", "", false
	}
	return parts[1], parts[2], parts[3], true
}

func parseAPIGatewayConnectionsPath(path string) (apiID, stage, connectionID string, ok bool) {
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) != 5 || parts[0] != "execute-api" || parts[3] != "@connections" {
		return "", "", "", false
	}
	return parts[1], parts[2], parts[4], true
}

// handleWebSocketAPILabInvoke is the HTTP lab stand-in for WebSocket $connect/$disconnect/$default.
// Not a full internet WebSocket gateway; clients POST to /ws-api/{apiId}/{stage}/{routeKey}.
func (s *Server) handleWebSocketAPILabInvoke(w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string, readOnly bool) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	apiID, stage, routeKey, ok := parseWebSocketAPILabPath(r.URL.Path)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	accountID, api, err := s.store.GetAPIGatewayAPIByID(apiID)
	if errors.Is(err, store.ErrAPIGatewayNotFound) {
		http.Error(w, "API not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if api.ProtocolType != store.APIGatewayProtocolWebSocket {
		http.Error(w, "not a WebSocket API", http.StatusBadRequest)
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
	route, err := s.store.MatchAPIGatewayWebSocketRoute(accountID, apiID, routeKey)
	if errors.Is(err, store.ErrAPIGatewayNotFound) {
		http.Error(w, "route not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	region := store.DefaultAPIGatewayRegion
	var verified *authn.Verified
	switch route.AuthorizationType {
	case store.APIGatewayAuthNone:
		if !s.cfg.OpenDataPlaneAllowed() {
			http.Error(w, "open data plane disabled (set NOCTAXRIS_ALLOW_OPEN_DATA_PLANE=1)", http.StatusForbidden)
			return
		}
		verified = &authn.Verified{AccountID: accountID, Region: region, Service: "execute-api"}
	case store.APIGatewayAuthIAM:
		v, err := authn.Verify(r, body, s.now(), sigv4Skew, s.lookupKey)
		if err != nil {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		if !strings.EqualFold(v.Service, "execute-api") || v.AccountID != accountID {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		routeARN := store.APIGatewayRouteARN(region, accountID, apiID, stage, "POST", routeKey)
		if !s.authorize(v, catalog.ActionExecuteAPIInvoke, routeARN) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		verified = v
	default:
		http.Error(w, "unsupported authorization", http.StatusBadRequest)
		return
	}

	var connectionID string
	switch routeKey {
	case "$connect":
		conn := s.store.CreateAPIGatewayWebSocketConnection(accountID, apiID, stage)
		connectionID = conn.ConnectionID
	case "$disconnect", "$default":
		connectionID = strings.TrimSpace(r.Header.Get("X-Amzn-Connection-Id"))
		if connectionID == "" {
			connectionID = strings.TrimSpace(r.URL.Query().Get("connectionId"))
		}
		if connectionID == "" {
			http.Error(w, "connectionId required", http.StatusBadRequest)
			return
		}
		if _, err := s.store.GetAPIGatewayWebSocketConnection(accountID, apiID, stage, connectionID); err != nil {
			http.Error(w, "GoneException", http.StatusGone)
			return
		}
	}

	integrationID := store.IntegrationIDFromTarget(route.Target)
	in, err := s.store.GetAPIGatewayIntegration(accountID, apiID, integrationID)
	if err != nil || in.IntegrationType != store.APIGatewayIntegrationAWSProxy {
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
	executeAPISourceARN := store.APIGatewayRouteARN(region, accountID, apiID, stage, "POST", routeKey)
	if credARN := strings.TrimSpace(in.CredentialsArn); credARN != "" {
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

	eventType := strings.ToUpper(strings.TrimPrefix(routeKey, "$"))
	eventJSON, _ := json.Marshal(map[string]any{
		"requestContext": map[string]any{
			"routeKey":     routeKey,
			"eventType":    eventType,
			"connectionId": connectionID,
			"apiId":        apiID,
			"stage":        stage,
			"requestId":    requestID,
			"accountId":    accountID,
			"domainName":   "127.0.0.1:4566",
			"connectedAt":  s.now().UnixMilli(),
		},
		"body":            string(body),
		"isBase64Encoded": false,
	})
	result, err := s.executeLambdaInvoke(r.Context(), fnAccount, fnName, fn, executedVersion, string(eventJSON))
	if err != nil {
		if routeKey == "$connect" {
			_ = s.store.DeleteAPIGatewayWebSocketConnection(accountID, apiID, stage, connectionID)
		}
		if strings.Contains(err.Error(), "compute unavailable") {
			http.Error(w, "compute unavailable", http.StatusServiceUnavailable)
			return
		}
		http.Error(w, "invoke failed", http.StatusInternalServerError)
		return
	}
	if routeKey == "$disconnect" {
		_ = s.store.DeleteAPIGatewayWebSocketConnection(accountID, apiID, stage, connectionID)
	}

	w.Header().Set("Content-Type", "application/json")
	if routeKey == "$connect" {
		w.Header().Set("X-Amzn-Connection-Id", connectionID)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"connectionId":` + jsonString(connectionID) + `}`))
	} else {
		writeHTTPAPIProxyResponseOpts(w, result, false)
	}
	s.writeSuccessAudit(r, requestID, eventID, verified, apiGatewayEventSource, "WebSocketInvoke", readOnly)
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// handleAPIGatewayConnections serves PostToConnection / GetConnection / DeleteConnection lab lite.
func (s *Server) handleAPIGatewayConnections(w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string, readOnly bool) {
	apiID, stage, connectionID, ok := parseAPIGatewayConnectionsPath(r.URL.Path)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	verified, err := authn.Verify(r, body, s.now(), sigv4Skew, s.lookupKey)
	if err != nil {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if !strings.EqualFold(verified.Service, "execute-api") {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	accountID, api, err := s.store.GetAPIGatewayAPIByID(apiID)
	if errors.Is(err, store.ErrAPIGatewayNotFound) {
		http.Error(w, "API not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if api.ProtocolType != store.APIGatewayProtocolWebSocket {
		http.Error(w, "not a WebSocket API", http.StatusBadRequest)
		return
	}
	if verified.AccountID != accountID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	resourceARN := "arn:aws:execute-api:" + store.DefaultAPIGatewayRegion + ":" + accountID + ":" + apiID + "/" + stage + "/@connections/" + connectionID
	if !s.authorize(verified, catalog.ActionExecuteAPIManageConnections, resourceARN) &&
		!s.authorize(verified, catalog.ActionExecuteAPIInvoke, resourceARN) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	switch r.Method {
	case http.MethodPost:
		if err := s.store.PostToAPIGatewayWebSocketConnection(accountID, apiID, stage, connectionID, body); err != nil {
			if errors.Is(err, store.ErrAPIGatewayNotFound) {
				http.Error(w, "GoneException", http.StatusGone)
				return
			}
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		s.writeSuccessAudit(r, requestID, eventID, verified, apiGatewayEventSource, "PostToConnection", readOnly)
	case http.MethodGet:
		conn, err := s.store.GetAPIGatewayWebSocketConnection(accountID, apiID, stage, connectionID)
		if errors.Is(err, store.ErrAPIGatewayNotFound) {
			http.Error(w, "GoneException", http.StatusGone)
			return
		}
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		payload, _ := json.Marshal(map[string]any{
			"ConnectedAt":  time.UnixMilli(conn.ConnectedAt).UTC().Format(time.RFC3339),
			"Identity":     map[string]any{"SourceIp": "127.0.0.1"},
			"LastActiveAt": time.UnixMilli(conn.ConnectedAt).UTC().Format(time.RFC3339),
			"Messages":     conn.Messages,
		})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(payload)
		s.writeSuccessAudit(r, requestID, eventID, verified, apiGatewayEventSource, "GetConnection", readOnly)
	case http.MethodDelete:
		if err := s.store.DeleteAPIGatewayWebSocketConnection(accountID, apiID, stage, connectionID); err != nil {
			if errors.Is(err, store.ErrAPIGatewayNotFound) {
				http.Error(w, "GoneException", http.StatusGone)
				return
			}
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		s.writeSuccessAudit(r, requestID, eventID, verified, apiGatewayEventSource, "DeleteConnection", readOnly)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
