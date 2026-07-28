package store_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
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
	// No DuckDB runner → Parquet emit fail-closed (not SUCCESS with JSON stand-in).
	if mod.ReportStatus != "ERROR" {
		t.Fatalf("expected ERROR without DuckDB runner, got %q", mod.ReportStatus)
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

func TestCURParquetEmitWithMockDuck(t *testing.T) {
	st := openCURStore(t)
	account := "000000000001"
	if _, err := st.CreateBucket(account, "billing-parquet"); err != nil {
		t.Fatalf("CreateBucket: %v", err)
	}
	t.Setenv("NOCTAXRIS_CUR_EMIT", "1")

	var gotSQL, gotSetup string
	st.SetCURDuckRunner(func(acc, querySQL, setupSQL string) error {
		if acc != account {
			t.Fatalf("account=%q want %q", acc, account)
		}
		gotSQL, gotSetup = querySQL, setupSQL
		// Simulate DuckDB writing destination Parquet (PAR1 magic).
		dest := destKeyFromCURSetup(t, setupSQL)
		_, err := st.PutObject(account, "billing-parquet", dest, store.PutObjectMeta{
			Data:        []byte("PAR1mock"),
			PlainSize:   8,
			ContentType: "application/vnd.apache.parquet",
		})
		return err
	})

	def, err := st.PutCURReportDefinition(account, "us-east-1", store.CURReportDefinition{
		ReportName:  "parquet-report",
		TimeUnit:    "MONTHLY",
		Format:      "Parquet",
		Compression: "Parquet",
		S3Bucket:    "billing-parquet",
		S3Prefix:    "reports",
		S3Region:    "us-east-1",
	})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if def.ReportStatus != "SUCCESS" {
		t.Fatalf("expected SUCCESS after mock Duck emit, got %q", def.ReportStatus)
	}
	if !strings.Contains(gotSetup, "FORMAT PARQUET") || !strings.Contains(gotSetup, "read_json_auto") {
		t.Fatalf("unexpected setup SQL: %s", gotSetup)
	}
	if !strings.Contains(gotSetup, "s3://billing-parquet/noctaxris-cur-staging/") {
		t.Fatalf("expected staging URI in setup SQL: %s", gotSetup)
	}
	if !strings.Contains(gotSetup, "s3://billing-parquet/reports/parquet-report/") || !strings.Contains(gotSetup, ".parquet") {
		t.Fatalf("expected dest parquet URI in setup SQL: %s", gotSetup)
	}
	if gotSQL != "SELECT 1 AS ok" {
		t.Fatalf("query SQL=%q", gotSQL)
	}

	// Staging NDJSON cleaned up; only parquet remains under reports/.
	staging, err := st.ListObjectsV2(account, "billing-parquet", "noctaxris-cur-staging/", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(staging.Contents) != 0 {
		t.Fatalf("expected staging cleanup, got %d objects", len(staging.Contents))
	}
	listed, err := st.ListObjectsV2(account, "billing-parquet", "reports/parquet-report/", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Contents) != 1 || !strings.HasSuffix(listed.Contents[0].Key, ".parquet") {
		t.Fatalf("expected one .parquet object, got %+v", listed.Contents)
	}
}

func TestCURParquetFailClosedWithoutDuck(t *testing.T) {
	st := openCURStore(t)
	account := "000000000001"
	if _, err := st.CreateBucket(account, "billing-no-duck"); err != nil {
		t.Fatalf("CreateBucket: %v", err)
	}
	t.Setenv("NOCTAXRIS_CUR_EMIT", "1")

	def, err := st.PutCURReportDefinition(account, "us-east-1", store.CURReportDefinition{
		ReportName:  "need-duck",
		TimeUnit:    "DAILY",
		Format:      "Parquet",
		Compression: "Parquet",
		S3Bucket:    "billing-no-duck",
		S3Region:    "us-east-1",
	})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if def.ReportStatus != "ERROR" {
		t.Fatalf("expected ERROR without DuckDB, got %q", def.ReportStatus)
	}
	listed, err := st.ListObjectsV2(account, "billing-no-duck", "", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, obj := range listed.Contents {
		if strings.HasSuffix(obj.Key, ".json") {
			t.Fatalf("must not write JSON stand-in for Format=Parquet: %s", obj.Key)
		}
	}
}

func TestCURParquetDuckRunnerErrorMarksERROR(t *testing.T) {
	st := openCURStore(t)
	account := "000000000001"
	if _, err := st.CreateBucket(account, "billing-duck-err"); err != nil {
		t.Fatalf("CreateBucket: %v", err)
	}
	t.Setenv("NOCTAXRIS_CUR_EMIT", "1")
	st.SetCURDuckRunner(func(accountID, querySQL, setupSQL string) error {
		return errors.New("duck unavailable")
	})

	def, err := st.PutCURReportDefinition(account, "us-east-1", store.CURReportDefinition{
		ReportName:  "duck-err",
		TimeUnit:    "MONTHLY",
		Format:      "Parquet",
		Compression: "Parquet",
		S3Bucket:    "billing-duck-err",
		S3Prefix:    "out",
		S3Region:    "us-east-1",
	})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if def.ReportStatus != "ERROR" {
		t.Fatalf("expected ERROR on duck failure, got %q", def.ReportStatus)
	}
	// Staging cleaned even on duck failure.
	staging, err := st.ListObjectsV2(account, "billing-duck-err", "noctaxris-cur-staging/", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(staging.Contents) != 0 {
		t.Fatalf("expected staging cleanup after duck error, got %d", len(staging.Contents))
	}
}

func TestBuildCURParquetDuckSQLEscapesQuotes(t *testing.T) {
	q, setup := store.BuildCURParquetDuckSQL("s3://b/a'b.ndjson", "s3://b/out'x.parquet")
	if q != "SELECT 1 AS ok" {
		t.Fatalf("query=%q", q)
	}
	if !strings.Contains(setup, "a''b") || !strings.Contains(setup, "out''x") {
		t.Fatalf("expected escaped quotes: %s", setup)
	}
	if !strings.Contains(setup, "(FORMAT PARQUET)") {
		t.Fatalf("missing FORMAT PARQUET: %s", setup)
	}
}

func destKeyFromCURSetup(t *testing.T, setupSQL string) string {
	t.Helper()
	const marker = " TO 's3://"
	i := strings.Index(setupSQL, marker)
	if i < 0 {
		t.Fatalf("no dest URI in %s", setupSQL)
	}
	rest := setupSQL[i+len(marker):]
	j := strings.Index(rest, "'")
	if j < 0 {
		t.Fatalf("unterminated dest URI in %s", setupSQL)
	}
	uri := rest[:j] // bucket/key
	slash := strings.Index(uri, "/")
	if slash < 0 {
		t.Fatalf("bad uri %q", uri)
	}
	return uri[slash+1:]
}
