package tagging_test

import (
	"encoding/json"
	"testing"

	taggingsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/tagging"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestTaggingJSON(t *testing.T) {
	failed := map[string]string{"arn:aws:s3:::bad": "invalid arn"}
	tagRaw, err := taggingsvc.TagResourcesJSON(failed)
	if err != nil {
		t.Fatal(err)
	}
	var tagOut map[string]any
	if err := json.Unmarshal(tagRaw, &tagOut); err != nil {
		t.Fatal(err)
	}
	fm, _ := tagOut["FailedResourcesMap"].(map[string]any)
	entry, _ := fm["arn:aws:s3:::bad"].(map[string]any)
	if entry["ErrorCode"] != "InvalidParameterException" {
		t.Fatalf("tag=%v", tagOut)
	}

	untagRaw, err := taggingsvc.UntagResourcesJSON(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(untagRaw, &tagOut); err != nil {
		t.Fatal(err)
	}
	fm, _ = tagOut["FailedResourcesMap"].(map[string]any)
	if len(fm) != 0 {
		t.Fatalf("untag=%v", tagOut)
	}

	res := []store.TaggedResource{{
		ResourceARN: "arn:aws:lambda:us-east-1:1:function:f",
		Tags:        []store.ResourceTag{{Key: "env", Value: "lab"}},
	}}
	getRaw, err := taggingsvc.GetResourcesJSON(res)
	if err != nil {
		t.Fatal(err)
	}
	var getOut map[string]any
	if err := json.Unmarshal(getRaw, &getOut); err != nil {
		t.Fatal(err)
	}
	list, _ := getOut["ResourceTagMappingList"].([]any)
	if len(list) != 1 {
		t.Fatalf("get=%v", getOut)
	}

	_, err = taggingsvc.GetResourcesJSON(nil)
	if err != nil {
		t.Fatal(err)
	}
}
