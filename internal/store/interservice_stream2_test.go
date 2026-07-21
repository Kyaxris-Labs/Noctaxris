package store_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func allowSNSSQSPolicy(t *testing.T, st *store.Store, account, queueName string) {
	t.Helper()
	policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"sns.amazonaws.com"},"Action":"sqs:SendMessage","Resource":"*"}]}`
	if err := st.SetQueueAttributes(account, queueName, map[string]string{"Policy": policy}); err != nil {
		t.Fatal(err)
	}
}

func TestDynamoStreamsEventSourceMappingPoll(t *testing.T) {
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
	if err := st.PutInlinePolicy(roleARN, "esm-ddb", allowDoc); err != nil {
		t.Fatal(err)
	}
	zip := testZip(t, map[string]string{"app.py": "def handler(e,c): return e"})
	fn, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID:    account,
		Region:       "us-east-1",
		FunctionName: "ddb-esm-fn",
		Runtime:      store.LambdaRuntimePython312,
		RoleARN:      "arn:aws:iam::" + account + ":role/lambda",
		Handler:      "app.handler",
		Zip:          zip,
	})
	if err != nil {
		t.Fatal(err)
	}
	table, err := st.CreateTableWithGSIs(account, "us-east-1", "ddb-esm-tbl", "pk", "S", "", "", store.SSETypeAWSOwned, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	table, err = st.UpdateTableStreamSpec(account, table.TableName, true, store.StreamViewNewImage)
	if err != nil {
		t.Fatal(err)
	}
	keys, _ := store.DynamoStreamKeysJSON(table, "1", "")
	item, _ := json.Marshal(map[string]any{"pk": map[string]string{"S": "1"}, "v": map[string]string{"S": "hello"}})
	if err := st.AppendDynamoStreamRecord(account, table.TableName, "INSERT", keys, item); err != nil {
		t.Fatal(err)
	}
	streamARN := table.StreamARN("us-east-1")
	m, err := st.CreateEventSourceMapping(store.CreateEventSourceMappingInput{
		AccountID:      account,
		FunctionName:   fn.FunctionName,
		EventSourceARN: streamARN,
		BatchSize:      5,
	})
	if err != nil {
		t.Fatal(err)
	}
	invoked := false
	if err := st.PollEventSourceMappingOnce(m.UUID, func(acct, name, eventJSON string) error {
		invoked = true
		if acct != account || name != fn.FunctionName {
			t.Fatalf("invoke acct=%s name=%s", acct, name)
		}
		if !strings.Contains(eventJSON, `"eventSource":"aws:dynamodb"`) {
			t.Fatalf("event=%s", eventJSON)
		}
		if !strings.Contains(eventJSON, streamARN) || !strings.Contains(eventJSON, "INSERT") {
			t.Fatalf("event=%s", eventJSON)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !invoked {
		t.Fatal("expected DynamoDB Streams ESM invoke")
	}
	// Second poll should not redeliver (cursor advanced).
	invoked = false
	if err := st.PollEventSourceMappingOnce(m.UUID, func(string, string, string) error {
		invoked = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if invoked {
		t.Fatal("expected no redelivery after cursor advance")
	}
}

func TestBudgetSNSNotifyOnCreate(t *testing.T) {
	st := openLambdaStore(t)
	account := "000000000001"
	topic, err := st.CreateTopic(account, "us-east-1", "budget-alerts", nil)
	if err != nil {
		t.Fatal(err)
	}
	q, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "budget-alerts-q", nil)
	if err != nil {
		t.Fatal(err)
	}
	allowSNSSQSPolicy(t, st, account, q.QueueName)
	if _, err := st.Subscribe(account, topic.TopicARN, "sqs", q.QueueARN); err != nil {
		t.Fatal(err)
	}
	notifications := []any{
		map[string]any{
			"Notification": map[string]any{"NotificationType": "ACTUAL", "Threshold": 80.0},
			"Subscribers": []any{
				map[string]any{"SubscriptionType": "SNS", "Address": topic.TopicARN},
			},
		},
	}
	if _, err := st.CreateBudget(account, "lab-budget", "COST", "MONTHLY", "100.0", "USD", notifications); err != nil {
		t.Fatal(err)
	}
	msgs, err := st.ReceiveMessages(account, q.QueueName, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 || !strings.Contains(string(msgs[0].Body), "lab-budget") {
		t.Fatalf("want budget SNS notify, got %+v", msgs)
	}
}

func TestConfigSNSNotifyOnStart(t *testing.T) {
	st := openLambdaStore(t)
	account := "000000000001"
	topic, err := st.CreateTopic(account, "us-east-1", "config-alerts", nil)
	if err != nil {
		t.Fatal(err)
	}
	q, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "config-alerts-q", nil)
	if err != nil {
		t.Fatal(err)
	}
	allowSNSSQSPolicy(t, st, account, q.QueueName)
	if _, err := st.Subscribe(account, topic.TopicARN, "sqs", q.QueueARN); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutConfigRecorder(account, "default", "", "ALL"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutConfigDeliveryChannel(account, "default", "config-bucket", "", topic.TopicARN); err != nil {
		t.Fatal(err)
	}
	if err := st.StartConfigRecorder(account, "default"); err != nil {
		t.Fatal(err)
	}
	st.NotifyConfigDeliveryChannelsSNS(account, "default")
	msgs, err := st.ReceiveMessages(account, q.QueueName, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 || !strings.Contains(string(msgs[0].Body), "ConfigurationHistoryDeliveryStarted") {
		t.Fatalf("want config SNS notify, got %+v", msgs)
	}
}

func TestAthenaMissingBucketFails(t *testing.T) {
	st := openLambdaStore(t)
	account := "000000000001"
	if err := st.EnsureGlueSchema(); err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureAthenaSchema(); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateGlueDatabase(account, "labdb", ""); err != nil {
		t.Fatal(err)
	}
	_, err := st.CreateGlueTable(account, store.GlueTableCreate{
		DatabaseName:    "labdb",
		Name:            "people",
		StorageLocation: "s3://missing-athena-bucket/data/",
		Columns:         []store.GlueColumn{{Name: "id", Type: "string"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	exec, err := st.StartAthenaQueryExecution(account, store.AthenaStartInput{
		QueryString: "SELECT id FROM labdb.people",
		Database:    "labdb",
	})
	if err != nil {
		t.Fatal(err)
	}
	if exec.State != "FAILED" || !strings.Contains(exec.ErrorMessage, "does not exist") {
		t.Fatalf("want FAILED missing bucket, got state=%s err=%q", exec.State, exec.ErrorMessage)
	}
}

func TestSESBounceSNSNotify(t *testing.T) {
	st := openLambdaStore(t)
	account := "000000000001"
	if err := st.EnsureSESSchema(); err != nil {
		t.Fatal(err)
	}
	topic, err := st.CreateTopic(account, "us-east-1", "ses-bounces", nil)
	if err != nil {
		t.Fatal(err)
	}
	q, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "ses-bounces-q", nil)
	if err != nil {
		t.Fatal(err)
	}
	allowSNSSQSPolicy(t, st, account, q.QueueName)
	if _, err := st.Subscribe(account, topic.TopicARN, "sqs", q.QueueARN); err != nil {
		t.Fatal(err)
	}
	if err := st.VerifySESEmailIdentity(account, "lab@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := st.SetSESIdentityNotificationTopic(account, "lab@example.com", "Bounce", topic.TopicARN); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SendSESEmail(account, "lab@example.com", []string{"bounce@lab.invalid"}, "hi", "body", ""); err != nil {
		t.Fatal(err)
	}
	msgs, err := st.ReceiveMessages(account, q.QueueName, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 || !strings.Contains(string(msgs[0].Body), "Bounce") || !strings.Contains(string(msgs[0].Body), "notificationType") {
		t.Fatalf("want SES bounce SNS, got %+v", msgs)
	}
	err = st.SetSESIdentityNotificationTopic(account, "lab@example.com", "Complaint", topic.TopicARN)
	if err == nil || !strings.Contains(err.Error(), "Bounce") {
		t.Fatalf("want Bounce-only error, got %v", err)
	}
}
