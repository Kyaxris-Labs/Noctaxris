package cloudcontrol_test

import (
	"encoding/json"
	"testing"

	ccsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/cloudcontrol"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestCloudControlJSON(t *testing.T) {
	progRaw, err := ccsvc.ProgressEventJSON("CREATE", "AWS::S3::Bucket", "bkt", "token-1", `{"BucketName":"lab"}`)
	if err != nil {
		t.Fatal(err)
	}
	var progOut map[string]any
	if err := json.Unmarshal(progRaw, &progOut); err != nil {
		t.Fatal(err)
	}
	ev, _ := progOut["ProgressEvent"].(map[string]any)
	if ev["OperationStatus"] != "SUCCESS" || ev["TypeName"] != "AWS::S3::Bucket" {
		t.Fatalf("progress=%v", progOut)
	}

	r := store.CloudControlResource{
		TypeName: "AWS::S3::Bucket", Identifier: "lab-bucket",
		Properties: `{"BucketName":"lab"}`,
	}
	getRaw, err := ccsvc.GetResourceJSON(r)
	if err != nil {
		t.Fatal(err)
	}
	var getOut map[string]any
	if err := json.Unmarshal(getRaw, &getOut); err != nil {
		t.Fatal(err)
	}
	if getOut["TypeName"] != r.TypeName {
		t.Fatalf("get=%v", getOut)
	}

	listRaw, err := ccsvc.ListResourcesJSON(r.TypeName, []store.CloudControlResource{r})
	if err != nil {
		t.Fatal(err)
	}
	var listOut map[string]any
	if err := json.Unmarshal(listRaw, &listOut); err != nil {
		t.Fatal(err)
	}
	descs, _ := listOut["ResourceDescriptions"].([]any)
	if len(descs) != 1 || listOut["TypeName"] != r.TypeName {
		t.Fatalf("list=%v", listOut)
	}

	_, err = ccsvc.ListResourcesJSON("t", nil)
	if err != nil {
		t.Fatal(err)
	}
}
