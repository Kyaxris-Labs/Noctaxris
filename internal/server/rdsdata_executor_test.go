package server

import (
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/compute"
)

func TestRDSDataPgxDisabledWithoutBuyIn(t *testing.T) {
	if rdsDataPgxEnabled() {
		t.Fatal("pgx path must stay disabled until go.mod buy-in")
	}
	t.Setenv(EnvRDSDataPgx, "1")
	if rdsDataPgxEnabled() {
		t.Fatal("NOCTAXRIS_RDS_DATA_PGX alone must not enable pgx without a linked driver")
	}
}

func TestMapPostgresSQLResultSelect(t *testing.T) {
	res := mapPostgresSQLResult(compute.PostgresSQLResult{
		Columns: []string{"?column?"},
		Rows:    [][]string{{"1"}},
		Select:  true,
	})
	if res.FormattedRecords != nestedRDSDataExecutorMarker {
		t.Fatalf("marker=%q", res.FormattedRecords)
	}
	if len(res.Records) != 1 || res.Records[0][0].StringValue == nil || *res.Records[0][0].StringValue != "1" {
		t.Fatalf("records=%v", res.Records)
	}
}

func TestMapPostgresSQLResultDML(t *testing.T) {
	res := mapPostgresSQLResult(compute.PostgresSQLResult{
		NumberOfRecordsUpdated: 2,
		Select:                 false,
	})
	if res.NumberOfRecordsUpdated != 2 {
		t.Fatalf("updated=%d", res.NumberOfRecordsUpdated)
	}
	if res.FormattedRecords != nestedRDSDataExecutorMarker {
		t.Fatalf("marker=%q", res.FormattedRecords)
	}
}
