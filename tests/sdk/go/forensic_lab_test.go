package sdk_test

import (
	"os"
	"testing"
	"time"
)

func TestForensicLabSetClockAndBulkSeed(t *testing.T) {
	requireReady(t)
	if os.Getenv("NOCTAXRIS_LAB_FORENSICS") != "1" {
		t.Skip("set NOCTAXRIS_LAB_FORENSICS=1 on the API process for NoctaxrisLab SetClock/BulkSeed")
	}

	fixed := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	setStatus, setBody, setParsed := signedJSONTarget(t, "noctaxris", "NoctaxrisLab.SetClock", map[string]any{
		"FixedTime": fixed.Format(time.RFC3339),
	})
	if setStatus != 200 {
		t.Fatalf("SetClock status=%d body=%s", setStatus, setBody)
	}
	if clock, _ := setParsed["ClockTime"].(string); clock != fixed.Format(time.RFC3339) {
		t.Fatalf("SetClock ClockTime=%v want %s body=%s", setParsed["ClockTime"], fixed.Format(time.RFC3339), setBody)
	}

	seedStatus, seedBody, seedParsed := signedJSONTarget(t, "noctaxris", "NoctaxrisLab.BulkSeed", map[string]any{
		"ScenarioId":       "crypto-mining",
		"IncludeGuardDuty": true,
	})
	if seedStatus != 200 {
		t.Fatalf("BulkSeed status=%d body=%s", seedStatus, seedBody)
	}
	if sid, _ := seedParsed["ScenarioId"].(string); sid != "crypto-mining" {
		t.Fatalf("BulkSeed ScenarioId=%v body=%s", seedParsed["ScenarioId"], seedBody)
	}
	ctCount, _ := seedParsed["CloudTrailEventCount"].(float64)
	if ctCount < 1 {
		t.Fatalf("BulkSeed CloudTrailEventCount=%v body=%s", seedParsed["CloudTrailEventCount"], seedBody)
	}
	gdCount, _ := seedParsed["GuardDutyFindingCount"].(float64)
	if gdCount < 1 {
		t.Fatalf("BulkSeed GuardDutyFindingCount=%v body=%s", seedParsed["GuardDutyFindingCount"], seedBody)
	}
}
