package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestCloudControlCreateListDelete(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustJSONTarget(t, handler, "CloudControlApi.CreateResource", "cloudcontrol", map[string]any{
		"TypeName":     "AWS::S3::Bucket",
		"DesiredState": `{"BucketName":"srv-cc-bucket"}`,
	}, now)
	if create.Code != http.StatusOK || !strings.Contains(create.Body.String(), "SUCCESS") {
		t.Fatalf("CreateResource status=%d body=%q", create.Code, create.Body.String())
	}

	unknown := mustJSONTarget(t, handler, "CloudControlApi.CreateResource", "cloudcontrol", map[string]any{
		"TypeName":     "AWS::EC2::Instance",
		"DesiredState": `{"InstanceType":"t3.micro"}`,
	}, now)
	if unknown.Code == http.StatusOK {
		t.Fatalf("expected unknown type fail closed, got %s", unknown.Body.String())
	}

	list := mustJSONTarget(t, handler, "CloudControlApi.ListResources", "cloudcontrol", map[string]any{
		"TypeName": "AWS::S3::Bucket",
	}, now)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "srv-cc-bucket") {
		t.Fatalf("ListResources status=%d body=%q", list.Code, list.Body.String())
	}

	del := mustJSONTarget(t, handler, "CloudControlApi.DeleteResource", "cloudcontrol", map[string]any{
		"TypeName":   "AWS::S3::Bucket",
		"Identifier": "srv-cc-bucket",
	}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteResource status=%d body=%q", del.Code, del.Body.String())
	}
}

func TestBCMExportCreateList(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustJSONTarget(t, handler, "AWSBillingAndCostManagementDataExports.CreateExport", "bcm-data-exports", map[string]any{
		"Export": map[string]any{
			"Name":        "srv-cur",
			"Description": "lab",
			"DestinationConfigurations": map[string]any{
				"S3Destination": map[string]any{
					"S3OutputConfigurations": map[string]any{"Format": "CSV"},
				},
			},
		},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateExport status=%d body=%q", create.Code, create.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(create.Body.Bytes(), &resp)
	arn, _ := resp["ExportArn"].(string)
	if arn == "" {
		t.Fatalf("missing arn: %s", create.Body.String())
	}

	list := mustJSONTarget(t, handler, "AWSBillingAndCostManagementDataExports.ListExports", "bcm-data-exports", map[string]any{}, now)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), arn) {
		t.Fatalf("ListExports status=%d body=%q", list.Code, list.Body.String())
	}
}

func TestCostExplorerGetCostAndUsage(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	got := mustJSONTarget(t, handler, "AWSInsightsIndexService.GetCostAndUsage", "ce", map[string]any{
		"TimePeriod":  map[string]string{"Start": "2026-07-01", "End": "2026-08-01"},
		"Granularity": "MONTHLY",
		"Metrics":     []string{"UnblendedCost"},
	}, now)
	if got.Code != http.StatusOK || !strings.Contains(got.Body.String(), "12.34") {
		t.Fatalf("GetCostAndUsage status=%d body=%q", got.Code, got.Body.String())
	}
}

func TestBudgetsCreateDescribeDelete(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustJSONTarget(t, handler, "AWSBudgetServiceGateway.CreateBudget", "budgets", map[string]any{
		"AccountId": "000000000001",
		"Budget": map[string]any{
			"BudgetName": "srv-budget",
			"BudgetType": "COST",
			"TimeUnit":   "MONTHLY",
			"BudgetLimit": map[string]string{
				"Amount": "50.0",
				"Unit":   "USD",
			},
		},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateBudget status=%d body=%q", create.Code, create.Body.String())
	}

	desc := mustJSONTarget(t, handler, "AWSBudgetServiceGateway.DescribeBudget", "budgets", map[string]any{
		"AccountId":  "000000000001",
		"BudgetName": "srv-budget",
	}, now)
	if desc.Code != http.StatusOK || !strings.Contains(desc.Body.String(), "50.0") {
		t.Fatalf("DescribeBudget status=%d body=%q", desc.Code, desc.Body.String())
	}

	del := mustJSONTarget(t, handler, "AWSBudgetServiceGateway.DeleteBudget", "budgets", map[string]any{
		"AccountId":  "000000000001",
		"BudgetName": "srv-budget",
	}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteBudget status=%d body=%q", del.Code, del.Body.String())
	}
}

func TestCodeDeployCreateDeploymentSucceeded(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	app := mustJSONTarget(t, handler, "CodeDeploy_20141006.CreateApplication", "codedeploy", map[string]any{
		"applicationName": "SrvApp",
		"computePlatform": "ECS",
	}, now)
	if app.Code != http.StatusOK {
		t.Fatalf("CreateApplication status=%d body=%q", app.Code, app.Body.String())
	}

	dg := mustJSONTarget(t, handler, "CodeDeploy_20141006.CreateDeploymentGroup", "codedeploy", map[string]any{
		"applicationName":     "SrvApp",
		"deploymentGroupName": "SrvDG",
		"ecsServices": []map[string]any{{
			"serviceName": "missing-ok",
			"clusterName": "default",
		}},
	}, now)
	if dg.Code != http.StatusOK {
		t.Fatalf("CreateDeploymentGroup status=%d body=%q", dg.Code, dg.Body.String())
	}

	dep := mustJSONTarget(t, handler, "CodeDeploy_20141006.CreateDeployment", "codedeploy", map[string]any{
		"applicationName":     "SrvApp",
		"deploymentGroupName": "SrvDG",
		"description":         "lab",
	}, now)
	if dep.Code != http.StatusOK {
		t.Fatalf("CreateDeployment status=%d body=%q", dep.Code, dep.Body.String())
	}
	var depResp map[string]any
	_ = json.Unmarshal(dep.Body.Bytes(), &depResp)
	id, _ := depResp["deploymentId"].(string)
	if id == "" {
		t.Fatalf("missing deploymentId: %s", dep.Body.String())
	}

	get := mustJSONTarget(t, handler, "CodeDeploy_20141006.GetDeployment", "codedeploy", map[string]any{
		"deploymentId": id,
	}, now)
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), "Succeeded") {
		t.Fatalf("GetDeployment status=%d body=%q", get.Code, get.Body.String())
	}
}

func TestCodeDeployLambdaPublishVersionHook(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "cd-lambda-role", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/cd-lambda-role"
	createFn := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "cd-hook-fn",
		"Runtime":      "python3.12",
		"Role":         roleARN,
		"Handler":      "app.handler",
		"Code":         map[string]any{"ZipFile": testLambdaZipB64(t)},
	}, now)
	if createFn.Code != http.StatusOK {
		t.Fatalf("CreateFunction status=%d body=%q", createFn.Code, createFn.Body.String())
	}

	app := mustJSONTarget(t, handler, "CodeDeploy_20141006.CreateApplication", "codedeploy", map[string]any{
		"applicationName": "LambdaApp",
		"computePlatform": "Lambda",
	}, now)
	if app.Code != http.StatusOK {
		t.Fatalf("CreateApplication status=%d", app.Code)
	}
	dg := mustJSONTarget(t, handler, "CodeDeploy_20141006.CreateDeploymentGroup", "codedeploy", map[string]any{
		"applicationName":     "LambdaApp",
		"deploymentGroupName": "LambdaDG",
		"lambdaFunctionName":  "cd-hook-fn",
	}, now)
	if dg.Code != http.StatusOK {
		t.Fatalf("CreateDeploymentGroup status=%d body=%q", dg.Code, dg.Body.String())
	}
	dep := mustJSONTarget(t, handler, "CodeDeploy_20141006.CreateDeployment", "codedeploy", map[string]any{
		"applicationName":     "LambdaApp",
		"deploymentGroupName": "LambdaDG",
	}, now)
	if dep.Code != http.StatusOK {
		t.Fatalf("CreateDeployment status=%d body=%q", dep.Code, dep.Body.String())
	}
	versions, err := st.ListVersionsByFunction(testAccountID, "cd-hook-fn")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, v := range versions {
		if v.Version >= 1 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected PublishVersion from CodeDeploy hook, versions=%+v", versions)
	}
}
