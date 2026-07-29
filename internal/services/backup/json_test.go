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
