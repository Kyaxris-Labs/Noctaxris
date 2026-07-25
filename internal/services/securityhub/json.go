package securityhub

import (
	"encoding/json"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// BatchImportFindingsJSON builds BatchImportFindings response.
func BatchImportFindingsJSON(succeeded []string, failed []map[string]any) ([]byte, error) {
	if succeeded == nil {
		succeeded = []string{}
	}
	if failed == nil {
		failed = []map[string]any{}
	}
	return json.Marshal(map[string]any{
		"FailedCount":    len(failed),
		"SuccessCount":   len(succeeded),
		"FailedFindings": failed,
	})
}

// GetFindingsJSON builds GetFindings response.
func GetFindingsJSON(findings []store.SecurityHubFinding) ([]byte, error) {
	if findings == nil {
		findings = []store.SecurityHubFinding{}
	}
	return json.Marshal(map[string]any{"Findings": findings})
}
