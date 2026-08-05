package glue_test

import (
	"encoding/json"
	"testing"

	gluesvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/glue"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestGlueJSON(t *testing.T) {
	for _, fn := range []func() ([]byte, error){
		gluesvc.CreateDatabaseJSON, gluesvc.CreateTableJSON, gluesvc.DeleteDatabaseJSON, gluesvc.DeleteTableJSON,
	} {
		raw, err := fn()
		if err != nil || string(raw) != "{}" {
			t.Fatalf("empty ok: %s %v", raw, err)
		}
	}

	db := store.GlueDatabase{Name: "db1", Description: "lab", CreatedAt: 1_700_000_000_000}
	raw, err := gluesvc.GetDatabaseJSON(db)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	_ = json.Unmarshal(raw, &out)

	raw, _ = gluesvc.GetDatabasesJSON([]store.GlueDatabase{db})
	_ = json.Unmarshal(raw, &out)
	if len(out["DatabaseList"].([]any)) != 1 {
		t.Fatalf("dbs=%v", out)
	}

	tbl := store.GlueTable{
		DatabaseName: "db1", Name: "t1", Description: "tbl",
		StorageLocation: "s3://b/p", CreatedAt: 1_700_000_000_000, UpdatedAt: 1_700_000_001_000,
		Columns:       []store.GlueColumn{{Name: "c1", Type: "string"}},
		PartitionKeys: []store.GlueColumn{{Name: "dt", Type: "string"}},
		InputFormat: "i", OutputFormat: "o",
		SerDeInfo: store.GlueSerDeInfo{
			Name: "serde", SerializationLibrary: "lib",
			Parameters: map[string]string{"k": "v"},
		},
		Parameters: map[string]string{"p": "1"},
		SchemaReference: store.GlueSchemaReference{
			SchemaVersionID: "sv", SchemaVersionNumber: 2,
			RegistryName: "reg", SchemaName: "sch",
		},
	}
	raw, _ = gluesvc.GetTableJSON(tbl)
	_ = json.Unmarshal(raw, &out)
	table, _ := out["Table"].(map[string]any)
	sd, _ := table["StorageDescriptor"].(map[string]any)
	if sd["SchemaReference"] == nil {
		t.Fatalf("table=%v", out)
	}

	tbl.SchemaReference = store.GlueSchemaReference{}
	raw, _ = gluesvc.GetTableJSON(tbl)

	raw, _ = gluesvc.GetTablesJSON([]store.GlueTable{tbl})
	_ = json.Unmarshal(raw, &out)
}
