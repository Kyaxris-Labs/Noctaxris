package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// Lab ALB listener path: /alb/{accountId}/{loadBalancerName}/{port}[/{path...}]
// Loopback-friendly open dataplane (same gate as Function URL NONE / HTTP API NONE).
// Invokes the first registered Lambda target after resource-policy Allow for
// elasticloadbalancing.amazonaws.com and optional WAF association on the LB ARN.

func isELBv2LabListenerPath(path string) bool {
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	return len(parts) >= 4 && parts[0] == "alb" && parts[1] != "" && parts[2] != "" && parts[3] != ""
}

func parseELBv2LabListenerPath(path string) (accountID, lbName string, port int, routePath string, ok bool) {
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) < 4 || parts[0] != "alb" {
		return "", "", 0, "", false
	}
	accountID = parts[1]
	lbName = parts[2]
	port, err := strconv.Atoi(parts[3])
	if err != nil || port <= 0 || accountID == "" || lbName == "" {
		return "", "", 0, "", false
	}
	if len(parts) == 4 {
		routePath = "/"
	} else {
		routePath = "/" + strings.Join(parts[4:], "/")
	}
	return accountID, lbName, port, routePath, true
}

func elbWAFCandidateARNs(lbARN string) []string {
	lbARN = strings.TrimSpace(lbARN)
	if lbARN == "" {
		return nil
	}
	return []string{lbARN}
}

// handleELBv2LabListener serves HTTP on /alb/{account}/{lbName}/{port}/...
func (s *Server) handleELBv2LabListener(w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string, readOnly bool) {
	accountID, lbName, port, routePath, ok := parseELBv2LabListenerPath(r.URL.Path)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !s.cfg.OpenDataPlaneAllowed() {
		http.Error(w, "open data plane disabled (set NOCTAXRIS_ALLOW_OPEN_DATA_PLANE=1)", http.StatusForbidden)
		return
	}

	lb, err := s.store.GetELBv2LoadBalancerByName(accountID, lbName)
	if errors.Is(err, store.ErrELBv2NotFound) {
		http.Error(w, "load balancer not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	region := store.DefaultELBv2Region
	if !s.enforceAssociatedWAF(w, r, accountID, elbWAFCandidateARNs(lb.ARN)) {
		return
	}

	listener, err := s.store.GetELBv2ListenerByPort(accountID, lb.ARN, port)
	if errors.Is(err, store.ErrELBv2NotFound) {
		http.Error(w, "listener not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	tgARN, err := s.store.ResolveELBv2ListenerTargetGroup(accountID, listener, routePath, r.Host)
	if err != nil || tgARN == "" {
		http.Error(w, "target group not found", http.StatusBadGateway)
		return
	}

	tgs, err := s.store.DescribeELBv2TargetGroups(accountID, []string{tgARN})
	if err != nil || len(tgs) == 0 {
		http.Error(w, "target group not found", http.StatusBadGateway)
		return
	}
	tg := tgs[0]
	if tg.TargetType != "lambda" {
		http.Error(w, "lab listener supports lambda target groups only", http.StatusBadGateway)
		return
	}

	targets, err := s.store.ListELBv2Targets(accountID, tg.ARN)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if len(targets) == 0 {
		http.Error(w, "no registered targets", http.StatusBadGateway)
		return
	}

	fnAccount, fnName, ok := store.ParseLambdaARNFromSFNResource(targets[0].ID)
	if !ok || fnName == "" {
		http.Error(w, "invalid lambda target", http.StatusBadGateway)
		return
	}
	if fnAccount == "" {
		fnAccount = accountID
	}
	fn, executedVersion, err := s.store.ResolveFunction(fnAccount, fnName, "$LATEST")
	if err != nil {
		http.Error(w, "lambda not found", http.StatusBadGateway)
		return
	}
	if !s.store.DeliveryTargetResourcePolicyAllows(
		fnAccount, fn.FunctionARN, catalog.ActionLambdaInvoke, authz.ServicePrincipalELB, tg.ARN,
	) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	query := map[string]string{}
	for k, vals := range r.URL.Query() {
		if len(vals) > 0 {
			query[k] = vals[0]
		}
	}
	headers := flattenHeaders(r.Header)
	if headers["x-forwarded-port"] == "" {
		headers["x-forwarded-port"] = strconv.Itoa(port)
	}
	if headers["x-forwarded-proto"] == "" {
		headers["x-forwarded-proto"] = "http"
	}
	if headers["host"] == "" {
		headers["host"] = r.Host
	}

	eventJSON, _ := json.Marshal(map[string]any{
		"requestContext": map[string]any{
			"elb": map[string]any{"targetGroupArn": tg.ARN},
		},
		"httpMethod":            r.Method,
		"path":                  routePath,
		"queryStringParameters": query,
		"headers":               headers,
		"body":                  string(body),
		"isBase64Encoded":       false,
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
	verified := &authn.Verified{AccountID: accountID, Region: region, Service: "elasticloadbalancing"}
	s.writeSuccessAudit(r, requestID, eventID, verified, elbv2EventSource, "LabListenerInvoke", readOnly)
}
