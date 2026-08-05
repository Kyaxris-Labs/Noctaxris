package server_test

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestAutoScalingDescribeDeleteLCUpdateAndDetachTG(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)
	account := testAccountID

	createLC := mustASGQuery(t, handler, strings.Join([]string{
		"Action=CreateLaunchConfiguration",
		"Version=2011-01-01",
		"LaunchConfigurationName=lc-cov",
		"ImageId=ami-cov",
		"InstanceType=t3.micro",
	}, "&"), now)
	if createLC.Code != 200 {
		t.Fatalf("CreateLaunchConfiguration status=%d body=%q", createLC.Code, createLC.Body.String())
	}

	descLC := mustASGQuery(t, handler,
		"Action=DescribeLaunchConfigurations&Version=2011-01-01&LaunchConfigurationNames.member.1=lc-cov", now)
	if descLC.Code != 200 || !strings.Contains(descLC.Body.String(), "lc-cov") {
		t.Fatalf("DescribeLaunchConfigurations status=%d body=%q", descLC.Code, descLC.Body.String())
	}
	descAll := mustASGQuery(t, handler, "Action=DescribeLaunchConfigurations&Version=2011-01-01", now)
	if descAll.Code != 200 || !strings.Contains(descAll.Body.String(), "lc-cov") {
		t.Fatalf("DescribeLaunchConfigurations all status=%d body=%q", descAll.Code, descAll.Body.String())
	}

	createASG := mustASGQuery(t, handler, strings.Join([]string{
		"Action=CreateAutoScalingGroup",
		"Version=2011-01-01",
		"AutoScalingGroupName=asg-cov",
		"LaunchConfigurationName=lc-cov",
		"MinSize=0",
		"MaxSize=2",
		"DesiredCapacity=0",
		"AvailabilityZones.member.1=us-east-1a",
	}, "&"), now)
	if createASG.Code != 200 {
		t.Fatalf("CreateAutoScalingGroup status=%d body=%q", createASG.Code, createASG.Body.String())
	}

	upd := mustASGQuery(t, handler, strings.Join([]string{
		"Action=UpdateAutoScalingGroup",
		"Version=2011-01-01",
		"AutoScalingGroupName=asg-cov",
		"MinSize=1",
		"MaxSize=3",
		"DesiredCapacity=1",
		"DefaultCooldown=90",
	}, "&"), now)
	if upd.Code != 200 {
		t.Fatalf("UpdateAutoScalingGroup status=%d body=%q", upd.Code, upd.Body.String())
	}
	desc := mustASGQuery(t, handler,
		"Action=DescribeAutoScalingGroups&Version=2011-01-01&AutoScalingGroupNames.member.1=asg-cov", now)
	if desc.Code != 200 || !strings.Contains(desc.Body.String(), "<DesiredCapacity>1</DesiredCapacity>") {
		t.Fatalf("after update: %q", desc.Body.String())
	}
	if strings.Count(desc.Body.String(), "<InstanceId>") != 1 {
		t.Fatalf("reconcile want 1 instance: %q", desc.Body.String())
	}

	updMiss := mustASGQuery(t, handler, strings.Join([]string{
		"Action=UpdateAutoScalingGroup",
		"Version=2011-01-01",
		"AutoScalingGroupName=missing-asg",
		"DesiredCapacity=1",
	}, "&"), now)
	if updMiss.Code != 400 {
		t.Fatalf("Update missing status=%d body=%q", updMiss.Code, updMiss.Body.String())
	}

	tg, err := st.CreateELBv2TargetGroup(account, "us-east-1", "asg-cov-tg", "instance", "HTTP", 80)
	if err != nil {
		t.Fatal(err)
	}
	attachTG := mustASGQuery(t, handler, strings.Join([]string{
		"Action=AttachLoadBalancerTargetGroups",
		"Version=2011-01-01",
		"AutoScalingGroupName=asg-cov",
		"TargetGroupARNs.member.1=" + url.QueryEscape(tg.ARN),
	}, "&"), now)
	if attachTG.Code != 200 {
		t.Fatalf("AttachLoadBalancerTargetGroups status=%d body=%q", attachTG.Code, attachTG.Body.String())
	}
	detachTG := mustASGQuery(t, handler, strings.Join([]string{
		"Action=DetachLoadBalancerTargetGroups",
		"Version=2011-01-01",
		"AutoScalingGroupName=asg-cov",
		"TargetGroupARNs.member.1=" + url.QueryEscape(tg.ARN),
	}, "&"), now)
	if detachTG.Code != 200 {
		t.Fatalf("DetachLoadBalancerTargetGroups status=%d body=%q", detachTG.Code, detachTG.Body.String())
	}
	detachMiss := mustASGQuery(t, handler, strings.Join([]string{
		"Action=DetachLoadBalancerTargetGroups",
		"Version=2011-01-01",
		"AutoScalingGroupName=missing-asg",
		"TargetGroupARNs.member.1=" + url.QueryEscape(tg.ARN),
	}, "&"), now)
	if detachMiss.Code != 400 {
		t.Fatalf("Detach missing ASG status=%d body=%q", detachMiss.Code, detachMiss.Body.String())
	}

	delLCInUse := mustASGQuery(t, handler, strings.Join([]string{
		"Action=DeleteLaunchConfiguration",
		"Version=2011-01-01",
		"LaunchConfigurationName=lc-cov",
	}, "&"), now)
	// In-use may be rejected or allowed depending on store; force-delete ASG first if needed.
	if delLCInUse.Code == 200 {
		t.Log("DeleteLaunchConfiguration while ASG exists succeeded")
	}

	delASG := mustASGQuery(t, handler, strings.Join([]string{
		"Action=DeleteAutoScalingGroup",
		"Version=2011-01-01",
		"AutoScalingGroupName=asg-cov",
		"ForceDelete=true",
	}, "&"), now)
	if delASG.Code != 200 {
		t.Fatalf("DeleteAutoScalingGroup status=%d body=%q", delASG.Code, delASG.Body.String())
	}

	delLC := mustASGQuery(t, handler, strings.Join([]string{
		"Action=DeleteLaunchConfiguration",
		"Version=2011-01-01",
		"LaunchConfigurationName=lc-cov",
	}, "&"), now)
	if delLC.Code != 200 && delLCInUse.Code != 200 {
		t.Fatalf("DeleteLaunchConfiguration status=%d body=%q", delLC.Code, delLC.Body.String())
	}
	delLCMiss := mustASGQuery(t, handler, strings.Join([]string{
		"Action=DeleteLaunchConfiguration",
		"Version=2011-01-01",
		"LaunchConfigurationName=missing-lc",
	}, "&"), now)
	if delLCMiss.Code != 400 {
		t.Fatalf("DeleteLaunchConfiguration missing status=%d body=%q", delLCMiss.Code, delLCMiss.Body.String())
	}
	emptyName := mustASGQuery(t, handler, strings.Join([]string{
		"Action=DeleteLaunchConfiguration",
		"Version=2011-01-01",
		"LaunchConfigurationName=",
	}, "&"), now)
	if emptyName.Code != 400 {
		t.Fatalf("DeleteLaunchConfiguration empty status=%d body=%q", emptyName.Code, emptyName.Body.String())
	}
}
