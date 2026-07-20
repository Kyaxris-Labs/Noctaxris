package rdsdata

import (
	"encoding/json"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// ExecuteStatementJSON builds an ExecuteStatement response.
func ExecuteStatementJSON(res store.RDSDataExecuteResult) ([]byte, error) {
	cols := make([]map[string]any, 0, len(res.ColumnMetadata))
	for _, c := range res.ColumnMetadata {
		cols = append(cols, map[string]any{
			"name":     c.Name,
			"typeName": c.TypeName,
			"label":    c.Label,
		})
	}
	records := make([][]map[string]any, 0, len(res.Records))
	for _, row := range res.Records {
		outRow := make([]map[string]any, 0, len(row))
		for _, cell := range row {
			m := map[string]any{}
			if cell.IsNull != nil && *cell.IsNull {
				m["isNull"] = true
			}
			if cell.StringValue != nil {
				m["stringValue"] = *cell.StringValue
			}
			if cell.LongValue != nil {
				m["longValue"] = *cell.LongValue
			}
			if cell.DoubleValue != nil {
				m["doubleValue"] = *cell.DoubleValue
			}
			if cell.BooleanValue != nil {
				m["booleanValue"] = *cell.BooleanValue
			}
			outRow = append(outRow, m)
		}
		records = append(records, outRow)
	}
	body := map[string]any{
		"numberOfRecordsUpdated": res.NumberOfRecordsUpdated,
		"records":                records,
		"columnMetadata":         cols,
	}
	if res.FormattedRecords != "" {
		body["formattedRecords"] = res.FormattedRecords
	}
	return json.Marshal(body)
}

// BeginTransactionJSON builds a BeginTransaction response.
func BeginTransactionJSON(transactionID string) ([]byte, error) {
	return json.Marshal(map[string]any{"transactionId": transactionID})
}

// CommitTransactionJSON builds a CommitTransaction response.
func CommitTransactionJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"transactionStatus": "Transaction Committed"})
}

// RollbackTransactionJSON builds a RollbackTransaction response.
func RollbackTransactionJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"transactionStatus": "Rollback Complete"})
}
