package store_test

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestMQESMRejectsStubAndCreationFailedBroker(t *testing.T) {
	st := openLambdaStore(t)
	account := "000000000001"
	if err := st.EnsureLambdaESMSchema(); err != nil {
		t.Fatal(err)
	}
	trust := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	roleARN, err := st.CreateRole(account, "lambda-mq-deny", trust)
	if err != nil {
		t.Fatal(err)
	}
	allowDoc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["mq:DescribeBroker"],"Resource":"*"}]}`
	if err := st.PutInlinePolicy(roleARN, "esm-mq", allowDoc); err != nil {
		t.Fatal(err)
	}
	zip := testZip(t, map[string]string{"app.py": "def handler(e,c): return e"})
	fn, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID:    account,
		Region:       "us-east-1",
		FunctionName: "mq-esm-reject",
		Runtime:      store.LambdaRuntimePython312,
		RoleARN:      roleARN,
		Handler:      "app.handler",
		Zip:          zip,
	})
	if err != nil {
		t.Fatal(err)
	}
	b, err := st.CreateMQBroker(account, "us-east-1", "stub-broker", "ACTIVEMQ", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	// CREATION_IN_PROGRESS / not RUNNING must fail closed.
	_, err = st.CreateEventSourceMapping(store.CreateEventSourceMappingInput{
		AccountID:      account,
		FunctionName:   fn.FunctionName,
		EventSourceARN: b.BrokerARN,
	})
	if !errors.Is(err, store.ErrESMMQBrokerNotReady) {
		t.Fatalf("want ErrESMMQBrokerNotReady for creating broker, got %v", err)
	}
	if err := st.SetMQContainerID(account, b.BrokerID, "", store.MQBrokerStateCreationFailed, "stub://127.0.0.1/mq/"+b.BrokerID); err != nil {
		t.Fatal(err)
	}
	_, err = st.CreateEventSourceMapping(store.CreateEventSourceMappingInput{
		AccountID:      account,
		FunctionName:   fn.FunctionName,
		EventSourceARN: b.BrokerARN,
	})
	if !errors.Is(err, store.ErrESMMQBrokerNotReady) {
		t.Fatalf("want ErrESMMQBrokerNotReady for stub broker, got %v", err)
	}
}

func TestMQESMPollInvokesWithInjectableReceiver(t *testing.T) {
	st := openLambdaStore(t)
	account := "000000000001"
	if err := st.EnsureLambdaESMSchema(); err != nil {
		t.Fatal(err)
	}
	trust := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	roleARN, err := st.CreateRole(account, "lambda-mq", trust)
	if err != nil {
		t.Fatal(err)
	}
	allowDoc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["mq:DescribeBroker"],"Resource":"*"}]}`
	if err := st.PutInlinePolicy(roleARN, "esm-mq", allowDoc); err != nil {
		t.Fatal(err)
	}
	zip := testZip(t, map[string]string{"app.py": "def handler(e,c): return e"})
	fn, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID:    account,
		Region:       "us-east-1",
		FunctionName: "mq-esm-fn",
		Runtime:      store.LambdaRuntimePython312,
		RoleARN:      roleARN,
		Handler:      "app.handler",
		Zip:          zip,
	})
	if err != nil {
		t.Fatal(err)
	}
	b, err := st.CreateMQBroker(account, "us-east-1", "run-broker", "ACTIVEMQ", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	ep := store.MQNestedAMQPEndpoint(b.BrokerID)
	if err := st.SetMQContainerID(account, b.BrokerID, "ctr-mq", store.MQBrokerStateRunning, ep); err != nil {
		t.Fatal(err)
	}
	payload := []byte("hello-mq")
	store.SetMQReceiveFunc(func(endpoint string, batchSize int) ([]store.MQESMMessage, error) {
		if endpoint != ep {
			t.Fatalf("endpoint=%q want %q", endpoint, ep)
		}
		if batchSize < 1 {
			t.Fatalf("batchSize=%d", batchSize)
		}
		return []store.MQESMMessage{{
			MessageID:   "msg-1",
			Data:        payload,
			Destination: store.LambdaESMLabMQQueue,
		}}, nil
	})
	t.Cleanup(func() { store.SetMQReceiveFunc(nil) })

	m, err := st.CreateEventSourceMapping(store.CreateEventSourceMappingInput{
		AccountID:      account,
		FunctionName:   fn.FunctionName,
		EventSourceARN: b.BrokerARN,
		BatchSize:      5,
	})
	if err != nil {
		t.Fatal(err)
	}
	invoked := false
	if err := st.PollEventSourceMappingOnce(m.UUID, func(acct, name, _, eventJSON string) (string, error) {
		invoked = true
		if acct != account || name != fn.FunctionName {
			t.Fatalf("invoke acct=%s name=%s", acct, name)
		}
		if !strings.Contains(eventJSON, `"eventSource":"aws:mq"`) {
			t.Fatalf("event=%s", eventJSON)
		}
		if !strings.Contains(eventJSON, b.BrokerARN) {
			t.Fatalf("event=%s", eventJSON)
		}
		if !strings.Contains(eventJSON, base64.StdEncoding.EncodeToString(payload)) {
			t.Fatalf("missing payload event=%s", eventJSON)
		}
		return "", nil
	}); err != nil {
		t.Fatal(err)
	}
	if !invoked {
		t.Fatal("expected MQ ESM invoke")
	}
}

func TestMQESMAuthzAndNestedHostAllowlist(t *testing.T) {
	st := openLambdaStore(t)
	account := "000000000001"
	if err := store.ValidateNestedMQHost("noctaxris-mq-abc"); err != nil {
		t.Fatal(err)
	}
	if err := store.ValidateNestedMQHost("127.0.0.1"); err == nil {
		t.Fatal("loopback must be rejected")
	}
	if err := store.ValidateNestedMQHost("evil.example"); err == nil {
		t.Fatal("non-prefix host must be rejected")
	}

	trust := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	noAuthRole, err := st.CreateRole(account, "lambda-mq-noauth", trust)
	if err != nil {
		t.Fatal(err)
	}
	zip := testZip(t, map[string]string{"app.py": "def handler(e,c): return e"})
	fn, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID:    account,
		Region:       "us-east-1",
		FunctionName: "mq-esm-noauth",
		Runtime:      store.LambdaRuntimePython312,
		RoleARN:      noAuthRole,
		Handler:      "app.handler",
		Zip:          zip,
	})
	if err != nil {
		t.Fatal(err)
	}
	b, err := st.CreateMQBroker(account, "us-east-1", "auth-broker", "RABBITMQ", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	ep := store.MQNestedAMQPEndpoint(b.BrokerID)
	if err := st.SetMQContainerID(account, b.BrokerID, "ctr-r", store.MQBrokerStateRunning, ep); err != nil {
		t.Fatal(err)
	}
	_, err = st.CreateEventSourceMapping(store.CreateEventSourceMappingInput{
		AccountID:      account,
		FunctionName:   fn.FunctionName,
		EventSourceARN: b.BrokerARN,
	})
	if !errors.Is(err, store.ErrESMSourceAuthz) {
		t.Fatalf("want ErrESMSourceAuthz got %v", err)
	}

	// RUNNING with non-allowlisted host must fail Create.
	roleARN, err := st.CreateRole(account, "lambda-mq-host", trust)
	if err != nil {
		t.Fatal(err)
	}
	allowDoc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["mq:DescribeBroker"],"Resource":"*"}]}`
	if err := st.PutInlinePolicy(roleARN, "esm-mq", allowDoc); err != nil {
		t.Fatal(err)
	}
	fnOK, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID:    account,
		Region:       "us-east-1",
		FunctionName: "mq-esm-host",
		Runtime:      store.LambdaRuntimePython312,
		RoleARN:      roleARN,
		Handler:      "app.handler",
		Zip:          zip,
	})
	if err != nil {
		t.Fatal(err)
	}
	b2, err := st.CreateMQBroker(account, "us-east-1", "bad-host-broker", "ACTIVEMQ", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetMQContainerID(account, b2.BrokerID, "ctr-bad", store.MQBrokerStateRunning, "amqp://evil.example:5672"); err != nil {
		t.Fatal(err)
	}
	_, err = st.CreateEventSourceMapping(store.CreateEventSourceMappingInput{
		AccountID:      account,
		FunctionName:   fnOK.FunctionName,
		EventSourceARN: b2.BrokerARN,
	})
	if !errors.Is(err, store.ErrESMMQHostNotAllowed) {
		t.Fatalf("want ErrESMMQHostNotAllowed got %v", err)
	}
}
