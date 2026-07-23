package server_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestDynamoDBListTagsOfResourceAndTagResource(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustDynamoJSON(t, handler, "CreateTable", map[string]any{
		"TableName": "tagged-table",
		"AttributeDefinitions": []map[string]any{
			{"AttributeName": "pk", "AttributeType": "S"},
		},
		"KeySchema": []map[string]any{
			{"AttributeName": "pk", "KeyType": "HASH"},
		},
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateTable status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	tableARN := "arn:aws:dynamodb:" + testRegion + ":" + testAccountID + ":table/tagged-table"

	listEmpty := mustDynamoJSON(t, handler, "ListTagsOfResource", map[string]any{
		"ResourceArn": tableARN,
	}, now)
	if listEmpty.Code != http.StatusOK {
		t.Fatalf("ListTagsOfResource status=%d body=%q", listEmpty.Code, listEmpty.Body.String())
	}
	var emptyOut map[string]any
	if err := json.Unmarshal(listEmpty.Body.Bytes(), &emptyOut); err != nil {
		t.Fatal(err)
	}
	if tags, _ := emptyOut["Tags"].([]any); len(tags) != 0 {
		t.Fatalf("Tags=%v want empty", emptyOut["Tags"])
	}

	tagRec := mustDynamoJSON(t, handler, "TagResource", map[string]any{
		"ResourceArn": tableARN,
		"Tags": []map[string]any{
			{"Key": "env", "Value": "lab"},
		},
	}, now)
	if tagRec.Code != http.StatusOK {
		t.Fatalf("TagResource status=%d body=%q", tagRec.Code, tagRec.Body.String())
	}

	listRec := mustDynamoJSON(t, handler, "ListTagsOfResource", map[string]any{
		"ResourceArn": tableARN,
	}, now)
	if listRec.Code != http.StatusOK {
		t.Fatalf("ListTagsOfResource#2 status=%d body=%q", listRec.Code, listRec.Body.String())
	}
	var listOut map[string]any
	if err := json.Unmarshal(listRec.Body.Bytes(), &listOut); err != nil {
		t.Fatal(err)
	}
	tags, _ := listOut["Tags"].([]any)
	if len(tags) != 1 {
		t.Fatalf("Tags=%v want 1", listOut["Tags"])
	}
	tag, _ := tags[0].(map[string]any)
	if tag["Key"] != "env" || tag["Value"] != "lab" {
		t.Fatalf("tag=%v", tag)
	}

	untag := mustDynamoJSON(t, handler, "UntagResource", map[string]any{
		"ResourceArn": tableARN,
		"TagKeys":     []string{"env"},
	}, now)
	if untag.Code != http.StatusOK {
		t.Fatalf("UntagResource status=%d body=%q", untag.Code, untag.Body.String())
	}
	list3 := mustDynamoJSON(t, handler, "ListTagsOfResource", map[string]any{
		"ResourceArn": tableARN,
	}, now)
	if list3.Code != http.StatusOK {
		t.Fatalf("ListTagsOfResource#3 status=%d body=%q", list3.Code, list3.Body.String())
	}
	var list3Out map[string]any
	if err := json.Unmarshal(list3.Body.Bytes(), &list3Out); err != nil {
		t.Fatal(err)
	}
	if tags, _ := list3Out["Tags"].([]any); len(tags) != 0 {
		t.Fatalf("Tags after UntagResource=%v want empty", list3Out["Tags"])
	}
}
