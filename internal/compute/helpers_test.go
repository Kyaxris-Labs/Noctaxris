package compute_test

import (
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/compute"
)

func TestResolveDuckHelpersAndNewRequest(t *testing.T) {
	t.Setenv(compute.EnvDuckDBImage, "")
	t.Setenv(compute.EnvDuckS3Endpoint, "")
	t.Setenv(compute.EnvDuckDBURL, "")
	t.Setenv(compute.EnvAthenaEngine, "")

	if got := compute.ResolveDuckImage(); got != compute.DefaultDuckImage {
		t.Fatalf("default image=%q", got)
	}
	t.Setenv(compute.EnvDuckDBImage, " floci/custom-duck:1 ")
	if got := compute.ResolveDuckImage(); got != "floci/custom-duck:1" {
		t.Fatalf("override image=%q", got)
	}

	if got := compute.ResolveDuckS3Endpoint(); got == "" {
		t.Fatal("default s3 endpoint must be non-empty")
	}
	t.Setenv(compute.EnvDuckS3Endpoint, " http://s3.lab:4566 ")
	if got := compute.ResolveDuckS3Endpoint(); got != "http://s3.lab:4566" {
		t.Fatalf("override s3=%q", got)
	}

	if got := compute.ConfiguredDuckURL(); got != "" {
		t.Fatalf("empty url=%q", got)
	}
	t.Setenv(compute.EnvDuckDBURL, " http://duck:3000/ ")
	if got := compute.ConfiguredDuckURL(); got != "http://duck:3000" {
		t.Fatalf("configured url=%q", got)
	}

	if compute.ResolveAthenaEngineMode() != compute.AthenaEngineAuto {
		t.Fatal("default auto")
	}
	t.Setenv(compute.EnvAthenaEngine, "duckdb")
	if compute.ResolveAthenaEngineMode() != compute.AthenaEngineDuckDB {
		t.Fatal("duckdb")
	}
	t.Setenv(compute.EnvAthenaEngine, "inprocess")
	if compute.ResolveAthenaEngineMode() != compute.AthenaEngineInProcess {
		t.Fatal("inprocess")
	}

	req := compute.NewDuckQueryRequest("SELECT 1", "SETUP", "")
	if req.S3AccessKey != "test" || req.SQL != "SELECT 1" || req.S3URLStyle != "path" {
		t.Fatalf("req=%+v", req)
	}
	req2 := compute.NewDuckQueryRequest("SELECT 2", "", "AKIATEST")
	if req2.S3AccessKey != "AKIATEST" {
		t.Fatalf("access key=%q", req2.S3AccessKey)
	}
}

func TestDuckCreateViewSQL_boundaries(t *testing.T) {
	sql := compute.DuckCreateViewSQL("t", "s3://b/p", "")
	if !strings.Contains(sql, "read_csv_auto") || !strings.Contains(sql, "s3://b/p/*") {
		t.Fatalf("default fn/glob: %s", sql)
	}
	sql = compute.DuckCreateViewSQL(`na"me`, "s3://b/file.parquet", "read_parquet")
	if !strings.Contains(sql, `"na""me"`) || !strings.Contains(sql, "file.parquet") {
		t.Fatalf("parquet file: %s", sql)
	}
	sql = compute.DuckCreateViewSQL("t", "s3://b/dir/", "read_json_auto")
	if !strings.Contains(sql, "s3://b/dir/*") {
		t.Fatalf("dir glob: %s", sql)
	}
	setup := compute.DuckSetupSQL([]compute.DuckTableRef{
		{Name: "", Location: "s3://b/x"},
		{Name: "ok", Location: ""},
		{Name: "people", Location: "s3://b/data", InputFormat: "parquet"},
	})
	if strings.Contains(setup, `""`) && !strings.Contains(setup, "people") {
		t.Fatalf("setup=%q", setup)
	}
	if !strings.Contains(setup, "people") || !strings.Contains(setup, "read_parquet") {
		t.Fatalf("setup=%q", setup)
	}
}

func TestBrokerPortPublishReady(t *testing.T) {
	t.Setenv(compute.EnvNestedPortPublish, "")
	t.Setenv(compute.EnvBrokerPortPublish, "")
	if compute.BrokerPortPublishReady() {
		t.Fatal("default off")
	}
	t.Setenv(compute.EnvBrokerPortPublish, "1")
	if !compute.BrokerPortPublishReady() {
		t.Fatal("broker gate")
	}
	t.Setenv(compute.EnvBrokerPortPublish, "")
	t.Setenv(compute.EnvNestedPortPublish, "true")
	if !compute.BrokerPortPublishReady() {
		t.Fatal("nested gate")
	}
}

func TestValidateImageRunOpts_boundaries(t *testing.T) {
	err := compute.ValidateImageRunOpts(compute.ImageRunOpts{})
	if err == nil {
		t.Fatal("empty opts")
	}
	base := compute.ImageRunOpts{
		ImageURI:      "public.ecr.aws/lambda/python:3.13",
		EventHostPath: "/tmp/event",
		Handler:       "mod.fn",
	}
	if err := compute.ValidateImageRunOpts(base); err != nil {
		t.Fatal(err)
	}
	badPath := base
	badPath.EventHostPath = "relative"
	if err := compute.ValidateImageRunOpts(badPath); err == nil {
		t.Fatal("relative EventHostPath")
	}
	emptyPath := base
	emptyPath.EventHostPath = ""
	if err := compute.ValidateImageRunOpts(emptyPath); err == nil {
		t.Fatal("empty EventHostPath")
	}
	badHandler := base
	badHandler.Handler = ".bad"
	if err := compute.ValidateImageRunOpts(badHandler); err == nil {
		t.Fatal("bad handler")
	}
	noHandler := base
	noHandler.Handler = ""
	noHandler.ImageURI = "unknown.example/not-lambda:1"
	noHandler.AllowDefaultEntrypoint = false
	if err := compute.ValidateImageRunOpts(noHandler); err == nil {
		t.Fatal("expected handler or allowlist failure")
	}
	neg := base
	neg.TimeoutSec = -1
	if err := compute.ValidateImageRunOpts(neg); err == nil {
		t.Fatal("negative timeout")
	}
}

func TestDinDPullHost(t *testing.T) {
	if got := compute.DinDPullHost("127.0.0.1:4566"); got != "host.docker.internal:4566" {
		t.Fatalf("got %q", got)
	}
	if got := compute.DinDPullHost("bad"); !strings.HasPrefix(got, "host.docker.internal:") {
		t.Fatalf("fallback=%q", got)
	}
	ref, ok := compute.LabImageForDinD("127.0.0.1:4566/000000000001/repo:tag", "127.0.0.1:4566")
	if !ok || !strings.HasPrefix(ref, "host.docker.internal:4566/") {
		t.Fatalf("ref=%q ok=%v", ref, ok)
	}
	ref, ok = compute.LabImageForDinD("public.ecr.aws/lambda/python:3.13", "127.0.0.1:4566")
	if ok || ref != "public.ecr.aws/lambda/python:3.13" {
		t.Fatalf("non-lab ref=%q ok=%v", ref, ok)
	}
}
