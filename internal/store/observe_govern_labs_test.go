package store_test

import (
	"errors"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestLightsailInstanceLifecycle(t *testing.T) {
	st := openStreamCStore(t)
	account := "000000000001"
	created, err := st.CreateLightsailInstances(account, "us-east-1", []string{"web-a"}, "us-east-1a", "ubuntu_22_04", "nano_3_0")
	if err != nil {
		t.Fatal(err)
	}
	if len(created) != 1 || created[0].StateName != "running" {
		t.Fatalf("created=%+v", created)
	}
	got, err := st.GetLightsailInstance(account, "us-east-1", "web-a")
	if err != nil {
		t.Fatal(err)
	}
	if got.BlueprintID != "ubuntu_22_04" {
		t.Fatalf("blueprint=%q", got.BlueprintID)
	}
	stopped, err := st.StopLightsailInstance(account, "us-east-1", "web-a")
	if err != nil || stopped.StateName != "stopped" {
		t.Fatalf("stop: %+v err=%v", stopped, err)
	}
	started, err := st.StartLightsailInstance(account, "us-east-1", "web-a")
	if err != nil || started.StateName != "running" {
		t.Fatalf("start: %+v err=%v", started, err)
	}
	if err := st.DeleteLightsailInstance(account, "us-east-1", "web-a"); err != nil {
		t.Fatal(err)
	}
	_, err = st.GetLightsailInstance(account, "us-east-1", "web-a")
	if !errors.Is(err, store.ErrLightsailNotFound) {
		t.Fatalf("after delete err=%v", err)
	}
}

func TestASGDesiredCapacityNoInstances(t *testing.T) {
	st := openStreamCStore(t)
	account := "000000000001"
	_, err := st.CreateLaunchConfiguration(account, "us-east-1", "lc-1", "ami-123", "t3.micro", "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	g, err := st.CreateAutoScalingGroup(account, "us-east-1", "asg-1", "lc-1", 1, 3, 2, []string{"us-east-1a"}, "", "EC2", 300)
	if err != nil {
		t.Fatal(err)
	}
	if g.DesiredCapacity != 2 {
		t.Fatalf("desired=%d", g.DesiredCapacity)
	}
	if len(g.Instances) != 0 {
		t.Fatalf("create without reconcile must leave Instances empty, got %+v", g.Instances)
	}
	if err := st.SetDesiredCapacity(account, "us-east-1", "asg-1", 3); err != nil {
		t.Fatal(err)
	}
	list, err := st.DescribeAutoScalingGroups(account, "us-east-1", []string{"asg-1"})
	if err != nil || len(list) != 1 || list[0].DesiredCapacity != 3 {
		t.Fatalf("list=%+v err=%v", list, err)
	}
}

func TestBeanstalkAppEnvLifecycle(t *testing.T) {
	st := openStreamCStore(t)
	account := "000000000001"
	_, err := st.CreateBeanstalkApplication(account, "us-east-1", "sample-app", "lab")
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.CreateBeanstalkApplicationVersion(account, "us-east-1", "sample-app", "v1", "", "src", "app.zip")
	if err != nil {
		t.Fatal(err)
	}
	env, err := st.CreateBeanstalkEnvironment(account, "us-east-1", "sample-app", "sample-env", "v1", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if env.Status != "Ready" || env.Health != "Green" {
		t.Fatalf("env=%+v", env)
	}
	term, err := st.TerminateBeanstalkEnvironment(account, "us-east-1", "sample-env", "")
	if err != nil || term.Status != "Terminated" {
		t.Fatalf("term=%+v err=%v", term, err)
	}
	if err := st.DeleteBeanstalkApplication(account, "us-east-1", "sample-app", true); err != nil {
		t.Fatal(err)
	}
}

func TestBackupJobCreatesRecoveryPoint(t *testing.T) {
	st := openStreamCStore(t)
	account := "000000000001"
	_, err := st.CreateBackupVault(account, "us-east-1", "lab-vault", "")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := st.CreateBackupPlan(account, "us-east-1", "daily", []any{
		map[string]any{"RuleName": "daily", "TargetBackupVaultName": "lab-vault", "ScheduleExpression": "cron(0 5 ? * * *)"},
	})
	if err != nil || plan.BackupPlanID == "" {
		t.Fatalf("plan=%+v err=%v", plan, err)
	}
	resource := "arn:aws:s3:::lab-bucket"
	job, rp, err := st.StartBackupJob(account, "us-east-1", "lab-vault", resource, "arn:aws:iam::000000000001:role/Backup")
	if err != nil {
		t.Fatal(err)
	}
	if job.State != "COMPLETED" || rp.ResourceType != "S3" {
		t.Fatalf("job=%+v rp=%+v", job, rp)
	}
	listed, err := st.ListRecoveryPointsByBackupVault(account, "us-east-1", "lab-vault")
	if err != nil || len(listed) != 1 {
		t.Fatalf("listed=%+v err=%v", listed, err)
	}
	v, err := st.DescribeBackupVault(account, "us-east-1", "lab-vault")
	if err != nil || v.NumberOfRecoveryPoints != 1 {
		t.Fatalf("vault=%+v err=%v", v, err)
	}
}
