package cur

import (
	"encoding/json"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func reportMap(d store.CURReportDefinition) map[string]any {
	out := map[string]any{
		"ReportName":                 d.ReportName,
		"TimeUnit":                   d.TimeUnit,
		"Format":                     d.Format,
		"Compression":                d.Compression,
		"S3Bucket":                   d.S3Bucket,
		"S3Prefix":                   d.S3Prefix,
		"S3Region":                   d.S3Region,
		"RefreshClosedReports":       d.RefreshClosedReports,
		"AdditionalSchemaElements":   d.AdditionalSchemaElements,
		"AdditionalArtifacts":        d.AdditionalArtifacts,
		"ReportStatus": map[string]string{
			"LastStatus": d.ReportStatus,
		},
	}
	if d.ReportVersioning != "" {
		out["ReportVersioning"] = d.ReportVersioning
	}
	return out
}

// PutReportDefinitionJSON builds PutReportDefinition response.
func PutReportDefinitionJSON(d store.CURReportDefinition) ([]byte, error) {
	return json.Marshal(reportMap(d))
}

// ModifyReportDefinitionJSON builds ModifyReportDefinition response.
func ModifyReportDefinitionJSON(d store.CURReportDefinition) ([]byte, error) {
	return json.Marshal(reportMap(d))
}

// DescribeReportDefinitionsJSON builds DescribeReportDefinitions response.
func DescribeReportDefinitionsJSON(defs []store.CURReportDefinition) ([]byte, error) {
	items := make([]map[string]any, 0, len(defs))
	for _, d := range defs {
		items = append(items, reportMap(d))
	}
	return json.Marshal(map[string]any{"ReportDefinitions": items})
}

// DeleteReportDefinitionJSON builds DeleteReportDefinition response.
func DeleteReportDefinitionJSON(reportName string, deleted bool) ([]byte, error) {
	if !deleted {
		return []byte(`{}`), nil
	}
	return json.Marshal(map[string]string{
		"ResponseMessage": "Report " + reportName + " has been deleted.",
	})
}
