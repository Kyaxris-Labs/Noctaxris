package store_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestESMFilterCriteriaOperatorsSQS(t *testing.T) {
	st := openLambdaStore(t)
	account := "000000000001"
	if err := st.EnsureLambdaESMSchema(); err != nil {
		t.Fatal(err)
	}
	zip := testZip(t, map[string]string{"app.py": "def handler(e,c): return e"})
	fn, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID: account, Region: "us-east-1", FunctionName: "filt-ops",
		Runtime: store.LambdaRuntimePython312, RoleARN: "arn:aws:iam::" + account + ":role/lambda",
		Handler: "app.handler", Zip: zip,
	})
	if err != nil {
		t.Fatal(err)
	}
	q, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "filt-ops-q", nil)
	if err != nil {
		t.Fatal(err)
	}
	allowLambdaESMQueuePolicy(t, st, account, q.QueueName)

	// Match JSON body with prefix + numeric + anything-but (EventBridge syntax).
	fc := `{"Filters":[{"Pattern":"{\"body\":{\"region\":[{\"prefix\":\"us-\"}],\"amount\":[{\"numeric\":[\">\",10,\"<=\",100]}],\"status\":[{\"anything-but\":\"drop\"}]}}"}]}`
	m, err := st.CreateEventSourceMapping(store.CreateEventSourceMappingInput{
		AccountID: account, FunctionName: fn.FunctionName, EventSourceARN: q.QueueARN,
		FilterCriteriaJSON: fc,
	})
	if err != nil {
		t.Fatal(err)
	}
	if m.FilterCriteriaJSON == "" {
		t.Fatal("expected FilterCriteria persisted")
	}

	bodies := []string{
		`{"region":"eu-west-1","amount":50,"status":"ok"}`,   // prefix miss
		`{"region":"us-east-1","amount":5,"status":"ok"}`,    // numeric miss
		`{"region":"us-east-1","amount":50,"status":"drop"}`, // anything-but miss
		`{"region":"us-west-2","amount":50,"status":"ok"}`,   // match
	}
	for _, b := range bodies {
		if _, err := st.SendMessage(account, q.QueueName, []byte(b), false, nil, "", nil); err != nil {
			t.Fatal(err)
		}
	}
	invoked := 0
	if err := st.PollEventSourceMappingOnce(m.UUID, func(_, _, _, eventJSON string) (string, error) {
		invoked++
		if !strings.Contains(eventJSON, "us-west-2") {
			t.Fatalf("event=%s", eventJSON)
		}
		var payload map[string]any
		_ = json.Unmarshal([]byte(eventJSON), &payload)
		recs, _ := payload["Records"].([]any)
		if len(recs) != 1 {
			t.Fatalf("want 1 matched record got %d", len(recs))
		}
		return "", nil
	}); err != nil {
		t.Fatal(err)
	}
	if invoked != 1 {
		t.Fatalf("invoked=%d", invoked)
	}
}

func TestESMFilterCriteriaRejectUnsupportedOperators(t *testing.T) {
	st := openLambdaStore(t)
	account := "000000000001"
	if err := st.EnsureLambdaESMSchema(); err != nil {
		t.Fatal(err)
	}
	zip := testZip(t, map[string]string{"app.py": "def handler(e,c): return e"})
	fn, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID: account, Region: "us-east-1", FunctionName: "filt-reject",
		Runtime: store.LambdaRuntimePython312, RoleARN: "arn:aws:iam::" + account + ":role/lambda",
		Handler: "app.handler", Zip: zip,
	})
	if err != nil {
		t.Fatal(err)
	}
	q, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "filt-reject-q", nil)
	if err != nil {
		t.Fatal(err)
	}
	allowLambdaESMQueuePolicy(t, st, account, q.QueueName)

	bad := []string{
		`{"Filters":[{"Pattern":"{\"$or\":[{\"body\":[\"a\"]},{\"body\":[\"b\"]}]}"}]}`,
		`{"Filters":[{"Pattern":"{\"body\":[{\"wildcard\":\"*/x\"}]}"}]}`,
		`{"Filters":[{"Pattern":"{\"body\":[{\"cidr\":\"10.0.0.0/8\"}]}"}]}`,
	}
	for _, fc := range bad {
		_, err := st.CreateEventSourceMapping(store.CreateEventSourceMappingInput{
			AccountID: account, FunctionName: fn.FunctionName, EventSourceARN: q.QueueARN,
			FilterCriteriaJSON: fc,
		})
		if !errors.Is(err, store.ErrInvalidESMFilterCriteria) {
			t.Fatalf("want ErrInvalidESMFilterCriteria for %s got %v", fc, err)
		}
	}
}

func TestESMFilterCriteriaUpdateAndExists(t *testing.T) {
	st := openLambdaStore(t)
	account := "000000000001"
	if err := st.EnsureLambdaESMSchema(); err != nil {
		t.Fatal(err)
	}
	zip := testZip(t, map[string]string{"app.py": "def handler(e,c): return e"})
	fn, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID: account, Region: "us-east-1", FunctionName: "filt-upd",
		Runtime: store.LambdaRuntimePython312, RoleARN: "arn:aws:iam::" + account + ":role/lambda",
		Handler: "app.handler", Zip: zip,
	})
	if err != nil {
		t.Fatal(err)
	}
	q, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "filt-upd-q", nil)
	if err != nil {
		t.Fatal(err)
	}
	allowLambdaESMQueuePolicy(t, st, account, q.QueueName)

	m, err := st.CreateEventSourceMapping(store.CreateEventSourceMappingInput{
		AccountID: account, FunctionName: fn.FunctionName, EventSourceARN: q.QueueARN,
		FilterCriteriaJSON: `{"Filters":[{"Pattern":"{\"body\":{\"flag\":[{\"exists\":true}]}}"}]}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.SendMessage(account, q.QueueName, []byte(`{"other":1}`), false, nil, "", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SendMessage(account, q.QueueName, []byte(`{"flag":true}`), false, nil, "", nil); err != nil {
		t.Fatal(err)
	}
	invoked := 0
	if err := st.PollEventSourceMappingOnce(m.UUID, func(_, _, _, _ string) (string, error) {
		invoked++
		return "", nil
	}); err != nil {
		t.Fatal(err)
	}
	if invoked != 1 {
		t.Fatalf("exists filter invoked=%d", invoked)
	}

	empty := "{}"
	updated, err := st.UpdateEventSourceMapping(store.UpdateEventSourceMappingInput{
		AccountID: account, UUID: m.UUID, FilterCriteriaJSON: &empty,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.FilterCriteriaJSON != "" {
		t.Fatalf("clear FilterCriteria want empty got %q", updated.FilterCriteriaJSON)
	}
	if _, err := st.SendMessage(account, q.QueueName, []byte(`{"any":1}`), false, nil, "", nil); err != nil {
		t.Fatal(err)
	}
	invoked = 0
	if err := st.PollEventSourceMappingOnce(m.UUID, func(_, _, _, _ string) (string, error) {
		invoked++
		return "", nil
	}); err != nil {
		t.Fatal(err)
	}
	if invoked != 1 {
		t.Fatalf("after clear invoked=%d", invoked)
	}
}

func TestESMFilterCriteriaDynamoDBNewImage(t *testing.T) {
	st := openLambdaStore(t)
	account := "000000000001"
	if err := st.EnsureLambdaESMSchema(); err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureDynamoDBStreamsSchema(); err != nil {
		t.Fatal(err)
	}
	trust := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	roleARN, err := st.CreateRole(account, "lambda", trust)
	if err != nil {
		t.Fatal(err)
	}
	allowDoc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"dynamodb:GetRecords","Resource":"*"}]}`
	if err := st.PutInlinePolicy(roleARN, "esm-ddb-filt", allowDoc); err != nil {
		t.Fatal(err)
	}
	zip := testZip(t, map[string]string{"app.py": "def handler(e,c): return e"})
	fn, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID: account, Region: "us-east-1", FunctionName: "ddb-filt-fn",
		Runtime: store.LambdaRuntimePython312, RoleARN: roleARN,
		Handler: "app.handler", Zip: zip,
	})
	if err != nil {
		t.Fatal(err)
	}
	table, err := st.CreateTableWithGSIs(account, "us-east-1", "ddb-filt-tbl", "pk", "S", "", "", store.SSETypeAWSOwned, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	table, err = st.UpdateTableStreamSpec(account, table.TableName, true, store.StreamViewNewImage)
	if err != nil {
		t.Fatal(err)
	}
	keysKeep, _ := store.DynamoStreamKeysJSON(table, "keep", "")
	keysDrop, _ := store.DynamoStreamKeysJSON(table, "drop", "")
	keepItem, _ := json.Marshal(map[string]any{"pk": map[string]string{"S": "keep"}, "status": map[string]string{"S": "ok"}})
	dropItem, _ := json.Marshal(map[string]any{"pk": map[string]string{"S": "drop"}, "status": map[string]string{"S": "no"}})
	if err := st.AppendDynamoStreamRecord(account, table.TableName, "INSERT", keysDrop, dropItem); err != nil {
		t.Fatal(err)
	}
	if err := st.AppendDynamoStreamRecord(account, table.TableName, "INSERT", keysKeep, keepItem); err != nil {
		t.Fatal(err)
	}
	fc := `{"Filters":[{"Pattern":"{\"eventName\":[\"INSERT\"],\"dynamodb\":{\"NewImage\":{\"status\":{\"S\":[\"ok\"]}}}}"}]}`
	m, err := st.CreateEventSourceMapping(store.CreateEventSourceMappingInput{
		AccountID: account, FunctionName: fn.FunctionName, EventSourceARN: table.StreamARN("us-east-1"),
		FilterCriteriaJSON: fc, BatchSize: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	invoked := 0
	if err := st.PollEventSourceMappingOnce(m.UUID, func(_, _, _, eventJSON string) (string, error) {
		invoked++
		if !strings.Contains(eventJSON, `"S":"ok"`) || strings.Contains(eventJSON, `"S":"no"`) {
			t.Fatalf("event=%s", eventJSON)
		}
		return "", nil
	}); err != nil {
		t.Fatal(err)
	}
	if invoked != 1 {
		t.Fatalf("invoked=%d", invoked)
	}
}
