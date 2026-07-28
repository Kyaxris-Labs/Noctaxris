package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// Lab NLB listener path: /nlb/{accountId}/{loadBalancerName}/{port}[/{path...}]
// HTTP lab shim on :4566 (not true TCP L4). Same open-dataplane gate as /alb/.

func isELBv2NLBLabListenerPath(path string) bool {
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	return len(parts) >= 4 && parts[0] == "nlb" && parts[1] != "" && parts[2] != "" && parts[3] != ""
}

func parseELBv2NLBLabListenerPath(path string) (accountID, lbName string, port int, routePath string, ok bool) {
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) < 4 || parts[0] != "nlb" {
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

// handleELBv2NLBLabListener serves HTTP on /nlb/{account}/{lbName}/{port}/...
func (s *Server) handleELBv2NLBLabListener(w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string, readOnly bool) {
	accountID, lbName, port, routePath, ok := parseELBv2NLBLabListenerPath(r.URL.Path)
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
	if lb.Type != "network" {
		http.Error(w, "application load balancers use the /alb/ lab dataplane", http.StatusBadRequest)
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

	tgARN := strings.TrimSpace(listener.TargetGroupARN)
	if tgARN == "" {
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

	targetURL, err := s.store.ELBv2NLBForwardURL(accountID, tg, targets[0], routePath, r.URL.RawQuery)
	if err != nil {
		logState.set(http.StatusBadGateway, 0, 0, tgARN)
		if strings.Contains(err.Error(), "not reachable") {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		http.Error(w, "invalid target", http.StatusBadGateway)
		return
	}

	status, respBody, respHeader, ferr := store.FetchELBv2LabHTTPForward(r.Context(), targetURL, r.Method, body, r.Header)
	if ferr != nil {
		logState.set(http.StatusBadGateway, 0, 0, tgARN)
		http.Error(w, "target forward failed", http.StatusBadGateway)
		return
	}
	for k, vals := range respHeader {
		lk := strings.ToLower(k)
		if lk == "connection" || lk == "transfer-encoding" {
			continue
		}
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	logState.set(status, status, int64(len(respBody)), tgARN)
	logState.failClosed = true
	if err := logState.emitNow(s, r); err != nil {
		http.Error(w, "access log delivery failed", http.StatusServiceUnavailable)
		return
	}
	logState.skipDefer = true
	w.WriteHeader(status)
	_, _ = w.Write(respBody)
	verified := &authn.Verified{AccountID: accountID, Region: region, Service: "elasticloadbalancing"}
	s.writeSuccessAudit(r, requestID, eventID, verified, elbv2EventSource, "NLBLabListenerForward", readOnly)
}
