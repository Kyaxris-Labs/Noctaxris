package store_test

import (
	"errors"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestGlueSchemaRegistryCRUDAndVersioning(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	region := "us-east-1"

	reg, err := st.CreateGlueRegistry(account, region, "lab-reg", "demo")
	if err != nil {
		t.Fatal(err)
	}
	if reg.Name != "lab-reg" || reg.RegistryARN == "" {
		t.Fatalf("registry=%#v", reg)
	}
	gotReg, err := st.GetGlueRegistry(account, "lab-reg")
	if err != nil {
		t.Fatal(err)
	}
	if gotReg.Description != "demo" {
		t.Fatalf("description=%q", gotReg.Description)
	}

	avro := `{"type":"record","name":"Person","fields":[{"name":"id","type":"long"},{"name":"name","type":"string"}]}`
	created, err := st.CreateGlueSchema(account, region, "lab-reg", "person", "AVRO", "NONE", "", avro)
	if err != nil {
		t.Fatal(err)
	}
	if created.VersionNumber != 1 || created.SchemaVersionID == "" {
		t.Fatalf("create schema=%#v", created)
	}
	if created.Schema.LatestSchemaVersion != 1 {
		t.Fatalf("latest=%d", created.Schema.LatestSchemaVersion)
	}

	v2Def := `{"type":"record","name":"Person","fields":[{"name":"id","type":"long"},{"name":"name","type":"string"},{"name":"email","type":"string"}]}`
	v2, err := st.RegisterGlueSchemaVersion(account, "lab-reg", "person", v2Def)
	if err != nil {
		t.Fatal(err)
	}
	if v2.VersionNumber != 2 {
		t.Fatalf("version=%d", v2.VersionNumber)
	}

	versions, err := st.ListGlueSchemaVersions(account, "lab-reg", "person")
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 2 {
		t.Fatalf("versions=%d", len(versions))
	}

	byID, err := st.GetGlueSchemaVersionByID(account, v2.SchemaVersionID)
	if err != nil {
		t.Fatal(err)
	}
	if byID.VersionNumber != 2 || byID.DataFormat != "AVRO" {
		t.Fatalf("byID=%#v", byID)
	}

	schemas, err := st.ListGlueSchemas(account, "lab-reg")
	if err != nil {
		t.Fatal(err)
	}
	if len(schemas) != 1 || schemas[0].LatestSchemaVersion != 2 {
		t.Fatalf("schemas=%#v", schemas)
	}

	if err := st.DeleteGlueSchema(account, "lab-reg", "person"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetGlueSchema(account, "lab-reg", "person"); !errors.Is(err, store.ErrGlueNotFound) {
		t.Fatalf("after delete schema err=%v", err)
	}
	if err := st.DeleteGlueRegistry(account, "lab-reg"); err != nil {
		t.Fatal(err)
	}
	regs, err := st.ListGlueRegistries(account)
	if err != nil {
		t.Fatal(err)
	}
	if len(regs) != 0 {
		t.Fatalf("regs=%#v", regs)
	}
}

func TestGlueSchemaRejectsInvalidDefinition(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	_, err := st.CreateGlueSchema(account, "us-east-1", "r1", "bad", "AVRO", "NONE", "", `{not json`)
	if !errors.Is(err, store.ErrGlueBadRequest) {
		t.Fatalf("err=%v want ErrGlueBadRequest", err)
	}
	_, err = st.CreateGlueSchema(account, "us-east-1", "r1", "proto", "PROTOBUF", "NONE", "", `message M {}`)
	if !errors.Is(err, store.ErrGlueBadRequest) {
		t.Fatalf("protobuf err=%v", err)
	}
}

func TestGlueGetTableResolvesSchemaReferenceColumns(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	region := "us-east-1"

	if _, err := st.CreateGlueDatabase(account, "labdb", ""); err != nil {
		t.Fatal(err)
	}
	jsonSchema := `{"type":"object","properties":{"sku":{"type":"string"},"qty":{"type":"integer"}}}`
	created, err := st.CreateGlueSchema(account, region, "catalog-reg", "item", "JSON", "NONE", "", jsonSchema)
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.CreateGlueTable(account, store.GlueTableCreate{
		DatabaseName: "labdb",
		Name:         "items",
		StorageLocation: "s3://lab/items/",
		Columns:      []store.GlueColumn{},
		SchemaReference: store.GlueSchemaReference{
			RegistryName:        "catalog-reg",
			SchemaName:          "item",
			SchemaVersionNumber: 1,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	tbl, err := st.GetGlueTable(account, "labdb", "items")
	if err != nil {
		t.Fatal(err)
	}
	if len(tbl.Columns) != 2 {
		t.Fatalf("columns=%#v", tbl.Columns)
	}
	if tbl.Columns[0].Name != "qty" || tbl.Columns[0].Type != "bigint" {
		t.Fatalf("col0=%#v", tbl.Columns[0])
	}
	if tbl.Columns[1].Name != "sku" || tbl.Columns[1].Type != "string" {
		t.Fatalf("col1=%#v", tbl.Columns[1])
	}

	_, err = st.CreateGlueTable(account, store.GlueTableCreate{
		DatabaseName:    "labdb",
		Name:            "items_by_id",
		StorageLocation: "s3://lab/items2/",
		Columns:         []store.GlueColumn{},
		SchemaReference: store.GlueSchemaReference{SchemaVersionID: created.SchemaVersionID},
	})
	if err != nil {
		t.Fatal(err)
	}
	tbl2, err := st.GetGlueTable(account, "labdb", "items_by_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(tbl2.Columns) != 2 {
		t.Fatalf("columns by id=%#v", tbl2.Columns)
	}

	_, err = st.CreateGlueTable(account, store.GlueTableCreate{
		DatabaseName:    "labdb",
		Name:            "items_params",
		StorageLocation: "s3://lab/items3/",
		Columns:         []store.GlueColumn{},
		Parameters: map[string]string{
			"RegistryName":        "catalog-reg",
			"SchemaName":          "item",
			"SchemaVersionNumber": "1",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	tables, err := st.GetGlueTables(account, "labdb")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tb := range tables {
		if tb.Name == "items_params" {
			found = true
			if len(tb.Columns) != 2 {
				t.Fatalf("params resolve columns=%#v", tb.Columns)
			}
		}
	}
	if !found {
		t.Fatal("items_params missing")
	}
}

func TestGlueSchemaDefinitionToColumnsAvroLite(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	avro := `{"type":"record","name":"Evt","fields":[{"name":"ts","type":"long"},{"name":"ok","type":["null","boolean"]}]}`
	created, err := st.CreateGlueSchema(account, "us-east-1", "", "evt", "AVRO", "NONE", "", avro)
	if err != nil {
		t.Fatal(err)
	}
	if created.Schema.RegistryName != "default-registry" {
		t.Fatalf("default registry=%q", created.Schema.RegistryName)
	}
	ver, err := st.GetGlueSchemaVersionByID(account, created.SchemaVersionID)
	if err != nil {
		t.Fatal(err)
	}
	if ver.Definition != avro {
		t.Fatalf("definition mismatch")
	}
}
