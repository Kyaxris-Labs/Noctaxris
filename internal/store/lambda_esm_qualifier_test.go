package store_test

import (
	"archive/zip"
	"bytes"
	"path/filepath"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestESMPersistsAndPollsQualifier(t *testing.T) {
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
	account := "000000000001"
	if err := st.EnsureRoot(account, "AKIAROOTEXAMPLE01", "secret-root-value"); err != nil {
		t.Fatal(err)
	}
	roleARN, err := st.CreateRole(account, "esm-qual", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	w, err := zw.Create("app.py")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("def handler(event, context):\n    return event\n")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	fn, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID: account, Region: "us-east-1", FunctionName: "qual-fn",
		RoleARN: roleARN, Runtime: "python3.12", Handler: "app.handler",
		Timeout: 3, Memory: 128, Zip: zipBuf.Bytes(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.PublishVersion(account, fn.FunctionName); err != nil {
		t.Fatal(err)
	}
	q, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "qual-q", nil)
	if err != nil {
		t.Fatal(err)
	}
	allowLambdaESMQueuePolicy(t, st, account, q.QueueName)
	m, err := st.CreateEventSourceMapping(store.CreateEventSourceMappingInput{
		AccountID: account, FunctionName: fn.FunctionName + ":1", EventSourceARN: q.QueueARN,
	})
	if err != nil {
		t.Fatal(err)
	}
	if m.Qualifier != "1" {
		t.Fatalf("Qualifier=%q want 1", m.Qualifier)
	}
	got, err := st.GetEventSourceMapping(account, m.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Qualifier != "1" {
		t.Fatalf("persisted Qualifier=%q", got.Qualifier)
	}
	if _, err := st.SendMessage(account, q.QueueName, []byte(`{"ping":1}`), false, nil, "", nil); err != nil {
		t.Fatal(err)
	}
	var sawQualifier string
	if err := st.PollEventSourceMappingOnce(m.UUID, func(_, _, qualifier, _ string) (string, error) {
		sawQualifier = qualifier
		return "", nil
	}); err != nil {
		t.Fatal(err)
	}
	if sawQualifier != "1" {
		t.Fatalf("poll qualifier=%q want 1", sawQualifier)
	}
}
