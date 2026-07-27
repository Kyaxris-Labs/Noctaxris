package store_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func openCURStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	key, err := store.LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestCURPutDescribeModifyDeleteAndEmit(t *testing.T) {
	st := openCURStore(t)
	account := "000000000001"
	if _, err := st.CreateBucket(account, "billing-lab"); err != nil {
		t.Fatalf("CreateBucket: %v", err)
	}
	t.Setenv("NOCTAXRIS_CUR_EMIT", "1")

	def, err := st.PutCURReportDefinition(account, "us-east-1", store.CURReportDefinition{
		ReportName:               "monthly-report",
		TimeUnit:                 "MONTHLY",
		Format:                   "textORcsv",
		Compression:              "GZIP",
		S3Bucket:                 "billing-lab",
		S3Prefix:                 "reports",
		S3Region:                 "us-east-1",
		AdditionalSchemaElements: []string{"RESOURCES"},
		ReportVersioning:         "OVERWRITE_REPORT",
	})
	if err != nil {
		t.Fatalf("PutCURReportDefinition: %v", err)
	}
	if def.ReportStatus != "SUCCESS" {
		t.Fatalf("expected SUCCESS status after emit, got %q", def.ReportStatus)
	}

	list, err := st.DescribeCURReportDefinitions(account)
	if err != nil || len(list) != 1 {
		t.Fatalf("DescribeCURReportDefinitions: err=%v len=%d", err, len(list))
	}

	listed, err := st.ListObjectsV2(account, "billing-lab", "reports/monthly-report/", "")
	if err != nil {
		t.Fatalf("ListObjectsV2: %v", err)
	}
	if len(listed.Contents) < 1 {
		t.Fatal("expected emitted CUR object in S3")
	}

	mod, err := st.ModifyCURReportDefinition(account, "us-east-1", "monthly-report", store.CURReportDefinition{
		ReportName:       "monthly-report",
		TimeUnit:         "DAILY",
		Format:           "Parquet",
		Compression:      "Parquet",
		S3Bucket:         "billing-lab",
		S3Prefix:         "reports",
		S3Region:         "us-east-1",
		ReportVersioning: "CREATE_NEW_REPORT",
	})
	if err != nil {
		t.Fatalf("ModifyCURReportDefinition: %v", err)
	}
	if mod.TimeUnit != "DAILY" || mod.Format != "Parquet" {
		t.Fatalf("modify mismatch: %+v", mod)
	}

	if _, err := st.PutCURReportDefinition(account, "us-east-1", store.CURReportDefinition{
		ReportName:  "monthly-report",
		TimeUnit:    "MONTHLY",
		Format:      "textORcsv",
		Compression: "ZIP",
		S3Bucket:    "billing-lab",
		S3Region:    "us-east-1",
	}); err == nil {
		t.Fatal("expected duplicate report name error")
	}

	if err := st.DeleteCURReportDefinition(account, "us-east-1", "monthly-report"); err != nil {
		t.Fatalf("DeleteCURReportDefinition: %v", err)
	}
	if err := st.DeleteCURReportDefinition(account, "us-east-1", "missing"); err != nil {
		t.Fatalf("idempotent delete: %v", err)
	}
}

func TestCURValidationRejectsBadFormat(t *testing.T) {
	st := openCURStore(t)
	_, err := st.PutCURReportDefinition("000000000001", "us-east-1", store.CURReportDefinition{
		ReportName:  "bad",
		TimeUnit:    "MONTHLY",
		Format:      "CSV",
		Compression: "GZIP",
		S3Bucket:    "billing-lab",
		S3Region:    "us-east-1",
	})
	if err == nil {
		t.Fatal("expected validation error for Format=CSV")
	}
}

func TestCUREmitOffSkipsS3(t *testing.T) {
	st := openCURStore(t)
	account := "000000000001"
	if _, err := st.CreateBucket(account, "billing-off"); err != nil {
		t.Fatalf("CreateBucket: %v", err)
	}
	if err := os.Setenv("NOCTAXRIS_CUR_EMIT", "0"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Unsetenv("NOCTAXRIS_CUR_EMIT") })

	def, err := st.PutCURReportDefinition(account, "us-east-1", store.CURReportDefinition{
		ReportName:  "no-emit",
		TimeUnit:    "MONTHLY",
		Format:      "textORcsv",
		Compression: "ZIP",
		S3Bucket:    "billing-off",
		S3Region:    "us-east-1",
	})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if def.ReportStatus != "PENDING" {
		t.Fatalf("expected PENDING when emit off, got %q", def.ReportStatus)
	}
	listed, err := st.ListObjectsV2(account, "billing-off", "", "")
	if err != nil {
		t.Fatalf("ListObjectsV2: %v", err)
	}
	if len(listed.Contents) != 0 {
		t.Fatalf("expected no objects when emit off, got %d", len(listed.Contents))
	}
}
