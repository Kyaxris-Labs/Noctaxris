package glue

import (
	"encoding/json"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func databaseInput(d store.GlueDatabase) map[string]any {
	return map[string]any{
		"Name":        d.Name,
		"Description": d.Description,
		"CreateTime":  float64(d.CreatedAt) / 1000.0,
	}
}

func tableInput(t store.GlueTable) map[string]any {
	cols := make([]map[string]any, 0, len(t.Columns))
	for _, c := range t.Columns {
		cols = append(cols, map[string]any{"Name": c.Name, "Type": c.Type})
	}
	pks := make([]map[string]any, 0, len(t.PartitionKeys))
	for _, c := range t.PartitionKeys {
		pks = append(pks, map[string]any{"Name": c.Name, "Type": c.Type})
	}
	serdeParams := map[string]string{}
	if t.SerDeInfo.Parameters != nil {
		serdeParams = t.SerDeInfo.Parameters
	}
	params := map[string]string{}
	if t.Parameters != nil {
		params = t.Parameters
	}
	sd := map[string]any{
		"Location":     t.StorageLocation,
		"Columns":      cols,
		"InputFormat":  t.InputFormat,
		"OutputFormat": t.OutputFormat,
		"SerdeInfo": map[string]any{
			"Name":                 t.SerDeInfo.Name,
			"SerializationLibrary": t.SerDeInfo.SerializationLibrary,
			"Parameters":           serdeParams,
		},
	}
	if t.SchemaReference.SchemaVersionID != "" || t.SchemaReference.RegistryName != "" || t.SchemaReference.SchemaName != "" {
		ref := map[string]any{}
		if t.SchemaReference.SchemaVersionID != "" {
			ref["SchemaVersionId"] = t.SchemaReference.SchemaVersionID
		}
		if t.SchemaReference.SchemaVersionNumber > 0 {
			ref["SchemaVersionNumber"] = t.SchemaReference.SchemaVersionNumber
		}
		if t.SchemaReference.RegistryName != "" || t.SchemaReference.SchemaName != "" {
			ref["SchemaId"] = map[string]any{
				"RegistryName": t.SchemaReference.RegistryName,
				"SchemaName":   t.SchemaReference.SchemaName,
			}
		}
		sd["SchemaReference"] = ref
	}
	return map[string]any{
		"Name":              t.Name,
		"DatabaseName":      t.DatabaseName,
		"Description":       t.Description,
		"StorageDescriptor": sd,
		"PartitionKeys":     pks,
		"Parameters":        params,
		"CreateTime":        float64(t.CreatedAt) / 1000.0,
		"UpdateTime":        float64(t.UpdatedAt) / 1000.0,
	}
}

// CreateDatabaseJSON is an empty OK body.
func CreateDatabaseJSON() ([]byte, error) { return []byte(`{}`), nil }

// CreateTableJSON is an empty OK body.
func CreateTableJSON() ([]byte, error) { return []byte(`{}`), nil }

// DeleteDatabaseJSON is an empty OK body.
func DeleteDatabaseJSON() ([]byte, error) { return []byte(`{}`), nil }

// DeleteTableJSON is an empty OK body.
func DeleteTableJSON() ([]byte, error) { return []byte(`{}`), nil }

// GetDatabaseJSON builds a GetDatabase response.
func GetDatabaseJSON(d store.GlueDatabase) ([]byte, error) {
	return json.Marshal(map[string]any{"Database": databaseInput(d)})
}

// GetDatabasesJSON builds a GetDatabases response.
func GetDatabasesJSON(dbs []store.GlueDatabase) ([]byte, error) {
	items := make([]map[string]any, 0, len(dbs))
	for _, d := range dbs {
		items = append(items, databaseInput(d))
	}
	return json.Marshal(map[string]any{"DatabaseList": items})
}

// GetTableJSON builds a GetTable response.
func GetTableJSON(t store.GlueTable) ([]byte, error) {
	return json.Marshal(map[string]any{"Table": tableInput(t)})
}

// GetTablesJSON builds a GetTables response.
func GetTablesJSON(tables []store.GlueTable) ([]byte, error) {
	items := make([]map[string]any, 0, len(tables))
	for _, t := range tables {
		items = append(items, tableInput(t))
	}
	return json.Marshal(map[string]any{"TableList": items})
}
