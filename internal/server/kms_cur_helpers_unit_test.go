package server

import (
	"testing"
)

func TestParseKMSGrantConstraintsTable(t *testing.T) {
	t.Parallel()
	ok, err := parseKMSGrantConstraints(nil)
	if err != nil || ok.EncryptionContextEquals != nil {
		t.Fatalf("nil=%+v err=%v", ok, err)
	}
	_, err = parseKMSGrantConstraints("bad")
	if err == nil {
		t.Fatal("non-object should fail")
	}
	_, err = parseKMSGrantConstraints(map[string]any{"Unknown": map[string]any{}})
	if err == nil {
		t.Fatal("unknown key should fail")
	}
	_, err = parseKMSGrantConstraints(map[string]any{
		"EncryptionContextEquals": map[string]any{"a": 1},
	})
	if err == nil {
		t.Fatal("non-string map value should fail")
	}
	got, err := parseKMSGrantConstraints(map[string]any{
		"EncryptionContextEquals": map[string]any{"dept": "lab"},
		"EncryptionContextSubset": map[string]string{"env": "dev"},
	})
	if err != nil || got.EncryptionContextEquals["dept"] != "lab" || got.EncryptionContextSubset["env"] != "dev" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestStringSliceFromAnyAndCURParse(t *testing.T) {
	t.Parallel()
	if stringSliceFromAny(nil) != nil {
		t.Fatal("nil")
	}
	if stringSliceFromAny("x") != nil {
		t.Fatal("wrong type")
	}
	got := stringSliceFromAny([]any{"a", 1, "b", nil})
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("%#v", got)
	}
	def := parseCURReportDefinition(map[string]any{
		"ReportDefinition": map[string]any{
			"ReportName":               "r1",
			"TimeUnit":                 "DAILY",
			"Format":                   "Parquet",
			"Compression":              "Parquet",
			"S3Bucket":                 "b",
			"S3Prefix":                 "p",
			"S3Region":                 "us-east-1",
			"ReportVersioning":         "CREATE_NEW_REPORT",
			"RefreshClosedReports":     true,
			"AdditionalSchemaElements": []any{"RESOURCES", 2},
			"AdditionalArtifacts":      []any{"ATHENA"},
		},
	})
	if def.ReportName != "r1" || !def.RefreshClosedReports || len(def.AdditionalSchemaElements) != 1 || def.AdditionalArtifacts[0] != "ATHENA" {
		t.Fatalf("%+v", def)
	}
}
