package server_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestSQSListQueueTagsAndTagQueue(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustSQSJSON(t, handler, "CreateQueue", map[string]any{
		"QueueName": "tagged-queue",
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateQueue status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	var createOut map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &createOut); err != nil {
		t.Fatal(err)
	}
	queueURL, _ := createOut["QueueUrl"].(string)
	if queueURL == "" {
		t.Fatal("missing QueueUrl")
	}

	listEmpty := mustSQSJSON(t, handler, "ListQueueTags", map[string]any{"QueueUrl": queueURL}, now)
	if listEmpty.Code != http.StatusOK {
		t.Fatalf("ListQueueTags status=%d body=%q", listEmpty.Code, listEmpty.Body.String())
	}
	var emptyOut map[string]any
	if err := json.Unmarshal(listEmpty.Body.Bytes(), &emptyOut); err != nil {
		t.Fatal(err)
	}
	tags, _ := emptyOut["Tags"].(map[string]any)
	if len(tags) != 0 {
		t.Fatalf("Tags=%v want empty", emptyOut["Tags"])
	}

	tagRec := mustSQSJSON(t, handler, "TagQueue", map[string]any{
		"QueueUrl": queueURL,
		"Tags":     map[string]any{"env": "lab"},
	}, now)
	if tagRec.Code != http.StatusOK {
		t.Fatalf("TagQueue status=%d body=%q", tagRec.Code, tagRec.Body.String())
	}

	listRec := mustSQSJSON(t, handler, "ListQueueTags", map[string]any{"QueueUrl": queueURL}, now)
	if listRec.Code != http.StatusOK {
		t.Fatalf("ListQueueTags#2 status=%d body=%q", listRec.Code, listRec.Body.String())
	}
	var listOut map[string]any
	if err := json.Unmarshal(listRec.Body.Bytes(), &listOut); err != nil {
		t.Fatal(err)
	}
	tags2, _ := listOut["Tags"].(map[string]any)
	if tags2["env"] != "lab" {
		t.Fatalf("Tags=%v", listOut["Tags"])
	}

	untag := mustSQSJSON(t, handler, "UntagQueue", map[string]any{
		"QueueUrl": queueURL,
		"TagKeys":  []string{"env"},
	}, now)
	if untag.Code != http.StatusOK {
		t.Fatalf("UntagQueue status=%d body=%q", untag.Code, untag.Body.String())
	}
	list3 := mustSQSJSON(t, handler, "ListQueueTags", map[string]any{"QueueUrl": queueURL}, now)
	var list3Out map[string]any
	if err := json.Unmarshal(list3.Body.Bytes(), &list3Out); err != nil {
		t.Fatal(err)
	}
	tags3, _ := list3Out["Tags"].(map[string]any)
	if len(tags3) != 0 {
		t.Fatalf("Tags after UntagQueue=%v want empty", list3Out["Tags"])
	}
}
