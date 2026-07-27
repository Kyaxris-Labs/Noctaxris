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

	rule, err := st.CreateELBv2Rule(account, "us-east-1", listener.ListenerARN, tgA.ARN, 10, []string{"/api*"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rule.RuleARN == "" || rule.Priority != 10 {
		t.Fatalf("rule=%+v", rule)
	}

	if _, err := st.CreateELBv2Rule(account, "us-east-1", listener.ListenerARN, tgA.ARN, 10, []string{"/other*"}, nil); !errors.Is(err, store.ErrELBv2PriorityInUse) {
		t.Fatalf("want PriorityInUse, got %v", err)
	}
	if _, err := st.CreateELBv2Rule(account, "us-east-1", listener.ListenerARN, "arn:aws:elasticloadbalancing:us-east-1:"+account+":targetgroup/missing/x", 20, []string{"/x*"}, nil); !errors.Is(err, store.ErrELBv2TGNotFound) {
		t.Fatalf("want TargetGroupNotFound, got %v", err)
	}
	if _, err := st.CreateELBv2Rule(account, "us-east-1", listener.ListenerARN, tgA.ARN, 30, []string{"/bad*mid*"}, nil); !errors.Is(err, store.ErrELBv2BadRequest) {
		t.Fatalf("want ValidationError for multi-*, got %v", err)
	}
	if _, err := st.CreateELBv2Rule(account, "us-east-1", listener.ListenerARN, tgA.ARN, 40, nil, nil); !errors.Is(err, store.ErrELBv2BadRequest) {
		t.Fatalf("want ValidationError for empty conditions, got %v", err)
	}

	gotAPI, err := st.ResolveELBv2ListenerTargetGroup(account, listener, "/api/x", "")
	if err != nil || gotAPI != tgA.ARN {
		t.Fatalf("api path tg=%q err=%v want %q", gotAPI, err, tgA.ARN)
	}
	gotOther, err := st.ResolveELBv2ListenerTargetGroup(account, listener, "/other", "")
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

	hostOnly, err := st.CreateELBv2Rule(account, "us-east-1", listener.ListenerARN, tgHost.ARN, 5, nil, []string{"api.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if len(hostOnly.HostHeaders) != 1 || hostOnly.HostHeaders[0] != "api.example.com" || len(hostOnly.PathPatterns) != 0 {
		t.Fatalf("host-only rule=%+v", hostOnly)
	}
	both, err := st.CreateELBv2Rule(account, "us-east-1", listener.ListenerARN, tgBoth.ARN, 10, []string{"/api*"}, []string{"api.*"})
	if err != nil {
		t.Fatal(err)
	}
	if len(both.PathPatterns) != 1 || len(both.HostHeaders) != 1 {
		t.Fatalf("path+host rule=%+v", both)
	}
	if _, err := st.CreateELBv2Rule(account, "us-east-1", listener.ListenerARN, tgHost.ARN, 20, nil, []string{"bad*mid*"}); !errors.Is(err, store.ErrELBv2BadRequest) {
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
	gotHost, err := st.ResolveELBv2ListenerTargetGroup(account, listener, "/other", "api.example.com")
	if err != nil || gotHost != tgHost.ARN {
		t.Fatalf("host-only tg=%q err=%v want %q", gotHost, err, tgHost.ARN)
	}
	// Path+host AND: both must match; wrong host falls through to default.
	gotMiss, err := st.ResolveELBv2ListenerTargetGroup(account, listener, "/api/x", "other.example.com")
	if err != nil || gotMiss != tgDefault.ARN {
		t.Fatalf("path without host tg=%q err=%v want default %q", gotMiss, err, tgDefault.ARN)
	}
	// Host-only does not match; path+host matches on priority 10.
	gotBoth, err := st.ResolveELBv2ListenerTargetGroup(account, listener, "/api/x", "api.lab.local")
	if err != nil || gotBoth != tgBoth.ARN {
		t.Fatalf("path+host tg=%q err=%v want %q", gotBoth, err, tgBoth.ARN)
	}
	gotDefault, err := st.ResolveELBv2ListenerTargetGroup(account, listener, "/other", "other.example.com")
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
