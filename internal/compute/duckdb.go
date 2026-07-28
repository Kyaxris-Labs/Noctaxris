package compute

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	// LabDuckContainerName is the shared nested DuckDB HTTP sidecar on noctaxris-data.
	LabDuckContainerName = "noctaxris-lab-duck"
	// DefaultDuckPort is the floci-duck-compatible HTTP listen port.
	DefaultDuckPort = 3000
	// DefaultDuckImage is the allowlisted DuckDB HTTP shim image (floci-duck compatible).
	DefaultDuckImage = "floci/floci-duck:latest"

	// EnvDuckDBURL points at an already-reachable DuckDB HTTP base URL (skips container ensure).
	EnvDuckDBURL = "NOCTAXRIS_DUCKDB_URL"
	// EnvDuckDBImage overrides DefaultDuckImage when ensuring the nested sidecar.
	EnvDuckDBImage = "NOCTAXRIS_DUCKDB_IMAGE"
	// EnvAthenaEngine selects Athena query path: auto | duckdb | inprocess.
	EnvAthenaEngine = "NOCTAXRIS_ATHENA_ENGINE"
	// EnvDuckS3Endpoint is the lab S3/API URL as seen from the DuckDB container.
	EnvDuckS3Endpoint = "NOCTAXRIS_DUCKDB_S3_ENDPOINT"
)

// AthenaEngineMode is the Athena SQL engine selection.
type AthenaEngineMode string

const (
	AthenaEngineAuto      AthenaEngineMode = "auto"
	AthenaEngineDuckDB    AthenaEngineMode = "duckdb"
	AthenaEngineInProcess AthenaEngineMode = "inprocess"
)

// DuckQueryRequest is the floci-duck-compatible /query JSON body.
// Stable for Athena and CUR callers.
type DuckQueryRequest struct {
	SQL         string `json:"sql"`
	SetupSQL    string `json:"setup_sql,omitempty"`
	S3Endpoint  string `json:"s3_endpoint"`
	S3Region    string `json:"s3_region"`
	S3AccessKey string `json:"s3_access_key"`
	S3SecretKey string `json:"s3_secret_key"`
	S3URLStyle  string `json:"s3_url_style"`
}

// DuckQueryResult is tabular output from /query.
type DuckQueryResult struct {
	Columns []string
	Rows    [][]string
}

// DuckTableRef describes a Glue-backed table for CREATE VIEW injection.
type DuckTableRef struct {
	Name            string
	Location        string // s3://bucket/prefix
	InputFormat     string
	SerializationLib string
}

// DuckHTTPDoer posts JSON to a DuckDB shim (tests inject fakes).
type DuckHTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// ResolveDuckImage returns EnvDuckDBImage or DefaultDuckImage.
func ResolveDuckImage() string {
	if v := strings.TrimSpace(os.Getenv(EnvDuckDBImage)); v != "" {
		return v
	}
	return DefaultDuckImage
}

// ResolveDuckS3Endpoint returns the S3 endpoint DuckDB should use for lab reads/writes.
func ResolveDuckS3Endpoint() string {
	if v := strings.TrimSpace(os.Getenv(EnvDuckS3Endpoint)); v != "" {
		return v
	}
	return defaultEndpointURL
}

// ResolveAthenaEngineMode parses NOCTAXRIS_ATHENA_ENGINE (default auto).
func ResolveAthenaEngineMode() AthenaEngineMode {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvAthenaEngine))) {
	case "duckdb", "duck":
		return AthenaEngineDuckDB
	case "inprocess", "in-process", "local":
		return AthenaEngineInProcess
	default:
		return AthenaEngineAuto
	}
}

// ConfiguredDuckURL returns NOCTAXRIS_DUCKDB_URL when set.
func ConfiguredDuckURL() string {
	return strings.TrimRight(strings.TrimSpace(os.Getenv(EnvDuckDBURL)), "/")
}

// DuckReadFunction selects DuckDB read_* from Glue InputFormat / SerDe library.
func DuckReadFunction(inputFormat, serializationLib string) string {
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

// DuckCreateViewSQL builds one CREATE OR REPLACE VIEW statement for a Glue table location.
func DuckCreateViewSQL(tableName, location, readFn string) string {
	name := strings.TrimSpace(tableName)
	loc := strings.TrimSpace(location)
	fn := strings.TrimSpace(readFn)
	if fn == "" {
		fn = "read_csv_auto"
	}
	if !strings.HasSuffix(loc, "/") && !strings.HasSuffix(strings.ToLower(loc), ".csv") &&
		!strings.HasSuffix(strings.ToLower(loc), ".json") && !strings.HasSuffix(strings.ToLower(loc), ".parquet") &&
		!strings.HasSuffix(strings.ToLower(loc), ".gz") {
		loc = strings.TrimRight(loc, "/") + "/*"
	} else if strings.HasSuffix(loc, "/") {
		loc = loc + "*"
	}
	return fmt.Sprintf("CREATE OR REPLACE VIEW %s AS SELECT * FROM %s('%s');", quoteDuckIdent(name), fn, escapeDuckString(loc))
}

// DuckSetupSQL builds setup_sql for all Glue tables in a database.
func DuckSetupSQL(tables []DuckTableRef) string {
	var b strings.Builder
	for _, t := range tables {
		if strings.TrimSpace(t.Name) == "" || strings.TrimSpace(t.Location) == "" {
			continue
		}
		fn := DuckReadFunction(t.InputFormat, t.SerializationLib)
		b.WriteString(DuckCreateViewSQL(t.Name, t.Location, fn))
		b.WriteByte('\n')
	}
	return b.String()
}

func quoteDuckIdent(name string) string {
	name = strings.ReplaceAll(name, `"`, `""`)
	return `"` + name + `"`
}

func escapeDuckString(s string) string {
	return strings.ReplaceAll(s, `'`, `''`)
}

// NewDuckQueryRequest builds a default path-style lab S3 request body.
func NewDuckQueryRequest(sql, setupSQL, accessKeyID string) DuckQueryRequest {
	ak := strings.TrimSpace(accessKeyID)
	if ak == "" {
		ak = "test"
	}
	return DuckQueryRequest{
		SQL:         sql,
		SetupSQL:    setupSQL,
		S3Endpoint:  ResolveDuckS3Endpoint(),
		S3Region:    "us-east-1",
		S3AccessKey: ak,
		S3SecretKey: "test",
		S3URLStyle:  "path",
	}
}

// QueryDuckHTTP POSTs to baseURL/query (floci-duck compatible).
func QueryDuckHTTP(ctx context.Context, doer DuckHTTPDoer, baseURL string, body DuckQueryRequest) (DuckQueryResult, error) {
	if doer == nil {
		doer = http.DefaultClient
	}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return DuckQueryResult{}, fmt.Errorf("compute: duckdb base URL is required")
	}
	if strings.TrimSpace(body.SQL) == "" {
		return DuckQueryResult{}, fmt.Errorf("compute: duckdb sql is required")
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return DuckQueryResult{}, fmt.Errorf("compute: duckdb marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/query", bytes.NewReader(payload))
	if err != nil {
		return DuckQueryResult{}, fmt.Errorf("compute: duckdb request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := doer.Do(req)
	if err != nil {
		return DuckQueryResult{}, fmt.Errorf("compute: duckdb dial: %w", err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 32<<20))
	if err != nil {
		return DuckQueryResult{}, fmt.Errorf("compute: duckdb read: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		return DuckQueryResult{}, fmt.Errorf("compute: duckdb HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(raw)))
	}
	return parseDuckQueryResponse(raw)
}

// ProbeDuckHealth GETs baseURL/health.
func ProbeDuckHealth(ctx context.Context, doer DuckHTTPDoer, baseURL string) bool {
	if doer == nil {
		doer = http.DefaultClient
	}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/health", nil)
	if err != nil {
		return false
	}
	res, err := doer.Do(req)
	if err != nil {
		return false
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
	return res.StatusCode == http.StatusOK
}

func parseDuckQueryResponse(raw []byte) (DuckQueryResult, error) {
	var envelope struct {
		Status  string           `json:"status"`
		Message string           `json:"message"`
		Rows    []map[string]any `json:"rows"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return DuckQueryResult{}, fmt.Errorf("compute: duckdb response json: %w", err)
	}
	if !strings.EqualFold(envelope.Status, "success") {
		msg := strings.TrimSpace(envelope.Message)
		if msg == "" {
			msg = "query failed"
		}
		return DuckQueryResult{}, fmt.Errorf("compute: duckdb query: %s", msg)
	}
	if len(envelope.Rows) == 0 {
		return DuckQueryResult{}, nil
	}
	colOrder := make([]string, 0)
	seen := map[string]struct{}{}
	for _, row := range envelope.Rows {
		for k := range row {
			if _, ok := seen[k]; ok {
				continue
			}
			seen[k] = struct{}{}
			colOrder = append(colOrder, k)
		}
	}
	outRows := make([][]string, 0, len(envelope.Rows))
	for _, row := range envelope.Rows {
		cells := make([]string, len(colOrder))
		for i, c := range colOrder {
			cells[i] = duckScalarString(row[c])
		}
		outRows = append(outRows, cells)
	}
	return DuckQueryResult{Columns: colOrder, Rows: outRows}, nil
}

func duckScalarString(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case float64:
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
		return fmt.Sprintf("%g", t)
	case bool:
		return fmt.Sprintf("%t", t)
	case json.Number:
		return t.String()
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return fmt.Sprint(t)
		}
		return string(b)
	}
}

// EnsureDuckDB starts or reuses the nested DuckDB sidecar on noctaxris-data.
// Host ports stay unpublished. ExtraHosts injects host.docker.internal for lab S3.
func (c *Client) EnsureDuckDB(ctx context.Context) (DataPlaneInstance, error) {
	if c == nil || c.cli == nil {
		return DataPlaneInstance{}, fmt.Errorf("compute: data-plane client unavailable")
	}
	img := ResolveDuckImage()
	if err := AllowImagePull(img, c.listenAddr); err != nil {
		return DataPlaneInstance{}, err
	}
	return c.EnsureDataPlaneByName(ctx, DataPlaneOpts{
		Kind:          DataKindDuckDB,
		Image:         img,
		Name:          LabDuckContainerName,
		ContainerPort: DefaultDuckPort,
		ExtraHosts:    []string{"host.docker.internal:host-gateway"},
	})
}

// WaitDuckHealthy polls nested /health via DinD exec (API is not on noctaxris-data).
func (c *Client) WaitDuckHealthy(ctx context.Context, containerID string, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	deadline := time.Now().Add(timeout)
	var last error
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return err
		}
		ok, err := c.probeDuckHealthExec(ctx, containerID)
		if err == nil && ok {
			return nil
		}
		last = err
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	if last != nil {
		return fmt.Errorf("compute: duckdb health timeout: %w", last)
	}
	return fmt.Errorf("compute: duckdb health timeout")
}

func (c *Client) probeDuckHealthExec(ctx context.Context, containerID string) (bool, error) {
	res, err := c.Exec(ctx, ExecOpts{
		ContainerID: containerID,
		Cmd:         []string{"wget", "-q", "-O", "-", "http://127.0.0.1:3000/health"},
	})
	if err != nil {
		// Busybox/curl fallbacks for alternate shim images.
		res, err = c.Exec(ctx, ExecOpts{
			ContainerID: containerID,
			Cmd:         []string{"sh", "-c", "wget -q -O - http://127.0.0.1:3000/health || curl -sf http://127.0.0.1:3000/health"},
		})
		if err != nil {
			return false, err
		}
	}
	return res.ExitCode == 0, nil
}

// ExecDuckQuery POSTs /query inside the nested DuckDB container (DinD exec).
// Prefer this over API-process HTTP dials to noctaxris-data hostnames.
func (c *Client) ExecDuckQuery(ctx context.Context, containerID string, body DuckQueryRequest) (DuckQueryResult, error) {
	if strings.TrimSpace(containerID) == "" {
		return DuckQueryResult{}, fmt.Errorf("compute: duckdb container ID is required")
	}
	if strings.TrimSpace(body.SQL) == "" {
		return DuckQueryResult{}, fmt.Errorf("compute: duckdb sql is required")
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return DuckQueryResult{}, fmt.Errorf("compute: duckdb marshal: %w", err)
	}
	// Pipe JSON via shell heredoc-equivalent; avoid host temp files.
	script := `payload=$(cat); wget -q -O - --header='Content-Type: application/json' --post-data="$payload" http://127.0.0.1:3000/query || curl -sf -H 'Content-Type: application/json' -d "$payload" http://127.0.0.1:3000/query`
	res, err := c.Exec(ctx, ExecOpts{
		ContainerID: containerID,
		Cmd:         []string{"sh", "-c", script},
		Stdin:       string(payload),
	})
	if err != nil {
		return DuckQueryResult{}, err
	}
	if res.ExitCode != 0 {
		msg := strings.TrimSpace(res.Stderr)
		if msg == "" {
			msg = strings.TrimSpace(res.Stdout)
		}
		if msg == "" {
			msg = fmt.Sprintf("duck query exit %d", res.ExitCode)
		}
		return DuckQueryResult{}, fmt.Errorf("compute: nested duckdb: %s", msg)
	}
	return parseDuckQueryResponse([]byte(res.Stdout))
}
