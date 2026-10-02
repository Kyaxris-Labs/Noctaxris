package server_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

const passRoleTrustBad = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::000000000001:root"},"Action":"sts:AssumeRole"}]}`


func TestEC2RunInstancesIamInstanceProfilePassRoleDeny(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "ec2-profile-bad", passRoleTrustBad, now)
	run := mustEC2Query(t, handler, strings.Join([]string{
		"Action=RunInstances",
		"Version=2016-11-15",
		"ImageId=ami-alpine",
		"InstanceType=t3.micro",
		"MinCount=1",
		"MaxCount=1",
		"IamInstanceProfile.Name=ec2-profile-bad",
	}, "&"), now)
	if run.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%q", run.Code, run.Body.String())
	}
	if !strings.Contains(run.Body.String(), "not authorized to pass role to EC2") {
		t.Fatalf("body=%q", run.Body.String())
	}
}

func TestASGLifecycleRoleARNPassRoleDeny(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "asg-hook-bad", passRoleTrustBad, now)

	createLC := mustASGQuery(t, handler, strings.Join([]string{
		"Action=CreateLaunchConfiguration",
		"Version=2011-01-01",
		"LaunchConfigurationName=lc-passrole-deny",
		"ImageId=ami-alpine",
		"InstanceType=t3.micro",
	}, "&"), now)
	if createLC.Code != http.StatusOK {
		t.Fatalf("CreateLaunchConfiguration status=%d body=%q", createLC.Code, createLC.Body.String())
	}
	createASG := mustASGQuery(t, handler, strings.Join([]string{
		"Action=CreateAutoScalingGroup",
		"Version=2011-01-01",
		"AutoScalingGroupName=asg-passrole-deny",
		"LaunchConfigurationName=lc-passrole-deny",
		"MinSize=0",
		"MaxSize=1",
		"DesiredCapacity=0",
		"AvailabilityZones.member.1=us-east-1a",
	}, "&"), now)
	if createASG.Code != http.StatusOK {
		t.Fatalf("CreateAutoScalingGroup status=%d body=%q", createASG.Code, createASG.Body.String())
	}

	badARN := "arn:aws:iam::" + testAccountID + ":role/asg-hook-bad"
	putHook := mustASGQuery(t, handler, strings.Join([]string{
		"Action=PutLifecycleHook",
		"Version=2011-01-01",
		"AutoScalingGroupName=asg-passrole-deny",
		"LifecycleHookName=hook-deny",
		"LifecycleTransition=autoscaling:EC2_INSTANCE_LAUNCHING",
		"RoleARN=" + url.QueryEscape(badARN),
	}, "&"), now)
	if putHook.Code != http.StatusForbidden {
		t.Fatalf("PutLifecycleHook status=%d body=%q", putHook.Code, putHook.Body.String())
	}
	if !strings.Contains(putHook.Body.String(), "not authorized to pass role to Auto Scaling") {
		t.Fatalf("body=%q", putHook.Body.String())
	}

	// LC profile wrong trust also denies (same ASG cluster).
	createLCBad := mustASGQuery(t, handler, strings.Join([]string{
		"Action=CreateLaunchConfiguration",
		"Version=2011-01-01",
		"LaunchConfigurationName=lc-profile-deny",
		"ImageId=ami-alpine",
		"InstanceType=t3.micro",
		"IamInstanceProfile=asg-hook-bad",
	}, "&"), now)
	if createLCBad.Code != http.StatusForbidden {
		t.Fatalf("CreateLaunchConfiguration profile status=%d body=%q", createLCBad.Code, createLCBad.Body.String())
	}
}

func TestBackupStartBackupJobPassRoleDeny(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "backup-bad", passRoleTrustBad, now)
	createVault := mustBackupREST(t, handler, http.MethodPut, "/backup-vaults/passrole-vault", map[string]any{}, now)
	if createVault.Code != http.StatusOK {
		t.Fatalf("CreateBackupVault status=%d body=%q", createVault.Code, createVault.Body.String())
	}
	badARN := "arn:aws:iam::" + testAccountID + ":role/backup-bad"
	start := mustBackupREST(t, handler, http.MethodPut, "/backup-jobs", map[string]any{
		"BackupVaultName": "passrole-vault",
		"ResourceArn":     "arn:aws:dynamodb:us-east-1:" + testAccountID + ":table/passrole",
		"IamRoleArn":      badARN,
	}, now)
	if start.Code != http.StatusForbidden {
		t.Fatalf("StartBackupJob status=%d body=%q", start.Code, start.Body.String())
	}
	if !strings.Contains(start.Body.String(), "not authorized to pass role to Backup") {
		t.Fatalf("body=%q", start.Body.String())
	}
}

func TestEKSCreateClusterPassRoleDeny(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "eks-bad", passRoleTrustBad, now)
	rec := mustEKSREST(t, handler, http.MethodPost, "/clusters", map[string]any{
		"name":    "eks-passrole-deny",
		"roleArn": "arn:aws:iam::" + testAccountID + ":role/eks-bad",
		"resourcesVpcConfig": map[string]any{
			"subnetIds": []string{"subnet-1"},
		},
	}, now)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("CreateCluster status=%d body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "not authorized to pass role to EKS") {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestEMRAddJobFlowStepsPassRoleDeny(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "emr-bad", passRoleTrustBad, now)
	run := mustJSONTarget(t, handler, "ElasticMapReduce.RunJobFlow", "elasticmapreduce", map[string]any{
		"Name": "emr-passrole",
		"Instances": map[string]any{
			"InstanceGroups": []map[string]any{
				{"Name": "master", "InstanceRole": "MASTER", "InstanceType": "m5.xlarge", "InstanceCount": 1},
			},
		},
	}, now)
	if run.Code != http.StatusOK {
		t.Fatalf("RunJobFlow status=%d body=%q", run.Code, run.Body.String())
	}
	var out map[string]any
	_ = json.Unmarshal(run.Body.Bytes(), &out)
	jobFlowID, _ := out["JobFlowId"].(string)
	if jobFlowID == "" {
		t.Fatalf("missing JobFlowId: %s", run.Body.String())
	}

	add := mustJSONTarget(t, handler, "ElasticMapReduce.AddJobFlowSteps", "elasticmapreduce", map[string]any{
		"JobFlowId": jobFlowID,
		"Steps": []map[string]any{
			{
				"Name":             "deny-step",
				"ActionOnFailure":  "CONTINUE",
				"ExecutionRoleArn": "arn:aws:iam::" + testAccountID + ":role/emr-bad",
				"HadoopJarStep": map[string]any{
					"Jar":  "command-runner.jar",
					"Args": []string{"echo", "no"},
				},
			},
		},
	}, now)
	if add.Code != http.StatusForbidden {
		t.Fatalf("AddJobFlowSteps status=%d body=%q", add.Code, add.Body.String())
	}
	if !strings.Contains(add.Body.String(), "not authorized to pass role to EMR") {
		t.Fatalf("body=%q", add.Body.String())
	}
}

func TestVPCFlowDeliverLogsPermissionArnPassRoleDeny(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "vpcflow-bad", passRoleTrustBad, now)
	create := mustVPCFlowJSON(t, handler, "AmazonEC2.CreateFlowLogs", map[string]any{
		"ResourceIds":              []string{"vpc-passrole001"},
		"ResourceType":             "VPC",
		"TrafficType":              "ALL",
		"LogDestinationType":       "cloud-watch-logs",
		"LogDestination":           "arn:aws:logs:us-east-1:" + testAccountID + ":log-group:flow",
		"DeliverLogsPermissionArn": "arn:aws:iam::" + testAccountID + ":role/vpcflow-bad",
	}, now)
	if create.Code != http.StatusForbidden {
		t.Fatalf("CreateFlowLogs status=%d body=%q", create.Code, create.Body.String())
	}
	if !strings.Contains(create.Body.String(), "not authorized to pass role to VPC Flow Logs") {
		t.Fatalf("body=%q", create.Body.String())
	}
}

func TestAPIGatewayRESTPutIntegrationCredentialsPassRoleDeny(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "apigw-rest-cred-bad", lambdaTrustOK, now)
	badARN := "arn:aws:iam::" + testAccountID + ":role/apigw-rest-cred-bad"

	create := mustAPIGatewayREST(t, handler, http.MethodPost, "/restapis", map[string]any{
		"name": "passrole-rest",
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateRestApi status=%d body=%q", create.Code, create.Body.String())
	}
	var apiResp map[string]any
	_ = json.Unmarshal(create.Body.Bytes(), &apiResp)
	apiID, _ := apiResp["id"].(string)
	rootID, _ := apiResp["rootResourceId"].(string)

	methodRec := mustAPIGatewayREST(t, handler, http.MethodPut,
		"/restapis/"+apiID+"/resources/"+rootID+"/methods/GET",
		map[string]any{"authorizationType": "NONE"}, now)
	if methodRec.Code != http.StatusCreated {
		t.Fatalf("PutMethod status=%d body=%q", methodRec.Code, methodRec.Body.String())
	}

	intRec := mustAPIGatewayREST(t, handler, http.MethodPut,
		"/restapis/"+apiID+"/resources/"+rootID+"/methods/GET/integration",
		map[string]any{
			"type":        "AWS_PROXY",
			"uri":         "arn:aws:apigateway:us-east-1:lambda:path/2015-03-31/functions/arn:aws:lambda:us-east-1:" + testAccountID + ":function:x/invocations",
			"credentials": badARN,
		}, now)
	if intRec.Code != http.StatusForbidden {
		t.Fatalf("PutIntegration status=%d body=%q", intRec.Code, intRec.Body.String())
	}
	if !strings.Contains(intRec.Body.String(), "not authorized to pass role to API Gateway") {
		t.Fatalf("body=%q", intRec.Body.String())
	}
}

func TestSecretsRotationRoleARNConfigurePassRoleDeny(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "secrets-rot-bad", passRoleTrustBad, now)
	createSec := mustSecretsJSON(t, handler, "CreateSecret", map[string]any{
		"Name":         "passrole-rot-cfg",
		"SecretString": "v1",
	}, now)
	if createSec.Code != http.StatusOK {
		t.Fatalf("CreateSecret status=%d body=%q", createSec.Code, createSec.Body.String())
	}
	rot := mustSecretsJSON(t, handler, "RotateSecret", map[string]any{
		"SecretId":          "passrole-rot-cfg",
		"RotationRoleARN":   "arn:aws:iam::" + testAccountID + ":role/secrets-rot-bad",
		"RotateImmediately": false,
	}, now)
	if rot.Code != http.StatusForbidden {
		t.Fatalf("RotateSecret configure status=%d body=%q", rot.Code, rot.Body.String())
	}
	if !strings.Contains(rot.Body.String(), "not authorized to pass role to Secrets Manager") {
		t.Fatalf("body=%q", rot.Body.String())
	}
}

func TestIoTCreateRoleAliasPassRoleDeny(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "iot-alias-bad", passRoleTrustBad, now)
	rec := mustJSONTarget(t, handler, "AWSIotService.CreateRoleAlias", "iot", map[string]any{
		"roleAlias": "passrole-deny-alias",
		"roleArn":   "arn:aws:iam::" + testAccountID + ":role/iot-alias-bad",
	}, now)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("CreateRoleAlias status=%d body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "not authorized to pass role to IoT") {
		t.Fatalf("body=%q", rec.Body.String())
	}
}
