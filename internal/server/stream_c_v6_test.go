package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
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

func TestCloudFrontCreateListDelete(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustJSONTarget(t, handler, "CloudFront_2016_01_28.CreateDistribution", "cloudfront", map[string]any{
		"DistributionConfig": map[string]any{
			"CallerReference": "cf-ref-1",
			"Comment":         "lab",
			"Enabled":         true,
			"Origins": map[string]any{
				"Items": []map[string]any{
					{"Id": "o1", "DomainName": "lab-bucket", "OriginType": "s3"},
					{"Id": "o2", "DomainName": "api-123", "OriginType": "apigateway"},
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

	reg := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.RegisterTargets", "elasticloadbalancing", map[string]any{
		"TargetGroupArn": tgARN,
		"Targets": []map[string]any{{
			"Id": "arn:aws:lambda:us-east-1:" + testAccountID + ":function:lab",
		}},
	}, now)
	if reg.Code != http.StatusOK {
		t.Fatalf("RegisterTargets status=%d body=%q", reg.Code, reg.Body.String())
	}

	reject := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.CreateTargetGroup", "elasticloadbalancing", map[string]any{
		"Name": "ec2-nope", "TargetType": "instance",
	}, now)
	if reject.Code == http.StatusOK {
		t.Fatalf("expected instance target type rejected")
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
