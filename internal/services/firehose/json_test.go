package firehose_test

import (
	"encoding/json"
	"strings"
	"testing"

	fhsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/firehose"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestDescribeDeliveryStreamJSONOpenSearch(t *testing.T) {
	st := store.FirehoseStream{
		Name:                 "lab-os",
		StreamARN:            store.FirehoseStreamARN("us-east-1", "000000000001", "lab-os"),
		DestType:             "OpenSearch",
		DestOpenSearchDomain: "events-domain",
		DestOpenSearchIndex:  "events",
		RoleARN:              "arn:aws:iam::000000000001:role/fh-os",
		CreatedAt:            1_700_000_000_000,
	}
	raw, err := fhsvc.DescribeDeliveryStreamJSON(st)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	desc, _ := body["DeliveryStreamDescription"].(map[string]any)
	dests, _ := desc["Destinations"].([]any)
	if len(dests) != 1 {
		t.Fatalf("Destinations len=%d body=%s", len(dests), raw)
	}
	dest, _ := dests[0].(map[string]any)
	for _, key := range []string{
		"AmazonopensearchserviceDestinationDescription",
		"OpenSearchDestinationDescription",
	} {
		osDesc, ok := dest[key].(map[string]any)
		if !ok {
			t.Fatalf("missing %s in %s", key, raw)
		}
		domainARN, _ := osDesc["DomainARN"].(string)
		wantARN := store.OpenSearchDomainARN("us-east-1", "000000000001", "events-domain")
		if domainARN != wantARN {
			t.Fatalf("%s DomainARN=%q want %q", key, domainARN, wantARN)
		}
		if osDesc["IndexName"] != "events" {
			t.Fatalf("%s IndexName=%v", key, osDesc["IndexName"])
		}
		if osDesc["RoleARN"] != st.RoleARN {
			t.Fatalf("%s RoleARN=%v", key, osDesc["RoleARN"])
		}
	}
	if strings.Contains(string(raw), "S3DestinationDescription") {
		t.Fatalf("unexpected S3 dest in OpenSearch describe: %s", raw)
	}
}

func TestFirehoseAllDestTypes(t *testing.T) {
	s3st := store.FirehoseStream{
		Name: "s3", StreamARN: store.FirehoseStreamARN("us-east-1", "000000000001", "s3"),
		DestType: "S3", DestBucket: "b", DestPrefix: "p/", RoleARN: "arn:role", CreatedAt: 1_700_000_000_000,
	}
	raw, err := fhsvc.DescribeDeliveryStreamJSON(s3st)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "S3DestinationDescription") {
		t.Fatalf("s3=%s", raw)
	}

	lam := s3st
	lam.Name, lam.DestType, lam.DestLambdaARN = "lam", "Lambda", "arn:aws:lambda:1:1:function:f"
	raw, _ = fhsvc.DescribeDeliveryStreamJSON(lam)

	vpc := s3st
	vpc.Name, vpc.DestType = "vpc", "VPCFlow"
	raw, _ = fhsvc.DescribeDeliveryStreamJSON(vpc)

	raw, _ = fhsvc.CreateDeliveryStreamJSON(s3st)
	raw, _ = fhsvc.ListDeliveryStreamsJSON([]store.FirehoseStream{s3st})
	_, _ = fhsvc.ListDeliveryStreamsJSON(nil)
	if _, err := fhsvc.DeleteDeliveryStreamJSON(); err != nil {
		t.Fatal(err)
	}
	raw, _ = fhsvc.PutRecordJSON("rec-1")
	raw, _ = fhsvc.PutRecordBatchJSON([]int{0, 2}, 3)
	if !strings.Contains(string(raw), "FailedPutCount") {
		t.Fatalf("batch=%s", raw)
	}
}
