package store_test

import (
	"path/filepath"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func openEC2NetworkStore(t *testing.T) *store.Store {
	t.Helper()
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
	return st
}

func TestEC2VpcSubnetCRUD(t *testing.T) {
	st := openEC2NetworkStore(t)
	account, region := "000000000001", "us-east-1"

	vpc, err := st.CreateVpc(account, region, "10.0.0.0/16")
	if err != nil {
		t.Fatal(err)
	}
	if vpc.VpcID == "" || vpc.State != "available" || vpc.CidrBlock != "10.0.0.0/16" {
		t.Fatalf("vpc=%+v", vpc)
	}

	sn, err := st.CreateSubnet(account, region, vpc.VpcID, "10.0.1.0/24", "us-east-1b")
	if err != nil {
		t.Fatal(err)
	}
	if sn.SubnetID == "" || sn.VpcID != vpc.VpcID || sn.AvailabilityZone != "us-east-1b" {
		t.Fatalf("subnet=%+v", sn)
	}

	vpcs, err := st.DescribeVpcs(account, region, []string{vpc.VpcID})
	if err != nil || len(vpcs) != 1 {
		t.Fatalf("describe vpcs: %v %+v", err, vpcs)
	}
	sns, err := st.DescribeSubnets(account, region, []string{sn.SubnetID})
	if err != nil || len(sns) != 1 {
		t.Fatalf("describe subnets: %v %+v", err, sns)
	}

	if _, err := st.CreateSubnet(account, region, "vpc-missing", "10.0.2.0/24", ""); err != store.ErrEC2VPCNotFound {
		t.Fatalf("want VPC not found, got %v", err)
	}

	if err := st.DeleteSubnet(account, region, sn.SubnetID); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteVpc(account, region, vpc.VpcID); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteVpc(account, region, vpc.VpcID); err != store.ErrEC2VPCNotFound {
		t.Fatalf("second delete: %v", err)
	}
}

func TestEC2SecurityGroupRulesPersist(t *testing.T) {
	st := openEC2NetworkStore(t)
	account, region := "000000000001", "us-east-1"

	vpc, err := st.CreateVpc(account, region, "10.1.0.0/16")
	if err != nil {
		t.Fatal(err)
	}
	sg, err := st.CreateSecurityGroup(account, region, "web", "web sg", vpc.VpcID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sg.IpPermissionsEgress) != 1 || sg.IpPermissionsEgress[0].IpProtocol != "-1" {
		t.Fatalf("want default egress, got %+v", sg.IpPermissionsEgress)
	}

	rule := store.EC2IpPermission{
		IpProtocol: "tcp", FromPort: 443, ToPort: 443,
		IpRanges: []store.EC2IpRange{{CidrIP: "0.0.0.0/0"}},
	}
	if err := st.AuthorizeSecurityGroupIngress(account, region, sg.GroupID, []store.EC2IpPermission{rule}); err != nil {
		t.Fatal(err)
	}
	list, err := st.DescribeSecurityGroups(account, region, []string{sg.GroupID}, nil)
	if err != nil || len(list) != 1 {
		t.Fatalf("describe: %v %+v", err, list)
	}
	if len(list[0].IpPermissions) != 1 || list[0].IpPermissions[0].FromPort != 443 {
		t.Fatalf("ingress=%+v", list[0].IpPermissions)
	}

	if err := st.RevokeSecurityGroupIngress(account, region, sg.GroupID, []store.EC2IpPermission{rule}); err != nil {
		t.Fatal(err)
	}
	list, err = st.DescribeSecurityGroups(account, region, []string{sg.GroupID}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(list[0].IpPermissions) != 0 {
		t.Fatalf("want empty ingress after revoke, got %+v", list[0].IpPermissions)
	}

	egressRule := store.EC2IpPermission{
		IpProtocol: "tcp", FromPort: 53, ToPort: 53,
		IpRanges: []store.EC2IpRange{{CidrIP: "10.0.0.0/8"}},
	}
	if err := st.AuthorizeSecurityGroupEgress(account, region, sg.GroupID, []store.EC2IpPermission{egressRule}); err != nil {
		t.Fatal(err)
	}
	if err := st.RevokeSecurityGroupEgress(account, region, sg.GroupID, []store.EC2IpPermission{egressRule}); err != nil {
		t.Fatal(err)
	}

	if err := st.DeleteSecurityGroup(account, region, sg.GroupID, ""); err != nil {
		t.Fatal(err)
	}
}

func TestEC2NetworkInterfacesSyntheticAndStub(t *testing.T) {
	st := openEC2NetworkStore(t)
	account, region := "000000000001", "us-east-1"

	vpc, err := st.CreateVpc(account, region, "10.2.0.0/16")
	if err != nil {
		t.Fatal(err)
	}
	sn, err := st.CreateSubnet(account, region, vpc.VpcID, "10.2.1.0/24", "us-east-1a")
	if err != nil {
		t.Fatal(err)
	}
	eni, err := st.CreateNetworkInterface(account, region, sn.SubnetID, "lab eni", "10.2.1.10")
	if err != nil {
		t.Fatal(err)
	}
	if eni.NetworkInterfaceID == "" || eni.PrivateIPAddress != "10.2.1.10" || eni.Status != "available" {
		t.Fatalf("eni=%+v", eni)
	}

	insts, err := st.RunInstances(account, region, store.RunInstancesInput{
		ImageID: "ami-alpine", MinCount: 1, MaxCount: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if insts[0].PrivateIP == "" {
		t.Fatal("want private IP on instance")
	}

	all, err := st.DescribeNetworkInterfaces(account, region, nil)
	if err != nil {
		t.Fatal(err)
	}
	var foundStub, foundSynth bool
	for _, n := range all {
		if n.NetworkInterfaceID == eni.NetworkInterfaceID {
			foundStub = true
		}
		if n.AttachmentInstanceID == insts[0].InstanceID && n.PrivateIPAddress == insts[0].PrivateIP {
			foundSynth = true
			if !n.Synthetic {
				t.Fatalf("instance ENI should be synthetic: %+v", n)
			}
		}
	}
	if !foundStub || !foundSynth {
		t.Fatalf("stub=%v synth=%v all=%+v", foundStub, foundSynth, all)
	}
}
