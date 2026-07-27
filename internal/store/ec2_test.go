package store_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestResolveEC2AMI(t *testing.T) {
	if got := store.ResolveEC2AMI("ami-alpine"); got != store.DefaultEC2DockerImage {
		t.Fatalf("alpine=%q", got)
	}
	if got := store.ResolveEC2AMI("ami-amazonlinux2023"); got != "public.ecr.aws/amazonlinux/amazonlinux:2023" {
		t.Fatalf("al2023=%q", got)
	}
	if got := store.ResolveEC2AMI("ami-ubuntu2204"); got != "public.ecr.aws/docker/library/ubuntu:22.04" {
		t.Fatalf("ubuntu=%q", got)
	}
	if got := store.ResolveEC2AMI("ami-0abc12345678"); got != store.DefaultEC2DockerImage {
		t.Fatalf("unknown fallback=%q", got)
	}
}

func TestDescribeEC2Images(t *testing.T) {
	all := store.DescribeEC2Images(nil)
	if len(all) != 3 {
		t.Fatalf("catalog len=%d want 3", len(all))
	}
	filtered := store.DescribeEC2Images([]string{"ami-ubuntu2204"})
	if len(filtered) != 1 || filtered[0].ImageID != "ami-ubuntu2204" {
		t.Fatalf("filter=%+v", filtered)
	}
	alias := store.DescribeEC2Images([]string{"ami-0abcdef1234567891"})
	if len(alias) != 1 || alias[0].ImageID != "ami-amazonlinux2023" {
		t.Fatalf("alias=%+v", alias)
	}
	empty := store.DescribeEC2Images([]string{"ami-missing"})
	if len(empty) != 0 {
		t.Fatalf("missing=%+v", empty)
	}
}

func TestRunInstancesUnknownAMIDefaultsDocker(t *testing.T) {
	dir := t.TempDir()
	key, err := store.LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	list, err := st.RunInstances("000000000001", "us-east-1", store.RunInstancesInput{
		ImageID: "ami-0realawsami12345", MinCount: 1, MaxCount: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if list[0].ImageID != "ami-0realawsami12345" {
		t.Fatalf("ImageID=%q", list[0].ImageID)
	}
	if list[0].DockerImage != store.DefaultEC2DockerImage {
		t.Fatalf("DockerImage=%q want alpine default", list[0].DockerImage)
	}
}

func TestRunDescribeStopStartTerminateInstances(t *testing.T) {
	dir := t.TempDir()
	key, err := store.LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	account := "000000000001"
	region := "us-east-1"
	list, err := st.RunInstances(account, region, store.RunInstancesInput{
		ImageID: "ami-alpine", MinCount: 1, MaxCount: 2, InstanceType: "t3.micro",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("want 2 instances, got %d", len(list))
	}
	for _, inst := range list {
		if inst.StateName != store.EC2StatePending {
			t.Fatalf("state=%q want pending", inst.StateName)
		}
		if inst.DockerImage != store.DefaultEC2DockerImage {
			t.Fatalf("docker=%q", inst.DockerImage)
		}
		if !strings.HasPrefix(inst.InstanceID, "i-") {
			t.Fatalf("instance id=%q", inst.InstanceID)
		}
	}

	desc, err := st.DescribeInstances(account, region, []string{list[0].InstanceID})
	if err != nil || len(desc) != 1 {
		t.Fatalf("describe: %v %#v", err, desc)
	}

	if err := st.SetEC2ContainerID(account, region, list[0].InstanceID, "cid-1", store.EC2StateRunning); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetEC2Instance(account, region, list[0].InstanceID)
	if err != nil {
		t.Fatal(err)
	}
	if got.StateName != store.EC2StateRunning || got.ContainerID != "cid-1" {
		t.Fatalf("got=%+v", got)
	}

	if err := st.SetEC2InstanceState(account, region, list[0].InstanceID, store.EC2StateStopped); err != nil {
		t.Fatal(err)
	}
	got, _ = st.GetEC2Instance(account, region, list[0].InstanceID)
	if got.StateName != store.EC2StateStopped || got.StateCode != store.EC2StateCodeStopped {
		t.Fatalf("stopped=%+v", got)
	}

	if err := st.SetEC2InstanceState(account, region, list[0].InstanceID, store.EC2StateTerminated); err != nil {
		t.Fatal(err)
	}
	_ = st.ClearEC2ContainerID(account, region, list[0].InstanceID)
	got, _ = st.GetEC2Instance(account, region, list[0].InstanceID)
	if got.StateName != store.EC2StateTerminated || got.ContainerID != "" {
		t.Fatalf("terminated=%+v", got)
	}
}
