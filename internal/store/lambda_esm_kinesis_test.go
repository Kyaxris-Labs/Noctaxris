package store_test

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestKinesisESMPollInvokesLambda(t *testing.T) {
	st := openLambdaStore(t)
	account := "000000000001"
	if err := st.EnsureLambdaESMSchema(); err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureKinesisSchema(); err != nil {
		t.Fatal(err)
	}
	trust := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	roleARN, err := st.CreateRole(account, "lambda-kinesis", trust)
	if err != nil {
		t.Fatal(err)
	}
	allowDoc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["kinesis:GetRecords","kinesis:GetShardIterator","kinesis:DescribeStream"],"Resource":"*"}]}`
	if err := st.PutInlinePolicy(roleARN, "esm-kinesis", allowDoc); err != nil {
		t.Fatal(err)
	}
	zip := testZip(t, map[string]string{"app.py": "def handler(e,c): return e"})
	fn, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID:    account,
		Region:       "us-east-1",
		FunctionName: "kinesis-esm-fn",
		Runtime:      store.LambdaRuntimePython312,
		RoleARN:      "arn:aws:iam::" + account + ":role/lambda-kinesis",
		Handler:      "app.handler",
		Zip:          zip,
	})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := st.CreateKinesisStream(account, "us-east-1", "esm-stream", 1)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"order":{"type":"buy"}}`)
	if _, _, err := st.PutKinesisRecord(account, stream.StreamName, "pk1", payload); err != nil {
		t.Fatal(err)
	}
	m, err := st.CreateEventSourceMapping(store.CreateEventSourceMappingInput{
		AccountID:      account,
		FunctionName:   fn.FunctionName,
		EventSourceARN: stream.StreamARN,
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
		if !strings.Contains(eventJSON, `"eventSource":"aws:kinesis"`) {
			t.Fatalf("event=%s", eventJSON)
		}
		if !strings.Contains(eventJSON, stream.StreamARN) {
			t.Fatalf("event=%s", eventJSON)
		}
		var env struct {
			Records []struct {
				Kinesis struct {
					Data         string `json:"data"`
					PartitionKey string `json:"partitionKey"`
				} `json:"kinesis"`
			} `json:"Records"`
		}
		if err := json.Unmarshal([]byte(eventJSON), &env); err != nil {
			t.Fatalf("unmarshal event: %v body=%s", err, eventJSON)
		}
		if len(env.Records) != 1 {
			t.Fatalf("records=%d event=%s", len(env.Records), eventJSON)
		}
		decoded, err := base64.StdEncoding.DecodeString(env.Records[0].Kinesis.Data)
		if err != nil {
			t.Fatalf("decode data: %v", err)
		}
		if string(decoded) != string(payload) {
			t.Fatalf("data=%q want %q", decoded, payload)
		}
		if env.Records[0].Kinesis.PartitionKey != "pk1" {
			t.Fatalf("partitionKey=%q", env.Records[0].Kinesis.PartitionKey)
		}
		return "", nil
	}); err != nil {
		t.Fatal(err)
	}
	if !invoked {
		t.Fatal("expected Kinesis ESM invoke")
	}
	invoked = false
	if err := st.PollEventSourceMappingOnce(m.UUID, func(string, string, string, string) (string, error) {
		invoked = true
		return "", nil
	}); err != nil {
		t.Fatal(err)
	}
	if invoked {
		t.Fatal("expected no redelivery after cursor advance")
	}
}

func TestKinesisESMFilterCriteriaDataAndAuthz(t *testing.T) {
	st := openLambdaStore(t)
	account := "000000000001"
	if err := st.EnsureLambdaESMSchema(); err != nil {
		t.Fatal(err)
	}
	trust := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	roleARN, err := st.CreateRole(account, "lambda-kinesis-fc", trust)
	if err != nil {
		t.Fatal(err)
	}
	allowDoc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["kinesis:GetRecords","kinesis:GetShardIterator"],"Resource":"*"}]}`
	if err := st.PutInlinePolicy(roleARN, "esm-kinesis", allowDoc); err != nil {
		t.Fatal(err)
	}
	zip := testZip(t, map[string]string{"app.py": "def handler(e,c): return e"})
	fn, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID:    account,
		Region:       "us-east-1",
		FunctionName: "kinesis-esm-fc",
		Runtime:      store.LambdaRuntimePython312,
		RoleARN:      "arn:aws:iam::" + account + ":role/lambda-kinesis-fc",
		Handler:      "app.handler",
		Zip:          zip,
	})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := st.CreateKinesisStream(account, "us-east-1", "esm-fc-stream", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.PutKinesisRecord(account, stream.StreamName, "pk-buy", []byte(`{"order":{"type":"buy"}}`)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.PutKinesisRecord(account, stream.StreamName, "pk-sell", []byte(`{"order":{"type":"sell"}}`)); err != nil {
		t.Fatal(err)
	}
	fc := `{"Filters":[{"Pattern":"{\"data\":{\"order\":{\"type\":[\"buy\"]}}}"}]}`
	m, err := st.CreateEventSourceMapping(store.CreateEventSourceMappingInput{
		AccountID:          account,
		FunctionName:       fn.FunctionName,
		EventSourceARN:     stream.StreamARN,
		BatchSize:          10,
		FilterCriteriaJSON: fc,
	})
	if err != nil {
		t.Fatal(err)
	}
	var got string
	if err := st.PollEventSourceMappingOnce(m.UUID, func(_, _, _, eventJSON string) (string, error) {
		got = eventJSON
		return "", nil
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `"eventSource":"aws:kinesis"`) {
		t.Fatalf("event=%s", got)
	}
	if strings.Count(got, `"partitionKey"`) != 1 || !strings.Contains(got, `"pk-buy"`) {
		t.Fatalf("want single buy record event=%s", got)
	}
	if strings.Contains(got, `"pk-sell"`) {
		t.Fatalf("sell record should be filtered out: %s", got)
	}

	noAuthRole, err := st.CreateRole(account, "lambda-kinesis-deny", trust)
	if err != nil {
		t.Fatal(err)
	}
	fnDeny, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID:    account,
		Region:       "us-east-1",
		FunctionName: "kinesis-esm-deny",
		Runtime:      store.LambdaRuntimePython312,
		RoleARN:      noAuthRole,
		Handler:      "app.handler",
		Zip:          zip,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.CreateEventSourceMapping(store.CreateEventSourceMappingInput{
		AccountID:      account,
		FunctionName:   fnDeny.FunctionName,
		EventSourceARN: stream.StreamARN,
	})
	if !errors.Is(err, store.ErrESMSourceAuthz) {
		t.Fatalf("want ErrESMSourceAuthz got %v", err)
	}
}

func TestKinesisESMInvokeErrorDoesNotAdvanceCursor(t *testing.T) {
	st := openLambdaStore(t)
	account := "000000000001"
	trust := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	roleARN, err := st.CreateRole(account, "lambda-kinesis-retry", trust)
	if err != nil {
		t.Fatal(err)
	}
	allowDoc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["kinesis:GetRecords","kinesis:GetShardIterator"],"Resource":"*"}]}`
	if err := st.PutInlinePolicy(roleARN, "esm-kinesis", allowDoc); err != nil {
		t.Fatal(err)
	}
	zip := testZip(t, map[string]string{"app.py": "def handler(e,c): return e"})
	fn, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID:    account,
		Region:       "us-east-1",
		FunctionName: "kinesis-esm-retry",
		Runtime:      store.LambdaRuntimePython312,
		RoleARN:      roleARN,
		Handler:      "app.handler",
		Zip:          zip,
	})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := st.CreateKinesisStream(account, "us-east-1", "esm-retry-stream", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.PutKinesisRecord(account, stream.StreamName, "pk", []byte("hello")); err != nil {
		t.Fatal(err)
	}
	m, err := st.CreateEventSourceMapping(store.CreateEventSourceMappingInput{
		AccountID:      account,
		FunctionName:   fn.FunctionName,
		EventSourceARN: stream.StreamARN,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = st.PollEventSourceMappingOnce(m.UUID, func(string, string, string, string) (string, error) {
		return "", errors.New("invoke failed")
	})
	if err == nil {
		t.Fatal("want invoke error")
	}
	invoked := false
	if err := st.PollEventSourceMappingOnce(m.UUID, func(_, _, _, eventJSON string) (string, error) {
		invoked = true
		if !strings.Contains(eventJSON, "hello") && !strings.Contains(eventJSON, base64.StdEncoding.EncodeToString([]byte("hello"))) {
			t.Fatalf("expected redelivery event=%s", eventJSON)
		}
		return "", nil
	}); err != nil {
		t.Fatal(err)
	}
	if !invoked {
		t.Fatal("expected redelivery after invoke failure")
	}
}
