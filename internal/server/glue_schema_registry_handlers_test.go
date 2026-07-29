package server_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestGlueSchemaRegistryHandlersAndGetTableResolve(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createReg := mustJSONTarget(t, handler, "AWSGlue.CreateRegistry", "glue", map[string]any{
		"RegistryName": "srv-reg",
		"Description":  "lab",
	}, now)
	if createReg.Code != http.StatusOK {
		t.Fatalf("CreateRegistry status=%d body=%q", createReg.Code, createReg.Body.String())
	}

	jsonSchema := `{"type":"object","properties":{"id":{"type":"string"},"amount":{"type":"number"}}}`
	createSchema := mustJSONTarget(t, handler, "AWSGlue.CreateSchema", "glue", map[string]any{
		"RegistryId":       map[string]any{"RegistryName": "srv-reg"},
		"SchemaName":       "order",
		"DataFormat":       "JSON",
		"Compatibility":    "NONE",
		"SchemaDefinition": jsonSchema,
	}, now)
	if createSchema.Code != http.StatusOK {
		t.Fatalf("CreateSchema status=%d body=%q", createSchema.Code, createSchema.Body.String())
	}
	var schemaOut map[string]any
	if err := json.Unmarshal(createSchema.Body.Bytes(), &schemaOut); err != nil {
		t.Fatal(err)
	}
	versionID, _ := schemaOut["SchemaVersionId"].(string)
	if versionID == "" {
		t.Fatalf("missing SchemaVersionId: %v", schemaOut)
	}

	listRegs := mustJSONTarget(t, handler, "AWSGlue.ListRegistries", "glue", map[string]any{}, now)
	if listRegs.Code != http.StatusOK {
		t.Fatalf("ListRegistries status=%d body=%q", listRegs.Code, listRegs.Body.String())
	}

	regVer := mustJSONTarget(t, handler, "AWSGlue.RegisterSchemaVersion", "glue", map[string]any{
		"SchemaId": map[string]any{
			"RegistryName": "srv-reg",
			"SchemaName":   "order",
		},
		"SchemaDefinition": `{"type":"object","properties":{"id":{"type":"string"},"amount":{"type":"number"},"note":{"type":"string"}}}`,
	}, now)
	if regVer.Code != http.StatusOK {
		t.Fatalf("RegisterSchemaVersion status=%d body=%q", regVer.Code, regVer.Body.String())
	}

	listVers := mustJSONTarget(t, handler, "AWSGlue.ListSchemaVersions", "glue", map[string]any{
		"SchemaId": map[string]any{"RegistryName": "srv-reg", "SchemaName": "order"},
	}, now)
	if listVers.Code != http.StatusOK {
		t.Fatalf("ListSchemaVersions status=%d body=%q", listVers.Code, listVers.Body.String())
	}
	var versOut map[string]any
	if err := json.Unmarshal(listVers.Body.Bytes(), &versOut); err != nil {
		t.Fatal(err)
	}
	schemas, _ := versOut["Schemas"].([]any)
	if len(schemas) != 2 {
		t.Fatalf("versions=%v", versOut)
	}

	createDB := mustJSONTarget(t, handler, "AWSGlue.CreateDatabase", "glue", map[string]any{
		"DatabaseInput": map[string]any{"Name": "schemadb"},
	}, now)
	if createDB.Code != http.StatusOK {
		t.Fatalf("CreateDatabase status=%d body=%q", createDB.Code, createDB.Body.String())
	}

	createTbl := mustJSONTarget(t, handler, "AWSGlue.CreateTable", "glue", map[string]any{
		"DatabaseName": "schemadb",
		"TableInput": map[string]any{
			"Name": "orders",
			"StorageDescriptor": map[string]any{
				"Location": "s3://lab/orders/",
				"Columns":  []any{},
				"SchemaReference": map[string]any{
					"SchemaId": map[string]any{
						"RegistryName": "srv-reg",
						"SchemaName":   "order",
					},
					"SchemaVersionNumber": float64(1),
				},
			},
		},
	}, now)
	if createTbl.Code != http.StatusOK {
		t.Fatalf("CreateTable status=%d body=%q", createTbl.Code, createTbl.Body.String())
	}

	getTbl := mustJSONTarget(t, handler, "AWSGlue.GetTable", "glue", map[string]any{
		"DatabaseName": "schemadb",
		"Name":         "orders",
	}, now)
	if getTbl.Code != http.StatusOK {
		t.Fatalf("GetTable status=%d body=%q", getTbl.Code, getTbl.Body.String())
	}
	var tblOut map[string]any
	if err := json.Unmarshal(getTbl.Body.Bytes(), &tblOut); err != nil {
		t.Fatal(err)
	}
	table, _ := tblOut["Table"].(map[string]any)
	sd, _ := table["StorageDescriptor"].(map[string]any)
	cols, _ := sd["Columns"].([]any)
	if len(cols) != 2 {
		t.Fatalf("resolved columns=%v body=%s", cols, getTbl.Body.String())
	}

	getVer := mustJSONTarget(t, handler, "AWSGlue.GetSchemaVersion", "glue", map[string]any{
		"SchemaVersionId": versionID,
	}, now)
	if getVer.Code != http.StatusOK {
		t.Fatalf("GetSchemaVersion status=%d body=%q", getVer.Code, getVer.Body.String())
	}
}
