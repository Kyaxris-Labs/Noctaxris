package store

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// AthenaDuckTable is Glue metadata for DuckDB CREATE VIEW injection.
type AthenaDuckTable struct {
	Name             string
	Location         string
	InputFormat      string
	SerializationLib string
}

// AthenaDuckRunner executes SQL after Glue view setup (HTTP or nested DinD exec).
type AthenaDuckRunner func(querySQL, setupSQL string) (cols []AthenaColumnInfo, rows [][]string, err error)

// ListAthenaDuckTables returns Glue tables for a database (DuckDB view injection).
func (s *Store) ListAthenaDuckTables(accountID, database string) ([]AthenaDuckTable, error) {
	tables, err := s.GetGlueTables(accountID, database)
	if err != nil {
		return nil, err
	}
	out := make([]AthenaDuckTable, 0, len(tables))
	for _, t := range tables {
		out = append(out, AthenaDuckTable{
			Name:             t.Name,
			Location:         t.StorageLocation,
			InputFormat:      t.InputFormat,
			SerializationLib: t.SerDeInfo.SerializationLibrary,
		})
	}
	return out, nil
}

// StartAthenaDuckQueryExecution runs user SQL via DuckDB after injecting Glue views.
// Fail-closed: runner errors mark the execution FAILED (persisted).
func (s *Store) StartAthenaDuckQueryExecution(accountID string, in AthenaStartInput, run AthenaDuckRunner) (AthenaQueryExecution, error) {
	if run == nil {
		return AthenaQueryExecution{}, fmt.Errorf("%w: DuckDB runner is required", ErrAthenaBadRequest)
	}
	q := strings.TrimSpace(in.QueryString)
	if q == "" {
		return AthenaQueryExecution{}, fmt.Errorf("%w: QueryString is required", ErrAthenaBadRequest)
	}
	wg := strings.TrimSpace(in.WorkGroup)
	if wg == "" {
		wg = DefaultAthenaWorkGroup
	}
	catalog := strings.TrimSpace(in.Catalog)
	if catalog == "" {
		catalog = "AwsDataCatalog"
	}
	dbName := strings.TrimSpace(in.Database)
	now := time.Now().UTC().UnixMilli()
	id := uuid.NewString()

	exec := AthenaQueryExecution{
		QueryExecutionID: id,
		QueryString:      q,
		DatabaseName:     dbName,
		CatalogName:      catalog,
		WorkGroup:        wg,
		OutputLocation:   strings.TrimSpace(in.OutputLocation),
		State:            "RUNNING",
		SubmissionMS:     now,
		ResultColumns:    []AthenaColumnInfo{},
		ResultRows:       [][]string{},
	}

	// Prefer db.table from SQL when present; else QueryExecutionContext.Database.
	parsedDB := dbName
	if m := athenaSelectRE.FindStringSubmatch(q); m != nil {
		fromPart, _ := splitAthenaFromAndSuffix(strings.TrimSpace(m[2]))
		if jm := athenaJoinRE.FindStringSubmatch(fromPart); jm != nil {
			ldb, _ := splitAthenaDBTable(strings.TrimSpace(jm[1]))
			if ldb != "" {
				parsedDB = ldb
			}
		} else if fm := athenaFromRE.FindStringSubmatch(fromPart); fm != nil {
			ldb, _ := splitAthenaDBTable(strings.TrimSpace(fm[1]))
			if ldb != "" {
				parsedDB = ldb
			}
		}
	}
	if parsedDB == "" {
		exec.State = "FAILED"
		exec.StateChangeReason = "Database is required in QueryExecutionContext or as db.table"
		exec.ErrorMessage = exec.StateChangeReason
		exec.CompletionMS = now
		if saveErr := s.saveAthenaExecution(accountID, exec); saveErr != nil {
			return AthenaQueryExecution{}, saveErr
		}
		return exec, nil
	}
	exec.DatabaseName = parsedDB

	tables, err := s.ListAthenaDuckTables(accountID, parsedDB)
	if err != nil {
		exec.State = "FAILED"
		exec.StateChangeReason = err.Error()
		exec.ErrorMessage = exec.StateChangeReason
		exec.CompletionMS = now
		if saveErr := s.saveAthenaExecution(accountID, exec); saveErr != nil {
			return AthenaQueryExecution{}, saveErr
		}
		return exec, nil
	}
	if len(tables) == 0 {
		exec.State = "FAILED"
		exec.StateChangeReason = fmt.Sprintf("TABLE_NOT_FOUND: no Glue tables in database %s", parsedDB)
		exec.ErrorMessage = exec.StateChangeReason
		exec.CompletionMS = now
		if saveErr := s.saveAthenaExecution(accountID, exec); saveErr != nil {
			return AthenaQueryExecution{}, saveErr
		}
		return exec, nil
	}

	setup := buildAthenaDuckSetupSQL(tables)
	cols, rows, runErr := run(q, setup)
	if runErr != nil {
		exec.State = "FAILED"
		exec.StateChangeReason = runErr.Error()
		exec.ErrorMessage = runErr.Error()
		exec.CompletionMS = time.Now().UTC().UnixMilli()
		if saveErr := s.saveAthenaExecution(accountID, exec); saveErr != nil {
			return AthenaQueryExecution{}, saveErr
		}
		return exec, nil
	}

	exec.ResultColumns = cols
	exec.ResultRows = athenaDuckResultRows(cols, rows)
	exec.State = "SUCCEEDED"
	exec.CompletionMS = time.Now().UTC().UnixMilli()

	if exec.OutputLocation != "" {
		if err := s.writeAthenaResultObject(accountID, exec); err != nil {
			exec.State = "FAILED"
			exec.StateChangeReason = fmt.Sprintf("OutputLocation write failed: %v", err)
			exec.ErrorMessage = exec.StateChangeReason
			exec.ResultColumns = nil
			exec.ResultRows = nil
			exec.CompletionMS = time.Now().UTC().UnixMilli()
		}
	}
	if err := s.saveAthenaExecution(accountID, exec); err != nil {
		return AthenaQueryExecution{}, err
	}
	return exec, nil
}

func buildAthenaDuckSetupSQL(tables []AthenaDuckTable) string {
	var b strings.Builder
	for _, t := range tables {
		if strings.TrimSpace(t.Name) == "" || strings.TrimSpace(t.Location) == "" {
			continue
		}
		fn := duckReadFunction(t.InputFormat, t.SerializationLib)
		loc := strings.TrimSpace(t.Location)
		lower := strings.ToLower(loc)
		if strings.HasSuffix(loc, "/") {
			loc = loc + "*"
		} else if !strings.HasSuffix(lower, ".csv") && !strings.HasSuffix(lower, ".json") &&
			!strings.HasSuffix(lower, ".parquet") && !strings.HasSuffix(lower, ".gz") &&
			!strings.Contains(loc, "*") {
			loc = strings.TrimRight(loc, "/") + "/*"
		}
		name := strings.ReplaceAll(t.Name, `"`, `""`)
		locEsc := strings.ReplaceAll(loc, `'`, `''`)
		b.WriteString(fmt.Sprintf(`CREATE OR REPLACE VIEW "%s" AS SELECT * FROM %s('%s');`, name, fn, locEsc))
		b.WriteByte('\n')
	}
	return b.String()
}

func duckReadFunction(inputFormat, serializationLib string) string {
	blob := strings.ToLower(inputFormat + " " + serializationLib)
	switch {
	case strings.Contains(blob, "parquet"):
		return "read_parquet"
	case strings.Contains(blob, "json"):
		return "read_json_auto"
	default:
		return "read_csv_auto"
	}
}

func athenaDuckResultRows(cols []AthenaColumnInfo, dataRows [][]string) [][]string {
	header := make([]string, len(cols))
	for i, c := range cols {
		header[i] = c.Name
	}
	out := make([][]string, 0, len(dataRows)+1)
	out = append(out, header)
	out = append(out, dataRows...)
	return out
}
