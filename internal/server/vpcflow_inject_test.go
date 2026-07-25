package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/config"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func mustVPCFlowJSON(t *testing.T, handler http.Handler, target string, payload map[string]any, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", target)
	signHeader(t, req, raw, testAccessKey, testSecret, testRegion, "ec2", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestVPCFlowInjectDisabledRejects(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	rec := mustVPCFlowJSON(t, handler, "NoctaxrisEC2.InjectFlowLogs", map[string]any{
		"FlowLogId": "fl-deadbeefdeadbeef",
	}, now)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%q want 403", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "NOCTAXRIS_VPCFLOW_INJECT") {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestVPCFlowCreateAndInjectS3(t *testing.T) {
	srv, st, _ := newTestServerStoreWith(t, func(cfg *config.Config) {
		cfg.VPCFlowInject = true
	})
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)
	account := testAccountID

	if _, err := st.CreateBucket(account, "vpc-flow-lab"); err != nil {
		t.Fatal(err)
	}

	createRec := mustVPCFlowJSON(t, handler, "AmazonEC2.CreateFlowLogs", map[string]any{
		"ResourceIds":        []string{"vpc-labopaque001"},
		"ResourceType":       "VPC",
		"TrafficType":        "ALL",
		"LogDestinationType": "s3",
		"LogDestination":     "arn:aws:s3:::vpc-flow-lab",
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	var createOut struct {
		FlowLogIds []string `json:"FlowLogIds"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &createOut); err != nil {
		t.Fatal(err)
	}
	if len(createOut.FlowLogIds) != 1 || !strings.HasPrefix(createOut.FlowLogIds[0], "fl-") {
		t.Fatalf("FlowLogIds=%v", createOut.FlowLogIds)
	}

	injectRec := mustVPCFlowJSON(t, handler, "NoctaxrisEC2.InjectFlowLogs", map[string]any{
		"FlowLogId": createOut.FlowLogIds[0],
	}, now)
	if injectRec.Code != http.StatusOK {
		t.Fatalf("inject status=%d body=%q", injectRec.Code, injectRec.Body.String())
	}

	listed, err := st.ListObjectsV2(account, "vpc-flow-lab", "AWSLogs/", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Contents) == 0 {
		t.Fatal("no flow log object in S3")
	}
	_, body, err := st.GetObject(account, "vpc-flow-lab", listed.Contents[0].Key)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, " ACCEPT OK") || !strings.Contains(text, " REJECT OK") {
		t.Fatalf("s3 body=%q", text)
	}
}

func TestVPCFlowInjectCustomRecords(t *testing.T) {
	srv, st, _ := newTestServerStoreWith(t, func(cfg *config.Config) {
		cfg.VPCFlowInject = true
	})
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)
	account := testAccountID

	if _, err := st.CreateBucket(account, "vpc-flow-custom"); err != nil {
		t.Fatal(err)
	}
	fl, err := st.CreateVPCFlowLog(account, store.CreateVPCFlowLogInput{
		LogDestinationType: "s3",
		LogDestination:     "arn:aws:s3:::vpc-flow-custom",
		Region:             testRegion,
	})
	if err != nil {
		t.Fatal(err)
	}

	line := store.FormatVPCFlowLogV2Line(store.VPCFlowRecord{
		Version: 2, AccountID: account, InterfaceID: "eni-custom",
		SrcAddr: "192.0.2.1", DstAddr: "192.0.2.2", SrcPort: 80, DstPort: 8080,
		Protocol: 6, Packets: 5, Bytes: 500, Start: 1700000000, End: 1700000060,
		Action: "ACCEPT", LogStatus: "OK",
	})
	rec := mustVPCFlowJSON(t, handler, "NoctaxrisEC2.InjectFlowLogs", map[string]any{
		"FlowLogId": fl.FlowLogID,
		"Lines":     []string{line},
	}, now)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	listed, err := st.ListObjectsV2(account, "vpc-flow-custom", "AWSLogs/", "")
	if err != nil {
		t.Fatal(err)
	}
	_, body, err := st.GetObject(account, "vpc-flow-custom", listed.Contents[0].Key)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "eni-custom") {
		t.Fatalf("body=%q", body)
	}
}
