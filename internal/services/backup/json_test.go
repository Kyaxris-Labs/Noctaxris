package backup_test

import (
	"encoding/json"
	"testing"

	backupsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/backup"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestBackupSelectionAndJobsJSON(t *testing.T) {
	sel := store.BackupSelection{
		BackupPlanID:  "plan-1",
		SelectionID:   "sel-1",
		SelectionName: "lab-sel",
		IamRoleARN:    "arn:aws:iam::000000000001:role/Backup",
		ResourcesJSON: `["arn:aws:s3:::lab-bucket"]`,
		CreatedAt:     1_700_000_000_000,
	}
	createRaw, err := backupsvc.CreateBackupSelectionJSON(sel)
	if err != nil {
		t.Fatal(err)
	}
	var createOut map[string]any
	if err := json.Unmarshal(createRaw, &createOut); err != nil {
		t.Fatal(err)
	}
	if createOut["SelectionId"] != "sel-1" || createOut["BackupPlanId"] != "plan-1" {
		t.Fatalf("create=%v", createOut)
	}

	getRaw, err := backupsvc.GetBackupSelectionJSON(sel)
	if err != nil {
		t.Fatal(err)
	}
	var getOut map[string]any
	if err := json.Unmarshal(getRaw, &getOut); err != nil {
		t.Fatal(err)
	}
	bs, _ := getOut["BackupSelection"].(map[string]any)
	if bs["IamRoleArn"] != sel.IamRoleARN {
		t.Fatalf("get selection=%v", getOut)
	}

	jobs := []store.BackupJob{{
		BackupJobID: "job-1", BackupVaultName: "lab-vault", ResourceARN: "arn:aws:s3:::lab-bucket",
		State: "COMPLETED", PercentDone: "100.0", CreatedAt: 1_700_000_000_000, CompletionDate: 1_700_000_000_000,
		RecoveryPointARN: "arn:aws:backup:us-east-1:000000000001:recovery-point:rp-1",
	}}
	listRaw, err := backupsvc.ListBackupJobsJSON(jobs)
	if err != nil {
		t.Fatal(err)
	}
	var listOut map[string]any
	if err := json.Unmarshal(listRaw, &listOut); err != nil {
		t.Fatal(err)
	}
	arr, _ := listOut["BackupJobs"].([]any)
	if len(arr) != 1 {
		t.Fatalf("jobs=%v", listOut)
	}
}

func TestBackupVaultPlanRecoveryJSON(t *testing.T) {
	ts := int64(1_700_000_000_000)
	vault := store.BackupVault{
		BackupVaultName: "lab-vault", BackupVaultARN: "arn:aws:backup:us-east-1:1:vault:lab-vault",
		CreatedAt: ts, NumberOfRecoveryPoints: 2, EncryptionKeyARN: "arn:aws:kms:us-east-1:1:key/k",
	}
	for _, fn := range []struct {
		name string
		run  func() ([]byte, error)
		key  string
	}{
		{"CreateBackupVault", func() ([]byte, error) { return backupsvc.CreateBackupVaultJSON(vault) }, "BackupVaultName"},
		{"DescribeBackupVault", func() ([]byte, error) { return backupsvc.DescribeBackupVaultJSON(vault) }, "EncryptionKeyArn"},
		{"ListBackupVaults", func() ([]byte, error) { return backupsvc.ListBackupVaultsJSON([]store.BackupVault{vault}) }, "BackupVaultList"},
	} {
		t.Run(fn.name, func(t *testing.T) {
			raw, err := fn.run()
			if err != nil {
				t.Fatal(err)
			}
			var out map[string]any
			if err := json.Unmarshal(raw, &out); err != nil {
				t.Fatal(err)
			}
			if fn.key == "BackupVaultList" {
				if _, ok := out[fn.key].([]any); !ok {
					t.Fatalf("missing %s: %v", fn.key, out)
				}
				return
			}
			if out[fn.key] == nil && fn.key != "EncryptionKeyArn" {
				t.Fatalf("%s missing %s: %v", fn.name, fn.key, out)
			}
		})
	}

	plan := store.BackupPlan{
		BackupPlanID: "plan-1", BackupPlanARN: "arn:aws:backup:us-east-1:1:plan:plan-1",
		BackupPlanName: "lab-plan", VersionID: "v1", CreatedAt: ts,
		RulesJSON: `[{"RuleName":"daily","TargetBackupVaultName":"lab-vault"}]`,
	}
	if _, err := backupsvc.CreateBackupPlanJSON(plan); err != nil {
		t.Fatal(err)
	}
	getPlan, err := backupsvc.GetBackupPlanJSON(plan)
	if err != nil {
		t.Fatal(err)
	}
	var planOut map[string]any
	if err := json.Unmarshal(getPlan, &planOut); err != nil {
		t.Fatal(err)
	}
	bp, _ := planOut["BackupPlan"].(map[string]any)
	if bp["BackupPlanName"] != "lab-plan" {
		t.Fatalf("BackupPlan=%v", bp)
	}
	if _, err := backupsvc.ListBackupPlansJSON([]store.BackupPlan{plan}); err != nil {
		t.Fatal(err)
	}

	job := store.BackupJob{
		BackupJobID: "job-2", BackupVaultName: "lab-vault", ResourceARN: "arn:aws:s3:::b",
		IamRoleARN: "arn:aws:iam::1:role/Backup", State: "RUNNING", PercentDone: "50.0",
		CreatedAt: ts, CompletionDate: ts, RecoveryPointARN: "arn:aws:backup:us-east-1:1:recovery-point:rp-2",
	}
	if _, err := backupsvc.StartBackupJobJSON(job); err != nil {
		t.Fatal(err)
	}
	descJob, err := backupsvc.DescribeBackupJobJSON(job)
	if err != nil {
		t.Fatal(err)
	}
	var jobOut map[string]any
	if err := json.Unmarshal(descJob, &jobOut); err != nil {
		t.Fatal(err)
	}
	if jobOut["CompletionDate"] == nil {
		t.Fatalf("CompletionDate missing: %v", jobOut)
	}

	rp := store.BackupRecoveryPoint{
		RecoveryPointARN: "arn:aws:backup:us-east-1:1:recovery-point:rp-3",
		BackupVaultName:  "lab-vault", ResourceARN: "arn:aws:s3:::b", ResourceType: "S3",
		Status: "COMPLETED", CreationDate: ts,
	}
	if _, err := backupsvc.DescribeRecoveryPointJSON(rp, vault.BackupVaultARN); err != nil {
		t.Fatal(err)
	}
	if _, err := backupsvc.ListRecoveryPointsJSON([]store.BackupRecoveryPoint{rp}); err != nil {
		t.Fatal(err)
	}

	emptySel := store.BackupSelection{BackupPlanID: "plan-1", SelectionID: "sel-0", CreatedAt: ts}
	if _, err := backupsvc.GetBackupSelectionJSON(emptySel); err != nil {
		t.Fatal(err)
	}
	if _, err := backupsvc.ListBackupSelectionsJSON(nil); err != nil {
		t.Fatal(err)
	}
}
