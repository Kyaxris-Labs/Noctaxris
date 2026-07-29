package store

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

const (
	// MaxSelectObjectBytes is the lab SelectObjectContent payload size cap.
	MaxSelectObjectBytes = 1 << 20 // 1 MiB
)

var (
	ErrSelectUnsupportedSQL      = errors.New("UnsupportedSelectSQL")
	ErrSelectUnsupportedFormat   = errors.New("UnsupportedSelectFormat")
	ErrSelectObjectTooLarge      = errors.New("SelectObjectTooLarge")
	ErrSelectInvalidRequest      = errors.New("InvalidRequest")
	selectSQLPattern             = regexp.MustCompile(`(?is)^\s*select\s+\*\s+from\s+s3object(?:\s+\w+)?(?:\s+limit\s+(\d+))?\s*$`)
)

// S3SelectInput describes how to parse the object for SelectObjectContent lite.
type S3SelectInput struct {
	Format         string // CSV or JSON
	Compression    string // NONE (default); others fail closed
	JSONType       string // DOCUMENT or LINES
	FileHeaderInfo string // NONE, IGNORE, USE (CSV)
	FieldDelimiter rune
	RecordDelimiter string
}

// S3SelectOutput describes the response serialization for SelectObjectContent lite.
type S3SelectOutput struct {
	Format          string // CSV or JSON
	FieldDelimiter  rune
	RecordDelimiter string
}

// S3SelectRequest is the in-process SelectObjectContent lite request.
type S3SelectRequest struct {
	Expression     string
	ExpressionType string
	Input          S3SelectInput
	Output         S3SelectOutput
}

// S3SelectResult is a simplified SelectObjectContent response (JSON rows / event-shaped).
type S3SelectResult struct {
	Records       []json.RawMessage `json:"Records"`
	BytesScanned  int64             `json:"BytesScanned"`
	BytesProcessed int64            `json:"BytesProcessed"`
	BytesReturned int64             `json:"BytesReturned"`
}

type selectSQL struct {
	limit *int
}

func parseSelectSQL(expression string) (selectSQL, error) {
	m := selectSQLPattern.FindStringSubmatch(strings.TrimSpace(expression))
	if m == nil {
		return selectSQL{}, fmt.Errorf("%w: only SELECT * FROM s3object [LIMIT n] is supported", ErrSelectUnsupportedSQL)
	}
	out := selectSQL{}
	if m[1] != "" {
		n, err := strconv.Atoi(m[1])
		if err != nil || n < 0 {
			return selectSQL{}, fmt.Errorf("%w: invalid LIMIT", ErrSelectUnsupportedSQL)
		}
		out.limit = &n
	}
	return out, nil
}

func normalizeSelectInput(in S3SelectInput) (S3SelectInput, error) {
	format := strings.ToUpper(strings.TrimSpace(in.Format))
	if format != "CSV" && format != "JSON" {
		return S3SelectInput{}, fmt.Errorf("%w: InputSerialization must be CSV or JSON", ErrSelectUnsupportedFormat)
	}
	compression := strings.ToUpper(strings.TrimSpace(in.Compression))
	if compression == "" {
		compression = "NONE"
	}
	if compression != "NONE" {
		return S3SelectInput{}, fmt.Errorf("%w: CompressionType %q is not supported (NONE only)", ErrSelectUnsupportedFormat, compression)
	}
	out := S3SelectInput{
		Format:          format,
		Compression:     compression,
		JSONType:        strings.ToUpper(strings.TrimSpace(in.JSONType)),
		FileHeaderInfo:  strings.ToUpper(strings.TrimSpace(in.FileHeaderInfo)),
		FieldDelimiter:  in.FieldDelimiter,
		RecordDelimiter: in.RecordDelimiter,
	}
	if out.Format == "CSV" {
		if out.FieldDelimiter == 0 {
			out.FieldDelimiter = ','
		}
		if out.FileHeaderInfo == "" {
			out.FileHeaderInfo = "NONE"
		}
		switch out.FileHeaderInfo {
		case "NONE", "IGNORE", "USE":
		default:
			return S3SelectInput{}, fmt.Errorf("%w: FileHeaderInfo must be NONE, IGNORE, or USE", ErrSelectInvalidRequest)
		}
	}
	if out.Format == "JSON" {
		if out.JSONType == "" {
			out.JSONType = "DOCUMENT"
		}
		if out.JSONType != "DOCUMENT" && out.JSONType != "LINES" {
			return S3SelectInput{}, fmt.Errorf("%w: JSON Type must be DOCUMENT or LINES", ErrSelectInvalidRequest)
		}
	}
	return out, nil
}

func normalizeSelectOutput(out S3SelectOutput, inputFormat string) (S3SelectOutput, error) {
	format := strings.ToUpper(strings.TrimSpace(out.Format))
	if format == "" {
		format = strings.ToUpper(strings.TrimSpace(inputFormat))
	}
	if format != "CSV" && format != "JSON" {
		return S3SelectOutput{}, fmt.Errorf("%w: OutputSerialization must be CSV or JSON", ErrSelectUnsupportedFormat)
	}
	normalized := S3SelectOutput{
		Format:          format,
		FieldDelimiter:  out.FieldDelimiter,
		RecordDelimiter: out.RecordDelimiter,
	}
	if normalized.FieldDelimiter == 0 {
		normalized.FieldDelimiter = ','
	}
	if normalized.RecordDelimiter == "" {
		normalized.RecordDelimiter = "\n"
	}
	return normalized, nil
}

// RunS3Select executes SelectObjectContent lite against plaintext object bytes.
func RunS3Select(data []byte, req S3SelectRequest) (S3SelectResult, error) {
	if int64(len(data)) > MaxSelectObjectBytes {
		return S3SelectResult{}, ErrSelectObjectTooLarge
	}
	exprType := strings.ToUpper(strings.TrimSpace(req.ExpressionType))
	if exprType != "" && exprType != "SQL" {
		return S3SelectResult{}, fmt.Errorf("%w: ExpressionType must be SQL", ErrSelectInvalidRequest)
	}
	sql, err := parseSelectSQL(req.Expression)
	if err != nil {
		return S3SelectResult{}, err
	}
	input, err := normalizeSelectInput(req.Input)
	if err != nil {
		return S3SelectResult{}, err
	}
	output, err := normalizeSelectOutput(req.Output, input.Format)
	if err != nil {
		return S3SelectResult{}, err
	}

	var rows []any
	switch input.Format {
	case "CSV":
		rows, err = parseSelectCSV(data, input)
	case "JSON":
		rows, err = parseSelectJSON(data, input)
	default:
		err = ErrSelectUnsupportedFormat
	}
	if err != nil {
		return S3SelectResult{}, err
	}
	if sql.limit != nil && *sql.limit < len(rows) {
		rows = rows[:*sql.limit]
	}

	encoded, err := encodeSelectRows(rows, output)
	if err != nil {
		return S3SelectResult{}, err
	}
	scanned := int64(len(data))
	returned := int64(0)
	for _, r := range encoded {
		returned += int64(len(r))
	}
	return S3SelectResult{
		Records:        encoded,
		BytesScanned:   scanned,
		BytesProcessed: scanned,
		BytesReturned:  returned,
	}, nil
}

func parseSelectCSV(data []byte, in S3SelectInput) ([]any, error) {
	r := csv.NewReader(bytes.NewReader(data))
	r.Comma = in.FieldDelimiter
	r.ReuseRecord = false
	r.FieldsPerRecord = -1
	all, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("%w: csv parse: %v", ErrSelectInvalidRequest, err)
	}
	if len(all) == 0 {
		return nil, nil
	}
	start := 0
	var headers []string
	switch in.FileHeaderInfo {
	case "USE":
		headers = all[0]
		start = 1
	case "IGNORE":
		start = 1
	}
	out := make([]any, 0, len(all)-start)
	for _, row := range all[start:] {
		if headers != nil {
			m := make(map[string]string, len(headers))
			for i, h := range headers {
				key := strings.TrimSpace(h)
				if key == "" {
					key = fmt.Sprintf("_%d", i+1)
				}
				val := ""
				if i < len(row) {
					val = row[i]
				}
				m[key] = val
			}
			out = append(out, m)
			continue
		}
		out = append(out, append([]string(nil), row...))
	}
	return out, nil
}

func parseSelectJSON(data []byte, in S3SelectInput) ([]any, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, nil
	}
	switch in.JSONType {
	case "DOCUMENT":
		var v any
		if err := json.Unmarshal(trimmed, &v); err != nil {
			return nil, fmt.Errorf("%w: json parse: %v", ErrSelectInvalidRequest, err)
		}
		switch rows := v.(type) {
		case []any:
			return rows, nil
		default:
			return []any{v}, nil
		}
	case "LINES":
		dec := json.NewDecoder(bytes.NewReader(trimmed))
		var out []any
		for {
			var v any
			if err := dec.Decode(&v); err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				return nil, fmt.Errorf("%w: json lines parse: %v", ErrSelectInvalidRequest, err)
			}
			out = append(out, v)
		}
		return out, nil
	default:
		return nil, ErrSelectUnsupportedFormat
	}
}

func encodeSelectRows(rows []any, out S3SelectOutput) ([]json.RawMessage, error) {
	encoded := make([]json.RawMessage, 0, len(rows))
	for _, row := range rows {
		switch out.Format {
		case "JSON":
			b, err := json.Marshal(row)
			if err != nil {
				return nil, fmt.Errorf("select encode json: %w", err)
			}
			encoded = append(encoded, b)
		case "CSV":
			line, err := encodeSelectCSVRow(row, out.FieldDelimiter)
			if err != nil {
				return nil, err
			}
			encoded = append(encoded, json.RawMessage(strconv.Quote(line)))
		default:
			return nil, ErrSelectUnsupportedFormat
		}
	}
	return encoded, nil
}

func encodeSelectCSVRow(row any, delim rune) (string, error) {
	var fields []string
	switch v := row.(type) {
	case []string:
		fields = v
	case []any:
		fields = make([]string, len(v))
		for i, cell := range v {
			fields[i] = fmt.Sprint(cell)
		}
	case map[string]string:
		// Stable-ish order is not guaranteed for maps; marshal as JSON object string for lab fidelity.
		b, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return string(b), nil
	case map[string]any:
		b, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return string(b), nil
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	w.Comma = delim
	if err := w.Write(fields); err != nil {
		return "", err
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return "", err
	}
	return strings.TrimRight(buf.String(), "\r\n"), nil
}
