package store_test

import (
	"encoding/csv"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestRecordAndGetAccessKeyLastUsed(t *testing.T) {
	st := openTestStore(t)
	const account = "000000000099"
	if err := st.EnsureRoot(account, "AKIAROOTEXAMPLE99", "secret"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.CreateUser(account, "forensics-user"); err != nil {
		t.Fatal(err)
	}
	akid, _, err := st.CreateUserAccessKey(account, "forensics-user")
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	if err := st.RecordAccessKeyLastUsed(akid, "s3", "us-east-1", at); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetAccessKeyLastUsed(account, akid)
	if err != nil {
		t.Fatal(err)
	}
	if got.UserName != "forensics-user" {
		t.Fatalf("UserName=%q", got.UserName)
	}
	if !got.HasLastUsed || !got.LastUsedDate.Equal(at) {
		t.Fatalf("LastUsed=%v has=%v", got.LastUsedDate, got.HasLastUsed)
	}
	if got.ServiceName != "s3" || got.Region != "us-east-1" {
		t.Fatalf("service=%q region=%q", got.ServiceName, got.Region)
	}
}

func TestCredentialReportGenerateAndGet(t *testing.T) {
	st := openTestStore(t)
	const account = "000000000098"
	if err := st.EnsureRoot(account, "AKIAROOTEXAMPLE98", "secret"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.CreateUser(account, "report-user"); err != nil {
		t.Fatal(err)
	}
	_, _, _, err := st.GetCredentialReport(account)
	if err == nil || err != store.ErrCredentialReportNotPresent {
		t.Fatalf("GetCredentialReport before generate: err=%v", err)
	}
	genAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := st.GenerateCredentialReport(account, genAt); err != nil {
		t.Fatal(err)
	}
	csvBytes, gotAt, state, err := st.GetCredentialReport(account)
	if err != nil {
		t.Fatal(err)
	}
	if state != "COMPLETE" {
		t.Fatalf("state=%q", state)
	}
	if !gotAt.Equal(genAt) {
		t.Fatalf("generated=%v want %v", gotAt, genAt)
	}
	rows, err := csv.NewReader(strings.NewReader(string(csvBytes))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) < 2 {
		t.Fatalf("rows=%d", len(rows))
	}
	if rows[1][0] != "report-user" {
		t.Fatalf("user column=%q", rows[1][0])
	}
}
