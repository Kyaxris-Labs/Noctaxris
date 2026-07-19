package server_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestFIFOCreateSendReceiveOrdering(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustSQSJSON(t, handler, "CreateQueue", map[string]any{
		"QueueName": "lab-orders.fifo",
		"Attributes": map[string]string{
			"FifoQueue":                 "true",
			"ContentBasedDeduplication":   "true",
			"VisibilityTimeout":         "0",
		},
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

	for _, body := range []string{"one", "two"} {
		sendRec := mustSQSJSON(t, handler, "SendMessage", map[string]any{
			"QueueUrl":       queueURL,
			"MessageBody":    body,
			"MessageGroupId": "g1",
		}, now)
		if sendRec.Code != http.StatusOK {
			t.Fatalf("SendMessage %q status=%d body=%q", body, sendRec.Code, sendRec.Body.String())
		}
	}

	dupRec := mustSQSJSON(t, handler, "SendMessage", map[string]any{
		"QueueUrl":       queueURL,
		"MessageBody":    "one",
		"MessageGroupId": "g1",
	}, now)
	if dupRec.Code != http.StatusOK {
		t.Fatalf("dedup send status=%d body=%q", dupRec.Code, dupRec.Body.String())
	}

	recvRec := mustSQSJSON(t, handler, "ReceiveMessage", map[string]any{
		"QueueUrl":            queueURL,
		"MaxNumberOfMessages": 1,
	}, now)
	if recvRec.Code != http.StatusOK {
		t.Fatalf("ReceiveMessage status=%d body=%q", recvRec.Code, recvRec.Body.String())
	}
	var recvOut map[string]any
	if err := json.Unmarshal(recvRec.Body.Bytes(), &recvOut); err != nil {
		t.Fatal(err)
	}
	msgs, _ := recvOut["Messages"].([]any)
	if len(msgs) != 1 {
		t.Fatalf("Messages len=%d body=%q", len(msgs), recvRec.Body.String())
	}
	msg, _ := msgs[0].(map[string]any)
	if body, _ := msg["Body"].(string); body != "one" {
		t.Fatalf("Body=%q want one", body)
	}
	attrs, _ := msg["Attributes"].(map[string]any)
	if attrs["MessageGroupId"] != "g1" {
		t.Fatalf("MessageGroupId=%v", attrs["MessageGroupId"])
	}
}

func TestFIFOCreateRejectsMissingFifoSuffix(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createRec := mustSQSJSON(t, handler, "CreateQueue", map[string]any{
		"QueueName": "not-fifo",
		"Attributes": map[string]string{
			"FifoQueue": "true",
		},
	}, now)
	if createRec.Code != http.StatusBadRequest {
		t.Fatalf("CreateQueue status=%d want 400 body=%q", createRec.Code, createRec.Body.String())
	}
}

func TestSQSFIFORedrivePolicy(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	dlqRec := mustSQSJSON(t, handler, "CreateQueue", map[string]any{
		"QueueName": "lab-dlq.fifo",
		"Attributes": map[string]string{
			"FifoQueue": "true",
		},
	}, now)
	if dlqRec.Code != http.StatusOK {
		t.Fatalf("CreateQueue DLQ status=%d body=%q", dlqRec.Code, dlqRec.Body.String())
	}
	var dlqOut map[string]any
	if err := json.Unmarshal(dlqRec.Body.Bytes(), &dlqOut); err != nil {
		t.Fatal(err)
	}
	dlqURL, _ := dlqOut["QueueUrl"].(string)

	attrRec := mustSQSJSON(t, handler, "GetQueueAttributes", map[string]any{
		"QueueUrl": dlqURL,
		"AttributeNames": []string{
			"QueueArn",
		},
	}, now)
	if attrRec.Code != http.StatusOK {
		t.Fatalf("GetQueueAttributes status=%d body=%q", attrRec.Code, attrRec.Body.String())
	}
	var attrOut map[string]any
	if err := json.Unmarshal(attrRec.Body.Bytes(), &attrOut); err != nil {
		t.Fatal(err)
	}
	attrs, _ := attrOut["Attributes"].(map[string]any)
	dlqARN, _ := attrs["QueueArn"].(string)
	if dlqARN == "" {
		t.Fatalf("missing QueueArn in %q", attrRec.Body.String())
	}

	srcRec := mustSQSJSON(t, handler, "CreateQueue", map[string]any{
		"QueueName": "lab-source",
		"Attributes": map[string]string{
			"VisibilityTimeout": "0",
			"RedrivePolicy":     `{"deadLetterTargetArn":"` + dlqARN + `","maxReceiveCount":"1"}`,
		},
	}, now)
	if srcRec.Code != http.StatusOK {
		t.Fatalf("CreateQueue source status=%d body=%q", srcRec.Code, srcRec.Body.String())
	}
	var srcOut map[string]any
	if err := json.Unmarshal(srcRec.Body.Bytes(), &srcOut); err != nil {
		t.Fatal(err)
	}
	srcURL, _ := srcOut["QueueUrl"].(string)

	sendRec := mustSQSJSON(t, handler, "SendMessage", map[string]any{
		"QueueUrl":    srcURL,
		"MessageBody": "poison",
	}, now)
	if sendRec.Code != http.StatusOK {
		t.Fatalf("SendMessage status=%d body=%q", sendRec.Code, sendRec.Body.String())
	}

	recv1 := mustSQSJSON(t, handler, "ReceiveMessage", map[string]any{
		"QueueUrl":            srcURL,
		"MaxNumberOfMessages": 1,
	}, now)
	if recv1.Code != http.StatusOK {
		t.Fatalf("ReceiveMessage 1 status=%d body=%q", recv1.Code, recv1.Body.String())
	}
	var recv1Out map[string]any
	if err := json.Unmarshal(recv1.Body.Bytes(), &recv1Out); err != nil {
		t.Fatal(err)
	}
	msgs1, _ := recv1Out["Messages"].([]any)
	if len(msgs1) != 1 {
		t.Fatalf("first receive len=%d", len(msgs1))
	}
	msg1, _ := msgs1[0].(map[string]any)
	handle, _ := msg1["ReceiptHandle"].(string)

	visRec := mustSQSJSON(t, handler, "ChangeMessageVisibility", map[string]any{
		"QueueUrl":          srcURL,
		"ReceiptHandle":     handle,
		"VisibilityTimeout": 0,
	}, now)
	if visRec.Code != http.StatusOK {
		t.Fatalf("ChangeMessageVisibility status=%d body=%q", visRec.Code, visRec.Body.String())
	}

	recv2 := mustSQSJSON(t, handler, "ReceiveMessage", map[string]any{
		"QueueUrl":            srcURL,
		"MaxNumberOfMessages": 1,
	}, now)
	if recv2.Code != http.StatusOK {
		t.Fatalf("ReceiveMessage 2 status=%d body=%q", recv2.Code, recv2.Body.String())
	}
	var recv2Out map[string]any
	if err := json.Unmarshal(recv2.Body.Bytes(), &recv2Out); err != nil {
		t.Fatal(err)
	}
	if msgs2, _ := recv2Out["Messages"].([]any); len(msgs2) != 0 {
		t.Fatalf("expected empty source queue after redrive, got %d", len(msgs2))
	}

	dlqRecv := mustSQSJSON(t, handler, "ReceiveMessage", map[string]any{
		"QueueUrl":            dlqURL,
		"MaxNumberOfMessages": 1,
	}, now)
	if dlqRecv.Code != http.StatusOK {
		t.Fatalf("DLQ ReceiveMessage status=%d body=%q", dlqRecv.Code, dlqRecv.Body.String())
	}
	var dlqRecvOut map[string]any
	if err := json.Unmarshal(dlqRecv.Body.Bytes(), &dlqRecvOut); err != nil {
		t.Fatal(err)
	}
	dlqMsgs, _ := dlqRecvOut["Messages"].([]any)
	if len(dlqMsgs) != 1 {
		t.Fatalf("DLQ messages len=%d body=%q", len(dlqMsgs), dlqRecv.Body.String())
	}
	dlqMsg, _ := dlqMsgs[0].(map[string]any)
	if body, _ := dlqMsg["Body"].(string); body != "poison" {
		t.Fatalf("DLQ body=%q want poison", body)
	}
}
