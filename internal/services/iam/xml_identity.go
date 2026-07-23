package iam

import (
	"encoding/xml"
	"fmt"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// --- Groups ---

type groupXML struct {
	Path      string `xml:"Path"`
	GroupName string `xml:"GroupName"`
	GroupId   string `xml:"GroupId"`
	Arn       string `xml:"Arn"`
}

type createGroupResponse struct {
	XMLName           xml.Name `xml:"CreateGroupResponse"`
	XMLNS             string   `xml:"xmlns,attr"`
	CreateGroupResult struct {
		Group groupXML `xml:"Group"`
	} `xml:"CreateGroupResult"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// CreateGroupXML builds CreateGroup response XML.
func CreateGroupXML(g store.Group, requestID string) ([]byte, error) {
	resp := createGroupResponse{XMLNS: iamXMLNS}
	resp.CreateGroupResult.Group = groupXML{Path: "/", GroupName: g.GroupName, GroupId: g.GroupID, Arn: g.ARN}
	resp.ResponseMetadata.RequestId = requestID
	return marshalResponse(resp)
}

type getGroupResponse struct {
	XMLName        xml.Name `xml:"GetGroupResponse"`
	XMLNS          string   `xml:"xmlns,attr"`
	GetGroupResult struct {
		Group       groupXML  `xml:"Group"`
		Users       []userXML `xml:"Users>member"`
		IsTruncated bool      `xml:"IsTruncated"`
	} `xml:"GetGroupResult"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// GetGroupXML builds GetGroup response XML.
func GetGroupXML(g store.Group, users []store.User, requestID string) ([]byte, error) {
	resp := getGroupResponse{XMLNS: iamXMLNS}
	resp.GetGroupResult.Group = groupXML{Path: "/", GroupName: g.GroupName, GroupId: g.GroupID, Arn: g.ARN}
	for _, u := range users {
		resp.GetGroupResult.Users = append(resp.GetGroupResult.Users, userXML{
			Path: "/", UserName: u.UserName, UserId: u.UserID, Arn: u.ARN,
		})
	}
	resp.ResponseMetadata.RequestId = requestID
	return marshalResponse(resp)
}

type listGroupsResponse struct {
	XMLName          xml.Name `xml:"ListGroupsResponse"`
	XMLNS            string   `xml:"xmlns,attr"`
	ListGroupsResult struct {
		Groups      []groupXML `xml:"Groups>member"`
		IsTruncated bool       `xml:"IsTruncated"`
	} `xml:"ListGroupsResult"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// ListGroupsXML builds ListGroups response XML.
func ListGroupsXML(groups []store.Group, requestID string) ([]byte, error) {
	resp := listGroupsResponse{XMLNS: iamXMLNS}
	for _, g := range groups {
		resp.ListGroupsResult.Groups = append(resp.ListGroupsResult.Groups, groupXML{
			Path: "/", GroupName: g.GroupName, GroupId: g.GroupID, Arn: g.ARN,
		})
	}
	resp.ResponseMetadata.RequestId = requestID
	return marshalResponse(resp)
}

func emptyIAMResponse(name, requestID string) ([]byte, error) {
	switch name {
	case "DeleteGroup":
		return marshalResponse(struct {
			XMLName          xml.Name         `xml:"DeleteGroupResponse"`
			XMLNS            string           `xml:"xmlns,attr"`
			ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
		}{XMLNS: iamXMLNS, ResponseMetadata: responseMetadata{RequestId: requestID}})
	case "AddUserToGroup":
		return marshalResponse(struct {
			XMLName          xml.Name         `xml:"AddUserToGroupResponse"`
			XMLNS            string           `xml:"xmlns,attr"`
			ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
		}{XMLNS: iamXMLNS, ResponseMetadata: responseMetadata{RequestId: requestID}})
	case "RemoveUserFromGroup":
		return marshalResponse(struct {
			XMLName          xml.Name         `xml:"RemoveUserFromGroupResponse"`
			XMLNS            string           `xml:"xmlns,attr"`
			ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
		}{XMLNS: iamXMLNS, ResponseMetadata: responseMetadata{RequestId: requestID}})
	case "AttachGroupPolicy":
		return marshalResponse(struct {
			XMLName          xml.Name         `xml:"AttachGroupPolicyResponse"`
			XMLNS            string           `xml:"xmlns,attr"`
			ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
		}{XMLNS: iamXMLNS, ResponseMetadata: responseMetadata{RequestId: requestID}})
	case "DetachGroupPolicy":
		return marshalResponse(struct {
			XMLName          xml.Name         `xml:"DetachGroupPolicyResponse"`
			XMLNS            string           `xml:"xmlns,attr"`
			ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
		}{XMLNS: iamXMLNS, ResponseMetadata: responseMetadata{RequestId: requestID}})
	case "PutGroupPolicy":
		return marshalResponse(struct {
			XMLName          xml.Name         `xml:"PutGroupPolicyResponse"`
			XMLNS            string           `xml:"xmlns,attr"`
			ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
		}{XMLNS: iamXMLNS, ResponseMetadata: responseMetadata{RequestId: requestID}})
	case "DeleteGroupPolicy":
		return marshalResponse(struct {
			XMLName          xml.Name         `xml:"DeleteGroupPolicyResponse"`
			XMLNS            string           `xml:"xmlns,attr"`
			ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
		}{XMLNS: iamXMLNS, ResponseMetadata: responseMetadata{RequestId: requestID}})
	case "PutUserPermissionsBoundary":
		return marshalResponse(struct {
			XMLName          xml.Name         `xml:"PutUserPermissionsBoundaryResponse"`
			XMLNS            string           `xml:"xmlns,attr"`
			ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
		}{XMLNS: iamXMLNS, ResponseMetadata: responseMetadata{RequestId: requestID}})
	case "DeleteUserPermissionsBoundary":
		return marshalResponse(struct {
			XMLName          xml.Name         `xml:"DeleteUserPermissionsBoundaryResponse"`
			XMLNS            string           `xml:"xmlns,attr"`
			ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
		}{XMLNS: iamXMLNS, ResponseMetadata: responseMetadata{RequestId: requestID}})
	case "PutRolePermissionsBoundary":
		return marshalResponse(struct {
			XMLName          xml.Name         `xml:"PutRolePermissionsBoundaryResponse"`
			XMLNS            string           `xml:"xmlns,attr"`
			ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
		}{XMLNS: iamXMLNS, ResponseMetadata: responseMetadata{RequestId: requestID}})
	case "DeleteRolePermissionsBoundary":
		return marshalResponse(struct {
			XMLName          xml.Name         `xml:"DeleteRolePermissionsBoundaryResponse"`
			XMLNS            string           `xml:"xmlns,attr"`
			ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
		}{XMLNS: iamXMLNS, ResponseMetadata: responseMetadata{RequestId: requestID}})
	case "DeleteInstanceProfile":
		return marshalResponse(struct {
			XMLName          xml.Name         `xml:"DeleteInstanceProfileResponse"`
			XMLNS            string           `xml:"xmlns,attr"`
			ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
		}{XMLNS: iamXMLNS, ResponseMetadata: responseMetadata{RequestId: requestID}})
	case "AddRoleToInstanceProfile":
		return marshalResponse(struct {
			XMLName          xml.Name         `xml:"AddRoleToInstanceProfileResponse"`
			XMLNS            string           `xml:"xmlns,attr"`
			ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
		}{XMLNS: iamXMLNS, ResponseMetadata: responseMetadata{RequestId: requestID}})
	case "RemoveRoleFromInstanceProfile":
		return marshalResponse(struct {
			XMLName          xml.Name         `xml:"RemoveRoleFromInstanceProfileResponse"`
			XMLNS            string           `xml:"xmlns,attr"`
			ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
		}{XMLNS: iamXMLNS, ResponseMetadata: responseMetadata{RequestId: requestID}})
	case "DeleteOpenIDConnectProvider":
		return marshalResponse(struct {
			XMLName          xml.Name         `xml:"DeleteOpenIDConnectProviderResponse"`
			XMLNS            string           `xml:"xmlns,attr"`
			ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
		}{XMLNS: iamXMLNS, ResponseMetadata: responseMetadata{RequestId: requestID}})
	case "DeleteSAMLProvider":
		return marshalResponse(struct {
			XMLName          xml.Name         `xml:"DeleteSAMLProviderResponse"`
			XMLNS            string           `xml:"xmlns,attr"`
			ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
		}{XMLNS: iamXMLNS, ResponseMetadata: responseMetadata{RequestId: requestID}})
	case "EnableMFADevice":
		return marshalResponse(struct {
			XMLName          xml.Name         `xml:"EnableMFADeviceResponse"`
			XMLNS            string           `xml:"xmlns,attr"`
			ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
		}{XMLNS: iamXMLNS, ResponseMetadata: responseMetadata{RequestId: requestID}})
	case "DeactivateMFADevice":
		return marshalResponse(struct {
			XMLName          xml.Name         `xml:"DeactivateMFADeviceResponse"`
			XMLNS            string           `xml:"xmlns,attr"`
			ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
		}{XMLNS: iamXMLNS, ResponseMetadata: responseMetadata{RequestId: requestID}})
	default:
		return nil, fmt.Errorf("unknown empty IAM response %q", name)
	}
}

// DeleteGroupXML builds DeleteGroup response XML.
func DeleteGroupXML(requestID string) ([]byte, error) {
	return emptyIAMResponse("DeleteGroup", requestID)
}

// AddUserToGroupXML builds AddUserToGroup response XML.
func AddUserToGroupXML(requestID string) ([]byte, error) {
	return emptyIAMResponse("AddUserToGroup", requestID)
}

// RemoveUserFromGroupXML builds RemoveUserFromGroup response XML.
func RemoveUserFromGroupXML(requestID string) ([]byte, error) {
	return emptyIAMResponse("RemoveUserFromGroup", requestID)
}

// AttachGroupPolicyXML builds AttachGroupPolicy response XML.
func AttachGroupPolicyXML(requestID string) ([]byte, error) {
	return emptyIAMResponse("AttachGroupPolicy", requestID)
}

// DetachGroupPolicyXML builds DetachGroupPolicy response XML.
func DetachGroupPolicyXML(requestID string) ([]byte, error) {
	return emptyIAMResponse("DetachGroupPolicy", requestID)
}

type listAttachedGroupPoliciesResponse struct {
	XMLName                         xml.Name `xml:"ListAttachedGroupPoliciesResponse"`
	XMLNS                           string   `xml:"xmlns,attr"`
	ListAttachedGroupPoliciesResult struct {
		AttachedPolicies []attachedPolicyXML `xml:"AttachedPolicies>member"`
		IsTruncated      bool                `xml:"IsTruncated"`
	} `xml:"ListAttachedGroupPoliciesResult"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// ListAttachedGroupPoliciesXML builds ListAttachedGroupPolicies response XML.
func ListAttachedGroupPoliciesXML(refs []store.AttachedPolicyRef, requestID string) ([]byte, error) {
	resp := listAttachedGroupPoliciesResponse{XMLNS: iamXMLNS}
	for _, r := range refs {
		resp.ListAttachedGroupPoliciesResult.AttachedPolicies = append(
			resp.ListAttachedGroupPoliciesResult.AttachedPolicies,
			attachedPolicyXML{PolicyName: r.PolicyName, PolicyArn: r.PolicyARN},
		)
	}
	resp.ResponseMetadata.RequestId = requestID
	return marshalResponse(resp)
}

// PutGroupPolicyXML builds PutGroupPolicy response XML.
func PutGroupPolicyXML(requestID string) ([]byte, error) {
	return emptyIAMResponse("PutGroupPolicy", requestID)
}

type getGroupPolicyResponse struct {
	XMLName              xml.Name `xml:"GetGroupPolicyResponse"`
	XMLNS                string   `xml:"xmlns,attr"`
	GetGroupPolicyResult struct {
		GroupName      string `xml:"GroupName"`
		PolicyName     string `xml:"PolicyName"`
		PolicyDocument string `xml:"PolicyDocument"`
	} `xml:"GetGroupPolicyResult"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// GetGroupPolicyXML builds GetGroupPolicy response XML.
func GetGroupPolicyXML(groupName, policyName, document, requestID string) ([]byte, error) {
	resp := getGroupPolicyResponse{XMLNS: iamXMLNS}
	resp.GetGroupPolicyResult.GroupName = groupName
	resp.GetGroupPolicyResult.PolicyName = policyName
	resp.GetGroupPolicyResult.PolicyDocument = document
	resp.ResponseMetadata.RequestId = requestID
	return marshalResponse(resp)
}

// DeleteGroupPolicyXML builds DeleteGroupPolicy response XML.
func DeleteGroupPolicyXML(requestID string) ([]byte, error) {
	return emptyIAMResponse("DeleteGroupPolicy", requestID)
}

type listGroupPoliciesResponse struct {
	XMLName                 xml.Name `xml:"ListGroupPoliciesResponse"`
	XMLNS                   string   `xml:"xmlns,attr"`
	ListGroupPoliciesResult struct {
		PolicyNames []string `xml:"PolicyNames>member"`
		IsTruncated bool     `xml:"IsTruncated"`
	} `xml:"ListGroupPoliciesResult"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// ListGroupPoliciesXML builds ListGroupPolicies response XML (inline policy names).
func ListGroupPoliciesXML(names []string, requestID string) ([]byte, error) {
	resp := listGroupPoliciesResponse{XMLNS: iamXMLNS}
	resp.ListGroupPoliciesResult.PolicyNames = names
	resp.ResponseMetadata.RequestId = requestID
	return marshalResponse(resp)
}

// --- Permissions boundaries ---

// PutUserPermissionsBoundaryXML builds PutUserPermissionsBoundary response XML.
func PutUserPermissionsBoundaryXML(requestID string) ([]byte, error) {
	return emptyIAMResponse("PutUserPermissionsBoundary", requestID)
}

type getUserPermissionsBoundaryResponse struct {
	XMLName                         xml.Name `xml:"GetUserPermissionsBoundaryResponse"`
	XMLNS                           string   `xml:"xmlns,attr"`
	GetUserPermissionsBoundaryResult struct {
		PermissionsBoundary struct {
			PermissionsBoundaryType string `xml:"PermissionsBoundaryType"`
			PermissionsBoundaryArn  string `xml:"PermissionsBoundaryArn"`
		} `xml:"PermissionsBoundary"`
	} `xml:"GetUserPermissionsBoundaryResult"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// GetUserPermissionsBoundaryXML builds GetUserPermissionsBoundary response XML.
func GetUserPermissionsBoundaryXML(policyARN, requestID string) ([]byte, error) {
	resp := getUserPermissionsBoundaryResponse{XMLNS: iamXMLNS}
	resp.GetUserPermissionsBoundaryResult.PermissionsBoundary.PermissionsBoundaryType = "Policy"
	resp.GetUserPermissionsBoundaryResult.PermissionsBoundary.PermissionsBoundaryArn = policyARN
	resp.ResponseMetadata.RequestId = requestID
	return marshalResponse(resp)
}

// DeleteUserPermissionsBoundaryXML builds DeleteUserPermissionsBoundary response XML.
func DeleteUserPermissionsBoundaryXML(requestID string) ([]byte, error) {
	return emptyIAMResponse("DeleteUserPermissionsBoundary", requestID)
}

// PutRolePermissionsBoundaryXML builds PutRolePermissionsBoundary response XML.
func PutRolePermissionsBoundaryXML(requestID string) ([]byte, error) {
	return emptyIAMResponse("PutRolePermissionsBoundary", requestID)
}

type getRolePermissionsBoundaryResponse struct {
	XMLName                         xml.Name `xml:"GetRolePermissionsBoundaryResponse"`
	XMLNS                           string   `xml:"xmlns,attr"`
	GetRolePermissionsBoundaryResult struct {
		PermissionsBoundary struct {
			PermissionsBoundaryType string `xml:"PermissionsBoundaryType"`
			PermissionsBoundaryArn  string `xml:"PermissionsBoundaryArn"`
		} `xml:"PermissionsBoundary"`
	} `xml:"GetRolePermissionsBoundaryResult"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// GetRolePermissionsBoundaryXML builds GetRolePermissionsBoundary response XML.
func GetRolePermissionsBoundaryXML(policyARN, requestID string) ([]byte, error) {
	resp := getRolePermissionsBoundaryResponse{XMLNS: iamXMLNS}
	resp.GetRolePermissionsBoundaryResult.PermissionsBoundary.PermissionsBoundaryType = "Policy"
	resp.GetRolePermissionsBoundaryResult.PermissionsBoundary.PermissionsBoundaryArn = policyARN
	resp.ResponseMetadata.RequestId = requestID
	return marshalResponse(resp)
}

// DeleteRolePermissionsBoundaryXML builds DeleteRolePermissionsBoundary response XML.
func DeleteRolePermissionsBoundaryXML(requestID string) ([]byte, error) {
	return emptyIAMResponse("DeleteRolePermissionsBoundary", requestID)
}

// --- Instance profiles ---

type roleSummaryXML struct {
	Path     string `xml:"Path"`
	RoleName string `xml:"RoleName"`
	RoleId   string `xml:"RoleId,omitempty"`
	Arn      string `xml:"Arn,omitempty"`
}

type instanceProfileXML struct {
	Path                string           `xml:"Path"`
	InstanceProfileName string           `xml:"InstanceProfileName"`
	Arn                 string           `xml:"Arn"`
	Roles               []roleSummaryXML `xml:"Roles>member,omitempty"`
}

func instanceProfileToXML(p store.InstanceProfile) instanceProfileXML {
	out := instanceProfileXML{
		Path: "/", InstanceProfileName: p.ProfileName, Arn: p.ProfileARN,
	}
	if p.RoleName != "" {
		out.Roles = []roleSummaryXML{{
			Path: "/", RoleName: p.RoleName, RoleId: p.RoleID, Arn: p.RoleARN,
		}}
	}
	return out
}

type createInstanceProfileResponse struct {
	XMLName                     xml.Name `xml:"CreateInstanceProfileResponse"`
	XMLNS                       string   `xml:"xmlns,attr"`
	CreateInstanceProfileResult struct {
		InstanceProfile instanceProfileXML `xml:"InstanceProfile"`
	} `xml:"CreateInstanceProfileResult"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// CreateInstanceProfileXML builds CreateInstanceProfile response XML.
func CreateInstanceProfileXML(p store.InstanceProfile, requestID string) ([]byte, error) {
	resp := createInstanceProfileResponse{XMLNS: iamXMLNS}
	resp.CreateInstanceProfileResult.InstanceProfile = instanceProfileToXML(p)
	resp.ResponseMetadata.RequestId = requestID
	return marshalResponse(resp)
}

// DeleteInstanceProfileXML builds DeleteInstanceProfile response XML.
func DeleteInstanceProfileXML(requestID string) ([]byte, error) {
	return emptyIAMResponse("DeleteInstanceProfile", requestID)
}

type getInstanceProfileResponse struct {
	XMLName                  xml.Name `xml:"GetInstanceProfileResponse"`
	XMLNS                    string   `xml:"xmlns,attr"`
	GetInstanceProfileResult struct {
		InstanceProfile instanceProfileXML `xml:"InstanceProfile"`
	} `xml:"GetInstanceProfileResult"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// GetInstanceProfileXML builds GetInstanceProfile response XML.
func GetInstanceProfileXML(p store.InstanceProfile, requestID string) ([]byte, error) {
	resp := getInstanceProfileResponse{XMLNS: iamXMLNS}
	resp.GetInstanceProfileResult.InstanceProfile = instanceProfileToXML(p)
	resp.ResponseMetadata.RequestId = requestID
	return marshalResponse(resp)
}

// AddRoleToInstanceProfileXML builds AddRoleToInstanceProfile response XML.
func AddRoleToInstanceProfileXML(requestID string) ([]byte, error) {
	return emptyIAMResponse("AddRoleToInstanceProfile", requestID)
}

// RemoveRoleFromInstanceProfileXML builds RemoveRoleFromInstanceProfile response XML.
func RemoveRoleFromInstanceProfileXML(requestID string) ([]byte, error) {
	return emptyIAMResponse("RemoveRoleFromInstanceProfile", requestID)
}

type listInstanceProfilesResponse struct {
	XMLName                    xml.Name `xml:"ListInstanceProfilesResponse"`
	XMLNS                      string   `xml:"xmlns,attr"`
	ListInstanceProfilesResult struct {
		InstanceProfiles []instanceProfileXML `xml:"InstanceProfiles>member"`
		IsTruncated      bool                 `xml:"IsTruncated"`
	} `xml:"ListInstanceProfilesResult"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// ListInstanceProfilesXML builds ListInstanceProfiles response XML.
func ListInstanceProfilesXML(profiles []store.InstanceProfile, requestID string) ([]byte, error) {
	resp := listInstanceProfilesResponse{XMLNS: iamXMLNS}
	for _, p := range profiles {
		resp.ListInstanceProfilesResult.InstanceProfiles = append(
			resp.ListInstanceProfilesResult.InstanceProfiles, instanceProfileToXML(p),
		)
	}
	resp.ResponseMetadata.RequestId = requestID
	return marshalResponse(resp)
}

type listInstanceProfilesForRoleResponse struct {
	XMLName                           xml.Name `xml:"ListInstanceProfilesForRoleResponse"`
	XMLNS                             string   `xml:"xmlns,attr"`
	ListInstanceProfilesForRoleResult struct {
		InstanceProfiles []instanceProfileXML `xml:"InstanceProfiles>member"`
		IsTruncated      bool                 `xml:"IsTruncated"`
	} `xml:"ListInstanceProfilesForRoleResult"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// ListInstanceProfilesForRoleXML builds ListInstanceProfilesForRole response XML.
func ListInstanceProfilesForRoleXML(profiles []store.InstanceProfile, requestID string) ([]byte, error) {
	resp := listInstanceProfilesForRoleResponse{XMLNS: iamXMLNS}
	for _, p := range profiles {
		resp.ListInstanceProfilesForRoleResult.InstanceProfiles = append(
			resp.ListInstanceProfilesForRoleResult.InstanceProfiles, instanceProfileToXML(p),
		)
	}
	resp.ResponseMetadata.RequestId = requestID
	return marshalResponse(resp)
}

// --- IdP ---

type createOpenIDConnectProviderResponse struct {
	XMLName                           xml.Name `xml:"CreateOpenIDConnectProviderResponse"`
	XMLNS                             string   `xml:"xmlns,attr"`
	CreateOpenIDConnectProviderResult struct {
		OpenIDConnectProviderArn string `xml:"OpenIDConnectProviderArn"`
	} `xml:"CreateOpenIDConnectProviderResult"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// CreateOpenIDConnectProviderXML builds CreateOpenIDConnectProvider response XML.
func CreateOpenIDConnectProviderXML(providerARN, requestID string) ([]byte, error) {
	resp := createOpenIDConnectProviderResponse{XMLNS: iamXMLNS}
	resp.CreateOpenIDConnectProviderResult.OpenIDConnectProviderArn = providerARN
	resp.ResponseMetadata.RequestId = requestID
	return marshalResponse(resp)
}

// DeleteOpenIDConnectProviderXML builds DeleteOpenIDConnectProvider response XML.
func DeleteOpenIDConnectProviderXML(requestID string) ([]byte, error) {
	return emptyIAMResponse("DeleteOpenIDConnectProvider", requestID)
}

type listOpenIDConnectProvidersResponse struct {
	XMLName                          xml.Name `xml:"ListOpenIDConnectProvidersResponse"`
	XMLNS                            string   `xml:"xmlns,attr"`
	ListOpenIDConnectProvidersResult struct {
		OpenIDConnectProviderList []struct {
			Arn string `xml:"Arn"`
		} `xml:"OpenIDConnectProviderList>member"`
	} `xml:"ListOpenIDConnectProvidersResult"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// ListOpenIDConnectProvidersXML builds ListOpenIDConnectProviders response XML.
func ListOpenIDConnectProvidersXML(providers []store.OIDCProvider, requestID string) ([]byte, error) {
	resp := listOpenIDConnectProvidersResponse{XMLNS: iamXMLNS}
	for _, p := range providers {
		resp.ListOpenIDConnectProvidersResult.OpenIDConnectProviderList = append(
			resp.ListOpenIDConnectProvidersResult.OpenIDConnectProviderList,
			struct {
				Arn string `xml:"Arn"`
			}{Arn: p.ProviderARN},
		)
	}
	resp.ResponseMetadata.RequestId = requestID
	return marshalResponse(resp)
}

type getOpenIDConnectProviderResponse struct {
	XMLName                        xml.Name `xml:"GetOpenIDConnectProviderResponse"`
	XMLNS                          string   `xml:"xmlns,attr"`
	GetOpenIDConnectProviderResult struct {
		Url             string   `xml:"Url"`
		ClientIDList    []string `xml:"ClientIDList>member"`
		ThumbprintList  []string `xml:"ThumbprintList>member,omitempty"`
	} `xml:"GetOpenIDConnectProviderResult"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// GetOpenIDConnectProviderXML builds GetOpenIDConnectProvider response XML.
func GetOpenIDConnectProviderXML(p store.OIDCProvider, requestID string) ([]byte, error) {
	resp := getOpenIDConnectProviderResponse{XMLNS: iamXMLNS}
	resp.GetOpenIDConnectProviderResult.Url = p.URL
	resp.GetOpenIDConnectProviderResult.ClientIDList = []string{p.ClientID}
	resp.ResponseMetadata.RequestId = requestID
	return marshalResponse(resp)
}

type createSAMLProviderResponse struct {
	XMLName                  xml.Name `xml:"CreateSAMLProviderResponse"`
	XMLNS                    string   `xml:"xmlns,attr"`
	CreateSAMLProviderResult struct {
		SAMLProviderArn string `xml:"SAMLProviderArn"`
	} `xml:"CreateSAMLProviderResult"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// CreateSAMLProviderXML builds CreateSAMLProvider response XML.
func CreateSAMLProviderXML(providerARN, requestID string) ([]byte, error) {
	resp := createSAMLProviderResponse{XMLNS: iamXMLNS}
	resp.CreateSAMLProviderResult.SAMLProviderArn = providerARN
	resp.ResponseMetadata.RequestId = requestID
	return marshalResponse(resp)
}

// DeleteSAMLProviderXML builds DeleteSAMLProvider response XML.
func DeleteSAMLProviderXML(requestID string) ([]byte, error) {
	return emptyIAMResponse("DeleteSAMLProvider", requestID)
}

type listSAMLProvidersResponse struct {
	XMLName                 xml.Name `xml:"ListSAMLProvidersResponse"`
	XMLNS                   string   `xml:"xmlns,attr"`
	ListSAMLProvidersResult struct {
		SAMLProviderList []struct {
			Arn string `xml:"Arn"`
		} `xml:"SAMLProviderList>member"`
	} `xml:"ListSAMLProvidersResult"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// ListSAMLProvidersXML builds ListSAMLProviders response XML.
func ListSAMLProvidersXML(providers []store.SAMLProvider, requestID string) ([]byte, error) {
	resp := listSAMLProvidersResponse{XMLNS: iamXMLNS}
	for _, p := range providers {
		resp.ListSAMLProvidersResult.SAMLProviderList = append(
			resp.ListSAMLProvidersResult.SAMLProviderList,
			struct {
				Arn string `xml:"Arn"`
			}{Arn: p.ProviderARN},
		)
	}
	resp.ResponseMetadata.RequestId = requestID
	return marshalResponse(resp)
}

type getSAMLProviderResponse struct {
	XMLName               xml.Name `xml:"GetSAMLProviderResponse"`
	XMLNS                 string   `xml:"xmlns,attr"`
	GetSAMLProviderResult struct {
		SAMLMetadataDocument string `xml:"SAMLMetadataDocument"`
	} `xml:"GetSAMLProviderResult"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// GetSAMLProviderXML builds GetSAMLProvider response XML.
func GetSAMLProviderXML(p store.SAMLProvider, requestID string) ([]byte, error) {
	resp := getSAMLProviderResponse{XMLNS: iamXMLNS}
	resp.GetSAMLProviderResult.SAMLMetadataDocument = p.MetadataXML
	resp.ResponseMetadata.RequestId = requestID
	return marshalResponse(resp)
}

// --- MFA ---

type virtualMFADeviceXML struct {
	SerialNumber     string `xml:"SerialNumber"`
	Base32StringSeed string `xml:"Base32StringSeed,omitempty"`
}

type createVirtualMFADeviceResponse struct {
	XMLName                     xml.Name `xml:"CreateVirtualMFADeviceResponse"`
	XMLNS                       string   `xml:"xmlns,attr"`
	CreateVirtualMFADeviceResult struct {
		VirtualMFADevice virtualMFADeviceXML `xml:"VirtualMFADevice"`
	} `xml:"CreateVirtualMFADeviceResult"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// CreateVirtualMFADeviceXML builds CreateVirtualMFADevice response XML.
// base32Seed is the lab seed material returned once at create time (hex string).
func CreateVirtualMFADeviceXML(serial, base32Seed, requestID string) ([]byte, error) {
	resp := createVirtualMFADeviceResponse{XMLNS: iamXMLNS}
	resp.CreateVirtualMFADeviceResult.VirtualMFADevice = virtualMFADeviceXML{
		SerialNumber: serial, Base32StringSeed: base32Seed,
	}
	resp.ResponseMetadata.RequestId = requestID
	return marshalResponse(resp)
}

type listMFADevicesResponse struct {
	XMLName              xml.Name `xml:"ListMFADevicesResponse"`
	XMLNS                string   `xml:"xmlns,attr"`
	ListMFADevicesResult struct {
		MFADevices  []mfaDeviceMemberXML `xml:"MFADevices>member"`
		IsTruncated bool                 `xml:"IsTruncated"`
	} `xml:"ListMFADevicesResult"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

type mfaDeviceMemberXML struct {
	UserName     string `xml:"UserName"`
	SerialNumber string `xml:"SerialNumber"`
	EnableDate   string `xml:"EnableDate,omitempty"`
}

// ListMFADevicesXML builds ListMFADevices response XML.
func ListMFADevicesXML(devices []store.MFADevice, requestID string) ([]byte, error) {
	resp := listMFADevicesResponse{XMLNS: iamXMLNS}
	for _, d := range devices {
		resp.ListMFADevicesResult.MFADevices = append(resp.ListMFADevicesResult.MFADevices, mfaDeviceMemberXML{
			UserName: d.UserName, SerialNumber: d.Serial,
		})
	}
	resp.ResponseMetadata.RequestId = requestID
	return marshalResponse(resp)
}

// EnableMFADeviceXML builds EnableMFADevice response XML.
func EnableMFADeviceXML(requestID string) ([]byte, error) {
	return emptyIAMResponse("EnableMFADevice", requestID)
}

// DeactivateMFADeviceXML builds DeactivateMFADevice response XML.
func DeactivateMFADeviceXML(requestID string) ([]byte, error) {
	return emptyIAMResponse("DeactivateMFADevice", requestID)
}
