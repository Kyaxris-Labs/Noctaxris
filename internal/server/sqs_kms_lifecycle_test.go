package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestSQSQueueLifecycleAndErrors(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustSQSJSON(t, handler, "CreateQueue", map[string]any{
		"QueueName": "cov-q",
		"Attributes": map[string]string{
			"VisibilityTimeout": "30",
		},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateQueue %d %s", create.Code, create.Body.String())
	}
	var createOut map[string]any
	if err := json.Unmarshal(create.Body.Bytes(), &createOut); err != nil {
		t.Fatal(err)
	}
	queueURL, _ := createOut["QueueUrl"].(string)
	if queueURL == "" {
		t.Fatalf("missing QueueUrl in %s", create.Body.String())
	}

	urlRec := mustSQSJSON(t, handler, "GetQueueUrl", map[string]any{"QueueName": "cov-q"}, now)
	if urlRec.Code != http.StatusOK || !strings.Contains(urlRec.Body.String(), "cov-q") {
		t.Fatalf("GetQueueUrl %d %s", urlRec.Code, urlRec.Body.String())
	}
	missingURL := mustSQSJSON(t, handler, "GetQueueUrl", map[string]any{"QueueName": "no-such-q"}, now)
	if missingURL.Code == http.StatusOK {
		t.Fatalf("GetQueueUrl missing should fail: %s", missingURL.Body.String())
	}

	attrs := mustSQSJSON(t, handler, "GetQueueAttributes", map[string]any{
		"QueueUrl":       queueURL,
		"AttributeNames": []string{"All"},
	}, now)
	if attrs.Code != http.StatusOK {
		t.Fatalf("GetQueueAttributes %d %s", attrs.Code, attrs.Body.String())
	}

	set := mustSQSJSON(t, handler, "SetQueueAttributes", map[string]any{
		"QueueUrl": queueURL,
		"Attributes": map[string]string{
			"VisibilityTimeout": "45",
		},
	}, now)
	if set.Code != http.StatusOK {
		t.Fatalf("SetQueueAttributes %d %s", set.Code, set.Body.String())
	}

	list := mustSQSJSON(t, handler, "ListQueues", map[string]any{}, now)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "cov-q") {
		t.Fatalf("ListQueues %d %s", list.Code, list.Body.String())
	}

	send := mustSQSJSON(t, handler, "SendMessage", map[string]any{
		"QueueUrl":    queueURL,
		"MessageBody": "hello-sqs",
	}, now)
	if send.Code != http.StatusOK {
		t.Fatalf("SendMessage %d %s", send.Code, send.Body.String())
	}
	sendBad := mustSQSJSON(t, handler, "SendMessage", map[string]any{
		"QueueUrl": "http://127.0.0.1:4566/000000000001/no-such-q", "MessageBody": "x",
	}, now)
	if sendBad.Code == http.StatusOK {
		t.Fatalf("SendMessage missing queue should fail")
	}

	recv := mustSQSJSON(t, handler, "ReceiveMessage", map[string]any{
		"QueueUrl":            queueURL,
		"MaxNumberOfMessages": 1,
		"WaitTimeSeconds":     0,
	}, now)
	if recv.Code != http.StatusOK {
		t.Fatalf("ReceiveMessage %d %s", recv.Code, recv.Body.String())
	}

	tag := mustSQSJSON(t, handler, "TagQueue", map[string]any{
		"QueueUrl": queueURL,
		"Tags":     map[string]string{"env": "lab"},
	}, now)
	if tag.Code != http.StatusOK {
		t.Fatalf("TagQueue %d %s", tag.Code, tag.Body.String())
	}
	listTags := mustSQSJSON(t, handler, "ListQueueTags", map[string]any{"QueueUrl": queueURL}, now)
	if listTags.Code != http.StatusOK {
		t.Fatalf("ListQueueTags %d %s", listTags.Code, listTags.Body.String())
	}
	untag := mustSQSJSON(t, handler, "UntagQueue", map[string]any{
		"QueueUrl": queueURL, "TagKeys": []string{"env"},
	}, now)
	if untag.Code != http.StatusOK {
		t.Fatalf("UntagQueue %d %s", untag.Code, untag.Body.String())
	}

	purge := mustSQSJSON(t, handler, "PurgeQueue", map[string]any{"QueueUrl": queueURL}, now)
	if purge.Code != http.StatusOK {
		t.Fatalf("PurgeQueue %d %s", purge.Code, purge.Body.String())
	}

	del := mustSQSJSON(t, handler, "DeleteQueue", map[string]any{"QueueUrl": queueURL}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteQueue %d %s", del.Code, del.Body.String())
	}
}

func TestKMSKeyLifecycleAndErrors(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustKMSJSON(t, handler, "CreateKey", map[string]any{}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateKey %d %s", create.Code, create.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	meta, _ := created["KeyMetadata"].(map[string]any)
	keyID, _ := meta["KeyId"].(string)
	if keyID == "" {
		t.Fatalf("missing KeyId: %v", created)
	}

	desc := mustKMSJSON(t, handler, "DescribeKey", map[string]any{"KeyId": keyID}, now)
	if desc.Code != http.StatusOK {
		t.Fatalf("DescribeKey %d %s", desc.Code, desc.Body.String())
	}
	missing := mustKMSJSON(t, handler, "DescribeKey", map[string]any{"KeyId": "no-such-key"}, now)
	if missing.Code == http.StatusOK {
		t.Fatalf("DescribeKey missing should fail")
	}

	list := mustKMSJSON(t, handler, "ListKeys", map[string]any{}, now)
	if list.Code != http.StatusOK {
		t.Fatalf("ListKeys %d %s", list.Code, list.Body.String())
	}

	alias := mustKMSJSON(t, handler, "CreateAlias", map[string]any{
		"AliasName":   "alias/lab-cov",
		"TargetKeyId": keyID,
	}, now)
	if alias.Code != http.StatusOK {
		t.Fatalf("CreateAlias %d %s", alias.Code, alias.Body.String())
	}
	listAlias := mustKMSJSON(t, handler, "ListAliases", map[string]any{}, now)
	if listAlias.Code != http.StatusOK || !strings.Contains(listAlias.Body.String(), "lab-cov") {
		t.Fatalf("ListAliases %d %s", listAlias.Code, listAlias.Body.String())
	}

	enc := mustKMSJSON(t, handler, "Encrypt", map[string]any{
		"KeyId":     keyID,
		"Plaintext": "aGVsbG8=",
	}, now)
	if enc.Code != http.StatusOK {
		t.Fatalf("Encrypt %d %s", enc.Code, enc.Body.String())
	}
	var encOut map[string]any
	if err := json.Unmarshal(enc.Body.Bytes(), &encOut); err != nil {
		t.Fatal(err)
	}
	blob, _ := encOut["CiphertextBlob"].(string)
	if blob == "" {
		t.Fatalf("missing CiphertextBlob: %v", encOut)
	}
	dec := mustKMSJSON(t, handler, "Decrypt", map[string]any{
		"CiphertextBlob": blob,
	}, now)
	if dec.Code != http.StatusOK {
		t.Fatalf("Decrypt %d %s", dec.Code, dec.Body.String())
	}

	delAlias := mustKMSJSON(t, handler, "DeleteAlias", map[string]any{
		"AliasName": "alias/lab-cov",
	}, now)
	if delAlias.Code != http.StatusOK {
		t.Fatalf("DeleteAlias %d %s", delAlias.Code, delAlias.Body.String())
	}

	sched := mustKMSJSON(t, handler, "ScheduleKeyDeletion", map[string]any{
		"KeyId":               keyID,
		"PendingWindowInDays": 7,
	}, now)
	if sched.Code != http.StatusOK {
		t.Fatalf("ScheduleKeyDeletion %d %s", sched.Code, sched.Body.String())
	}
	cancel := mustKMSJSON(t, handler, "CancelKeyDeletion", map[string]any{"KeyId": keyID}, now)
	if cancel.Code != http.StatusOK {
		t.Fatalf("CancelKeyDeletion %d %s", cancel.Code, cancel.Body.String())
	}
}
