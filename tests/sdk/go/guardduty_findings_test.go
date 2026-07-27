package sdk_test

import (
	"os"
	"testing"
)

func TestGuardDutyCreateInjectListGetFindings(t *testing.T) {
	requireReady(t)
	if os.Getenv("NOCTAXRIS_GUARDDUTY_INJECT") != "1" {
		t.Skip("set NOCTAXRIS_GUARDDUTY_INJECT=1 on the API process for lab InjectFindings")
	}

	createStatus, createBody, createParsed := signedJSONTarget(t, "guardduty", "GuardDuty.CreateDetector", map[string]any{})
	if createStatus != 200 {
		t.Fatalf("CreateDetector status=%d body=%s", createStatus, createBody)
	}
	detectorID, _ := createParsed["DetectorId"].(string)
	if detectorID == "" {
		t.Fatalf("CreateDetector missing DetectorId: %v", createParsed)
	}

	injStatus, injBody, injParsed := signedJSONTarget(t, "guardduty", "NoctaxrisGuardDuty.InjectFindings", map[string]any{
		"DetectorId": detectorID,
		"Findings": []map[string]any{{
			"type":     "UnauthorizedAccess:IAMUser/InstanceCredentialExfiltration.InsideAWS",
			"severity": 8,
			"title":    "sdk cred exfil",
		}},
	})
	if injStatus != 200 {
		t.Fatalf("InjectFindings status=%d body=%s", injStatus, injBody)
	}
	injIDs, _ := injParsed["FindingIds"].([]any)
	if len(injIDs) != 1 {
		t.Fatalf("InjectFindings FindingIds=%v", injParsed)
	}
	findingID, _ := injIDs[0].(string)
	if findingID == "" {
		t.Fatalf("InjectFindings empty FindingId: %v", injParsed)
	}

	listStatus, listBody, listParsed := signedJSONTarget(t, "guardduty", "GuardDuty.ListFindings", map[string]any{
		"DetectorId": detectorID,
	})
	if listStatus != 200 {
		t.Fatalf("ListFindings status=%d body=%s", listStatus, listBody)
	}
	listed, _ := listParsed["FindingIds"].([]any)
	listedOK := false
	for _, id := range listed {
		if s, ok := id.(string); ok && s == findingID {
			listedOK = true
			break
		}
	}
	if !listedOK {
		t.Fatalf("ListFindings missing %s: %v", findingID, listParsed)
	}

	getStatus, getBody, getParsed := signedJSONTarget(t, "guardduty", "GuardDuty.GetFindings", map[string]any{
		"DetectorId": detectorID,
		"FindingIds": []string{findingID},
	})
	if getStatus != 200 {
		t.Fatalf("GetFindings status=%d body=%s", getStatus, getBody)
	}
	findings, _ := getParsed["Findings"].([]any)
	if len(findings) != 1 {
		t.Fatalf("GetFindings Findings=%v", getParsed)
	}
	f, _ := findings[0].(map[string]any)
	ftype, _ := f["type"].(string)
	if ftype == "" {
		t.Fatalf("GetFindings missing type: %v", f)
	}
}
