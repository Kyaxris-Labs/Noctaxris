package store_test

import (
	"path/filepath"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestIsWAFAssociableResourceARN(t *testing.T) {
	ok := []string{
		"arn:aws:apigateway:us-east-1::/apis/abc123",
		"arn:aws:apigateway:us-east-1::/apis/abc123/stages/$default",
		"arn:aws:apigateway:us-east-1::/restapis/api1/stages/prod",
		"arn:aws:execute-api:us-east-1:000000000001:abc123",
		"arn:aws:execute-api:us-east-1:000000000001:abc123/$default/GET/hello",
		"arn:aws:elasticloadbalancing:us-east-1:000000000001:loadbalancer/app/lab/abc",
		"arn:aws:appsync:us-east-1:000000000001:apis/gql1",
		"arn:aws:cognito-idp:us-east-1:000000000001:userpool/us-east-1_abc",
		"arn:aws:lambda:us-east-1:000000000001:function:fn",
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

	d, err := st.CreateCloudFrontDistribution("000000000001", "lab", "ref-1", true, []store.CloudFrontOrigin{
		{ID: "s3", DomainName: "lab-bucket", OriginType: "s3"},
		{ID: "gw", DomainName: "api-id-1", OriginType: "apigateway"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := st.GetCloudFrontDistribution("000000000001", d.ID)
	if err != nil || got.DomainName == "" {
		t.Fatalf("get: %+v err=%v", got, err)
	}
	list, err := st.ListCloudFrontDistributions("000000000001")
	if err != nil || len(list) != 1 {
		t.Fatalf("list=%v err=%v", list, err)
	}
	if err := st.DeleteCloudFrontDistribution("000000000001", d.ID); err != nil {
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

	lb, err := st.CreateELBv2LoadBalancer("000000000001", "us-east-1", "lab-alb", "internet-facing")
	if err != nil {
		t.Fatal(err)
	}
	tg, err := st.CreateELBv2TargetGroup("000000000001", "us-east-1", "lab-tg", "lambda", "HTTP", 80)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateELBv2TargetGroup("000000000001", "us-east-1", "bad", "instance", "HTTP", 80); err == nil {
		t.Fatal("expected instance target type rejected")
	}
	listener, err := st.CreateELBv2Listener("000000000001", "us-east-1", lb.ARN, tg.ARN, "HTTP", 80)
	if err != nil || listener.ListenerARN == "" {
		t.Fatalf("listener=%+v err=%v", listener, err)
	}
	fnARN := "arn:aws:lambda:us-east-1:000000000001:function:lab"
	if err := st.RegisterELBv2Targets("000000000001", tg.ARN, []store.ELBv2Target{{ID: fnARN}}); err != nil {
		t.Fatal(err)
	}
	if err := st.RegisterELBv2Targets("000000000001", tg.ARN, []store.ELBv2Target{{ID: "i-ec2"}}); err == nil {
		t.Fatal("expected non-lambda id rejected on lambda TG")
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
