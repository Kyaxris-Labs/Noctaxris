package backup

import (
	"encoding/json"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func isoMilli(ms int64) string {
	return time.UnixMilli(ms).UTC().Format(time.RFC3339Nano)
}

// CreateBackupVaultJSON builds CreateBackupVault response.
func CreateBackupVaultJSON(v store.BackupVault) ([]byte, error) {
	return json.Marshal(map[string]any{
		"BackupVaultName": v.BackupVaultName,
		"BackupVaultArn":  v.BackupVaultARN,
		"CreationDate":    isoMilli(v.CreatedAt),
	})
}

// DescribeBackupVaultJSON builds DescribeBackupVault response.
func DescribeBackupVaultJSON(v store.BackupVault) ([]byte, error) {
	out := map[string]any{
		"BackupVaultName":        v.BackupVaultName,
		"BackupVaultArn":         v.BackupVaultARN,
		"CreationDate":           isoMilli(v.CreatedAt),
		"NumberOfRecoveryPoints": v.NumberOfRecoveryPoints,
	}
	if v.EncryptionKeyARN != "" {
		out["EncryptionKeyArn"] = v.EncryptionKeyARN
	}
	return json.Marshal(out)
}

// ListBackupVaultsJSON builds ListBackupVaults response.
func ListBackupVaultsJSON(vaults []store.BackupVault) ([]byte, error) {
	items := make([]map[string]any, 0, len(vaults))
	for _, v := range vaults {
		item := map[string]any{
			"BackupVaultName":        v.BackupVaultName,
			"BackupVaultArn":         v.BackupVaultARN,
			"CreationDate":           isoMilli(v.CreatedAt),
			"NumberOfRecoveryPoints": v.NumberOfRecoveryPoints,
		}
		items = append(items, item)
	}
	return json.Marshal(map[string]any{"BackupVaultList": items})
}

// CreateBackupPlanJSON builds CreateBackupPlan response.
func CreateBackupPlanJSON(p store.BackupPlan) ([]byte, error) {
	return json.Marshal(map[string]any{
		"BackupPlanId":  p.BackupPlanID,
		"BackupPlanArn": p.BackupPlanARN,
		"CreationDate":  isoMilli(p.CreatedAt),
		"VersionId":     p.VersionID,
	})
}

// GetBackupPlanJSON builds GetBackupPlan response.
func GetBackupPlanJSON(p store.BackupPlan) ([]byte, error) {
	var rules any = []any{}
	_ = json.Unmarshal([]byte(p.RulesJSON), &rules)
	return json.Marshal(map[string]any{
		"BackupPlanId":  p.BackupPlanID,
		"BackupPlanArn": p.BackupPlanARN,
		"CreationDate":  isoMilli(p.CreatedAt),
		"VersionId":     p.VersionID,
		"BackupPlan": map[string]any{
			"BackupPlanName": p.BackupPlanName,
			"Rules":          rules,
		},
	})
}

// ListBackupPlansJSON builds ListBackupPlans response.
func ListBackupPlansJSON(plans []store.BackupPlan) ([]byte, error) {
	items := make([]map[string]any, 0, len(plans))
	for _, p := range plans {
		items = append(items, map[string]any{
			"BackupPlanId":   p.BackupPlanID,
			"BackupPlanArn":  p.BackupPlanARN,
			"BackupPlanName": p.BackupPlanName,
			"CreationDate":   isoMilli(p.CreatedAt),
			"VersionId":      p.VersionID,
		})
	}
	return json.Marshal(map[string]any{"BackupPlansList": items})
}

// StartBackupJobJSON builds StartBackupJob response.
func StartBackupJobJSON(j store.BackupJob) ([]byte, error) {
	return json.Marshal(map[string]any{
		"BackupJobId":      j.BackupJobID,
		"RecoveryPointArn": j.RecoveryPointARN,
		"CreationDate":     isoMilli(j.CreatedAt),
	})
}

// DescribeBackupJobJSON builds DescribeBackupJob response.
func DescribeBackupJobJSON(j store.BackupJob) ([]byte, error) {
	out := map[string]any{
		"BackupJobId":      j.BackupJobID,
		"BackupVaultName":  j.BackupVaultName,
		"ResourceArn":      j.ResourceARN,
		"IamRoleArn":       j.IamRoleARN,
		"State":            j.State,
		"PercentDone":      j.PercentDone,
		"CreationDate":     isoMilli(j.CreatedAt),
		"RecoveryPointArn": j.RecoveryPointARN,
	}
	if j.CompletionDate > 0 {
		out["CompletionDate"] = isoMilli(j.CompletionDate)
	}
	return json.Marshal(out)
}

// DescribeRecoveryPointJSON builds DescribeRecoveryPoint response.
func DescribeRecoveryPointJSON(rp store.BackupRecoveryPoint, vaultARN string) ([]byte, error) {
	return json.Marshal(map[string]any{
		"RecoveryPointArn": rp.RecoveryPointARN,
		"BackupVaultName":  rp.BackupVaultName,
		"BackupVaultArn":   vaultARN,
		"ResourceArn":      rp.ResourceARN,
		"ResourceType":     rp.ResourceType,
		"Status":           rp.Status,
		"CreationDate":     isoMilli(rp.CreationDate),
	})
}

// ListRecoveryPointsJSON builds ListRecoveryPointsByBackupVault response.
func ListRecoveryPointsJSON(points []store.BackupRecoveryPoint) ([]byte, error) {
	items := make([]map[string]any, 0, len(points))
	for _, rp := range points {
		items = append(items, map[string]any{
			"RecoveryPointArn": rp.RecoveryPointARN,
			"BackupVaultName":  rp.BackupVaultName,
			"ResourceArn":      rp.ResourceARN,
			"ResourceType":     rp.ResourceType,
			"Status":           rp.Status,
			"CreationDate":     isoMilli(rp.CreationDate),
		})
	}
	return json.Marshal(map[string]any{"RecoveryPoints": items})
}

// ListBackupJobsJSON builds ListBackupJobs response.
func ListBackupJobsJSON(jobs []store.BackupJob) ([]byte, error) {
	items := make([]map[string]any, 0, len(jobs))
	for _, j := range jobs {
		item := map[string]any{
			"BackupJobId":      j.BackupJobID,
			"BackupVaultName":  j.BackupVaultName,
			"ResourceArn":      j.ResourceARN,
			"IamRoleArn":       j.IamRoleARN,
			"State":            j.State,
			"PercentDone":      j.PercentDone,
			"CreationDate":     isoMilli(j.CreatedAt),
			"RecoveryPointArn": j.RecoveryPointARN,
		}
		if j.CompletionDate > 0 {
			item["CompletionDate"] = isoMilli(j.CompletionDate)
		}
		items = append(items, item)
	}
	return json.Marshal(map[string]any{"BackupJobs": items})
}

func backupSelectionResources(sel store.BackupSelection) []string {
	var resources []string
	if sel.ResourcesJSON != "" {
		_ = json.Unmarshal([]byte(sel.ResourcesJSON), &resources)
	}
	if resources == nil {
		resources = []string{}
	}
	return resources
}

// CreateBackupSelectionJSON builds CreateBackupSelection response.
func CreateBackupSelectionJSON(sel store.BackupSelection) ([]byte, error) {
	return json.Marshal(map[string]any{
		"BackupPlanId": sel.BackupPlanID,
		"SelectionId":  sel.SelectionID,
		"CreationDate": isoMilli(sel.CreatedAt),
	})
}

// GetBackupSelectionJSON builds GetBackupSelection response.
func GetBackupSelectionJSON(sel store.BackupSelection) ([]byte, error) {
	return json.Marshal(map[string]any{
		"BackupPlanId": sel.BackupPlanID,
		"SelectionId":  sel.SelectionID,
		"CreationDate": isoMilli(sel.CreatedAt),
		"BackupSelection": map[string]any{
			"SelectionName": sel.SelectionName,
			"IamRoleArn":    sel.IamRoleARN,
			"Resources":     backupSelectionResources(sel),
		},
	})
}

// ListBackupSelectionsJSON builds ListBackupSelections response.
func ListBackupSelectionsJSON(selections []store.BackupSelection) ([]byte, error) {
	items := make([]map[string]any, 0, len(selections))
	for _, sel := range selections {
		items = append(items, map[string]any{
			"BackupPlanId":  sel.BackupPlanID,
			"SelectionId":   sel.SelectionID,
			"SelectionName": sel.SelectionName,
			"IamRoleArn":    sel.IamRoleARN,
			"CreationDate":  isoMilli(sel.CreatedAt),
		})
	}
	return json.Marshal(map[string]any{"BackupSelectionsList": items})
}
