package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestAthenaQueryRoundTrip(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createDB := mustJSONTarget(t, handler, "AWSGlue.CreateDatabase", "glue", map[string]any{
		"DatabaseInput": map[string]any{"Name": "athdb"},
	}, now)
	if createDB.Code != http.StatusOK {
		t.Fatalf("CreateDatabase status=%d body=%q", createDB.Code, createDB.Body.String())
	}

	if _, err := st.CreateBucket(testAccountID, "ath-lab"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutObject(testAccountID, "ath-lab", "p/data.csv", store.PutObjectMeta{
		Data: []byte("id\na\nb\n"), PlainSize: 8, ContentType: "text/csv",
	}); err != nil {
		t.Fatal(err)
	}

	createTbl := mustJSONTarget(t, handler, "AWSGlue.CreateTable", "glue", map[string]any{
		"DatabaseName": "athdb",
		"TableInput": map[string]any{
			"Name": "t1",
			"StorageDescriptor": map[string]any{
				"Location": "s3://ath-lab/p/",
				"Columns":  []map[string]any{{"Name": "id", "Type": "string"}},
				"SerdeInfo": map[string]any{
					"SerializationLibrary": "org.apache.hadoop.hive.serde2.lazy.LazySimpleSerDe",
					"Parameters":           map[string]string{"field.delim": ","},
				},
			},
			"PartitionKeys": []map[string]any{{"Name": "dt", "Type": "string"}},
		},
	}, now)
	if createTbl.Code != http.StatusOK {
		t.Fatalf("CreateTable status=%d body=%q", createTbl.Code, createTbl.Body.String())
	}

	getTbl := mustJSONTarget(t, handler, "AWSGlue.GetTable", "glue", map[string]any{
		"DatabaseName": "athdb", "Name": "t1",
	}, now)
	if getTbl.Code != http.StatusOK || !strings.Contains(getTbl.Body.String(), "PartitionKeys") {
		t.Fatalf("GetTable status=%d body=%q", getTbl.Code, getTbl.Body.String())
	}

	start := mustJSONTarget(t, handler, "AmazonAthena.StartQueryExecution", "athena", map[string]any{
		"QueryString": "SELECT * FROM athdb.t1 LIMIT 5",
		"QueryExecutionContext": map[string]any{
			"Database": "athdb",
		},
	}, now)
	if start.Code != http.StatusOK {
		t.Fatalf("StartQueryExecution status=%d body=%q", start.Code, start.Body.String())
	}
	var startBody map[string]any
	if err := json.Unmarshal(start.Body.Bytes(), &startBody); err != nil {
		t.Fatal(err)
	}
	qid, _ := startBody["QueryExecutionId"].(string)
	if qid == "" {
		t.Fatalf("missing QueryExecutionId: %s", start.Body.String())
	}

	get := mustJSONTarget(t, handler, "AmazonAthena.GetQueryExecution", "athena", map[string]any{
		"QueryExecutionId": qid,
	}, now)
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), "SUCCEEDED") {
		t.Fatalf("GetQueryExecution status=%d body=%q", get.Code, get.Body.String())
	}

	results := mustJSONTarget(t, handler, "AmazonAthena.GetQueryResults", "athena", map[string]any{
		"QueryExecutionId": qid,
	}, now)
	if results.Code != http.StatusOK {
		t.Fatalf("GetQueryResults status=%d body=%q", results.Code, results.Body.String())
	}
}

func TestAthenaMissingBucketFails(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createDB := mustJSONTarget(t, handler, "AWSGlue.CreateDatabase", "glue", map[string]any{
		"DatabaseInput": map[string]any{"Name": "missdb"},
	}, now)
	if createDB.Code != http.StatusOK {
		t.Fatalf("CreateDatabase status=%d body=%q", createDB.Code, createDB.Body.String())
	}
	createTbl := mustJSONTarget(t, handler, "AWSGlue.CreateTable", "glue", map[string]any{
		"DatabaseName": "missdb",
		"TableInput": map[string]any{
			"Name": "t1",
			"StorageDescriptor": map[string]any{
				"Location": "s3://missing-bucket/p/",
				"Columns":  []map[string]any{{"Name": "id", "Type": "string"}},
			},
		},
	}, now)
	if createTbl.Code != http.StatusOK {
		t.Fatalf("CreateTable status=%d body=%q", createTbl.Code, createTbl.Body.String())
	}
	start := mustJSONTarget(t, handler, "AmazonAthena.StartQueryExecution", "athena", map[string]any{
		"QueryString":           "SELECT * FROM missdb.t1",
		"QueryExecutionContext": map[string]any{"Database": "missdb"},
	}, now)
	if start.Code != http.StatusOK {
		t.Fatalf("StartQueryExecution status=%d body=%q", start.Code, start.Body.String())
	}
	var startBody map[string]any
	if err := json.Unmarshal(start.Body.Bytes(), &startBody); err != nil {
		t.Fatal(err)
	}
	qid, _ := startBody["QueryExecutionId"].(string)
	get := mustJSONTarget(t, handler, "AmazonAthena.GetQueryExecution", "athena", map[string]any{
		"QueryExecutionId": qid,
	}, now)
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), "FAILED") {
		t.Fatalf("want FAILED for missing bucket, status=%d body=%q", get.Code, get.Body.String())
	}
}

func TestOpenSearchDomainHTTP(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustJSONTarget(t, handler, "AmazonOpenSearchService.CreateDomain", "es", map[string]any{
		"DomainName":    "srv-domain",
		"EngineVersion": "OpenSearch_2.11",
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateDomain status=%d body=%q", create.Code, create.Body.String())
	}

	desc := mustJSONTarget(t, handler, "AmazonOpenSearchService.DescribeDomain", "es", map[string]any{
		"DomainName": "srv-domain",
	}, now)
	body := desc.Body.String()
	if desc.Code != http.StatusOK || !strings.Contains(body, "stub://127.0.0.1/opensearch/") {
		t.Fatalf("DescribeDomain status=%d body=%q", desc.Code, desc.Body.String())
	}
	if !strings.Contains(body, `"DomainStatus":"CreateFailed"`) && !strings.Contains(body, `"DomainStatus": "CreateFailed"`) {
		t.Fatalf("DescribeDomain must not claim Active without engine; body=%q", body)
	}
	if strings.Contains(body, `"DomainStatus":"Active"`) || strings.Contains(body, `"DomainStatus": "Active"`) {
		t.Fatalf("DescribeDomain must not claim Active on stub://; body=%q", body)
	}
	if strings.Contains(body, `"Created":true`) || strings.Contains(body, `"Created": true`) {
		t.Fatalf("CreateFailed stub must not set Created=true; body=%q", body)
	}
	if !strings.Contains(body, `"FailureReason"`) {
		t.Fatalf("CreateFailed without DinD should surface FailureReason hint; body=%q", body)
	}

	list := mustJSONTarget(t, handler, "AmazonOpenSearchService.ListDomainNames", "es", map[string]any{}, now)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "srv-domain") {
		t.Fatalf("ListDomainNames status=%d body=%q", list.Code, list.Body.String())
	}

	del := mustJSONTarget(t, handler, "AmazonOpenSearchService.DeleteDomain", "es", map[string]any{
		"DomainName": "srv-domain",
	}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteDomain status=%d body=%q", del.Code, del.Body.String())
	}
}
