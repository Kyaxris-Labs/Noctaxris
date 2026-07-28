package compute_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/compute"
)

func TestDuckReadFunctionAndSetupSQL(t *testing.T) {
	t.Parallel()
	if got := compute.DuckReadFunction("org.apache.hadoop.hive.ql.io.parquet.MapredParquetInputFormat", ""); got != "read_parquet" {
		t.Fatalf("parquet fn=%q", got)
	}
	if got := compute.DuckReadFunction("", "org.openx.data.jsonserde.JsonSerDe"); got != "read_json_auto" {
		t.Fatalf("json fn=%q", got)
	}
	if got := compute.DuckReadFunction("", ""); got != "read_csv_auto" {
		t.Fatalf("csv fn=%q", got)
	}
	sql := compute.DuckSetupSQL([]compute.DuckTableRef{{
		Name:     "people",
		Location: "s3://bucket/data/",
	}})
	if !strings.Contains(sql, "CREATE OR REPLACE VIEW") || !strings.Contains(sql, "read_csv_auto") {
		t.Fatalf("setup=%q", sql)
	}
	if !strings.Contains(sql, "s3://bucket/data/*") {
		t.Fatalf("expected glob location, got %q", sql)
	}
}

func TestQueryDuckHTTPSuccess(t *testing.T) {
	t.Parallel()
	doer := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/query" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		var req compute.DuckQueryRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatal(err)
		}
		if req.SQL != "SELECT 1 AS n" || req.SetupSQL == "" {
			t.Fatalf("req=%+v", req)
		}
		resp := `{"status":"success","rows":[{"n":1},{"n":2}]}`
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(resp)),
			Header:     make(http.Header),
		}, nil
	})
	res, err := compute.QueryDuckHTTP(context.Background(), doer, "http://duck.lab", compute.DuckQueryRequest{
		SQL:      "SELECT 1 AS n",
		SetupSQL: "CREATE VIEW t AS SELECT 1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Columns) != 1 || res.Columns[0] != "n" {
		t.Fatalf("cols=%v", res.Columns)
	}
	if len(res.Rows) != 2 || res.Rows[0][0] != "1" {
		t.Fatalf("rows=%v", res.Rows)
	}
}

func TestQueryDuckHTTPFailClosed(t *testing.T) {
	t.Parallel()
	doer := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		resp := `{"status":"error","message":"boom"}`
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(resp)),
			Header:     make(http.Header),
		}, nil
	})
	_, err := compute.QueryDuckHTTP(context.Background(), doer, "http://duck.lab", compute.DuckQueryRequest{SQL: "SELECT 1"})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err=%v", err)
	}
}

func TestResolveAthenaEngineMode(t *testing.T) {
	t.Setenv(compute.EnvAthenaEngine, "duckdb")
	if compute.ResolveAthenaEngineMode() != compute.AthenaEngineDuckDB {
		t.Fatal("want duckdb")
	}
	t.Setenv(compute.EnvAthenaEngine, "inprocess")
	if compute.ResolveAthenaEngineMode() != compute.AthenaEngineInProcess {
		t.Fatal("want inprocess")
	}
	t.Setenv(compute.EnvAthenaEngine, "")
	if compute.ResolveAthenaEngineMode() != compute.AthenaEngineAuto {
		t.Fatal("want auto")
	}
}

func TestAllowImagePullDuck(t *testing.T) {
	t.Parallel()
	if err := compute.AllowImagePull(compute.DefaultDuckImage, "127.0.0.1:4566"); err != nil {
		t.Fatal(err)
	}
}

func TestValidateDataPlaneOptsDuckDB(t *testing.T) {
	t.Parallel()
	if err := compute.ValidateDataPlaneOpts(compute.DataPlaneOpts{
		Kind:  compute.DataKindDuckDB,
		Image: compute.DefaultDuckImage,
	}); err != nil {
		t.Fatal(err)
	}
	if compute.DefaultDataPlaneImage(compute.DataKindDuckDB) != compute.DefaultDuckImage {
		t.Fatal("default image")
	}
	if compute.DefaultDataPlanePort(compute.DataKindDuckDB) != compute.DefaultDuckPort {
		t.Fatal("default port")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) Do(r *http.Request) (*http.Response, error) { return f(r) }
