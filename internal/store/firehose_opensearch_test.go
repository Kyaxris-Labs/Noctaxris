package store_test

import (
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestFirehoseOpenSearchPutCallsIndexer(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	domain := "fh-os-lab"
	if _, err := st.CreateOpenSearchDomain(account, "us-east-1", domain, "OpenSearch_2.11"); err != nil {
		t.Fatal(err)
	}
	endpoint := store.OpenSearchNestedEndpoint(domain)
	if err := st.SetOpenSearchContainerID(account, domain, "ctr-fh-os", store.OpenSearchDomainStatusActive, endpoint, ""); err != nil {
		t.Fatal(err)
	}

	trust := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"firehose.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	roleARN, err := st.CreateRole(account, "fh-os-role", trust)
	if err != nil {
		t.Fatal(err)
	}
	allow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"es:ESHttpPut","Resource":"*"}]}`
	if err := st.PutInlinePolicy(roleARN, "os-put", allow); err != nil {
		t.Fatal(err)
	}

	var calls atomic.Int32
	var gotIndex, gotDomain string
	var gotData []byte
	st.SetFirehoseOpenSearchIndexer(func(accountID string, stream store.FirehoseStream, recordID string, data []byte) error {
		calls.Add(1)
		gotDomain = stream.DestOpenSearchDomain
		gotIndex = stream.DestOpenSearchIndex
		gotData = append([]byte(nil), data...)
		if accountID != account || recordID == "" {
			t.Fatalf("unexpected indexer args account=%q recordID=%q", accountID, recordID)
		}
		return nil
	})
	t.Cleanup(func() { st.SetFirehoseOpenSearchIndexer(nil) })

	_, err = st.CreateFirehoseOpenSearchStream(account, "us-east-1", "fh-os-stream", roleARN, domain, "events")
	if err != nil {
		t.Fatal(err)
	}
	id, err := st.PutFirehoseRecord(account, "fh-os-stream", []byte(`{"hello":"os"}`))
	if err != nil || id == "" {
		t.Fatalf("put: %v %s", err, id)
	}
	if calls.Load() != 1 {
		t.Fatalf("indexer calls=%d want 1", calls.Load())
	}
	if gotDomain != domain || gotIndex != "events" {
		t.Fatalf("dest domain=%q index=%q", gotDomain, gotIndex)
	}
	if string(gotData) != `{"hello":"os"}` {
		t.Fatalf("data=%q", gotData)
	}
}

func TestFirehoseOpenSearchCreateRejectsNonActive(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	if _, err := st.CreateOpenSearchDomain(account, "us-east-1", "fh-os-creating", ""); err != nil {
		t.Fatal(err)
	}
	_, err := st.CreateFirehoseOpenSearchStream(account, "us-east-1", "bad-stream", "", "fh-os-creating", "idx")
	if !errors.Is(err, store.ErrFirehoseBadReq) {
		t.Fatalf("err=%v want ErrFirehoseBadReq", err)
	}
	if !strings.Contains(err.Error(), "Active") && !strings.Contains(err.Error(), "OpenSearch") {
		t.Fatalf("err=%v want Active/OpenSearch hint", err)
	}
}

func TestFirehoseOpenSearchCreateRejectsStubEndpoint(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	domain := "fh-os-stub"
	if _, err := st.CreateOpenSearchDomain(account, "us-east-1", domain, ""); err != nil {
		t.Fatal(err)
	}
	if err := st.SetOpenSearchContainerID(account, domain, "", store.OpenSearchDomainStatusCreateFailed, "", "fail"); err != nil {
		t.Fatal(err)
	}
	_, err := st.CreateFirehoseOpenSearchStream(account, "us-east-1", "stub-stream", "", domain, "idx")
	if !errors.Is(err, store.ErrFirehoseBadReq) {
		t.Fatalf("err=%v want ErrFirehoseBadReq", err)
	}
}

func TestFirehoseOpenSearchPutRequiresRoleARN(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	domain := "fh-os-norole"
	if _, err := st.CreateOpenSearchDomain(account, "us-east-1", domain, ""); err != nil {
		t.Fatal(err)
	}
	endpoint := store.OpenSearchNestedEndpoint(domain)
	if err := st.SetOpenSearchContainerID(account, domain, "ctr", store.OpenSearchDomainStatusActive, endpoint, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateFirehoseOpenSearchStream(account, "us-east-1", "norole-stream", "", domain, "idx"); err != nil {
		t.Fatal(err)
	}
	st.SetFirehoseOpenSearchIndexer(func(string, store.FirehoseStream, string, []byte) error {
		t.Fatal("indexer must not run when unauthorized")
		return nil
	})
	t.Cleanup(func() { st.SetFirehoseOpenSearchIndexer(nil) })
	if _, err := st.PutFirehoseRecord(account, "norole-stream", []byte("x")); err == nil {
		t.Fatal("expected Put denied without RoleARN (no OpenSearch resource policy)")
	}
}

func TestFirehoseOpenSearchDefaultIndexerAllowlistsHost(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	domain := "fh-os-default"
	if _, err := st.CreateOpenSearchDomain(account, "us-east-1", domain, ""); err != nil {
		t.Fatal(err)
	}
	endpoint := store.OpenSearchNestedEndpoint(domain)
	if err := st.SetOpenSearchContainerID(account, domain, "ctr", store.OpenSearchDomainStatusActive, endpoint, ""); err != nil {
		t.Fatal(err)
	}
	trust := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"firehose.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	roleARN, err := st.CreateRole(account, "fh-os-default-role", trust)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutInlinePolicy(roleARN, "os-put", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"es:ESHttpPut","Resource":"*"}]}`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateFirehoseOpenSearchStream(account, "us-east-1", "default-stream", roleARN, domain, "idx"); err != nil {
		t.Fatal(err)
	}
	// After create, point the domain at a non-nested host. Default indexer must refuse (no WAN dial).
	if err := st.SetOpenSearchContainerID(account, domain, "ctr", store.OpenSearchDomainStatusActive, "evil.example:9200", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutFirehoseRecord(account, "default-stream", []byte(`{}`)); err == nil {
		t.Fatal("expected default indexer to refuse non-nested host")
	}
}
