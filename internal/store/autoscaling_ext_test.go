package store_test

import (
	"errors"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestASGScalingPolicyLifecycle(t *testing.T) {
	st := openStreamCStore(t)
	account := "000000000001"
	region := "us-east-1"

	_, err := st.CreateLaunchConfiguration(account, region, "lc-pol", "ami-alpine", "t3.micro", "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.CreateAutoScalingGroup(account, region, "asg-pol", "lc-pol", 0, 3, 0, []string{"us-east-1a"}, "", "EC2", 300)
	if err != nil {
		t.Fatal(err)
	}

	warmup := 120
	tt := &store.ASGTargetTrackingConfiguration{
		TargetValue:          40,
		PredefinedMetricType: "ASGAverageCPUUtilization",
	}
	pol, err := st.PutScalingPolicy(account, region, "asg-pol", "cpu-track", "TargetTrackingScaling", "", 0, 300, &warmup, tt)
	if err != nil {
		t.Fatal(err)
	}
	if pol.PolicyARN == "" || pol.PolicyType != "TargetTrackingScaling" {
		t.Fatalf("policy=%+v", pol)
	}

	list, err := st.DescribePolicies(account, region, "asg-pol", nil)
	if err != nil || len(list) != 1 {
		t.Fatalf("describe=%+v err=%v", list, err)
	}
	if list[0].TargetTracking == nil || list[0].TargetTracking.TargetValue != 40 {
		t.Fatalf("target tracking=%+v", list[0].TargetTracking)
	}
	if list[0].EstimatedInstanceWarmup == nil || *list[0].EstimatedInstanceWarmup != 120 {
		t.Fatalf("warmup=%v", list[0].EstimatedInstanceWarmup)
	}

	_, err = st.PutScalingPolicy(account, region, "asg-pol", "cpu-track", "SimpleScaling", "ChangeInCapacity", 1, 60, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	list, err = st.DescribePolicies(account, region, "asg-pol", []string{"cpu-track"})
	if err != nil || len(list) != 1 || list[0].PolicyType != "SimpleScaling" || list[0].ScalingAdjustment != 1 {
		t.Fatalf("updated=%+v err=%v", list, err)
	}
	if list[0].PolicyARN != pol.PolicyARN {
		t.Fatalf("arn changed on update: %s vs %s", list[0].PolicyARN, pol.PolicyARN)
	}

	if err := st.DeletePolicy(account, region, "asg-pol", "cpu-track"); err != nil {
		t.Fatal(err)
	}
	list, err = st.DescribePolicies(account, region, "asg-pol", nil)
	if err != nil || len(list) != 0 {
		t.Fatalf("after delete=%+v err=%v", list, err)
	}
}

func TestASGLifecycleHooks(t *testing.T) {
	st := openStreamCStore(t)
	account := "000000000001"
	region := "us-east-1"

	_, err := st.CreateLaunchConfiguration(account, region, "lc-hook", "ami-alpine", "t3.micro", "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.CreateAutoScalingGroup(account, region, "asg-hook", "lc-hook", 0, 2, 0, nil, "", "EC2", 300)
	if err != nil {
		t.Fatal(err)
	}

	timeout := 300
	_, err = st.PutLifecycleHook(account, region, "asg-hook", "launch-hook",
		"autoscaling:EC2_INSTANCE_LAUNCHING", "", "", "", &timeout, "CONTINUE")
	if err != nil {
		t.Fatal(err)
	}
	hooks, err := st.DescribeLifecycleHooks(account, region, "asg-hook", nil)
	if err != nil || len(hooks) != 1 {
		t.Fatalf("hooks=%+v err=%v", hooks, err)
	}
	if hooks[0].HeartbeatTimeout != 300 || hooks[0].DefaultResult != "CONTINUE" {
		t.Fatalf("hook=%+v", hooks[0])
	}
	if hooks[0].GlobalTimeout != 172800 {
		t.Fatalf("globalTimeout=%d", hooks[0].GlobalTimeout)
	}

	if err := st.DeleteLifecycleHook(account, region, "asg-hook", "launch-hook"); err != nil {
		t.Fatal(err)
	}
	hooks, err = st.DescribeLifecycleHooks(account, region, "asg-hook", nil)
	if err != nil || len(hooks) != 0 {
		t.Fatalf("after delete=%+v err=%v", hooks, err)
	}
}

func TestASGAttachDetachInstancesNoDinD(t *testing.T) {
	st := openStreamCStore(t)
	account := "000000000001"
	region := "us-east-1"

	_, err := st.CreateLaunchConfiguration(account, region, "lc-att", "ami-alpine", "t3.micro", "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.CreateAutoScalingGroup(account, region, "asg-att", "lc-att", 0, 4, 0, []string{"us-east-1a"}, "", "EC2", 300)
	if err != nil {
		t.Fatal(err)
	}

	launched, err := st.RunInstances(account, region, store.RunInstancesInput{
		ImageID: "ami-alpine", InstanceType: "t3.micro", MinCount: 2, MaxCount: 2, AvailabilityZone: "us-east-1a",
	})
	if err != nil || len(launched) != 2 {
		t.Fatalf("run instances: %+v err=%v", launched, err)
	}
	ids := []string{launched[0].InstanceID, launched[1].InstanceID}

	if err := st.AttachASGInstances(account, region, "asg-att", ids); err != nil {
		t.Fatal(err)
	}
	groups, err := st.DescribeAutoScalingGroups(account, region, []string{"asg-att"})
	if err != nil || len(groups) != 1 {
		t.Fatal(err)
	}
	if groups[0].DesiredCapacity != 2 || len(groups[0].Instances) != 2 {
		t.Fatalf("after attach desired=%d instances=%d", groups[0].DesiredCapacity, len(groups[0].Instances))
	}

	desc, err := st.DescribeAutoScalingInstances(account, region, nil)
	if err != nil || len(desc) != 2 {
		t.Fatalf("describe instances=%+v err=%v", desc, err)
	}
	if desc[0].AutoScalingGroupName != "asg-att" {
		t.Fatalf("asg name=%q", desc[0].AutoScalingGroupName)
	}

	if err := st.DetachASGInstances(account, region, "asg-att", []string{ids[0]}, true); err != nil {
		t.Fatal(err)
	}
	groups, err = st.DescribeAutoScalingGroups(account, region, []string{"asg-att"})
	if err != nil {
		t.Fatal(err)
	}
	if groups[0].DesiredCapacity != 1 || len(groups[0].Instances) != 1 {
		t.Fatalf("after detach desired=%d instances=%d", groups[0].DesiredCapacity, len(groups[0].Instances))
	}

	if err := st.AttachASGInstances(account, region, "asg-att", []string{"i-missing"}); !errors.Is(err, store.ErrASGBadRequest) {
		t.Fatalf("missing instance: %v", err)
	}
}

func TestASGAttachInstancesExceedsMax(t *testing.T) {
	st := openStreamCStore(t)
	account := "000000000001"
	region := "us-east-1"

	_, err := st.CreateLaunchConfiguration(account, region, "lc-max", "ami-alpine", "t3.micro", "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.CreateAutoScalingGroup(account, region, "asg-max", "lc-max", 0, 1, 0, nil, "", "EC2", 300)
	if err != nil {
		t.Fatal(err)
	}
	launched, err := st.RunInstances(account, region, store.RunInstancesInput{
		ImageID: "ami-alpine", InstanceType: "t3.micro", MinCount: 2, MaxCount: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = st.AttachASGInstances(account, region, "asg-max", []string{launched[0].InstanceID, launched[1].InstanceID})
	if !errors.Is(err, store.ErrASGBadRequest) {
		t.Fatalf("want max exceed, got %v", err)
	}
	groups, err := st.DescribeAutoScalingGroups(account, region, []string{"asg-max"})
	if err != nil || len(groups[0].Instances) != 0 {
		t.Fatalf("membership should be empty: %+v err=%v", groups, err)
	}
}

func TestASGTargetGroupAttachValidatesELBv2(t *testing.T) {
	st := openStreamCStore(t)
	account := "000000000001"
	region := "us-east-1"

	_, err := st.CreateLaunchConfiguration(account, region, "lc-tg", "ami-alpine", "t3.micro", "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.CreateAutoScalingGroup(account, region, "asg-tg", "lc-tg", 0, 2, 0, nil, "", "EC2", 300)
	if err != nil {
		t.Fatal(err)
	}

	missing := "arn:aws:elasticloadbalancing:us-east-1:" + account + ":targetgroup/missing/abcdef0123456789"
	if err := st.AttachLoadBalancerTargetGroups(account, region, "asg-tg", []string{missing}); !errors.Is(err, store.ErrASGBadRequest) {
		t.Fatalf("want missing TG validation, got %v", err)
	}

	tg, err := st.CreateELBv2TargetGroup(account, region, "asg-lab-tg", "instance", "HTTP", 80)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachLoadBalancerTargetGroups(account, region, "asg-tg", []string{tg.ARN}); err != nil {
		t.Fatal(err)
	}
	arns, err := st.DescribeLoadBalancerTargetGroups(account, region, "asg-tg")
	if err != nil || len(arns) != 1 || arns[0] != tg.ARN {
		t.Fatalf("arns=%v err=%v", arns, err)
	}
	groups, err := st.DescribeAutoScalingGroups(account, region, []string{"asg-tg"})
	if err != nil || len(groups[0].TargetGroupARNs) != 1 {
		t.Fatalf("group tgs=%+v err=%v", groups, err)
	}

	if err := st.DetachLoadBalancerTargetGroups(account, region, "asg-tg", []string{tg.ARN}); err != nil {
		t.Fatal(err)
	}
	arns, err = st.DescribeLoadBalancerTargetGroups(account, region, "asg-tg")
	if err != nil || len(arns) != 0 {
		t.Fatalf("after detach arns=%v err=%v", arns, err)
	}
}
