package store_test

import (
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestGuardDutyInjectListGetFindings(t *testing.T) {
	st := openKMSStore(t)
	det, err := st.CreateGuardDutyDetector("000000000001")
	if err != nil {
		t.Fatal(err)
	}
	ids, err := st.InjectGuardDutyFindings("000000000001", det.DetectorID, "us-east-1", []store.GuardDutyFinding{
		{Type: "UnauthorizedAccess:IAMUser/InstanceCredentialExfiltration.InsideAWS", Severity: 8, Title: "cred exfil"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 {
		t.Fatalf("ids=%v", ids)
	}
	listed, err := st.ListGuardDutyFindingIDs("000000000001", det.DetectorID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0] != ids[0] {
		t.Fatalf("listed=%v want %v", listed, ids)
	}
	got, err := st.GetGuardDutyFindings("000000000001", det.DetectorID, ids)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Type == "" || got[0].Arn == "" {
		t.Fatalf("finding=%+v", got)
	}
}

func TestSecurityHubBatchImportAndGet(t *testing.T) {
	st := openKMSStore(t)
	ok, failed, err := st.BatchImportSecurityHubFindings("000000000001", "us-east-1", []store.SecurityHubFinding{
		{
			GeneratorId: "noctaxris-lab",
			Types:       []string{"Software and Configuration Checks/Vulnerabilities/CVE"},
			Title:       "lab finding",
			Description: "test",
			Severity:    map[string]any{"Label": "HIGH"},
			Resources: []map[string]any{{
				"Type": "AwsS3Bucket",
				"Id":   "arn:aws:s3:::lab",
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(ok) != 1 || len(failed) != 0 {
		t.Fatalf("ok=%v failed=%v", ok, failed)
	}
	findings, err := st.GetSecurityHubFindings("000000000001", store.SecurityHubFindingsFilter{
		SeverityLabel: "HIGH",
		ResourceType:  "AwsS3Bucket",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].Title != "lab finding" {
		t.Fatalf("findings=%+v", findings)
	}
}
