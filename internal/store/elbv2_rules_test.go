package store_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestELBv2PathRuleMatchAndPriority(t *testing.T) {
	dir := t.TempDir()
	key, err := store.LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	account := "000000000001"
	lb, err := st.CreateELBv2LoadBalancer(account, "us-east-1", "rules-alb", "internet-facing", "application")
	if err != nil {
		t.Fatal(err)
	}
	tgA, err := st.CreateELBv2TargetGroup(account, "us-east-1", "tg-a", "lambda", "HTTP", 80)
	if err != nil {
		t.Fatal(err)
	}
	tgB, err := st.CreateELBv2TargetGroup(account, "us-east-1", "tg-b", "lambda", "HTTP", 80)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := st.CreateELBv2Listener(account, "us-east-1", lb.ARN, tgB.ARN, "HTTP", 80)
	if err != nil {
		t.Fatal(err)
	}

	rule, err := st.CreateELBv2Rule(account, "us-east-1", listener.ListenerARN, tgA.ARN, 10, store.ELBv2RuleConditions{PathPatterns: []string{"/api*"}})
	if err != nil {
		t.Fatal(err)
	}
	if rule.RuleARN == "" || rule.Priority != 10 {
		t.Fatalf("rule=%+v", rule)
	}

	if _, err := st.CreateELBv2Rule(account, "us-east-1", listener.ListenerARN, tgA.ARN, 10, store.ELBv2RuleConditions{PathPatterns: []string{"/other*"}}); !errors.Is(err, store.ErrELBv2PriorityInUse) {
		t.Fatalf("want PriorityInUse, got %v", err)
	}
	if _, err := st.CreateELBv2Rule(account, "us-east-1", listener.ListenerARN, "arn:aws:elasticloadbalancing:us-east-1:"+account+":targetgroup/missing/x", 20, store.ELBv2RuleConditions{PathPatterns: []string{"/x*"}}); !errors.Is(err, store.ErrELBv2TGNotFound) {
		t.Fatalf("want TargetGroupNotFound, got %v", err)
	}
	if _, err := st.CreateELBv2Rule(account, "us-east-1", listener.ListenerARN, tgA.ARN, 30, store.ELBv2RuleConditions{PathPatterns: []string{"/bad*mid*"}}); !errors.Is(err, store.ErrELBv2BadRequest) {
		t.Fatalf("want ValidationError for multi-*, got %v", err)
	}
	if _, err := st.CreateELBv2Rule(account, "us-east-1", listener.ListenerARN, tgA.ARN, 40, store.ELBv2RuleConditions{}); !errors.Is(err, store.ErrELBv2BadRequest) {
		t.Fatalf("want ValidationError for empty conditions, got %v", err)
	}

	gotAPI, err := st.ResolveELBv2ListenerTargetGroup(account, listener, store.ELBv2RuleMatchInput{Path: "/api/x"})
	if err != nil || gotAPI != tgA.ARN {
		t.Fatalf("api path tg=%q err=%v want %q", gotAPI, err, tgA.ARN)
	}
	gotOther, err := st.ResolveELBv2ListenerTargetGroup(account, listener, store.ELBv2RuleMatchInput{Path: "/other"})
	if err != nil || gotOther != tgB.ARN {
		t.Fatalf("default path tg=%q err=%v want %q", gotOther, err, tgB.ARN)
	}
	if !store.MatchELBv2PathPattern("/foo", "/foo") || store.MatchELBv2PathPattern("/foo/x", "/foo") {
		t.Fatal("exact match broken")
	}

	listed, err := st.DescribeELBv2Rules(account, listener.ListenerARN, nil)
	if err != nil || len(listed) != 1 || listed[0].RuleARN != rule.RuleARN {
		t.Fatalf("describe=%v err=%v", listed, err)
	}
	if err := st.DeleteELBv2Rule(account, rule.RuleARN); err != nil {
		t.Fatal(err)
	}
	listed, err = st.DescribeELBv2Rules(account, listener.ListenerARN, nil)
	if err != nil || len(listed) != 0 {
		t.Fatalf("after delete describe=%v err=%v", listed, err)
	}
}

func TestELBv2HostHeaderRuleMatchAndPriority(t *testing.T) {
	dir := t.TempDir()
	key, err := store.LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	account := "000000000001"
	lb, err := st.CreateELBv2LoadBalancer(account, "us-east-1", "host-alb", "internet-facing", "application")
	if err != nil {
		t.Fatal(err)
	}
	tgHost, err := st.CreateELBv2TargetGroup(account, "us-east-1", "tg-host", "lambda", "HTTP", 80)
	if err != nil {
		t.Fatal(err)
	}
	tgBoth, err := st.CreateELBv2TargetGroup(account, "us-east-1", "tg-both", "lambda", "HTTP", 80)
	if err != nil {
		t.Fatal(err)
	}
	tgDefault, err := st.CreateELBv2TargetGroup(account, "us-east-1", "tg-def", "lambda", "HTTP", 80)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := st.CreateELBv2Listener(account, "us-east-1", lb.ARN, tgDefault.ARN, "HTTP", 80)
	if err != nil {
		t.Fatal(err)
	}

	hostOnly, err := st.CreateELBv2Rule(account, "us-east-1", listener.ListenerARN, tgHost.ARN, 5, store.ELBv2RuleConditions{HostHeaders: []string{"api.example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(hostOnly.HostHeaders) != 1 || hostOnly.HostHeaders[0] != "api.example.com" || len(hostOnly.PathPatterns) != 0 {
		t.Fatalf("host-only rule=%+v", hostOnly)
	}
	both, err := st.CreateELBv2Rule(account, "us-east-1", listener.ListenerARN, tgBoth.ARN, 10, store.ELBv2RuleConditions{
		PathPatterns: []string{"/api*"}, HostHeaders: []string{"api.*"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(both.PathPatterns) != 1 || len(both.HostHeaders) != 1 {
		t.Fatalf("path+host rule=%+v", both)
	}
	if _, err := st.CreateELBv2Rule(account, "us-east-1", listener.ListenerARN, tgHost.ARN, 20, store.ELBv2RuleConditions{HostHeaders: []string{"bad*mid*"}}); !errors.Is(err, store.ErrELBv2BadRequest) {
		t.Fatalf("want ValidationError for multi-* host, got %v", err)
	}

	if !store.MatchELBv2HostHeader("API.Example.COM:443", "api.example.com") {
		t.Fatal("exact host match should be case-insensitive and strip port")
	}
	if !store.MatchELBv2HostHeader("api.lab.local", "api.*") {
		t.Fatal("trailing * host prefix should match")
	}
	if store.MatchELBv2HostHeader("other.example.com", "api.example.com") {
		t.Fatal("non-matching host should fail")
	}

	// Priority 5 host-only wins over priority 10 path+host when host matches.
	gotHost, err := st.ResolveELBv2ListenerTargetGroup(account, listener, store.ELBv2RuleMatchInput{Path: "/other", Host: "api.example.com"})
	if err != nil || gotHost != tgHost.ARN {
		t.Fatalf("host-only tg=%q err=%v want %q", gotHost, err, tgHost.ARN)
	}
	// Path+host AND: both must match; wrong host falls through to default.
	gotMiss, err := st.ResolveELBv2ListenerTargetGroup(account, listener, store.ELBv2RuleMatchInput{Path: "/api/x", Host: "other.example.com"})
	if err != nil || gotMiss != tgDefault.ARN {
		t.Fatalf("path without host tg=%q err=%v want default %q", gotMiss, err, tgDefault.ARN)
	}
	// Host-only does not match; path+host matches on priority 10.
	gotBoth, err := st.ResolveELBv2ListenerTargetGroup(account, listener, store.ELBv2RuleMatchInput{Path: "/api/x", Host: "api.lab.local"})
	if err != nil || gotBoth != tgBoth.ARN {
		t.Fatalf("path+host tg=%q err=%v want %q", gotBoth, err, tgBoth.ARN)
	}
	gotDefault, err := st.ResolveELBv2ListenerTargetGroup(account, listener, store.ELBv2RuleMatchInput{Path: "/other", Host: "other.example.com"})
	if err != nil || gotDefault != tgDefault.ARN {
		t.Fatalf("default tg=%q err=%v want %q", gotDefault, err, tgDefault.ARN)
	}

	listed, err := st.DescribeELBv2Rules(account, listener.ListenerARN, nil)
	if err != nil || len(listed) != 2 {
		t.Fatalf("describe=%v err=%v", listed, err)
	}
	if listed[0].Priority != 5 || len(listed[0].HostHeaders) != 1 || len(listed[0].PathPatterns) != 0 {
		t.Fatalf("first rule=%+v", listed[0])
	}
	if listed[1].Priority != 10 || len(listed[1].HostHeaders) != 1 || len(listed[1].PathPatterns) != 1 {
		t.Fatalf("second rule=%+v", listed[1])
	}
}

func TestELBv2ModifyListenerAndRule(t *testing.T) {
	dir := t.TempDir()
	key, err := store.LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	account := "000000000001"
	lb, err := st.CreateELBv2LoadBalancer(account, "us-east-1", "mod-alb", "internet-facing", "application")
	if err != nil {
		t.Fatal(err)
	}
	tgA, err := st.CreateELBv2TargetGroup(account, "us-east-1", "mod-tg-a", "lambda", "HTTP", 80)
	if err != nil {
		t.Fatal(err)
	}
	tgB, err := st.CreateELBv2TargetGroup(account, "us-east-1", "mod-tg-b", "lambda", "HTTP", 80)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := st.CreateELBv2Listener(account, "us-east-1", lb.ARN, tgA.ARN, "HTTP", 80)
	if err != nil {
		t.Fatal(err)
	}
	newPort := 8080
	newProto := "HTTPS"
	modL, err := st.ModifyELBv2Listener(account, listener.ListenerARN, &newPort, &newProto, &tgB.ARN)
	if err != nil {
		t.Fatal(err)
	}
	if modL.Port != 8080 || modL.Protocol != "HTTPS" || modL.TargetGroupARN != tgB.ARN {
		t.Fatalf("modified listener=%+v", modL)
	}
	gotL, err := st.GetELBv2ListenerByPort(account, lb.ARN, 8080)
	if err != nil || gotL.TargetGroupARN != tgB.ARN {
		t.Fatalf("get by new port=%+v err=%v", gotL, err)
	}

	rule, err := st.CreateELBv2Rule(account, "us-east-1", listener.ListenerARN, tgA.ARN, 10, store.ELBv2RuleConditions{
		PathPatterns: []string{"/old*"},
	})
	if err != nil {
		t.Fatal(err)
	}
	modRule, err := st.ModifyELBv2Rule(account, rule.RuleARN, &store.ELBv2RuleConditions{
		PathPatterns: []string{"/new*"},
		HTTPHeaders: []store.ELBv2HTTPHeaderCond{{
			Name: "X-Route", Values: []string{"alpha"},
		}},
	}, &tgB.ARN)
	if err != nil {
		t.Fatal(err)
	}
	if modRule.TargetGroupARN != tgB.ARN || len(modRule.PathPatterns) != 1 || modRule.PathPatterns[0] != "/new*" {
		t.Fatalf("modified rule=%+v", modRule)
	}
	if len(modRule.HTTPHeaders) != 1 || modRule.HTTPHeaders[0].Name != "X-Route" {
		t.Fatalf("http-header not stored: %+v", modRule)
	}

	hit, err := st.ResolveELBv2ListenerTargetGroup(account, modL, store.ELBv2RuleMatchInput{
		Path: "/new/x", Headers: map[string]string{"x-route": "alpha"},
	})
	if err != nil || hit != tgB.ARN {
		t.Fatalf("resolve after modify tg=%q err=%v", hit, err)
	}
	miss, err := st.ResolveELBv2ListenerTargetGroup(account, modL, store.ELBv2RuleMatchInput{Path: "/old/x"})
	if err != nil || miss != tgB.ARN {
		t.Fatalf("default after modify tg=%q err=%v want %q", miss, err, tgB.ARN)
	}
}

func TestELBv2RicherConditionMatch(t *testing.T) {
	dir := t.TempDir()
	key, err := store.LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	account := "000000000001"
	lb, err := st.CreateELBv2LoadBalancer(account, "us-east-1", "rich-alb", "internet-facing", "application")
	if err != nil {
		t.Fatal(err)
	}
	tgHdr, err := st.CreateELBv2TargetGroup(account, "us-east-1", "tg-hdr", "lambda", "HTTP", 80)
	if err != nil {
		t.Fatal(err)
	}
	tgQS, err := st.CreateELBv2TargetGroup(account, "us-east-1", "tg-qs", "lambda", "HTTP", 80)
	if err != nil {
		t.Fatal(err)
	}
	tgIP, err := st.CreateELBv2TargetGroup(account, "us-east-1", "tg-ip", "lambda", "HTTP", 80)
	if err != nil {
		t.Fatal(err)
	}
	tgDef, err := st.CreateELBv2TargetGroup(account, "us-east-1", "tg-rich-def", "lambda", "HTTP", 80)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := st.CreateELBv2Listener(account, "us-east-1", lb.ARN, tgDef.ARN, "HTTP", 80)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := st.CreateELBv2Rule(account, "us-east-1", listener.ListenerARN, tgHdr.ARN, 5, store.ELBv2RuleConditions{
		HTTPHeaders: []store.ELBv2HTTPHeaderCond{{Name: "User-Agent", Values: []string{"*Chrome*"}}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateELBv2Rule(account, "us-east-1", listener.ListenerARN, tgQS.ARN, 10, store.ELBv2RuleConditions{
		QueryStrings: []store.ELBv2QueryStringCond{{Key: "version", Value: "v1"}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateELBv2Rule(account, "us-east-1", listener.ListenerARN, tgIP.ARN, 15, store.ELBv2RuleConditions{
		SourceIPs: []string{"192.0.2.0/24"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateELBv2Rule(account, "us-east-1", listener.ListenerARN, tgHdr.ARN, 20, store.ELBv2RuleConditions{
		SourceIPs: []string{"255.255.255.255/32"},
	}); !errors.Is(err, store.ErrELBv2BadRequest) {
		t.Fatalf("want ValidationError for 255.255.255.255/32, got %v", err)
	}

	if !store.MatchELBv2LiteWildcard("Mozilla Chrome Safari", "*Chrome*", true) {
		t.Fatal("*contains* wildcard broken")
	}
	if !store.MatchELBv2QueryString(map[string]string{"version": "v1"}, store.ELBv2QueryStringCond{Key: "version", Value: "v1"}) {
		t.Fatal("query key/value match broken")
	}
	if !store.MatchELBv2SourceIP("192.0.2.44", []string{"192.0.2.0/24"}) {
		t.Fatal("source-ip CIDR match broken")
	}

	gotHdr, err := st.ResolveELBv2ListenerTargetGroup(account, listener, store.ELBv2RuleMatchInput{
		Path: "/", Headers: map[string]string{"user-agent": "Mozilla/5.0 Chrome/120"},
	})
	if err != nil || gotHdr != tgHdr.ARN {
		t.Fatalf("http-header tg=%q err=%v", gotHdr, err)
	}
	gotQS, err := st.ResolveELBv2ListenerTargetGroup(account, listener, store.ELBv2RuleMatchInput{
		Path: "/", Query: map[string]string{"version": "v1"},
	})
	if err != nil || gotQS != tgQS.ARN {
		t.Fatalf("query-string tg=%q err=%v", gotQS, err)
	}
	gotIP, err := st.ResolveELBv2ListenerTargetGroup(account, listener, store.ELBv2RuleMatchInput{
		Path: "/", SourceIP: "192.0.2.10",
	})
	if err != nil || gotIP != tgIP.ARN {
		t.Fatalf("source-ip tg=%q err=%v", gotIP, err)
	}
	gotDef, err := st.ResolveELBv2ListenerTargetGroup(account, listener, store.ELBv2RuleMatchInput{
		Path: "/", SourceIP: "203.0.113.1",
	})
	if err != nil || gotDef != tgDef.ARN {
		t.Fatalf("default tg=%q err=%v", gotDef, err)
	}
}
