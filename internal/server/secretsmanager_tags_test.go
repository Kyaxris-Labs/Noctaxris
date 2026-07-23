package server_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestSecretsListTagsForResourceAndTagResource(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustSecretsJSON(t, handler, "CreateSecret", map[string]any{
		"Name":         "tagged-secret",
		"SecretString": "s3cret",
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateSecret status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	var createOut map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &createOut); err != nil {
		t.Fatal(err)
	}
	arn, _ := createOut["ARN"].(string)
	if arn == "" {
		t.Fatal("missing ARN")
	}

	listEmpty := mustSecretsJSON(t, handler, "ListTagsForResource", map[string]any{
		"SecretId": arn,
	}, now)
	if listEmpty.Code != http.StatusOK {
		t.Fatalf("ListTagsForResource status=%d body=%q", listEmpty.Code, listEmpty.Body.String())
	}
	var emptyOut map[string]any
	if err := json.Unmarshal(listEmpty.Body.Bytes(), &emptyOut); err != nil {
		t.Fatal(err)
	}
	if tags, _ := emptyOut["Tags"].([]any); len(tags) != 0 {
		t.Fatalf("Tags=%v want empty", emptyOut["Tags"])
	}

	tagRec := mustSecretsJSON(t, handler, "TagResource", map[string]any{
		"SecretId": arn,
		"Tags": []map[string]any{
			{"Key": "env", "Value": "lab"},
		},
	}, now)
	if tagRec.Code != http.StatusOK {
		t.Fatalf("TagResource status=%d body=%q", tagRec.Code, tagRec.Body.String())
	}

	listRec := mustSecretsJSON(t, handler, "ListTagsForResource", map[string]any{
		"SecretId": "tagged-secret",
	}, now)
	if listRec.Code != http.StatusOK {
		t.Fatalf("ListTagsForResource#2 status=%d body=%q", listRec.Code, listRec.Body.String())
	}
	var listOut map[string]any
	if err := json.Unmarshal(listRec.Body.Bytes(), &listOut); err != nil {
		t.Fatal(err)
	}
	tags, _ := listOut["Tags"].([]any)
	if len(tags) != 1 {
		t.Fatalf("Tags=%v want 1", listOut["Tags"])
	}
	first, _ := tags[0].(map[string]any)
	if first["Key"] != "env" || first["Value"] != "lab" {
		t.Fatalf("tag=%v", first)
	}

	untag := mustSecretsJSON(t, handler, "UntagResource", map[string]any{
		"SecretId": arn,
		"TagKeys":  []string{"env"},
	}, now)
	if untag.Code != http.StatusOK {
		t.Fatalf("UntagResource status=%d body=%q", untag.Code, untag.Body.String())
	}
	list3 := mustSecretsJSON(t, handler, "ListTagsForResource", map[string]any{
		"SecretId": arn,
	}, now)
	var list3Out map[string]any
	if err := json.Unmarshal(list3.Body.Bytes(), &list3Out); err != nil {
		t.Fatal(err)
	}
	if tags3, _ := list3Out["Tags"].([]any); len(tags3) != 0 {
		t.Fatalf("Tags after UntagResource=%v want empty", list3Out["Tags"])
	}
}
