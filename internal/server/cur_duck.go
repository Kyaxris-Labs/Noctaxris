package server

import (
	"fmt"
)

// wireCURDuckRunner registers nested DuckDB / NOCTAXRIS_DUCKDB_URL for CUR Parquet emit.
// Reuses the Athena DuckDB ensure + ExecDuckQuery / QueryDuckHTTP path.
func (s *Server) wireCURDuckRunner() {
	if s == nil || s.store == nil {
		return
	}
	s.store.SetCURDuckRunner(func(accountID, querySQL, setupSQL string) error {
		run, err := s.athenaDuckRunner(accountID)
		if err != nil {
			return err
		}
		_, _, err = run(querySQL, setupSQL)
		if err != nil {
			return fmt.Errorf("cur parquet duckdb: %w", err)
		}
		return nil
	})
}
