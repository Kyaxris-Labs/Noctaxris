package store

import (
	"fmt"
	"path"
	"strings"
)

// CURDuckRunner runs DuckDB SQL for CUR Parquet emission (HTTP or nested DinD exec).
// querySQL is typically a trivial SELECT; setupSQL holds COPY ... FORMAT PARQUET.
type CURDuckRunner func(accountID, querySQL, setupSQL string) error

// SetCURDuckRunner registers the DuckDB executor used when Format=Parquet.
// Nil clears the runner (Parquet emit then fail-closes to ERROR).
func (s *Store) SetCURDuckRunner(fn CURDuckRunner) {
	s.curDuckMu.Lock()
	defer s.curDuckMu.Unlock()
	s.curDuckRunner = fn
}

func (s *Store) getCURDuckRunner() CURDuckRunner {
	s.curDuckMu.Lock()
	defer s.curDuckMu.Unlock()
	return s.curDuckRunner
}

// BuildCURParquetDuckSQL builds floci-duck /query bodies for NDJSON → Parquet COPY.
// Paths are escaped for single-quoted SQL literals (callers must still reject unsafe key segments).
func BuildCURParquetDuckSQL(stagingS3URI, destS3URI string) (querySQL, setupSQL string) {
	staging := escapeCURDuckLiteral(strings.TrimSpace(stagingS3URI))
	dest := escapeCURDuckLiteral(strings.TrimSpace(destS3URI))
	setupSQL = fmt.Sprintf(
		"COPY (SELECT * FROM read_json_auto('%s', format='newline_delimited')) TO '%s' (FORMAT PARQUET);",
		staging, dest,
	)
	return "SELECT 1 AS ok", setupSQL
}

func escapeCURDuckLiteral(raw string) string {
	return strings.ReplaceAll(raw, `'`, `''`)
}

func curParquetStagingKey(reportName, runID string) string {
	return path.Join("noctaxris-cur-staging", reportName, runID+".ndjson")
}

func curParquetDestKey(s3Prefix, reportName, runID string) string {
	key := path.Join(strings.Trim(s3Prefix, "/"), reportName, runID+".parquet")
	return strings.TrimPrefix(key, "/")
}

func curS3URI(bucket, key string) string {
	return "s3://" + bucket + "/" + strings.TrimPrefix(key, "/")
}
