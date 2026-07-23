package server_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestKMSListResourceTagsAndTagResource(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustKMSJSON(t, handler, "CreateKey", map[string]any{
		"Tags": []map[string]any{
			{"TagKey": "env", "TagValue": "lab"},
		},
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateKey status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	var createOut map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &createOut); err != nil {
		t.Fatal(err)
	}
	meta, _ := createOut["KeyMetadata"].(map[string]any)
	keyID, _ := meta["KeyId"].(string)
	if keyID == "" {
		t.Fatal("missing KeyId")
	}

	listRec := mustKMSJSON(t, handler, "ListResourceTags", map[string]any{"KeyId": keyID}, now)
	if listRec.Code != http.StatusOK {
		t.Fatalf("ListResourceTags status=%d body=%q", listRec.Code, listRec.Body.String())
	}
	var listOut map[string]any
	if err := json.Unmarshal(listRec.Body.Bytes(), &listOut); err != nil {
		t.Fatal(err)
	}
	if listOut["Truncated"] != false {
		t.Fatalf("Truncated=%v", listOut["Truncated"])
	}
	tags, _ := listOut["Tags"].([]any)
	if len(tags) != 1 {
		t.Fatalf("Tags=%v want 1", listOut["Tags"])
	}
	first, _ := tags[0].(map[string]any)
	if first["TagKey"] != "env" || first["TagValue"] != "lab" {
		t.Fatalf("tag=%v", first)
	}

	tagRec := mustKMSJSON(t, handler, "TagResource", map[string]any{
		"KeyId": keyID,
		"Tags": []map[string]any{
			{"TagKey": "team", "TagValue": "platform"},
		},
	}, now)
	if tagRec.Code != http.StatusOK {
		t.Fatalf("TagResource status=%d body=%q", tagRec.Code, tagRec.Body.String())
	}

	list2 := mustKMSJSON(t, handler, "ListResourceTags", map[string]any{"KeyId": keyID}, now)
	if list2.Code != http.StatusOK {
		t.Fatalf("ListResourceTags#2 status=%d body=%q", list2.Code, list2.Body.String())
	}
	var list2Out map[string]any
	if err := json.Unmarshal(list2.Body.Bytes(), &list2Out); err != nil {
		t.Fatal(err)
	}
	tags2, _ := list2Out["Tags"].([]any)
	if len(tags2) != 2 {
		t.Fatalf("Tags after TagResource=%v want 2", list2Out["Tags"])
	}

	untag := mustKMSJSON(t, handler, "UntagResource", map[string]any{
		"KeyId":   keyID,
		"TagKeys": []string{"env"},
	}, now)
	if untag.Code != http.StatusOK {
		t.Fatalf("UntagResource status=%d body=%q", untag.Code, untag.Body.String())
	}
	list3 := mustKMSJSON(t, handler, "ListResourceTags", map[string]any{"KeyId": keyID}, now)
	var list3Out map[string]any
	if err := json.Unmarshal(list3.Body.Bytes(), &list3Out); err != nil {
		t.Fatal(err)
	}
	tags3, _ := list3Out["Tags"].([]any)
	if len(tags3) != 1 {
		t.Fatalf("Tags after UntagResource=%v want 1", list3Out["Tags"])
	}
	only, _ := tags3[0].(map[string]any)
	if only["TagKey"] != "team" {
		t.Fatalf("remaining tag=%v", only)
	}
}
