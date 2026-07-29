package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	backupsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/backup"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	backupJSONContentType = "application/json"
	backupEventSource     = "backup.amazonaws.com"
)

func (s *Server) handleBackup(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = backupAction(action)

	switch action {
	case catalog.ActionBackupCreateBackupVault:
		s.backupCreateVault(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionBackupDescribeBackupVault:
		s.backupDescribeVault(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionBackupListBackupVaults:
		s.backupListVaults(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionBackupDeleteBackupVault:
		s.backupDeleteVault(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionBackupCreateBackupPlan:
		s.backupCreatePlan(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionBackupGetBackupPlan:
		s.backupGetPlan(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionBackupListBackupPlans:
		s.backupListPlans(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionBackupDeleteBackupPlan:
		s.backupDeletePlan(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionBackupStartBackupJob:
		s.backupStartJob(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionBackupDescribeBackupJob:
		s.backupDescribeJob(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionBackupDescribeRecoveryPoint:
		s.backupDescribeRecoveryPoint(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionBackupListRecoveryPointsByBackupVault:
		s.backupListRecoveryPoints(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionBackupListBackupJobs:
		s.backupListJobs(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionBackupStopBackupJob:
		s.backupStopJob(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionBackupDeleteRecoveryPoint:
		s.backupDeleteRecoveryPoint(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionBackupCreateBackupSelection:
		s.backupCreateSelection(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionBackupGetBackupSelection:
		s.backupGetSelection(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionBackupListBackupSelections:
		s.backupListSelections(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionBackupDeleteBackupSelection:
		s.backupDeleteSelection(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeBackupError(w, r, body, requestID, http.StatusNotImplemented, "InvalidRequestException",
			"This Backup action is not implemented.", readOnly, eventID, verified)
	}
}

func backupAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "CreateBackupVault":
		return catalog.ActionBackupCreateBackupVault
	case "DescribeBackupVault":
		return catalog.ActionBackupDescribeBackupVault
	case "ListBackupVaults":
		return catalog.ActionBackupListBackupVaults
	case "DeleteBackupVault":
		return catalog.ActionBackupDeleteBackupVault
	case "CreateBackupPlan":
		return catalog.ActionBackupCreateBackupPlan
	case "GetBackupPlan":
		return catalog.ActionBackupGetBackupPlan
	case "ListBackupPlans":
		return catalog.ActionBackupListBackupPlans
	case "DeleteBackupPlan":
		return catalog.ActionBackupDeleteBackupPlan
	case "StartBackupJob":
		return catalog.ActionBackupStartBackupJob
	case "DescribeBackupJob":
		return catalog.ActionBackupDescribeBackupJob
	case "DescribeRecoveryPoint":
		return catalog.ActionBackupDescribeRecoveryPoint
	case "ListRecoveryPointsByBackupVault":
		return catalog.ActionBackupListRecoveryPointsByBackupVault
	case "ListBackupJobs":
		return catalog.ActionBackupListBackupJobs
	case "StopBackupJob":
		return catalog.ActionBackupStopBackupJob
	case "DeleteRecoveryPoint":
		return catalog.ActionBackupDeleteRecoveryPoint
	case "CreateBackupSelection":
		return catalog.ActionBackupCreateBackupSelection
	case "GetBackupSelection":
		return catalog.ActionBackupGetBackupSelection
	case "ListBackupSelections":
		return catalog.ActionBackupListBackupSelections
	case "DeleteBackupSelection":
		return catalog.ActionBackupDeleteBackupSelection
	default:
		return action
	}
}

func isBackupRESTPath(path string) bool {
	p := strings.ToLower(path)
	return strings.HasPrefix(p, "/backup-vaults") ||
		strings.HasPrefix(p, "/backup/") ||
		strings.HasPrefix(p, "/backup-jobs")
}

func resolveBackupREST(r *http.Request) (string, map[string]any) {
	path := strings.TrimSuffix(r.URL.Path, "/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	params := map[string]any{}
	method := r.Method

	// /backup-vaults
	if len(parts) == 1 && parts[0] == "backup-vaults" && method == http.MethodGet {
		return catalog.ActionBackupListBackupVaults, params
	}
	// /backup-vaults/{name}
	if len(parts) == 2 && parts[0] == "backup-vaults" {
		params["BackupVaultName"] = parts[1]
		switch method {
		case http.MethodPut:
			return catalog.ActionBackupCreateBackupVault, params
		case http.MethodGet:
			return catalog.ActionBackupDescribeBackupVault, params
		case http.MethodDelete:
			return catalog.ActionBackupDeleteBackupVault, params
		}
	}
	// /backup-vaults/{name}/recovery-points
	if len(parts) == 3 && parts[0] == "backup-vaults" && parts[2] == "recovery-points" && method == http.MethodGet {
		params["BackupVaultName"] = parts[1]
		return catalog.ActionBackupListRecoveryPointsByBackupVault, params
	}
	// /backup-vaults/{name}/recovery-points/{arn...}
	if len(parts) >= 4 && parts[0] == "backup-vaults" && parts[2] == "recovery-points" {
		params["BackupVaultName"] = parts[1]
		arnPath := strings.Join(parts[3:], "/")
		if decoded, err := url.PathUnescape(arnPath); err == nil {
			arnPath = decoded
		}
		params["RecoveryPointArn"] = arnPath
		switch method {
		case http.MethodGet:
			return catalog.ActionBackupDescribeRecoveryPoint, params
		case http.MethodDelete:
			return catalog.ActionBackupDeleteRecoveryPoint, params
		}
	}
	// /backup/plans
	if len(parts) == 2 && parts[0] == "backup" && parts[1] == "plans" {
		switch method {
		case http.MethodPut:
			return catalog.ActionBackupCreateBackupPlan, params
		case http.MethodGet:
			return catalog.ActionBackupListBackupPlans, params
		}
	}
	// /backup/plans/{id}/selections
	if len(parts) == 4 && parts[0] == "backup" && parts[1] == "plans" && parts[3] == "selections" {
		params["BackupPlanId"] = parts[2]
		switch method {
		case http.MethodPut:
			return catalog.ActionBackupCreateBackupSelection, params
		case http.MethodGet:
			return catalog.ActionBackupListBackupSelections, params
		}
	}
	// /backup/plans/{id}/selections/{selectionId}
	if len(parts) == 5 && parts[0] == "backup" && parts[1] == "plans" && parts[3] == "selections" {
		params["BackupPlanId"] = parts[2]
		params["SelectionId"] = parts[4]
		switch method {
		case http.MethodGet:
			return catalog.ActionBackupGetBackupSelection, params
		case http.MethodDelete:
			return catalog.ActionBackupDeleteBackupSelection, params
		}
	}
	// /backup/plans/{id}
	if len(parts) == 3 && parts[0] == "backup" && parts[1] == "plans" {
		params["BackupPlanId"] = parts[2]
		switch method {
		case http.MethodGet:
			return catalog.ActionBackupGetBackupPlan, params
		case http.MethodDelete:
			return catalog.ActionBackupDeleteBackupPlan, params
		}
	}
	// /backup-jobs
	if len(parts) == 1 && parts[0] == "backup-jobs" {
		switch method {
		case http.MethodPut:
			return catalog.ActionBackupStartBackupJob, params
		case http.MethodGet:
			q := r.URL.Query()
			if v := q.Get("backupVaultName"); v != "" {
				params["ByBackupVaultName"] = v
			}
			if v := q.Get("resourceArn"); v != "" {
				params["ByResourceArn"] = v
			}
			if v := q.Get("resourceType"); v != "" {
				params["ByResourceType"] = v
			}
			if v := q.Get("state"); v != "" {
				params["ByState"] = v
			}
			return catalog.ActionBackupListBackupJobs, params
		}
	}
	// /backup-jobs/{id}
	if len(parts) == 2 && parts[0] == "backup-jobs" {
		params["BackupJobId"] = parts[1]
		switch method {
		case http.MethodGet:
			return catalog.ActionBackupDescribeBackupJob, params
		case http.MethodPost:
			return catalog.ActionBackupStopBackupJob, params
		}
	}
	return "", nil
}

func (s *Server) backupRegion(verified *authn.Verified) string {
	if verified.Region != "" {
		return verified.Region
	}
	return store.DefaultBackupRegion
}

func backupMergeParams(pathParams, bodyParams map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range bodyParams {
		out[k] = v
	}
	for k, v := range pathParams {
		if _, exists := out[k]; !exists {
			out[k] = v
		}
	}
	return out
}

func (s *Server) backupCreateVault(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionBackupCreateBackupVault, "*") {
		s.writeBackupError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform backup:CreateBackupVault.", readOnly, eventID, verified)
		return
	}
	name, _ := params["BackupVaultName"].(string)
	key, _ := params["EncryptionKeyArn"].(string)
	v, err := s.store.CreateBackupVault(verified.AccountID, s.backupRegion(verified), name, key)
	if errors.Is(err, store.ErrBackupExists) {
		s.writeBackupError(w, r, body, requestID, http.StatusBadRequest, "AlreadyExistsException",
			"Backup vault already exists.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrBackupBadRequest) {
		s.writeBackupError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterValueException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeBackupError(w, r, body, requestID, http.StatusInternalServerError, "ServiceUnavailableException",
			"Unable to create backup vault.", readOnly, eventID, verified)
		return
	}
	payload, _ := backupsvc.CreateBackupVaultJSON(v)
	s.writeBackupOK(w, http.StatusOK, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, backupEventSource, "CreateBackupVault", readOnly)
}

func (s *Server) backupDescribeVault(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionBackupDescribeBackupVault, "*") {
		s.writeBackupError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform backup:DescribeBackupVault.", readOnly, eventID, verified)
		return
	}
	name, _ := params["BackupVaultName"].(string)
	v, err := s.store.DescribeBackupVault(verified.AccountID, s.backupRegion(verified), name)
	if errors.Is(err, store.ErrBackupNotFound) {
		s.writeBackupError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Backup vault not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeBackupError(w, r, body, requestID, http.StatusInternalServerError, "ServiceUnavailableException",
			"Unable to describe backup vault.", readOnly, eventID, verified)
		return
	}
	payload, _ := backupsvc.DescribeBackupVaultJSON(v)
	s.writeBackupOK(w, http.StatusOK, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, backupEventSource, "DescribeBackupVault", readOnly)
}

func (s *Server) backupListVaults(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool,
) {
	if !s.authorize(verified, catalog.ActionBackupListBackupVaults, "*") {
		s.writeBackupError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform backup:ListBackupVaults.", readOnly, eventID, verified)
		return
	}
	list, err := s.store.ListBackupVaults(verified.AccountID, s.backupRegion(verified))
	if err != nil {
		s.writeBackupError(w, r, body, requestID, http.StatusInternalServerError, "ServiceUnavailableException",
			"Unable to list backup vaults.", readOnly, eventID, verified)
		return
	}
	payload, _ := backupsvc.ListBackupVaultsJSON(list)
	s.writeBackupOK(w, http.StatusOK, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, backupEventSource, "ListBackupVaults", readOnly)
}

func (s *Server) backupDeleteVault(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionBackupDeleteBackupVault, "*") {
		s.writeBackupError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform backup:DeleteBackupVault.", readOnly, eventID, verified)
		return
	}
	name, _ := params["BackupVaultName"].(string)
	err := s.store.DeleteBackupVault(verified.AccountID, s.backupRegion(verified), name)
	if errors.Is(err, store.ErrBackupNotFound) {
		s.writeBackupError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Backup vault not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrBackupConflict) {
		s.writeBackupError(w, r, body, requestID, http.StatusBadRequest, "InvalidRequestException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeBackupError(w, r, body, requestID, http.StatusInternalServerError, "ServiceUnavailableException",
			"Unable to delete backup vault.", readOnly, eventID, verified)
		return
	}
	w.WriteHeader(http.StatusNoContent)
	s.writeSuccessAudit(r, requestID, eventID, verified, backupEventSource, "DeleteBackupVault", readOnly)
}

func (s *Server) backupCreatePlan(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionBackupCreateBackupPlan, "*") {
		s.writeBackupError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform backup:CreateBackupPlan.", readOnly, eventID, verified)
		return
	}
	planName := ""
	var rules any
	if plan, ok := params["BackupPlan"].(map[string]any); ok {
		planName, _ = plan["BackupPlanName"].(string)
		rules = plan["Rules"]
	}
	p, err := s.store.CreateBackupPlan(verified.AccountID, s.backupRegion(verified), planName, rules)
	if errors.Is(err, store.ErrBackupBadRequest) {
		s.writeBackupError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterValueException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeBackupError(w, r, body, requestID, http.StatusInternalServerError, "ServiceUnavailableException",
			"Unable to create backup plan.", readOnly, eventID, verified)
		return
	}
	payload, _ := backupsvc.CreateBackupPlanJSON(p)
	s.writeBackupOK(w, http.StatusOK, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, backupEventSource, "CreateBackupPlan", readOnly)
}

func (s *Server) backupGetPlan(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionBackupGetBackupPlan, "*") {
		s.writeBackupError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform backup:GetBackupPlan.", readOnly, eventID, verified)
		return
	}
	id, _ := params["BackupPlanId"].(string)
	p, err := s.store.GetBackupPlan(verified.AccountID, s.backupRegion(verified), id)
	if errors.Is(err, store.ErrBackupNotFound) {
		s.writeBackupError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Backup plan not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeBackupError(w, r, body, requestID, http.StatusInternalServerError, "ServiceUnavailableException",
			"Unable to get backup plan.", readOnly, eventID, verified)
		return
	}
	payload, _ := backupsvc.GetBackupPlanJSON(p)
	s.writeBackupOK(w, http.StatusOK, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, backupEventSource, "GetBackupPlan", readOnly)
}

func (s *Server) backupListPlans(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool,
) {
	if !s.authorize(verified, catalog.ActionBackupListBackupPlans, "*") {
		s.writeBackupError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform backup:ListBackupPlans.", readOnly, eventID, verified)
		return
	}
	list, err := s.store.ListBackupPlans(verified.AccountID, s.backupRegion(verified))
	if err != nil {
		s.writeBackupError(w, r, body, requestID, http.StatusInternalServerError, "ServiceUnavailableException",
			"Unable to list backup plans.", readOnly, eventID, verified)
		return
	}
	payload, _ := backupsvc.ListBackupPlansJSON(list)
	s.writeBackupOK(w, http.StatusOK, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, backupEventSource, "ListBackupPlans", readOnly)
}

func (s *Server) backupDeletePlan(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionBackupDeleteBackupPlan, "*") {
		s.writeBackupError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform backup:DeleteBackupPlan.", readOnly, eventID, verified)
		return
	}
	id, _ := params["BackupPlanId"].(string)
	err := s.store.DeleteBackupPlan(verified.AccountID, s.backupRegion(verified), id)
	if errors.Is(err, store.ErrBackupNotFound) {
		s.writeBackupError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Backup plan not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeBackupError(w, r, body, requestID, http.StatusInternalServerError, "ServiceUnavailableException",
			"Unable to delete backup plan.", readOnly, eventID, verified)
		return
	}
	w.WriteHeader(http.StatusNoContent)
	s.writeSuccessAudit(r, requestID, eventID, verified, backupEventSource, "DeleteBackupPlan", readOnly)
}

func (s *Server) backupStartJob(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionBackupStartBackupJob, "*") {
		s.writeBackupError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform backup:StartBackupJob.", readOnly, eventID, verified)
		return
	}
	vault, _ := params["BackupVaultName"].(string)
	resource, _ := params["ResourceArn"].(string)
	role, _ := params["IamRoleArn"].(string)
	job, _, err := s.store.StartBackupJob(verified.AccountID, s.backupRegion(verified), vault, resource, role)
	if errors.Is(err, store.ErrBackupNotFound) {
		s.writeBackupError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Backup vault not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrBackupBadRequest) {
		s.writeBackupError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterValueException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeBackupError(w, r, body, requestID, http.StatusInternalServerError, "ServiceUnavailableException",
			"Unable to start backup job.", readOnly, eventID, verified)
		return
	}
	payload, _ := backupsvc.StartBackupJobJSON(job)
	s.writeBackupOK(w, http.StatusOK, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, backupEventSource, "StartBackupJob", readOnly)
}

func (s *Server) backupDescribeJob(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionBackupDescribeBackupJob, "*") {
		s.writeBackupError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform backup:DescribeBackupJob.", readOnly, eventID, verified)
		return
	}
	id, _ := params["BackupJobId"].(string)
	job, err := s.store.DescribeBackupJob(verified.AccountID, s.backupRegion(verified), id)
	if errors.Is(err, store.ErrBackupNotFound) {
		s.writeBackupError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Backup job not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeBackupError(w, r, body, requestID, http.StatusInternalServerError, "ServiceUnavailableException",
			"Unable to describe backup job.", readOnly, eventID, verified)
		return
	}
	payload, _ := backupsvc.DescribeBackupJobJSON(job)
	s.writeBackupOK(w, http.StatusOK, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, backupEventSource, "DescribeBackupJob", readOnly)
}

func (s *Server) backupDescribeRecoveryPoint(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionBackupDescribeRecoveryPoint, "*") {
		s.writeBackupError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform backup:DescribeRecoveryPoint.", readOnly, eventID, verified)
		return
	}
	vault, _ := params["BackupVaultName"].(string)
	arn, _ := params["RecoveryPointArn"].(string)
	rp, err := s.store.DescribeRecoveryPoint(verified.AccountID, s.backupRegion(verified), vault, arn)
	if errors.Is(err, store.ErrBackupNotFound) {
		s.writeBackupError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Recovery point not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeBackupError(w, r, body, requestID, http.StatusInternalServerError, "ServiceUnavailableException",
			"Unable to describe recovery point.", readOnly, eventID, verified)
		return
	}
	vaultARN := store.BackupVaultARN(s.backupRegion(verified), verified.AccountID, vault)
	payload, _ := backupsvc.DescribeRecoveryPointJSON(rp, vaultARN)
	s.writeBackupOK(w, http.StatusOK, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, backupEventSource, "DescribeRecoveryPoint", readOnly)
}

func (s *Server) backupListRecoveryPoints(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionBackupListRecoveryPointsByBackupVault, "*") {
		s.writeBackupError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform backup:ListRecoveryPointsByBackupVault.", readOnly, eventID, verified)
		return
	}
	vault, _ := params["BackupVaultName"].(string)
	list, err := s.store.ListRecoveryPointsByBackupVault(verified.AccountID, s.backupRegion(verified), vault)
	if errors.Is(err, store.ErrBackupNotFound) {
		s.writeBackupError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Backup vault not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeBackupError(w, r, body, requestID, http.StatusInternalServerError, "ServiceUnavailableException",
			"Unable to list recovery points.", readOnly, eventID, verified)
		return
	}
	payload, _ := backupsvc.ListRecoveryPointsJSON(list)
	s.writeBackupOK(w, http.StatusOK, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, backupEventSource, "ListRecoveryPointsByBackupVault", readOnly)
}

func backupStringParam(params map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := params[key].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func backupResourcesFromParams(params map[string]any) []string {
	raw, ok := params["Resources"]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	default:
		return nil
	}
}

func (s *Server) backupListJobs(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionBackupListBackupJobs, "*") {
		s.writeBackupError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform backup:ListBackupJobs.", readOnly, eventID, verified)
		return
	}
	filter := store.BackupJobListFilter{
		BackupVaultName: backupStringParam(params, "ByBackupVaultName", "BackupVaultName"),
		ResourceARN:     backupStringParam(params, "ByResourceArn", "ResourceArn"),
		ResourceType:    backupStringParam(params, "ByResourceType", "ResourceType"),
		State:           backupStringParam(params, "ByState", "State"),
	}
	list, err := s.store.ListBackupJobs(verified.AccountID, s.backupRegion(verified), filter)
	if err != nil {
		s.writeBackupError(w, r, body, requestID, http.StatusInternalServerError, "ServiceUnavailableException",
			"Unable to list backup jobs.", readOnly, eventID, verified)
		return
	}
	payload, _ := backupsvc.ListBackupJobsJSON(list)
	s.writeBackupOK(w, http.StatusOK, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, backupEventSource, "ListBackupJobs", readOnly)
}

func (s *Server) backupStopJob(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionBackupStopBackupJob, "*") {
		s.writeBackupError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform backup:StopBackupJob.", readOnly, eventID, verified)
		return
	}
	id, _ := params["BackupJobId"].(string)
	err := s.store.StopBackupJob(verified.AccountID, s.backupRegion(verified), id)
	if errors.Is(err, store.ErrBackupNotFound) {
		s.writeBackupError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Backup job not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeBackupError(w, r, body, requestID, http.StatusInternalServerError, "ServiceUnavailableException",
			"Unable to stop backup job.", readOnly, eventID, verified)
		return
	}
	w.WriteHeader(http.StatusOK)
	s.writeSuccessAudit(r, requestID, eventID, verified, backupEventSource, "StopBackupJob", readOnly)
}

func (s *Server) backupDeleteRecoveryPoint(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionBackupDeleteRecoveryPoint, "*") {
		s.writeBackupError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform backup:DeleteRecoveryPoint.", readOnly, eventID, verified)
		return
	}
	vault, _ := params["BackupVaultName"].(string)
	arn, _ := params["RecoveryPointArn"].(string)
	err := s.store.DeleteRecoveryPoint(verified.AccountID, s.backupRegion(verified), vault, arn)
	if errors.Is(err, store.ErrBackupNotFound) {
		s.writeBackupError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Recovery point not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrBackupBadRequest) {
		s.writeBackupError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterValueException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeBackupError(w, r, body, requestID, http.StatusInternalServerError, "ServiceUnavailableException",
			"Unable to delete recovery point.", readOnly, eventID, verified)
		return
	}
	w.WriteHeader(http.StatusOK)
	s.writeSuccessAudit(r, requestID, eventID, verified, backupEventSource, "DeleteRecoveryPoint", readOnly)
}

func (s *Server) backupCreateSelection(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionBackupCreateBackupSelection, "*") {
		s.writeBackupError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform backup:CreateBackupSelection.", readOnly, eventID, verified)
		return
	}
	planID, _ := params["BackupPlanId"].(string)
	selectionName := ""
	iamRole := ""
	var resources []string
	if sel, ok := params["BackupSelection"].(map[string]any); ok {
		selectionName, _ = sel["SelectionName"].(string)
		iamRole, _ = sel["IamRoleArn"].(string)
		resources = backupResourcesFromParams(sel)
	}
	created, err := s.store.CreateBackupSelection(verified.AccountID, s.backupRegion(verified), planID, selectionName, iamRole, resources)
	if errors.Is(err, store.ErrBackupNotFound) {
		s.writeBackupError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Backup plan not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrBackupBadRequest) {
		s.writeBackupError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterValueException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeBackupError(w, r, body, requestID, http.StatusInternalServerError, "ServiceUnavailableException",
			"Unable to create backup selection.", readOnly, eventID, verified)
		return
	}
	payload, _ := backupsvc.CreateBackupSelectionJSON(created)
	s.writeBackupOK(w, http.StatusOK, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, backupEventSource, "CreateBackupSelection", readOnly)
}

func (s *Server) backupGetSelection(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionBackupGetBackupSelection, "*") {
		s.writeBackupError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform backup:GetBackupSelection.", readOnly, eventID, verified)
		return
	}
	planID, _ := params["BackupPlanId"].(string)
	selectionID, _ := params["SelectionId"].(string)
	sel, err := s.store.GetBackupSelection(verified.AccountID, s.backupRegion(verified), planID, selectionID)
	if errors.Is(err, store.ErrBackupNotFound) {
		s.writeBackupError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Backup selection not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeBackupError(w, r, body, requestID, http.StatusInternalServerError, "ServiceUnavailableException",
			"Unable to get backup selection.", readOnly, eventID, verified)
		return
	}
	payload, _ := backupsvc.GetBackupSelectionJSON(sel)
	s.writeBackupOK(w, http.StatusOK, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, backupEventSource, "GetBackupSelection", readOnly)
}

func (s *Server) backupListSelections(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionBackupListBackupSelections, "*") {
		s.writeBackupError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform backup:ListBackupSelections.", readOnly, eventID, verified)
		return
	}
	planID, _ := params["BackupPlanId"].(string)
	list, err := s.store.ListBackupSelections(verified.AccountID, s.backupRegion(verified), planID)
	if errors.Is(err, store.ErrBackupNotFound) {
		s.writeBackupError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Backup plan not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeBackupError(w, r, body, requestID, http.StatusInternalServerError, "ServiceUnavailableException",
			"Unable to list backup selections.", readOnly, eventID, verified)
		return
	}
	payload, _ := backupsvc.ListBackupSelectionsJSON(list)
	s.writeBackupOK(w, http.StatusOK, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, backupEventSource, "ListBackupSelections", readOnly)
}

func (s *Server) backupDeleteSelection(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionBackupDeleteBackupSelection, "*") {
		s.writeBackupError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform backup:DeleteBackupSelection.", readOnly, eventID, verified)
		return
	}
	planID, _ := params["BackupPlanId"].(string)
	selectionID, _ := params["SelectionId"].(string)
	err := s.store.DeleteBackupSelection(verified.AccountID, s.backupRegion(verified), planID, selectionID)
	if errors.Is(err, store.ErrBackupNotFound) {
		s.writeBackupError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Backup selection not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeBackupError(w, r, body, requestID, http.StatusInternalServerError, "ServiceUnavailableException",
			"Unable to delete backup selection.", readOnly, eventID, verified)
		return
	}
	w.WriteHeader(http.StatusOK)
	s.writeSuccessAudit(r, requestID, eventID, verified, backupEventSource, "DeleteBackupSelection", readOnly)
}

func (s *Server) writeBackupOK(w http.ResponseWriter, status int, payload []byte) {
	w.Header().Set("Content-Type", backupJSONContentType)
	w.WriteHeader(status)
	_, _ = w.Write(payload)
}

func (s *Server) writeBackupError(
	w http.ResponseWriter, r *http.Request, body []byte, requestID string, status int, code, message string,
	readOnly bool, eventID string, verified *authn.Verified,
) {
	w.Header().Set("Content-Type", backupJSONContentType)
	w.Header().Set("x-amzn-ErrorType", code)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"__type": code, "message": message})
	_ = body
	s.writeSuccessAudit(r, requestID, eventID, verified, backupEventSource, code, readOnly)
}
