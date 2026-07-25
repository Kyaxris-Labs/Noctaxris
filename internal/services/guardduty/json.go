package guardduty

import (
	"encoding/json"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// CreateDetectorJSON builds CreateDetector response.
func CreateDetectorJSON(detectorID string) ([]byte, error) {
	return json.Marshal(map[string]any{"DetectorId": detectorID})
}

// ListDetectorsJSON builds ListDetectors response.
func ListDetectorsJSON(ids []string) ([]byte, error) {
	if ids == nil {
		ids = []string{}
	}
	return json.Marshal(map[string]any{"DetectorIds": ids})
}

// ListFindingsJSON builds ListFindings response.
func ListFindingsJSON(ids []string) ([]byte, error) {
	if ids == nil {
		ids = []string{}
	}
	return json.Marshal(map[string]any{"FindingIds": ids})
}

// GetFindingsJSON builds GetFindings response (camelCase per GuardDuty API).
func GetFindingsJSON(findings []store.GuardDutyFinding) ([]byte, error) {
	if findings == nil {
		findings = []store.GuardDutyFinding{}
	}
	return json.Marshal(map[string]any{"Findings": findings})
}

// InjectFindingsJSON builds lab InjectFindings response.
func InjectFindingsJSON(ids []string) ([]byte, error) {
	if ids == nil {
		ids = []string{}
	}
	return json.Marshal(map[string]any{"FindingIds": ids})
}
