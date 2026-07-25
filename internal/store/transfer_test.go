package store_test

import (
	"errors"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestTransferCreateServerOnline(t *testing.T) {
	st := openStreamCStore(t)
	account := "000000000001"
	sv, err := st.CreateTransferServer(account, "us-east-1", []string{"SFTP"})
	if err != nil {
		t.Fatal(err)
	}
	if sv.State != "ONLINE" {
		t.Fatalf("State=%q want ONLINE", sv.State)
	}
	got, err := st.DescribeTransferServer(account, sv.ServerID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != "ONLINE" {
		t.Fatalf("describe State=%q want ONLINE", got.State)
	}
}

func TestTransferFilePutGetRoundTrip(t *testing.T) {
	st := openStreamCStore(t)
	account := "000000000001"
	sv, err := st.CreateTransferServer(account, "us-east-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateTransferUser(account, sv.ServerID, "alice", "/alice", ""); err != nil {
		t.Fatal(err)
	}
	want := []byte("lab-transfer-payload")
	if err := st.TransferPutFile(account, sv.ServerID, "alice", "inbox/hello.txt", want); err != nil {
		t.Fatal(err)
	}
	got, err := st.TransferGetFile(account, sv.ServerID, "alice", "inbox/hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("got %q want %q", got, want)
	}
	entries, err := st.TransferListDirectory(account, sv.ServerID, "alice", "inbox")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name != "hello.txt" || entries[0].IsDir {
		t.Fatalf("entries=%+v", entries)
	}
}

func TestTransferPathTraversalRejected(t *testing.T) {
	st := openStreamCStore(t)
	account := "000000000001"
	sv, err := st.CreateTransferServer(account, "us-east-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateTransferUser(account, sv.ServerID, "alice", "/alice", ""); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"../secret", "foo/../../etc/passwd", ".."} {
		err := st.TransferPutFile(account, sv.ServerID, "alice", rel, []byte("x"))
		if !errors.Is(err, store.ErrTransferPathEscape) {
			t.Fatalf("put path %q: err=%v want ErrTransferPathEscape", rel, err)
		}
	}
	_, err = st.TransferGetFile(account, sv.ServerID, "alice", "../secret")
	if !errors.Is(err, store.ErrTransferPathEscape) {
		t.Fatalf("get ../secret: err=%v want ErrTransferPathEscape", err)
	}
}
