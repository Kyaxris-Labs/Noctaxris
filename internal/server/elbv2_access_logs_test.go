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

func TestELBv2LabListenerWritesAccessLogToS3(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	if _, err := st.CreateBucket(testAccountID, "alb-access-logs"); err != nil {
		t.Fatal(err)
	}

	srv.SetLambdaInvokeHookForTest(func(_ context.Context, _, name string, _ store.LambdaFunction, _, _ string) ([]byte, error) {
		if name != "lab" {
			t.Fatalf("function=%q", name)
		}
		return []byte(`{"statusCode":200,"headers":{"content-type":"text/plain"},"body":"alb-ok"}`), nil
	})

	lb := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.CreateLoadBalancer", "elasticloadbalancing", map[string]any{
		"Name": "accesslog-alb",
	}, now)
	var lbOut map[string]any
	_ = json.Unmarshal(lb.Body.Bytes(), &lbOut)
	lbs, _ := lbOut["LoadBalancers"].([]any)
	lbMap, _ := lbs[0].(map[string]any)
	lbARN, _ := lbMap["LoadBalancerArn"].(string)

	mod := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.ModifyLoadBalancerAttributes", "elasticloadbalancing", map[string]any{
		"LoadBalancerArn": lbARN,
		"Attributes": []map[string]any{
			{"Key": store.ELBv2AttrAccessLogsS3Enabled, "Value": "true"},
			{"Key": store.ELBv2AttrAccessLogsS3Bucket, "Value": "alb-access-logs"},
			{"Key": store.ELBv2AttrAccessLogsS3Prefix, "Value": "alb/"},
		},
	}, now)
	if mod.Code != http.StatusOK {
		t.Fatalf("ModifyLoadBalancerAttributes status=%d body=%q", mod.Code, mod.Body.String())
	}

	tg := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.CreateTargetGroup", "elasticloadbalancing", map[string]any{
		"Name": "accesslog-tg", "TargetType": "lambda",
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

	mustCreateIAMRole(t, handler, "elb-accesslog-exec", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/elb-accesslog-exec"
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
	mustAddLambdaServicePermission(t, handler, "lab", "elasticloadbalancing.amazonaws.com", "elb-accesslog", now)
	fnARN := "arn:aws:lambda:us-east-1:" + testAccountID + ":function:lab"
	reg := mustJSONTarget(t, handler, "ElasticLoadBalancing_v2.RegisterTargets", "elasticloadbalancing", map[string]any{
		"TargetGroupArn": tgARN,
		"Targets":        []map[string]any{{"Id": fnARN}},
	}, now)
	if reg.Code != http.StatusOK {
		t.Fatalf("RegisterTargets status=%d body=%q", reg.Code, reg.Body.String())
	}

	path := "/alb/" + testAccountID + "/accesslog-alb/80/hello"
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566"+path, []byte(`{"ping":1}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "alb-ok" {
		t.Fatalf("lab listener status=%d body=%q", rec.Code, rec.Body.String())
	}

	prefix := "alb/AWSLogs/" + testAccountID + "/elasticloadbalancing/us-east-1/"
	list, err := st.ListObjectsV2(testAccountID, "alb-access-logs", prefix, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Contents) == 0 {
		t.Fatalf("no access log objects under %q", prefix)
	}
	_, data, err := st.GetObject(testAccountID, "alb-access-logs", list.Contents[0].Key)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"POST /hello HTTP/1.1"`) {
		t.Fatalf("access log missing request line: %q", string(data))
	}
}

func TestCloudFrontEdgeWritesAccessLogToS3(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	now := time.Now().UTC().Truncate(time.Second)

	const logBucket = "cf-edge-logs"
	const originBucket = "cf-edge-origin"
	if _, err := st.CreateBucket(testAccountID, logBucket); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateBucket(testAccountID, originBucket); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutObject(testAccountID, originBucket, "docs/hi.txt", store.PutObjectMeta{
		Data: []byte("edge-object-bytes"), PlainSize: 17, ContentType: "text/plain",
	}); err != nil {
		t.Fatal(err)
	}

	handler := srv.Handler()
	create := mustJSONTarget(t, handler, "CloudFront_2020_05_31.CreateDistribution", "cloudfront", map[string]any{
		"DistributionConfig": map[string]any{
			"CallerReference": "cf-accesslog-ref",
			"Comment":         "lab",
			"Enabled":         true,
			"Origins": map[string]any{
				"Items": []map[string]any{{
					"Id": "o1", "DomainName": originBucket, "S3OriginConfig": map[string]any{},
				}},
			},
			"Logging": map[string]any{
				"Enabled": true,
				"Bucket":  logBucket + ".s3.amazonaws.com",
				"Prefix":  "edge/",
			},
		},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateDistribution status=%d body=%q", create.Code, create.Body.String())
	}
	var out map[string]any
	_ = json.Unmarshal(create.Body.Bytes(), &out)
	dist, _ := out["Distribution"].(map[string]any)
	distID, _ := dist["Id"].(string)
	if distID == "" {
		t.Fatalf("missing distribution id in %v", out)
	}

	edge := srv.CloudFrontEdgeHandler()
	url := "http://127.0.0.1:4566/cloudfront/" + distID + "/docs/hi.txt"
	req := mustNewRequest(t, http.MethodGet, url, nil)
	req.Header.Del("Content-Type")
	signS3Header(t, req, nil, testAccessKey, testSecret, testRegion, "cloudfront", now)

	rec := httptest.NewRecorder()
	edge.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("edge GET status=%d body=%q", rec.Code, rec.Body.String())
	}

	prefix := "edge/AWSLogs/" + testAccountID + "/CloudFront/"
	list, err := st.ListObjectsV2(testAccountID, logBucket, prefix, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Contents) == 0 {
		t.Fatalf("no cloudfront access log objects under %q", prefix)
	}
	_, data, err := st.GetObject(testAccountID, logBucket, list.Contents[0].Key)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "GET") || !strings.Contains(string(data), "/docs/hi.txt") {
		t.Fatalf("cf access log: %q", string(data))
	}
}
