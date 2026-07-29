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

func TestLightsailDiskStaticIPKeyPairPorts(t *testing.T) {
	st := openStreamCStore(t)
	account := "000000000001"
	region := "us-east-1"
	if _, err := st.CreateLightsailInstances(account, region, []string{"web-a"}, "us-east-1a", "ubuntu_22_04", "nano_3_0"); err != nil {
		t.Fatal(err)
	}

	disk, err := st.CreateLightsailDisk(account, region, "data-1", "us-east-1a", 32)
	if err != nil || disk.State != "available" {
		t.Fatalf("create disk: %+v err=%v", disk, err)
	}
	if _, err := st.CreateLightsailDisk(account, region, "data-1", "us-east-1a", 32); !errors.Is(err, store.ErrLightsailExists) {
		t.Fatalf("dup disk err=%v", err)
	}
	if _, err := st.CreateLightsailDisk(account, region, "tiny", "us-east-1a", 4); !errors.Is(err, store.ErrLightsailBadRequest) {
		t.Fatalf("small disk err=%v", err)
	}
	attached, err := st.AttachLightsailDisk(account, region, "data-1", "web-a", "/dev/xvdf")
	if err != nil || !attached.IsAttached || attached.State != "in-use" {
		t.Fatalf("attach: %+v err=%v", attached, err)
	}
	if _, err := st.DeleteLightsailDisk(account, region, "data-1"); !errors.Is(err, store.ErrLightsailBadRequest) {
		t.Fatalf("delete attached err=%v", err)
	}
	detached, err := st.DetachLightsailDisk(account, region, "data-1")
	if err != nil || detached.IsAttached {
		t.Fatalf("detach: %+v err=%v", detached, err)
	}
	if _, err := st.DeleteLightsailDisk(account, region, "data-1"); err != nil {
		t.Fatal(err)
	}

	ip, err := st.AllocateLightsailStaticIP(account, region, "web-ip")
	if err != nil || ip.IPAddress == "" {
		t.Fatalf("allocate: %+v err=%v", ip, err)
	}
	attachedIP, err := st.AttachLightsailStaticIP(account, region, "web-ip", "web-a")
	if err != nil || !attachedIP.IsAttached {
		t.Fatalf("attach ip: %+v err=%v", attachedIP, err)
	}
	inst, err := st.GetLightsailInstance(account, region, "web-a")
	if err != nil || !inst.IsStaticIP || inst.PublicIP != attachedIP.IPAddress {
		t.Fatalf("instance after attach: %+v err=%v", inst, err)
	}
	if _, err := st.DetachLightsailStaticIP(account, region, "web-ip"); err != nil {
		t.Fatal(err)
	}
	inst, err = st.GetLightsailInstance(account, region, "web-a")
	if err != nil || inst.IsStaticIP {
		t.Fatalf("instance after detach: %+v err=%v", inst, err)
	}
	if _, err := st.ReleaseLightsailStaticIP(account, region, "web-ip"); err != nil {
		t.Fatal(err)
	}

	kp, priv, err := st.CreateLightsailKeyPair(account, region, "lab-key")
	if err != nil || kp.Fingerprint == "" || priv == "" {
		t.Fatalf("create key: %+v priv=%q err=%v", kp, priv, err)
	}
	gotKP, err := st.GetLightsailKeyPair(account, region, "lab-key")
	if err != nil || gotKP.Name != "lab-key" {
		t.Fatalf("get key: %+v err=%v", gotKP, err)
	}
	if _, err := st.DeleteLightsailKeyPair(account, region, "lab-key"); err != nil {
		t.Fatal(err)
	}

	if _, _, err := st.OpenLightsailInstancePublicPorts(account, region, "web-a", map[string]any{
		"fromPort": float64(443), "toPort": float64(443), "protocol": "tcp", "cidrs": []any{"0.0.0.0/0"},
	}); err != nil {
		t.Fatal(err)
	}
	ports, err := st.GetLightsailInstancePortStates(account, region, "web-a")
	if err != nil || len(ports) != 1 || ports[0].FromPort != 443 {
		t.Fatalf("ports=%+v err=%v", ports, err)
	}
	if _, err := st.CloseLightsailInstancePublicPorts(account, region, "web-a", map[string]any{
		"fromPort": float64(443), "toPort": float64(443), "protocol": "tcp",
	}); err != nil {
		t.Fatal(err)
	}
	ports, err = st.GetLightsailInstancePortStates(account, region, "web-a")
	if err != nil || len(ports) != 0 {
		t.Fatalf("after close ports=%+v err=%v", ports, err)
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

func TestBackupSelectionListJobsStopDeleteRecoveryPoint(t *testing.T) {
	st := openStreamCStore(t)
	account := "000000000001"
	if _, err := st.CreateBackupVault(account, "us-east-1", "lab-vault", ""); err != nil {
		t.Fatal(err)
	}
	plan, err := st.CreateBackupPlan(account, "us-east-1", "daily", []any{
		map[string]any{"RuleName": "daily", "TargetBackupVaultName": "lab-vault"},
	})
	if err != nil {
		t.Fatal(err)
	}
	resources := []string{"arn:aws:s3:::lab-bucket", "arn:aws:dynamodb:us-east-1:000000000001:table/lab"}
	sel, err := st.CreateBackupSelection(account, "us-east-1", plan.BackupPlanID, "lab-sel",
		"arn:aws:iam::000000000001:role/Backup", resources)
	if err != nil || sel.SelectionID == "" {
		t.Fatalf("selection=%+v err=%v", sel, err)
	}
	got, err := st.GetBackupSelection(account, "us-east-1", plan.BackupPlanID, sel.SelectionID)
	if err != nil || got.IamRoleARN == "" || got.ResourcesJSON == "" {
		t.Fatalf("get=%+v err=%v", got, err)
	}
	sels, err := st.ListBackupSelections(account, "us-east-1", plan.BackupPlanID)
	if err != nil || len(sels) != 1 {
		t.Fatalf("list selections=%+v err=%v", sels, err)
	}

	jobS3, _, err := st.StartBackupJob(account, "us-east-1", "lab-vault", resources[0], "arn:aws:iam::000000000001:role/Backup")
	if err != nil {
		t.Fatal(err)
	}
	jobDDB, rp, err := st.StartBackupJob(account, "us-east-1", "lab-vault", resources[1], "arn:aws:iam::000000000001:role/Backup")
	if err != nil {
		t.Fatal(err)
	}
	all, err := st.ListBackupJobs(account, "us-east-1", store.BackupJobListFilter{})
	if err != nil || len(all) != 2 {
		t.Fatalf("all jobs=%+v err=%v", all, err)
	}
	filtered, err := st.ListBackupJobs(account, "us-east-1", store.BackupJobListFilter{
		ResourceARN: resources[0], State: "COMPLETED",
	})
	if err != nil || len(filtered) != 1 || filtered[0].BackupJobID != jobS3.BackupJobID {
		t.Fatalf("filtered=%+v err=%v", filtered, err)
	}
	byType, err := st.ListBackupJobs(account, "us-east-1", store.BackupJobListFilter{ResourceType: "DynamoDB"})
	if err != nil || len(byType) != 1 || byType[0].BackupJobID != jobDDB.BackupJobID {
		t.Fatalf("byType=%+v err=%v", byType, err)
	}
	if err := st.StopBackupJob(account, "us-east-1", jobS3.BackupJobID); err != nil {
		t.Fatalf("stop completed: %v", err)
	}
	if err := st.StopBackupJob(account, "us-east-1", "missing-job"); !errors.Is(err, store.ErrBackupNotFound) {
		t.Fatalf("stop missing err=%v", err)
	}

	if err := st.DeleteRecoveryPoint(account, "us-east-1", "lab-vault", rp.RecoveryPointARN); err != nil {
		t.Fatal(err)
	}
	listed, err := st.ListRecoveryPointsByBackupVault(account, "us-east-1", "lab-vault")
	if err != nil || len(listed) != 1 {
		t.Fatalf("after delete listed=%+v err=%v", listed, err)
	}
	v, err := st.DescribeBackupVault(account, "us-east-1", "lab-vault")
	if err != nil || v.NumberOfRecoveryPoints != 1 {
		t.Fatalf("vault count=%+v err=%v", v, err)
	}

	if err := st.DeleteBackupSelection(account, "us-east-1", plan.BackupPlanID, sel.SelectionID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetBackupSelection(account, "us-east-1", plan.BackupPlanID, sel.SelectionID); !errors.Is(err, store.ErrBackupNotFound) {
		t.Fatalf("after delete selection err=%v", err)
	}
}
