package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func mustNeptuneQuery(t *testing.T, handler http.Handler, body string, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(body))
	signHeader(t, req, []byte(body), testAccessKey, testSecret, testRegion, "neptune", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestNeptuneHandlers(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createBody := strings.Join([]string{
		"Action=CreateDBCluster",
		"Version=2014-10-31",
		"DBClusterIdentifier=lab-neptune-1",
		"Engine=neptune",
	}, "&")
	create := mustNeptuneQuery(t, handler, createBody, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateDBCluster status=%d body=%q", create.Code, create.Body.String())
	}
	if !strings.Contains(create.Body.String(), "lab-neptune-1") || !strings.Contains(create.Body.String(), "neptune.noctaxris.internal") {
		t.Fatalf("create body=%q", create.Body.String())
	}
	if !strings.Contains(create.Body.String(), "creating") {
		t.Fatalf("without DinD expect creating, body=%q", create.Body.String())
	}
	if strings.Contains(create.Body.String(), "available") {
		t.Fatalf("must not claim available without nested engine: %q", create.Body.String())
	}

	desc := mustNeptuneQuery(t, handler,
		"Action=DescribeDBClusters&Version=2014-10-31&DBClusterIdentifier=lab-neptune-1", now)
	if desc.Code != http.StatusOK || !strings.Contains(desc.Body.String(), "8182") {
		t.Fatalf("DescribeDBClusters status=%d body=%q", desc.Code, desc.Body.String())
	}

	rej := mustNeptuneQuery(t, handler, strings.Join([]string{
		"Action=CreateDBCluster",
		"Version=2014-10-31",
		"DBClusterIdentifier=doc-1",
		"Engine=docdb",
	}, "&"), now)
	if rej.Code == http.StatusOK {
		t.Fatalf("docdb should be rejected on neptune surface: %q", rej.Body.String())
	}

	del := mustNeptuneQuery(t, handler,
		"Action=DeleteDBCluster&Version=2014-10-31&DBClusterIdentifier=lab-neptune-1", now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteDBCluster status=%d body=%q", del.Code, del.Body.String())
	}
}

func TestNeptuneNeo4jEngineSelection(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	t.Run("GraphEngine param neo4j", func(t *testing.T) {
		create := mustNeptuneQuery(t, handler, strings.Join([]string{
			"Action=CreateDBCluster",
			"Version=2014-10-31",
			"DBClusterIdentifier=lab-neo4j-param",
			"Engine=neptune",
			"GraphEngine=neo4j",
		}, "&"), now)
		if create.Code != http.StatusOK {
			t.Fatalf("status=%d body=%q", create.Code, create.Body.String())
		}
		if !strings.Contains(create.Body.String(), "7687") {
			t.Fatalf("want Bolt port 7687, body=%q", create.Body.String())
		}
		desc := mustNeptuneQuery(t, handler,
			"Action=DescribeDBClusters&Version=2014-10-31&DBClusterIdentifier=lab-neo4j-param", now)
		if desc.Code != http.StatusOK || !strings.Contains(desc.Body.String(), "7687") {
			t.Fatalf("describe status=%d body=%q", desc.Code, desc.Body.String())
		}
	})

	t.Run("tag noctaxris:neptune-engine", func(t *testing.T) {
		create := mustNeptuneQuery(t, handler, strings.Join([]string{
			"Action=CreateDBCluster",
			"Version=2014-10-31",
			"DBClusterIdentifier=lab-neo4j-tag",
			"Engine=neptune",
			"Tags.member.1.Key="+url.QueryEscape("noctaxris:neptune-engine"),
			"Tags.member.1.Value=neo4j",
		}, "&"), now)
		if create.Code != http.StatusOK {
			t.Fatalf("status=%d body=%q", create.Code, create.Body.String())
		}
		if !strings.Contains(create.Body.String(), "7687") {
			t.Fatalf("want Bolt port, body=%q", create.Body.String())
		}
	})

	t.Run("env neo4j", func(t *testing.T) {
		t.Setenv("NOCTAXRIS_NEPTUNE_ENGINE", "neo4j")
		create := mustNeptuneQuery(t, handler, strings.Join([]string{
			"Action=CreateDBCluster",
			"Version=2014-10-31",
			"DBClusterIdentifier=lab-neo4j-env",
			"Engine=neptune",
		}, "&"), now)
		if create.Code != http.StatusOK {
			t.Fatalf("status=%d body=%q", create.Code, create.Body.String())
		}
		if !strings.Contains(create.Body.String(), "7687") {
			t.Fatalf("want Bolt port from env, body=%q", create.Body.String())
		}
	})

	t.Run("fail-closed unknown engine", func(t *testing.T) {
		t.Setenv("NOCTAXRIS_NEPTUNE_ENGINE", "")
		rej := mustNeptuneQuery(t, handler, strings.Join([]string{
			"Action=CreateDBCluster",
			"Version=2014-10-31",
			"DBClusterIdentifier=lab-bad-engine",
			"Engine=neptune",
			"GraphEngine=arangodb",
		}, "&"), now)
		if rej.Code == http.StatusOK {
			t.Fatalf("unknown GraphEngine must fail closed: %q", rej.Body.String())
		}
		if !strings.Contains(rej.Body.String(), "InvalidParameterValue") {
			t.Fatalf("want InvalidParameterValue, body=%q", rej.Body.String())
		}
	})

	t.Run("fail-closed unknown env", func(t *testing.T) {
		t.Setenv("NOCTAXRIS_NEPTUNE_ENGINE", "not-real")
		rej := mustNeptuneQuery(t, handler, strings.Join([]string{
			"Action=CreateDBCluster",
			"Version=2014-10-31",
			"DBClusterIdentifier=lab-bad-env",
			"Engine=neptune",
		}, "&"), now)
		if rej.Code == http.StatusOK {
			t.Fatalf("unknown env must fail closed: %q", rej.Body.String())
		}
	})
}

func mustMSKJSON(t *testing.T, handler http.Handler, target string, payload map[string]any, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "Kafka_1.0."+target)
	signHeader(t, req, raw, testAccessKey, testSecret, testRegion, "kafka", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func mustMSKREST(t *testing.T, handler http.Handler, method, path string, payload map[string]any, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	var raw []byte
	var err error
	if payload != nil {
		raw, err = json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
	} else {
		raw = []byte{}
	}
	req := mustNewRequest(t, method, "http://127.0.0.1:4566"+path, raw)
	req.Header.Set("Content-Type", "application/json")
	signHeader(t, req, raw, testAccessKey, testSecret, testRegion, "kafka", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestMSKHandlers(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustMSKJSON(t, handler, "CreateCluster", map[string]any{
		"ClusterName":         "lab-msk-1",
		"KafkaVersion":        "3.6.0",
		"NumberOfBrokerNodes": 1,
		"BrokerNodeGroupInfo": map[string]any{
			"InstanceType":   "kafka.m5.large",
			"ClientSubnets":  []string{"subnet-1"},
		},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateCluster status=%d body=%q", create.Code, create.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(create.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	arn, _ := out["ClusterArn"].(string)
	if arn == "" {
		t.Fatalf("missing ClusterArn: %v", out)
	}

	desc := mustMSKJSON(t, handler, "DescribeCluster", map[string]any{"ClusterArn": arn}, now)
	if desc.Code != http.StatusOK {
		t.Fatalf("DescribeCluster status=%d body=%q", desc.Code, desc.Body.String())
	}
	body := desc.Body.String()
	if !strings.Contains(body, `"State":"FAILED"`) && !strings.Contains(body, `"State": "FAILED"`) {
		t.Fatalf("want FAILED without DinD; body=%q", body)
	}
	if strings.Contains(body, `"State":"ACTIVE"`) {
		t.Fatalf("must not claim ACTIVE without nested broker; body=%q", body)
	}

	list := mustMSKJSON(t, handler, "ListClusters", map[string]any{}, now)
	if list.Code != http.StatusOK {
		t.Fatalf("ListClusters status=%d body=%q", list.Code, list.Body.String())
	}

	boot := mustMSKJSON(t, handler, "GetBootstrapBrokers", map[string]any{"ClusterArn": arn}, now)
	if boot.Code != http.StatusOK {
		t.Fatalf("GetBootstrapBrokers status=%d body=%q", boot.Code, boot.Body.String())
	}
	var bootOut map[string]any
	if err := json.Unmarshal(boot.Body.Bytes(), &bootOut); err != nil {
		t.Fatal(err)
	}
	if s, _ := bootOut["BootstrapBrokerString"].(string); s != "" {
		t.Fatalf("non-ACTIVE must not return brokers, got %q", s)
	}

	del := mustMSKJSON(t, handler, "DeleteCluster", map[string]any{"ClusterArn": arn}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteCluster status=%d body=%q", del.Code, del.Body.String())
	}
}

func TestMSKRESTCreateListDelete(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustMSKREST(t, handler, http.MethodPost, "/v1/clusters", map[string]any{
		"clusterName":         "lab-msk-rest",
		"kafkaVersion":        "3.6.0",
		"numberOfBrokerNodes": 1,
		"brokerNodeGroupInfo": map[string]any{
			"instanceType":  "kafka.m5.large",
			"clientSubnets": []string{"subnet-1"},
		},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("REST CreateCluster status=%d body=%q", create.Code, create.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(create.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	arn, _ := out["ClusterArn"].(string)
	if arn == "" {
		t.Fatalf("missing ClusterArn: %v", out)
	}
	list := mustMSKREST(t, handler, http.MethodGet, "/v1/clusters", nil, now)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "lab-msk-rest") {
		t.Fatalf("REST ListClusters status=%d body=%q", list.Code, list.Body.String())
	}
	encoded := url.PathEscape(arn)
	del := mustMSKREST(t, handler, http.MethodDelete, "/v1/clusters/"+encoded, nil, now)
	if del.Code != http.StatusOK {
		t.Fatalf("REST DeleteCluster status=%d body=%q", del.Code, del.Body.String())
	}
}
