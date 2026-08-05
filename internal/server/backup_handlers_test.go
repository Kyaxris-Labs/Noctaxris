package server_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestBackupDescribeListDeleteVaultPlanJob(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	emptyVaults := mustBackupREST(t, handler, http.MethodGet, "/backup-vaults", nil, now)
	if emptyVaults.Code != http.StatusOK {
		t.Fatalf("ListBackupVaults empty status=%d body=%q", emptyVaults.Code, emptyVaults.Body.String())
	}

	createVault := mustBackupREST(t, handler, http.MethodPut, "/backup-vaults/cov-vault", map[string]any{}, now)
	if createVault.Code != http.StatusOK || !strings.Contains(createVault.Body.String(), "cov-vault") {
		t.Fatalf("CreateBackupVault status=%d body=%q", createVault.Code, createVault.Body.String())
	}

	descVault := mustBackupREST(t, handler, http.MethodGet, "/backup-vaults/cov-vault", nil, now)
	if descVault.Code != http.StatusOK || !strings.Contains(descVault.Body.String(), "cov-vault") {
		t.Fatalf("DescribeBackupVault status=%d body=%q", descVault.Code, descVault.Body.String())
	}

	listVaults := mustBackupREST(t, handler, http.MethodGet, "/backup-vaults", nil, now)
	if listVaults.Code != http.StatusOK || !strings.Contains(listVaults.Body.String(), "cov-vault") {
		t.Fatalf("ListBackupVaults status=%d body=%q", listVaults.Code, listVaults.Body.String())
	}

	missingVault := mustBackupREST(t, handler, http.MethodGet, "/backup-vaults/no-such-vault", nil, now)
	if missingVault.Code != http.StatusBadRequest || !strings.Contains(missingVault.Body.String(), "ResourceNotFoundException") {
		t.Fatalf("DescribeBackupVault missing want ResourceNotFound status=%d body=%q", missingVault.Code, missingVault.Body.String())
	}

	emptyPlans := mustBackupREST(t, handler, http.MethodGet, "/backup/plans", nil, now)
	if emptyPlans.Code != http.StatusOK {
		t.Fatalf("ListBackupPlans empty status=%d body=%q", emptyPlans.Code, emptyPlans.Body.String())
	}

	createPlan := mustBackupREST(t, handler, http.MethodPut, "/backup/plans/", map[string]any{
		"BackupPlan": map[string]any{
			"BackupPlanName": "cov-plan",
			"Rules": []map[string]any{
				{"RuleName": "daily", "TargetBackupVaultName": "cov-vault", "ScheduleExpression": "cron(0 5 ? * * *)"},
			},
		},
	}, now)
	if createPlan.Code != http.StatusOK || !strings.Contains(createPlan.Body.String(), "BackupPlanId") {
		t.Fatalf("CreateBackupPlan status=%d body=%q", createPlan.Code, createPlan.Body.String())
	}
	var planOut map[string]any
	_ = json.Unmarshal(createPlan.Body.Bytes(), &planOut)
	planID, _ := planOut["BackupPlanId"].(string)

	getPlan := mustBackupREST(t, handler, http.MethodGet, "/backup/plans/"+planID, nil, now)
	if getPlan.Code != http.StatusOK || !strings.Contains(getPlan.Body.String(), "cov-plan") {
		t.Fatalf("GetBackupPlan status=%d body=%q", getPlan.Code, getPlan.Body.String())
	}

	listPlans := mustBackupREST(t, handler, http.MethodGet, "/backup/plans", nil, now)
	if listPlans.Code != http.StatusOK || !strings.Contains(listPlans.Body.String(), planID) {
		t.Fatalf("ListBackupPlans status=%d body=%q", listPlans.Code, listPlans.Body.String())
	}

	missingPlan := mustBackupREST(t, handler, http.MethodGet, "/backup/plans/plan-missing", nil, now)
	if missingPlan.Code != http.StatusBadRequest || !strings.Contains(missingPlan.Body.String(), "ResourceNotFoundException") {
		t.Fatalf("GetBackupPlan missing want ResourceNotFound status=%d body=%q", missingPlan.Code, missingPlan.Body.String())
	}

	start := mustBackupREST(t, handler, http.MethodPut, "/backup-jobs", map[string]any{
		"BackupVaultName": "cov-vault",
		"ResourceArn":     "arn:aws:dynamodb:us-east-1:000000000001:table/cov",
		"IamRoleArn":      "arn:aws:iam::000000000001:role/Backup",
	}, now)
	if start.Code != http.StatusOK {
		t.Fatalf("StartBackupJob status=%d body=%q", start.Code, start.Body.String())
	}
	var startOut map[string]any
	_ = json.Unmarshal(start.Body.Bytes(), &startOut)
	jobID, _ := startOut["BackupJobId"].(string)
	rpARN, _ := startOut["RecoveryPointArn"].(string)

	descJob := mustBackupREST(t, handler, http.MethodGet, "/backup-jobs/"+jobID, nil, now)
	if descJob.Code != http.StatusOK || !strings.Contains(descJob.Body.String(), jobID) {
		t.Fatalf("DescribeBackupJob status=%d body=%q", descJob.Code, descJob.Body.String())
	}

	missingJob := mustBackupREST(t, handler, http.MethodGet, "/backup-jobs/job-missing", nil, now)
	if missingJob.Code != http.StatusBadRequest || !strings.Contains(missingJob.Body.String(), "ResourceNotFoundException") {
		t.Fatalf("DescribeBackupJob missing want ResourceNotFound status=%d body=%q", missingJob.Code, missingJob.Body.String())
	}

	descRP := mustBackupREST(t, handler, http.MethodGet, "/backup-vaults/cov-vault/recovery-points/"+url.PathEscape(rpARN), nil, now)
	if descRP.Code != http.StatusOK || !strings.Contains(descRP.Body.String(), rpARN) {
		t.Fatalf("DescribeRecoveryPoint status=%d body=%q", descRP.Code, descRP.Body.String())
	}

	missingRP := mustBackupREST(t, handler, http.MethodGet, "/backup-vaults/cov-vault/recovery-points/"+url.PathEscape("arn:aws:backup:us-east-1:000000000001:recovery-point:missing"), nil, now)
	if missingRP.Code != http.StatusBadRequest || !strings.Contains(missingRP.Body.String(), "ResourceNotFoundException") {
		t.Fatalf("DescribeRecoveryPoint missing want ResourceNotFound status=%d body=%q", missingRP.Code, missingRP.Body.String())
	}

	delRP := mustBackupREST(t, handler, http.MethodDelete, "/backup-vaults/cov-vault/recovery-points/"+url.PathEscape(rpARN), nil, now)
	if delRP.Code != http.StatusOK {
		t.Fatalf("DeleteRecoveryPoint status=%d body=%q", delRP.Code, delRP.Body.String())
	}

	delPlan := mustBackupREST(t, handler, http.MethodDelete, "/backup/plans/"+planID, nil, now)
	if delPlan.Code != http.StatusNoContent {
		t.Fatalf("DeleteBackupPlan status=%d body=%q", delPlan.Code, delPlan.Body.String())
	}

	delPlanGone := mustBackupREST(t, handler, http.MethodDelete, "/backup/plans/"+planID, nil, now)
	if delPlanGone.Code != http.StatusBadRequest || !strings.Contains(delPlanGone.Body.String(), "ResourceNotFoundException") {
		t.Fatalf("DeleteBackupPlan missing want ResourceNotFound status=%d body=%q", delPlanGone.Code, delPlanGone.Body.String())
	}

	delVault := mustBackupREST(t, handler, http.MethodDelete, "/backup-vaults/cov-vault", nil, now)
	if delVault.Code != http.StatusNoContent {
		t.Fatalf("DeleteBackupVault status=%d body=%q", delVault.Code, delVault.Body.String())
	}

	delVaultGone := mustBackupREST(t, handler, http.MethodDelete, "/backup-vaults/cov-vault", nil, now)
	if delVaultGone.Code != http.StatusBadRequest || !strings.Contains(delVaultGone.Body.String(), "ResourceNotFoundException") {
		t.Fatalf("DeleteBackupVault missing want ResourceNotFound status=%d body=%q", delVaultGone.Code, delVaultGone.Body.String())
	}
}

func TestBackupJSONTargetDescribeAndUnknown(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustJSONTarget(t, handler, "AWSBackup.CreateBackupVault", "backup", map[string]any{
		"BackupVaultName": "json-vault",
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateBackupVault JSON status=%d body=%q", create.Code, create.Body.String())
	}

	desc := mustJSONTarget(t, handler, "AWSBackup.DescribeBackupVault", "backup", map[string]any{
		"BackupVaultName": "json-vault",
	}, now)
	if desc.Code != http.StatusOK || !strings.Contains(desc.Body.String(), "json-vault") {
		t.Fatalf("DescribeBackupVault JSON status=%d body=%q", desc.Code, desc.Body.String())
	}

	list := mustJSONTarget(t, handler, "AWSBackup.ListBackupVaults", "backup", map[string]any{}, now)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "json-vault") {
		t.Fatalf("ListBackupVaults JSON status=%d body=%q", list.Code, list.Body.String())
	}

	unknown := mustJSONTarget(t, handler, "AWSBackup.TagResource", "backup", map[string]any{}, now)
	if unknown.Code != http.StatusNotImplemented || !strings.Contains(unknown.Body.String(), "InvalidRequestException") {
		t.Fatalf("unknown Backup action want InvalidRequestException status=%d body=%q", unknown.Code, unknown.Body.String())
	}
}
