package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func mustEKSREST(t *testing.T, handler http.Handler, method, path string, payload map[string]any, now time.Time) *httptest.ResponseRecorder {
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
	signHeader(t, req, raw, testAccessKey, testSecret, testRegion, "eks", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestEKSHandlersCRUD(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustEKSREST(t, handler, http.MethodPost, "/clusters", map[string]any{
		"name":    "lab-eks-1",
		"roleArn": "arn:aws:iam::000000000001:role/eks",
		"version": "1.29",
		"resourcesVpcConfig": map[string]any{
			"subnetIds":         []string{"subnet-1"},
			"securityGroupIds":  []string{},
			"endpointPublicAccess": true,
		},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateCluster status=%d body=%q", create.Code, create.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	cluster, _ := created["cluster"].(map[string]any)
	if cluster == nil {
		t.Fatalf("missing cluster: %v", created)
	}
	if cluster["status"] != "ACTIVE" {
		t.Fatalf("want ACTIVE metadata-only, got %v", cluster["status"])
	}
	ep, _ := cluster["endpoint"].(string)
	if !strings.Contains(ep, "noctaxris-eks-lab-eks-1:6443") {
		t.Fatalf("endpoint=%q", ep)
	}
	ca, _ := cluster["certificateAuthority"].(map[string]any)
	if ca == nil {
		t.Fatalf("missing certificateAuthority: %v", cluster)
	}
	if _, ok := ca["data"]; ok {
		t.Fatalf("metadata-only must not claim CA data: %v", ca)
	}

	desc := mustEKSREST(t, handler, http.MethodGet, "/clusters/lab-eks-1", nil, now)
	if desc.Code != http.StatusOK || !strings.Contains(desc.Body.String(), `"ACTIVE"`) {
		t.Fatalf("DescribeCluster status=%d body=%q", desc.Code, desc.Body.String())
	}

	list := mustEKSREST(t, handler, http.MethodGet, "/clusters", nil, now)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "lab-eks-1") {
		t.Fatalf("ListClusters status=%d body=%q", list.Code, list.Body.String())
	}

	nodes := mustEKSREST(t, handler, http.MethodGet, "/clusters/lab-eks-1/node-groups", nil, now)
	if nodes.Code != http.StatusOK {
		t.Fatalf("ListNodegroups status=%d body=%q", nodes.Code, nodes.Body.String())
	}
	var ng map[string]any
	if err := json.Unmarshal(nodes.Body.Bytes(), &ng); err != nil {
		t.Fatal(err)
	}
	arr, _ := ng["nodegroups"].([]any)
	if len(arr) != 0 {
		t.Fatalf("want empty nodegroups stub, got %v", ng)
	}

	dup := mustEKSREST(t, handler, http.MethodPost, "/clusters", map[string]any{
		"name":    "lab-eks-1",
		"roleArn": "arn:aws:iam::000000000001:role/eks",
		"resourcesVpcConfig": map[string]any{
			"subnetIds": []string{"subnet-1"},
		},
	}, now)
	if dup.Code != http.StatusConflict {
		t.Fatalf("duplicate want 409, got %d body=%q", dup.Code, dup.Body.String())
	}

	del := mustEKSREST(t, handler, http.MethodDelete, "/clusters/lab-eks-1", nil, now)
	if del.Code != http.StatusOK || !strings.Contains(del.Body.String(), "DELETING") {
		t.Fatalf("DeleteCluster status=%d body=%q", del.Code, del.Body.String())
	}

	missing := mustEKSREST(t, handler, http.MethodGet, "/clusters/lab-eks-1", nil, now)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("after delete want 404, got %d body=%q", missing.Code, missing.Body.String())
	}
}
