package ec2_test

import (
	"encoding/json"
	"strings"
	"testing"

	ec2svc "github.com/Kyaxris-Labs/Noctaxris/internal/services/ec2"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestEC2FlowLogsJSON(t *testing.T) {
	createRaw, err := ec2svc.CreateFlowLogsJSON([]string{"fl-1"}, []map[string]any{{"Error": "x"}})
	if err != nil {
		t.Fatal(err)
	}
	nilRaw, err := ec2svc.CreateFlowLogsJSON(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	injectRaw, err := ec2svc.InjectFlowLogsJSON(5, "fl-1")
	if err != nil {
		t.Fatal(err)
	}
	var inject map[string]any
	if err := json.Unmarshal(injectRaw, &inject); err != nil {
		t.Fatal(err)
	}
	if inject["DeliveredRecords"] != float64(5) {
		t.Fatalf("inject=%v", inject)
	}
	if !strings.Contains(string(createRaw), "fl-1") || !strings.Contains(string(nilRaw), "FlowLogIds") {
		t.Fatalf("create=%s nil=%s", createRaw, nilRaw)
	}
}

func TestEC2InstanceAndImageXML(t *testing.T) {
	inst := store.EC2Instance{
		InstanceID:       "i-abc",
		ImageID:          "ami-1",
		InstanceType:     "t3.micro",
		StateCode:        16,
		StateName:        "running",
		PrivateIP:        "10.0.0.5",
		PublicIP:         "203.0.113.1",
		KeyName:          "lab",
		CreatedAt:        1_700_000_000_000,
		AvailabilityZone: "us-east-1a",
	}
	runRaw, err := ec2svc.RunInstancesXML([]store.EC2Instance{inst}, "req-1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(runRaw), "i-abc") || !strings.Contains(string(runRaw), "RunInstancesResponse") {
		t.Fatalf("run=%s", runRaw)
	}

	img := store.EC2AMI{
		ImageID:            "ami-1",
		OwnerID:            "000000000001",
		State:              "available",
		Public:             true,
		Architecture:       "x86_64",
		ImageType:          "machine",
		Name:               "lab-ami",
		Description:        "desc",
		RootDeviceType:     "ebs",
		RootDeviceName:     "/dev/xvda",
		VirtualizationType: "hvm",
		Hypervisor:         "xen",
		ImageOwnerAlias:    "amazon",
		CreationDate:       "2024-01-01T00:00:00Z",
	}
	descImg, err := ec2svc.DescribeImagesXML([]store.EC2AMI{img}, "req-2")
	if err != nil {
		t.Fatal(err)
	}
	descInst, err := ec2svc.DescribeInstancesXML([]store.EC2Instance{inst}, "req-3")
	if err != nil {
		t.Fatal(err)
	}
	emptyDesc, err := ec2svc.DescribeInstancesXML(nil, "req-4")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(descImg), "lab-ami") || !strings.Contains(string(descInst), "DescribeInstancesResponse") {
		t.Fatalf("img=%s inst=%s", descImg, descInst)
	}
	if !strings.Contains(string(emptyDesc), "DescribeInstancesResponse") {
		t.Fatalf("empty=%s", emptyDesc)
	}

	chg := []ec2svc.StateChange{{
		InstanceID:   "i-abc",
		PreviousName: "running",
		PreviousCode: 16,
		CurrentName:  "stopped",
		CurrentCode:  80,
	}}
	term, err := ec2svc.TerminateInstancesXML(chg, "req-5")
	if err != nil {
		t.Fatal(err)
	}
	stop, err := ec2svc.StopInstancesXML(chg, "req-6")
	if err != nil {
		t.Fatal(err)
	}
	start, err := ec2svc.StartInstancesXML(chg, "req-7")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(term), "TerminateInstancesResponse") ||
		!strings.Contains(string(stop), "StopInstancesResponse") ||
		!strings.Contains(string(start), "StartInstancesResponse") {
		t.Fatalf("state changes term=%s stop=%s start=%s", term, stop, start)
	}

	errXML := ec2svc.ErrorXML("InvalidInstanceID.NotFound", "not found", "req-err")
	if len(errXML) == 0 || !strings.Contains(string(errXML), "Error") {
		t.Fatalf("err=%s", errXML)
	}
}

func TestEC2NetworkXML(t *testing.T) {
	vpc := store.EC2Vpc{VpcID: "vpc-1", State: "available", CidrBlock: "10.0.0.0/16"}
	createVpc, err := ec2svc.CreateVpcXML(vpc, "req-v")
	if err != nil {
		t.Fatal(err)
	}
	descVpc, err := ec2svc.DescribeVpcsXML([]store.EC2Vpc{vpc}, "req-v2")
	if err != nil {
		t.Fatal(err)
	}
	delVpc, err := ec2svc.DeleteVpcXML("req-v3")
	if err != nil {
		t.Fatal(err)
	}

	sn := store.EC2Subnet{
		SubnetID:         "subnet-1",
		State:            "available",
		VpcID:            "vpc-1",
		CidrBlock:        "10.0.1.0/24",
		AvailabilityZone: "us-east-1a",
	}
	createSn, err := ec2svc.CreateSubnetXML(sn, "req-s")
	if err != nil {
		t.Fatal(err)
	}
	descSn, err := ec2svc.DescribeSubnetsXML([]store.EC2Subnet{sn}, "req-s2")
	if err != nil {
		t.Fatal(err)
	}
	delSn, err := ec2svc.DeleteSubnetXML("req-s3")
	if err != nil {
		t.Fatal(err)
	}

	sg := store.EC2SecurityGroup{
		OwnerID:     "000000000001",
		GroupID:     "sg-1",
		GroupName:   "lab",
		Description: "lab sg",
		VpcID:       "vpc-1",
		IpPermissions: []store.EC2IpPermission{{
			IpProtocol: "tcp",
			FromPort:   80,
			ToPort:     80,
			IpRanges:   []store.EC2IpRange{{CidrIP: "0.0.0.0/0"}},
		}},
		IpPermissionsEgress: []store.EC2IpPermission{{
			IpProtocol: "-1",
			FromPort:   -1,
			ToPort:     -1,
			IpRanges:   []store.EC2IpRange{{CidrIP: "0.0.0.0/0"}},
		}},
	}
	createSg, err := ec2svc.CreateSecurityGroupXML("sg-1", "req-g")
	if err != nil {
		t.Fatal(err)
	}
	descSg, err := ec2svc.DescribeSecurityGroupsXML([]store.EC2SecurityGroup{sg}, "req-g2")
	if err != nil {
		t.Fatal(err)
	}
	delSg, err := ec2svc.DeleteSecurityGroupXML("req-g3")
	if err != nil {
		t.Fatal(err)
	}
	auth, err := ec2svc.ReturnTrueXML("AuthorizeSecurityGroupIngress", "req-a")
	if err != nil {
		t.Fatal(err)
	}
	revoke, err := ec2svc.ReturnTrueXML("RevokeSecurityGroupIngress", "req-r")
	if err != nil {
		t.Fatal(err)
	}

	eni := store.EC2NetworkInterface{
		NetworkInterfaceID:   "eni-1",
		SubnetID:             "subnet-1",
		VpcID:                "vpc-1",
		Description:          "lab",
		PrivateIPAddress:     "10.0.1.5",
		Status:               "in-use",
		AvailabilityZone:     "us-east-1a",
		AttachmentInstanceID: "i-abc",
	}
	createEni, err := ec2svc.CreateNetworkInterfaceXML(eni, "req-e")
	if err != nil {
		t.Fatal(err)
	}
	descEni, err := ec2svc.DescribeNetworkInterfacesXML([]store.EC2NetworkInterface{eni}, "req-e2")
	if err != nil {
		t.Fatal(err)
	}

	for name, raw := range map[string][]byte{
		"vpc": createVpc, "descVpc": descVpc, "delVpc": delVpc,
		"sn": createSn, "descSn": descSn, "delSn": delSn,
		"sg": createSg, "descSg": descSg, "delSg": delSg,
		"auth": auth, "revoke": revoke,
		"eni": createEni, "descEni": descEni,
	} {
		if !strings.Contains(string(raw), "Response") {
			t.Fatalf("%s missing response tag: %s", name, raw)
		}
	}
}
