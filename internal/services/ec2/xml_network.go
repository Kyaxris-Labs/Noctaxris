package ec2

import (
	"encoding/xml"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

type vpcXML struct {
	VpcID     string `xml:"vpcId"`
	State     string `xml:"state"`
	CidrBlock string `xml:"cidrBlock"`
}

type subnetXML struct {
	SubnetID         string `xml:"subnetId"`
	State            string `xml:"state"`
	VpcID            string `xml:"vpcId"`
	CidrBlock        string `xml:"cidrBlock"`
	AvailabilityZone string `xml:"availabilityZone"`
}

type ipRangeXML struct {
	CidrIP string `xml:"cidrIp"`
}

type ipPermissionXML struct {
	IpProtocol string `xml:"ipProtocol"`
	FromPort   *int   `xml:"fromPort,omitempty"`
	ToPort     *int   `xml:"toPort,omitempty"`
	IpRanges   struct {
		Item []ipRangeXML `xml:"item"`
	} `xml:"ipRanges"`
}

type securityGroupXML struct {
	OwnerID          string `xml:"ownerId"`
	GroupID          string `xml:"groupId"`
	GroupName        string `xml:"groupName"`
	GroupDescription string `xml:"groupDescription"`
	VpcID            string `xml:"vpcId,omitempty"`
	IpPermissions    struct {
		Item []ipPermissionXML `xml:"item"`
	} `xml:"ipPermissions"`
	IpPermissionsEgress struct {
		Item []ipPermissionXML `xml:"item"`
	} `xml:"ipPermissionsEgress"`
}

type networkInterfaceXML struct {
	NetworkInterfaceID string `xml:"networkInterfaceId"`
	SubnetID           string `xml:"subnetId,omitempty"`
	VpcID              string `xml:"vpcId,omitempty"`
	Description        string `xml:"description,omitempty"`
	PrivateIPAddress   string `xml:"privateIpAddress,omitempty"`
	Status             string `xml:"status"`
	AvailabilityZone   string `xml:"availabilityZone,omitempty"`
	Attachment         *struct {
		InstanceID string `xml:"instanceId"`
		Status     string `xml:"status"`
	} `xml:"attachment,omitempty"`
}

func toIpPermissionXML(p store.EC2IpPermission) ipPermissionXML {
	out := ipPermissionXML{IpProtocol: p.IpProtocol}
	if p.IpProtocol != "-1" {
		fp, tp := p.FromPort, p.ToPort
		out.FromPort = &fp
		out.ToPort = &tp
	} else if p.FromPort >= 0 {
		fp, tp := p.FromPort, p.ToPort
		out.FromPort = &fp
		out.ToPort = &tp
	}
	for _, r := range p.IpRanges {
		out.IpRanges.Item = append(out.IpRanges.Item, ipRangeXML{CidrIP: r.CidrIP})
	}
	return out
}

func toSecurityGroupXML(sg store.EC2SecurityGroup) securityGroupXML {
	out := securityGroupXML{
		OwnerID: sg.OwnerID, GroupID: sg.GroupID, GroupName: sg.GroupName,
		GroupDescription: sg.Description, VpcID: sg.VpcID,
	}
	for _, p := range sg.IpPermissions {
		out.IpPermissions.Item = append(out.IpPermissions.Item, toIpPermissionXML(p))
	}
	for _, p := range sg.IpPermissionsEgress {
		out.IpPermissionsEgress.Item = append(out.IpPermissionsEgress.Item, toIpPermissionXML(p))
	}
	return out
}

func toNetworkInterfaceXML(n store.EC2NetworkInterface) networkInterfaceXML {
	out := networkInterfaceXML{
		NetworkInterfaceID: n.NetworkInterfaceID,
		SubnetID:           n.SubnetID,
		VpcID:              n.VpcID,
		Description:        n.Description,
		PrivateIPAddress:   n.PrivateIPAddress,
		Status:             n.Status,
		AvailabilityZone:   n.AvailabilityZone,
	}
	if n.AttachmentInstanceID != "" {
		out.Attachment = &struct {
			InstanceID string `xml:"instanceId"`
			Status     string `xml:"status"`
		}{InstanceID: n.AttachmentInstanceID, Status: "attached"}
	}
	return out
}

// CreateVpcXML builds a CreateVpc Query response.
func CreateVpcXML(vpc store.EC2Vpc, requestID string) ([]byte, error) {
	type response struct {
		XMLName   xml.Name `xml:"CreateVpcResponse"`
		XMLNS     string   `xml:"xmlns,attr"`
		RequestID string   `xml:"requestId"`
		Vpc       vpcXML   `xml:"vpc"`
	}
	return xml.Marshal(response{
		XMLNS: ec2XMLNS, RequestID: requestID,
		Vpc: vpcXML{VpcID: vpc.VpcID, State: vpc.State, CidrBlock: vpc.CidrBlock},
	})
}

// DescribeVpcsXML builds a DescribeVpcs Query response.
func DescribeVpcsXML(vpcs []store.EC2Vpc, requestID string) ([]byte, error) {
	type response struct {
		XMLName   xml.Name `xml:"DescribeVpcsResponse"`
		XMLNS     string   `xml:"xmlns,attr"`
		RequestID string   `xml:"requestId"`
		VpcSet    struct {
			Item []vpcXML `xml:"item"`
		} `xml:"vpcSet"`
	}
	r := response{XMLNS: ec2XMLNS, RequestID: requestID}
	for _, v := range vpcs {
		r.VpcSet.Item = append(r.VpcSet.Item, vpcXML{VpcID: v.VpcID, State: v.State, CidrBlock: v.CidrBlock})
	}
	return xml.Marshal(r)
}

// DeleteVpcXML builds a DeleteVpc Query response.
func DeleteVpcXML(requestID string) ([]byte, error) {
	type response struct {
		XMLName   xml.Name `xml:"DeleteVpcResponse"`
		XMLNS     string   `xml:"xmlns,attr"`
		RequestID string   `xml:"requestId"`
		Return    bool     `xml:"return"`
	}
	return xml.Marshal(response{XMLNS: ec2XMLNS, RequestID: requestID, Return: true})
}

// CreateSubnetXML builds a CreateSubnet Query response.
func CreateSubnetXML(sn store.EC2Subnet, requestID string) ([]byte, error) {
	type response struct {
		XMLName   xml.Name  `xml:"CreateSubnetResponse"`
		XMLNS     string    `xml:"xmlns,attr"`
		RequestID string    `xml:"requestId"`
		Subnet    subnetXML `xml:"subnet"`
	}
	return xml.Marshal(response{
		XMLNS: ec2XMLNS, RequestID: requestID,
		Subnet: subnetXML{
			SubnetID: sn.SubnetID, State: sn.State, VpcID: sn.VpcID,
			CidrBlock: sn.CidrBlock, AvailabilityZone: sn.AvailabilityZone,
		},
	})
}

// DescribeSubnetsXML builds a DescribeSubnets Query response.
func DescribeSubnetsXML(subnets []store.EC2Subnet, requestID string) ([]byte, error) {
	type response struct {
		XMLName   xml.Name `xml:"DescribeSubnetsResponse"`
		XMLNS     string   `xml:"xmlns,attr"`
		RequestID string   `xml:"requestId"`
		SubnetSet struct {
			Item []subnetXML `xml:"item"`
		} `xml:"subnetSet"`
	}
	r := response{XMLNS: ec2XMLNS, RequestID: requestID}
	for _, sn := range subnets {
		r.SubnetSet.Item = append(r.SubnetSet.Item, subnetXML{
			SubnetID: sn.SubnetID, State: sn.State, VpcID: sn.VpcID,
			CidrBlock: sn.CidrBlock, AvailabilityZone: sn.AvailabilityZone,
		})
	}
	return xml.Marshal(r)
}

// DeleteSubnetXML builds a DeleteSubnet Query response.
func DeleteSubnetXML(requestID string) ([]byte, error) {
	type response struct {
		XMLName   xml.Name `xml:"DeleteSubnetResponse"`
		XMLNS     string   `xml:"xmlns,attr"`
		RequestID string   `xml:"requestId"`
		Return    bool     `xml:"return"`
	}
	return xml.Marshal(response{XMLNS: ec2XMLNS, RequestID: requestID, Return: true})
}

// CreateSecurityGroupXML builds a CreateSecurityGroup Query response.
func CreateSecurityGroupXML(groupID, requestID string) ([]byte, error) {
	type response struct {
		XMLName   xml.Name `xml:"CreateSecurityGroupResponse"`
		XMLNS     string   `xml:"xmlns,attr"`
		RequestID string   `xml:"requestId"`
		GroupID   string   `xml:"groupId"`
	}
	return xml.Marshal(response{XMLNS: ec2XMLNS, RequestID: requestID, GroupID: groupID})
}

// DeleteSecurityGroupXML builds a DeleteSecurityGroup Query response.
func DeleteSecurityGroupXML(requestID string) ([]byte, error) {
	type response struct {
		XMLName   xml.Name `xml:"DeleteSecurityGroupResponse"`
		XMLNS     string   `xml:"xmlns,attr"`
		RequestID string   `xml:"requestId"`
		Return    bool     `xml:"return"`
	}
	return xml.Marshal(response{XMLNS: ec2XMLNS, RequestID: requestID, Return: true})
}

// DescribeSecurityGroupsXML builds a DescribeSecurityGroups Query response.
func DescribeSecurityGroupsXML(groups []store.EC2SecurityGroup, requestID string) ([]byte, error) {
	type response struct {
		XMLName           xml.Name `xml:"DescribeSecurityGroupsResponse"`
		XMLNS             string   `xml:"xmlns,attr"`
		RequestID         string   `xml:"requestId"`
		SecurityGroupInfo struct {
			Item []securityGroupXML `xml:"item"`
		} `xml:"securityGroupInfo"`
	}
	r := response{XMLNS: ec2XMLNS, RequestID: requestID}
	for _, sg := range groups {
		r.SecurityGroupInfo.Item = append(r.SecurityGroupInfo.Item, toSecurityGroupXML(sg))
	}
	return xml.Marshal(r)
}

// ReturnTrueXML builds Authorize/Revoke SecurityGroup* responses with <return>true</return>.
func ReturnTrueXML(action, requestID string) ([]byte, error) {
	type response struct {
		XMLName   xml.Name
		XMLNS     string `xml:"xmlns,attr"`
		RequestID string `xml:"requestId"`
		Return    bool   `xml:"return"`
	}
	return xml.Marshal(response{
		XMLName: xml.Name{Local: action + "Response"},
		XMLNS:   ec2XMLNS, RequestID: requestID, Return: true,
	})
}

// CreateNetworkInterfaceXML builds a CreateNetworkInterface Query response.
func CreateNetworkInterfaceXML(n store.EC2NetworkInterface, requestID string) ([]byte, error) {
	type response struct {
		XMLName          xml.Name            `xml:"CreateNetworkInterfaceResponse"`
		XMLNS            string              `xml:"xmlns,attr"`
		RequestID        string              `xml:"requestId"`
		NetworkInterface networkInterfaceXML `xml:"networkInterface"`
	}
	return xml.Marshal(response{
		XMLNS: ec2XMLNS, RequestID: requestID,
		NetworkInterface: toNetworkInterfaceXML(n),
	})
}

// DescribeNetworkInterfacesXML builds a DescribeNetworkInterfaces Query response.
func DescribeNetworkInterfacesXML(enis []store.EC2NetworkInterface, requestID string) ([]byte, error) {
	type response struct {
		XMLName             xml.Name `xml:"DescribeNetworkInterfacesResponse"`
		XMLNS               string   `xml:"xmlns,attr"`
		RequestID           string   `xml:"requestId"`
		NetworkInterfaceSet struct {
			Item []networkInterfaceXML `xml:"item"`
		} `xml:"networkInterfaceSet"`
	}
	r := response{XMLNS: ec2XMLNS, RequestID: requestID}
	for _, n := range enis {
		r.NetworkInterfaceSet.Item = append(r.NetworkInterfaceSet.Item, toNetworkInterfaceXML(n))
	}
	return xml.Marshal(r)
}
