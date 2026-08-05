package bcmexports_test

import (
	"encoding/json"
	"testing"

	bcmsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/bcmexports"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestBCMExportsJSON(t *testing.T) {
	e := store.BCMExport{
		ExportName: "lab-export", ExportARN: "arn:aws:bcm:us-east-1:1:export/lab",
		Description: "d", FilePath: "cur/", Format: "TEXT_OR_CSV",
	}
	createRaw, err := bcmsvc.CreateExportJSON(e)
	if err != nil {
		t.Fatal(err)
	}
	var createOut map[string]string
	if err := json.Unmarshal(createRaw, &createOut); err != nil {
		t.Fatal(err)
	}
	if createOut["ExportArn"] != e.ExportARN {
		t.Fatalf("create=%v", createOut)
	}

	getRaw, err := bcmsvc.GetExportJSON(e)
	if err != nil {
		t.Fatal(err)
	}
	var getOut map[string]any
	if err := json.Unmarshal(getRaw, &getOut); err != nil {
		t.Fatal(err)
	}
	exp, _ := getOut["Export"].(map[string]any)
	if exp["Name"] != e.ExportName {
		t.Fatalf("get=%v", getOut)
	}

	listRaw, err := bcmsvc.ListExportsJSON([]store.BCMExport{e})
	if err != nil {
		t.Fatal(err)
	}
	var listOut map[string]any
	if err := json.Unmarshal(listRaw, &listOut); err != nil {
		t.Fatal(err)
	}
	items, _ := listOut["Exports"].([]any)
	if len(items) != 1 {
		t.Fatalf("list=%v", listOut)
	}

	delRaw, err := bcmsvc.DeleteExportJSON()
	if err != nil || string(delRaw) != "{}" {
		t.Fatalf("delete=%s", delRaw)
	}
}
