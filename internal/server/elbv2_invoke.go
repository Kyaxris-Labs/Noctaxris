package server

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

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
	return isELBv2LabMuxListenerPath("alb", path)
}

func parseELBv2LabListenerPath(path string) (accountID, lbName string, port int, routePath string, ok bool) {
	return parseELBv2LabMuxListenerPath("alb", path)
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
	if lb.Type == "network" {
		http.Error(w, "network load balancers use the /nlb/ lab dataplane (HTTP shim, not TCP L4)", http.StatusBadRequest)
		return
	}

	region := store.DefaultELBv2Region
	logState := &elbLabAccessLogState{
		accountID: accountID,
		lb:        lb,
		region:    region,
		routePath: routePath,
		body:      body,
	}
	defer logState.emit(s, r)

	if !s.enforceAssociatedWAF(w, r, accountID, elbWAFCandidateARNs(lb.ARN)) {
		logState.set(http.StatusForbidden, 0, 0, "")
		return
	}

	listener, err := s.store.GetELBv2ListenerByPort(accountID, lb.ARN, port)
	if errors.Is(err, store.ErrELBv2NotFound) {
		logState.set(http.StatusNotFound, 0, 0, "")
		http.Error(w, "listener not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	tgARN, err := s.store.ResolveELBv2ListenerTargetGroup(accountID, listener, store.ELBv2RuleMatchInput{
		Path:     routePath,
		Host:     r.Host,
		Headers:  elbLowerHeaderMap(r.Header),
		Query:    elbQueryFirstValues(r),
		SourceIP: peerClientIP(r),
	})
	if err != nil || tgARN == "" {
		logState.set(http.StatusBadGateway, 0, 0, "")
		http.Error(w, "target group not found", http.StatusBadGateway)
		return
	}
	logState.tgARN = tgARN

	tgs, err := s.store.DescribeELBv2TargetGroups(accountID, []string{tgARN})
	if err != nil || len(tgs) == 0 {
		logState.set(http.StatusBadGateway, 0, 0, tgARN)
		http.Error(w, "target group not found", http.StatusBadGateway)
		return
	}
	tg := tgs[0]
	if tg.TargetType != "lambda" {
		logState.set(http.StatusBadGateway, 0, 0, tgARN)
		http.Error(w, "lab listener supports lambda target groups only", http.StatusBadGateway)
		return
	}

	targets, err := s.store.ListELBv2Targets(accountID, tg.ARN)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if len(targets) == 0 {
		logState.set(http.StatusBadGateway, 0, 0, tgARN)
		http.Error(w, "no registered targets", http.StatusBadGateway)
		return
	}

	fnAccount, fnName, ok := store.ParseLambdaARNFromSFNResource(targets[0].ID)
	if !ok || fnName == "" {
		logState.set(http.StatusBadGateway, 0, 0, tgARN)
		http.Error(w, "invalid lambda target", http.StatusBadGateway)
		return
	}
	if fnAccount == "" {
		fnAccount = accountID
	}
	fn, executedVersion, err := s.store.ResolveFunction(fnAccount, fnName, "$LATEST")
	if err != nil {
		logState.set(http.StatusBadGateway, 0, 0, tgARN)
		http.Error(w, "lambda not found", http.StatusBadGateway)
		return
	}
	if !s.store.DeliveryTargetResourcePolicyAllows(
		fnAccount, fn.FunctionARN, catalog.ActionLambdaInvoke, authz.ServicePrincipalELB, tg.ARN,
	) {
		logState.set(http.StatusForbidden, 0, 0, tgARN)
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
			logState.set(http.StatusServiceUnavailable, 0, 0, tgARN)
			http.Error(w, "compute unavailable", http.StatusServiceUnavailable)
			return
		}
		logState.set(http.StatusInternalServerError, 0, 0, tgARN)
		http.Error(w, "invoke failed", http.StatusInternalServerError)
		return
	}
	elbStatus, targetStatus, sentBytes := elbStatusFromLambdaProxy(result)
	logState.set(elbStatus, targetStatus, sentBytes, tgARN)
	logState.failClosed = true
	if err := logState.emitNow(s, r); err != nil {
		http.Error(w, "access log delivery failed", http.StatusServiceUnavailable)
		return
	}
	logState.skipDefer = true
	writeHTTPAPIProxyResponse(w, result)
	verified := &authn.Verified{AccountID: accountID, Region: region, Service: "elasticloadbalancing"}
	s.writeSuccessAudit(r, requestID, eventID, verified, elbv2EventSource, "LabListenerInvoke", readOnly)
}

type elbLabAccessLogState struct {
	accountID   string
	lb          store.ELBv2LoadBalancer
	region      string
	routePath   string
	body        []byte
	tgARN       string
	elbStatus   int
	targetStatus int
	sentBytes   int64
	recorded    bool
	failClosed  bool
	skipDefer   bool
}

func (st *elbLabAccessLogState) set(elbStatus, targetStatus int, sentBytes int64, tgARN string) {
	st.elbStatus = elbStatus
	st.targetStatus = targetStatus
	st.sentBytes = sentBytes
	if tgARN != "" {
		st.tgARN = tgARN
	}
	st.recorded = true
}

func (st *elbLabAccessLogState) emit(s *Server, r *http.Request) {
	if st.skipDefer || !st.recorded {
		return
	}
	_ = st.emitNow(s, r)
}

func (st *elbLabAccessLogState) emitNow(s *Server, r *http.Request) error {
	cfg, err := s.store.ELBv2AccessLogsConfigForARN(st.accountID, st.lb.ARN)
	if err != nil || !cfg.Enabled {
		return nil
	}
	clientIP, clientPort := elbClientEndpoint(r)
	in := store.ELBv2AccessLogInput{
		Region:           st.region,
		ClientIP:         clientIP,
		ClientPort:       clientPort,
		RequestMethod:    r.Method,
		RequestPath:      st.routePath,
		UserAgent:        r.UserAgent(),
		TargetGroupARN:   st.tgARN,
		ElbStatusCode:    st.elbStatus,
		TargetStatusCode: st.targetStatus,
		ReceivedBytes:    int64(len(st.body)),
		SentBytes:        st.sentBytes,
		RequestTime:      time.Now().UTC(),
	}
	err = s.store.AppendELBv2AccessLog(st.accountID, st.lb, in)
	if err != nil && st.failClosed {
		return err
	}
	return nil
}

func elbClientEndpoint(r *http.Request) (string, int) {
	host := peerClientIP(r)
	port := 0
	if h, p, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		if host == "" || host == "@" {
			host = peerClientIP(r)
		} else if host != "" {
			host = h
		}
		if p != "" {
			port, _ = strconv.Atoi(p)
		}
	}
	return host, port
}

func elbQueryFirstValues(r *http.Request) map[string]string {
	out := map[string]string{}
	for k, vals := range r.URL.Query() {
		if len(vals) == 0 {
			continue
		}
		out[strings.ToLower(k)] = vals[0]
	}
	return out
}

func elbLowerHeaderMap(h http.Header) map[string]string {
	out := map[string]string{}
	for k, vals := range h {
		if len(vals) == 0 {
			continue
		}
		out[strings.ToLower(k)] = vals[0]
	}
	return out
}

func elbStatusFromLambdaProxy(result []byte) (elbStatus, targetStatus int, sentBytes int64) {
	elbStatus = http.StatusOK
	targetStatus = http.StatusOK
	var proxy struct {
		StatusCode int    `json:"statusCode"`
		Body       string `json:"body"`
	}
	if json.Unmarshal(result, &proxy) == nil && proxy.StatusCode > 0 {
		elbStatus = proxy.StatusCode
		targetStatus = proxy.StatusCode
		sentBytes = int64(len(proxy.Body))
	} else {
		sentBytes = int64(len(result))
	}
	return elbStatus, targetStatus, sentBytes
}
