package sqs_test

import (
	"bytes"
	"encoding/json"
	"testing"

	sqssvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/sqs"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestSQSJSONAndCrypto(t *testing.T) {
	url := "http://127.0.0.1:4566/000000000001/lab-queue"
	if _, err := sqssvc.CreateQueueJSON(url); err != nil {
		t.Fatal(err)
	}
	if _, err := sqssvc.GetQueueURLJSON(url); err != nil {
		t.Fatal(err)
	}
	if _, err := sqssvc.GetQueueAttributesJSON(map[string]string{"VisibilityTimeout": "30"}); err != nil {
		t.Fatal(err)
	}
	if _, err := sqssvc.GetQueueAttributesJSON(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := sqssvc.ListQueuesJSON([]store.Queue{{QueueURL: url}}); err != nil {
		t.Fatal(err)
	}
	if _, err := sqssvc.EmptyOKJSON(); err != nil {
		t.Fatal(err)
	}
	if _, err := sqssvc.ListQueueTagsJSON(map[string]string{"env": "lab"}); err != nil {
		t.Fatal(err)
	}
	md5 := sqssvc.MD5Hex("body")
	if _, err := sqssvc.SendMessageJSON("mid", md5, md5, 42); err != nil {
		t.Fatal(err)
	}
	msg := sqssvc.ReceivedMessage{
		MessageID: "m1", ReceiptHandle: "rh", Body: "hi", MD5OfBody: md5,
		Attributes: map[string]string{"SenderId": "1"}, MessageGroupID: "g1", SequenceNumber: 7,
		MessageAttributes: map[string]any{"k": map[string]any{"StringValue": "v", "DataType": "String"}},
	}
	recv, err := sqssvc.ReceiveMessageJSON([]sqssvc.ReceivedMessage{msg})
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(recv) {
		t.Fatal("receive invalid json")
	}
	ok := []sqssvc.SendMessageBatchSuccess{{ID: "1", MessageID: "m1", MD5OfMessageBody: md5}}
	fail := []sqssvc.BatchFailure{{ID: "2", Code: "Error", Message: "bad", SenderFault: true}}
	if _, err := sqssvc.SendMessageBatchJSON(ok, fail); err != nil {
		t.Fatal(err)
	}
	if _, err := sqssvc.DeleteMessageBatchJSON([]string{"m1"}, fail); err != nil {
		t.Fatal(err)
	}
	attrMD5, err := sqssvc.MD5OfMessageAttributesHex(map[string]any{"a": map[string]any{"StringValue": "x"}})
	if err != nil || attrMD5 == "" {
		t.Fatalf("md5 attrs: %s %v", attrMD5, err)
	}
	filtered := sqssvc.FilterAttributes(map[string]string{"A": "1", "B": "2"}, []string{"A"})
	if filtered["B"] != "" || filtered["A"] != "1" {
		t.Fatalf("filter=%v", filtered)
	}

	if sqssvc.ModeFromAttributes(nil) != sqssvc.SSENone {
		t.Fatal("sse none")
	}
	if sqssvc.ModeFromAttributes(map[string]string{"KmsMasterKeyId": "key"}) != sqssvc.SSEKMS {
		t.Fatal("sse kms")
	}
	if sqssvc.ModeFromAttributes(map[string]string{"SqsManagedSseEnabled": "true"}) != sqssvc.SSESQS {
		t.Fatal("sse sqs")
	}
	if sqssvc.KmsMasterKeyID(map[string]string{"KmsMasterKeyId": "  key "}) != "key" {
		t.Fatal("kms id")
	}
	dek, err := sqssvc.NewRandomDEK()
	if err != nil || len(dek) != 32 {
		t.Fatal(err)
	}
	sealed, err := sqssvc.EncryptAES256GCM(dek, []byte("plaintext"))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := sqssvc.DecryptAES256GCM(dek, sealed)
	if err != nil || !bytes.Equal(plain, []byte("plaintext")) {
		t.Fatalf("roundtrip: %q %v", plain, err)
	}
}
