package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/services/organizations"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
	"github.com/Kyaxris-Labs/Noctaxris/internal/validate"
)

const labOrgID = "o-noctaxris"

// handleOrgsDepth dispatches Organizations depth APIs (ListAccounts, OUs, SCP/RCP).
func (s *Server) handleOrgsDepth(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	switch action {
	case catalog.ActionOrgsListAccounts, "ListAccounts":
		s.handleListAccounts(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionOrgsCreateOrganizationalUnit, "CreateOrganizationalUnit":
		s.handleCreateOrganizationalUnit(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionOrgsListOrganizationalUnitsForParent, "ListOrganizationalUnitsForParent":
		s.handleListOrganizationalUnitsForParent(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionOrgsEnablePolicyType, "EnablePolicyType":
		s.handleEnablePolicyType(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionOrgsCreatePolicy, catalog.ActionIAMCreatePolicy, "CreatePolicy":
		s.handleOrgsCreatePolicy(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionOrgsAttachPolicy, "AttachPolicy":
		s.handleOrgsAttachPolicy(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionOrgsDetachPolicy, "DetachPolicy":
		s.handleOrgsDetachPolicy(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionOrgsDescribePolicy, "DescribePolicy":
		s.handleOrgsDescribePolicy(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionOrgsMoveAccount, "MoveAccount":
		s.handleOrgsMoveAccount(w, r, body, requestID, eventID, verified, readOnly)
	default:
		s.writeAPIError(w, r, body, requestID, http.StatusBadRequest, "InvalidAction",
			"Unsupported Organizations action.", readOnly, eventID,
			verified.AccessKeyID, verified.AccountID, true)
	}
}

func (s *Server) handleListAccounts(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
) {
	if !s.authorizeOrgs(verified, catalog.ActionOrgsListAccounts, "*") {
		s.writeAPIError(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform organizations:ListAccounts.", readOnly, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}

	accounts, err := s.store.ListAccounts()
	if err != nil {
		s.writeAPIError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list accounts.", readOnly, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}

	ids := make([]string, 0, len(accounts))
	jsonAccounts := make([]map[string]any, 0, len(accounts))
	for _, a := range accounts {
		ids = append(ids, a.AccountID)
		jsonAccounts = append(jsonAccounts, map[string]any{
			"Id":     a.AccountID,
			"Status": "ACTIVE",
			"State":  "ACTIVE",
		})
	}

	if wantsJSON(r, body) {
		payload, err := json.Marshal(map[string]any{"Accounts": jsonAccounts})
		if err != nil {
			s.writeAPIError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to build ListAccounts response.", readOnly, eventID,
				verified.AccessKeyID, verified.AccountID, true)
			return
		}
		s.writeJSONOK(w, requestID, payload)
	} else {
		payload, err := organizations.ListAccountsXML(organizations.ListAccountsResult{
			RequestID:  requestID,
			AccountIDs: ids,
		})
		if err != nil {
			s.writeAPIError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to build ListAccounts response.", readOnly, eventID,
				verified.AccessKeyID, verified.AccountID, true)
			return
		}
		s.writeXMLOK(w, requestID, payload)
	}
	s.writeSuccessAudit(r, requestID, eventID, verified, "organizations.amazonaws.com", "ListAccounts", true)
}

func (s *Server) handleCreateOrganizationalUnit(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
) {
	if !s.authorizeOrgs(verified, catalog.ActionOrgsCreateOrganizationalUnit, "*") {
		s.writeAPIError(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform organizations:CreateOrganizationalUnit.", readOnly, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}

	params := requestParams(r, body)
	parentID := params["ParentId"]
	name := params["Name"]
	if parentID == "" || name == "" {
		s.writeAPIError(w, r, body, requestID, http.StatusBadRequest, "ValidationError",
			"ParentId and Name are required.", readOnly, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}
	if parentID != store.OrgRootID && !strings.HasPrefix(parentID, "ou-") {
		s.writeAPIError(w, r, body, requestID, http.StatusBadRequest, "ValidationError",
			"ParentId must be the lab root or an OU id.", readOnly, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}

	ouID, err := s.store.CreateOrganizationalUnit(parentID, name)
	if err != nil {
		s.writeAPIError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create organizational unit.", readOnly, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}
	arn := orgOUARN(verified.AccountID, ouID)

	if wantsJSON(r, body) {
		payload, err := json.Marshal(map[string]any{
			"OrganizationalUnit": map[string]any{
				"Id":   ouID,
				"Name": name,
				"Arn":  arn,
			},
		})
		if err != nil {
			s.writeAPIError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to build CreateOrganizationalUnit response.", readOnly, eventID,
				verified.AccessKeyID, verified.AccountID, true)
			return
		}
		s.writeJSONOK(w, requestID, payload)
	} else {
		payload, err := organizations.CreateOrganizationalUnitXML(organizations.CreateOrganizationalUnitResult{
			RequestID: requestID,
			ID:        ouID,
			Name:      name,
			Arn:       arn,
		})
		if err != nil {
			s.writeAPIError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to build CreateOrganizationalUnit response.", readOnly, eventID,
				verified.AccessKeyID, verified.AccountID, true)
			return
		}
		s.writeXMLOK(w, requestID, payload)
	}
	s.writeSuccessAudit(r, requestID, eventID, verified, "organizations.amazonaws.com", "CreateOrganizationalUnit", false)
}

func (s *Server) handleListOrganizationalUnitsForParent(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
) {
	if !s.authorizeOrgs(verified, catalog.ActionOrgsListOrganizationalUnitsForParent, "*") {
		s.writeAPIError(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform organizations:ListOrganizationalUnitsForParent.", readOnly, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}

	params := requestParams(r, body)
	parentID := params["ParentId"]
	if parentID == "" {
		parentID = store.OrgRootID
	}

	ous, err := s.store.ListOrganizationalUnitsForParent(parentID)
	if err != nil {
		s.writeAPIError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list organizational units.", readOnly, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}

	units := make([]organizations.CreateOrganizationalUnitResult, 0, len(ous))
	jsonUnits := make([]map[string]any, 0, len(ous))
	for _, ou := range ous {
		arn := orgOUARN(verified.AccountID, ou.ID)
		units = append(units, organizations.CreateOrganizationalUnitResult{
			ID:   ou.ID,
			Name: ou.Name,
			Arn:  arn,
		})
		jsonUnits = append(jsonUnits, map[string]any{
			"Id":   ou.ID,
			"Name": ou.Name,
			"Arn":  arn,
		})
	}

	if wantsJSON(r, body) {
		payload, err := json.Marshal(map[string]any{"OrganizationalUnits": jsonUnits})
		if err != nil {
			s.writeAPIError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to build ListOrganizationalUnitsForParent response.", readOnly, eventID,
				verified.AccessKeyID, verified.AccountID, true)
			return
		}
		s.writeJSONOK(w, requestID, payload)
	} else {
		payload, err := organizations.ListOrganizationalUnitsForParentXML(organizations.ListOrganizationalUnitsForParentResult{
			RequestID: requestID,
			Units:     units,
		})
		if err != nil {
			s.writeAPIError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to build ListOrganizationalUnitsForParent response.", readOnly, eventID,
				verified.AccessKeyID, verified.AccountID, true)
			return
		}
		s.writeXMLOK(w, requestID, payload)
	}
	s.writeSuccessAudit(r, requestID, eventID, verified, "organizations.amazonaws.com", "ListOrganizationalUnitsForParent", true)
}

func (s *Server) handleEnablePolicyType(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
) {
	if !s.authorizeOrgs(verified, catalog.ActionOrgsEnablePolicyType, "*") {
		s.writeAPIError(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform organizations:EnablePolicyType.", readOnly, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}

	params := requestParams(r, body)
	rootID := params["RootId"]
	apiType := params["PolicyType"]
	if rootID == "" {
		rootID = store.OrgRootID
	}
	storeType, ok := orgAPITypeToStore(apiType)
	if !ok {
		s.writeAPIError(w, r, body, requestID, http.StatusBadRequest, "ValidationError",
			"PolicyType must be SERVICE_CONTROL_POLICY or RESOURCE_CONTROL_POLICY.", readOnly, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}
	if rootID != store.OrgRootID {
		s.writeAPIError(w, r, body, requestID, http.StatusBadRequest, "ValidationError",
			"RootId must be the lab root.", readOnly, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}

	if err := s.store.EnableOrgPolicyType(rootID, storeType); err != nil {
		s.writeAPIError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to enable policy type.", readOnly, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}

	enabled, err := s.store.ListEnabledOrgPolicyTypes(rootID)
	if err != nil {
		s.writeAPIError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list enabled policy types.", readOnly, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}
	apiTypes := make([]string, 0, len(enabled))
	jsonTypes := make([]map[string]any, 0, len(enabled))
	for _, t := range enabled {
		api := orgStoreTypeToAPI(t)
		apiTypes = append(apiTypes, api)
		jsonTypes = append(jsonTypes, map[string]any{"Type": api, "Status": "ENABLED"})
	}
	rootArn := fmt.Sprintf("arn:aws:organizations::%s:root/%s/%s", verified.AccountID, labOrgID, rootID)

	if wantsJSON(r, body) {
		payload, err := json.Marshal(map[string]any{
			"Root": map[string]any{
				"Id":          rootID,
				"Arn":         rootArn,
				"Name":        "Root",
				"PolicyTypes": jsonTypes,
			},
		})
		if err != nil {
			s.writeAPIError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to build EnablePolicyType response.", readOnly, eventID,
				verified.AccessKeyID, verified.AccountID, true)
			return
		}
		s.writeJSONOK(w, requestID, payload)
	} else {
		payload, err := organizations.EnablePolicyTypeXML(organizations.EnablePolicyTypeResult{
			RequestID:   requestID,
			RootID:      rootID,
			RootArn:     rootArn,
			RootName:    "Root",
			PolicyTypes: apiTypes,
		})
		if err != nil {
			s.writeAPIError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to build EnablePolicyType response.", readOnly, eventID,
				verified.AccessKeyID, verified.AccountID, true)
			return
		}
		s.writeXMLOK(w, requestID, payload)
	}
	s.writeSuccessAudit(r, requestID, eventID, verified, "organizations.amazonaws.com", "EnablePolicyType", false)
}

func (s *Server) handleOrgsCreatePolicy(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
) {
	if !s.authorizeOrgs(verified, catalog.ActionOrgsCreatePolicy, "*") {
		s.writeAPIError(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform organizations:CreatePolicy.", readOnly, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}

	params := requestParams(r, body)
	name := params["Name"]
	content := params["Content"]
	if content == "" {
		content = params["PolicyDocument"]
	}
	apiType := params["Type"]
	if apiType == "" {
		apiType = params["PolicyType"]
	}
	description := params["Description"]
	storeType, ok := orgAPITypeToStore(apiType)
	if !ok || name == "" || content == "" {
		s.writeAPIError(w, r, body, requestID, http.StatusBadRequest, "ValidationError",
			"Name, Content, and Type (SERVICE_CONTROL_POLICY or RESOURCE_CONTROL_POLICY) are required.", readOnly, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}
	if !s.store.IsOrgPolicyTypeEnabled(store.OrgRootID, storeType) {
		s.writeAPIError(w, r, body, requestID, http.StatusBadRequest, "PolicyTypeNotEnabledException",
			"Policy type is not enabled for the organization root.", readOnly, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}

	policyID, err := s.store.CreateOrgPolicy(storeType, name, content)
	if err != nil {
		if validate.IsInvalid(err) {
			s.writeAPIError(w, r, body, requestID, http.StatusBadRequest, "ValidationError",
				err.Error(), readOnly, eventID,
				verified.AccessKeyID, verified.AccountID, true)
			return
		}
		s.writeAPIError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create policy.", readOnly, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}

	apiTypeOut := orgStoreTypeToAPI(storeType)
	arn := orgPolicyARN(verified.AccountID, storeType, policyID)
	result := organizations.OrgPolicyResult{
		RequestID:   requestID,
		ID:          policyID,
		Name:        name,
		Type:        apiTypeOut,
		Arn:         arn,
		Content:     content,
		Description: description,
	}

	if wantsJSON(r, body) {
		payload, err := json.Marshal(map[string]any{
			"Policy": map[string]any{
				"Content": content,
				"PolicySummary": map[string]any{
					"Id":          policyID,
					"Arn":         arn,
					"Name":        name,
					"Type":        apiTypeOut,
					"Description": description,
					"AwsManaged":  false,
				},
			},
		})
		if err != nil {
			s.writeAPIError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to build CreatePolicy response.", readOnly, eventID,
				verified.AccessKeyID, verified.AccountID, true)
			return
		}
		s.writeJSONOK(w, requestID, payload)
	} else {
		payload, err := organizations.CreatePolicyXML(result)
		if err != nil {
			s.writeAPIError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to build CreatePolicy response.", readOnly, eventID,
				verified.AccessKeyID, verified.AccountID, true)
			return
		}
		s.writeXMLOK(w, requestID, payload)
	}
	s.writeSuccessAudit(r, requestID, eventID, verified, "organizations.amazonaws.com", "CreatePolicy", false)
}

func (s *Server) handleOrgsAttachPolicy(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
) {
	if !s.authorizeOrgs(verified, catalog.ActionOrgsAttachPolicy, "*") {
		s.writeAPIError(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform organizations:AttachPolicy.", readOnly, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}

	params := requestParams(r, body)
	policyID := params["PolicyId"]
	targetID := params["TargetId"]
	if policyID == "" || targetID == "" {
		s.writeAPIError(w, r, body, requestID, http.StatusBadRequest, "ValidationError",
			"PolicyId and TargetId are required.", readOnly, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}
	targetType, ok := orgTargetType(targetID)
	if !ok {
		s.writeAPIError(w, r, body, requestID, http.StatusBadRequest, "ValidationError",
			"TargetId must be a root, OU, or account id.", readOnly, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}

	if err := s.store.AttachOrgPolicy(policyID, targetType, targetID); err != nil {
		s.writeAPIError(w, r, body, requestID, http.StatusBadRequest, "PolicyNotFoundException",
			"Policy or target could not be attached.", readOnly, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}

	if wantsJSON(r, body) {
		s.writeJSONOK(w, requestID, []byte("{}"))
	} else {
		payload, err := organizations.AttachPolicyXML(requestID)
		if err != nil {
			s.writeAPIError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to build AttachPolicy response.", readOnly, eventID,
				verified.AccessKeyID, verified.AccountID, true)
			return
		}
		s.writeXMLOK(w, requestID, payload)
	}
	s.writeSuccessAudit(r, requestID, eventID, verified, "organizations.amazonaws.com", "AttachPolicy", false)
}

func (s *Server) handleOrgsDetachPolicy(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
) {
	if !s.authorizeOrgs(verified, catalog.ActionOrgsDetachPolicy, "*") {
		s.writeAPIError(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform organizations:DetachPolicy.", readOnly, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}

	params := requestParams(r, body)
	policyID := params["PolicyId"]
	targetID := params["TargetId"]
	if policyID == "" || targetID == "" {
		s.writeAPIError(w, r, body, requestID, http.StatusBadRequest, "ValidationError",
			"PolicyId and TargetId are required.", readOnly, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}
	targetType, ok := orgTargetType(targetID)
	if !ok {
		s.writeAPIError(w, r, body, requestID, http.StatusBadRequest, "ValidationError",
			"TargetId must be a root, OU, or account id.", readOnly, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}

	if err := s.store.DetachOrgPolicy(policyID, targetType, targetID); err != nil {
		s.writeAPIError(w, r, body, requestID, http.StatusBadRequest, "PolicyNotFoundException",
			"Policy attachment not found.", readOnly, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}

	if wantsJSON(r, body) {
		s.writeJSONOK(w, requestID, []byte("{}"))
	} else {
		payload, err := organizations.DetachPolicyXML(requestID)
		if err != nil {
			s.writeAPIError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to build DetachPolicy response.", readOnly, eventID,
				verified.AccessKeyID, verified.AccountID, true)
			return
		}
		s.writeXMLOK(w, requestID, payload)
	}
	s.writeSuccessAudit(r, requestID, eventID, verified, "organizations.amazonaws.com", "DetachPolicy", false)
}

func (s *Server) handleOrgsDescribePolicy(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
) {
	if !s.authorizeOrgs(verified, catalog.ActionOrgsDescribePolicy, "*") {
		s.writeAPIError(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform organizations:DescribePolicy.", readOnly, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}

	params := requestParams(r, body)
	policyID := params["PolicyId"]
	if policyID == "" {
		s.writeAPIError(w, r, body, requestID, http.StatusBadRequest, "ValidationError",
			"PolicyId is required.", readOnly, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}

	p, err := s.store.GetOrgPolicy(policyID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || strings.Contains(err.Error(), "sql: no rows") {
			s.writeAPIError(w, r, body, requestID, http.StatusBadRequest, "PolicyNotFoundException",
				"Policy not found.", readOnly, eventID,
				verified.AccessKeyID, verified.AccountID, true)
			return
		}
		s.writeAPIError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe policy.", readOnly, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}

	apiType := orgStoreTypeToAPI(p.Type)
	arn := orgPolicyARN(verified.AccountID, p.Type, p.ID)
	result := organizations.OrgPolicyResult{
		RequestID: requestID,
		ID:        p.ID,
		Name:      p.Name,
		Type:      apiType,
		Arn:       arn,
		Content:   p.Document,
	}

	if wantsJSON(r, body) {
		payload, err := json.Marshal(map[string]any{
			"Policy": map[string]any{
				"Content": p.Document,
				"PolicySummary": map[string]any{
					"Id":         p.ID,
					"Arn":        arn,
					"Name":       p.Name,
					"Type":       apiType,
					"AwsManaged": false,
				},
			},
		})
		if err != nil {
			s.writeAPIError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to build DescribePolicy response.", readOnly, eventID,
				verified.AccessKeyID, verified.AccountID, true)
			return
		}
		s.writeJSONOK(w, requestID, payload)
	} else {
		payload, err := organizations.DescribePolicyXML(result)
		if err != nil {
			s.writeAPIError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to build DescribePolicy response.", readOnly, eventID,
				verified.AccessKeyID, verified.AccountID, true)
			return
		}
		s.writeXMLOK(w, requestID, payload)
	}
	s.writeSuccessAudit(r, requestID, eventID, verified, "organizations.amazonaws.com", "DescribePolicy", true)
}

func (s *Server) handleOrgsMoveAccount(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
) {
	if !s.authorizeOrgs(verified, catalog.ActionOrgsMoveAccount, "*") {
		s.writeAPIError(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform organizations:MoveAccount.", readOnly, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}

	params := requestParams(r, body)
	accountID := params["AccountId"]
	sourceParentID := params["SourceParentId"]
	destinationParentID := params["DestinationParentId"]
	if accountID == "" || sourceParentID == "" || destinationParentID == "" {
		s.writeAPIError(w, r, body, requestID, http.StatusBadRequest, "ValidationError",
			"AccountId, SourceParentId, and DestinationParentId are required.", readOnly, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}
	if err := validate.AccountID(accountID); err != nil {
		s.writeAPIError(w, r, body, requestID, http.StatusBadRequest, "ValidationError",
			"AccountId must be a 12-digit account id.", readOnly, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}

	if err := s.store.MoveAccount(accountID, sourceParentID, destinationParentID); err != nil {
		msg := err.Error()
		code := "InvalidInputException"
		switch {
		case strings.Contains(msg, "account not found"):
			code = "AccountNotFoundException"
		case strings.Contains(msg, "source parent mismatch"), strings.Contains(msg, "move account source:"):
			code = "SourceParentNotFoundException"
		case strings.Contains(msg, "move account destination:"),
			strings.Contains(msg, "organizational unit") && strings.Contains(msg, "not found") && !strings.Contains(msg, "source:"):
			code = "DestinationParentNotFoundException"
		case strings.Contains(msg, "already under destination"):
			code = "DuplicateAccountException"
		}
		s.writeAPIError(w, r, body, requestID, http.StatusBadRequest, code,
			"Unable to move account.", readOnly, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}

	if wantsJSON(r, body) {
		s.writeJSONOK(w, requestID, []byte("{}"))
	} else {
		payload, err := organizations.MoveAccountXML(requestID)
		if err != nil {
			s.writeAPIError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to build MoveAccount response.", readOnly, eventID,
				verified.AccessKeyID, verified.AccountID, true)
			return
		}
		s.writeXMLOK(w, requestID, payload)
	}
	s.writeSuccessAudit(r, requestID, eventID, verified, "organizations.amazonaws.com", "MoveAccount", false)
}

func orgAPITypeToStore(apiType string) (string, bool) {
	switch apiType {
	case "SERVICE_CONTROL_POLICY", "SCP":
		return "SCP", true
	case "RESOURCE_CONTROL_POLICY", "RCP":
		return "RCP", true
	default:
		return "", false
	}
}

func orgStoreTypeToAPI(storeType string) string {
	switch storeType {
	case "RCP":
		return "RESOURCE_CONTROL_POLICY"
	default:
		return "SERVICE_CONTROL_POLICY"
	}
}

func orgTargetType(targetID string) (string, bool) {
	switch {
	case targetID == store.OrgRootID || strings.HasPrefix(targetID, "r-"):
		return "root", true
	case strings.HasPrefix(targetID, "ou-"):
		return "ou", true
	case len(targetID) == 12:
		for _, c := range targetID {
			if c < '0' || c > '9' {
				return "", false
			}
		}
		return "account", true
	default:
		return "", false
	}
}

func orgOUARN(accountID, ouID string) string {
	return fmt.Sprintf("arn:aws:organizations::%s:ou/%s/%s/%s", accountID, labOrgID, store.OrgRootID, ouID)
}

func orgPolicyARN(accountID, storeType, policyID string) string {
	kind := "service_control_policy"
	if storeType == "RCP" {
		kind = "resource_control_policy"
	}
	return fmt.Sprintf("arn:aws:organizations::%s:policy/%s/%s/%s", accountID, labOrgID, kind, policyID)
}

// isOrgsDepthAction reports whether action should be handled by Organizations depth handlers.
// CreatePolicy is shared with IAM and is routed here only when the SigV4 service is organizations.
func isOrgsDepthAction(action, service string) bool {
	switch action {
	case catalog.ActionOrgsListAccounts, "ListAccounts",
		catalog.ActionOrgsCreateOrganizationalUnit, "CreateOrganizationalUnit",
		catalog.ActionOrgsListOrganizationalUnitsForParent, "ListOrganizationalUnitsForParent",
		catalog.ActionOrgsEnablePolicyType, "EnablePolicyType",
		catalog.ActionOrgsCreatePolicy,
		catalog.ActionOrgsAttachPolicy, "AttachPolicy",
		catalog.ActionOrgsDetachPolicy, "DetachPolicy",
		catalog.ActionOrgsDescribePolicy, "DescribePolicy",
		catalog.ActionOrgsMoveAccount, "MoveAccount":
		return true
	case catalog.ActionIAMCreatePolicy, "CreatePolicy":
		return strings.EqualFold(service, "organizations")
	default:
		return false
	}
}
