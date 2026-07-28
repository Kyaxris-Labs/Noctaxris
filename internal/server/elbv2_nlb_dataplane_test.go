package server_test

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestELBv2NLBLabListenerForwardsToIPTarget(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/svc/ping" {
			t.Fatalf("path=%q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("nlb-ok"))
	}))
	t.Cleanup(backend.Close)

	host, portStr, err := net.SplitHostPort(backend.Listener.Addr().String())
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}
	_ = portStr

	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	lb := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.CreateLoadBalancer", "elasticloadbalancing", map[string]any{
		"Name": "fwd-nlb", "Type": "network",
	}, now)
	var lbOut map[string]any
	_ = json.Unmarshal(lb.Body.Bytes(), &lbOut)
	lbARN, _ := lbOut["LoadBalancers"].([]any)[0].(map[string]any)["LoadBalancerArn"].(string)

	tg := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.CreateTargetGroup", "elasticloadbalancing", map[string]any{
		"Name": "fwd-nlb-tg", "TargetType": "ip", "Protocol": "TCP", "Port": 80,
	}, now)
	var tgOut map[string]any
	_ = json.Unmarshal(tg.Body.Bytes(), &tgOut)
	tgARNStr, _ := tgOut["TargetGroups"].([]any)[0].(map[string]any)["TargetGroupArn"].(string)

	mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.CreateListener", "elasticloadbalancing", map[string]any{
		"LoadBalancerArn": lbARN,
		"Protocol":        "TCP",
		"Port":            80,
		"DefaultActions":  []map[string]any{{"Type": "forward", "TargetGroupArn": tgARNStr}},
	}, now)

	reg := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.RegisterTargets", "elasticloadbalancing", map[string]any{
		"TargetGroupArn": tgARNStr,
		"Targets":        []map[string]any{{"Id": host, "Port": mustAtoi(t, portStr)}},
	}, now)
	if reg.Code != http.StatusOK {
		t.Fatalf("RegisterTargets status=%d body=%q", reg.Code, reg.Body.String())
	}

	path := "/nlb/" + testAccountID + "/fwd-nlb/80/svc/ping"
	req := mustNewRequest(t, http.MethodGet, "http://127.0.0.1:4566"+path, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "nlb-ok" {
		t.Fatalf("nlb listener status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestELBv2NLBLabListenerRejectsApplicationLB(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	lb := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.CreateLoadBalancer", "elasticloadbalancing", map[string]any{
		"Name": "app-only",
	}, now)
	lbARN, _ := jsonUnmarshalLBARN(t, lb.Body.Bytes())

	tg := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.CreateTargetGroup", "elasticloadbalancing", map[string]any{
		"Name": "app-tg", "TargetType": "lambda",
	}, now)
	tgARN, _ := jsonUnmarshalTGARN(t, tg.Body.Bytes())

	mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.CreateListener", "elasticloadbalancing", map[string]any{
		"LoadBalancerArn": lbARN,
		"Protocol":        "HTTP",
		"Port":            80,
		"DefaultActions":  []map[string]any{{"Type": "forward", "TargetGroupArn": tgARN}},
	}, now)

	path := "/nlb/" + testAccountID + "/app-only/80/"
	req := mustNewRequest(t, http.MethodGet, "http://127.0.0.1:4566"+path, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "/alb/") {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestELBv2ALBLabListenerRejectsNetworkLB(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	lb := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.CreateLoadBalancer", "elasticloadbalancing", map[string]any{
		"Name": "net-only", "Type": "network",
	}, now)
	lbARN, _ := jsonUnmarshalLBARN(t, lb.Body.Bytes())

	tg := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.CreateTargetGroup", "elasticloadbalancing", map[string]any{
		"Name": "net-tg", "TargetType": "ip", "Protocol": "TCP", "Port": 80,
	}, now)
	tgARN, _ := jsonUnmarshalTGARN(t, tg.Body.Bytes())

	mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.CreateListener", "elasticloadbalancing", map[string]any{
		"LoadBalancerArn": lbARN,
		"Protocol":        "TCP",
		"Port":            80,
		"DefaultActions":  []map[string]any{{"Type": "forward", "TargetGroupArn": tgARN}},
	}, now)

	path := "/alb/" + testAccountID + "/net-only/80/"
	req := mustNewRequest(t, http.MethodGet, "http://127.0.0.1:4566"+path, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "/nlb/") {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func jsonUnmarshalLBARN(t *testing.T, raw []byte) (string, bool) {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	arn, _ := out["LoadBalancers"].([]any)[0].(map[string]any)["LoadBalancerArn"].(string)
	return arn, arn != ""
}

func jsonUnmarshalTGARN(t *testing.T, raw []byte) (string, bool) {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	arn, _ := out["TargetGroups"].([]any)[0].(map[string]any)["TargetGroupArn"].(string)
	return arn, arn != ""
}

func mustAtoi(t *testing.T, s string) int {
	t.Helper()
	n, err := strconv.Atoi(s)
	if err != nil {
		t.Fatal(err)
	}
	return n
}
