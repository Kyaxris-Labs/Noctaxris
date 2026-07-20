package athena

import (
	"encoding/json"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// StartQueryExecutionJSON builds StartQueryExecution response.
func StartQueryExecutionJSON(id string) ([]byte, error) {
	return json.Marshal(map[string]any{"QueryExecutionId": id})
}

// GetQueryExecutionJSON builds GetQueryExecution response.
func GetQueryExecutionJSON(e store.AthenaQueryExecution) ([]byte, error) {
	status := map[string]any{
		"State":             e.State,
		"SubmissionDateTime": float64(e.SubmissionMS) / 1000.0,
	}
	if e.CompletionMS > 0 {
		status["CompletionDateTime"] = float64(e.CompletionMS) / 1000.0
	}
	if e.StateChangeReason != "" {
		status["StateChangeReason"] = e.StateChangeReason
	}
	if e.ErrorMessage != "" && e.State == "FAILED" {
		status["AthenaError"] = map[string]any{
			"ErrorMessage": e.ErrorMessage,
			"ErrorCategory": 1,
			"ErrorType":     1,
			"Retryable":     false,
		}
	}
	qe := map[string]any{
		"QueryExecutionId": e.QueryExecutionID,
		"Query":            e.QueryString,
		"StatementType":    "DML",
		"WorkGroup":        e.WorkGroup,
		"Status":           status,
		"QueryExecutionContext": map[string]any{
			"Database": e.DatabaseName,
			"Catalog":  e.CatalogName,
		},
		"ResultConfiguration": map[string]any{
			"OutputLocation": e.OutputLocation,
		},
		"Statistics": map[string]any{
			"DataScannedInBytes":         0,
			"EngineExecutionTimeInMillis": maxInt64(0, e.CompletionMS-e.SubmissionMS),
			"TotalExecutionTimeInMillis":  maxInt64(0, e.CompletionMS-e.SubmissionMS),
		},
	}
	return json.Marshal(map[string]any{"QueryExecution": qe})
}

// GetQueryResultsJSON builds GetQueryResults response.
func GetQueryResultsJSON(e store.AthenaQueryExecution) ([]byte, error) {
	colInfo := make([]map[string]any, 0, len(e.ResultColumns))
	for _, c := range e.ResultColumns {
		colInfo = append(colInfo, map[string]any{
			"Name":          c.Name,
			"Label":         c.Name,
			"Type":          c.Type,
			"CatalogName":   "hive",
			"SchemaName":    "",
			"TableName":     "",
			"Precision":     0,
			"Scale":         0,
			"Nullable":      "UNKNOWN",
			"CaseSensitive": false,
		})
	}
	rows := make([]map[string]any, 0, len(e.ResultRows))
	for _, r := range e.ResultRows {
		data := make([]map[string]any, 0, len(r))
		for _, v := range r {
			data = append(data, map[string]any{"VarCharValue": v})
		}
		rows = append(rows, map[string]any{"Data": data})
	}
	return json.Marshal(map[string]any{
		"ResultSet": map[string]any{
			"ResultSetMetadata": map[string]any{"ColumnInfo": colInfo},
			"Rows":              rows,
		},
		"UpdateCount": 0,
	})
}

// StopQueryExecutionJSON is an empty OK body.
func StopQueryExecutionJSON() ([]byte, error) { return []byte(`{}`), nil }

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
