package server

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/sts"
	"github.com/Kyaxris-Labs/Noctaxris/internal/services/iam"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
	"github.com/Kyaxris-Labs/Noctaxris/internal/validate"
)

// handleIAMIdentity handles IAM query actions for groups, boundaries, instance profiles, IdP, and MFA.
// Returns handled=false when action is not one of those identity APIs.
func (s *Server) handleIAMIdentity(
	w http.ResponseWriter,
	r *http.Request,
	params map[string]string,
	requestID, eventID, action string,
	accountID string,
	verifiedAccessKeyID string,
	readOnly bool,
) (payload []byte, handled bool, err error) {
	switch action {
	case catalog.ActionIAMCreateGroup, "CreateGroup":
		handled = true
		groupName := params["GroupName"]
		if groupName == "" {
			s.writeAWSError(w, requestID, http.StatusBadRequest, "ValidationError",
				"GroupName is required.", readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
			return nil, true, errHandled
		}
		groupID, arn, createErr := s.store.CreateGroup(accountID, groupName)
		if createErr != nil {
			if validate.IsInvalid(createErr) {
				s.writeAWSError(w, requestID, http.StatusBadRequest, "ValidationError",
					createErr.Error(), readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
				return nil, true, errHandled
			}
			s.writeAWSError(w, requestID, http.StatusConflict, "EntityAlreadyExists",
				"Group already exists.", readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
			return nil, true, errHandled
		}
		payload, err = iam.CreateGroupXML(store.Group{
			AccountID: accountID, GroupName: groupName, GroupID: groupID, ARN: arn,
		}, requestID)

	case catalog.ActionIAMDeleteGroup, "DeleteGroup":
		handled = true
		if delErr := s.store.DeleteGroup(accountID, params["GroupName"]); delErr != nil {
			if errors.Is(delErr, sql.ErrNoRows) {
				s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
					"Group not found.", readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
				return nil, true, errHandled
			}
			s.writeAWSError(w, requestID, http.StatusBadRequest, "DeleteConflict",
				"Unable to delete group.", readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
			return nil, true, errHandled
		}
		payload, err = iam.DeleteGroupXML(requestID)

	case catalog.ActionIAMGetGroup, "GetGroup":
		handled = true
		g, getErr := s.store.GetGroup(accountID, params["GroupName"])
		if getErr != nil {
			s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
				"Group not found.", readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
			return nil, true, errHandled
		}
		members, listErr := s.store.ListGroupMembers(accountID, params["GroupName"])
		if listErr != nil {
			s.writeAWSError(w, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to list group members.", readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
			return nil, true, errHandled
		}
		payload, err = iam.GetGroupXML(g, members, requestID)

	case catalog.ActionIAMListGroups, "ListGroups":
		handled = true
		groups, listErr := s.store.ListGroups(accountID)
		if listErr != nil {
			s.writeAWSError(w, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to list groups.", readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
			return nil, true, errHandled
		}
		payload, err = iam.ListGroupsXML(groups, requestID)

	case catalog.ActionIAMAddUserToGroup, "AddUserToGroup":
		handled = true
		if addErr := s.store.AddUserToGroup(accountID, params["GroupName"], params["UserName"]); addErr != nil {
			s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
				"Unable to add user to group.", readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
			return nil, true, errHandled
		}
		payload, err = iam.AddUserToGroupXML(requestID)

	case catalog.ActionIAMRemoveUserFromGroup, "RemoveUserFromGroup":
		handled = true
		if remErr := s.store.RemoveUserFromGroup(accountID, params["GroupName"], params["UserName"]); remErr != nil {
			s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
				"Unable to remove user from group.", readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
			return nil, true, errHandled
		}
		payload, err = iam.RemoveUserFromGroupXML(requestID)

	case catalog.ActionIAMAttachGroupPolicy, "AttachGroupPolicy":
		handled = true
		if attErr := s.store.AttachGroupPolicy(accountID, params["GroupName"], params["PolicyArn"]); attErr != nil {
			s.writeAWSError(w, requestID, http.StatusBadRequest, "NoSuchEntity",
				"Unable to attach group policy.", readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
			return nil, true, errHandled
		}
		payload, err = iam.AttachGroupPolicyXML(requestID)

	case catalog.ActionIAMDetachGroupPolicy, "DetachGroupPolicy":
		handled = true
		if detErr := s.store.DetachGroupPolicy(accountID, params["GroupName"], params["PolicyArn"]); detErr != nil {
			s.writeAWSError(w, requestID, http.StatusBadRequest, "NoSuchEntity",
				"Unable to detach group policy.", readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
			return nil, true, errHandled
		}
		payload, err = iam.DetachGroupPolicyXML(requestID)

	case catalog.ActionIAMListAttachedGroupPolicies, "ListAttachedGroupPolicies":
		handled = true
		g, getErr := s.store.GetGroup(accountID, params["GroupName"])
		if getErr != nil {
			s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
				"Group not found.", readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
			return nil, true, errHandled
		}
		refs, listErr := s.store.ListAttachedPolicyRefs(g.ARN)
		if listErr != nil {
			s.writeAWSError(w, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to list attached group policies.", readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
			return nil, true, errHandled
		}
		payload, err = iam.ListAttachedGroupPoliciesXML(refs, requestID)

	case catalog.ActionIAMPutGroupPolicy, "PutGroupPolicy":
		handled = true
		doc, _ := url.QueryUnescape(params["PolicyDocument"])
		if doc == "" {
			doc = params["PolicyDocument"]
		}
		if putErr := s.store.PutGroupInlinePolicy(accountID, params["GroupName"], params["PolicyName"], doc); putErr != nil {
			s.writeAWSError(w, requestID, http.StatusBadRequest, "MalformedPolicyDocument",
				"Unable to put group policy.", readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
			return nil, true, errHandled
		}
		payload, err = iam.PutGroupPolicyXML(requestID)

	case catalog.ActionIAMGetGroupPolicy, "GetGroupPolicy":
		handled = true
		g, getErr := s.store.GetGroup(accountID, params["GroupName"])
		if getErr != nil {
			s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
				"Group not found.", readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
			return nil, true, errHandled
		}
		p, getErr := s.store.GetInlinePolicy(g.ARN, params["PolicyName"])
		if getErr != nil {
			s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
				"Policy not found.", readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
			return nil, true, errHandled
		}
		payload, err = iam.GetGroupPolicyXML(params["GroupName"], params["PolicyName"], p.Document, requestID)

	case catalog.ActionIAMDeleteGroupPolicy, "DeleteGroupPolicy":
		handled = true
		g, getErr := s.store.GetGroup(accountID, params["GroupName"])
		if getErr != nil {
			s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
				"Group not found.", readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
			return nil, true, errHandled
		}
		if delErr := s.store.DeleteInlinePolicy(g.ARN, params["PolicyName"]); delErr != nil {
			s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
				"Policy not found.", readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
			return nil, true, errHandled
		}
		payload, err = iam.DeleteGroupPolicyXML(requestID)

	case catalog.ActionIAMListGroupPolicies, "ListGroupPolicies":
		handled = true
		g, getErr := s.store.GetGroup(accountID, params["GroupName"])
		if getErr != nil {
			s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
				"Group not found.", readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
			return nil, true, errHandled
		}
		pols, listErr := s.store.ListInlinePolicies(g.ARN)
		if listErr != nil {
			s.writeAWSError(w, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to list group policies.", readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
			return nil, true, errHandled
		}
		names := make([]string, 0, len(pols))
		for _, p := range pols {
			names = append(names, p.PolicyName)
		}
		payload, err = iam.ListGroupPoliciesXML(names, requestID)

	case catalog.ActionIAMPutUserPermissionsBoundary, "PutUserPermissionsBoundary":
		handled = true
		if putErr := s.store.PutUserPermissionsBoundary(accountID, params["UserName"], params["PermissionsBoundary"]); putErr != nil {
			s.writeAWSError(w, requestID, http.StatusBadRequest, "NoSuchEntity",
				"Unable to put user permissions boundary.", readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
			return nil, true, errHandled
		}
		payload, err = iam.PutUserPermissionsBoundaryXML(requestID)

	case catalog.ActionIAMGetUserPermissionsBoundary, "GetUserPermissionsBoundary":
		handled = true
		arn, getErr := s.store.GetUserPermissionsBoundary(accountID, params["UserName"])
		if getErr != nil {
			s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
				"Permissions boundary not found.", readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
			return nil, true, errHandled
		}
		payload, err = iam.GetUserPermissionsBoundaryXML(arn, requestID)

	case catalog.ActionIAMDeleteUserPermissionsBoundary, "DeleteUserPermissionsBoundary":
		handled = true
		if delErr := s.store.DeleteUserPermissionsBoundary(accountID, params["UserName"]); delErr != nil {
			s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
				"Permissions boundary not found.", readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
			return nil, true, errHandled
		}
		payload, err = iam.DeleteUserPermissionsBoundaryXML(requestID)

	case catalog.ActionIAMPutRolePermissionsBoundary, "PutRolePermissionsBoundary":
		handled = true
		if putErr := s.store.PutRolePermissionsBoundary(accountID, params["RoleName"], params["PermissionsBoundary"]); putErr != nil {
			s.writeAWSError(w, requestID, http.StatusBadRequest, "NoSuchEntity",
				"Unable to put role permissions boundary.", readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
			return nil, true, errHandled
		}
		payload, err = iam.PutRolePermissionsBoundaryXML(requestID)

	case catalog.ActionIAMGetRolePermissionsBoundary, "GetRolePermissionsBoundary":
		handled = true
		arn, getErr := s.store.GetRolePermissionsBoundary(accountID, params["RoleName"])
		if getErr != nil {
			s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
				"Permissions boundary not found.", readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
			return nil, true, errHandled
		}
		payload, err = iam.GetRolePermissionsBoundaryXML(arn, requestID)

	case catalog.ActionIAMDeleteRolePermissionsBoundary, "DeleteRolePermissionsBoundary":
		handled = true
		if delErr := s.store.DeleteRolePermissionsBoundary(accountID, params["RoleName"]); delErr != nil {
			s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
				"Permissions boundary not found.", readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
			return nil, true, errHandled
		}
		payload, err = iam.DeleteRolePermissionsBoundaryXML(requestID)

	case catalog.ActionIAMCreateInstanceProfile, "CreateInstanceProfile":
		handled = true
		name := params["InstanceProfileName"]
		arn, createErr := s.store.CreateInstanceProfile(accountID, name)
		if createErr != nil {
			if validate.IsInvalid(createErr) {
				s.writeAWSError(w, requestID, http.StatusBadRequest, "ValidationError",
					createErr.Error(), readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
				return nil, true, errHandled
			}
			s.writeAWSError(w, requestID, http.StatusConflict, "EntityAlreadyExists",
				"Instance profile already exists.", readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
			return nil, true, errHandled
		}
		payload, err = iam.CreateInstanceProfileXML(store.InstanceProfile{
			AccountID: accountID, ProfileName: name, ProfileARN: arn,
		}, requestID)

	case catalog.ActionIAMDeleteInstanceProfile, "DeleteInstanceProfile":
		handled = true
		if delErr := s.store.DeleteInstanceProfile(accountID, params["InstanceProfileName"]); delErr != nil {
			if errors.Is(delErr, sql.ErrNoRows) {
				s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
					"Instance profile not found.", readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
				return nil, true, errHandled
			}
			s.writeAWSError(w, requestID, http.StatusBadRequest, "DeleteConflict",
				"Unable to delete instance profile.", readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
			return nil, true, errHandled
		}
		payload, err = iam.DeleteInstanceProfileXML(requestID)

	case catalog.ActionIAMGetInstanceProfile, "GetInstanceProfile":
		handled = true
		p, getErr := s.store.GetInstanceProfile(accountID, params["InstanceProfileName"])
		if getErr != nil {
			s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
				"Instance profile not found.", readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
			return nil, true, errHandled
		}
		payload, err = iam.GetInstanceProfileXML(p, requestID)

	case catalog.ActionIAMAddRoleToInstanceProfile, "AddRoleToInstanceProfile":
		handled = true
		if addErr := s.store.AddRoleToInstanceProfile(accountID, params["InstanceProfileName"], params["RoleName"]); addErr != nil {
			// Store wraps GetInstanceProfile/GetRole (sql.ErrNoRows) for missing entities;
			// one-role limit returns a plain error without ErrNoRows.
			if errors.Is(addErr, sql.ErrNoRows) {
				s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
					"Instance profile or role not found.", readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
				return nil, true, errHandled
			}
			s.writeAWSError(w, requestID, http.StatusBadRequest, "LimitExceeded",
				addErr.Error(), readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
			return nil, true, errHandled
		}
		payload, err = iam.AddRoleToInstanceProfileXML(requestID)

	case catalog.ActionIAMRemoveRoleFromInstanceProfile, "RemoveRoleFromInstanceProfile":
		handled = true
		if remErr := s.store.RemoveRoleFromInstanceProfile(accountID, params["InstanceProfileName"], params["RoleName"]); remErr != nil {
			s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
				"Unable to remove role from instance profile.", readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
			return nil, true, errHandled
		}
		payload, err = iam.RemoveRoleFromInstanceProfileXML(requestID)

	case catalog.ActionIAMListInstanceProfiles, "ListInstanceProfiles":
		handled = true
		profiles, listErr := s.store.ListInstanceProfiles(accountID)
		if listErr != nil {
			s.writeAWSError(w, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to list instance profiles.", readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
			return nil, true, errHandled
		}
		payload, err = iam.ListInstanceProfilesXML(profiles, requestID)

	case catalog.ActionIAMCreateOpenIDConnectProvider, "CreateOpenIDConnectProvider":
		handled = true
		oidcURL := params["Url"]
		clientID := params["ClientIDList.member.1"]
		if clientID == "" {
			clientID = params["ClientId"]
		}
		arn, createErr := s.store.PutOIDCProvider(accountID, oidcURL, clientID)
		if createErr != nil {
			s.writeAWSError(w, requestID, http.StatusBadRequest, "ValidationError",
				createErr.Error(), readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
			return nil, true, errHandled
		}
		payload, err = iam.CreateOpenIDConnectProviderXML(arn, requestID)

	case catalog.ActionIAMDeleteOpenIDConnectProvider, "DeleteOpenIDConnectProvider":
		handled = true
		if delErr := s.store.DeleteOIDCProvider(params["OpenIDConnectProviderArn"]); delErr != nil {
			s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
				"OIDC provider not found.", readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
			return nil, true, errHandled
		}
		payload, err = iam.DeleteOpenIDConnectProviderXML(requestID)

	case catalog.ActionIAMListOpenIDConnectProviders, "ListOpenIDConnectProviders":
		handled = true
		providers, listErr := s.store.ListOIDCProviders(accountID)
		if listErr != nil {
			s.writeAWSError(w, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to list OIDC providers.", readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
			return nil, true, errHandled
		}
		payload, err = iam.ListOpenIDConnectProvidersXML(providers, requestID)

	case catalog.ActionIAMGetOpenIDConnectProvider, "GetOpenIDConnectProvider":
		handled = true
		p, getErr := s.store.GetOIDCProvider(params["OpenIDConnectProviderArn"])
		if getErr != nil {
			s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
				"OIDC provider not found.", readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
			return nil, true, errHandled
		}
		payload, err = iam.GetOpenIDConnectProviderXML(p, requestID)

	case catalog.ActionIAMCreateSAMLProvider, "CreateSAMLProvider":
		handled = true
		meta, _ := url.QueryUnescape(params["SAMLMetadataDocument"])
		if meta == "" {
			meta = params["SAMLMetadataDocument"]
		}
		arn, createErr := s.store.PutSAMLProvider(accountID, params["Name"], meta)
		if createErr != nil {
			s.writeAWSError(w, requestID, http.StatusBadRequest, "ValidationError",
				createErr.Error(), readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
			return nil, true, errHandled
		}
		payload, err = iam.CreateSAMLProviderXML(arn, requestID)

	case catalog.ActionIAMDeleteSAMLProvider, "DeleteSAMLProvider":
		handled = true
		if delErr := s.store.DeleteSAMLProvider(params["SAMLProviderArn"]); delErr != nil {
			s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
				"SAML provider not found.", readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
			return nil, true, errHandled
		}
		payload, err = iam.DeleteSAMLProviderXML(requestID)

	case catalog.ActionIAMListSAMLProviders, "ListSAMLProviders":
		handled = true
		providers, listErr := s.store.ListSAMLProviders(accountID)
		if listErr != nil {
			s.writeAWSError(w, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to list SAML providers.", readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
			return nil, true, errHandled
		}
		payload, err = iam.ListSAMLProvidersXML(providers, requestID)

	case catalog.ActionIAMGetSAMLProvider, "GetSAMLProvider":
		handled = true
		p, getErr := s.store.GetSAMLProvider(params["SAMLProviderArn"])
		if getErr != nil {
			s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
				"SAML provider not found.", readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
			return nil, true, errHandled
		}
		payload, err = iam.GetSAMLProviderXML(p, requestID)

	case catalog.ActionIAMCreateVirtualMFADevice, "CreateVirtualMFADevice":
		handled = true
		seed := make([]byte, 20)
		if _, randErr := rand.Read(seed); randErr != nil {
			s.writeAWSError(w, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to generate MFA seed.", readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
			return nil, true, errHandled
		}
		serial, createErr := s.store.CreateVirtualMFADevice(accountID, seed)
		if createErr != nil {
			s.writeAWSError(w, requestID, http.StatusBadRequest, "ValidationError",
				createErr.Error(), readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
			return nil, true, errHandled
		}
		// Lab: return hex(seed) in Base32StringSeed for deterministic TokenCode tests.
		payload, err = iam.CreateVirtualMFADeviceXML(serial, hex.EncodeToString(seed), requestID)

	case catalog.ActionIAMEnableMFADevice, "EnableMFADevice":
		handled = true
		userName := params["UserName"]
		serial := params["SerialNumber"]
		code1 := params["AuthenticationCode1"]
		code2 := params["AuthenticationCode2"]
		if userName == "" || serial == "" {
			s.writeAWSError(w, requestID, http.StatusBadRequest, "ValidationError",
				"UserName and SerialNumber are required.", readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
			return nil, true, errHandled
		}
		if code1 == "" || code2 == "" {
			s.writeAWSError(w, requestID, http.StatusBadRequest, "ValidationError",
				"AuthenticationCode1 and AuthenticationCode2 are required.", readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
			return nil, true, errHandled
		}
		dev, getDevErr := s.store.GetMFADevice(accountID, serial)
		if getDevErr != nil {
			s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
				"Unable to enable MFA device.", readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
			return nil, true, errHandled
		}
		if !sts.ValidateLabEnrollmentCodes(dev.Seed, code1, code2, time.Now().UTC()) {
			s.writeAWSError(w, requestID, http.StatusBadRequest, "InvalidAuthenticationCode",
				"Authentication codes do not match consecutive lab MFA tokens.", readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
			return nil, true, errHandled
		}
		if enErr := s.store.EnableMFADevice(accountID, serial, userName); enErr != nil {
			if errors.Is(enErr, store.ErrMFADeviceAlreadyEnabled) {
				s.writeAWSError(w, requestID, http.StatusConflict, "EntityAlreadyExists",
					"MFA device is already enabled.", readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
				return nil, true, errHandled
			}
			s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
				"Unable to enable MFA device.", readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
			return nil, true, errHandled
		}
		payload, err = iam.EnableMFADeviceXML(requestID)

	case catalog.ActionIAMListMFADevices, "ListMFADevices":
		handled = true
		devices, listErr := s.store.ListMFADevices(accountID, params["UserName"])
		if listErr != nil {
			s.writeAWSError(w, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to list MFA devices.", readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
			return nil, true, errHandled
		}
		payload, err = iam.ListMFADevicesXML(devices, requestID)

	case catalog.ActionIAMDeactivateMFADevice, "DeactivateMFADevice":
		handled = true
		userName := params["UserName"]
		serial := params["SerialNumber"]
		if userName == "" || serial == "" {
			s.writeAWSError(w, requestID, http.StatusBadRequest, "ValidationError",
				"UserName and SerialNumber are required.", readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
			return nil, true, errHandled
		}
		if deErr := s.store.DeactivateMFADevice(accountID, serial, userName); deErr != nil {
			s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
				"MFA device not found.", readOnly, r, eventID, verifiedAccessKeyID, accountID, true)
			return nil, true, errHandled
		}
		payload, err = iam.DeactivateMFADeviceXML(requestID)

	default:
		return nil, false, nil
	}

	return payload, handled, err
}

// errHandled signals that handleIAMIdentity already wrote an error response.
var errHandled = errors.New("iam t2 handler wrote response")
