package store

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrAthenaNotFound   = errors.New("InvalidRequestException")
	ErrAthenaBadRequest = errors.New("InvalidRequestException")
)

const DefaultAthenaRegion = "us-east-1"
const DefaultAthenaWorkGroup = "primary"

const athenaSchema = `
CREATE TABLE IF NOT EXISTS athena_query_executions (
  account_id TEXT NOT NULL,
  query_execution_id TEXT NOT NULL,
  query_string TEXT NOT NULL,
  database_name TEXT NOT NULL DEFAULT '',
  catalog_name TEXT NOT NULL DEFAULT 'AwsDataCatalog',
  work_group TEXT NOT NULL DEFAULT 'primary',
  output_location TEXT NOT NULL DEFAULT '',
  state TEXT NOT NULL,
  state_change_reason TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  result_columns_json TEXT NOT NULL DEFAULT '[]',
  result_rows_json TEXT NOT NULL DEFAULT '[]',
  submission_ms INTEGER NOT NULL,
  completion_ms INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (account_id, query_execution_id)
);
`

// AthenaQueryExecution is a persisted Athena query execution.
type AthenaQueryExecution struct {
	QueryExecutionID  string
	QueryString       string
	DatabaseName      string
	CatalogName       string
	WorkGroup         string
	OutputLocation    string
	State             string
	StateChangeReason string
	ErrorMessage      string
	ResultColumns     []AthenaColumnInfo
	ResultRows        [][]string
	SubmissionMS      int64
	CompletionMS      int64
}

// AthenaColumnInfo is GetQueryResults ColumnInfo subset.
type AthenaColumnInfo struct {
	Name string `json:"Name"`
	Type string `json:"Type"`
}

// AthenaStartInput is StartQueryExecution input.
type AthenaStartInput struct {
	QueryString    string
	Database       string
	Catalog        string
	WorkGroup      string
	OutputLocation string
}

var athenaSelectRE = regexp.MustCompile(`(?is)^\s*SELECT\s+(.+?)\s+FROM\s+([^\s;]+)\s*(?:LIMIT\s+(\d+))?\s*;?\s*$`)

// EnsureAthenaSchema creates Athena tables if missing.
func EnsureAthenaSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure athena schema: db is nil")
	}
	if _, err := db.Exec(athenaSchema); err != nil {
		return fmt.Errorf("ensure athena schema: %w", err)
	}
	return nil
}

// EnsureAthenaSchema ensures Athena tables on an open store.
func (s *Store) EnsureAthenaSchema() error {
	return EnsureAthenaSchema(s.db)
}

// StartAthenaQueryExecution runs an in-process SELECT subset and persists the execution.
func (s *Store) StartAthenaQueryExecution(accountID string, in AthenaStartInput) (AthenaQueryExecution, error) {
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

	parsed, err := parseAthenaSelect(q)
	if err != nil {
		exec.State = "FAILED"
		exec.StateChangeReason = err.Error()
		exec.ErrorMessage = err.Error()
		exec.CompletionMS = now
		if saveErr := s.saveAthenaExecution(accountID, exec); saveErr != nil {
			return AthenaQueryExecution{}, saveErr
		}
		return exec, nil
	}
	if parsed.Database == "" {
		parsed.Database = dbName
	}
	if parsed.Database == "" {
		exec.State = "FAILED"
		exec.StateChangeReason = "Database is required in QueryExecutionContext or as db.table"
		exec.ErrorMessage = exec.StateChangeReason
		exec.CompletionMS = now
		if saveErr := s.saveAthenaExecution(accountID, exec); saveErr != nil {
			return AthenaQueryExecution{}, saveErr
		}
		return exec, nil
	}
	exec.DatabaseName = parsed.Database

	table, err := s.GetGlueTable(accountID, parsed.Database, parsed.Table)
	if err != nil {
		exec.State = "FAILED"
		if errors.Is(err, ErrGlueNotFound) {
			exec.StateChangeReason = fmt.Sprintf("TABLE_NOT_FOUND: %s.%s", parsed.Database, parsed.Table)
		} else {
			exec.StateChangeReason = err.Error()
		}
		exec.ErrorMessage = exec.StateChangeReason
		exec.CompletionMS = now
		if saveErr := s.saveAthenaExecution(accountID, exec); saveErr != nil {
			return AthenaQueryExecution{}, saveErr
		}
		return exec, nil
	}

	cols, rows, err := s.readAthenaTableRows(accountID, table, parsed)
	if err != nil {
		exec.State = "FAILED"
		exec.StateChangeReason = err.Error()
		exec.ErrorMessage = err.Error()
		exec.CompletionMS = now
		if saveErr := s.saveAthenaExecution(accountID, exec); saveErr != nil {
			return AthenaQueryExecution{}, saveErr
		}
		return exec, nil
	}
	exec.ResultColumns = cols
	exec.ResultRows = rows
	exec.State = "SUCCEEDED"
	exec.CompletionMS = time.Now().UTC().UnixMilli()

	if exec.OutputLocation != "" {
		_ = s.writeAthenaResultObject(accountID, exec)
	}
	if err := s.saveAthenaExecution(accountID, exec); err != nil {
		return AthenaQueryExecution{}, err
	}
	return exec, nil
}

type athenaParsedSelect struct {
	Columns  []string // empty or ["*"] means all
	Database string
	Table    string
	Limit    int // 0 = no limit
}

func parseAthenaSelect(q string) (athenaParsedSelect, error) {
	m := athenaSelectRE.FindStringSubmatch(q)
	if m == nil {
		return athenaParsedSelect{}, fmt.Errorf("%w: unsupported SQL (lab supports SELECT cols FROM db.table [LIMIT n])", ErrAthenaBadRequest)
	}
	colPart := strings.TrimSpace(m[1])
	fromPart := strings.TrimSpace(m[2])
	limitPart := strings.TrimSpace(m[3])

	var cols []string
	if colPart == "*" {
		cols = []string{"*"}
	} else {
		for _, c := range strings.Split(colPart, ",") {
			c = strings.TrimSpace(c)
			if c == "" {
				continue
			}
			// strip simple aliases: col AS alias → col
			if i := strings.Index(strings.ToUpper(c), " AS "); i >= 0 {
				c = strings.TrimSpace(c[:i])
			}
			cols = append(cols, c)
		}
		if len(cols) == 0 {
			return athenaParsedSelect{}, fmt.Errorf("%w: SELECT column list is empty", ErrAthenaBadRequest)
		}
	}

	dbName, table := "", fromPart
	if i := strings.LastIndex(fromPart, "."); i >= 0 {
		dbName = strings.TrimSpace(fromPart[:i])
		table = strings.TrimSpace(fromPart[i+1:])
	}
	table = strings.Trim(table, "`\"")
	dbName = strings.Trim(dbName, "`\"")
	if table == "" {
		return athenaParsedSelect{}, fmt.Errorf("%w: table name required", ErrAthenaBadRequest)
	}

	out := athenaParsedSelect{Columns: cols, Database: dbName, Table: table}
	if limitPart != "" {
		n, err := strconv.Atoi(limitPart)
		if err != nil || n < 0 {
			return athenaParsedSelect{}, fmt.Errorf("%w: invalid LIMIT", ErrAthenaBadRequest)
		}
		out.Limit = n
	}
	return out, nil
}

func (s *Store) readAthenaTableRows(accountID string, table GlueTable, parsed athenaParsedSelect) ([]AthenaColumnInfo, [][]string, error) {
	selected := table.Columns
	if len(parsed.Columns) > 0 && parsed.Columns[0] != "*" {
		byName := map[string]GlueColumn{}
		for _, c := range table.Columns {
			byName[strings.ToLower(c.Name)] = c
		}
		selected = nil
		for _, name := range parsed.Columns {
			c, ok := byName[strings.ToLower(name)]
			if !ok {
				return nil, nil, fmt.Errorf("%w: column not found: %s", ErrAthenaBadRequest, name)
			}
			selected = append(selected, c)
		}
	}
	colInfos := make([]AthenaColumnInfo, 0, len(selected))
	for _, c := range selected {
		typ := c.Type
		if typ == "" {
			typ = "varchar"
		}
		colInfos = append(colInfos, AthenaColumnInfo{Name: c.Name, Type: typ})
	}

	bucket, prefix, err := parseS3Location(table.StorageLocation)
	if err != nil {
		return nil, nil, err
	}
	listed, err := s.ListObjectsV2(accountID, bucket, prefix, "")
	if err != nil {
		if errors.Is(err, ErrNoSuchBucket) {
			return colInfos, [][]string{}, nil
		}
		return nil, nil, err
	}

	jsonMode := isGlueJSON(table)
	var dataRows [][]string
	for _, obj := range listed.Contents {
		_, data, err := s.GetObject(accountID, bucket, obj.Key)
		if err != nil {
			continue
		}
		if jsonMode {
			rows, err := parseJSONLines(data, selected)
			if err != nil {
				return nil, nil, err
			}
			dataRows = append(dataRows, rows...)
		} else {
			rows, err := parseCSVRows(data, selected)
			if err != nil {
				return nil, nil, err
			}
			dataRows = append(dataRows, rows...)
		}
		if parsed.Limit > 0 && len(dataRows) >= parsed.Limit {
			break
		}
	}
	if parsed.Limit > 0 && len(dataRows) > parsed.Limit {
		dataRows = dataRows[:parsed.Limit]
	}

	// Athena includes a header row matching column names as the first ResultSet row.
	header := make([]string, len(colInfos))
	for i, c := range colInfos {
		header[i] = c.Name
	}
	out := make([][]string, 0, 1+len(dataRows))
	out = append(out, header)
	out = append(out, dataRows...)
	return colInfos, out, nil
}

func isGlueJSON(table GlueTable) bool {
	lib := strings.ToLower(table.SerDeInfo.SerializationLibrary)
	inFmt := strings.ToLower(table.InputFormat)
	if strings.Contains(lib, "json") || strings.Contains(inFmt, "json") {
		return true
	}
	return false
}

func parseS3Location(loc string) (bucket, prefix string, err error) {
	loc = strings.TrimSpace(loc)
	if !strings.HasPrefix(strings.ToLower(loc), "s3://") {
		return "", "", fmt.Errorf("%w: StorageLocation must be s3://bucket/prefix", ErrAthenaBadRequest)
	}
	rest := loc[5:]
	parts := strings.SplitN(rest, "/", 2)
	bucket = parts[0]
	if bucket == "" {
		return "", "", fmt.Errorf("%w: StorageLocation missing bucket", ErrAthenaBadRequest)
	}
	if len(parts) == 2 {
		prefix = parts[1]
	}
	return bucket, prefix, nil
}

func parseCSVRows(data []byte, columns []GlueColumn) ([][]string, error) {
	r := csv.NewReader(strings.NewReader(string(data)))
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true
	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("%w: csv parse: %v", ErrAthenaBadRequest, err)
	}
	if len(records) == 0 {
		return nil, nil
	}
	header := records[0]
	idx := map[string]int{}
	for i, h := range header {
		idx[strings.ToLower(strings.TrimSpace(h))] = i
	}
	// If header does not match column names, treat first row as data (positional).
	headerMatch := true
	for _, c := range columns {
		if _, ok := idx[strings.ToLower(c.Name)]; !ok {
			headerMatch = false
			break
		}
	}
	start := 0
	if headerMatch {
		start = 1
	}
	var out [][]string
	for _, rec := range records[start:] {
		row := make([]string, len(columns))
		for i, c := range columns {
			if headerMatch {
				if j, ok := idx[strings.ToLower(c.Name)]; ok && j < len(rec) {
					row[i] = rec[j]
				}
			} else if i < len(rec) {
				row[i] = rec[i]
			}
		}
		out = append(out, row)
	}
	return out, nil
}

func parseJSONLines(data []byte, columns []GlueColumn) ([][]string, error) {
	text := strings.TrimSpace(string(data))
	if text == "" {
		return nil, nil
	}
	// Single JSON array
	if strings.HasPrefix(text, "[") {
		var arr []map[string]any
		if err := json.Unmarshal([]byte(text), &arr); err != nil {
			return nil, fmt.Errorf("%w: json array parse: %v", ErrAthenaBadRequest, err)
		}
		out := make([][]string, 0, len(arr))
		for _, obj := range arr {
			out = append(out, jsonObjectRow(obj, columns))
		}
		return out, nil
	}
	var out [][]string
	dec := json.NewDecoder(strings.NewReader(text))
	for {
		var obj map[string]any
		if err := dec.Decode(&obj); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			// fallback: line-oriented
			lines := strings.Split(text, "\n")
			out = nil
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				var m map[string]any
				if err := json.Unmarshal([]byte(line), &m); err != nil {
					return nil, fmt.Errorf("%w: json line parse: %v", ErrAthenaBadRequest, err)
				}
				out = append(out, jsonObjectRow(m, columns))
			}
			return out, nil
		}
		out = append(out, jsonObjectRow(obj, columns))
	}
	return out, nil
}

func jsonObjectRow(obj map[string]any, columns []GlueColumn) []string {
	row := make([]string, len(columns))
	for i, c := range columns {
		v, ok := obj[c.Name]
		if !ok {
			// case-insensitive fallback
			for k, vv := range obj {
				if strings.EqualFold(k, c.Name) {
					v, ok = vv, true
					break
				}
			}
		}
		if !ok || v == nil {
			row[i] = ""
			continue
		}
		switch t := v.(type) {
		case string:
			row[i] = t
		case float64:
			row[i] = strconv.FormatFloat(t, 'f', -1, 64)
		case bool:
			row[i] = strconv.FormatBool(t)
		default:
			b, _ := json.Marshal(t)
			row[i] = string(b)
		}
	}
	return row
}

func (s *Store) writeAthenaResultObject(accountID string, exec AthenaQueryExecution) error {
	bucket, keyPrefix, err := parseS3Location(exec.OutputLocation)
	if err != nil {
		return err
	}
	if _, err := s.GetBucket(accountID, bucket); err != nil {
		return err
	}
	key := strings.TrimSuffix(keyPrefix, "/")
	if key != "" {
		key += "/"
	}
	key += exec.QueryExecutionID + ".csv"
	var b strings.Builder
	w := csv.NewWriter(&b)
	for _, row := range exec.ResultRows {
		_ = w.Write(row)
	}
	w.Flush()
	_, err = s.PutObject(accountID, bucket, key, PutObjectMeta{
		Data:        []byte(b.String()),
		PlainSize:   int64(b.Len()),
		ContentType: "text/csv",
	})
	return err
}

func (s *Store) saveAthenaExecution(accountID string, exec AthenaQueryExecution) error {
	colJSON, err := json.Marshal(exec.ResultColumns)
	if err != nil {
		return fmt.Errorf("marshal athena columns: %w", err)
	}
	rowJSON, err := json.Marshal(exec.ResultRows)
	if err != nil {
		return fmt.Errorf("marshal athena rows: %w", err)
	}
	_, err = s.db.Exec(
		`INSERT INTO athena_query_executions (
		   account_id, query_execution_id, query_string, database_name, catalog_name, work_group,
		   output_location, state, state_change_reason, error_message, result_columns_json, result_rows_json,
		   submission_ms, completion_ms)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(account_id, query_execution_id) DO UPDATE SET
		   state = excluded.state,
		   state_change_reason = excluded.state_change_reason,
		   error_message = excluded.error_message,
		   result_columns_json = excluded.result_columns_json,
		   result_rows_json = excluded.result_rows_json,
		   completion_ms = excluded.completion_ms`,
		accountID, exec.QueryExecutionID, exec.QueryString, exec.DatabaseName, exec.CatalogName, exec.WorkGroup,
		exec.OutputLocation, exec.State, exec.StateChangeReason, exec.ErrorMessage, string(colJSON), string(rowJSON),
		exec.SubmissionMS, exec.CompletionMS,
	)
	if err != nil {
		return fmt.Errorf("save athena execution: %w", err)
	}
	return nil
}

// GetAthenaQueryExecution returns a query execution by id.
func (s *Store) GetAthenaQueryExecution(accountID, id string) (AthenaQueryExecution, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return AthenaQueryExecution{}, fmt.Errorf("%w: QueryExecutionId is required", ErrAthenaBadRequest)
	}
	var exec AthenaQueryExecution
	var colJSON, rowJSON string
	err := s.db.QueryRow(
		`SELECT query_execution_id, query_string, database_name, catalog_name, work_group, output_location,
		        state, state_change_reason, error_message, result_columns_json, result_rows_json, submission_ms, completion_ms
		 FROM athena_query_executions WHERE account_id = ? AND query_execution_id = ?`,
		accountID, id,
	).Scan(
		&exec.QueryExecutionID, &exec.QueryString, &exec.DatabaseName, &exec.CatalogName, &exec.WorkGroup, &exec.OutputLocation,
		&exec.State, &exec.StateChangeReason, &exec.ErrorMessage, &colJSON, &rowJSON, &exec.SubmissionMS, &exec.CompletionMS,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return AthenaQueryExecution{}, ErrAthenaNotFound
	}
	if err != nil {
		return AthenaQueryExecution{}, fmt.Errorf("get athena execution: %w", err)
	}
	_ = json.Unmarshal([]byte(colJSON), &exec.ResultColumns)
	_ = json.Unmarshal([]byte(rowJSON), &exec.ResultRows)
	if exec.ResultColumns == nil {
		exec.ResultColumns = []AthenaColumnInfo{}
	}
	if exec.ResultRows == nil {
		exec.ResultRows = [][]string{}
	}
	return exec, nil
}

// GetAthenaQueryResults returns stored result columns and rows for a SUCCEEDED query.
func (s *Store) GetAthenaQueryResults(accountID, id string) (AthenaQueryExecution, error) {
	exec, err := s.GetAthenaQueryExecution(accountID, id)
	if err != nil {
		return AthenaQueryExecution{}, err
	}
	if exec.State == "FAILED" || exec.State == "CANCELLED" {
		return AthenaQueryExecution{}, fmt.Errorf("%w: QueryExecution state is %s", ErrAthenaBadRequest, exec.State)
	}
	if exec.State != "SUCCEEDED" {
		return AthenaQueryExecution{}, fmt.Errorf("%w: QueryExecution is not complete", ErrAthenaBadRequest)
	}
	return exec, nil
}

// StopAthenaQueryExecution marks a running query cancelled (idempotent for terminal states).
func (s *Store) StopAthenaQueryExecution(accountID, id string) error {
	exec, err := s.GetAthenaQueryExecution(accountID, id)
	if err != nil {
		return err
	}
	if exec.State == "SUCCEEDED" || exec.State == "FAILED" || exec.State == "CANCELLED" {
		return nil
	}
	exec.State = "CANCELLED"
	exec.StateChangeReason = "Query cancelled by user"
	exec.CompletionMS = time.Now().UTC().UnixMilli()
	return s.saveAthenaExecution(accountID, exec)
}
