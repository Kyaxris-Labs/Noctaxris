package sdk_test

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	"github.com/aws/aws-sdk-go-v2/service/elasticbeanstalk"
	"github.com/aws/aws-sdk-go-v2/service/lightsail"
)

func TestLightsailCreateGetDelete(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	c := lightsail.NewFromConfig(cfg, func(o *lightsail.Options) {
		o.BaseEndpoint = aws.String(endpoint())
	})
	ctx := context.Background()
	name := uniquePrefix(t) + "-ls"

	if _, err := c.CreateInstances(ctx, &lightsail.CreateInstancesInput{
		InstanceNames:    []string{name},
		AvailabilityZone: aws.String("us-east-1a"),
		BlueprintId:      aws.String("ubuntu_22_04"),
		BundleId:         aws.String("nano_3_0"),
	}); err != nil {
		t.Fatalf("CreateInstances: %v", err)
	}
	t.Cleanup(func() {
		_, _ = c.DeleteInstance(ctx, &lightsail.DeleteInstanceInput{InstanceName: aws.String(name)})
	})

	got, err := c.GetInstance(ctx, &lightsail.GetInstanceInput{InstanceName: aws.String(name)})
	if err != nil {
		t.Fatalf("GetInstance: %v", err)
	}
	if got.Instance == nil || aws.ToString(got.Instance.Name) != name {
		t.Fatalf("instance=%+v", got.Instance)
	}
}

func TestAutoScalingGroupDesiredCapacity(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	c := autoscaling.NewFromConfig(cfg, func(o *autoscaling.Options) {
		o.BaseEndpoint = aws.String(endpoint())
	})
	ctx := context.Background()
	prefix := uniquePrefix(t)
	lc := prefix + "-lc"
	asg := prefix + "-asg"

	if _, err := c.CreateLaunchConfiguration(ctx, &autoscaling.CreateLaunchConfigurationInput{
		LaunchConfigurationName: aws.String(lc),
		ImageId:                 aws.String("ami-12345678"),
		InstanceType:            aws.String("t3.micro"),
	}); err != nil {
		t.Fatalf("CreateLaunchConfiguration: %v", err)
	}
	t.Cleanup(func() {
		_, _ = c.DeleteAutoScalingGroup(ctx, &autoscaling.DeleteAutoScalingGroupInput{
			AutoScalingGroupName: aws.String(asg), ForceDelete: aws.Bool(true),
		})
		_, _ = c.DeleteLaunchConfiguration(ctx, &autoscaling.DeleteLaunchConfigurationInput{
			LaunchConfigurationName: aws.String(lc),
		})
	})

	if _, err := c.CreateAutoScalingGroup(ctx, &autoscaling.CreateAutoScalingGroupInput{
		AutoScalingGroupName:    aws.String(asg),
		LaunchConfigurationName: aws.String(lc),
		MinSize:                 aws.Int32(1),
		MaxSize:                 aws.Int32(3),
		DesiredCapacity:         aws.Int32(1),
		AvailabilityZones:       []string{"us-east-1a"},
	}); err != nil {
		t.Fatalf("CreateAutoScalingGroup: %v", err)
	}

	if _, err := c.SetDesiredCapacity(ctx, &autoscaling.SetDesiredCapacityInput{
		AutoScalingGroupName: aws.String(asg),
		DesiredCapacity:      aws.Int32(2),
	}); err != nil {
		t.Fatalf("SetDesiredCapacity: %v", err)
	}

	desc, err := c.DescribeAutoScalingGroups(ctx, &autoscaling.DescribeAutoScalingGroupsInput{
		AutoScalingGroupNames: []string{asg},
	})
	if err != nil || len(desc.AutoScalingGroups) != 1 {
		t.Fatalf("DescribeAutoScalingGroups: %+v err=%v", desc, err)
	}
	if aws.ToInt32(desc.AutoScalingGroups[0].DesiredCapacity) != 2 {
		t.Fatalf("desired=%v", desc.AutoScalingGroups[0].DesiredCapacity)
	}
	members := desc.AutoScalingGroups[0].Instances
	if len(members) != 2 {
		t.Fatalf("expected 2 Instances after SetDesiredCapacity, got %+v", members)
	}
	for i, m := range members {
		id := aws.ToString(m.InstanceId)
		if !strings.HasPrefix(id, "i-") {
			t.Fatalf("member[%d] InstanceId=%q", i, id)
		}
		ls := string(m.LifecycleState)
		if ls != "Pending" && ls != "InService" {
			t.Fatalf("member[%d] LifecycleState=%q (Pending OK without engine)", i, ls)
		}
	}
}

func TestBeanstalkApplicationEnvironment(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	c := elasticbeanstalk.NewFromConfig(cfg, func(o *elasticbeanstalk.Options) {
		o.BaseEndpoint = aws.String(endpoint())
	})
	ctx := context.Background()
	prefix := uniquePrefix(t)
	app := prefix + "-app"
	env := prefix + "-env"

	if _, err := c.CreateApplication(ctx, &elasticbeanstalk.CreateApplicationInput{
		ApplicationName: aws.String(app),
	}); err != nil {
		t.Fatalf("CreateApplication: %v", err)
	}
	t.Cleanup(func() {
		_, _ = c.TerminateEnvironment(ctx, &elasticbeanstalk.TerminateEnvironmentInput{
			EnvironmentName: aws.String(env),
		})
		_, _ = c.DeleteApplication(ctx, &elasticbeanstalk.DeleteApplicationInput{
			ApplicationName:     aws.String(app),
			TerminateEnvByForce: aws.Bool(true),
		})
	})

	created, err := c.CreateEnvironment(ctx, &elasticbeanstalk.CreateEnvironmentInput{
		ApplicationName: aws.String(app),
		EnvironmentName: aws.String(env),
	})
	if err != nil {
		t.Fatalf("CreateEnvironment: %v", err)
	}
	if string(created.Status) != "Ready" {
		t.Fatalf("status=%q", string(created.Status))
	}
}

func TestBackupVaultPlanJob(t *testing.T) {
	requireReady(t)
	prefix := uniquePrefix(t)
	vault := strings.ToLower(prefix + "-vault")

	status, body := signedHTTP(t, "backup", "PUT", "/backup-vaults/"+url.PathEscape(vault), mustJSONBytes(t, map[string]any{}), "application/json")
	if status != 200 {
		t.Fatalf("CreateBackupVault status=%d body=%s", status, body)
	}
	t.Cleanup(func() {
		_, _ = signedHTTP(t, "backup", "DELETE", "/backup-vaults/"+url.PathEscape(vault), nil, "")
	})

	status, body = signedHTTP(t, "backup", "PUT", "/backup/plans/", mustJSONBytes(t, map[string]any{
		"BackupPlan": map[string]any{
			"BackupPlanName": prefix + "-plan",
			"Rules": []map[string]any{
				{"RuleName": "daily", "TargetBackupVaultName": vault, "ScheduleExpression": "cron(0 5 ? * * *)"},
			},
		},
	}), "application/json")
	if status != 200 || !strings.Contains(string(body), "BackupPlanId") {
		t.Fatalf("CreateBackupPlan status=%d body=%s", status, body)
	}
	var plan map[string]any
	_ = json.Unmarshal(body, &plan)
	if id, _ := plan["BackupPlanId"].(string); id != "" {
		t.Cleanup(func() {
			_, _ = signedHTTP(t, "backup", "DELETE", "/backup/plans/"+url.PathEscape(id), nil, "")
		})
	}

	status, body = signedHTTP(t, "backup", "PUT", "/backup-jobs", mustJSONBytes(t, map[string]any{
		"BackupVaultName": vault,
		"ResourceArn":     "arn:aws:s3:::lab-bucket",
		"IamRoleArn":      "arn:aws:iam::000000000001:role/Backup",
	}), "application/json")
	if status != 200 || !strings.Contains(string(body), "RecoveryPointArn") {
		t.Fatalf("StartBackupJob status=%d body=%s", status, body)
	}

	status, body = signedHTTP(t, "backup", "GET", "/backup-vaults/"+url.PathEscape(vault)+"/recovery-points/", nil, "")
	if status != 200 || !strings.Contains(string(body), "S3") {
		t.Fatalf("ListRecoveryPoints status=%d body=%s", status, body)
	}
}

func mustJSONBytes(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
