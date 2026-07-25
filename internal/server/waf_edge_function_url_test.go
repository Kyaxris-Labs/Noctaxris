package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestWAFAssociateHTTPAPIAndRejectUnknown(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustJSONTarget(t, handler, "AWSWAF_20190729.CreateWebACL", "wafv2", map[string]any{
		"Name": "edge-acl", "Scope": "REGIONAL",
		"DefaultAction": map[string]any{"Allow": map[string]any{}},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateWebACL status=%d body=%q", create.Code, create.Body.String())
	}
	var out map[string]any
	_ = json.Unmarshal(create.Body.Bytes(), &out)
	sum, _ := out["Summary"].(map[string]any)
	arn, _ := sum["ARN"].(string)

	ok := mustJSONTarget(t, handler, "AWSWAF_20190729.AssociateWebACL", "wafv2", map[string]any{
		"WebACLArn":   arn,
		"ResourceArn": "arn:aws:apigateway:us-east-1::/apis/http1/stages/$default",
	}, now)
	if ok.Code != http.StatusOK {
		t.Fatalf("AssociateWebACL http api status=%d body=%q", ok.Code, ok.Body.String())
	}

	bad := mustJSONTarget(t, handler, "AWSWAF_20190729.AssociateWebACL", "wafv2", map[string]any{
		"WebACLArn":   arn,
		"ResourceArn": "arn:aws:s3:::not-supported",
	}, now)
	if bad.Code == http.StatusOK {
		t.Fatalf("expected fail closed, got %s", bad.Body.String())
	}
}

func TestWAFAssociateRejectsPhantomWebACL(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	phantom := "arn:aws:wafv2:us-east-1:" + testAccountID + ":regional/webacl/missing/00000000-0000-0000-0000-000000000000"
	bad := mustJSONTarget(t, handler, "AWSWAF_20190729.AssociateWebACL", "wafv2", map[string]any{
		"WebACLArn":   phantom,
		"ResourceArn": "arn:aws:apigateway:us-east-1::/apis/http1/stages/$default",
	}, now)
	if bad.Code == http.StatusOK {
		t.Fatalf("phantom AssociateWebACL should fail, got %s", bad.Body.String())
	}
	if !strings.Contains(bad.Body.String(), "WAFNonexistentItemException") &&
		!strings.Contains(bad.Body.String(), "not found") {
		t.Fatalf("expected nonexistent ACL error, body=%q", bad.Body.String())
	}
}

func TestWAFAssociateBlocksHTTPAPIInvoke(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "waf-block-role", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/waf-block-role"
	fnRec := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "waf-block-fn",
		"Runtime":      "python3.12",
		"Role":         roleARN,
		"Handler":      "app.handler",
		"Code":         map[string]any{"ZipFile": testLambdaZipB64(t)},
	}, now)
	if fnRec.Code != http.StatusOK {
		t.Fatalf("CreateFunction status=%d", fnRec.Code)
	}
	lambdaARN := "arn:aws:lambda:us-east-1:" + testAccountID + ":function:waf-block-fn"
	apiID, integrationID := mustCreateHTTPAPIWithIntegration(t, handler, "waf-http", lambdaARN, now)
	mustJSONTarget(t, handler, "ApiGatewayV2.CreateRoute", "apigateway", map[string]any{
		"ApiId": apiID, "RouteKey": "GET /blocked", "Target": "integrations/" + integrationID,
		"AuthorizationType": "NONE",
	}, now)
	mustJSONTarget(t, handler, "ApiGatewayV2.CreateStage", "apigateway", map[string]any{
		"ApiId": apiID, "StageName": "$default",
	}, now)

	create := mustJSONTarget(t, handler, "AWSWAF_20190729.CreateWebACL", "wafv2", map[string]any{
		"Name": "block-acl", "Scope": "REGIONAL",
		"DefaultAction": map[string]any{"Block": map[string]any{}},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateWebACL status=%d body=%q", create.Code, create.Body.String())
	}
	var out map[string]any
	_ = json.Unmarshal(create.Body.Bytes(), &out)
	sum, _ := out["Summary"].(map[string]any)
	aclARN, _ := sum["ARN"].(string)

	assoc := mustJSONTarget(t, handler, "AWSWAF_20190729.AssociateWebACL", "wafv2", map[string]any{
		"WebACLArn":   aclARN,
		"ResourceArn": "arn:aws:apigateway:us-east-1::/apis/" + apiID + "/stages/$default",
	}, now)
	if assoc.Code != http.StatusOK {
		t.Fatalf("AssociateWebACL status=%d body=%q", assoc.Code, assoc.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4566/http-api/"+apiID+"/$default/blocked", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("WAF block invoke status=%d want 403 body=%q", rec.Code, rec.Body.String())
	}
}

func TestWAFByteMatchBlocksHTTPAPIInvoke(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "waf-bm-role", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/waf-bm-role"
	fnRec := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "waf-bm-fn",
		"Runtime":      "python3.12",
		"Role":         roleARN,
		"Handler":      "app.handler",
		"Code":         map[string]any{"ZipFile": testLambdaZipB64(t)},
	}, now)
	if fnRec.Code != http.StatusOK {
		t.Fatalf("CreateFunction status=%d", fnRec.Code)
	}
	lambdaARN := "arn:aws:lambda:us-east-1:" + testAccountID + ":function:waf-bm-fn"
	apiID, integrationID := mustCreateHTTPAPIWithIntegration(t, handler, "waf-bm-http", lambdaARN, now)
	mustAddLambdaServicePermission(t, handler, "waf-bm-fn", "apigateway.amazonaws.com", "waf-bm-apigw", now)
	mustJSONTarget(t, handler, "ApiGatewayV2.CreateRoute", "apigateway", map[string]any{
		"ApiId": apiID, "RouteKey": "GET /admin", "Target": "integrations/" + integrationID,
		"AuthorizationType": "NONE",
	}, now)
	mustJSONTarget(t, handler, "ApiGatewayV2.CreateRoute", "apigateway", map[string]any{
		"ApiId": apiID, "RouteKey": "GET /public", "Target": "integrations/" + integrationID,
		"AuthorizationType": "NONE",
	}, now)
	mustJSONTarget(t, handler, "ApiGatewayV2.CreateStage", "apigateway", map[string]any{
		"ApiId": apiID, "StageName": "$default",
	}, now)

	create := mustJSONTarget(t, handler, "AWSWAF_20190729.CreateWebACL", "wafv2", map[string]any{
		"Name": "bytematch-acl", "Scope": "REGIONAL",
		"DefaultAction": map[string]any{"Allow": map[string]any{}},
		"Rules": []map[string]any{{
			"Name": "block-admin", "Priority": 1,
			"Action": map[string]any{"Block": map[string]any{}},
			"Statement": map[string]any{
				"ByteMatchStatement": map[string]any{
					"SearchString":         "/admin",
					"PositionalConstraint": "CONTAINS",
					"FieldToMatch":         map[string]any{"UriPath": map[string]any{}},
				},
			},
		}},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateWebACL status=%d body=%q", create.Code, create.Body.String())
	}
	var out map[string]any
	_ = json.Unmarshal(create.Body.Bytes(), &out)
	sum, _ := out["Summary"].(map[string]any)
	aclARN, _ := sum["ARN"].(string)

	assoc := mustJSONTarget(t, handler, "AWSWAF_20190729.AssociateWebACL", "wafv2", map[string]any{
		"WebACLArn":   aclARN,
		"ResourceArn": "arn:aws:apigateway:us-east-1::/apis/" + apiID + "/stages/$default",
	}, now)
	if assoc.Code != http.StatusOK {
		t.Fatalf("AssociateWebACL status=%d body=%q", assoc.Code, assoc.Body.String())
	}

	blockReq := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4566/http-api/"+apiID+"/$default/admin", nil)
	blockRec := httptest.NewRecorder()
	handler.ServeHTTP(blockRec, blockReq)
	if blockRec.Code != http.StatusForbidden {
		t.Fatalf("admin invoke status=%d want 403 body=%q", blockRec.Code, blockRec.Body.String())
	}

	okReq := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4566/http-api/"+apiID+"/$default/public", nil)
	okRec := httptest.NewRecorder()
	handler.ServeHTTP(okRec, okReq)
	if okRec.Code == http.StatusForbidden {
		t.Fatalf("public invoke blocked body=%q", okRec.Body.String())
	}
}

func TestCloudFrontCreateListDelete(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	missing := mustJSONTarget(t, handler, "CloudFront_2016_01_28.CreateDistribution", "cloudfront", map[string]any{
		"DistributionConfig": map[string]any{
			"CallerReference": "cf-missing",
			"Comment":         "lab",
			"Enabled":         true,
			"Origins": map[string]any{
				"Items": []map[string]any{
					{"Id": "o1", "DomainName": "no-such-bucket", "OriginType": "s3"},
				},
			},
		},
	}, now)
	if missing.Code == http.StatusOK {
		t.Fatalf("missing S3 origin must fail closed: %q", missing.Body.String())
	}

	if _, err := st.CreateBucket(testAccountID, "lab-bucket"); err != nil {
		t.Fatal(err)
	}
	api, err := st.CreateAPIGatewayAPI(testAccountID, "us-east-1", "cf-origin", "HTTP")
	if err != nil {
		t.Fatal(err)
	}

	create := mustJSONTarget(t, handler, "CloudFront_2016_01_28.CreateDistribution", "cloudfront", map[string]any{
		"DistributionConfig": map[string]any{
			"CallerReference": "cf-ref-1",
			"Comment":         "lab",
			"Enabled":         true,
			"Origins": map[string]any{
				"Items": []map[string]any{
					{"Id": "o1", "DomainName": "lab-bucket", "OriginType": "s3"},
					{"Id": "o2", "DomainName": api.APIID, "OriginType": "apigateway"},
				},
			},
		},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateDistribution status=%d body=%q", create.Code, create.Body.String())
	}
	var created map[string]any
	_ = json.Unmarshal(create.Body.Bytes(), &created)
	dist, _ := created["Distribution"].(map[string]any)
	id, _ := dist["Id"].(string)
	if id == "" {
		t.Fatalf("missing id: %s", create.Body.String())
	}
	status, _ := dist["Status"].(string)
	if status != "Deployed" {
		t.Fatalf("Status=%q want Deployed body=%q", status, create.Body.String())
	}
	domain, _ := dist["DomainName"].(string)
	if domain == "" || !strings.Contains(domain, "cloudfront.noctaxris.local") {
		t.Fatalf("DomainName=%q want lab fake-edge host body=%q", domain, create.Body.String())
	}

	list := mustJSONTarget(t, handler, "CloudFront_2016_01_28.ListDistributions", "cloudfront", map[string]any{}, now)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), id) {
		t.Fatalf("ListDistributions status=%d body=%q", list.Code, list.Body.String())
	}

	del := mustJSONTarget(t, handler, "CloudFront_2016_01_28.DeleteDistribution", "cloudfront", map[string]any{
		"Id": id,
	}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteDistribution status=%d body=%q", del.Code, del.Body.String())
	}
}

func TestELBv2CreateListenerAndRegisterLambda(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	lb := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.CreateLoadBalancer", "elasticloadbalancing", map[string]any{
		"Name": "lab-alb",
	}, now)
	if lb.Code != http.StatusOK {
		t.Fatalf("CreateLoadBalancer status=%d body=%q", lb.Code, lb.Body.String())
	}
	var lbOut map[string]any
	_ = json.Unmarshal(lb.Body.Bytes(), &lbOut)
	lbs, _ := lbOut["LoadBalancers"].([]any)
	lbMap, _ := lbs[0].(map[string]any)
	lbARN, _ := lbMap["LoadBalancerArn"].(string)

	tg := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.CreateTargetGroup", "elasticloadbalancing", map[string]any{
		"Name": "lab-tg", "TargetType": "lambda",
	}, now)
	if tg.Code != http.StatusOK {
		t.Fatalf("CreateTargetGroup status=%d body=%q", tg.Code, tg.Body.String())
	}
	var tgOut map[string]any
	_ = json.Unmarshal(tg.Body.Bytes(), &tgOut)
	tgs, _ := tgOut["TargetGroups"].([]any)
	tgMap, _ := tgs[0].(map[string]any)
	tgARN, _ := tgMap["TargetGroupArn"].(string)

	listener := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.CreateListener", "elasticloadbalancing", map[string]any{
		"LoadBalancerArn": lbARN,
		"Protocol":        "HTTP",
		"Port":            80,
		"DefaultActions": []map[string]any{{
			"Type": "forward", "TargetGroupArn": tgARN,
		}},
	}, now)
	if listener.Code != http.StatusOK {
		t.Fatalf("CreateListener status=%d body=%q", listener.Code, listener.Body.String())
	}

	missing := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.RegisterTargets", "elasticloadbalancing", map[string]any{
		"TargetGroupArn": tgARN,
		"Targets": []map[string]any{{
			"Id": "arn:aws:lambda:us-east-1:" + testAccountID + ":function:missing",
		}},
	}, now)
	if missing.Code == http.StatusOK {
		t.Fatalf("unresolved lambda target must be rejected: %q", missing.Body.String())
	}

	mustCreateIAMRole(t, handler, "elb-lambda-exec", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/elb-lambda-exec"
	createFn := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "lab",
		"Runtime":      "python3.12",
		"Role":         roleARN,
		"Handler":      "app.handler",
		"Code":         map[string]any{"ZipFile": testLambdaZipB64(t)},
	}, now)
	if createFn.Code != http.StatusOK {
		t.Fatalf("CreateFunction status=%d body=%q", createFn.Code, createFn.Body.String())
	}
	mustAddLambdaServicePermission(t, handler, "lab", "elasticloadbalancing.amazonaws.com", "elb-allow", now)

	fnARN := "arn:aws:lambda:us-east-1:" + testAccountID + ":function:lab"
	reg := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.RegisterTargets", "elasticloadbalancing", map[string]any{
		"TargetGroupArn": tgARN,
		"Targets":        []map[string]any{{"Id": fnARN}},
	}, now)
	if reg.Code != http.StatusOK {
		t.Fatalf("RegisterTargets status=%d body=%q", reg.Code, reg.Body.String())
	}

	health := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.DescribeTargetHealth", "elasticloadbalancing", map[string]any{
		"TargetGroupArn": tgARN,
	}, now)
	if health.Code != http.StatusOK || !strings.Contains(health.Body.String(), `"healthy"`) {
		t.Fatalf("DescribeTargetHealth want healthy with listener, status=%d body=%q", health.Code, health.Body.String())
	}

	reject := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.CreateTargetGroup", "elasticloadbalancing", map[string]any{
		"Name": "ec2-nope", "TargetType": "instance",
	}, now)
	if reject.Code == http.StatusOK {
		t.Fatalf("expected instance target type rejected")
	}

	nlb := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.CreateLoadBalancer", "elasticloadbalancing", map[string]any{
		"Name": "lab-nlb", "Type": "network",
	}, now)
	if nlb.Code == http.StatusOK {
		t.Fatalf("Type=network must be rejected: %q", nlb.Body.String())
	}
}

func TestELBv2LabListenerInvokesLambda(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	var gotEvent string
	srv.SetLambdaInvokeHookForTest(func(_ context.Context, _, name string, _ store.LambdaFunction, _, eventJSON string) ([]byte, error) {
		if name != "lab" {
			t.Fatalf("function=%q", name)
		}
		gotEvent = eventJSON
		return []byte(`{"statusCode":200,"headers":{"content-type":"text/plain"},"body":"alb-ok"}`), nil
	})

	lb := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.CreateLoadBalancer", "elasticloadbalancing", map[string]any{
		"Name": "invoke-alb",
	}, now)
	var lbOut map[string]any
	_ = json.Unmarshal(lb.Body.Bytes(), &lbOut)
	lbs, _ := lbOut["LoadBalancers"].([]any)
	lbMap, _ := lbs[0].(map[string]any)
	lbARN, _ := lbMap["LoadBalancerArn"].(string)

	tg := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.CreateTargetGroup", "elasticloadbalancing", map[string]any{
		"Name": "invoke-tg", "TargetType": "lambda",
	}, now)
	var tgOut map[string]any
	_ = json.Unmarshal(tg.Body.Bytes(), &tgOut)
	tgs, _ := tgOut["TargetGroups"].([]any)
	tgMap, _ := tgs[0].(map[string]any)
	tgARN, _ := tgMap["TargetGroupArn"].(string)

	mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.CreateListener", "elasticloadbalancing", map[string]any{
		"LoadBalancerArn": lbARN,
		"Protocol":        "HTTP",
		"Port":            80,
		"DefaultActions":  []map[string]any{{"Type": "forward", "TargetGroupArn": tgARN}},
	}, now)

	mustCreateIAMRole(t, handler, "elb-invoke-exec", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/elb-invoke-exec"
	createFn := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "lab",
		"Runtime":      "python3.12",
		"Role":         roleARN,
		"Handler":      "app.handler",
		"Code":         map[string]any{"ZipFile": testLambdaZipB64(t)},
	}, now)
	if createFn.Code != http.StatusOK {
		t.Fatalf("CreateFunction status=%d body=%q", createFn.Code, createFn.Body.String())
	}
	mustAddLambdaServicePermission(t, handler, "lab", "elasticloadbalancing.amazonaws.com", "elb-invoke", now)
	fnARN := "arn:aws:lambda:us-east-1:" + testAccountID + ":function:lab"
	reg := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.RegisterTargets", "elasticloadbalancing", map[string]any{
		"TargetGroupArn": tgARN,
		"Targets":        []map[string]any{{"Id": fnARN}},
	}, now)
	if reg.Code != http.StatusOK {
		t.Fatalf("RegisterTargets status=%d body=%q", reg.Code, reg.Body.String())
	}

	path := "/alb/" + testAccountID + "/invoke-alb/80/hello"
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566"+path, []byte(`{"ping":1}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "alb-ok" {
		t.Fatalf("lab listener status=%d body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(gotEvent, `"path":"/hello"`) || !strings.Contains(gotEvent, tgARN) {
		t.Fatalf("ALB event missing path/tg: %s", gotEvent)
	}
}

func TestWAFAssociateAcceptsELBWithLabListener(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustJSONTarget(t, handler, "AWSWAF_20190729.CreateWebACL", "wafv2", map[string]any{
		"Name": "elb-acl", "Scope": "REGIONAL",
		"DefaultAction": map[string]any{"Allow": map[string]any{}},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateWebACL status=%d body=%q", create.Code, create.Body.String())
	}
	var out map[string]any
	_ = json.Unmarshal(create.Body.Bytes(), &out)
	sum, _ := out["Summary"].(map[string]any)
	aclARN, _ := sum["ARN"].(string)

	alb := mustJSONTarget(t, handler, "AWSWAF_20190729.AssociateWebACL", "wafv2", map[string]any{
		"WebACLArn":   aclARN,
		"ResourceArn": "arn:aws:elasticloadbalancing:us-east-1:" + testAccountID + ":loadbalancer/app/lab/abc",
	}, now)
	if alb.Code != http.StatusOK {
		t.Fatalf("ALB AssociateWebACL should succeed with lab listener enforce path: %q", alb.Body.String())
	}

	cognito := mustJSONTarget(t, handler, "AWSWAF_20190729.AssociateWebACL", "wafv2", map[string]any{
		"WebACLArn":   aclARN,
		"ResourceArn": "arn:aws:cognito-idp:us-east-1:" + testAccountID + ":userpool/us-east-1_abc",
	}, now)
	if cognito.Code == http.StatusOK {
		t.Fatalf("Cognito AssociateWebACL must still fail closed: %q", cognito.Body.String())
	}
}

func TestS3VectorsPutAndQuery(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	bucket := mustJSONTarget(t, handler, "AmazonS3Vectors.CreateVectorBucket", "s3vectors", map[string]any{
		"vectorBucketName": "vec-lab",
	}, now)
	if bucket.Code != http.StatusOK {
		t.Fatalf("CreateVectorBucket status=%d body=%q", bucket.Code, bucket.Body.String())
	}

	idx := mustJSONTarget(t, handler, "AmazonS3Vectors.CreateIndex", "s3vectors", map[string]any{
		"vectorBucketName": "vec-lab",
		"indexName":        "idx",
		"dimension":        2,
		"distanceMetric":   "cosine",
	}, now)
	if idx.Code != http.StatusOK {
		t.Fatalf("CreateIndex status=%d body=%q", idx.Code, idx.Body.String())
	}

	put := mustJSONTarget(t, handler, "AmazonS3Vectors.PutVectors", "s3vectors", map[string]any{
		"vectorBucketName": "vec-lab",
		"indexName":        "idx",
		"vectors": []map[string]any{
			{"key": "a", "data": map[string]any{"float32": []float64{1, 0}}},
			{"key": "b", "data": map[string]any{"float32": []float64{0, 1}}},
		},
	}, now)
	if put.Code != http.StatusOK {
		t.Fatalf("PutVectors status=%d body=%q", put.Code, put.Body.String())
	}

	query := mustJSONTarget(t, handler, "AmazonS3Vectors.QueryVectors", "s3vectors", map[string]any{
		"vectorBucketName": "vec-lab",
		"indexName":        "idx",
		"topK":             1,
		"queryVector":      map[string]any{"float32": []float64{1, 0}},
	}, now)
	if query.Code != http.StatusOK || !strings.Contains(query.Body.String(), `"key":"a"`) {
		t.Fatalf("QueryVectors status=%d body=%q", query.Code, query.Body.String())
	}
}

func TestEdgePassRolePrincipals(t *testing.T) {
	if authz.ServicePrincipalAPIGateway != "apigateway.amazonaws.com" {
		t.Fatalf("gateway principal=%q", authz.ServicePrincipalAPIGateway)
	}
	if authz.ServicePrincipalCognitoIDP != "cognito-idp.amazonaws.com" {
		t.Fatalf("cognito principal=%q", authz.ServicePrincipalCognitoIDP)
	}
	trust := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"apigateway.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	if !authz.TrustAllowsService(trust, authz.ServicePrincipalAPIGateway) {
		t.Fatal("expected API Gateway trust allow")
	}
	if authz.TrustAllowsService(trust, authz.ServicePrincipalCognitoIDP) {
		t.Fatal("expected Cognito trust deny on gateway-only trust")
	}
}

func TestFunctionURLCORSNone(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "url-cors-role", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/url-cors-role"
	createFn := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "cors-fn",
		"Runtime":      "python3.12",
		"Role":         roleARN,
		"Handler":      "app.handler",
		"Code":         map[string]any{"ZipFile": testLambdaZipB64(t)},
	}, now)
	if createFn.Code != http.StatusOK {
		t.Fatalf("CreateFunction status=%d body=%q", createFn.Code, createFn.Body.String())
	}

	urlCfg := mustLambdaJSON(t, handler, "CreateFunctionUrlConfig", map[string]any{
		"FunctionName": "cors-fn",
		"AuthType":     "NONE",
	}, now)
	if urlCfg.Code != http.StatusOK {
		t.Fatalf("CreateFunctionUrlConfig status=%d body=%q", urlCfg.Code, urlCfg.Body.String())
	}

	req := httptest.NewRequest(http.MethodOptions, "/lambda-url/"+testAccountID+"/cors-fn", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("OPTIONS status=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("missing CORS ACAO: %v", rec.Header())
	}
}
