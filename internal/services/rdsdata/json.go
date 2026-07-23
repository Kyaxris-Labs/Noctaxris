package rdsdata

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// ApplyFormatRecordsAsJSON converts records into AWS simplified JSON (array of
// column-label objects) and clears records/columnMetadata. Lab executor markers
// in FormattedRecords are replaced by the JSON result set.
func ApplyFormatRecordsAsJSON(res store.RDSDataExecuteResult) (store.RDSDataExecuteResult, error) {
	rows := make([]map[string]any, 0, len(res.Records))
	for _, row := range res.Records {
		obj := make(map[string]any, len(row))
		for i, cell := range row {
			key := ""
			if i < len(res.ColumnMetadata) {
				key = res.ColumnMetadata[i].Label
				if key == "" {
					key = res.ColumnMetadata[i].Name
				}
			}
			if key == "" {
				key = fmt.Sprintf("column_%d", i)
			}
			obj[key] = fieldJSONScalar(cell)
		}
		rows = append(rows, obj)
	}
	raw, err := json.Marshal(rows)
	if err != nil {
		return store.RDSDataExecuteResult{}, err
	}
	res.FormattedRecords = string(raw)
	res.Records = nil
	res.ColumnMetadata = nil
	return res, nil
}

func fieldJSONScalar(cell store.RDSDataField) any {
	if cell.IsNull != nil && *cell.IsNull {
		return nil
	}
	switch {
	case cell.BooleanValue != nil:
		return *cell.BooleanValue
	case cell.LongValue != nil:
		return *cell.LongValue
	case cell.DoubleValue != nil:
		return *cell.DoubleValue
	case cell.StringValue != nil:
		return *cell.StringValue
	case cell.BlobValue != nil:
		return *cell.BlobValue
	default:
		return nil
	}
}

func fieldWireMap(cell store.RDSDataField) map[string]any {
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
	if cell.BlobValue != nil {
		m["blobValue"] = *cell.BlobValue
	}
	return m
}

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
			outRow = append(outRow, fieldWireMap(cell))
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
	if len(res.GeneratedFields) > 0 {
		gen := make([]map[string]any, 0, len(res.GeneratedFields))
		for _, cell := range res.GeneratedFields {
			gen = append(gen, fieldWireMap(cell))
		}
		body["generatedFields"] = gen
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

// BatchExecuteStatementJSON builds a BatchExecuteStatement response (AWS UpdateResult shape).
// generatedFields come from explicit GeneratedFields or the first RETURNING row in Records.
func BatchExecuteStatementJSON(results []store.RDSDataExecuteResult) ([]byte, error) {
	resps := make([]map[string]any, 0, len(results))
	for _, res := range results {
		fields := res.GeneratedFields
		if len(fields) == 0 && len(res.Records) > 0 {
			fields = res.Records[0]
		}
		gen := make([]map[string]any, 0, len(fields))
		for _, cell := range fields {
			gen = append(gen, fieldWireMap(cell))
		}
		if gen == nil {
			gen = []map[string]any{}
		}
		resps = append(resps, map[string]any{"generatedFields": gen})
	}
	return json.Marshal(map[string]any{"updateResults": resps})
}

// WantsFormatRecordsAsJSON reports whether formatRecordsAs is JSON (case-insensitive).
func WantsFormatRecordsAsJSON(format string) bool {
	return strings.EqualFold(strings.TrimSpace(format), "JSON")
}
