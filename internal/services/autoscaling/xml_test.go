package autoscaling_test

import (
	"strings"
	"testing"

	asgsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/autoscaling"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestAutoScalingXML(t *testing.T) {
	req := "req-asg"
	if _, err := asgsvc.EmptyOKXML("UpdateAutoScalingGroup", req); err != nil {
		t.Fatal(err)
	}
	lc := store.ASGLaunchConfiguration{
		LaunchConfigurationName: "lab-lc", ImageID: "ami-1", InstanceType: "t3.micro",
	}
	lcOut, err := asgsvc.DescribeLaunchConfigurationsXML([]store.ASGLaunchConfiguration{lc}, req)
	if err != nil || !strings.Contains(string(lcOut), "lab-lc") {
		t.Fatalf("lc: %s %v", lcOut, err)
	}
	asg := store.AutoScalingGroup{
		AutoScalingGroupName: "lab-asg", LaunchConfigurationName: lc.LaunchConfigurationName,
		MinSize: 1, MaxSize: 3, DesiredCapacity: 2,
	}
	if _, err := asgsvc.DescribeAutoScalingGroupsXML([]store.AutoScalingGroup{asg}, req); err != nil {
		t.Fatal(err)
	}
	if _, err := asgsvc.PutScalingPolicyXML("arn:aws:autoscaling:us-east-1:1:scalingPolicy:uuid", req); err != nil {
		t.Fatal(err)
	}
	pol := store.ASGScalingPolicy{PolicyName: "cpu", AutoScalingGroupName: asg.AutoScalingGroupName, PolicyARN: "arn:policy"}
	if _, err := asgsvc.DescribePoliciesXML([]store.ASGScalingPolicy{pol}, req); err != nil {
		t.Fatal(err)
	}
	hook := store.ASGLifecycleHook{LifecycleHookName: "hook", AutoScalingGroupName: asg.AutoScalingGroupName}
	if _, err := asgsvc.DescribeLifecycleHooksXML([]store.ASGLifecycleHook{hook}, req); err != nil {
		t.Fatal(err)
	}
	inst := store.ASGDescribedInstance{InstanceID: "i-1", AutoScalingGroupName: asg.AutoScalingGroupName, LifecycleState: "InService"}
	if _, err := asgsvc.DescribeAutoScalingInstancesXML([]store.ASGDescribedInstance{inst}, req); err != nil {
		t.Fatal(err)
	}
	if _, err := asgsvc.DetachInstancesXML(req); err != nil {
		t.Fatal(err)
	}
	if _, err := asgsvc.DescribeLoadBalancerTargetGroupsXML([]string{"arn:aws:elasticloadbalancing:us-east-1:1:targetgroup/tg/1"}, req); err != nil {
		t.Fatal(err)
	}
}
