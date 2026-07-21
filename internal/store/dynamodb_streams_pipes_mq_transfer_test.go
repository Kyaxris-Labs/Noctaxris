package store_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func openStreamCStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	key, err := store.LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestDynamoDBStreamsLite(t *testing.T) {
	st := openStreamCStore(t)
	account := "000000000001"
	table, err := st.CreateTableWithGSIs(account, "us-east-1", "stream-tbl", "pk", "S", "", "", store.SSETypeAWSOwned, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	table, err = st.UpdateTableStreamSpec(account, table.TableName, true, store.StreamViewNewImage)
	if err != nil || !table.StreamEnabled || table.StreamLabel == "" {
		t.Fatalf("enable stream: %+v err=%v", table, err)
	}
	keys, _ := store.DynamoStreamKeysJSON(table, "1", "")
	item, _ := json.Marshal(map[string]any{"pk": map[string]string{"S": "1"}, "v": map[string]string{"S": "a"}})
	if err := st.PutItemBytes(account, table.TableName, "1", "", "", "", item, false, nil); err != nil {
		t.Fatal(err)
	}
	if err := st.AppendDynamoStreamRecord(account, table.TableName, "INSERT", keys, item); err != nil {
		t.Fatal(err)
	}
	streams, err := st.ListDynamoStreams(account, table.TableName)
	if err != nil || len(streams) != 1 {
		t.Fatalf("list=%v err=%v", streams, err)
	}
	arn := table.StreamARN("us-east-1")
	acct, name, label, ok := store.ParseDynamoStreamARN(arn)
	if !ok || acct != account || name != table.TableName || label != table.StreamLabel {
		t.Fatalf("parse arn=%q", arn)
	}
	it, err := st.GetDynamoStreamShardIterator(account, table.TableName, store.LabDynamoStreamShardID, "TRIM_HORIZON", "")
	if err != nil {
		t.Fatal(err)
	}
	recs, next, err := st.GetDynamoStreamRecords(it, 10)
	if err != nil || len(recs) != 1 || next == "" {
		t.Fatalf("records=%v next=%q err=%v", recs, next, err)
	}
	if recs[0].EventName != "INSERT" || recs[0].NewImageJSON == "" {
		t.Fatalf("record=%+v", recs[0])
	}
}

func TestPipesSQSToSQS(t *testing.T) {
	st := openStreamCStore(t)
	account := "000000000001"
	src, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "pipe-src", nil)
	if err != nil {
		t.Fatal(err)
	}
	dst, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "pipe-dst", nil)
	if err != nil {
		t.Fatal(err)
	}
	dstPolicy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"pipes.amazonaws.com"},"Action":"sqs:SendMessage","Resource":"` + dst.QueueARN + `"}]}`
	if err := st.SetQueueAttributes(account, dst.QueueName, map[string]string{"Policy": dstPolicy}); err != nil {
		t.Fatal(err)
	}
	srcPolicy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"pipes.amazonaws.com"},"Action":["sqs:ReceiveMessage","sqs:DeleteMessage"],"Resource":"` + src.QueueARN + `"}]}`
	if err := st.SetQueueAttributes(account, src.QueueName, map[string]string{"Policy": srcPolicy}); err != nil {
		t.Fatal(err)
	}
	p, err := st.CreatePipe(account, "us-east-1", "lab-pipe", "", src.QueueARN, dst.QueueARN, "", "RUNNING")
	if err != nil || p.ARN == "" {
		t.Fatalf("create pipe: %+v err=%v", p, err)
	}
	if _, err := st.SendMessage(account, src.QueueName, []byte(`{"x":1}`), false, nil, "", nil); err != nil {
		t.Fatal(err)
	}
	if err := st.PollPipeOnce(account, p.Name, nil); err != nil {
		t.Fatal(err)
	}
	got, err := st.ReceiveMessages(account, dst.QueueName, 10)
	if err != nil || len(got) != 1 {
		t.Fatalf("dst msgs=%v err=%v", got, err)
	}
	listed, err := st.ListPipes(account)
	if err != nil || len(listed) != 1 {
		t.Fatalf("list=%v err=%v", listed, err)
	}
	if err := st.DeletePipe(account, p.Name); err != nil {
		t.Fatal(err)
	}
}

func TestPipesDeliveryDeniedWithoutTargetPolicy(t *testing.T) {
	st := openStreamCStore(t)
	account := "000000000001"
	src, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "pipe-deny-src", nil)
	if err != nil {
		t.Fatal(err)
	}
	dst, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "pipe-deny-dst", nil)
	if err != nil {
		t.Fatal(err)
	}
	p, err := st.CreatePipe(account, "us-east-1", "deny-pipe", "", src.QueueARN, dst.QueueARN, "", "RUNNING")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.SendMessage(account, src.QueueName, []byte(`{"x":1}`), false, nil, "", nil); err != nil {
		t.Fatal(err)
	}
	if err := st.PollPipeOnce(account, p.Name, nil); err == nil {
		t.Fatal("expected delivery deny without RoleArn or target policy")
	}
	got, err := st.ReceiveMessages(account, dst.QueueName, 10)
	if err != nil || len(got) != 0 {
		t.Fatalf("dst msgs=%v err=%v want none", got, err)
	}
}

func TestMQBrokerStubCRUD(t *testing.T) {
	st := openStreamCStore(t)
	account := "000000000001"
	b, err := st.CreateMQBroker(account, "us-east-1", "lab-broker", "ACTIVEMQ", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if b.BrokerID == "" || b.StubEndpoint == "" || !stringsHasPrefix(b.StubEndpoint, "stub://127.0.0.1/") {
		t.Fatalf("broker=%+v", b)
	}
	got, err := st.DescribeMQBroker(account, b.BrokerID)
	if err != nil || got.BrokerName != "lab-broker" {
		t.Fatalf("describe=%+v err=%v", got, err)
	}
	list, err := st.ListMQBrokers(account)
	if err != nil || len(list) != 1 {
		t.Fatalf("list=%v err=%v", list, err)
	}
	if err := st.DeleteMQBroker(account, b.BrokerID); err != nil {
		t.Fatal(err)
	}
}

func TestTransferServerUserSandbox(t *testing.T) {
	st := openStreamCStore(t)
	account := "000000000001"
	sv, err := st.CreateTransferServer(account, "us-east-1", []string{"SFTP"})
	if err != nil {
		t.Fatal(err)
	}
	if sv.EndpointType != "VPC" || sv.State != "OFFLINE" {
		t.Fatalf("want VPC/OFFLINE without listener, got EndpointType=%q State=%q", sv.EndpointType, sv.State)
	}
	u, err := st.CreateTransferUser(account, sv.ServerID, "alice", "/alice", "")
	if err != nil || u.UserName != "alice" {
		t.Fatalf("user=%+v err=%v", u, err)
	}
	home, err := st.TransferUserHomePath(account, sv.ServerID, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(home); err != nil {
		t.Fatalf("home missing: %v", err)
	}
	marker := filepath.Join(home, "note.txt")
	if err := os.WriteFile(marker, []byte("hi"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteTransferUser(account, sv.ServerID, "alice"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Fatalf("home should be removed, err=%v", err)
	}
	if err := st.DeleteTransferServer(account, sv.ServerID); err != nil {
		t.Fatal(err)
	}
}

func stringsHasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
