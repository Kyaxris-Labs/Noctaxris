package store_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestAsyncInvokeFailureDeliversToDLQ(t *testing.T) {
	st := openLambdaStore(t)
	account := "000000000001"
	zip := testZip(t, map[string]string{"app.py": "def handler(e,c): return e"})

	if _, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "lambda-dlq", nil); err != nil {
		t.Fatal(err)
	}
	dlqARN := store.QueueARN("us-east-1", account, "lambda-dlq")

	_, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID:           account,
		Region:              "us-east-1",
		FunctionName:        "async-fail",
		RoleARN:             "arn:aws:iam::000000000001:role/lambda-exec",
		Runtime:             store.LambdaRuntimePython312,
		Handler:             "app.handler",
		Timeout:             3,
		Memory:              128,
		Zip:                 zip,
		DeadLetterTargetArn: dlqARN,
	})
	if err != nil {
		t.Fatal(err)
	}

	eventJSON := `{"task":"boom"}`
	job, err := st.EnqueueAsyncInvoke(account, "async-fail", "$LATEST", eventJSON)
	if err != nil {
		t.Fatal(err)
	}

	invokeErr := errors.New("synthetic invoke failure")
	if err := st.ProcessAsyncInvocation(job.InvocationID, store.LambdaAsyncMaxRetries, func() error {
		return invokeErr
	}); err != nil {
		t.Fatal(err)
	}

	msgs, err := st.ReceiveMessages(account, "lambda-dlq", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("dlq messages=%d want 1", len(msgs))
	}
	var body map[string]any
	if err := json.Unmarshal(msgs[0].Body, &body); err != nil {
		t.Fatal(err)
	}
	payload, _ := body["requestPayload"].(map[string]any)
	if payload == nil || payload["task"] != "boom" {
		t.Fatalf("dlq body=%s", string(msgs[0].Body))
	}
	if body["errorMessage"] != invokeErr.Error() {
		t.Fatalf("errorMessage=%v want %q", body["errorMessage"], invokeErr.Error())
	}

	updated, err := st.GetAsyncInvocation(job.InvocationID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != "failed" {
		t.Fatalf("status=%q want failed", updated.Status)
	}
	if updated.Attempts != store.LambdaAsyncMaxRetries+1 {
		t.Fatalf("attempts=%d want %d", updated.Attempts, store.LambdaAsyncMaxRetries+1)
	}
}

func TestAsyncInvokeDestinationOnFailureDLQ(t *testing.T) {
	st := openLambdaStore(t)
	account := "000000000001"
	zip := testZip(t, map[string]string{"app.py": "x=1"})

	if _, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "dest-dlq", nil); err != nil {
		t.Fatal(err)
	}
	destARN := store.QueueARN("us-east-1", account, "dest-dlq")

	_, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID:               account,
		Region:                  "us-east-1",
		FunctionName:            "dest-fail",
		RoleARN:                 "arn:aws:iam::000000000001:role/lambda-exec",
		Runtime:                 store.LambdaRuntimePython312,
		Handler:                 "app.handler",
		Timeout:                 3,
		Memory:                  128,
		Zip:                     zip,
		DestinationOnFailureArn: destARN,
	})
	if err != nil {
		t.Fatal(err)
	}

	job, err := st.EnqueueAsyncInvoke(account, "dest-fail", "$LATEST", `{"n":1}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ProcessAsyncInvocation(job.InvocationID, store.LambdaAsyncMaxRetries, func() error {
		return errors.New("fail")
	}); err != nil {
		t.Fatal(err)
	}
	msgs, err := st.ReceiveMessages(account, "dest-dlq", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 || !bytes.Contains(msgs[0].Body, []byte(`"n":1`)) {
		t.Fatalf("dlq message=%+v", msgs)
	}
}

func TestAsyncInvokeDestinationOnFailureSNS(t *testing.T) {
	st := openLambdaStore(t)
	account := "000000000001"
	zip := testZip(t, map[string]string{"app.py": "x=1"})

	topic, err := st.CreateTopic(account, "us-east-1", "lambda-fail-topic", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "lambda-fail-q", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Subscribe(account, "lambda-fail-topic", "sqs", store.QueueARN("us-east-1", account, "lambda-fail-q")); err != nil {
		t.Fatal(err)
	}

	_, err = st.CreateFunction(store.CreateFunctionMeta{
		AccountID:               account,
		Region:                  "us-east-1",
		FunctionName:            "dest-fail-sns",
		RoleARN:                 "arn:aws:iam::000000000001:role/lambda-exec",
		Runtime:                 store.LambdaRuntimePython312,
		Handler:                 "app.handler",
		Timeout:                 3,
		Memory:                  128,
		Zip:                     zip,
		DestinationOnFailureArn: topic.TopicARN,
	})
	if err != nil {
		t.Fatal(err)
	}

	job, err := st.EnqueueAsyncInvoke(account, "dest-fail-sns", "$LATEST", `{"n":2}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ProcessAsyncInvocation(job.InvocationID, store.LambdaAsyncMaxRetries, func() error {
		return errors.New("fail")
	}); err != nil {
		t.Fatal(err)
	}
	msgs, err := st.ReceiveMessages(account, "lambda-fail-q", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 || !bytes.Contains(msgs[0].Body, []byte(`\"n\":2`)) {
		t.Fatalf("sns destination message=%+v", msgs)
	}
}
