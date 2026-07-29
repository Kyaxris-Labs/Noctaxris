package store

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

func validateGlueSchemaDefinition(dataFormat, definition string) error {
	cols, err := glueSchemaDefinitionToColumns(dataFormat, definition)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrGlueBadRequest, err)
	}
	if len(cols) == 0 {
		return fmt.Errorf("%w: schema definition has no fields", ErrGlueBadRequest)
	}
	return nil
}

func glueSchemaDefinitionToColumns(dataFormat, definition string) ([]GlueColumn, error) {
	definition = strings.TrimSpace(definition)
	if definition == "" {
		return nil, fmt.Errorf("empty schema definition")
	}
	switch strings.ToUpper(strings.TrimSpace(dataFormat)) {
	case "AVRO":
		return glueAvroFieldsLite(definition)
	case "JSON":
		return glueJSONSchemaFieldsLite(definition)
	default:
		return nil, fmt.Errorf("unsupported DataFormat %q", dataFormat)
	}
}

func glueAvroFieldsLite(definition string) ([]GlueColumn, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal([]byte(definition), &root); err != nil {
		return nil, fmt.Errorf("invalid AVRO schema JSON: %w", err)
	}
	typeRaw, ok := root["type"]
	if !ok {
		return nil, fmt.Errorf("AVRO schema missing type")
	}
	var typeStr string
	if err := json.Unmarshal(typeRaw, &typeStr); err != nil || typeStr != "record" {
		return nil, fmt.Errorf("AVRO schema type must be record")
	}
	fieldsRaw, ok := root["fields"]
	if !ok {
		return nil, fmt.Errorf("AVRO schema missing fields")
	}
	var fields []map[string]json.RawMessage
	if err := json.Unmarshal(fieldsRaw, &fields); err != nil {
		return nil, fmt.Errorf("AVRO fields: %w", err)
	}
	cols := make([]GlueColumn, 0, len(fields))
	for _, f := range fields {
		var name string
		if err := json.Unmarshal(f["name"], &name); err != nil || strings.TrimSpace(name) == "" {
			continue
		}
		cols = append(cols, GlueColumn{Name: name, Type: glueAvroTypeLite(f["type"])})
	}
	if len(cols) == 0 {
		return nil, fmt.Errorf("AVRO record has no named fields")
	}
	return cols, nil
}

func glueAvroTypeLite(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "string"
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return glueAvroPrimitiveToHive(s)
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err == nil {
		// nullable union ["null","string"] lite
		for _, item := range arr {
			var prim string
			if err := json.Unmarshal(item, &prim); err == nil && prim != "null" {
				return glueAvroPrimitiveToHive(prim)
			}
		}
		return "string"
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err == nil {
		var t string
		_ = json.Unmarshal(obj["type"], &t)
		switch t {
		case "array":
			return "array<" + glueAvroTypeLite(obj["items"]) + ">"
		case "map":
			return "map<string," + glueAvroTypeLite(obj["values"]) + ">"
		case "record", "enum", "fixed":
			return "string"
		default:
			if t != "" {
				return glueAvroPrimitiveToHive(t)
			}
		}
	}
	return "string"
}

func glueAvroPrimitiveToHive(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "long":
		return "bigint"
	case "int":
		return "int"
	case "boolean":
		return "boolean"
	case "double":
		return "double"
	case "float":
		return "float"
	case "bytes":
		return "binary"
	case "string", "null", "enum":
		return "string"
	default:
		return "string"
	}
}

func glueJSONSchemaFieldsLite(definition string) ([]GlueColumn, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal([]byte(definition), &root); err != nil {
		return nil, fmt.Errorf("invalid JSON Schema: %w", err)
	}
	var typeStr string
	_ = json.Unmarshal(root["type"], &typeStr)
	if typeStr != "" && typeStr != "object" {
		return nil, fmt.Errorf("JSON Schema type must be object")
	}
	propsRaw, ok := root["properties"]
	if !ok {
		return nil, fmt.Errorf("JSON Schema missing properties")
	}
	var props map[string]json.RawMessage
	if err := json.Unmarshal(propsRaw, &props); err != nil {
		return nil, fmt.Errorf("JSON Schema properties: %w", err)
	}
	if len(props) == 0 {
		return nil, fmt.Errorf("JSON Schema properties empty")
	}
	names := make([]string, 0, len(props))
	for name := range props {
		names = append(names, name)
	}
	sort.Strings(names)
	cols := make([]GlueColumn, 0, len(names))
	for _, name := range names {
		cols = append(cols, GlueColumn{Name: name, Type: glueJSONTypeLite(props[name])})
	}
	return cols, nil
}

func glueJSONTypeLite(raw json.RawMessage) string {
	var node map[string]json.RawMessage
	if err := json.Unmarshal(raw, &node); err != nil {
		return "string"
	}
	var t string
	_ = json.Unmarshal(node["type"], &t)
	switch t {
	case "string":
		return "string"
	case "integer":
		return "bigint"
	case "number":
		return "double"
	case "boolean":
		return "boolean"
	case "array":
		return "array<" + glueJSONTypeLite(node["items"]) + ">"
	case "object":
		return "string"
	default:
		return "string"
	}
}
