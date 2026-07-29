package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestELBv2PathRuleLabListenerSelectsTargetGroup(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	var gotEvents []string
	srv.SetLambdaInvokeHookForTest(func(_ context.Context, _, name string, _ store.LambdaFunction, _, eventJSON string) ([]byte, error) {
		gotEvents = append(gotEvents, name+"|"+eventJSON)
		return []byte(`{"statusCode":200,"headers":{"content-type":"text/plain"},"body":"ok"}`), nil
	})

	lb := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.CreateLoadBalancer", "elasticloadbalancing", map[string]any{
		"Name": "path-alb",
	}, now)
	var lbOut map[string]any
	_ = json.Unmarshal(lb.Body.Bytes(), &lbOut)
	lbs, _ := lbOut["LoadBalancers"].([]any)
	lbMap, _ := lbs[0].(map[string]any)
	lbARN, _ := lbMap["LoadBalancerArn"].(string)

	tgA := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.CreateTargetGroup", "elasticloadbalancing", map[string]any{
		"Name": "path-tg-a", "TargetType": "lambda",
	}, now)
	var tgAOut map[string]any
	_ = json.Unmarshal(tgA.Body.Bytes(), &tgAOut)
	tgAARN, _ := tgAOut["TargetGroups"].([]any)[0].(map[string]any)["TargetGroupArn"].(string)

	tgB := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.CreateTargetGroup", "elasticloadbalancing", map[string]any{
		"Name": "path-tg-b", "TargetType": "lambda",
	}, now)
	var tgBOut map[string]any
	_ = json.Unmarshal(tgB.Body.Bytes(), &tgBOut)
	tgBARN, _ := tgBOut["TargetGroups"].([]any)[0].(map[string]any)["TargetGroupArn"].(string)

	listener := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.CreateListener", "elasticloadbalancing", map[string]any{
		"LoadBalancerArn": lbARN,
		"Protocol":        "HTTP",
		"Port":            80,
		"DefaultActions":  []map[string]any{{"Type": "forward", "TargetGroupArn": tgBARN}},
	}, now)
	if listener.Code != http.StatusOK {
		t.Fatalf("CreateListener status=%d body=%q", listener.Code, listener.Body.String())
	}
	var listenerOut map[string]any
	_ = json.Unmarshal(listener.Body.Bytes(), &listenerOut)
	listenerARN, _ := listenerOut["Listeners"].([]any)[0].(map[string]any)["ListenerArn"].(string)

	mustCreateIAMRole(t, handler, "elb-path-exec", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/elb-path-exec"
	for _, name := range []string{"fn-a", "fn-b"} {
		createFn := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
			"FunctionName": name,
			"Runtime":      "python3.12",
			"Role":         roleARN,
			"Handler":      "app.handler",
			"Code":         map[string]any{"ZipFile": testLambdaZipB64(t)},
		}, now)
		if createFn.Code != http.StatusOK {
			t.Fatalf("CreateFunction %s status=%d body=%q", name, createFn.Code, createFn.Body.String())
		}
		mustAddLambdaServicePermission(t, handler, name, "elasticloadbalancing.amazonaws.com", "elb-"+name, now)
	}
	fnAARN := "arn:aws:lambda:us-east-1:" + testAccountID + ":function:fn-a"
	fnBARN := "arn:aws:lambda:us-east-1:" + testAccountID + ":function:fn-b"
	regA := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.RegisterTargets", "elasticloadbalancing", map[string]any{
		"TargetGroupArn": tgAARN,
		"Targets":        []map[string]any{{"Id": fnAARN}},
	}, now)
	if regA.Code != http.StatusOK {
		t.Fatalf("RegisterTargets A status=%d body=%q", regA.Code, regA.Body.String())
	}
	regB := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.RegisterTargets", "elasticloadbalancing", map[string]any{
		"TargetGroupArn": tgBARN,
		"Targets":        []map[string]any{{"Id": fnBARN}},
	}, now)
	if regB.Code != http.StatusOK {
		t.Fatalf("RegisterTargets B status=%d body=%q", regB.Code, regB.Body.String())
	}

	createRule := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.CreateRule", "elasticloadbalancing", map[string]any{
		"ListenerArn": listenerARN,
		"Priority":    5,
		"Conditions": []map[string]any{{
			"Field": "path-pattern", "Values": []string{"/api*"},
		}},
		"Actions": []map[string]any{{
			"Type": "forward", "TargetGroupArn": tgAARN,
		}},
	}, now)
	if createRule.Code != http.StatusOK {
		t.Fatalf("CreateRule status=%d body=%q", createRule.Code, createRule.Body.String())
	}
	conflict := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.CreateRule", "elasticloadbalancing", map[string]any{
		"ListenerArn": listenerARN,
		"Priority":    5,
		"Conditions":  []map[string]any{{"Field": "path-pattern", "Values": []string{"/z*"}}},
		"Actions":     []map[string]any{{"Type": "forward", "TargetGroupArn": tgAARN}},
	}, now)
	if conflict.Code == http.StatusOK || !strings.Contains(conflict.Body.String(), "PriorityInUse") {
		t.Fatalf("want PriorityInUse, got status=%d body=%q", conflict.Code, conflict.Body.String())
	}

	desc := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.DescribeRules", "elasticloadbalancing", map[string]any{
		"ListenerArn": listenerARN,
	}, now)
	if desc.Code != http.StatusOK || !strings.Contains(desc.Body.String(), `"/api*"`) {
		t.Fatalf("DescribeRules status=%d body=%q", desc.Code, desc.Body.String())
	}

	apiPath := "/alb/" + testAccountID + "/path-alb/80/api/x"
	apiRec := httptest.NewRecorder()
	handler.ServeHTTP(apiRec, mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566"+apiPath, []byte(`{}`)))
	if apiRec.Code != http.StatusOK {
		t.Fatalf("api invoke status=%d body=%q", apiRec.Code, apiRec.Body.String())
	}

	otherPath := "/alb/" + testAccountID + "/path-alb/80/other"
	otherRec := httptest.NewRecorder()
	handler.ServeHTTP(otherRec, mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566"+otherPath, []byte(`{}`)))
	if otherRec.Code != http.StatusOK {
		t.Fatalf("default invoke status=%d body=%q", otherRec.Code, otherRec.Body.String())
	}

	if len(gotEvents) != 2 {
		t.Fatalf("invokes=%d want 2 events=%v", len(gotEvents), gotEvents)
	}
	if !strings.HasPrefix(gotEvents[0], "fn-a|") || !strings.Contains(gotEvents[0], tgAARN) {
		t.Fatalf("api path should hit TG-A / fn-a: %s", gotEvents[0])
	}
	if !strings.HasPrefix(gotEvents[1], "fn-b|") || !strings.Contains(gotEvents[1], tgBARN) {
		t.Fatalf("other path should hit TG-B / fn-b: %s", gotEvents[1])
	}

	var createOut map[string]any
	_ = json.Unmarshal(createRule.Body.Bytes(), &createOut)
	ruleARN, _ := createOut["Rules"].([]any)[0].(map[string]any)["RuleArn"].(string)
	del := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.DeleteRule", "elasticloadbalancing", map[string]any{
		"RuleArn": ruleARN,
	}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteRule status=%d body=%q", del.Code, del.Body.String())
	}
}

func TestELBv2HostHeaderRuleLabListenerSelectsTargetGroup(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	var gotEvents []string
	srv.SetLambdaInvokeHookForTest(func(_ context.Context, _, name string, _ store.LambdaFunction, _, eventJSON string) ([]byte, error) {
		gotEvents = append(gotEvents, name+"|"+eventJSON)
		return []byte(`{"statusCode":200,"headers":{"content-type":"text/plain"},"body":"ok"}`), nil
	})

	lb := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.CreateLoadBalancer", "elasticloadbalancing", map[string]any{
		"Name": "host-alb",
	}, now)
	var lbOut map[string]any
	_ = json.Unmarshal(lb.Body.Bytes(), &lbOut)
	lbARN, _ := lbOut["LoadBalancers"].([]any)[0].(map[string]any)["LoadBalancerArn"].(string)

	tgHost := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.CreateTargetGroup", "elasticloadbalancing", map[string]any{
		"Name": "host-tg", "TargetType": "lambda",
	}, now)
	var tgHostOut map[string]any
	_ = json.Unmarshal(tgHost.Body.Bytes(), &tgHostOut)
	tgHostARN, _ := tgHostOut["TargetGroups"].([]any)[0].(map[string]any)["TargetGroupArn"].(string)

	tgBoth := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.CreateTargetGroup", "elasticloadbalancing", map[string]any{
		"Name": "both-tg", "TargetType": "lambda",
	}, now)
	var tgBothOut map[string]any
	_ = json.Unmarshal(tgBoth.Body.Bytes(), &tgBothOut)
	tgBothARN, _ := tgBothOut["TargetGroups"].([]any)[0].(map[string]any)["TargetGroupArn"].(string)

	tgDef := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.CreateTargetGroup", "elasticloadbalancing", map[string]any{
		"Name": "def-tg", "TargetType": "lambda",
	}, now)
	var tgDefOut map[string]any
	_ = json.Unmarshal(tgDef.Body.Bytes(), &tgDefOut)
	tgDefARN, _ := tgDefOut["TargetGroups"].([]any)[0].(map[string]any)["TargetGroupArn"].(string)

	listener := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.CreateListener", "elasticloadbalancing", map[string]any{
		"LoadBalancerArn": lbARN,
		"Protocol":        "HTTP",
		"Port":            80,
		"DefaultActions":  []map[string]any{{"Type": "forward", "TargetGroupArn": tgDefARN}},
	}, now)
	if listener.Code != http.StatusOK {
		t.Fatalf("CreateListener status=%d body=%q", listener.Code, listener.Body.String())
	}
	var listenerOut map[string]any
	_ = json.Unmarshal(listener.Body.Bytes(), &listenerOut)
	listenerARN, _ := listenerOut["Listeners"].([]any)[0].(map[string]any)["ListenerArn"].(string)

	mustCreateIAMRole(t, handler, "elb-host-exec", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/elb-host-exec"
	for _, name := range []string{"fn-host", "fn-both", "fn-def"} {
		createFn := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
			"FunctionName": name,
			"Runtime":      "python3.12",
			"Role":         roleARN,
			"Handler":      "app.handler",
			"Code":         map[string]any{"ZipFile": testLambdaZipB64(t)},
		}, now)
		if createFn.Code != http.StatusOK {
			t.Fatalf("CreateFunction %s status=%d body=%q", name, createFn.Code, createFn.Body.String())
		}
		mustAddLambdaServicePermission(t, handler, name, "elasticloadbalancing.amazonaws.com", "elb-"+name, now)
	}
	reg := func(tgARN, fn string) {
		t.Helper()
		fnARN := "arn:aws:lambda:us-east-1:" + testAccountID + ":function:" + fn
		out := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.RegisterTargets", "elasticloadbalancing", map[string]any{
			"TargetGroupArn": tgARN,
			"Targets":        []map[string]any{{"Id": fnARN}},
		}, now)
		if out.Code != http.StatusOK {
			t.Fatalf("RegisterTargets %s status=%d body=%q", fn, out.Code, out.Body.String())
		}
	}
	reg(tgHostARN, "fn-host")
	reg(tgBothARN, "fn-both")
	reg(tgDefARN, "fn-def")

	hostRule := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.CreateRule", "elasticloadbalancing", map[string]any{
		"ListenerArn": listenerARN,
		"Priority":    5,
		"Conditions": []map[string]any{{
			"Field": "host-header", "Values": []string{"api.example.com"},
		}},
		"Actions": []map[string]any{{"Type": "forward", "TargetGroupArn": tgHostARN}},
	}, now)
	if hostRule.Code != http.StatusOK {
		t.Fatalf("CreateRule host-only status=%d body=%q", hostRule.Code, hostRule.Body.String())
	}
	bothRule := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.CreateRule", "elasticloadbalancing", map[string]any{
		"ListenerArn": listenerARN,
		"Priority":    10,
		"Conditions": []map[string]any{
			{"Field": "host-header", "Values": []string{"lab.*"}},
			{"Field": "path-pattern", "Values": []string{"/api*"}},
		},
		"Actions": []map[string]any{{"Type": "forward", "TargetGroupArn": tgBothARN}},
	}, now)
	if bothRule.Code != http.StatusOK {
		t.Fatalf("CreateRule path+host status=%d body=%q", bothRule.Code, bothRule.Body.String())
	}

	desc := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.DescribeRules", "elasticloadbalancing", map[string]any{
		"ListenerArn": listenerARN,
	}, now)
	if desc.Code != http.StatusOK || !strings.Contains(desc.Body.String(), "host-header") || !strings.Contains(desc.Body.String(), "api.example.com") {
		t.Fatalf("DescribeRules status=%d body=%q", desc.Code, desc.Body.String())
	}

	invoke := func(path, host string) {
		t.Helper()
		req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566"+path, []byte(`{}`))
		req.Host = host
		req.Header.Set("Host", host)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("invoke path=%s host=%s status=%d body=%q", path, host, rec.Code, rec.Body.String())
		}
	}

	base := "/alb/" + testAccountID + "/host-alb/80"
	invoke(base+"/x", "api.example.com")
	invoke(base+"/api/x", "lab.example.com")
	invoke(base+"/api/x", "other.example.com")

	if len(gotEvents) != 3 {
		t.Fatalf("invokes=%d want 3 events=%v", len(gotEvents), gotEvents)
	}
	if !strings.HasPrefix(gotEvents[0], "fn-host|") || !strings.Contains(gotEvents[0], tgHostARN) {
		t.Fatalf("host-only should hit fn-host: %s", gotEvents[0])
	}
	if !strings.HasPrefix(gotEvents[1], "fn-both|") || !strings.Contains(gotEvents[1], tgBothARN) {
		t.Fatalf("path+host should hit fn-both: %s", gotEvents[1])
	}
	if !strings.HasPrefix(gotEvents[2], "fn-def|") || !strings.Contains(gotEvents[2], tgDefARN) {
		t.Fatalf("default should hit fn-def: %s", gotEvents[2])
	}
}

func TestELBv2ModifyAndRicherConditionsHandlers(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	var gotEvents []string
	srv.SetLambdaInvokeHookForTest(func(_ context.Context, _, name string, _ store.LambdaFunction, _, eventJSON string) ([]byte, error) {
		gotEvents = append(gotEvents, name+"|"+eventJSON)
		return []byte(`{"statusCode":200,"headers":{"content-type":"text/plain"},"body":"ok"}`), nil
	})

	lb := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.CreateLoadBalancer", "elasticloadbalancing", map[string]any{
		"Name": "mod-rich-alb",
	}, now)
	var lbOut map[string]any
	_ = json.Unmarshal(lb.Body.Bytes(), &lbOut)
	lbARN, _ := lbOut["LoadBalancers"].([]any)[0].(map[string]any)["LoadBalancerArn"].(string)

	tgA := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.CreateTargetGroup", "elasticloadbalancing", map[string]any{
		"Name": "mod-rich-tg-a", "TargetType": "lambda",
	}, now)
	var tgAOut map[string]any
	_ = json.Unmarshal(tgA.Body.Bytes(), &tgAOut)
	tgAARN, _ := tgAOut["TargetGroups"].([]any)[0].(map[string]any)["TargetGroupArn"].(string)

	tgB := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.CreateTargetGroup", "elasticloadbalancing", map[string]any{
		"Name": "mod-rich-tg-b", "TargetType": "lambda",
	}, now)
	var tgBOut map[string]any
	_ = json.Unmarshal(tgB.Body.Bytes(), &tgBOut)
	tgBARN, _ := tgBOut["TargetGroups"].([]any)[0].(map[string]any)["TargetGroupArn"].(string)

	listener := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.CreateListener", "elasticloadbalancing", map[string]any{
		"LoadBalancerArn": lbARN,
		"Protocol":        "HTTP",
		"Port":            80,
		"DefaultActions":  []map[string]any{{"Type": "forward", "TargetGroupArn": tgBARN}},
	}, now)
	if listener.Code != http.StatusOK {
		t.Fatalf("CreateListener status=%d body=%q", listener.Code, listener.Body.String())
	}
	var listenerOut map[string]any
	_ = json.Unmarshal(listener.Body.Bytes(), &listenerOut)
	listenerARN, _ := listenerOut["Listeners"].([]any)[0].(map[string]any)["ListenerArn"].(string)

	mustCreateIAMRole(t, handler, "elb-mod-exec", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/elb-mod-exec"
	for _, name := range []string{"fn-mod-a", "fn-mod-b"} {
		createFn := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
			"FunctionName": name,
			"Runtime":      "python3.12",
			"Role":         roleARN,
			"Handler":      "app.handler",
			"Code":         map[string]any{"ZipFile": testLambdaZipB64(t)},
		}, now)
		if createFn.Code != http.StatusOK {
			t.Fatalf("CreateFunction %s status=%d body=%q", name, createFn.Code, createFn.Body.String())
		}
		mustAddLambdaServicePermission(t, handler, name, "elasticloadbalancing.amazonaws.com", "elb-"+name, now)
	}
	reg := func(tgARN, fn string) {
		t.Helper()
		fnARN := "arn:aws:lambda:us-east-1:" + testAccountID + ":function:" + fn
		out := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.RegisterTargets", "elasticloadbalancing", map[string]any{
			"TargetGroupArn": tgARN,
			"Targets":        []map[string]any{{"Id": fnARN}},
		}, now)
		if out.Code != http.StatusOK {
			t.Fatalf("RegisterTargets status=%d body=%q", out.Code, out.Body.String())
		}
	}
	reg(tgAARN, "fn-mod-a")
	reg(tgBARN, "fn-mod-b")

	createRule := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.CreateRule", "elasticloadbalancing", map[string]any{
		"ListenerArn": listenerARN,
		"Priority":    5,
		"Conditions": []map[string]any{{
			"Field": "http-header",
			"HttpHeaderConfig": map[string]any{
				"HttpHeaderName": "X-Route",
				"Values":         []string{"alpha"},
			},
		}},
		"Actions": []map[string]any{{"Type": "forward", "TargetGroupArn": tgAARN}},
	}, now)
	if createRule.Code != http.StatusOK {
		t.Fatalf("CreateRule http-header status=%d body=%q", createRule.Code, createRule.Body.String())
	}
	var ruleOut map[string]any
	_ = json.Unmarshal(createRule.Body.Bytes(), &ruleOut)
	ruleARN, _ := ruleOut["Rules"].([]any)[0].(map[string]any)["RuleArn"].(string)

	modRule := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.ModifyRule", "elasticloadbalancing", map[string]any{
		"RuleArn": ruleARN,
		"Conditions": []map[string]any{
			{
				"Field": "http-header",
				"HttpHeaderConfig": map[string]any{
					"HttpHeaderName": "X-Route",
					"Values":         []string{"beta"},
				},
			},
			{
				"Field": "query-string",
				"QueryStringConfig": map[string]any{
					"Values": []map[string]any{{"Key": "env", "Value": "lab"}},
				},
			},
		},
		"Actions": []map[string]any{{"Type": "forward", "TargetGroupArn": tgAARN}},
	}, now)
	if modRule.Code != http.StatusOK || !strings.Contains(modRule.Body.String(), "query-string") {
		t.Fatalf("ModifyRule status=%d body=%q", modRule.Code, modRule.Body.String())
	}

	modListener := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.ModifyListener", "elasticloadbalancing", map[string]any{
		"ListenerArn":    listenerARN,
		"DefaultActions": []map[string]any{{"Type": "forward", "TargetGroupArn": tgBARN}},
	}, now)
	if modListener.Code != http.StatusOK {
		t.Fatalf("ModifyListener status=%d body=%q", modListener.Code, modListener.Body.String())
	}

	qsRule := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.CreateRule", "elasticloadbalancing", map[string]any{
		"ListenerArn": listenerARN,
		"Priority":    10,
		"Conditions": []map[string]any{{
			"Field": "source-ip",
			"SourceIpConfig": map[string]any{
				"Values": []string{"127.0.0.0/8"},
			},
		}},
		"Actions": []map[string]any{{"Type": "forward", "TargetGroupArn": tgAARN}},
	}, now)
	if qsRule.Code != http.StatusOK {
		t.Fatalf("CreateRule source-ip status=%d body=%q", qsRule.Code, qsRule.Body.String())
	}

	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/alb/"+testAccountID+"/mod-rich-alb/80/?env=lab", []byte(`{}`))
	req.Header.Set("X-Route", "beta")
	req.RemoteAddr = "127.0.0.1:54321"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("invoke richer conditions status=%d body=%q", rec.Code, rec.Body.String())
	}
	if len(gotEvents) != 1 || !strings.HasPrefix(gotEvents[0], "fn-mod-a|") {
		t.Fatalf("want fn-mod-a, got %v", gotEvents)
	}
}
