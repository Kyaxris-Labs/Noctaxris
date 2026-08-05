package cur_test

import (
	"encoding/json"
	"testing"

	cursvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/cur"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestCURJSON(t *testing.T) {
	d := store.CURReportDefinition{
		ReportName: "lab-report", TimeUnit: "DAILY", Format: "textORcsv",
		Compression: "GZIP", S3Bucket: "b", S3Prefix: "p/", S3Region: "us-east-1",
		RefreshClosedReports: true, ReportStatus: "SUCCESS",
		AdditionalSchemaElements: []string{"RESOURCES"},
		AdditionalArtifacts:      []string{"REDSHIFT"},
		ReportVersioning:         "CREATE_NEW_REPORT",
	}
	putRaw, err := cursvc.PutReportDefinitionJSON(d)
	if err != nil {
		t.Fatal(err)
	}
	var putOut map[string]any
	if err := json.Unmarshal(putRaw, &putOut); err != nil {
		t.Fatal(err)
	}
	if putOut["ReportVersioning"] != "CREATE_NEW_REPORT" {
		t.Fatalf("put=%v", putOut)
	}

	d.ReportVersioning = ""
	modRaw, err := cursvc.ModifyReportDefinitionJSON(d)
	if err != nil {
		t.Fatal(err)
	}
	var modOut map[string]any
	if err := json.Unmarshal(modRaw, &modOut); err != nil {
		t.Fatal(err)
	}
	if _, ok := modOut["ReportVersioning"]; ok {
		t.Fatalf("omit empty versioning: %v", modOut)
	}

	descRaw, err := cursvc.DescribeReportDefinitionsJSON([]store.CURReportDefinition{d})
	if err != nil {
		t.Fatal(err)
	}
	var descOut map[string]any
	if err := json.Unmarshal(descRaw, &descOut); err != nil {
		t.Fatal(err)
	}
	defs, _ := descOut["ReportDefinitions"].([]any)
	if len(defs) != 1 {
		t.Fatalf("describe=%v", descOut)
	}

	notDelRaw, err := cursvc.DeleteReportDefinitionJSON("lab-report", false)
	if err != nil || string(notDelRaw) != "{}" {
		t.Fatalf("not deleted=%s", notDelRaw)
	}
	delRaw, err := cursvc.DeleteReportDefinitionJSON("lab-report", true)
	if err != nil {
		t.Fatal(err)
	}
	var delOut map[string]string
	if err := json.Unmarshal(delRaw, &delOut); err != nil {
		t.Fatal(err)
	}
	if delOut["ResponseMessage"] == "" {
		t.Fatalf("deleted=%v", delOut)
	}
}
