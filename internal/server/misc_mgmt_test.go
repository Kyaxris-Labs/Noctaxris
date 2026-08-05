package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestCodeDeployListDeploymentsAndUnknown(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	app := mustJSONTarget(t, handler, "CodeDeploy_20141006.CreateApplication", "codedeploy", map[string]any{
		"applicationName": "cov-app",
	}, now)
	if app.Code != http.StatusOK {
		t.Fatalf("CreateApplication status=%d body=%q", app.Code, app.Body.String())
	}
	dg := mustJSONTarget(t, handler, "CodeDeploy_20141006.CreateDeploymentGroup", "codedeploy", map[string]any{
		"applicationName":     "cov-app",
		"deploymentGroupName": "cov-dg",
		"serviceRoleArn":      "arn:aws:iam::" + testAccountID + ":role/cd",
	}, now)
	if dg.Code != http.StatusOK {
		// role may need to exist; create role then retry
		mustCreateIAMRole(t, handler, "cd", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"codedeploy.amazonaws.com"},"Action":"sts:AssumeRole"}]}`, now)
		dg = mustJSONTarget(t, handler, "CodeDeploy_20141006.CreateDeploymentGroup", "codedeploy", map[string]any{
			"applicationName":     "cov-app",
			"deploymentGroupName": "cov-dg",
			"serviceRoleArn":      "arn:aws:iam::" + testAccountID + ":role/cd",
		}, now)
	}
	if dg.Code != http.StatusOK {
		t.Fatalf("CreateDeploymentGroup status=%d body=%q", dg.Code, dg.Body.String())
	}
	dep := mustJSONTarget(t, handler, "CodeDeploy_20141006.CreateDeployment", "codedeploy", map[string]any{
		"applicationName":     "cov-app",
		"deploymentGroupName": "cov-dg",
	}, now)
	if dep.Code != http.StatusOK {
		t.Fatalf("CreateDeployment status=%d body=%q", dep.Code, dep.Body.String())
	}

	list := mustJSONTarget(t, handler, "CodeDeploy_20141006.ListDeployments", "codedeploy", map[string]any{
		"applicationName":     "cov-app",
		"deploymentGroupName": "cov-dg",
	}, now)
	if list.Code != http.StatusOK {
		t.Fatalf("ListDeployments status=%d body=%q", list.Code, list.Body.String())
	}
	unknown := mustJSONTarget(t, handler, "CodeDeploy_20141006.DeleteApplication", "codedeploy", map[string]any{}, now)
	if unknown.Code != http.StatusNotImplemented || !strings.Contains(unknown.Body.String(), "InvalidAction") {
		t.Fatalf("unknown CodeDeploy action want InvalidAction status=%d body=%q", unknown.Code, unknown.Body.String())
	}
}

func TestConfigDescribeComplianceAndUnknown(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "config-role", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"config.amazonaws.com"},"Action":"sts:AssumeRole"}]}`, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/config-role"

	form := url.Values{
		"Action":                       {"PutConfigurationRecorder"},
		"Version":                      {"2014-11-12"},
		"ConfigurationRecorder.Name":   {"default"},
		"ConfigurationRecorder.roleARN": {roleARN},
	}
	body := []byte(form.Encode())
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", body)
	signHeader(t, req, body, testAccessKey, testSecret, testRegion, "config", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PutConfigurationRecorder status=%d body=%q", rec.Code, rec.Body.String())
	}

	comp := url.Values{
		"Action":  {"DescribeComplianceByConfigRule"},
		"Version": {"2014-11-12"},
	}
	compBody := []byte(comp.Encode())
	compReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", compBody)
	signHeader(t, compReq, compBody, testAccessKey, testSecret, testRegion, "config", now)
	compRec := httptest.NewRecorder()
	handler.ServeHTTP(compRec, compReq)
	if compRec.Code != http.StatusOK {
		t.Fatalf("DescribeComplianceByConfigRule status=%d body=%q", compRec.Code, compRec.Body.String())
	}

	bad := url.Values{"Action": {"DeleteConfigRule"}, "Version": {"2014-11-12"}}
	badBody := []byte(bad.Encode())
	badReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", badBody)
	signHeader(t, badReq, badBody, testAccessKey, testSecret, testRegion, "config", now)
	badRec := httptest.NewRecorder()
	handler.ServeHTTP(badRec, badReq)
	if badRec.Code == http.StatusOK {
		t.Fatalf("unknown Config action should fail: %q", badRec.Body.String())
	}
}

func TestCostExplorerForecastAndPricingAttributes(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	forecast := mustJSONTarget(t, handler, "AWSInsightsIndexService.GetCostForecast", "ce", map[string]any{
		"TimePeriod": map[string]any{"Start": "2026-01-01", "End": "2026-02-01"},
		"Metric":     "UNBLENDED_COST",
		"Granularity": "MONTHLY",
	}, now)
	if forecast.Code != http.StatusOK {
		t.Fatalf("GetCostForecast status=%d body=%q", forecast.Code, forecast.Body.String())
	}
	badCE := mustJSONTarget(t, handler, "AWSInsightsIndexService.GetAnomalies", "ce", map[string]any{}, now)
	if badCE.Code == http.StatusOK {
		t.Fatalf("unknown CE action should fail: %q", badCE.Body.String())
	}

	attrs := mustJSONTarget(t, handler, "AWSPriceListService.GetAttributeValues", "pricing", map[string]any{
		"ServiceCode":   "AmazonEC2",
		"AttributeName": "location",
	}, now)
	if attrs.Code != http.StatusOK {
		t.Fatalf("GetAttributeValues status=%d body=%q", attrs.Code, attrs.Body.String())
	}
	badPricing := mustJSONTarget(t, handler, "AWSPriceListService.DescribeServices", "pricing", map[string]any{}, now)
	// DescribeServices may be implemented; unknown action should fail
	_ = badPricing
	unknown := mustJSONTarget(t, handler, "AWSPriceListService.NotReal", "pricing", map[string]any{}, now)
	if unknown.Code == http.StatusOK {
		t.Fatalf("unknown Pricing action should fail: %q", unknown.Body.String())
	}
}

func TestCloudFrontGetDistributionCoverage(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)
	if _, err := st.CreateBucket(testAccountID, "cf-get-bucket"); err != nil {
		t.Fatal(err)
	}

	create := mustJSONTarget(t, handler, "CloudFront_2016_01_28.CreateDistribution", "cloudfront", map[string]any{
		"DistributionConfig": map[string]any{
			"CallerReference": "cov-cf-1",
			"Comment":         "cov",
			"Enabled":         true,
			"Origins": map[string]any{
				"Items": []map[string]any{
					{"Id": "o1", "DomainName": "cf-get-bucket", "OriginType": "s3", "S3OriginConfig": map[string]any{}},
				},
			},
			"DefaultCacheBehavior": map[string]any{
				"TargetOriginId":       "o1",
				"ViewerProtocolPolicy": "allow-all",
			},
		},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateDistribution status=%d body=%q", create.Code, create.Body.String())
	}
	var out map[string]any
	_ = json.Unmarshal(create.Body.Bytes(), &out)
	dist, _ := out["Distribution"].(map[string]any)
	id, _ := dist["Id"].(string)
	if id == "" {
		id, _ = out["Id"].(string)
	}
	if id == "" {
		t.Fatalf("missing distribution id: %s", create.Body.String())
	}
	get := mustJSONTarget(t, handler, "CloudFront_2016_01_28.GetDistribution", "cloudfront", map[string]any{
		"Id": id,
	}, now)
	if get.Code != http.StatusOK {
		t.Fatalf("GetDistribution status=%d body=%q", get.Code, get.Body.String())
	}
	missing := mustJSONTarget(t, handler, "CloudFront_2016_01_28.GetDistribution", "cloudfront", map[string]any{
		"Id": "EDOESNOTEXIST",
	}, now)
	if missing.Code == http.StatusOK {
		t.Fatalf("GetDistribution missing should fail: %q", missing.Body.String())
	}
}

func TestCodeBuildBatchGetBuildsCoverage(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "cb-cov-role", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"codebuild.amazonaws.com"},"Action":"sts:AssumeRole"}]}`, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/cb-cov-role"
	create := mustCodeBuildJSON(t, handler, "CreateProject", map[string]any{
		"name": "cb-cov", "serviceRole": roleARN,
		"source": map[string]any{
			"type": "NO_SOURCE", "buildspec": "version: 0.2\nphases:\n  build:\n    commands:\n      - echo ok\n",
		},
		"environment": map[string]any{
			"type": "LINUX_CONTAINER", "image": "alpine:3.20", "computeType": "BUILD_GENERAL1_SMALL",
		},
		"artifacts": map[string]any{"type": "NO_ARTIFACTS"},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateProject status=%d body=%q", create.Code, create.Body.String())
	}
	// StartBuild without DinD should record a failed/unavailable build or error; still exercise BatchGetBuilds
	start := mustCodeBuildJSON(t, handler, "StartBuild", map[string]any{"projectName": "cb-cov"}, now)
	ids := []string{}
	if start.Code == http.StatusOK {
		var startOut map[string]any
		_ = json.Unmarshal(start.Body.Bytes(), &startOut)
		if build, ok := startOut["build"].(map[string]any); ok {
			if id, _ := build["id"].(string); id != "" {
				ids = append(ids, id)
			}
		}
	}
	list := mustCodeBuildJSON(t, handler, "ListBuilds", map[string]any{"projectName": "cb-cov"}, now)
	if list.Code == http.StatusOK {
		var listOut map[string]any
		_ = json.Unmarshal(list.Body.Bytes(), &listOut)
		if raw, ok := listOut["ids"].([]any); ok {
			for _, v := range raw {
				if s, ok := v.(string); ok {
					ids = append(ids, s)
				}
			}
		}
	}
	batch := mustCodeBuildJSON(t, handler, "BatchGetBuilds", map[string]any{"ids": ids}, now)
	if batch.Code != http.StatusOK {
		t.Fatalf("BatchGetBuilds status=%d body=%q", batch.Code, batch.Body.String())
	}
	empty := mustCodeBuildJSON(t, handler, "BatchGetBuilds", map[string]any{"ids": []string{"missing"}}, now)
	if empty.Code != http.StatusOK && empty.Code != http.StatusBadRequest {
		t.Fatalf("BatchGetBuilds missing unexpected status=%d body=%q", empty.Code, empty.Body.String())
	}
}

func TestSecretsUpdateSecretVersionStageCoverage(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustJSONTarget(t, handler, "secretsmanager.CreateSecret", "secretsmanager", map[string]any{
		"Name":         "cov-secret",
		"SecretString": "v1",
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateSecret status=%d body=%q", create.Code, create.Body.String())
	}
	put := mustJSONTarget(t, handler, "secretsmanager.PutSecretValue", "secretsmanager", map[string]any{
		"SecretId":     "cov-secret",
		"SecretString": "v2",
	}, now)
	if put.Code != http.StatusOK {
		t.Fatalf("PutSecretValue status=%d body=%q", put.Code, put.Body.String())
	}
	var putOut map[string]any
	_ = json.Unmarshal(put.Body.Bytes(), &putOut)
	ver, _ := putOut["VersionId"].(string)

	upd := mustJSONTarget(t, handler, "secretsmanager.UpdateSecretVersionStage", "secretsmanager", map[string]any{
		"SecretId":     "cov-secret",
		"VersionStage": "AWSCURRENT",
		"MoveToVersionId": ver,
	}, now)
	if upd.Code != http.StatusOK && upd.Code != http.StatusBadRequest {
		t.Fatalf("UpdateSecretVersionStage status=%d body=%q", upd.Code, upd.Body.String())
	}
	missing := mustJSONTarget(t, handler, "secretsmanager.UpdateSecretVersionStage", "secretsmanager", map[string]any{
		"SecretId":        "missing",
		"VersionStage":    "AWSCURRENT",
		"MoveToVersionId": "nope",
	}, now)
	if missing.Code == http.StatusOK {
		t.Fatalf("UpdateSecretVersionStage missing should fail: %q", missing.Body.String())
	}
}

func TestServiceDiscoveryDeregisterAndError(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	ns := mustJSONTarget(t, handler, "Route53AutoNaming_v20170314.CreatePrivateDnsNamespace", "servicediscovery", map[string]any{
		"Name": "cov.local", "Vpc": "vpc-1",
	}, now)
	if ns.Code != http.StatusOK {
		t.Fatalf("CreatePrivateDnsNamespace status=%d body=%q", ns.Code, ns.Body.String())
	}
	var nsOut map[string]any
	_ = json.Unmarshal(ns.Body.Bytes(), &nsOut)
	nsID, _ := nsOut["Namespace"].(map[string]any)["Id"].(string)
	if nsID == "" {
		nsID, _ = nsOut["Id"].(string)
	}
	svc := mustJSONTarget(t, handler, "Route53AutoNaming_v20170314.CreateService", "servicediscovery", map[string]any{
		"Name": "api", "NamespaceId": nsID,
	}, now)
	if svc.Code != http.StatusOK {
		t.Fatalf("CreateService status=%d body=%q", svc.Code, svc.Body.String())
	}
	var svcOut map[string]any
	_ = json.Unmarshal(svc.Body.Bytes(), &svcOut)
	svcID, _ := svcOut["Service"].(map[string]any)["Id"].(string)
	if svcID == "" {
		svcID, _ = svcOut["Id"].(string)
	}
	reg := mustJSONTarget(t, handler, "Route53AutoNaming_v20170314.RegisterInstance", "servicediscovery", map[string]any{
		"ServiceId":  svcID,
		"InstanceId": "i-1",
		"Attributes": map[string]string{"AWS_INSTANCE_IPV4": "10.0.0.1"},
	}, now)
	if reg.Code != http.StatusOK {
		t.Fatalf("RegisterInstance status=%d body=%q", reg.Code, reg.Body.String())
	}
	dereg := mustJSONTarget(t, handler, "Route53AutoNaming_v20170314.DeregisterInstance", "servicediscovery", map[string]any{
		"ServiceId": svcID, "InstanceId": "i-1",
	}, now)
	if dereg.Code != http.StatusOK {
		t.Fatalf("DeregisterInstance status=%d body=%q", dereg.Code, dereg.Body.String())
	}
	deregGone := mustJSONTarget(t, handler, "Route53AutoNaming_v20170314.DeregisterInstance", "servicediscovery", map[string]any{
		"ServiceId": svcID, "InstanceId": "i-1",
	}, now)
	if deregGone.Code == http.StatusOK {
		t.Fatalf("DeregisterInstance missing should fail: %q", deregGone.Body.String())
	}
	unknown := mustJSONTarget(t, handler, "Route53AutoNaming_v20170314.NotReal", "servicediscovery", map[string]any{}, now)
	if unknown.Code == http.StatusOK {
		t.Fatalf("unknown SD action should fail: %q", unknown.Body.String())
	}
}
