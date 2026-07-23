package iam

import (
	"encoding/xml"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const iamXMLNS = "https://iam.amazonaws.com/doc/2010-05-08/"

type responseMetadata struct {
	RequestId string `xml:"RequestId"`
}

func marshalResponse(v any) ([]byte, error) {
	out, err := xml.Marshal(v)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// --- Users ---

type createUserResponse struct {
	XMLName          xml.Name `xml:"CreateUserResponse"`
	XMLNS            string   `xml:"xmlns,attr"`
	CreateUserResult struct {
		User userXML `xml:"User"`
	} `xml:"CreateUserResult"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

type permissionsBoundaryXML struct {
	PermissionsBoundaryType string `xml:"PermissionsBoundaryType"`
	PermissionsBoundaryArn  string `xml:"PermissionsBoundaryArn"`
}

type userXML struct {
	Path                string                  `xml:"Path"`
	UserName            string                  `xml:"UserName"`
	UserId              string                  `xml:"UserId"`
	Arn                 string                  `xml:"Arn"`
	CreateDate          string                  `xml:"CreateDate,omitempty"`
	PermissionsBoundary *permissionsBoundaryXML `xml:"PermissionsBoundary,omitempty"`
}

// CreateUserXML builds CreateUser response XML.
func CreateUserXML(u store.User, requestID string) ([]byte, error) {
	resp := createUserResponse{XMLNS: iamXMLNS}
	resp.CreateUserResult.User = userXML{
		Path:       "/",
		UserName:   u.UserName,
		UserId:     u.UserID,
		Arn:        u.ARN,
		CreateDate: u.CreateDate,
	}
	resp.ResponseMetadata.RequestId = requestID
	return marshalResponse(resp)
}

type getUserResponse struct {
	XMLName       xml.Name `xml:"GetUserResponse"`
	XMLNS         string   `xml:"xmlns,attr"`
	GetUserResult struct {
		User userXML `xml:"User"`
	} `xml:"GetUserResult"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// GetUserXML builds GetUser response XML.
// boundaryARN, when non-empty, is embedded as PermissionsBoundary (AWS-shaped).
func GetUserXML(u store.User, boundaryARN, requestID string) ([]byte, error) {
	resp := getUserResponse{XMLNS: iamXMLNS}
	resp.GetUserResult.User = userXML{
		Path: "/", UserName: u.UserName, UserId: u.UserID, Arn: u.ARN, CreateDate: u.CreateDate,
	}
	if boundaryARN != "" {
		resp.GetUserResult.User.PermissionsBoundary = &permissionsBoundaryXML{
			PermissionsBoundaryType: "PermissionsBoundaryPolicy",
			PermissionsBoundaryArn:  boundaryARN,
		}
	}
	resp.ResponseMetadata.RequestId = requestID
	return marshalResponse(resp)
}

type listUsersResponse struct {
	XMLName         xml.Name `xml:"ListUsersResponse"`
	XMLNS           string   `xml:"xmlns,attr"`
	ListUsersResult struct {
		Users       []userXML `xml:"Users>member"`
		IsTruncated bool      `xml:"IsTruncated"`
	} `xml:"ListUsersResult"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// ListUsersXML builds ListUsers response XML.
func ListUsersXML(users []store.User, requestID string) ([]byte, error) {
	resp := listUsersResponse{XMLNS: iamXMLNS}
	for _, u := range users {
		resp.ListUsersResult.Users = append(resp.ListUsersResult.Users, userXML{
			Path: "/", UserName: u.UserName, UserId: u.UserID, Arn: u.ARN, CreateDate: u.CreateDate,
		})
	}
	resp.ResponseMetadata.RequestId = requestID
	return marshalResponse(resp)
}

type deleteUserResponse struct {
	XMLName          xml.Name         `xml:"DeleteUserResponse"`
	XMLNS            string           `xml:"xmlns,attr"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// DeleteUserXML builds DeleteUser response XML.
func DeleteUserXML(requestID string) ([]byte, error) {
	return marshalResponse(deleteUserResponse{XMLNS: iamXMLNS, ResponseMetadata: responseMetadata{RequestId: requestID}})
}

// --- Access keys ---

type accessKeyXML struct {
	UserName        string `xml:"UserName"`
	AccessKeyId     string `xml:"AccessKeyId"`
	Status          string `xml:"Status"`
	SecretAccessKey string `xml:"SecretAccessKey,omitempty"`
	CreateDate      string `xml:"CreateDate,omitempty"`
}

type createAccessKeyResponse struct {
	XMLName               xml.Name `xml:"CreateAccessKeyResponse"`
	XMLNS                 string   `xml:"xmlns,attr"`
	CreateAccessKeyResult struct {
		AccessKey accessKeyXML `xml:"AccessKey"`
	} `xml:"CreateAccessKeyResult"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// CreateAccessKeyXML builds CreateAccessKey response XML (includes secret once).
func CreateAccessKeyXML(userName, accessKeyID, secret, requestID string) ([]byte, error) {
	resp := createAccessKeyResponse{XMLNS: iamXMLNS}
	resp.CreateAccessKeyResult.AccessKey = accessKeyXML{
		UserName:        userName,
		AccessKeyId:     accessKeyID,
		Status:          store.AccessKeyStatusActive,
		SecretAccessKey: secret,
		CreateDate:      time.Now().UTC().Format(time.RFC3339),
	}
	resp.ResponseMetadata.RequestId = requestID
	return marshalResponse(resp)
}

type listAccessKeysResponse struct {
	XMLName              xml.Name `xml:"ListAccessKeysResponse"`
	XMLNS                string   `xml:"xmlns,attr"`
	ListAccessKeysResult struct {
		AccessKeyMetadata []accessKeyMetaXML `xml:"AccessKeyMetadata>member"`
		IsTruncated       bool               `xml:"IsTruncated"`
	} `xml:"ListAccessKeysResult"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

type accessKeyMetaXML struct {
	UserName    string `xml:"UserName"`
	AccessKeyId string `xml:"AccessKeyId"`
	Status      string `xml:"Status"`
}

// ListAccessKeysXML builds ListAccessKeys response XML.
func ListAccessKeysXML(keys []store.AccessKeyMeta, requestID string) ([]byte, error) {
	resp := listAccessKeysResponse{XMLNS: iamXMLNS}
	for _, k := range keys {
		resp.ListAccessKeysResult.AccessKeyMetadata = append(resp.ListAccessKeysResult.AccessKeyMetadata, accessKeyMetaXML{
			UserName: k.UserName, AccessKeyId: k.AccessKeyID, Status: k.Status,
		})
	}
	resp.ResponseMetadata.RequestId = requestID
	return marshalResponse(resp)
}

type deleteAccessKeyResponse struct {
	XMLName          xml.Name         `xml:"DeleteAccessKeyResponse"`
	XMLNS            string           `xml:"xmlns,attr"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// DeleteAccessKeyXML builds DeleteAccessKey response XML.
func DeleteAccessKeyXML(requestID string) ([]byte, error) {
	return marshalResponse(deleteAccessKeyResponse{XMLNS: iamXMLNS, ResponseMetadata: responseMetadata{RequestId: requestID}})
}

type updateAccessKeyResponse struct {
	XMLName          xml.Name         `xml:"UpdateAccessKeyResponse"`
	XMLNS            string           `xml:"xmlns,attr"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// UpdateAccessKeyXML builds UpdateAccessKey response XML.
func UpdateAccessKeyXML(requestID string) ([]byte, error) {
	return marshalResponse(updateAccessKeyResponse{XMLNS: iamXMLNS, ResponseMetadata: responseMetadata{RequestId: requestID}})
}

// --- Managed policies ---

type policyXML struct {
	PolicyName       string `xml:"PolicyName"`
	PolicyId         string `xml:"PolicyId"`
	Arn              string `xml:"Arn"`
	Path             string `xml:"Path"`
	DefaultVersionId string `xml:"DefaultVersionId"`
	AttachmentCount  int    `xml:"AttachmentCount,omitempty"`
	IsAttachable     bool   `xml:"IsAttachable"`
}

type createPolicyResponse struct {
	XMLName            xml.Name `xml:"CreatePolicyResponse"`
	XMLNS              string   `xml:"xmlns,attr"`
	CreatePolicyResult struct {
		Policy policyXML `xml:"Policy"`
	} `xml:"CreatePolicyResult"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// CreatePolicyXML builds CreatePolicy response XML.
func CreatePolicyXML(p store.ManagedPolicy, requestID string) ([]byte, error) {
	resp := createPolicyResponse{XMLNS: iamXMLNS}
	resp.CreatePolicyResult.Policy = policyFromManaged(p)
	resp.ResponseMetadata.RequestId = requestID
	return marshalResponse(resp)
}

func policyFromManaged(p store.ManagedPolicy) policyXML {
	return policyXML{
		PolicyName:       p.PolicyName,
		PolicyId:         p.PolicyID,
		Arn:              p.PolicyARN,
		Path:             "/",
		DefaultVersionId: p.DefaultVersionID,
		IsAttachable:     true,
	}
}

type getPolicyResponse struct {
	XMLName         xml.Name `xml:"GetPolicyResponse"`
	XMLNS           string   `xml:"xmlns,attr"`
	GetPolicyResult struct {
		Policy policyXML `xml:"Policy"`
	} `xml:"GetPolicyResult"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// GetPolicyXML builds GetPolicy response XML.
func GetPolicyXML(p store.ManagedPolicy, requestID string) ([]byte, error) {
	resp := getPolicyResponse{XMLNS: iamXMLNS}
	resp.GetPolicyResult.Policy = policyFromManaged(p)
	resp.ResponseMetadata.RequestId = requestID
	return marshalResponse(resp)
}

type listPoliciesResponse struct {
	XMLName            xml.Name `xml:"ListPoliciesResponse"`
	XMLNS              string   `xml:"xmlns,attr"`
	ListPoliciesResult struct {
		Policies    []policyXML `xml:"Policies>member"`
		IsTruncated bool        `xml:"IsTruncated"`
	} `xml:"ListPoliciesResult"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// ListPoliciesXML builds ListPolicies response XML.
func ListPoliciesXML(policies []store.ManagedPolicy, requestID string) ([]byte, error) {
	resp := listPoliciesResponse{XMLNS: iamXMLNS}
	for _, p := range policies {
		resp.ListPoliciesResult.Policies = append(resp.ListPoliciesResult.Policies, policyFromManaged(p))
	}
	resp.ResponseMetadata.RequestId = requestID
	return marshalResponse(resp)
}

type deletePolicyResponse struct {
	XMLName          xml.Name         `xml:"DeletePolicyResponse"`
	XMLNS            string           `xml:"xmlns,attr"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// DeletePolicyXML builds DeletePolicy response XML.
func DeletePolicyXML(requestID string) ([]byte, error) {
	return marshalResponse(deletePolicyResponse{XMLNS: iamXMLNS, ResponseMetadata: responseMetadata{RequestId: requestID}})
}

// --- Managed policy versions ---

type policyVersionXML struct {
	VersionId        string `xml:"VersionId"`
	IsDefaultVersion bool   `xml:"IsDefaultVersion"`
	CreateDate       string `xml:"CreateDate,omitempty"`
	Document         string `xml:"Document,omitempty"`
}

func policyVersionFromStore(v store.ManagedPolicyVersion, includeDocument bool) policyVersionXML {
	out := policyVersionXML{
		VersionId:        v.VersionID,
		IsDefaultVersion: v.IsDefaultVersion,
		CreateDate:       v.CreateDate,
	}
	if includeDocument {
		out.Document = v.Document
	}
	return out
}

type createPolicyVersionResponse struct {
	XMLName                   xml.Name `xml:"CreatePolicyVersionResponse"`
	XMLNS                     string   `xml:"xmlns,attr"`
	CreatePolicyVersionResult struct {
		PolicyVersion policyVersionXML `xml:"PolicyVersion"`
	} `xml:"CreatePolicyVersionResult"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// CreatePolicyVersionXML builds CreatePolicyVersion response XML.
func CreatePolicyVersionXML(v store.ManagedPolicyVersion, requestID string) ([]byte, error) {
	resp := createPolicyVersionResponse{XMLNS: iamXMLNS}
	resp.CreatePolicyVersionResult.PolicyVersion = policyVersionFromStore(v, false)
	resp.ResponseMetadata.RequestId = requestID
	return marshalResponse(resp)
}

type getPolicyVersionResponse struct {
	XMLName                xml.Name `xml:"GetPolicyVersionResponse"`
	XMLNS                  string   `xml:"xmlns,attr"`
	GetPolicyVersionResult struct {
		PolicyVersion policyVersionXML `xml:"PolicyVersion"`
	} `xml:"GetPolicyVersionResult"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// GetPolicyVersionXML builds GetPolicyVersion response XML (includes Document).
func GetPolicyVersionXML(v store.ManagedPolicyVersion, requestID string) ([]byte, error) {
	resp := getPolicyVersionResponse{XMLNS: iamXMLNS}
	resp.GetPolicyVersionResult.PolicyVersion = policyVersionFromStore(v, true)
	resp.ResponseMetadata.RequestId = requestID
	return marshalResponse(resp)
}

type listPolicyVersionsResponse struct {
	XMLName                  xml.Name `xml:"ListPolicyVersionsResponse"`
	XMLNS                    string   `xml:"xmlns,attr"`
	ListPolicyVersionsResult struct {
		Versions    []policyVersionXML `xml:"Versions>member"`
		IsTruncated bool               `xml:"IsTruncated"`
	} `xml:"ListPolicyVersionsResult"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// ListPolicyVersionsXML builds ListPolicyVersions response XML.
func ListPolicyVersionsXML(versions []store.ManagedPolicyVersion, requestID string) ([]byte, error) {
	resp := listPolicyVersionsResponse{XMLNS: iamXMLNS}
	for _, v := range versions {
		resp.ListPolicyVersionsResult.Versions = append(resp.ListPolicyVersionsResult.Versions, policyVersionFromStore(v, false))
	}
	resp.ResponseMetadata.RequestId = requestID
	return marshalResponse(resp)
}

type deletePolicyVersionResponse struct {
	XMLName          xml.Name         `xml:"DeletePolicyVersionResponse"`
	XMLNS            string           `xml:"xmlns,attr"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// DeletePolicyVersionXML builds DeletePolicyVersion response XML.
func DeletePolicyVersionXML(requestID string) ([]byte, error) {
	return marshalResponse(deletePolicyVersionResponse{XMLNS: iamXMLNS, ResponseMetadata: responseMetadata{RequestId: requestID}})
}

type setDefaultPolicyVersionResponse struct {
	XMLName          xml.Name         `xml:"SetDefaultPolicyVersionResponse"`
	XMLNS            string           `xml:"xmlns,attr"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// SetDefaultPolicyVersionXML builds SetDefaultPolicyVersion response XML.
func SetDefaultPolicyVersionXML(requestID string) ([]byte, error) {
	return marshalResponse(setDefaultPolicyVersionResponse{XMLNS: iamXMLNS, ResponseMetadata: responseMetadata{RequestId: requestID}})
}

// --- Attachments ---

type attachUserPolicyResponse struct {
	XMLName          xml.Name         `xml:"AttachUserPolicyResponse"`
	XMLNS            string           `xml:"xmlns,attr"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// AttachUserPolicyXML builds AttachUserPolicy response XML.
func AttachUserPolicyXML(requestID string) ([]byte, error) {
	return marshalResponse(attachUserPolicyResponse{XMLNS: iamXMLNS, ResponseMetadata: responseMetadata{RequestId: requestID}})
}

type detachUserPolicyResponse struct {
	XMLName          xml.Name         `xml:"DetachUserPolicyResponse"`
	XMLNS            string           `xml:"xmlns,attr"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// DetachUserPolicyXML builds DetachUserPolicy response XML.
func DetachUserPolicyXML(requestID string) ([]byte, error) {
	return marshalResponse(detachUserPolicyResponse{XMLNS: iamXMLNS, ResponseMetadata: responseMetadata{RequestId: requestID}})
}

type attachRolePolicyResponse struct {
	XMLName          xml.Name         `xml:"AttachRolePolicyResponse"`
	XMLNS            string           `xml:"xmlns,attr"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// AttachRolePolicyXML builds AttachRolePolicy response XML.
func AttachRolePolicyXML(requestID string) ([]byte, error) {
	return marshalResponse(attachRolePolicyResponse{XMLNS: iamXMLNS, ResponseMetadata: responseMetadata{RequestId: requestID}})
}

type detachRolePolicyResponse struct {
	XMLName          xml.Name         `xml:"DetachRolePolicyResponse"`
	XMLNS            string           `xml:"xmlns,attr"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// DetachRolePolicyXML builds DetachRolePolicy response XML.
func DetachRolePolicyXML(requestID string) ([]byte, error) {
	return marshalResponse(detachRolePolicyResponse{XMLNS: iamXMLNS, ResponseMetadata: responseMetadata{RequestId: requestID}})
}

type attachedPolicyXML struct {
	PolicyName string `xml:"PolicyName"`
	PolicyArn  string `xml:"PolicyArn"`
}

type listAttachedUserPoliciesResponse struct {
	XMLName                        xml.Name `xml:"ListAttachedUserPoliciesResponse"`
	XMLNS                          string   `xml:"xmlns,attr"`
	ListAttachedUserPoliciesResult struct {
		AttachedPolicies []attachedPolicyXML `xml:"AttachedPolicies>member"`
		IsTruncated      bool                `xml:"IsTruncated"`
	} `xml:"ListAttachedUserPoliciesResult"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// ListAttachedUserPoliciesXML builds ListAttachedUserPolicies response XML.
func ListAttachedUserPoliciesXML(refs []store.AttachedPolicyRef, requestID string) ([]byte, error) {
	resp := listAttachedUserPoliciesResponse{XMLNS: iamXMLNS}
	for _, r := range refs {
		resp.ListAttachedUserPoliciesResult.AttachedPolicies = append(resp.ListAttachedUserPoliciesResult.AttachedPolicies, attachedPolicyXML{
			PolicyName: r.PolicyName, PolicyArn: r.PolicyARN,
		})
	}
	resp.ResponseMetadata.RequestId = requestID
	return marshalResponse(resp)
}

type listAttachedRolePoliciesResponse struct {
	XMLName                        xml.Name `xml:"ListAttachedRolePoliciesResponse"`
	XMLNS                          string   `xml:"xmlns,attr"`
	ListAttachedRolePoliciesResult struct {
		AttachedPolicies []attachedPolicyXML `xml:"AttachedPolicies>member"`
		IsTruncated      bool                `xml:"IsTruncated"`
	} `xml:"ListAttachedRolePoliciesResult"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// ListAttachedRolePoliciesXML builds ListAttachedRolePolicies response XML.
func ListAttachedRolePoliciesXML(refs []store.AttachedPolicyRef, requestID string) ([]byte, error) {
	resp := listAttachedRolePoliciesResponse{XMLNS: iamXMLNS}
	for _, r := range refs {
		resp.ListAttachedRolePoliciesResult.AttachedPolicies = append(resp.ListAttachedRolePoliciesResult.AttachedPolicies, attachedPolicyXML{
			PolicyName: r.PolicyName, PolicyArn: r.PolicyARN,
		})
	}
	resp.ResponseMetadata.RequestId = requestID
	return marshalResponse(resp)
}

// --- Inline policies ---

type putUserPolicyResponse struct {
	XMLName          xml.Name         `xml:"PutUserPolicyResponse"`
	XMLNS            string           `xml:"xmlns,attr"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// PutUserPolicyXML builds PutUserPolicy response XML.
func PutUserPolicyXML(requestID string) ([]byte, error) {
	return marshalResponse(putUserPolicyResponse{XMLNS: iamXMLNS, ResponseMetadata: responseMetadata{RequestId: requestID}})
}

type getUserPolicyResponse struct {
	XMLName             xml.Name `xml:"GetUserPolicyResponse"`
	XMLNS               string   `xml:"xmlns,attr"`
	GetUserPolicyResult struct {
		UserName       string `xml:"UserName"`
		PolicyName     string `xml:"PolicyName"`
		PolicyDocument string `xml:"PolicyDocument"`
	} `xml:"GetUserPolicyResult"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// GetUserPolicyXML builds GetUserPolicy response XML.
func GetUserPolicyXML(userName, policyName, document, requestID string) ([]byte, error) {
	resp := getUserPolicyResponse{XMLNS: iamXMLNS}
	resp.GetUserPolicyResult.UserName = userName
	resp.GetUserPolicyResult.PolicyName = policyName
	resp.GetUserPolicyResult.PolicyDocument = document
	resp.ResponseMetadata.RequestId = requestID
	return marshalResponse(resp)
}

type deleteUserPolicyResponse struct {
	XMLName          xml.Name         `xml:"DeleteUserPolicyResponse"`
	XMLNS            string           `xml:"xmlns,attr"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// DeleteUserPolicyXML builds DeleteUserPolicy response XML.
func DeleteUserPolicyXML(requestID string) ([]byte, error) {
	return marshalResponse(deleteUserPolicyResponse{XMLNS: iamXMLNS, ResponseMetadata: responseMetadata{RequestId: requestID}})
}

type listUserPoliciesResponse struct {
	XMLName                xml.Name `xml:"ListUserPoliciesResponse"`
	XMLNS                  string   `xml:"xmlns,attr"`
	ListUserPoliciesResult struct {
		PolicyNames []string `xml:"PolicyNames>member"`
		IsTruncated bool     `xml:"IsTruncated"`
	} `xml:"ListUserPoliciesResult"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// ListUserPoliciesXML builds ListUserPolicies response XML.
func ListUserPoliciesXML(names []string, requestID string) ([]byte, error) {
	resp := listUserPoliciesResponse{XMLNS: iamXMLNS}
	resp.ListUserPoliciesResult.PolicyNames = names
	resp.ResponseMetadata.RequestId = requestID
	return marshalResponse(resp)
}

type putRolePolicyResponse struct {
	XMLName          xml.Name         `xml:"PutRolePolicyResponse"`
	XMLNS            string           `xml:"xmlns,attr"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// PutRolePolicyXML builds PutRolePolicy response XML.
func PutRolePolicyXML(requestID string) ([]byte, error) {
	return marshalResponse(putRolePolicyResponse{XMLNS: iamXMLNS, ResponseMetadata: responseMetadata{RequestId: requestID}})
}

type getRolePolicyResponse struct {
	XMLName             xml.Name `xml:"GetRolePolicyResponse"`
	XMLNS               string   `xml:"xmlns,attr"`
	GetRolePolicyResult struct {
		RoleName       string `xml:"RoleName"`
		PolicyName     string `xml:"PolicyName"`
		PolicyDocument string `xml:"PolicyDocument"`
	} `xml:"GetRolePolicyResult"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// GetRolePolicyXML builds GetRolePolicy response XML.
func GetRolePolicyXML(roleName, policyName, document, requestID string) ([]byte, error) {
	resp := getRolePolicyResponse{XMLNS: iamXMLNS}
	resp.GetRolePolicyResult.RoleName = roleName
	resp.GetRolePolicyResult.PolicyName = policyName
	resp.GetRolePolicyResult.PolicyDocument = document
	resp.ResponseMetadata.RequestId = requestID
	return marshalResponse(resp)
}

type deleteRolePolicyResponse struct {
	XMLName          xml.Name         `xml:"DeleteRolePolicyResponse"`
	XMLNS            string           `xml:"xmlns,attr"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// DeleteRolePolicyXML builds DeleteRolePolicy response XML.
func DeleteRolePolicyXML(requestID string) ([]byte, error) {
	return marshalResponse(deleteRolePolicyResponse{XMLNS: iamXMLNS, ResponseMetadata: responseMetadata{RequestId: requestID}})
}

type listRolePoliciesResponse struct {
	XMLName                xml.Name `xml:"ListRolePoliciesResponse"`
	XMLNS                  string   `xml:"xmlns,attr"`
	ListRolePoliciesResult struct {
		PolicyNames []string `xml:"PolicyNames>member"`
		IsTruncated bool     `xml:"IsTruncated"`
	} `xml:"ListRolePoliciesResult"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// ListRolePoliciesXML builds ListRolePolicies response XML.
func ListRolePoliciesXML(names []string, requestID string) ([]byte, error) {
	resp := listRolePoliciesResponse{XMLNS: iamXMLNS}
	resp.ListRolePoliciesResult.PolicyNames = names
	resp.ResponseMetadata.RequestId = requestID
	return marshalResponse(resp)
}

// --- Roles ---

type roleXML struct {
	Path                     string                  `xml:"Path"`
	RoleName                 string                  `xml:"RoleName"`
	RoleId                   string                  `xml:"RoleId"`
	Arn                      string                  `xml:"Arn"`
	CreateDate               string                  `xml:"CreateDate,omitempty"`
	AssumeRolePolicyDocument string                  `xml:"AssumeRolePolicyDocument,omitempty"`
	MaxSessionDuration       int                     `xml:"MaxSessionDuration,omitempty"`
	PermissionsBoundary      *permissionsBoundaryXML `xml:"PermissionsBoundary,omitempty"`
}

type createRoleResponse struct {
	XMLName          xml.Name `xml:"CreateRoleResponse"`
	XMLNS            string   `xml:"xmlns,attr"`
	CreateRoleResult struct {
		Role roleXML `xml:"Role"`
	} `xml:"CreateRoleResult"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// CreateRoleXML builds CreateRole response XML.
func CreateRoleXML(r store.Role, requestID string) ([]byte, error) {
	resp := createRoleResponse{XMLNS: iamXMLNS}
	maxDur := r.MaxSessionDuration
	if maxDur <= 0 {
		maxDur = 3600
	}
	resp.CreateRoleResult.Role = roleXML{
		Path: "/", RoleName: r.RoleName, RoleId: r.RoleID, Arn: r.RoleARN,
		CreateDate: r.CreateDate, AssumeRolePolicyDocument: r.TrustPolicy,
		MaxSessionDuration: maxDur,
	}
	resp.ResponseMetadata.RequestId = requestID
	return marshalResponse(resp)
}

type getRoleResponse struct {
	XMLName       xml.Name `xml:"GetRoleResponse"`
	XMLNS         string   `xml:"xmlns,attr"`
	GetRoleResult struct {
		Role roleXML `xml:"Role"`
	} `xml:"GetRoleResult"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// GetRoleXML builds GetRole response XML.
// boundaryARN, when non-empty, is embedded as PermissionsBoundary (AWS-shaped).
func GetRoleXML(r store.Role, boundaryARN, requestID string) ([]byte, error) {
	resp := getRoleResponse{XMLNS: iamXMLNS}
	maxDur := r.MaxSessionDuration
	if maxDur <= 0 {
		maxDur = 3600
	}
	resp.GetRoleResult.Role = roleXML{
		Path: "/", RoleName: r.RoleName, RoleId: r.RoleID, Arn: r.RoleARN,
		CreateDate: r.CreateDate, AssumeRolePolicyDocument: r.TrustPolicy,
		MaxSessionDuration: maxDur,
	}
	if boundaryARN != "" {
		resp.GetRoleResult.Role.PermissionsBoundary = &permissionsBoundaryXML{
			PermissionsBoundaryType: "PermissionsBoundaryPolicy",
			PermissionsBoundaryArn:  boundaryARN,
		}
	}
	resp.ResponseMetadata.RequestId = requestID
	return marshalResponse(resp)
}

type listRolesResponse struct {
	XMLName         xml.Name `xml:"ListRolesResponse"`
	XMLNS           string   `xml:"xmlns,attr"`
	ListRolesResult struct {
		Roles       []roleXML `xml:"Roles>member"`
		IsTruncated bool      `xml:"IsTruncated"`
	} `xml:"ListRolesResult"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// ListRolesXML builds ListRoles response XML.
func ListRolesXML(roles []store.Role, requestID string) ([]byte, error) {
	resp := listRolesResponse{XMLNS: iamXMLNS}
	for _, r := range roles {
		resp.ListRolesResult.Roles = append(resp.ListRolesResult.Roles, roleXML{
			Path: "/", RoleName: r.RoleName, RoleId: r.RoleID, Arn: r.RoleARN,
			CreateDate: r.CreateDate, AssumeRolePolicyDocument: r.TrustPolicy,
		})
	}
	resp.ResponseMetadata.RequestId = requestID
	return marshalResponse(resp)
}

type deleteRoleResponse struct {
	XMLName          xml.Name         `xml:"DeleteRoleResponse"`
	XMLNS            string           `xml:"xmlns,attr"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// DeleteRoleXML builds DeleteRole response XML.
func DeleteRoleXML(requestID string) ([]byte, error) {
	return marshalResponse(deleteRoleResponse{XMLNS: iamXMLNS, ResponseMetadata: responseMetadata{RequestId: requestID}})
}

type updateAssumeRolePolicyResponse struct {
	XMLName          xml.Name         `xml:"UpdateAssumeRolePolicyResponse"`
	XMLNS            string           `xml:"xmlns,attr"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// UpdateAssumeRolePolicyXML builds UpdateAssumeRolePolicy response XML.
func UpdateAssumeRolePolicyXML(requestID string) ([]byte, error) {
	return marshalResponse(updateAssumeRolePolicyResponse{XMLNS: iamXMLNS, ResponseMetadata: responseMetadata{RequestId: requestID}})
}
