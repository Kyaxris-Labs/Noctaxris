package store_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func elbv2OpenTestStore(t *testing.T) *store.Store {
	t.Helper()
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
	return st
}

func TestELBv2NLBForwardURLIPTarget(t *testing.T) {
	st := elbv2OpenTestStore(t)
	account := "000000000001"
	tg, err := st.CreateELBv2TargetGroup(account, "us-east-1", "nlb-ip-tg", "ip", "TCP", 8080)
	if err != nil {
		t.Fatalf("CreateELBv2TargetGroup: %v", err)
	}
	target := store.ELBv2Target{ID: "10.0.0.42", Port: 0}
	got, err := st.ELBv2NLBForwardURL(account, tg, target, "/health", "x=1")
	if err != nil {
		t.Fatalf("ELBv2NLBForwardURL: %v", err)
	}
	if !strings.HasPrefix(got, "http://10.0.0.42:8080/health?x=1") {
		t.Fatalf("url=%q", got)
	}
}

func TestELBv2NLBForwardURLDeniesLinkLocalAndUnspecified(t *testing.T) {
	st := elbv2OpenTestStore(t)
	account := "000000000001"
	tg, err := st.CreateELBv2TargetGroup(account, "us-east-1", "nlb-deny-tg", "ip", "TCP", 80)
	if err != nil {
		t.Fatalf("CreateELBv2TargetGroup: %v", err)
	}
	for _, id := range []string{"0.0.0.0", "169.254.169.254", "::"} {
		_, err := st.ELBv2NLBForwardURL(account, tg, store.ELBv2Target{ID: id}, "/", "")
		if err == nil {
			t.Fatalf("want deny for %s", id)
		}
	}
}

func TestELBv2NLBForwardURLInstanceRequiresPrivateIP(t *testing.T) {
	st := elbv2OpenTestStore(t)
	account := "000000000001"
	tg, err := st.CreateELBv2TargetGroup(account, "us-east-1", "nlb-inst-tg", "instance", "TCP", 80)
	if err != nil {
		t.Fatalf("CreateELBv2TargetGroup: %v", err)
	}
	_, err = st.ELBv2NLBForwardURL(account, tg, store.ELBv2Target{ID: "i-missing"}, "/", "")
	if err == nil || !strings.Contains(err.Error(), "not reachable") {
		t.Fatalf("want not reachable, got %v", err)
	}
}

func TestELBv2NLBForwardURLHostPortOverrideAndLambdaTargetReject(t *testing.T) {
	st := elbv2OpenTestStore(t)
	account := "000000000001"
	tg, err := st.CreateELBv2TargetGroup(account, "us-east-1", "nlb-hp-tg", "ip", "TCP", 80)
	if err != nil {
		t.Fatal(err)
	}
	got, err := st.ELBv2NLBForwardURL(account, tg, store.ELBv2Target{ID: "10.0.0.9", Port: 9090}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "http://10.0.0.9:9090/") {
		t.Fatalf("url=%q", got)
	}
	got, err = st.ELBv2NLBForwardURL(account, tg, store.ELBv2Target{ID: "10.0.0.9:7070"}, "/x", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, ":7070/") {
		t.Fatalf("hostport url=%q", got)
	}

	lambdaTG, err := st.CreateELBv2TargetGroup(account, "us-east-1", "nlb-lambda-tg", "lambda", "HTTP", 80)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.ELBv2NLBForwardURL(account, lambdaTG, store.ELBv2Target{ID: "arn:aws:lambda:us-east-1:1:function:f"}, "/", ""); err == nil {
		t.Fatal("expected lambda target type reject")
	}
}

func TestFetchELBv2LabHTTPForwardHappyAndNegatives(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Echo", r.Header.Get("X-Lab"))
		if r.Method == http.MethodPost {
			b, _ := io.ReadAll(r.Body)
			_, _ = w.Write([]byte("body:" + string(b)))
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	status, body, hdr, err := store.FetchELBv2LabHTTPForward(ctx, srv.URL+"/h", "GET", nil, http.Header{
		"X-Lab":          []string{"y"},
		"Host":           []string{"evil"},
		"Connection":     []string{"close"},
		"Content-Length": []string{"1"},
	})
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	if status != 200 || string(body) != "ok" || hdr.Get("X-Echo") != "y" {
		t.Fatalf("status=%d body=%q echo=%q", status, body, hdr.Get("X-Echo"))
	}
	status, body, _, err = store.FetchELBv2LabHTTPForward(ctx, srv.URL+"/h", "POST", []byte("hi"), nil)
	if err != nil || status != 200 || string(body) != "body:hi" {
		t.Fatalf("POST: status=%d body=%q err=%v", status, body, err)
	}

	if _, _, _, err := store.FetchELBv2LabHTTPForward(ctx, "https://example.com/", "GET", nil, nil); err == nil {
		t.Fatal("expected https reject")
	}
	if _, _, _, err := store.FetchELBv2LabHTTPForward(ctx, "http://example.com/", "GET", nil, nil); err == nil {
		t.Fatal("expected non-ip host reject")
	}
	if _, _, _, err := store.FetchELBv2LabHTTPForward(ctx, "not-a-url", "GET", nil, nil); err == nil {
		t.Fatal("expected invalid url reject")
	}
}

func TestELBv2NLBForwardURLInstanceWithPrivateIP(t *testing.T) {
	st := elbv2OpenTestStore(t)
	account := "000000000001"
	inst, err := st.RunInstances(account, "us-east-1", store.RunInstancesInput{
		ImageID: "ami-lab", MinCount: 1, MaxCount: 1,
	})
	if err != nil {
		t.Fatalf("RunInstances: %v", err)
	}
	if len(inst) != 1 {
		t.Fatalf("instances=%d", len(inst))
	}
	if err := st.SetEC2PrivateIP(account, "us-east-1", inst[0].InstanceID, "10.0.9.9"); err != nil {
		t.Fatal(err)
	}
	tg, err := st.CreateELBv2TargetGroup(account, "us-east-1", "nlb-inst-ok", "instance", "TCP", 8080)
	if err != nil {
		t.Fatal(err)
	}
	got, err := st.ELBv2NLBForwardURL(account, tg, store.ELBv2Target{ID: inst[0].InstanceID}, "/ready", "a=1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "10.0.9.9") || !strings.Contains(got, ":8080/ready?a=1") {
		t.Fatalf("url=%q", got)
	}
}
