package server_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/compute"
)

func TestCURParquetFailClosedWithoutDuckEngine(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	t.Setenv(compute.EnvDuckDBURL, "")
	t.Setenv("NOCTAXRIS_CUR_EMIT", "1")
	// Empty DockerHost on test server → nested DuckDB unavailable → ReportStatus ERROR.

	account := "000000000001"
	if _, err := st.CreateBucket(account, "cur-parquet-fail"); err != nil {
		t.Fatalf("CreateBucket: %v", err)
	}

	handler := srv.Handler()
	now := time.Now().UTC()

	put := mustJSONTarget(t, handler, "AWSOrigamiServiceGatewayService.PutReportDefinition", "cur", map[string]any{
		"ReportDefinition": map[string]any{
			"ReportName":  "lab-parquet",
			"TimeUnit":    "MONTHLY",
			"Format":      "Parquet",
			"Compression": "Parquet",
			"S3Bucket":    "cur-parquet-fail",
			"S3Prefix":    "reports",
			"S3Region":    "us-east-1",
		},
	}, now)
	if put.Code != http.StatusOK {
		t.Fatalf("PutReportDefinition status=%d body=%s", put.Code, put.Body.String())
	}

	got, err := st.GetCURReportDefinition(account, "us-east-1", "lab-parquet")
	if err != nil {
		t.Fatalf("GetCURReportDefinition: %v", err)
	}
	if got.ReportStatus != "ERROR" {
		t.Fatalf("expected ERROR without DuckDB, got %q", got.ReportStatus)
	}
}
