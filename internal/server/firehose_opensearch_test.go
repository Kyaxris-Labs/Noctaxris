package server_test

import (
	"encoding/base64"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestFirehoseOpenSearchPutRecord(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	domain := "fh-srv-os"
	if _, err := st.CreateOpenSearchDomain(testAccountID, "us-east-1", domain, "OpenSearch_2.11"); err != nil {
		t.Fatal(err)
	}
	endpoint := store.OpenSearchNestedEndpoint(domain)
	if err := st.SetOpenSearchContainerID(testAccountID, domain, "ctr-srv", store.OpenSearchDomainStatusActive, endpoint, ""); err != nil {
		t.Fatal(err)
	}

	trust := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"firehose.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	roleARN, err := st.CreateRole(testAccountID, "fh-srv-os-role", trust)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutInlinePolicy(roleARN, "os-put", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"es:ESHttpPut","Resource":"*"}]}`); err != nil {
		t.Fatal(err)
	}

	var calls atomic.Int32
	st.SetFirehoseOpenSearchIndexer(func(accountID string, stream store.FirehoseStream, recordID string, data []byte) error {
		calls.Add(1)
		if stream.DestOpenSearchDomain != domain || stream.DestOpenSearchIndex != "events" {
			t.Fatalf("dest=%q/%q", stream.DestOpenSearchDomain, stream.DestOpenSearchIndex)
		}
		if string(data) != `{"k":1}` {
			t.Fatalf("data=%q", data)
		}
		return nil
	})
	t.Cleanup(func() { st.SetFirehoseOpenSearchIndexer(nil) })

	create := mustJSONTarget(t, handler, "Firehose_20150804.CreateDeliveryStream", "firehose", map[string]any{
		"DeliveryStreamName": "lab-fh-os",
		"OpenSearchDestinationConfiguration": map[string]any{
			"DomainName": domain,
			"IndexName":  "events",
			"RoleARN":    roleARN,
		},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateDeliveryStream status=%d body=%q", create.Code, create.Body.String())
	}

	put := mustJSONTarget(t, handler, "Firehose_20150804.PutRecord", "firehose", map[string]any{
		"DeliveryStreamName": "lab-fh-os",
		"Record":             map[string]any{"Data": base64.StdEncoding.EncodeToString([]byte(`{"k":1}`))},
	}, now)
	if put.Code != http.StatusOK {
		t.Fatalf("PutRecord status=%d body=%q", put.Code, put.Body.String())
	}
	if calls.Load() != 1 {
		t.Fatalf("indexer calls=%d want 1", calls.Load())
	}
}

func TestFirehoseOpenSearchCreateRejectsCreatingDomain(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	domain := "fh-srv-os-creating"
	if _, err := st.CreateOpenSearchDomain(testAccountID, "us-east-1", domain, ""); err != nil {
		t.Fatal(err)
	}

	create := mustJSONTarget(t, handler, "Firehose_20150804.CreateDeliveryStream", "firehose", map[string]any{
		"DeliveryStreamName": "lab-fh-os-bad",
		"AmazonopensearchserviceDestinationConfiguration": map[string]any{
			"DomainARN": store.OpenSearchDomainARN("us-east-1", testAccountID, domain),
			"IndexName": "events",
		},
	}, now)
	if create.Code != http.StatusBadRequest {
		t.Fatalf("CreateDeliveryStream status=%d body=%q want 400", create.Code, create.Body.String())
	}
}
