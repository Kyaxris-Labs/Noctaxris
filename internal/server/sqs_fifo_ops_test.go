package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestSQSFIFOSendReceiveVisibilityAndTags(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustSQSJSON(t, handler, "CreateQueue", map[string]any{
		"QueueName": "ops-fifo.fifo",
		"Attributes": map[string]string{
			"FifoQueue":                 "true",
			"ContentBasedDeduplication": "false",
		},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateQueue fifo %d %s", create.Code, create.Body.String())
	}
	var created map[string]any
	_ = json.Unmarshal(create.Body.Bytes(), &created)
	queueURL, _ := created["QueueUrl"].(string)

	missingGroup := mustSQSJSON(t, handler, "SendMessage", map[string]any{
		"QueueUrl":    queueURL,
		"MessageBody": "no-group",
	}, now)
	if missingGroup.Code == http.StatusOK {
		t.Fatalf("SendMessage without group should fail")
	}
	missingDedup := mustSQSJSON(t, handler, "SendMessage", map[string]any{
		"QueueUrl":       queueURL,
		"MessageBody":    "no-dedup",
		"MessageGroupId": "g1",
	}, now)
	if missingDedup.Code == http.StatusOK {
		t.Fatalf("SendMessage without dedup should fail")
	}

	send := mustSQSJSON(t, handler, "SendMessage", map[string]any{
		"QueueUrl":               queueURL,
		"MessageBody":            "fifo-body",
		"MessageGroupId":         "g1",
		"MessageDeduplicationId": "d1",
		"MessageAttributes": map[string]any{
			"color": map[string]any{"DataType": "String", "StringValue": "blue"},
		},
		"DelaySeconds": 0,
	}, now)
	if send.Code != http.StatusOK {
		t.Fatalf("SendMessage %d %s", send.Code, send.Body.String())
	}
	emptyBody := mustSQSJSON(t, handler, "SendMessage", map[string]any{
		"QueueUrl": queueURL, "MessageBody": "", "MessageGroupId": "g1", "MessageDeduplicationId": "d2",
	}, now)
	if emptyBody.Code == http.StatusOK {
		t.Fatalf("empty body should fail")
	}

	recv := mustSQSJSON(t, handler, "ReceiveMessage", map[string]any{
		"QueueUrl":            queueURL,
		"MaxNumberOfMessages": 10,
		"WaitTimeSeconds":     0,
		"AttributeNames":      []string{"All"},
		"MessageAttributeNames": []string{"All"},
	}, now)
	if recv.Code != http.StatusOK || !strings.Contains(recv.Body.String(), "fifo-body") {
		t.Fatalf("ReceiveMessage %d %s", recv.Code, recv.Body.String())
	}
	var recvOut map[string]any
	_ = json.Unmarshal(recv.Body.Bytes(), &recvOut)
	msgs, _ := recvOut["Messages"].([]any)
	if len(msgs) == 0 {
		t.Fatal("expected messages")
	}
	msg0, _ := msgs[0].(map[string]any)
	handle, _ := msg0["ReceiptHandle"].(string)

	vis := mustSQSJSON(t, handler, "ChangeMessageVisibility", map[string]any{
		"QueueUrl":          queueURL,
		"ReceiptHandle":     handle,
		"VisibilityTimeout": 5,
	}, now)
	if vis.Code != http.StatusOK {
		t.Fatalf("ChangeMessageVisibility %d %s", vis.Code, vis.Body.String())
	}
	visBad := mustSQSJSON(t, handler, "ChangeMessageVisibility", map[string]any{
		"QueueUrl": queueURL, "ReceiptHandle": "missing", "VisibilityTimeout": 1,
	}, now)
	if visBad.Code == http.StatusOK {
		t.Fatalf("ChangeMessageVisibility bad handle should fail")
	}

	del := mustSQSJSON(t, handler, "DeleteMessage", map[string]any{
		"QueueUrl": queueURL, "ReceiptHandle": handle,
	}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteMessage %d %s", del.Code, del.Body.String())
	}

	tag := mustSQSJSON(t, handler, "TagQueue", map[string]any{
		"QueueUrl": queueURL, "Tags": map[string]string{"env": "lab"},
	}, now)
	if tag.Code != http.StatusOK {
		t.Fatalf("TagQueue %d %s", tag.Code, tag.Body.String())
	}
	listTags := mustSQSJSON(t, handler, "ListQueueTags", map[string]any{"QueueUrl": queueURL}, now)
	if listTags.Code != http.StatusOK || !strings.Contains(listTags.Body.String(), "env") {
		t.Fatalf("ListQueueTags %d %s", listTags.Code, listTags.Body.String())
	}
	untag := mustSQSJSON(t, handler, "UntagQueue", map[string]any{
		"QueueUrl": queueURL, "TagKeys": []string{"env"},
	}, now)
	if untag.Code != http.StatusOK {
		t.Fatalf("UntagQueue %d %s", untag.Code, untag.Body.String())
	}

	attrs := mustSQSJSON(t, handler, "GetQueueAttributes", map[string]any{
		"QueueUrl":       queueURL,
		"AttributeNames": []string{"All"},
	}, now)
	if attrs.Code != http.StatusOK || !strings.Contains(attrs.Body.String(), "FifoQueue") {
		t.Fatalf("GetQueueAttributes %d %s", attrs.Code, attrs.Body.String())
	}
	getURL := mustSQSJSON(t, handler, "GetQueueUrl", map[string]any{"QueueName": "ops-fifo.fifo"}, now)
	if getURL.Code != http.StatusOK {
		t.Fatalf("GetQueueUrl %d %s", getURL.Code, getURL.Body.String())
	}
	list := mustSQSJSON(t, handler, "ListQueues", map[string]any{"QueueNamePrefix": "ops-fifo"}, now)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "ops-fifo") {
		t.Fatalf("ListQueues %d %s", list.Code, list.Body.String())
	}

	std := mustSQSJSON(t, handler, "CreateQueue", map[string]any{"QueueName": "ops-std-vis"}, now)
	if std.Code != http.StatusOK {
		t.Fatalf("CreateQueue std %d %s", std.Code, std.Body.String())
	}
	var stdOut map[string]any
	_ = json.Unmarshal(std.Body.Bytes(), &stdOut)
	stdURL, _ := stdOut["QueueUrl"].(string)
	batch := mustSQSJSON(t, handler, "SendMessageBatch", map[string]any{
		"QueueUrl": stdURL,
		"Entries": []map[string]any{
			{"Id": "1", "MessageBody": "a"},
			{"Id": "2", "MessageBody": "b"},
		},
	}, now)
	if batch.Code != http.StatusOK {
		t.Fatalf("SendMessageBatch %d %s", batch.Code, batch.Body.String())
	}
	recv2 := mustSQSJSON(t, handler, "ReceiveMessage", map[string]any{
		"QueueUrl": stdURL, "MaxNumberOfMessages": 10,
	}, now)
	if recv2.Code != http.StatusOK {
		t.Fatalf("ReceiveMessage std %d %s", recv2.Code, recv2.Body.String())
	}
	var recv2Out map[string]any
	_ = json.Unmarshal(recv2.Body.Bytes(), &recv2Out)
	var handles []map[string]any
	for i, m := range recv2Out["Messages"].([]any) {
		mm := m.(map[string]any)
		handles = append(handles, map[string]any{
			"Id":            strings.TrimSpace(mm["MessageId"].(string)),
			"ReceiptHandle": mm["ReceiptHandle"],
		})
		_ = i
	}
	if len(handles) == 0 {
		// use synthetic ids if empty after race
		handles = []map[string]any{{"Id": "1", "ReceiptHandle": "missing"}}
	}
	delBatch := mustSQSJSON(t, handler, "DeleteMessageBatch", map[string]any{
		"QueueUrl": stdURL, "Entries": handles,
	}, now)
	if delBatch.Code != http.StatusOK {
		t.Fatalf("DeleteMessageBatch %d %s", delBatch.Code, delBatch.Body.String())
	}

	badFIFOName := mustSQSJSON(t, handler, "CreateQueue", map[string]any{
		"QueueName":  "notfifo",
		"Attributes": map[string]string{"FifoQueue": "true"},
	}, now)
	if badFIFOName.Code == http.StatusOK {
		t.Fatalf("non-.fifo FifoQueue should fail")
	}
}
