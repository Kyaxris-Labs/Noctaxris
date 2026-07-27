package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLightsailHandlers(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	bp := mustJSONTarget(t, handler, "Lightsail_20161128.GetBlueprints", "lightsail", map[string]any{}, now)
	if bp.Code != http.StatusOK || !strings.Contains(bp.Body.String(), "ubuntu_22_04") {
		t.Fatalf("GetBlueprints status=%d body=%q", bp.Code, bp.Body.String())
	}

	create := mustJSONTarget(t, handler, "Lightsail_20161128.CreateInstances", "lightsail", map[string]any{
		"instanceNames":    []string{"web-a"},
		"availabilityZone": "us-east-1a",
		"blueprintId":      "ubuntu_22_04",
		"bundleId":         "nano_3_0",
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateInstances status=%d body=%q", create.Code, create.Body.String())
	}

	get := mustJSONTarget(t, handler, "Lightsail_20161128.GetInstance", "lightsail", map[string]any{
		"instanceName": "web-a",
	}, now)
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), `"running"`) {
		t.Fatalf("GetInstance status=%d body=%q", get.Code, get.Body.String())
	}

	stop := mustJSONTarget(t, handler, "Lightsail_20161128.StopInstance", "lightsail", map[string]any{
		"instanceName": "web-a",
	}, now)
	if stop.Code != http.StatusOK {
		t.Fatalf("StopInstance status=%d body=%q", stop.Code, stop.Body.String())
	}
	get = mustJSONTarget(t, handler, "Lightsail_20161128.GetInstance", "lightsail", map[string]any{
		"instanceName": "web-a",
	}, now)
	if !strings.Contains(get.Body.String(), `"stopped"`) {
		t.Fatalf("expected stopped: %q", get.Body.String())
	}

	del := mustJSONTarget(t, handler, "Lightsail_20161128.DeleteInstance", "lightsail", map[string]any{
		"instanceName": "web-a",
	}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteInstance status=%d body=%q", del.Code, del.Body.String())
	}
}

func mustASGQuery(t *testing.T, handler http.Handler, body string, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(body))
	signHeader(t, req, []byte(body), testAccessKey, testSecret, testRegion, "autoscaling", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestAutoScalingHandlers(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createLC := mustASGQuery(t, handler, strings.Join([]string{
		"Action=CreateLaunchConfiguration",
		"Version=2011-01-01",
		"LaunchConfigurationName=lc-lab",
		"ImageId=ami-123",
		"InstanceType=t3.micro",
	}, "&"), now)
	if createLC.Code != http.StatusOK {
		t.Fatalf("CreateLaunchConfiguration status=%d body=%q", createLC.Code, createLC.Body.String())
	}

	createASG := mustASGQuery(t, handler, strings.Join([]string{
		"Action=CreateAutoScalingGroup",
		"Version=2011-01-01",
		"AutoScalingGroupName=asg-lab",
		"LaunchConfigurationName=lc-lab",
		"MinSize=1",
		"MaxSize=3",
		"DesiredCapacity=2",
		"AvailabilityZones.member.1=us-east-1a",
	}, "&"), now)
	if createASG.Code != http.StatusOK {
		t.Fatalf("CreateAutoScalingGroup status=%d body=%q", createASG.Code, createASG.Body.String())
	}

	desc := mustASGQuery(t, handler,
		"Action=DescribeAutoScalingGroups&Version=2011-01-01&AutoScalingGroupNames.member.1=asg-lab", now)
	if desc.Code != http.StatusOK || !strings.Contains(desc.Body.String(), "<DesiredCapacity>2</DesiredCapacity>") {
		t.Fatalf("DescribeAutoScalingGroups status=%d body=%q", desc.Code, desc.Body.String())
	}
	if !strings.Contains(desc.Body.String(), "<InstanceId>") {
		t.Fatalf("expected reconciled InstanceId members: %q", desc.Body.String())
	}
	if strings.Count(desc.Body.String(), "<InstanceId>") != 2 {
		t.Fatalf("want 2 InstanceId members: %q", desc.Body.String())
	}

	set := mustASGQuery(t, handler, strings.Join([]string{
		"Action=SetDesiredCapacity",
		"Version=2011-01-01",
		"AutoScalingGroupName=asg-lab",
		"DesiredCapacity=3",
	}, "&"), now)
	if set.Code != http.StatusOK {
		t.Fatalf("SetDesiredCapacity status=%d body=%q", set.Code, set.Body.String())
	}

	desc3 := mustASGQuery(t, handler,
		"Action=DescribeAutoScalingGroups&Version=2011-01-01&AutoScalingGroupNames.member.1=asg-lab", now)
	if desc3.Code != http.StatusOK || strings.Count(desc3.Body.String(), "<InstanceId>") != 3 {
		t.Fatalf("after set desired=3: %q", desc3.Body.String())
	}

	del := mustASGQuery(t, handler, strings.Join([]string{
		"Action=DeleteAutoScalingGroup",
		"Version=2011-01-01",
		"AutoScalingGroupName=asg-lab",
		"ForceDelete=true",
	}, "&"), now)
	if del.Code != http.StatusOK {
		t.Fatalf("ForceDelete status=%d body=%q", del.Code, del.Body.String())
	}
}

func mustBeanstalkQuery(t *testing.T, handler http.Handler, body string, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(body))
	signHeader(t, req, []byte(body), testAccessKey, testSecret, testRegion, "elasticbeanstalk", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestBeanstalkHandlers(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	stacks := mustBeanstalkQuery(t, handler, "Action=ListAvailableSolutionStacks&Version=2010-12-01", now)
	if stacks.Code != http.StatusOK || !strings.Contains(stacks.Body.String(), "Docker") {
		t.Fatalf("ListAvailableSolutionStacks status=%d body=%q", stacks.Code, stacks.Body.String())
	}

	createApp := mustBeanstalkQuery(t, handler,
		"Action=CreateApplication&Version=2010-12-01&ApplicationName=sample-app", now)
	if createApp.Code != http.StatusOK {
		t.Fatalf("CreateApplication status=%d body=%q", createApp.Code, createApp.Body.String())
	}
	appBody := createApp.Body.String()
	// Must stay on elasticbeanstalk Query XML — not AppConfig JSON CreateApplication.
	if !strings.Contains(appBody, "CreateApplicationResponse") ||
		!strings.Contains(appBody, "<ApplicationName>sample-app</ApplicationName>") {
		t.Fatalf("want Beanstalk CreateApplication XML: %q", appBody)
	}
	if strings.Contains(appBody, `"ApplicationId"`) || strings.HasPrefix(strings.TrimSpace(appBody), "{") {
		t.Fatalf("CreateApplication stolen by AppConfig JSON: %q", appBody)
	}

	createEnv := mustBeanstalkQuery(t, handler, strings.Join([]string{
		"Action=CreateEnvironment",
		"Version=2010-12-01",
		"ApplicationName=sample-app",
		"EnvironmentName=sample-env",
	}, "&"), now)
	if createEnv.Code != http.StatusOK || !strings.Contains(createEnv.Body.String(), "<Status>Ready</Status>") {
		t.Fatalf("CreateEnvironment status=%d body=%q", createEnv.Code, createEnv.Body.String())
	}

	term := mustBeanstalkQuery(t, handler,
		"Action=TerminateEnvironment&Version=2010-12-01&EnvironmentName=sample-env", now)
	if term.Code != http.StatusOK || !strings.Contains(term.Body.String(), "Terminated") {
		t.Fatalf("TerminateEnvironment status=%d body=%q", term.Code, term.Body.String())
	}
}

func mustBackupREST(t *testing.T, handler http.Handler, method, path string, payload map[string]any, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	var body []byte
	if payload != nil {
		var err error
		body, err = json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
	}
	req := mustNewRequest(t, method, "http://127.0.0.1:4566"+path, body)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	signHeader(t, req, body, testAccessKey, testSecret, testRegion, "backup", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestBackupHandlers(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createVault := mustBackupREST(t, handler, http.MethodPut, "/backup-vaults/lab-vault", map[string]any{}, now)
	if createVault.Code != http.StatusOK || !strings.Contains(createVault.Body.String(), "lab-vault") {
		t.Fatalf("CreateBackupVault status=%d body=%q", createVault.Code, createVault.Body.String())
	}

	createPlan := mustBackupREST(t, handler, http.MethodPut, "/backup/plans/", map[string]any{
		"BackupPlan": map[string]any{
			"BackupPlanName": "daily",
			"Rules": []map[string]any{
				{"RuleName": "daily", "TargetBackupVaultName": "lab-vault", "ScheduleExpression": "cron(0 5 ? * * *)"},
			},
		},
	}, now)
	if createPlan.Code != http.StatusOK || !strings.Contains(createPlan.Body.String(), "BackupPlanId") {
		t.Fatalf("CreateBackupPlan status=%d body=%q", createPlan.Code, createPlan.Body.String())
	}

	start := mustBackupREST(t, handler, http.MethodPut, "/backup-jobs", map[string]any{
		"BackupVaultName": "lab-vault",
		"ResourceArn":     "arn:aws:dynamodb:us-east-1:000000000001:table/lab",
		"IamRoleArn":      "arn:aws:iam::000000000001:role/Backup",
	}, now)
	if start.Code != http.StatusOK || !strings.Contains(start.Body.String(), "RecoveryPointArn") {
		t.Fatalf("StartBackupJob status=%d body=%q", start.Code, start.Body.String())
	}

	listRP := mustBackupREST(t, handler, http.MethodGet, "/backup-vaults/lab-vault/recovery-points/", nil, now)
	if listRP.Code != http.StatusOK || !strings.Contains(listRP.Body.String(), "DynamoDB") {
		t.Fatalf("ListRecoveryPoints status=%d body=%q", listRP.Code, listRP.Body.String())
	}
}
