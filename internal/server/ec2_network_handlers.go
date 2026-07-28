package server

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	ec2svc "github.com/Kyaxris-Labs/Noctaxris/internal/services/ec2"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func (s *Server) handleEC2Network(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) bool {
	switch action {
	case catalog.ActionEC2CreateVpc:
		s.ec2CreateVpc(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionEC2DeleteVpc:
		s.ec2DeleteVpc(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionEC2DescribeVpcs:
		s.ec2DescribeVpcs(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionEC2CreateSubnet:
		s.ec2CreateSubnet(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionEC2DeleteSubnet:
		s.ec2DeleteSubnet(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionEC2DescribeSubnets:
		s.ec2DescribeSubnets(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionEC2CreateSecurityGroup:
		s.ec2CreateSecurityGroup(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionEC2DeleteSecurityGroup:
		s.ec2DeleteSecurityGroup(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionEC2DescribeSecurityGroups:
		s.ec2DescribeSecurityGroups(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionEC2AuthorizeSecurityGroupIngress:
		s.ec2AuthorizeSG(w, r, body, requestID, eventID, verified, readOnly, true, true)
	case catalog.ActionEC2AuthorizeSecurityGroupEgress:
		s.ec2AuthorizeSG(w, r, body, requestID, eventID, verified, readOnly, false, true)
	case catalog.ActionEC2RevokeSecurityGroupIngress:
		s.ec2AuthorizeSG(w, r, body, requestID, eventID, verified, readOnly, true, false)
	case catalog.ActionEC2RevokeSecurityGroupEgress:
		s.ec2AuthorizeSG(w, r, body, requestID, eventID, verified, readOnly, false, false)
	case catalog.ActionEC2DescribeNetworkInterfaces:
		s.ec2DescribeNetworkInterfaces(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionEC2CreateNetworkInterface:
		s.ec2CreateNetworkInterface(w, r, body, requestID, eventID, verified, readOnly)
	default:
		return false
	}
	return true
}

func (s *Server) ec2CreateVpc(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool,
) {
	if !s.authorize(verified, catalog.ActionEC2CreateVpc, "*") {
		s.writeEC2Error(w, r, requestID, http.StatusForbidden, "UnauthorizedOperation",
			"User is not authorized to perform ec2:CreateVpc.", readOnly, eventID, verified)
		return
	}
	params := formParams(r, body)
	vpc, err := s.store.CreateVpc(verified.AccountID, s.ec2Region(verified), params.Get("CidrBlock"))
	if errors.Is(err, store.ErrEC2BadRequest) {
		s.writeEC2Error(w, r, requestID, http.StatusBadRequest, "InvalidParameterValue",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeEC2Error(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create VPC.", readOnly, eventID, verified)
		return
	}
	payload, _ := ec2svc.CreateVpcXML(vpc, requestID)
	s.writeEC2OK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ec2EventSource, "CreateVpc", readOnly)
}

func (s *Server) ec2DeleteVpc(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool,
) {
	if !s.authorize(verified, catalog.ActionEC2DeleteVpc, "*") {
		s.writeEC2Error(w, r, requestID, http.StatusForbidden, "UnauthorizedOperation",
			"User is not authorized to perform ec2:DeleteVpc.", readOnly, eventID, verified)
		return
	}
	params := formParams(r, body)
	err := s.store.DeleteVpc(verified.AccountID, s.ec2Region(verified), params.Get("VpcId"))
	if errors.Is(err, store.ErrEC2VPCNotFound) {
		s.writeEC2Error(w, r, requestID, http.StatusBadRequest, "InvalidVpcID.NotFound",
			"The vpc ID does not exist", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrEC2BadRequest) {
		s.writeEC2Error(w, r, requestID, http.StatusBadRequest, "MissingParameter",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeEC2Error(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete VPC.", readOnly, eventID, verified)
		return
	}
	payload, _ := ec2svc.DeleteVpcXML(requestID)
	s.writeEC2OK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ec2EventSource, "DeleteVpc", readOnly)
}

func (s *Server) ec2DescribeVpcs(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool,
) {
	if !s.authorize(verified, catalog.ActionEC2DescribeVpcs, "*") {
		s.writeEC2Error(w, r, requestID, http.StatusForbidden, "UnauthorizedOperation",
			"User is not authorized to perform ec2:DescribeVpcs.", readOnly, eventID, verified)
		return
	}
	params := formParams(r, body)
	list, err := s.store.DescribeVpcs(verified.AccountID, s.ec2Region(verified), formMemberList(params, "VpcId"))
	if err != nil {
		s.writeEC2Error(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe VPCs.", readOnly, eventID, verified)
		return
	}
	payload, _ := ec2svc.DescribeVpcsXML(list, requestID)
	s.writeEC2OK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ec2EventSource, "DescribeVpcs", true)
}

func (s *Server) ec2CreateSubnet(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool,
) {
	if !s.authorize(verified, catalog.ActionEC2CreateSubnet, "*") {
		s.writeEC2Error(w, r, requestID, http.StatusForbidden, "UnauthorizedOperation",
			"User is not authorized to perform ec2:CreateSubnet.", readOnly, eventID, verified)
		return
	}
	params := formParams(r, body)
	sn, err := s.store.CreateSubnet(
		verified.AccountID, s.ec2Region(verified),
		params.Get("VpcId"), params.Get("CidrBlock"), params.Get("AvailabilityZone"),
	)
	if errors.Is(err, store.ErrEC2VPCNotFound) {
		s.writeEC2Error(w, r, requestID, http.StatusBadRequest, "InvalidVpcID.NotFound",
			"The vpc ID does not exist", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrEC2BadRequest) {
		s.writeEC2Error(w, r, requestID, http.StatusBadRequest, "InvalidParameterValue",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeEC2Error(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create subnet.", readOnly, eventID, verified)
		return
	}
	payload, _ := ec2svc.CreateSubnetXML(sn, requestID)
	s.writeEC2OK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ec2EventSource, "CreateSubnet", readOnly)
}

func (s *Server) ec2DeleteSubnet(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool,
) {
	if !s.authorize(verified, catalog.ActionEC2DeleteSubnet, "*") {
		s.writeEC2Error(w, r, requestID, http.StatusForbidden, "UnauthorizedOperation",
			"User is not authorized to perform ec2:DeleteSubnet.", readOnly, eventID, verified)
		return
	}
	params := formParams(r, body)
	err := s.store.DeleteSubnet(verified.AccountID, s.ec2Region(verified), params.Get("SubnetId"))
	if errors.Is(err, store.ErrEC2SubnetNotFound) {
		s.writeEC2Error(w, r, requestID, http.StatusBadRequest, "InvalidSubnetID.NotFound",
			"The subnet ID does not exist", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrEC2BadRequest) {
		s.writeEC2Error(w, r, requestID, http.StatusBadRequest, "MissingParameter",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeEC2Error(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete subnet.", readOnly, eventID, verified)
		return
	}
	payload, _ := ec2svc.DeleteSubnetXML(requestID)
	s.writeEC2OK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ec2EventSource, "DeleteSubnet", readOnly)
}

func (s *Server) ec2DescribeSubnets(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool,
) {
	if !s.authorize(verified, catalog.ActionEC2DescribeSubnets, "*") {
		s.writeEC2Error(w, r, requestID, http.StatusForbidden, "UnauthorizedOperation",
			"User is not authorized to perform ec2:DescribeSubnets.", readOnly, eventID, verified)
		return
	}
	params := formParams(r, body)
	list, err := s.store.DescribeSubnets(verified.AccountID, s.ec2Region(verified), formMemberList(params, "SubnetId"))
	if err != nil {
		s.writeEC2Error(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe subnets.", readOnly, eventID, verified)
		return
	}
	payload, _ := ec2svc.DescribeSubnetsXML(list, requestID)
	s.writeEC2OK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ec2EventSource, "DescribeSubnets", true)
}

func (s *Server) ec2CreateSecurityGroup(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool,
) {
	if !s.authorize(verified, catalog.ActionEC2CreateSecurityGroup, "*") {
		s.writeEC2Error(w, r, requestID, http.StatusForbidden, "UnauthorizedOperation",
			"User is not authorized to perform ec2:CreateSecurityGroup.", readOnly, eventID, verified)
		return
	}
	params := formParams(r, body)
	sg, err := s.store.CreateSecurityGroup(
		verified.AccountID, s.ec2Region(verified),
		params.Get("GroupName"), params.Get("GroupDescription"), params.Get("VpcId"),
	)
	if errors.Is(err, store.ErrEC2VPCNotFound) {
		s.writeEC2Error(w, r, requestID, http.StatusBadRequest, "InvalidVpcID.NotFound",
			"The vpc ID does not exist", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrEC2BadRequest) || errors.Is(err, store.ErrEC2NetworkConflict) {
		s.writeEC2Error(w, r, requestID, http.StatusBadRequest, "InvalidParameterValue",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeEC2Error(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create security group.", readOnly, eventID, verified)
		return
	}
	payload, _ := ec2svc.CreateSecurityGroupXML(sg.GroupID, requestID)
	s.writeEC2OK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ec2EventSource, "CreateSecurityGroup", readOnly)
}

func (s *Server) ec2DeleteSecurityGroup(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool,
) {
	if !s.authorize(verified, catalog.ActionEC2DeleteSecurityGroup, "*") {
		s.writeEC2Error(w, r, requestID, http.StatusForbidden, "UnauthorizedOperation",
			"User is not authorized to perform ec2:DeleteSecurityGroup.", readOnly, eventID, verified)
		return
	}
	params := formParams(r, body)
	err := s.store.DeleteSecurityGroup(
		verified.AccountID, s.ec2Region(verified), params.Get("GroupId"), params.Get("GroupName"),
	)
	if errors.Is(err, store.ErrEC2SGNotFound) {
		s.writeEC2Error(w, r, requestID, http.StatusBadRequest, "InvalidGroup.NotFound",
			"The security group does not exist", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrEC2BadRequest) {
		s.writeEC2Error(w, r, requestID, http.StatusBadRequest, "MissingParameter",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeEC2Error(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete security group.", readOnly, eventID, verified)
		return
	}
	payload, _ := ec2svc.DeleteSecurityGroupXML(requestID)
	s.writeEC2OK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ec2EventSource, "DeleteSecurityGroup", readOnly)
}

func (s *Server) ec2DescribeSecurityGroups(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool,
) {
	if !s.authorize(verified, catalog.ActionEC2DescribeSecurityGroups, "*") {
		s.writeEC2Error(w, r, requestID, http.StatusForbidden, "UnauthorizedOperation",
			"User is not authorized to perform ec2:DescribeSecurityGroups.", readOnly, eventID, verified)
		return
	}
	params := formParams(r, body)
	list, err := s.store.DescribeSecurityGroups(
		verified.AccountID, s.ec2Region(verified),
		formMemberList(params, "GroupId"), formMemberList(params, "GroupName"),
	)
	if err != nil {
		s.writeEC2Error(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe security groups.", readOnly, eventID, verified)
		return
	}
	payload, _ := ec2svc.DescribeSecurityGroupsXML(list, requestID)
	s.writeEC2OK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ec2EventSource, "DescribeSecurityGroups", true)
}

func (s *Server) ec2AuthorizeSG(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, ingress, authorize bool,
) {
	var action, short string
	switch {
	case ingress && authorize:
		action, short = catalog.ActionEC2AuthorizeSecurityGroupIngress, "AuthorizeSecurityGroupIngress"
	case !ingress && authorize:
		action, short = catalog.ActionEC2AuthorizeSecurityGroupEgress, "AuthorizeSecurityGroupEgress"
	case ingress && !authorize:
		action, short = catalog.ActionEC2RevokeSecurityGroupIngress, "RevokeSecurityGroupIngress"
	default:
		action, short = catalog.ActionEC2RevokeSecurityGroupEgress, "RevokeSecurityGroupEgress"
	}
	if !s.authorize(verified, action, "*") {
		s.writeEC2Error(w, r, requestID, http.StatusForbidden, "UnauthorizedOperation",
			"User is not authorized to perform "+action+".", readOnly, eventID, verified)
		return
	}
	params := formParams(r, body)
	groupID := params.Get("GroupId")
	rules := parseEC2IpPermissions(params)
	var err error
	switch {
	case ingress && authorize:
		err = s.store.AuthorizeSecurityGroupIngress(verified.AccountID, s.ec2Region(verified), groupID, rules)
	case !ingress && authorize:
		err = s.store.AuthorizeSecurityGroupEgress(verified.AccountID, s.ec2Region(verified), groupID, rules)
	case ingress && !authorize:
		err = s.store.RevokeSecurityGroupIngress(verified.AccountID, s.ec2Region(verified), groupID, rules)
	default:
		err = s.store.RevokeSecurityGroupEgress(verified.AccountID, s.ec2Region(verified), groupID, rules)
	}
	if errors.Is(err, store.ErrEC2SGNotFound) {
		s.writeEC2Error(w, r, requestID, http.StatusBadRequest, "InvalidGroup.NotFound",
			"The security group does not exist", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrEC2BadRequest) {
		s.writeEC2Error(w, r, requestID, http.StatusBadRequest, "InvalidParameterValue",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeEC2Error(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to update security group rules.", readOnly, eventID, verified)
		return
	}
	payload, _ := ec2svc.ReturnTrueXML(short, requestID)
	s.writeEC2OK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ec2EventSource, short, readOnly)
}

func (s *Server) ec2DescribeNetworkInterfaces(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool,
) {
	if !s.authorize(verified, catalog.ActionEC2DescribeNetworkInterfaces, "*") {
		s.writeEC2Error(w, r, requestID, http.StatusForbidden, "UnauthorizedOperation",
			"User is not authorized to perform ec2:DescribeNetworkInterfaces.", readOnly, eventID, verified)
		return
	}
	params := formParams(r, body)
	list, err := s.store.DescribeNetworkInterfaces(
		verified.AccountID, s.ec2Region(verified), formMemberList(params, "NetworkInterfaceId"),
	)
	if err != nil {
		s.writeEC2Error(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe network interfaces.", readOnly, eventID, verified)
		return
	}
	payload, _ := ec2svc.DescribeNetworkInterfacesXML(list, requestID)
	s.writeEC2OK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ec2EventSource, "DescribeNetworkInterfaces", true)
}

func (s *Server) ec2CreateNetworkInterface(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool,
) {
	if !s.authorize(verified, catalog.ActionEC2CreateNetworkInterface, "*") {
		s.writeEC2Error(w, r, requestID, http.StatusForbidden, "UnauthorizedOperation",
			"User is not authorized to perform ec2:CreateNetworkInterface.", readOnly, eventID, verified)
		return
	}
	params := formParams(r, body)
	eni, err := s.store.CreateNetworkInterface(
		verified.AccountID, s.ec2Region(verified),
		params.Get("SubnetId"), params.Get("Description"), params.Get("PrivateIpAddress"),
	)
	if errors.Is(err, store.ErrEC2SubnetNotFound) {
		s.writeEC2Error(w, r, requestID, http.StatusBadRequest, "InvalidSubnetID.NotFound",
			"The subnet ID does not exist", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrEC2BadRequest) {
		s.writeEC2Error(w, r, requestID, http.StatusBadRequest, "InvalidParameterValue",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeEC2Error(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create network interface.", readOnly, eventID, verified)
		return
	}
	payload, _ := ec2svc.CreateNetworkInterfaceXML(eni, requestID)
	s.writeEC2OK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ec2EventSource, "CreateNetworkInterface", readOnly)
}

// parseEC2IpPermissions reads IpPermissions.N.* or flat IpProtocol/FromPort/ToPort/CidrIp.
func parseEC2IpPermissions(params url.Values) []store.EC2IpPermission {
	var out []store.EC2IpPermission
	for i := 1; ; i++ {
		prefix := "IpPermissions." + strconv.Itoa(i) + "."
		proto := strings.TrimSpace(params.Get(prefix + "IpProtocol"))
		if proto == "" {
			break
		}
		fromPort, _ := strconv.Atoi(params.Get(prefix + "FromPort"))
		toPort, _ := strconv.Atoi(params.Get(prefix + "ToPort"))
		if params.Get(prefix+"FromPort") == "" && proto == "-1" {
			fromPort, toPort = -1, -1
		}
		var ranges []store.EC2IpRange
		for j := 1; ; j++ {
			cidr := strings.TrimSpace(params.Get(prefix + "IpRanges." + strconv.Itoa(j) + ".CidrIp"))
			if cidr == "" {
				break
			}
			ranges = append(ranges, store.EC2IpRange{CidrIP: cidr})
		}
		if len(ranges) == 0 {
			if cidr := strings.TrimSpace(params.Get(prefix + "CidrIp")); cidr != "" {
				ranges = append(ranges, store.EC2IpRange{CidrIP: cidr})
			}
		}
		out = append(out, store.EC2IpPermission{
			IpProtocol: proto, FromPort: fromPort, ToPort: toPort, IpRanges: ranges,
		})
	}
	if len(out) > 0 {
		return out
	}
	proto := strings.TrimSpace(params.Get("IpProtocol"))
	if proto == "" {
		return nil
	}
	fromPort, _ := strconv.Atoi(params.Get("FromPort"))
	toPort, _ := strconv.Atoi(params.Get("ToPort"))
	if params.Get("FromPort") == "" && proto == "-1" {
		fromPort, toPort = -1, -1
	}
	var ranges []store.EC2IpRange
	if cidr := strings.TrimSpace(params.Get("CidrIp")); cidr != "" {
		ranges = append(ranges, store.EC2IpRange{CidrIP: cidr})
	}
	return []store.EC2IpPermission{{
		IpProtocol: proto, FromPort: fromPort, ToPort: toPort, IpRanges: ranges,
	}}
}
