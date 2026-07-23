package server_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestSSMListTagsForResourceAndAddTags(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	putRec := mustSSMJSON(t, handler, "PutParameter", map[string]any{
		"Name":  "/lab/tagged",
		"Value": "v1",
		"Type":  "String",
	}, now)
	if putRec.Code != http.StatusOK {
		t.Fatalf("PutParameter status=%d body=%q", putRec.Code, putRec.Body.String())
	}

	listEmpty := mustSSMJSON(t, handler, "ListTagsForResource", map[string]any{
		"ResourceType": "Parameter",
		"ResourceId":   "/lab/tagged",
	}, now)
	if listEmpty.Code != http.StatusOK {
		t.Fatalf("ListTagsForResource status=%d body=%q", listEmpty.Code, listEmpty.Body.String())
	}
	var emptyOut map[string]any
	if err := json.Unmarshal(listEmpty.Body.Bytes(), &emptyOut); err != nil {
		t.Fatal(err)
	}
	if tagList, _ := emptyOut["TagList"].([]any); len(tagList) != 0 {
		t.Fatalf("TagList=%v want empty", emptyOut["TagList"])
	}

	addRec := mustSSMJSON(t, handler, "AddTagsToResource", map[string]any{
		"ResourceType": "Parameter",
		"ResourceId":   "/lab/tagged",
		"Tags": []map[string]any{
			{"Key": "env", "Value": "lab"},
		},
	}, now)
	if addRec.Code != http.StatusOK {
		t.Fatalf("AddTagsToResource status=%d body=%q", addRec.Code, addRec.Body.String())
	}

	listRec := mustSSMJSON(t, handler, "ListTagsForResource", map[string]any{
		"ResourceType": "Parameter",
		"ResourceId":   "/lab/tagged",
	}, now)
	if listRec.Code != http.StatusOK {
		t.Fatalf("ListTagsForResource#2 status=%d body=%q", listRec.Code, listRec.Body.String())
	}
	var listOut map[string]any
	if err := json.Unmarshal(listRec.Body.Bytes(), &listOut); err != nil {
		t.Fatal(err)
	}
	tagList, _ := listOut["TagList"].([]any)
	if len(tagList) != 1 {
		t.Fatalf("TagList=%v want 1", listOut["TagList"])
	}
	first, _ := tagList[0].(map[string]any)
	if first["Key"] != "env" || first["Value"] != "lab" {
		t.Fatalf("tag=%v", first)
	}

	remove := mustSSMJSON(t, handler, "RemoveTagsFromResource", map[string]any{
		"ResourceType": "Parameter",
		"ResourceId":   "/lab/tagged",
		"TagKeys":      []string{"env"},
	}, now)
	if remove.Code != http.StatusOK {
		t.Fatalf("RemoveTagsFromResource status=%d body=%q", remove.Code, remove.Body.String())
	}
	list3 := mustSSMJSON(t, handler, "ListTagsForResource", map[string]any{
		"ResourceType": "Parameter",
		"ResourceId":   "/lab/tagged",
	}, now)
	var list3Out map[string]any
	if err := json.Unmarshal(list3.Body.Bytes(), &list3Out); err != nil {
		t.Fatal(err)
	}
	if tagList3, _ := list3Out["TagList"].([]any); len(tagList3) != 0 {
		t.Fatalf("TagList after remove=%v want empty", list3Out["TagList"])
	}
}
