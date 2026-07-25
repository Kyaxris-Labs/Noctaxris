package store_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestIsWAFAssociableResourceARN(t *testing.T) {
	ok := []string{
		"arn:aws:apigateway:us-east-1::/apis/abc123",
		"arn:aws:apigateway:us-east-1::/apis/abc123/stages/$default",
		"arn:aws:execute-api:us-east-1:000000000001:abc123",
		"arn:aws:execute-api:us-east-1:000000000001:abc123/$default/GET/hello",
		"arn:aws:appsync:us-east-1:000000000001:apis/gql1",
		"arn:aws:lambda:us-east-1:000000000001:function:fn",
		"arn:aws:elasticloadbalancing:us-east-1:000000000001:loadbalancer/app/lab/abc",
	}
	for _, arn := range ok {
		if !store.IsWAFAssociableResourceARN(arn) {
			t.Fatalf("expected ok: %s", arn)
		}
	}
	bad := []string{
		"",
		"not-an-arn",
		"arn:aws:s3:::bucket",
		"arn:aws:apigateway:us-east-1::/unknown/x",
		"arn:aws:apigateway:us-east-1::/restapis/api1/stages/prod",
		"arn:aws:elasticloadbalancing:us-east-1:000000000001:loadbalancer/net/lab/abc",
		"arn:aws:cognito-idp:us-east-1:000000000001:userpool/us-east-1_abc",
	}
	for _, arn := range bad {
		if store.IsWAFAssociableResourceARN(arn) {
			t.Fatalf("expected fail-closed: %s", arn)
		}
	}
}

func TestAssociateWAFWebACLHTTPAPI(t *testing.T) {
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

	acl, err := st.CreateWAFWebACL("000000000001", "us-east-1", "lab", "REGIONAL", "", "Allow", nil)
	if err != nil {
		t.Fatal(err)
	}
	httpAPI := "arn:aws:apigateway:us-east-1::/apis/httpapi1/stages/$default"
	if err := st.AssociateWAFWebACL("000000000001", acl.ARN, httpAPI); err != nil {
		t.Fatalf("associate http api: %v", err)
	}
	if err := st.AssociateWAFWebACL("000000000001", acl.ARN, "arn:aws:s3:::nope"); err == nil {
		t.Fatal("expected fail closed on unknown ResourceArn")
	}
	phantom := "arn:aws:wafv2:us-east-1:000000000001:regional/webacl/missing/00000000-0000-0000-0000-000000000000"
	if err := st.AssociateWAFWebACL("000000000001", phantom, httpAPI); !errors.Is(err, store.ErrWAFNotFound) {
		t.Fatalf("want ErrWAFNotFound for phantom ACL, got %v", err)
	}
}

func TestCloudFrontDistributionCRUD(t *testing.T) {
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
	if _, err := st.CreateBucket(account, "lab-bucket"); err != nil {
		t.Fatal(err)
	}
	api, err := st.CreateAPIGatewayAPI(account, "us-east-1", "cf-origin", "HTTP")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateCloudFrontDistribution(account, "lab", "ref-missing", true, []store.CloudFrontOrigin{
		{ID: "s3", DomainName: "missing-bucket", OriginType: "s3"},
	}); !errors.Is(err, store.ErrCloudFrontBadRequest) {
		t.Fatalf("want origin fail-closed, got %v", err)
	}
	d, err := st.CreateCloudFrontDistribution(account, "lab", "ref-1", true, []store.CloudFrontOrigin{
		{ID: "s3", DomainName: "lab-bucket", OriginType: "s3"},
		{ID: "gw", DomainName: api.APIID, OriginType: "apigateway"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.Status != store.CloudFrontStatusDeployed {
		t.Fatalf("status=%q want %q", d.Status, store.CloudFrontStatusDeployed)
	}
	if d.DomainName == "" || !strings.Contains(d.DomainName, "cloudfront.noctaxris.local") {
		t.Fatalf("DomainName=%q", d.DomainName)
	}
	got, err := st.GetCloudFrontDistribution(account, d.ID)
	if err != nil || got.Status != store.CloudFrontStatusDeployed || got.DomainName != d.DomainName {
		t.Fatalf("get: %+v err=%v", got, err)
	}
	list, err := st.ListCloudFrontDistributions(account)
	if err != nil || len(list) != 1 {
		t.Fatalf("list=%v err=%v", list, err)
	}
	if err := st.DeleteCloudFrontDistribution(account, d.ID); err != nil {
		t.Fatal(err)
	}
}

func TestELBv2LambdaTarget(t *testing.T) {
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
	lb, err := st.CreateELBv2LoadBalancer(account, "us-east-1", "lab-alb", "internet-facing")
	if err != nil {
		t.Fatal(err)
	}
	tg, err := st.CreateELBv2TargetGroup(account, "us-east-1", "lab-tg", "lambda", "HTTP", 80)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateELBv2TargetGroup(account, "us-east-1", "bad", "instance", "HTTP", 80); err == nil {
		t.Fatal("expected instance target type rejected")
	}
	listener, err := st.CreateELBv2Listener(account, "us-east-1", lb.ARN, tg.ARN, "HTTP", 80)
	if err != nil || listener.ListenerARN == "" {
		t.Fatalf("listener=%+v err=%v", listener, err)
	}
	missing := "arn:aws:lambda:us-east-1:" + account + ":function:missing"
	if err := st.RegisterELBv2Targets(account, tg.ARN, []store.ELBv2Target{{ID: missing}}); err == nil {
		t.Fatal("expected unresolved lambda target rejected")
	}
	zip := testZip(t, map[string]string{"app.py": "def handler(e,c): return e"})
	fn, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID: account, Region: "us-east-1", FunctionName: "lab",
		RoleARN: "arn:aws:iam::" + account + ":role/r", Runtime: store.LambdaRuntimePython312,
		Handler: "app.handler", Timeout: 3, Memory: 128, Zip: zip,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.RegisterELBv2Targets(account, tg.ARN, []store.ELBv2Target{{ID: fn.FunctionARN}}); err == nil {
		t.Fatal("expected RegisterTargets without ELB permission rejected")
	}
	if _, err := st.AddFunctionPermission(account, "lab", "elb-allow", "lambda:InvokeFunction",
		"elasticloadbalancing.amazonaws.com", "", tg.ARN); err != nil {
		t.Fatal(err)
	}
	if err := st.RegisterELBv2Targets(account, tg.ARN, []store.ELBv2Target{{ID: fn.FunctionARN}}); err != nil {
		t.Fatal(err)
	}
	if err := st.RegisterELBv2Targets(account, tg.ARN, []store.ELBv2Target{{ID: "i-ec2"}}); err == nil {
		t.Fatal("expected non-lambda id rejected on lambda TG")
	}
	targets, err := st.ListELBv2Targets(account, tg.ARN)
	if err != nil || len(targets) != 1 {
		t.Fatalf("targets=%v err=%v", targets, err)
	}
	health, err := st.DescribeELBv2TargetHealth(account, tg.ARN)
	if err != nil || len(health) != 1 || health[0].State != "healthy" {
		t.Fatalf("health=%v err=%v want healthy with listener", health, err)
	}
	tg2, err := st.CreateELBv2TargetGroup(account, "us-east-1", "no-listener", "lambda", "HTTP", 80)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddFunctionPermission(account, "lab", "elb-allow-2", "lambda:InvokeFunction",
		"elasticloadbalancing.amazonaws.com", "", tg2.ARN); err != nil {
		t.Fatal(err)
	}
	if err := st.RegisterELBv2Targets(account, tg2.ARN, []store.ELBv2Target{{ID: fn.FunctionARN}}); err != nil {
		t.Fatal(err)
	}
	unused, err := st.DescribeELBv2TargetHealth(account, tg2.ARN)
	if err != nil || len(unused) != 1 || unused[0].State != "unused" {
		t.Fatalf("health=%v err=%v want unused without listener", unused, err)
	}
}

func TestS3VectorsPutQuery(t *testing.T) {
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

	if _, err := st.CreateS3VectorBucket("000000000001", "vec-lab"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateS3VectorIndex("000000000001", "vec-lab", "idx", "cosine", 3); err != nil {
		t.Fatal(err)
	}
	err = st.PutS3Vectors("000000000001", "vec-lab", "idx", []store.S3Vector{
		{Key: "a", Data: []float64{1, 0, 0}},
		{Key: "b", Data: []float64{0, 1, 0}},
		{Key: "c", Data: []float64{0.9, 0.1, 0}},
	})
	if err != nil {
		t.Fatal(err)
	}
	matches, err := st.QueryS3Vectors("000000000001", "vec-lab", "idx", []float64{1, 0, 0}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 2 || matches[0].Key != "a" {
		t.Fatalf("matches=%+v", matches)
	}
}
