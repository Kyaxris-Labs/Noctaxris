package store

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
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

var (
	athenaSelectRE = regexp.MustCompile(`(?is)^\s*SELECT\s+(.+)\s+FROM\s+(.+)$`)
	athenaWhereRE  = regexp.MustCompile(`(?is)^WHERE\s+((?:[a-zA-Z_][a-zA-Z0-9_]*\.)?[a-zA-Z_][a-zA-Z0-9_]*)\s*=\s*'([^']*)'\s*(.*)$`)
	athenaLimitRE  = regexp.MustCompile(`(?is)^LIMIT\s+(\d+)\s*(.*)$`)
	athenaGroupRE  = regexp.MustCompile(`(?is)^GROUP\s+BY\s+((?:[a-zA-Z_][a-zA-Z0-9_]*\.)?[a-zA-Z_][a-zA-Z0-9_]*)\s*(.*)$`)
	athenaOrderRE  = regexp.MustCompile(`(?is)^ORDER\s+BY\s+((?:[a-zA-Z_][a-zA-Z0-9_]*\.)?[a-zA-Z_][a-zA-Z0-9_]*)(?:\s+(ASC|DESC))?\s*(.*)$`)
	athenaJoinRE   = regexp.MustCompile(`(?is)^([^\s]+)\s+([a-zA-Z_][a-zA-Z0-9_]*)\s+(?:INNER\s+)?JOIN\s+([^\s]+)\s+([a-zA-Z_][a-zA-Z0-9_]*)\s+ON\s+([a-zA-Z_][a-zA-Z0-9_]*)\.([a-zA-Z_][a-zA-Z0-9_]*)\s*=\s*([a-zA-Z_][a-zA-Z0-9_]*)\.([a-zA-Z_][a-zA-Z0-9_]*)\s*$`)
	athenaFromRE   = regexp.MustCompile(`(?is)^([^\s]+)(?:\s+([a-zA-Z_][a-zA-Z0-9_]*))?\s*$`)
)

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

	var cols []AthenaColumnInfo
	var rows [][]string
	if parsed.JoinTable != "" {
		joinDB := parsed.JoinDatabase
		if joinDB == "" {
			joinDB = parsed.Database
		}
		right, jerr := s.GetGlueTable(accountID, joinDB, parsed.JoinTable)
		if jerr != nil {
			exec.State = "FAILED"
			if errors.Is(jerr, ErrGlueNotFound) {
				exec.StateChangeReason = fmt.Sprintf("TABLE_NOT_FOUND: %s.%s", joinDB, parsed.JoinTable)
			} else {
				exec.StateChangeReason = jerr.Error()
			}
			exec.ErrorMessage = exec.StateChangeReason
			exec.CompletionMS = now
			if saveErr := s.saveAthenaExecution(accountID, exec); saveErr != nil {
				return AthenaQueryExecution{}, saveErr
			}
			return exec, nil
		}
		cols, rows, err = s.readAthenaJoinRows(accountID, table, right, parsed)
	} else {
		cols, rows, err = s.readAthenaTableRows(accountID, table, parsed)
	}
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

type athenaParsedSelect struct {
	Columns     []string // empty or ["*"] means all
	Database    string
	Table       string
	Limit       int // 0 = no limit
	WhereColumn string
	WhereValue  string
	CountStar   bool
	OrderColumn string
	OrderDesc   bool
	GroupColumn string
	// Join fields (empty TableAlias means no join)
	TableAlias     string
	JoinDatabase   string
	JoinTable      string
	JoinAlias      string
	JoinLeftCol    string
	JoinRightCol   string
	JoinLeftAlias  string
	JoinRightAlias string
}

const athenaSQLHelp = "unsupported SQL (lab supports SELECT cols|COUNT(*) FROM db.table [alias] [JOIN|INNER JOIN db.t2 b ON a.x = b.x] [WHERE col = 'literal'] [GROUP BY col] [ORDER BY col [ASC|DESC]] [LIMIT n])"

func parseAthenaSelect(q string) (athenaParsedSelect, error) {
	q = strings.TrimSpace(q)
	q = strings.TrimSuffix(q, ";")
	q = strings.TrimSpace(q)
	m := athenaSelectRE.FindStringSubmatch(q)
	if m == nil {
		return athenaParsedSelect{}, fmt.Errorf("%w: %s", ErrAthenaBadRequest, athenaSQLHelp)
	}
	colPart := strings.TrimSpace(m[1])
	fromAndSuffix := strings.TrimSpace(m[2])
	fromPart, suffix := splitAthenaFromAndSuffix(fromAndSuffix)

	var cols []string
	countStar := false
	colNorm := strings.ReplaceAll(strings.ToLower(colPart), " ", "")
	if colNorm == "count(*)" {
		countStar = true
	} else if colPart == "*" {
		cols = []string{"*"}
	} else {
		for _, c := range strings.Split(colPart, ",") {
			c = strings.TrimSpace(c)
			if c == "" {
				continue
			}
			if i := strings.Index(strings.ToUpper(c), " AS "); i >= 0 {
				c = strings.TrimSpace(c[:i])
			}
			if strings.ReplaceAll(strings.ToLower(c), " ", "") == "count(*)" {
				countStar = true
				continue
			}
			cols = append(cols, c)
		}
		if !countStar && len(cols) == 0 {
			return athenaParsedSelect{}, fmt.Errorf("%w: SELECT column list is empty", ErrAthenaBadRequest)
		}
	}

	out := athenaParsedSelect{Columns: cols, CountStar: countStar}
	fromUpper := strings.ToUpper(fromPart)
	if strings.Contains(fromUpper, " LEFT JOIN ") || strings.Contains(fromUpper, " RIGHT JOIN ") ||
		strings.Contains(fromUpper, " FULL JOIN ") || strings.Contains(fromUpper, " CROSS JOIN ") ||
		strings.Contains(fromUpper, " OUTER JOIN ") {
		return athenaParsedSelect{}, fmt.Errorf("%w: %s", ErrAthenaBadRequest, athenaSQLHelp)
	}
	if jm := athenaJoinRE.FindStringSubmatch(fromPart); jm != nil {
		leftRef, leftAlias := strings.TrimSpace(jm[1]), strings.TrimSpace(jm[2])
		rightRef, rightAlias := strings.TrimSpace(jm[3]), strings.TrimSpace(jm[4])
		onLeftAlias, onLeftCol := strings.TrimSpace(jm[5]), strings.TrimSpace(jm[6])
		onRightAlias, onRightCol := strings.TrimSpace(jm[7]), strings.TrimSpace(jm[8])
		ldb, ltbl := splitAthenaDBTable(leftRef)
		rdb, rtbl := splitAthenaDBTable(rightRef)
		if ltbl == "" || rtbl == "" {
			return athenaParsedSelect{}, fmt.Errorf("%w: JOIN requires table names", ErrAthenaBadRequest)
		}
		// Normalize ON so JoinLeft* is left table and JoinRight* is right table.
		switch {
		case strings.EqualFold(onLeftAlias, leftAlias) && strings.EqualFold(onRightAlias, rightAlias):
			// already oriented
		case strings.EqualFold(onLeftAlias, rightAlias) && strings.EqualFold(onRightAlias, leftAlias):
			onLeftAlias, onLeftCol, onRightAlias, onRightCol = onRightAlias, onRightCol, onLeftAlias, onLeftCol
		default:
			return athenaParsedSelect{}, fmt.Errorf("%w: JOIN ON aliases must match table aliases", ErrAthenaBadRequest)
		}
		out.Database, out.Table, out.TableAlias = ldb, ltbl, leftAlias
		out.JoinDatabase, out.JoinTable, out.JoinAlias = rdb, rtbl, rightAlias
		out.JoinLeftAlias, out.JoinLeftCol = onLeftAlias, onLeftCol
		out.JoinRightAlias, out.JoinRightCol = onRightAlias, onRightCol
	} else if strings.Contains(fromUpper, " JOIN ") {
		return athenaParsedSelect{}, fmt.Errorf("%w: %s", ErrAthenaBadRequest, athenaSQLHelp)
	} else {
		fm := athenaFromRE.FindStringSubmatch(fromPart)
		if fm == nil {
			return athenaParsedSelect{}, fmt.Errorf("%w: %s", ErrAthenaBadRequest, athenaSQLHelp)
		}
		dbName, table := splitAthenaDBTable(strings.TrimSpace(fm[1]))
		if table == "" {
			return athenaParsedSelect{}, fmt.Errorf("%w: table name required", ErrAthenaBadRequest)
		}
		out.Database, out.Table = dbName, table
		if len(fm) > 2 {
			out.TableAlias = strings.TrimSpace(fm[2])
		}
	}

	whereCol, whereVal, limit, orderCol, orderDesc, groupCol, err := parseAthenaSelectSuffix(suffix)
	if err != nil {
		return athenaParsedSelect{}, err
	}
	out.WhereColumn, out.WhereValue, out.Limit = whereCol, whereVal, limit
	out.OrderColumn, out.OrderDesc, out.GroupColumn = orderCol, orderDesc, groupCol
	if out.GroupColumn != "" && !out.CountStar {
		hasCount := false
		for _, c := range out.Columns {
			if strings.EqualFold(strings.ReplaceAll(c, " ", ""), "COUNT(*)") {
				hasCount = true
				break
			}
		}
		if !hasCount {
			return athenaParsedSelect{}, fmt.Errorf("%w: GROUP BY requires COUNT(*) in SELECT list", ErrAthenaBadRequest)
		}
	}
	return out, nil
}

func splitAthenaDBTable(ref string) (dbName, table string) {
	ref = strings.TrimSpace(ref)
	if i := strings.LastIndex(ref, "."); i >= 0 {
		dbName = strings.Trim(strings.TrimSpace(ref[:i]), "`\"")
		table = strings.Trim(strings.TrimSpace(ref[i+1:]), "`\"")
		return dbName, table
	}
	return "", strings.Trim(ref, "`\"")
}

func splitAthenaFromAndSuffix(s string) (fromPart, suffix string) {
	upper := strings.ToUpper(s)
	cut := -1
	for _, kw := range []string{" WHERE ", " GROUP ", " ORDER ", " LIMIT "} {
		if i := strings.Index(upper, kw); i >= 0 && (cut < 0 || i < cut) {
			cut = i
		}
	}
	// also allow leading clause without preceding space when fromPart is exact
	if cut < 0 {
		for _, kw := range []string{"WHERE ", "GROUP ", "ORDER ", "LIMIT "} {
			if strings.HasPrefix(upper, kw) {
				return "", strings.TrimSpace(s)
			}
		}
		return strings.TrimSpace(s), ""
	}
	return strings.TrimSpace(s[:cut]), strings.TrimSpace(s[cut:])
}

func parseAthenaSelectSuffix(s string) (whereCol, whereVal string, limit int, orderCol string, orderDesc bool, groupCol string, err error) {
	s = strings.TrimSpace(s)
	for s != "" {
		upper := strings.ToUpper(s)
		switch {
		case strings.HasPrefix(upper, "WHERE "):
			m := athenaWhereRE.FindStringSubmatch(s)
			if m == nil {
				return "", "", 0, "", false, "", fmt.Errorf("%w: invalid WHERE clause (lab supports col = 'literal')", ErrAthenaBadRequest)
			}
			whereCol = strings.TrimSpace(m[1])
			whereVal = m[2]
			s = strings.TrimSpace(m[3])
		case strings.HasPrefix(upper, "GROUP "):
			m := athenaGroupRE.FindStringSubmatch(s)
			if m == nil {
				return "", "", 0, "", false, "", fmt.Errorf("%w: invalid GROUP BY", ErrAthenaBadRequest)
			}
			groupCol = strings.TrimSpace(m[1])
			s = strings.TrimSpace(m[2])
		case strings.HasPrefix(upper, "ORDER "):
			m := athenaOrderRE.FindStringSubmatch(s)
			if m == nil {
				return "", "", 0, "", false, "", fmt.Errorf("%w: invalid ORDER BY", ErrAthenaBadRequest)
			}
			orderCol = strings.TrimSpace(m[1])
			orderDesc = strings.EqualFold(strings.TrimSpace(m[2]), "DESC")
			s = strings.TrimSpace(m[3])
		case strings.HasPrefix(upper, "LIMIT "):
			m := athenaLimitRE.FindStringSubmatch(s)
			if m == nil {
				return "", "", 0, "", false, "", fmt.Errorf("%w: invalid LIMIT", ErrAthenaBadRequest)
			}
			n, convErr := strconv.Atoi(m[1])
			if convErr != nil || n < 0 {
				return "", "", 0, "", false, "", fmt.Errorf("%w: invalid LIMIT", ErrAthenaBadRequest)
			}
			limit = n
			s = strings.TrimSpace(m[2])
		default:
			return "", "", 0, "", false, "", fmt.Errorf("%w: unsupported SQL clause", ErrAthenaBadRequest)
		}
	}
	return whereCol, whereVal, limit, orderCol, orderDesc, groupCol, nil
}

func (s *Store) readAthenaTableRows(accountID string, table GlueTable, parsed athenaParsedSelect) ([]AthenaColumnInfo, [][]string, error) {
	loadCols := table.Columns
	selected := table.Columns
	if parsed.GroupColumn != "" {
		selected = nil
	} else if parsed.CountStar && len(parsed.Columns) == 0 {
		selected = nil
	} else if len(parsed.Columns) > 0 && parsed.Columns[0] != "*" {
		byName := map[string]GlueColumn{}
		for _, c := range table.Columns {
			byName[strings.ToLower(c.Name)] = c
		}
		selected = nil
		for _, name := range parsed.Columns {
			c, ok := byName[strings.ToLower(athenaBareCol(name))]
			if !ok {
				return nil, nil, fmt.Errorf("%w: column not found: %s", ErrAthenaBadRequest, name)
			}
			selected = append(selected, c)
		}
	}

	bucket, prefix, err := parseS3Location(table.StorageLocation)
	if err != nil {
		return nil, nil, err
	}
	listed, err := s.ListObjectsV2(accountID, bucket, prefix, "")
	if err != nil {
		if errors.Is(err, ErrNoSuchBucket) {
			return nil, nil, fmt.Errorf("%w: S3 location bucket does not exist: %s", ErrAthenaBadRequest, bucket)
		}
		return nil, nil, err
	}

	jsonMode := isGlueJSON(table)
	var dataRows [][]string
	earlyLimit := parsed.Limit > 0 && parsed.WhereColumn == "" && !parsed.CountStar && parsed.OrderColumn == "" && parsed.GroupColumn == ""
	for _, obj := range listed.Contents {
		_, data, err := s.GetObject(accountID, bucket, obj.Key)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: GetObject failed for s3://%s/%s: %v", ErrAthenaBadRequest, bucket, obj.Key, err)
		}
		if jsonMode {
			rows, err := parseJSONLines(data, loadCols)
			if err != nil {
				return nil, nil, err
			}
			dataRows = append(dataRows, rows...)
		} else {
			rows, err := parseCSVRows(data, loadCols)
			if err != nil {
				return nil, nil, err
			}
			dataRows = append(dataRows, rows...)
		}
		if earlyLimit && len(dataRows) >= parsed.Limit {
			break
		}
	}
	if parsed.WhereColumn != "" {
		dataRows, err = filterAthenaRows(dataRows, table.Columns, athenaBareCol(parsed.WhereColumn), parsed.WhereValue)
		if err != nil {
			return nil, nil, err
		}
	}
	if parsed.GroupColumn != "" {
		return athenaGroupByCount(dataRows, table.Columns, parsed)
	}
	if parsed.CountStar {
		colInfos := []AthenaColumnInfo{{Name: "_col0", Type: "bigint"}}
		out := [][]string{{"_col0"}, {strconv.Itoa(len(dataRows))}}
		return colInfos, out, nil
	}
	dataRows = projectAthenaRows(dataRows, table.Columns, selected)
	if parsed.OrderColumn != "" {
		dataRows, err = sortAthenaRows(dataRows, selected, athenaBareCol(parsed.OrderColumn), parsed.OrderDesc)
		if err != nil {
			return nil, nil, err
		}
	}
	if earlyLimit && len(dataRows) > parsed.Limit {
		dataRows = dataRows[:parsed.Limit]
	} else if !earlyLimit && parsed.Limit > 0 && len(dataRows) > parsed.Limit {
		dataRows = dataRows[:parsed.Limit]
	}

	colInfos := make([]AthenaColumnInfo, 0, len(selected))
	for _, c := range selected {
		typ := c.Type
		if typ == "" {
			typ = "varchar"
		}
		colInfos = append(colInfos, AthenaColumnInfo{Name: c.Name, Type: typ})
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

func athenaBareCol(name string) string {
	name = strings.TrimSpace(name)
	if i := strings.LastIndex(name, "."); i >= 0 {
		return name[i+1:]
	}
	return name
}

func athenaGroupByCount(dataRows [][]string, columns []GlueColumn, parsed athenaParsedSelect) ([]AthenaColumnInfo, [][]string, error) {
	groupName := athenaBareCol(parsed.GroupColumn)
	idx := -1
	for i, c := range columns {
		if strings.EqualFold(c.Name, groupName) {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, nil, fmt.Errorf("%w: GROUP BY column not found: %s", ErrAthenaBadRequest, groupName)
	}
	counts := map[string]int{}
	orderKeys := []string{}
	for _, row := range dataRows {
		if idx >= len(row) {
			continue
		}
		k := row[idx]
		if _, ok := counts[k]; !ok {
			orderKeys = append(orderKeys, k)
		}
		counts[k]++
	}
	if parsed.OrderColumn != "" && strings.EqualFold(athenaBareCol(parsed.OrderColumn), groupName) {
		sort.SliceStable(orderKeys, func(i, j int) bool {
			if parsed.OrderDesc {
				return orderKeys[i] > orderKeys[j]
			}
			return orderKeys[i] < orderKeys[j]
		})
	}

	includeKey := false
	hasCount := parsed.CountStar
	for _, c := range parsed.Columns {
		norm := strings.ReplaceAll(strings.ToLower(c), " ", "")
		if norm == "count(*)" {
			hasCount = true
			continue
		}
		if c == "*" {
			continue
		}
		if strings.EqualFold(athenaBareCol(c), groupName) {
			includeKey = true
		} else if athenaBareCol(c) != "" {
			// Lab GROUP BY SELECT may list the group key under its bare name.
			includeKey = true
		}
	}
	if hasCount && len(parsed.Columns) > 0 && !includeKey {
		// COUNT(*) was stripped from Columns at parse time; remaining cols are group keys.
		includeKey = true
	}
	if !hasCount {
		return nil, nil, fmt.Errorf("%w: GROUP BY requires COUNT(*) in SELECT list", ErrAthenaBadRequest)
	}

	var colInfos []AthenaColumnInfo
	var header []string
	if includeKey {
		typ := "varchar"
		for _, c := range columns {
			if strings.EqualFold(c.Name, groupName) && c.Type != "" {
				typ = c.Type
			}
		}
		colInfos = append(colInfos, AthenaColumnInfo{Name: groupName, Type: typ})
		header = append(header, groupName)
	}
	colInfos = append(colInfos, AthenaColumnInfo{Name: "_col0", Type: "bigint"})
	header = append(header, "_col0")

	out := [][]string{header}
	for _, k := range orderKeys {
		row := []string{}
		if includeKey {
			row = append(row, k)
		}
		row = append(row, strconv.Itoa(counts[k]))
		out = append(out, row)
	}
	if parsed.Limit > 0 && len(out)-1 > parsed.Limit {
		out = out[:1+parsed.Limit]
	}
	return colInfos, out, nil
}

func sortAthenaRows(rows [][]string, selected []GlueColumn, orderCol string, desc bool) ([][]string, error) {
	idx := -1
	for i, c := range selected {
		if strings.EqualFold(c.Name, orderCol) {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, fmt.Errorf("%w: ORDER BY column not found: %s", ErrAthenaBadRequest, orderCol)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := "", ""
		if idx < len(rows[i]) {
			a = rows[i][idx]
		}
		if idx < len(rows[j]) {
			b = rows[j][idx]
		}
		if desc {
			return a > b
		}
		return a < b
	})
	return rows, nil
}

func (s *Store) loadAthenaRawRows(accountID string, table GlueTable) ([][]string, error) {
	bucket, prefix, err := parseS3Location(table.StorageLocation)
	if err != nil {
		return nil, err
	}
	listed, err := s.ListObjectsV2(accountID, bucket, prefix, "")
	if err != nil {
		if errors.Is(err, ErrNoSuchBucket) {
			return nil, fmt.Errorf("%w: S3 location bucket does not exist: %s", ErrAthenaBadRequest, bucket)
		}
		return nil, err
	}
	jsonMode := isGlueJSON(table)
	var dataRows [][]string
	for _, obj := range listed.Contents {
		_, data, err := s.GetObject(accountID, bucket, obj.Key)
		if err != nil {
			return nil, fmt.Errorf("%w: GetObject failed for s3://%s/%s: %v", ErrAthenaBadRequest, bucket, obj.Key, err)
		}
		if jsonMode {
			rows, err := parseJSONLines(data, table.Columns)
			if err != nil {
				return nil, err
			}
			dataRows = append(dataRows, rows...)
		} else {
			rows, err := parseCSVRows(data, table.Columns)
			if err != nil {
				return nil, err
			}
			dataRows = append(dataRows, rows...)
		}
	}
	return dataRows, nil
}

func (s *Store) readAthenaJoinRows(accountID string, left, right GlueTable, parsed athenaParsedSelect) ([]AthenaColumnInfo, [][]string, error) {
	leftRows, err := s.loadAthenaRawRows(accountID, left)
	if err != nil {
		return nil, nil, err
	}
	rightRows, err := s.loadAthenaRawRows(accountID, right)
	if err != nil {
		return nil, nil, err
	}
	leftOn := athenaBareCol(parsed.JoinLeftCol)
	rightOn := athenaBareCol(parsed.JoinRightCol)
	li, ri := -1, -1
	for i, c := range left.Columns {
		if strings.EqualFold(c.Name, leftOn) {
			li = i
			break
		}
	}
	for i, c := range right.Columns {
		if strings.EqualFold(c.Name, rightOn) {
			ri = i
			break
		}
	}
	if li < 0 || ri < 0 {
		return nil, nil, fmt.Errorf("%w: JOIN ON columns not found", ErrAthenaBadRequest)
	}
	rightByKey := map[string][][]string{}
	for _, row := range rightRows {
		if ri >= len(row) {
			continue
		}
		k := row[ri]
		rightByKey[k] = append(rightByKey[k], row)
	}
	var joined [][]string
	joinedCols := append([]GlueColumn{}, left.Columns...)
	joinedCols = append(joinedCols, right.Columns...)
	for _, lrow := range leftRows {
		if li >= len(lrow) {
			continue
		}
		for _, rrow := range rightByKey[lrow[li]] {
			merged := append(append([]string{}, lrow...), rrow...)
			joined = append(joined, merged)
		}
	}
	if parsed.WhereColumn != "" {
		joined, err = filterAthenaJoinRows(joined, left, right, parsed)
		if err != nil {
			return nil, nil, err
		}
	}
	if parsed.GroupColumn != "" {
		return athenaGroupByCount(joined, joinedCols, parsed)
	}
	if parsed.CountStar {
		colInfos := []AthenaColumnInfo{{Name: "_col0", Type: "bigint"}}
		out := [][]string{{"_col0"}, {strconv.Itoa(len(joined))}}
		return colInfos, out, nil
	}
	selected, colInfos, err := resolveAthenaJoinProjection(left, right, parsed)
	if err != nil {
		return nil, nil, err
	}
	var dataRows [][]string
	for _, row := range joined {
		proj := make([]string, len(selected))
		for i, idx := range selected {
			if idx < len(row) {
				proj[i] = row[idx]
			}
		}
		dataRows = append(dataRows, proj)
	}
	if parsed.OrderColumn != "" {
		orderIdx := -1
		want := strings.ToLower(parsed.OrderColumn)
		for i, c := range colInfos {
			qualLeft := strings.ToLower(parsed.TableAlias + "." + c.Name)
			qualRight := strings.ToLower(parsed.JoinAlias + "." + c.Name)
			if strings.ToLower(c.Name) == athenaBareCol(want) || strings.ToLower(c.Name) == want || qualLeft == want || qualRight == want {
				orderIdx = i
				break
			}
		}
		if orderIdx < 0 {
			return nil, nil, fmt.Errorf("%w: ORDER BY column not found: %s", ErrAthenaBadRequest, parsed.OrderColumn)
		}
		desc := parsed.OrderDesc
		sort.SliceStable(dataRows, func(i, j int) bool {
			a, b := dataRows[i][orderIdx], dataRows[j][orderIdx]
			if desc {
				return a > b
			}
			return a < b
		})
	}
	if parsed.Limit > 0 && len(dataRows) > parsed.Limit {
		dataRows = dataRows[:parsed.Limit]
	}
	header := make([]string, len(colInfos))
	for i, c := range colInfos {
		header[i] = c.Name
	}
	out := make([][]string, 0, 1+len(dataRows))
	out = append(out, header)
	out = append(out, dataRows...)
	return colInfos, out, nil
}

func filterAthenaJoinRows(rows [][]string, left, right GlueTable, parsed athenaParsedSelect) ([][]string, error) {
	col := parsed.WhereColumn
	bare := athenaBareCol(col)
	alias := ""
	if i := strings.Index(col, "."); i >= 0 {
		alias = col[:i]
	}
	idx := -1
	if alias == "" || strings.EqualFold(alias, parsed.TableAlias) {
		for i, c := range left.Columns {
			if strings.EqualFold(c.Name, bare) {
				idx = i
				break
			}
		}
	}
	if idx < 0 && (alias == "" || strings.EqualFold(alias, parsed.JoinAlias)) {
		for i, c := range right.Columns {
			if strings.EqualFold(c.Name, bare) {
				idx = len(left.Columns) + i
				break
			}
		}
	}
	if idx < 0 {
		return nil, fmt.Errorf("%w: WHERE column not found: %s", ErrAthenaBadRequest, col)
	}
	var out [][]string
	for _, row := range rows {
		if idx < len(row) && row[idx] == parsed.WhereValue {
			out = append(out, row)
		}
	}
	return out, nil
}

func resolveAthenaJoinProjection(left, right GlueTable, parsed athenaParsedSelect) (indices []int, infos []AthenaColumnInfo, err error) {
	if len(parsed.Columns) == 0 || (len(parsed.Columns) == 1 && parsed.Columns[0] == "*") {
		for i, c := range left.Columns {
			indices = append(indices, i)
			typ := c.Type
			if typ == "" {
				typ = "varchar"
			}
			infos = append(infos, AthenaColumnInfo{Name: c.Name, Type: typ})
		}
		for i, c := range right.Columns {
			indices = append(indices, len(left.Columns)+i)
			typ := c.Type
			if typ == "" {
				typ = "varchar"
			}
			infos = append(infos, AthenaColumnInfo{Name: c.Name, Type: typ})
		}
		return indices, infos, nil
	}
	for _, name := range parsed.Columns {
		alias, bare := "", athenaBareCol(name)
		if i := strings.Index(name, "."); i >= 0 {
			alias = name[:i]
		}
		found := false
		if alias == "" || strings.EqualFold(alias, parsed.TableAlias) {
			for i, c := range left.Columns {
				if strings.EqualFold(c.Name, bare) {
					indices = append(indices, i)
					typ := c.Type
					if typ == "" {
						typ = "varchar"
					}
					infos = append(infos, AthenaColumnInfo{Name: c.Name, Type: typ})
					found = true
					break
				}
			}
		}
		if !found && (alias == "" || strings.EqualFold(alias, parsed.JoinAlias)) {
			for i, c := range right.Columns {
				if strings.EqualFold(c.Name, bare) {
					indices = append(indices, len(left.Columns)+i)
					typ := c.Type
					if typ == "" {
						typ = "varchar"
					}
					infos = append(infos, AthenaColumnInfo{Name: c.Name, Type: typ})
					found = true
					break
				}
			}
		}
		if !found {
			return nil, nil, fmt.Errorf("%w: column not found: %s", ErrAthenaBadRequest, name)
		}
	}
	return indices, infos, nil
}

func filterAthenaRows(rows [][]string, columns []GlueColumn, whereCol, whereVal string) ([][]string, error) {
	idx := -1
	for i, c := range columns {
		if strings.EqualFold(c.Name, whereCol) {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, fmt.Errorf("%w: column not found: %s", ErrAthenaBadRequest, whereCol)
	}
	var out [][]string
	for _, row := range rows {
		if idx < len(row) && row[idx] == whereVal {
			out = append(out, row)
		}
	}
	return out, nil
}

func projectAthenaRows(rows [][]string, allCols, selected []GlueColumn) [][]string {
	if len(selected) == len(allCols) {
		same := true
		for i := range selected {
			if !strings.EqualFold(selected[i].Name, allCols[i].Name) {
				same = false
				break
			}
		}
		if same {
			return rows
		}
	}
	idxByName := map[string]int{}
	for i, c := range allCols {
		idxByName[strings.ToLower(c.Name)] = i
	}
	out := make([][]string, 0, len(rows))
	for _, row := range rows {
		proj := make([]string, len(selected))
		for i, c := range selected {
			if j, ok := idxByName[strings.ToLower(c.Name)]; ok && j < len(row) {
				proj[i] = row[j]
			}
		}
		out = append(out, proj)
	}
	return out
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
