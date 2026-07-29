package store_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestELBv2NetworkLoadBalancerCreateDescribeDelete(t *testing.T) {
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
	lb, err := st.CreateELBv2LoadBalancer(account, "us-east-1", "lab-nlb", "internet-facing", "network")
	if err != nil {
		t.Fatal(err)
	}
	if lb.Type != "network" {
		t.Fatalf("Type=%q want network", lb.Type)
	}
	if !strings.Contains(lb.ARN, "loadbalancer/net/") {
		t.Fatalf("ARN=%q want loadbalancer/net/", lb.ARN)
	}

	listed, err := st.DescribeELBv2LoadBalancers(account, []string{lb.ARN})
	if err != nil || len(listed) != 1 || listed[0].Type != "network" {
		t.Fatalf("describe=%+v err=%v", listed, err)
	}

	tg, err := st.CreateELBv2TargetGroup(account, "us-east-1", "nlb-tg", "instance", "TCP", 80)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.RegisterELBv2Targets(account, tg.ARN, []store.ELBv2Target{{ID: "i-labdeadbeef01", Port: 80}}); err != nil {
		t.Fatal(err)
	}

	listener, err := st.CreateELBv2Listener(account, "us-east-1", lb.ARN, tg.ARN, "TCP", 80)
	if err != nil || listener.Protocol != "TCP" {
		t.Fatalf("listener=%+v err=%v", listener, err)
	}
	if !strings.Contains(listener.ListenerARN, ":listener/net/") {
		t.Fatalf("listener ARN=%q want listener/net/", listener.ListenerARN)
	}

	if _, err := st.CreateELBv2Listener(account, "us-east-1", lb.ARN, tg.ARN, "HTTP", 8080); !errors.Is(err, store.ErrELBv2BadRequest) {
		t.Fatalf("HTTP on NLB want ValidationError got %v", err)
	}

	tlsTG, err := st.CreateELBv2TargetGroup(account, "us-east-1", "nlb-tls-tg", "ip", "TLS", 443)
	if err != nil {
		t.Fatal(err)
	}
	tlsListener, err := st.CreateELBv2Listener(account, "us-east-1", lb.ARN, tlsTG.ARN, "TLS", 443)
	if err != nil || tlsListener.Protocol != "TLS" {
		t.Fatalf("TLS listener=%+v err=%v", tlsListener, err)
	}

	if _, err := st.CreateELBv2Rule(account, "us-east-1", listener.ListenerARN, tg.ARN, 10, store.ELBv2RuleConditions{PathPatterns: []string{"/x*"}}); !errors.Is(err, store.ErrELBv2BadRequest) {
		t.Fatalf("CreateRule on NLB want ValidationError got %v", err)
	}

	health, err := st.DescribeELBv2TargetHealth(account, tg.ARN)
	if err != nil || len(health) != 1 || health[0].State != "healthy" {
		t.Fatalf("health=%+v err=%v want healthy", health, err)
	}

	if err := st.DeleteELBv2LoadBalancer(account, lb.ARN); err != nil {
		t.Fatal(err)
	}
	after, err := st.DescribeELBv2LoadBalancers(account, []string{lb.ARN})
	if err != nil || len(after) != 0 {
		t.Fatalf("after delete=%+v err=%v", after, err)
	}
}

func TestELBv2RejectInvalidLoadBalancerType(t *testing.T) {
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
	if _, err := st.CreateELBv2LoadBalancer(account, "us-east-1", "bad-gw", "internet-facing", "gateway"); !errors.Is(err, store.ErrELBv2BadRequest) {
		t.Fatalf("gateway type want ValidationError got %v", err)
	}
	if _, err := st.CreateELBv2LoadBalancer(account, "us-east-1", "bad-classic", "internet-facing", "classic"); !errors.Is(err, store.ErrELBv2BadRequest) {
		t.Fatalf("classic type want ValidationError got %v", err)
	}

	alb, err := st.CreateELBv2LoadBalancer(account, "us-east-1", "default-alb", "internet-facing", "")
	if err != nil || alb.Type != "application" {
		t.Fatalf("empty Type want application got %+v err=%v", alb, err)
	}
	if !strings.Contains(alb.ARN, "loadbalancer/app/") {
		t.Fatalf("ALB ARN=%q", alb.ARN)
	}
}
