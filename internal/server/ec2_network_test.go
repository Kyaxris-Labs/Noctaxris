package server_test

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestEC2VpcSubnetSecurityGroupENIQuery(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createVpc := mustEC2Query(t, handler, strings.Join([]string{
		"Action=CreateVpc",
		"Version=2016-11-15",
		"CidrBlock=10.0.0.0/16",
	}, "&"), now)
	if createVpc.Code != http.StatusOK {
		t.Fatalf("CreateVpc status=%d body=%q", createVpc.Code, createVpc.Body.String())
	}
	body := createVpc.Body.String()
	if !strings.Contains(body, "<state>available</state>") || !strings.Contains(body, "10.0.0.0/16") {
		t.Fatalf("CreateVpc body=%q", body)
	}
	vpcRe := regexp.MustCompile(`<vpcId>(vpc-[a-f0-9]+)</vpcId>`)
	vm := vpcRe.FindStringSubmatch(body)
	if len(vm) < 2 {
		t.Fatalf("missing vpcId: %q", body)
	}
	vpcID := vm[1]

	createSubnet := mustEC2Query(t, handler, strings.Join([]string{
		"Action=CreateSubnet",
		"Version=2016-11-15",
		"VpcId=" + vpcID,
		"CidrBlock=10.0.1.0/24",
		"AvailabilityZone=us-east-1a",
	}, "&"), now)
	if createSubnet.Code != http.StatusOK {
		t.Fatalf("CreateSubnet status=%d body=%q", createSubnet.Code, createSubnet.Body.String())
	}
	subnetRe := regexp.MustCompile(`<subnetId>(subnet-[a-f0-9]+)</subnetId>`)
	sm := subnetRe.FindStringSubmatch(createSubnet.Body.String())
	if len(sm) < 2 {
		t.Fatalf("missing subnetId: %q", createSubnet.Body.String())
	}
	subnetID := sm[1]

	descVpcs := mustEC2Query(t, handler,
		"Action=DescribeVpcs&Version=2016-11-15&VpcId.1="+vpcID, now)
	if descVpcs.Code != http.StatusOK || !strings.Contains(descVpcs.Body.String(), vpcID) {
		t.Fatalf("DescribeVpcs status=%d body=%q", descVpcs.Code, descVpcs.Body.String())
	}

	createSG := mustEC2Query(t, handler, strings.Join([]string{
		"Action=CreateSecurityGroup",
		"Version=2016-11-15",
		"GroupName=lab-web",
		"GroupDescription=lab+web",
		"VpcId=" + vpcID,
	}, "&"), now)
	if createSG.Code != http.StatusOK {
		t.Fatalf("CreateSecurityGroup status=%d body=%q", createSG.Code, createSG.Body.String())
	}
	sgRe := regexp.MustCompile(`<groupId>(sg-[a-f0-9]+)</groupId>`)
	gm := sgRe.FindStringSubmatch(createSG.Body.String())
	if len(gm) < 2 {
		t.Fatalf("missing groupId: %q", createSG.Body.String())
	}
	sgID := gm[1]

	authIn := mustEC2Query(t, handler, strings.Join([]string{
		"Action=AuthorizeSecurityGroupIngress",
		"Version=2016-11-15",
		"GroupId=" + sgID,
		"IpPermissions.1.IpProtocol=tcp",
		"IpPermissions.1.FromPort=80",
		"IpPermissions.1.ToPort=80",
		"IpPermissions.1.IpRanges.1.CidrIp=0.0.0.0/0",
	}, "&"), now)
	if authIn.Code != http.StatusOK || !strings.Contains(authIn.Body.String(), "<return>true</return>") {
		t.Fatalf("AuthorizeIngress status=%d body=%q", authIn.Code, authIn.Body.String())
	}

	descSG := mustEC2Query(t, handler,
		"Action=DescribeSecurityGroups&Version=2016-11-15&GroupId.1="+sgID, now)
	if descSG.Code != http.StatusOK {
		t.Fatalf("DescribeSecurityGroups status=%d body=%q", descSG.Code, descSG.Body.String())
	}
	sgBody := descSG.Body.String()
	if !strings.Contains(sgBody, "<fromPort>80</fromPort>") || !strings.Contains(sgBody, "0.0.0.0/0") {
		t.Fatalf("want persisted ingress: %q", sgBody)
	}

	revokeIn := mustEC2Query(t, handler, strings.Join([]string{
		"Action=RevokeSecurityGroupIngress",
		"Version=2016-11-15",
		"GroupId=" + sgID,
		"IpPermissions.1.IpProtocol=tcp",
		"IpPermissions.1.FromPort=80",
		"IpPermissions.1.ToPort=80",
		"IpPermissions.1.IpRanges.1.CidrIp=0.0.0.0/0",
	}, "&"), now)
	if revokeIn.Code != http.StatusOK {
		t.Fatalf("RevokeIngress status=%d body=%q", revokeIn.Code, revokeIn.Body.String())
	}

	createENI := mustEC2Query(t, handler, strings.Join([]string{
		"Action=CreateNetworkInterface",
		"Version=2016-11-15",
		"SubnetId=" + subnetID,
		"Description=lab-eni",
		"PrivateIpAddress=10.0.1.20",
	}, "&"), now)
	if createENI.Code != http.StatusOK {
		t.Fatalf("CreateNetworkInterface status=%d body=%q", createENI.Code, createENI.Body.String())
	}
	if !strings.Contains(createENI.Body.String(), "eni-") ||
		!strings.Contains(createENI.Body.String(), "10.0.1.20") {
		t.Fatalf("CreateNetworkInterface body=%q", createENI.Body.String())
	}

	run := mustEC2Query(t, handler, strings.Join([]string{
		"Action=RunInstances",
		"Version=2016-11-15",
		"ImageId=ami-alpine",
		"MinCount=1",
		"MaxCount=1",
	}, "&"), now)
	if run.Code != http.StatusOK {
		t.Fatalf("RunInstances status=%d body=%q", run.Code, run.Body.String())
	}

	descENI := mustEC2Query(t, handler, "Action=DescribeNetworkInterfaces&Version=2016-11-15", now)
	if descENI.Code != http.StatusOK {
		t.Fatalf("DescribeNetworkInterfaces status=%d body=%q", descENI.Code, descENI.Body.String())
	}
	eniBody := descENI.Body.String()
	if !strings.Contains(eniBody, "<networkInterfaceSet>") || !strings.Contains(eniBody, "eni-") {
		t.Fatalf("want ENIs: %q", eniBody)
	}
	if !strings.Contains(eniBody, "<instanceId>i-") {
		t.Fatalf("want synthetic attachment to instance: %q", eniBody)
	}

	delSG := mustEC2Query(t, handler,
		"Action=DeleteSecurityGroup&Version=2016-11-15&GroupId="+sgID, now)
	if delSG.Code != http.StatusOK {
		t.Fatalf("DeleteSecurityGroup status=%d body=%q", delSG.Code, delSG.Body.String())
	}
	delSubnet := mustEC2Query(t, handler,
		"Action=DeleteSubnet&Version=2016-11-15&SubnetId="+subnetID, now)
	if delSubnet.Code != http.StatusOK {
		t.Fatalf("DeleteSubnet status=%d body=%q", delSubnet.Code, delSubnet.Body.String())
	}
	delVpc := mustEC2Query(t, handler,
		"Action=DeleteVpc&Version=2016-11-15&VpcId="+vpcID, now)
	if delVpc.Code != http.StatusOK {
		t.Fatalf("DeleteVpc status=%d body=%q", delVpc.Code, delVpc.Body.String())
	}
}
