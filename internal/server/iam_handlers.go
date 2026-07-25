package server

import (
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/services/iam"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
	"github.com/Kyaxris-Labs/Noctaxris/internal/validate"
)

func (s *Server) handleIAM(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	// AWS: GetSessionToken credentials cannot call IAM unless MFA was used to mint them.
	if s.getSessionTokenSessionBlocksIAM(verified) {
		s.writeAWSError(w, requestID, http.StatusForbidden, "AccessDenied",
			"Temporary credentials from GetSessionToken cannot call IAM without MFA.", readOnly, r, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}
	params := requestParams(r, body)
	accountID := verified.AccountID
	resource := s.iamRequestResource(accountID, action, params, verified)
	if !s.authorize(verified, action, resource) {
		s.writeAWSError(w, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform "+action+".", readOnly, r, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}

	var (
		payload []byte
		err     error
	)

	switch action {
	case catalog.ActionIAMCreateUser, "CreateUser":
		userName := params["UserName"]
		if userName == "" {
			s.writeAWSError(w, requestID, http.StatusBadRequest, "ValidationError",
				"UserName is required.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
		userID, arn, err := s.store.CreateUser(accountID, userName)
		if err != nil {
			if validate.IsInvalid(err) {
				s.writeAWSError(w, requestID, http.StatusBadRequest, "ValidationError",
					err.Error(), readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
				return
			}
			s.writeAWSError(w, requestID, http.StatusConflict, "EntityAlreadyExists",
				"User already exists.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
		u := store.User{AccountID: accountID, UserName: userName, UserID: userID, ARN: arn}
		if got, getErr := s.store.GetUser(accountID, userName); getErr == nil {
			u = got
		}
		payload, err = iam.CreateUserXML(u, requestID)
	case catalog.ActionIAMGetUser, "GetUser":
		userName := params["UserName"]
		if userName == "" && verified.Principal.UserName != "" {
			userName = verified.Principal.UserName
		}
		if userName == "" {
			s.writeAWSError(w, requestID, http.StatusBadRequest, "ValidationError",
				"UserName is required.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
		u, err := s.store.GetUser(accountID, userName)
		if err != nil {
			s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
				"User not found.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
		boundaryARN := ""
		if arn, bErr := s.store.GetUserPermissionsBoundary(accountID, userName); bErr == nil {
			boundaryARN = arn
		}
		payload, err = iam.GetUserXML(u, boundaryARN, requestID)
	case catalog.ActionIAMListUsers, "ListUsers":
		users, listErr := s.store.ListUsers(accountID)
		if listErr != nil {
			s.writeAWSError(w, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to list users.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
		payload, err = iam.ListUsersXML(users, requestID)
	case catalog.ActionIAMDeleteUser, "DeleteUser":
		userName := params["UserName"]
		if err := s.store.DeleteUser(accountID, userName); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
					"User not found.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
				return
			}
			s.writeAWSError(w, requestID, http.StatusBadRequest, "DeleteConflict",
				"Unable to delete user.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
		payload, err = iam.DeleteUserXML(requestID)
	case catalog.ActionIAMCreateAccessKey, "CreateAccessKey":
		userName := params["UserName"]
		if userName == "" {
			userName = verified.Principal.UserName
		}
		keyID, secret, createErr := s.store.CreateUserAccessKey(accountID, userName)
		if createErr != nil {
			s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
				"Unable to create access key.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
		payload, err = iam.CreateAccessKeyXML(userName, keyID, secret, requestID)
	case catalog.ActionIAMDeleteAccessKey, "DeleteAccessKey":
		keyID := strings.TrimSpace(params["AccessKeyId"])
		if ak, lookupErr := s.store.LookupAccessKeyRecord(keyID); lookupErr == nil {
			if ak.IsRoot {
				s.writeAWSError(w, requestID, http.StatusForbidden, "AccessDenied",
					"Cannot delete root access keys.", readOnly, r, eventID,
					verified.AccessKeyID, verified.AccountID, true)
				return
			}
			if accessKeyUserNameMismatch(params["UserName"], ak) {
				s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
					"Access key not found.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
				return
			}
		}
		if err := s.store.DeleteAccessKeyInAccount(accountID, keyID); err != nil {
			s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
				"Access key not found.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
		payload, err = iam.DeleteAccessKeyXML(requestID)
	case catalog.ActionIAMListAccessKeys, "ListAccessKeys":
		userName := params["UserName"]
		if userName == "" {
			userName = verified.Principal.UserName
		}
		keys, listErr := s.store.ListAccessKeys(accountID, userName)
		if listErr != nil {
			s.writeAWSError(w, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to list access keys.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
		payload, err = iam.ListAccessKeysXML(keys, requestID)
	case catalog.ActionIAMUpdateAccessKey, "UpdateAccessKey":
		keyID := strings.TrimSpace(params["AccessKeyId"])
		if ak, lookupErr := s.store.LookupAccessKeyRecord(keyID); lookupErr == nil {
			if ak.IsRoot {
				s.writeAWSError(w, requestID, http.StatusForbidden, "AccessDenied",
					"Cannot update root access keys.", readOnly, r, eventID,
					verified.AccessKeyID, verified.AccountID, true)
				return
			}
			if accessKeyUserNameMismatch(params["UserName"], ak) {
				s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
					"Access key not found.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
				return
			}
		}
		if err := s.store.UpdateAccessKeyInAccount(accountID, keyID, params["Status"]); err != nil {
			s.writeAWSError(w, requestID, http.StatusBadRequest, "ValidationError",
				"Unable to update access key.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
		payload, err = iam.UpdateAccessKeyXML(requestID)
	case catalog.ActionIAMGetAccessKeyLastUsed, "GetAccessKeyLastUsed":
		keyID := strings.TrimSpace(params["AccessKeyId"])
		if keyID == "" {
			s.writeAWSError(w, requestID, http.StatusBadRequest, "ValidationError",
				"AccessKeyId is required.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
		if ak, lookupErr := s.store.LookupAccessKeyRecord(keyID); lookupErr == nil {
			if accessKeyUserNameMismatch(params["UserName"], ak) {
				s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
					"The specified access key does not exist.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
				return
			}
		}
		usage, getErr := s.store.GetAccessKeyLastUsed(accountID, keyID)
		if getErr != nil {
			if errors.Is(getErr, sql.ErrNoRows) {
				s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
					"The specified access key does not exist.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
				return
			}
			s.writeAWSError(w, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to get access key last used.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
		payload, err = iam.GetAccessKeyLastUsedXML(usage, requestID)
	case catalog.ActionIAMGenerateCredentialReport, "GenerateCredentialReport":
		if genErr := s.store.GenerateCredentialReport(accountID, s.now().UTC()); genErr != nil {
			s.writeAWSError(w, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to generate credential report.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
		payload, err = iam.GenerateCredentialReportXML("COMPLETE", requestID)
	case catalog.ActionIAMGetCredentialReport, "GetCredentialReport":
		csv, generated, state, getErr := s.store.GetCredentialReport(accountID)
		if getErr != nil {
			if errors.Is(getErr, store.ErrCredentialReportNotPresent) {
				s.writeAWSError(w, requestID, http.StatusNotFound, "ReportNotPresent",
					"Credential report does not exist.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
				return
			}
			s.writeAWSError(w, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to get credential report.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
		payload, err = iam.GetCredentialReportXML(csv, generated, state, requestID)
	case catalog.ActionIAMCreatePolicy, "CreatePolicy":
		doc := params["PolicyDocument"]
		name := params["PolicyName"]
		arn, createErr := s.store.CreateManagedPolicy(accountID, name, doc)
		if createErr != nil {
			code := "MalformedPolicyDocument"
			if validate.IsInvalid(createErr) && !strings.Contains(createErr.Error(), "PolicyDocument") {
				code = "ValidationError"
			}
			s.writeAWSError(w, requestID, http.StatusBadRequest, code,
				createErr.Error(), readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
		p, getErr := s.store.GetManagedPolicy(arn)
		if getErr != nil {
			s.writeAWSError(w, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to load created policy.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
		payload, err = iam.CreatePolicyXML(p, requestID)
	case catalog.ActionIAMGetPolicy, "GetPolicy":
		p, getErr := s.store.GetManagedPolicy(params["PolicyArn"])
		if getErr != nil {
			s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
				"Policy not found.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
		payload, err = iam.GetPolicyXML(p, requestID)
	case catalog.ActionIAMListPolicies, "ListPolicies":
		policies, listErr := s.store.ListManagedPolicies(accountID)
		if listErr != nil {
			s.writeAWSError(w, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to list policies.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
		payload, err = iam.ListPoliciesXML(policies, requestID)
	case catalog.ActionIAMDeletePolicy, "DeletePolicy":
		if err := s.store.DeleteManagedPolicy(params["PolicyArn"]); err != nil {
			s.writeAWSError(w, requestID, http.StatusBadRequest, "DeleteConflict",
				"Unable to delete policy.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
		payload, err = iam.DeletePolicyXML(requestID)
	case catalog.ActionIAMCreatePolicyVersion, "CreatePolicyVersion":
		setDefault := strings.EqualFold(params["SetAsDefault"], "true")
		v, createErr := s.store.CreateManagedPolicyVersion(params["PolicyArn"], params["PolicyDocument"], setDefault)
		if createErr != nil {
			switch {
			case errors.Is(createErr, sql.ErrNoRows):
				s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
					"Policy not found.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			case errors.Is(createErr, store.ErrPolicyVersionLimit):
				s.writeAWSError(w, requestID, http.StatusBadRequest, "LimitExceeded",
					createErr.Error(), readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			case validate.IsInvalid(createErr):
				code := "MalformedPolicyDocument"
				if !strings.Contains(createErr.Error(), "PolicyDocument") {
					code = "ValidationError"
				}
				s.writeAWSError(w, requestID, http.StatusBadRequest, code,
					createErr.Error(), readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			default:
				s.writeAWSError(w, requestID, http.StatusInternalServerError, "InternalFailure",
					"Unable to create policy version.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			}
			return
		}
		payload, err = iam.CreatePolicyVersionXML(v, requestID)
	case catalog.ActionIAMGetPolicyVersion, "GetPolicyVersion":
		v, getErr := s.store.GetManagedPolicyVersion(params["PolicyArn"], params["VersionId"])
		if getErr != nil {
			if errors.Is(getErr, store.ErrNoSuchPolicyVersion) || errors.Is(getErr, sql.ErrNoRows) {
				s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
					"Policy version not found.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
				return
			}
			s.writeAWSError(w, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to get policy version.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
		payload, err = iam.GetPolicyVersionXML(v, requestID)
	case catalog.ActionIAMListPolicyVersions, "ListPolicyVersions":
		versions, listErr := s.store.ListManagedPolicyVersions(params["PolicyArn"])
		if listErr != nil {
			if errors.Is(listErr, sql.ErrNoRows) {
				s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
					"Policy not found.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
				return
			}
			s.writeAWSError(w, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to list policy versions.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
		payload, err = iam.ListPolicyVersionsXML(versions, requestID)
	case catalog.ActionIAMDeletePolicyVersion, "DeletePolicyVersion":
		if delErr := s.store.DeleteManagedPolicyVersion(params["PolicyArn"], params["VersionId"]); delErr != nil {
			switch {
			case errors.Is(delErr, store.ErrDeleteDefaultPolicyVersion):
				s.writeAWSError(w, requestID, http.StatusBadRequest, "DeleteConflict",
					delErr.Error(), readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			case errors.Is(delErr, store.ErrNoSuchPolicyVersion), errors.Is(delErr, sql.ErrNoRows):
				s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
					"Policy version not found.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			default:
				s.writeAWSError(w, requestID, http.StatusBadRequest, "DeleteConflict",
					"Unable to delete policy version.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			}
			return
		}
		payload, err = iam.DeletePolicyVersionXML(requestID)
	case catalog.ActionIAMSetDefaultPolicyVersion, "SetDefaultPolicyVersion":
		if setErr := s.store.SetDefaultManagedPolicyVersion(params["PolicyArn"], params["VersionId"]); setErr != nil {
			switch {
			case errors.Is(setErr, store.ErrNoSuchPolicyVersion):
				s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
					"Policy version not found.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			case errors.Is(setErr, sql.ErrNoRows):
				s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
					"Policy not found.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			default:
				s.writeAWSError(w, requestID, http.StatusBadRequest, "ValidationError",
					setErr.Error(), readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			}
			return
		}
		payload, err = iam.SetDefaultPolicyVersionXML(requestID)
	case catalog.ActionIAMAttachUserPolicy, "AttachUserPolicy":
		if err := s.store.AttachUserPolicy(accountID, params["UserName"], params["PolicyArn"]); err != nil {
			s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
				"Unable to attach user policy.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
		payload, err = iam.AttachUserPolicyXML(requestID)
	case catalog.ActionIAMDetachUserPolicy, "DetachUserPolicy":
		if err := s.store.DetachUserPolicy(accountID, params["UserName"], params["PolicyArn"]); err != nil {
			s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
				"Unable to detach user policy.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
		payload, err = iam.DetachUserPolicyXML(requestID)
	case catalog.ActionIAMAttachRolePolicy, "AttachRolePolicy":
		if err := s.store.AttachRolePolicy(accountID, params["RoleName"], params["PolicyArn"]); err != nil {
			s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
				"Unable to attach role policy.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
		payload, err = iam.AttachRolePolicyXML(requestID)
	case catalog.ActionIAMDetachRolePolicy, "DetachRolePolicy":
		if err := s.store.DetachRolePolicy(accountID, params["RoleName"], params["PolicyArn"]); err != nil {
			s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
				"Unable to detach role policy.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
		payload, err = iam.DetachRolePolicyXML(requestID)
	case catalog.ActionIAMListAttachedUserPolicies, "ListAttachedUserPolicies":
		u, getErr := s.store.GetUser(accountID, params["UserName"])
		if getErr != nil {
			s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
				"User not found.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
		refs, listErr := s.store.ListAttachedPolicyRefs(u.ARN)
		if listErr != nil {
			s.writeAWSError(w, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to list attached policies.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
		payload, err = iam.ListAttachedUserPoliciesXML(refs, requestID)
	case catalog.ActionIAMListAttachedRolePolicies, "ListAttachedRolePolicies":
		roleARN, _, getErr := s.store.GetRole(accountID, params["RoleName"])
		if getErr != nil {
			s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
				"Role not found.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
		refs, listErr := s.store.ListAttachedPolicyRefs(roleARN)
		if listErr != nil {
			s.writeAWSError(w, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to list attached policies.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
		payload, err = iam.ListAttachedRolePoliciesXML(refs, requestID)
	case catalog.ActionIAMPutUserPolicy, "PutUserPolicy":
		u, getErr := s.store.GetUser(accountID, params["UserName"])
		if getErr != nil {
			s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
				"User not found.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
		doc, _ := url.QueryUnescape(params["PolicyDocument"])
		if doc == "" {
			doc = params["PolicyDocument"]
		}
		if err := s.store.PutInlinePolicy(u.ARN, params["PolicyName"], doc); err != nil {
			s.writeAWSError(w, requestID, http.StatusBadRequest, "MalformedPolicyDocument",
				"Unable to put user policy.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
		payload, err = iam.PutUserPolicyXML(requestID)
	case catalog.ActionIAMGetUserPolicy, "GetUserPolicy":
		u, getErr := s.store.GetUser(accountID, params["UserName"])
		if getErr != nil {
			s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
				"User not found.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
		p, getErr := s.store.GetInlinePolicy(u.ARN, params["PolicyName"])
		if getErr != nil {
			s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
				"Policy not found.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
		payload, err = iam.GetUserPolicyXML(params["UserName"], params["PolicyName"], p.Document, requestID)
	case catalog.ActionIAMDeleteUserPolicy, "DeleteUserPolicy":
		u, getErr := s.store.GetUser(accountID, params["UserName"])
		if getErr != nil {
			s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
				"User not found.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
		if err := s.store.DeleteInlinePolicy(u.ARN, params["PolicyName"]); err != nil {
			s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
				"Policy not found.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
		payload, err = iam.DeleteUserPolicyXML(requestID)
	case catalog.ActionIAMListUserPolicies, "ListUserPolicies":
		u, getErr := s.store.GetUser(accountID, params["UserName"])
		if getErr != nil {
			s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
				"User not found.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
		pols, listErr := s.store.ListInlinePolicies(u.ARN)
		if listErr != nil {
			s.writeAWSError(w, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to list user policies.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
		names := make([]string, 0, len(pols))
		for _, p := range pols {
			names = append(names, p.PolicyName)
		}
		payload, err = iam.ListUserPoliciesXML(names, requestID)
	case catalog.ActionIAMPutRolePolicy, "PutRolePolicy":
		roleARN, _, getErr := s.store.GetRole(accountID, params["RoleName"])
		if getErr != nil {
			s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
				"Role not found.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
		doc, _ := url.QueryUnescape(params["PolicyDocument"])
		if doc == "" {
			doc = params["PolicyDocument"]
		}
		if err := s.store.PutInlinePolicy(roleARN, params["PolicyName"], doc); err != nil {
			s.writeAWSError(w, requestID, http.StatusBadRequest, "MalformedPolicyDocument",
				"Unable to put role policy.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
		payload, err = iam.PutRolePolicyXML(requestID)
	case catalog.ActionIAMGetRolePolicy, "GetRolePolicy":
		roleARN, _, getErr := s.store.GetRole(accountID, params["RoleName"])
		if getErr != nil {
			s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
				"Role not found.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
		p, getErr := s.store.GetInlinePolicy(roleARN, params["PolicyName"])
		if getErr != nil {
			s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
				"Policy not found.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
		payload, err = iam.GetRolePolicyXML(params["RoleName"], params["PolicyName"], p.Document, requestID)
	case catalog.ActionIAMDeleteRolePolicy, "DeleteRolePolicy":
		roleARN, _, getErr := s.store.GetRole(accountID, params["RoleName"])
		if getErr != nil {
			s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
				"Role not found.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
		if err := s.store.DeleteInlinePolicy(roleARN, params["PolicyName"]); err != nil {
			s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
				"Policy not found.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
		payload, err = iam.DeleteRolePolicyXML(requestID)
	case catalog.ActionIAMListRolePolicies, "ListRolePolicies":
		roleARN, _, getErr := s.store.GetRole(accountID, params["RoleName"])
		if getErr != nil {
			s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
				"Role not found.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
		pols, listErr := s.store.ListInlinePolicies(roleARN)
		if listErr != nil {
			s.writeAWSError(w, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to list role policies.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
		names := make([]string, 0, len(pols))
		for _, p := range pols {
			names = append(names, p.PolicyName)
		}
		payload, err = iam.ListRolePoliciesXML(names, requestID)
	case catalog.ActionIAMCreateRole, "CreateRole":
		roleName := params["RoleName"]
		trust, _ := url.QueryUnescape(params["AssumeRolePolicyDocument"])
		if trust == "" {
			trust = params["AssumeRolePolicyDocument"]
		}
		maxSession := 0
		if raw := strings.TrimSpace(params["MaxSessionDuration"]); raw != "" {
			secs, parseErr := strconv.Atoi(raw)
			if parseErr != nil {
				s.writeAWSError(w, requestID, http.StatusBadRequest, "ValidationError",
					"MaxSessionDuration must be an integer between 3600 and 43200.", readOnly, r, eventID,
					verified.AccessKeyID, verified.AccountID, true)
				return
			}
			maxSession = secs
		}
		roleARN, createErr := s.store.CreateRoleOpts(accountID, roleName, trust, maxSession)
		if createErr != nil {
			if validate.IsInvalid(createErr) || strings.Contains(createErr.Error(), "MaxSessionDuration") {
				code := "ValidationError"
				if strings.Contains(createErr.Error(), "PolicyDocument") {
					code = "MalformedPolicyDocument"
				}
				s.writeAWSError(w, requestID, http.StatusBadRequest, code,
					createErr.Error(), readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
				return
			}
			s.writeAWSError(w, requestID, http.StatusConflict, "EntityAlreadyExists",
				"Role already exists.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
		rec, getErr := s.store.GetRoleRecord(accountID, roleName)
		if getErr != nil {
			rec = store.Role{AccountID: accountID, RoleName: roleName, RoleARN: roleARN, TrustPolicy: trust}
		}
		payload, err = iam.CreateRoleXML(rec, requestID)
	case catalog.ActionIAMGetRole, "GetRole":
		rec, getErr := s.store.GetRoleRecord(accountID, params["RoleName"])
		if getErr != nil {
			s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
				"Role not found.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
		boundaryARN := ""
		if arn, bErr := s.store.GetRolePermissionsBoundary(accountID, params["RoleName"]); bErr == nil {
			boundaryARN = arn
		}
		payload, err = iam.GetRoleXML(rec, boundaryARN, requestID)
	case catalog.ActionIAMListRoles, "ListRoles":
		roles, listErr := s.store.ListRoles(accountID)
		if listErr != nil {
			s.writeAWSError(w, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to list roles.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
		payload, err = iam.ListRolesXML(roles, requestID)
	case catalog.ActionIAMDeleteRole, "DeleteRole":
		if err := s.store.DeleteRole(accountID, params["RoleName"]); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
					"Role not found.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
				return
			}
			s.writeAWSError(w, requestID, http.StatusBadRequest, "DeleteConflict",
				"Unable to delete role.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
		payload, err = iam.DeleteRoleXML(requestID)
	case catalog.ActionIAMUpdateAssumeRolePolicy, "UpdateAssumeRolePolicy":
		trust, _ := url.QueryUnescape(params["PolicyDocument"])
		if trust == "" {
			trust = params["PolicyDocument"]
		}
		if err := s.store.UpdateAssumeRolePolicy(accountID, params["RoleName"], trust); err != nil {
			s.writeAWSError(w, requestID, http.StatusNotFound, "NoSuchEntity",
				"Role not found.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
			return
		}
		payload, err = iam.UpdateAssumeRolePolicyXML(requestID)
	default:
		identityPayload, identityHandled, identityErr := s.handleIAMIdentity(w, r, params, requestID, eventID, action, accountID, verified.AccessKeyID, readOnly)
		if identityErr != nil {
			if errors.Is(identityErr, errHandled) {
				return
			}
			s.writeAWSError(w, requestID, http.StatusBadRequest, "ValidationError", identityErr.Error(), readOnly, r, eventID,
				verified.AccessKeyID, verified.AccountID, true)
			return
		}
		if identityHandled {
			s.writeXMLOK(w, requestID, identityPayload)
			eventName := action
			if i := strings.Index(action, ":"); i >= 0 {
				eventName = action[i+1:]
			}
			s.writeSuccessAudit(r, requestID, eventID, verified, "iam.amazonaws.com", eventName, readOnly)
			return
		}
		s.writeAWSError(w, requestID, http.StatusNotImplemented, "NotImplemented",
			"This API action is not implemented in Noctaxris.", readOnly, r, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}

	if err != nil {
		s.writeAWSError(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build IAM response.", readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
		return
	}
	s.writeXMLOK(w, requestID, payload)
	eventName := action
	if i := strings.Index(action, ":"); i >= 0 {
		eventName = action[i+1:]
	}
	s.writeSuccessAudit(r, requestID, eventID, verified, "iam.amazonaws.com", eventName, readOnly)
}

// iamRequestResource returns the AWS-shaped IAM resource ARN for authorize.
// List-all and unknown create shapes fall back to "*".
func (s *Server) iamRequestResource(accountID, action string, params map[string]string, verified *authn.Verified) string {
	userName := strings.TrimSpace(params["UserName"])
	roleName := strings.TrimSpace(params["RoleName"])
	groupName := strings.TrimSpace(params["GroupName"])
	policyArn := strings.TrimSpace(params["PolicyArn"])
	profileName := strings.TrimSpace(params["InstanceProfileName"])
	accessKeyID := strings.TrimSpace(params["AccessKeyId"])

	switch action {
	case catalog.ActionIAMCreateUser, "CreateUser",
		catalog.ActionIAMGetUser, "GetUser",
		catalog.ActionIAMDeleteUser, "DeleteUser",
		catalog.ActionIAMAttachUserPolicy, "AttachUserPolicy",
		catalog.ActionIAMDetachUserPolicy, "DetachUserPolicy",
		catalog.ActionIAMListAttachedUserPolicies, "ListAttachedUserPolicies",
		catalog.ActionIAMPutUserPolicy, "PutUserPolicy",
		catalog.ActionIAMGetUserPolicy, "GetUserPolicy",
		catalog.ActionIAMDeleteUserPolicy, "DeleteUserPolicy",
		catalog.ActionIAMListUserPolicies, "ListUserPolicies",
		catalog.ActionIAMPutUserPermissionsBoundary, "PutUserPermissionsBoundary",
		catalog.ActionIAMGetUserPermissionsBoundary, "GetUserPermissionsBoundary",
		catalog.ActionIAMDeleteUserPermissionsBoundary, "DeleteUserPermissionsBoundary",
		catalog.ActionIAMListMFADevices, "ListMFADevices":
		if userName == "" && verified != nil {
			userName = verified.Principal.UserName
		}
		if userName != "" {
			return store.UserARN(accountID, "/", userName)
		}
	case catalog.ActionIAMCreateAccessKey, "CreateAccessKey",
		catalog.ActionIAMListAccessKeys, "ListAccessKeys":
		if userName == "" && verified != nil {
			userName = verified.Principal.UserName
		}
		if userName != "" {
			return store.UserARN(accountID, "/", userName)
		}
	case catalog.ActionIAMDeleteAccessKey, "DeleteAccessKey",
		catalog.ActionIAMUpdateAccessKey, "UpdateAccessKey",
		catalog.ActionIAMGetAccessKeyLastUsed, "GetAccessKeyLastUsed":
		// Resource is always the key owner from AccessKeyId. Request UserName is
		// only an optional consistency check in the handler (mismatch -> NoSuchEntity).
		if accessKeyID != "" {
			if ak, err := s.store.LookupAccessKeyRecord(accessKeyID); err == nil {
				if ak.UserName != "" {
					return store.UserARN(ak.AccountID, "/", ak.UserName)
				}
				if ak.IsRoot {
					return "arn:aws:iam::" + ak.AccountID + ":root"
				}
			}
			return "arn:aws:iam::" + accountID + ":access-key/" + accessKeyID
		}
	case catalog.ActionIAMCreateRole, "CreateRole",
		catalog.ActionIAMGetRole, "GetRole",
		catalog.ActionIAMDeleteRole, "DeleteRole",
		catalog.ActionIAMAttachRolePolicy, "AttachRolePolicy",
		catalog.ActionIAMDetachRolePolicy, "DetachRolePolicy",
		catalog.ActionIAMListAttachedRolePolicies, "ListAttachedRolePolicies",
		catalog.ActionIAMPutRolePolicy, "PutRolePolicy",
		catalog.ActionIAMGetRolePolicy, "GetRolePolicy",
		catalog.ActionIAMDeleteRolePolicy, "DeleteRolePolicy",
		catalog.ActionIAMListRolePolicies, "ListRolePolicies",
		catalog.ActionIAMUpdateAssumeRolePolicy, "UpdateAssumeRolePolicy",
		catalog.ActionIAMPutRolePermissionsBoundary, "PutRolePermissionsBoundary",
		catalog.ActionIAMGetRolePermissionsBoundary, "GetRolePermissionsBoundary",
		catalog.ActionIAMDeleteRolePermissionsBoundary, "DeleteRolePermissionsBoundary",
		catalog.ActionIAMListInstanceProfilesForRole, "ListInstanceProfilesForRole":
		if roleName != "" {
			return store.RoleARN(accountID, roleName)
		}
	case catalog.ActionIAMCreateGroup, "CreateGroup",
		catalog.ActionIAMDeleteGroup, "DeleteGroup",
		catalog.ActionIAMGetGroup, "GetGroup",
		catalog.ActionIAMAddUserToGroup, "AddUserToGroup",
		catalog.ActionIAMRemoveUserFromGroup, "RemoveUserFromGroup",
		catalog.ActionIAMAttachGroupPolicy, "AttachGroupPolicy",
		catalog.ActionIAMDetachGroupPolicy, "DetachGroupPolicy",
		catalog.ActionIAMListAttachedGroupPolicies, "ListAttachedGroupPolicies",
		catalog.ActionIAMPutGroupPolicy, "PutGroupPolicy",
		catalog.ActionIAMGetGroupPolicy, "GetGroupPolicy",
		catalog.ActionIAMDeleteGroupPolicy, "DeleteGroupPolicy",
		catalog.ActionIAMListGroupPolicies, "ListGroupPolicies":
		if groupName != "" {
			return store.GroupARN(accountID, "/", groupName)
		}
	case catalog.ActionIAMCreatePolicy, "CreatePolicy":
		if name := strings.TrimSpace(params["PolicyName"]); name != "" {
			return store.PolicyARN(accountID, "/", name)
		}
	case catalog.ActionIAMGetPolicy, "GetPolicy",
		catalog.ActionIAMDeletePolicy, "DeletePolicy",
		catalog.ActionIAMCreatePolicyVersion, "CreatePolicyVersion",
		catalog.ActionIAMGetPolicyVersion, "GetPolicyVersion",
		catalog.ActionIAMListPolicyVersions, "ListPolicyVersions",
		catalog.ActionIAMDeletePolicyVersion, "DeletePolicyVersion",
		catalog.ActionIAMSetDefaultPolicyVersion, "SetDefaultPolicyVersion":
		if policyArn != "" {
			return policyArn
		}
	case catalog.ActionIAMCreateInstanceProfile, "CreateInstanceProfile",
		catalog.ActionIAMDeleteInstanceProfile, "DeleteInstanceProfile",
		catalog.ActionIAMGetInstanceProfile, "GetInstanceProfile",
		catalog.ActionIAMAddRoleToInstanceProfile, "AddRoleToInstanceProfile",
		catalog.ActionIAMRemoveRoleFromInstanceProfile, "RemoveRoleFromInstanceProfile":
		if profileName != "" {
			return store.InstanceProfileARN(accountID, "/", profileName)
		}
	case catalog.ActionIAMCreateOpenIDConnectProvider, "CreateOpenIDConnectProvider":
		if u := strings.TrimSpace(params["Url"]); u != "" {
			return store.OIDCProviderARN(accountID, u)
		}
	case catalog.ActionIAMDeleteOpenIDConnectProvider, "DeleteOpenIDConnectProvider",
		catalog.ActionIAMGetOpenIDConnectProvider, "GetOpenIDConnectProvider":
		if arn := strings.TrimSpace(params["OpenIDConnectProviderArn"]); arn != "" {
			return arn
		}
	case catalog.ActionIAMCreateSAMLProvider, "CreateSAMLProvider":
		if name := strings.TrimSpace(params["Name"]); name != "" {
			return store.SAMLProviderARN(accountID, name)
		}
	case catalog.ActionIAMDeleteSAMLProvider, "DeleteSAMLProvider",
		catalog.ActionIAMGetSAMLProvider, "GetSAMLProvider":
		if arn := strings.TrimSpace(params["SAMLProviderArn"]); arn != "" {
			return arn
		}
	case catalog.ActionIAMEnableMFADevice, "EnableMFADevice",
		catalog.ActionIAMDeactivateMFADevice, "DeactivateMFADevice":
		if serial := strings.TrimSpace(params["SerialNumber"]); serial != "" {
			if strings.HasPrefix(serial, "arn:") {
				return serial
			}
			return store.MFADeviceARN(accountID, serial)
		}
		if userName != "" {
			return store.UserARN(accountID, "/", userName)
		}
	}
	return "*"
}

// accessKeyUserNameMismatch reports whether optional request UserName disagrees
// with the access-key owner. Empty UserName is ignored.
func accessKeyUserNameMismatch(requestedUserName string, ak store.AccessKey) bool {
	requestedUserName = strings.TrimSpace(requestedUserName)
	if requestedUserName == "" {
		return false
	}
	return ak.UserName != requestedUserName
}
