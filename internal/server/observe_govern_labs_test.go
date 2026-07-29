package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
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

func TestLightsailDiskStaticIPKeyPairPortHandlers(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustOK := func(target string, body map[string]any) *httptest.ResponseRecorder {
		t.Helper()
		rec := mustJSONTarget(t, handler, target, "lightsail", body, now)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%q", target, rec.Code, rec.Body.String())
		}
		return rec
	}

	mustOK("Lightsail_20161128.CreateInstances", map[string]any{
		"instanceNames":    []string{"web-a"},
		"availabilityZone": "us-east-1a",
		"blueprintId":      "ubuntu_22_04",
		"bundleId":         "nano_3_0",
	})
	mustOK("Lightsail_20161128.CreateDisk", map[string]any{
		"diskName": "data-1", "availabilityZone": "us-east-1a", "sizeInGb": 32,
	})
	getDisk := mustOK("Lightsail_20161128.GetDisk", map[string]any{"diskName": "data-1"})
	if !strings.Contains(getDisk.Body.String(), `"available"`) {
		t.Fatalf("GetDisk: %q", getDisk.Body.String())
	}
	mustOK("Lightsail_20161128.AttachDisk", map[string]any{
		"diskName": "data-1", "instanceName": "web-a", "diskPath": "/dev/xvdf",
	})
	getDisk = mustOK("Lightsail_20161128.GetDisk", map[string]any{"diskName": "data-1"})
	if !strings.Contains(getDisk.Body.String(), `"in-use"`) || !strings.Contains(getDisk.Body.String(), `"web-a"`) {
		t.Fatalf("attached GetDisk: %q", getDisk.Body.String())
	}
	mustOK("Lightsail_20161128.DetachDisk", map[string]any{"diskName": "data-1"})
	mustOK("Lightsail_20161128.DeleteDisk", map[string]any{"diskName": "data-1"})

	badDisk := mustJSONTarget(t, handler, "Lightsail_20161128.CreateDisk", "lightsail", map[string]any{
		"diskName": "tiny", "availabilityZone": "us-east-1a", "sizeInGb": 4,
	}, now)
	if badDisk.Code != http.StatusBadRequest {
		t.Fatalf("small disk status=%d", badDisk.Code)
	}

	mustOK("Lightsail_20161128.AllocateStaticIp", map[string]any{"staticIpName": "web-ip"})
	mustOK("Lightsail_20161128.AttachStaticIp", map[string]any{
		"staticIpName": "web-ip", "instanceName": "web-a",
	})
	getInst := mustOK("Lightsail_20161128.GetInstance", map[string]any{"instanceName": "web-a"})
	var instBody map[string]any
	if err := json.Unmarshal(getInst.Body.Bytes(), &instBody); err != nil {
		t.Fatal(err)
	}
	inst, _ := instBody["instance"].(map[string]any)
	if inst["isStaticIp"] != true {
		t.Fatalf("expected isStaticIp true: %#v", inst)
	}
	mustOK("Lightsail_20161128.DetachStaticIp", map[string]any{"staticIpName": "web-ip"})
	mustOK("Lightsail_20161128.ReleaseStaticIp", map[string]any{"staticIpName": "web-ip"})

	createKP := mustOK("Lightsail_20161128.CreateKeyPair", map[string]any{"keyPairName": "lab-key"})
	if !strings.Contains(createKP.Body.String(), "privateKeyBase64") {
		t.Fatalf("CreateKeyPair missing private key: %q", createKP.Body.String())
	}
	mustOK("Lightsail_20161128.GetKeyPair", map[string]any{"keyPairName": "lab-key"})
	mustOK("Lightsail_20161128.DeleteKeyPair", map[string]any{"keyPairName": "lab-key"})

	mustOK("Lightsail_20161128.OpenInstancePublicPorts", map[string]any{
		"instanceName": "web-a",
		"portInfo":     map[string]any{"fromPort": 80, "toPort": 80, "protocol": "tcp"},
	})
	ports := mustOK("Lightsail_20161128.GetInstancePortStates", map[string]any{"instanceName": "web-a"})
	if !strings.Contains(ports.Body.String(), `"fromPort":80`) && !strings.Contains(ports.Body.String(), `"fromPort": 80`) {
		t.Fatalf("port states: %q", ports.Body.String())
	}
	mustOK("Lightsail_20161128.CloseInstancePublicPorts", map[string]any{
		"instanceName": "web-a",
		"portInfo":     map[string]any{"fromPort": 80, "toPort": 80, "protocol": "tcp"},
	})
	ports = mustOK("Lightsail_20161128.GetInstancePortStates", map[string]any{"instanceName": "web-a"})
	if strings.Contains(ports.Body.String(), `"fromPort":80`) || strings.Contains(ports.Body.String(), `"fromPort": 80`) {
		t.Fatalf("expected empty ports: %q", ports.Body.String())
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

func TestAutoScalingPolicyHookTGAndAttachHandlers(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)
	account := "000000000001"

	createLC := mustASGQuery(t, handler, strings.Join([]string{
		"Action=CreateLaunchConfiguration",
		"Version=2011-01-01",
		"LaunchConfigurationName=lc-ext",
		"ImageId=ami-123",
		"InstanceType=t3.micro",
	}, "&"), now)
	if createLC.Code != http.StatusOK {
		t.Fatalf("CreateLaunchConfiguration status=%d body=%q", createLC.Code, createLC.Body.String())
	}
	createASG := mustASGQuery(t, handler, strings.Join([]string{
		"Action=CreateAutoScalingGroup",
		"Version=2011-01-01",
		"AutoScalingGroupName=asg-ext",
		"LaunchConfigurationName=lc-ext",
		"MinSize=0",
		"MaxSize=4",
		"DesiredCapacity=0",
		"AvailabilityZones.member.1=us-east-1a",
	}, "&"), now)
	if createASG.Code != http.StatusOK {
		t.Fatalf("CreateAutoScalingGroup status=%d body=%q", createASG.Code, createASG.Body.String())
	}

	putPol := mustASGQuery(t, handler, strings.Join([]string{
		"Action=PutScalingPolicy",
		"Version=2011-01-01",
		"AutoScalingGroupName=asg-ext",
		"PolicyName=scale-out",
		"PolicyType=SimpleScaling",
		"AdjustmentType=ChangeInCapacity",
		"ScalingAdjustment=1",
		"Cooldown=60",
	}, "&"), now)
	if putPol.Code != http.StatusOK || !strings.Contains(putPol.Body.String(), "<PolicyARN>") {
		t.Fatalf("PutScalingPolicy status=%d body=%q", putPol.Code, putPol.Body.String())
	}
	descPol := mustASGQuery(t, handler,
		"Action=DescribePolicies&Version=2011-01-01&AutoScalingGroupName=asg-ext", now)
	if descPol.Code != http.StatusOK || !strings.Contains(descPol.Body.String(), "scale-out") {
		t.Fatalf("DescribePolicies status=%d body=%q", descPol.Code, descPol.Body.String())
	}

	putHook := mustASGQuery(t, handler, strings.Join([]string{
		"Action=PutLifecycleHook",
		"Version=2011-01-01",
		"AutoScalingGroupName=asg-ext",
		"LifecycleHookName=launch-hook",
		"LifecycleTransition=autoscaling:EC2_INSTANCE_LAUNCHING",
		"HeartbeatTimeout=300",
		"DefaultResult=CONTINUE",
	}, "&"), now)
	if putHook.Code != http.StatusOK {
		t.Fatalf("PutLifecycleHook status=%d body=%q", putHook.Code, putHook.Body.String())
	}
	descHook := mustASGQuery(t, handler,
		"Action=DescribeLifecycleHooks&Version=2011-01-01&AutoScalingGroupName=asg-ext", now)
	if descHook.Code != http.StatusOK || !strings.Contains(descHook.Body.String(), "launch-hook") {
		t.Fatalf("DescribeLifecycleHooks status=%d body=%q", descHook.Code, descHook.Body.String())
	}

	tg, err := st.CreateELBv2TargetGroup(account, "us-east-1", "asg-ext-tg", "instance", "HTTP", 80)
	if err != nil {
		t.Fatal(err)
	}
	attachTG := mustASGQuery(t, handler, strings.Join([]string{
		"Action=AttachLoadBalancerTargetGroups",
		"Version=2011-01-01",
		"AutoScalingGroupName=asg-ext",
		"TargetGroupARNs.member.1=" + url.QueryEscape(tg.ARN),
	}, "&"), now)
	if attachTG.Code != http.StatusOK {
		t.Fatalf("AttachLoadBalancerTargetGroups status=%d body=%q", attachTG.Code, attachTG.Body.String())
	}
	descTG := mustASGQuery(t, handler,
		"Action=DescribeLoadBalancerTargetGroups&Version=2011-01-01&AutoScalingGroupName=asg-ext", now)
	if descTG.Code != http.StatusOK || !strings.Contains(descTG.Body.String(), tg.ARN) {
		t.Fatalf("DescribeLoadBalancerTargetGroups status=%d body=%q", descTG.Code, descTG.Body.String())
	}
	badTG := mustASGQuery(t, handler, strings.Join([]string{
		"Action=AttachLoadBalancerTargetGroups",
		"Version=2011-01-01",
		"AutoScalingGroupName=asg-ext",
		"TargetGroupARNs.member.1=" + url.QueryEscape("arn:aws:elasticloadbalancing:us-east-1:"+account+":targetgroup/nope/0123456789abcdef"),
	}, "&"), now)
	if badTG.Code != http.StatusBadRequest {
		t.Fatalf("missing TG want 400 got %d body=%q", badTG.Code, badTG.Body.String())
	}

	launched, err := st.RunInstances(account, "us-east-1", store.RunInstancesInput{
		ImageID: "ami-123", InstanceType: "t3.micro", MinCount: 1, MaxCount: 1, AvailabilityZone: "us-east-1a",
	})
	if err != nil || len(launched) != 1 {
		t.Fatalf("RunInstances: %+v err=%v", launched, err)
	}
	attachInst := mustASGQuery(t, handler, strings.Join([]string{
		"Action=AttachInstances",
		"Version=2011-01-01",
		"AutoScalingGroupName=asg-ext",
		"InstanceIds.member.1=" + launched[0].InstanceID,
	}, "&"), now)
	if attachInst.Code != http.StatusOK {
		t.Fatalf("AttachInstances status=%d body=%q", attachInst.Code, attachInst.Body.String())
	}
	descInst := mustASGQuery(t, handler,
		"Action=DescribeAutoScalingInstances&Version=2011-01-01", now)
	if descInst.Code != http.StatusOK || !strings.Contains(descInst.Body.String(), launched[0].InstanceID) {
		t.Fatalf("DescribeAutoScalingInstances status=%d body=%q", descInst.Code, descInst.Body.String())
	}
	detachInst := mustASGQuery(t, handler, strings.Join([]string{
		"Action=DetachInstances",
		"Version=2011-01-01",
		"AutoScalingGroupName=asg-ext",
		"InstanceIds.member.1=" + launched[0].InstanceID,
		"ShouldDecrementDesiredCapacity=true",
	}, "&"), now)
	if detachInst.Code != http.StatusOK || !strings.Contains(detachInst.Body.String(), "DetachInstancesResult") {
		t.Fatalf("DetachInstances status=%d body=%q", detachInst.Code, detachInst.Body.String())
	}

	delPol := mustASGQuery(t, handler, strings.Join([]string{
		"Action=DeletePolicy",
		"Version=2011-01-01",
		"AutoScalingGroupName=asg-ext",
		"PolicyName=scale-out",
	}, "&"), now)
	if delPol.Code != http.StatusOK {
		t.Fatalf("DeletePolicy status=%d body=%q", delPol.Code, delPol.Body.String())
	}
	delHook := mustASGQuery(t, handler, strings.Join([]string{
		"Action=DeleteLifecycleHook",
		"Version=2011-01-01",
		"AutoScalingGroupName=asg-ext",
		"LifecycleHookName=launch-hook",
	}, "&"), now)
	if delHook.Code != http.StatusOK {
		t.Fatalf("DeleteLifecycleHook status=%d body=%q", delHook.Code, delHook.Body.String())
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
	var planOut map[string]any
	if err := json.Unmarshal(createPlan.Body.Bytes(), &planOut); err != nil {
		t.Fatal(err)
	}
	planID, _ := planOut["BackupPlanId"].(string)
	if planID == "" {
		t.Fatal("missing BackupPlanId")
	}

	createSel := mustBackupREST(t, handler, http.MethodPut, "/backup/plans/"+planID+"/selections/", map[string]any{
		"BackupSelection": map[string]any{
			"SelectionName": "lab-sel",
			"IamRoleArn":    "arn:aws:iam::000000000001:role/Backup",
			"Resources":     []string{"arn:aws:dynamodb:us-east-1:000000000001:table/lab"},
		},
	}, now)
	if createSel.Code != http.StatusOK || !strings.Contains(createSel.Body.String(), "SelectionId") {
		t.Fatalf("CreateBackupSelection status=%d body=%q", createSel.Code, createSel.Body.String())
	}
	var selOut map[string]any
	if err := json.Unmarshal(createSel.Body.Bytes(), &selOut); err != nil {
		t.Fatal(err)
	}
	selectionID, _ := selOut["SelectionId"].(string)

	listSel := mustBackupREST(t, handler, http.MethodGet, "/backup/plans/"+planID+"/selections/", nil, now)
	if listSel.Code != http.StatusOK || !strings.Contains(listSel.Body.String(), "lab-sel") {
		t.Fatalf("ListBackupSelections status=%d body=%q", listSel.Code, listSel.Body.String())
	}
	getSel := mustBackupREST(t, handler, http.MethodGet, "/backup/plans/"+planID+"/selections/"+selectionID, nil, now)
	if getSel.Code != http.StatusOK || !strings.Contains(getSel.Body.String(), "IamRoleArn") {
		t.Fatalf("GetBackupSelection status=%d body=%q", getSel.Code, getSel.Body.String())
	}

	start := mustBackupREST(t, handler, http.MethodPut, "/backup-jobs", map[string]any{
		"BackupVaultName": "lab-vault",
		"ResourceArn":     "arn:aws:dynamodb:us-east-1:000000000001:table/lab",
		"IamRoleArn":      "arn:aws:iam::000000000001:role/Backup",
	}, now)
	if start.Code != http.StatusOK || !strings.Contains(start.Body.String(), "RecoveryPointArn") {
		t.Fatalf("StartBackupJob status=%d body=%q", start.Code, start.Body.String())
	}
	var startOut map[string]any
	if err := json.Unmarshal(start.Body.Bytes(), &startOut); err != nil {
		t.Fatal(err)
	}
	jobID, _ := startOut["BackupJobId"].(string)
	rpARN, _ := startOut["RecoveryPointArn"].(string)

	listJobs := mustBackupREST(t, handler, http.MethodGet, "/backup-jobs/", nil, now)
	if listJobs.Code != http.StatusOK || !strings.Contains(listJobs.Body.String(), jobID) {
		t.Fatalf("ListBackupJobs status=%d body=%q", listJobs.Code, listJobs.Body.String())
	}
	stop := mustBackupREST(t, handler, http.MethodPost, "/backup-jobs/"+jobID, nil, now)
	if stop.Code != http.StatusOK {
		t.Fatalf("StopBackupJob status=%d body=%q", stop.Code, stop.Body.String())
	}

	listRP := mustBackupREST(t, handler, http.MethodGet, "/backup-vaults/lab-vault/recovery-points/", nil, now)
	if listRP.Code != http.StatusOK || !strings.Contains(listRP.Body.String(), "DynamoDB") {
		t.Fatalf("ListRecoveryPoints status=%d body=%q", listRP.Code, listRP.Body.String())
	}
	delRP := mustBackupREST(t, handler, http.MethodDelete, "/backup-vaults/lab-vault/recovery-points/"+url.PathEscape(rpARN), nil, now)
	if delRP.Code != http.StatusOK {
		t.Fatalf("DeleteRecoveryPoint status=%d body=%q", delRP.Code, delRP.Body.String())
	}
	listRP2 := mustBackupREST(t, handler, http.MethodGet, "/backup-vaults/lab-vault/recovery-points/", nil, now)
	if listRP2.Code != http.StatusOK || strings.Contains(listRP2.Body.String(), rpARN) {
		t.Fatalf("ListRecoveryPoints after delete status=%d body=%q", listRP2.Code, listRP2.Body.String())
	}

	delSel := mustBackupREST(t, handler, http.MethodDelete, "/backup/plans/"+planID+"/selections/"+selectionID, nil, now)
	if delSel.Code != http.StatusOK {
		t.Fatalf("DeleteBackupSelection status=%d body=%q", delSel.Code, delSel.Body.String())
	}
}
