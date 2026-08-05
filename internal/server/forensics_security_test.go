package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/config"
)

func mustGuardDutyJSON(t *testing.T, handler http.Handler, target string, payload map[string]any, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "AWSGuardDuty."+target)
	signHeader(t, req, raw, testAccessKey, testSecret, testRegion, "guardduty", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func mustSecurityHubJSON(t *testing.T, handler http.Handler, target string, payload map[string]any, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "AWSSecurityHub."+target)
	signHeader(t, req, raw, testAccessKey, testSecret, testRegion, "securityhub", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func mustDetectiveJSON(t *testing.T, handler http.Handler, target string, payload map[string]any, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "AmazonDetective."+target)
	signHeader(t, req, raw, testAccessKey, testSecret, testRegion, "detective", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestGuardDutyDetectorFindingsInject(t *testing.T) {
	srv, st, _ := newTestServerStoreWith(t, func(cfg *config.Config) {
		cfg.GuardDutyInject = true
	})
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	disabledSrv, _ := newTestServer(t)
	disabled := mustGuardDutyJSON(t, disabledSrv.Handler(), "InjectFindings", map[string]any{
		"Finding": map[string]any{"Type": "UnauthorizedAccess:EC2/SSHBruteForce"},
	}, now)
	if disabled.Code != http.StatusForbidden {
		t.Fatalf("inject disabled status=%d body=%q", disabled.Code, disabled.Body.String())
	}

	create := mustGuardDutyJSON(t, handler, "CreateDetector", map[string]any{}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateDetector status=%d body=%q", create.Code, create.Body.String())
	}
	var createOut map[string]any
	_ = json.Unmarshal(create.Body.Bytes(), &createOut)
	detectorID, _ := createOut["DetectorId"].(string)
	if detectorID == "" {
		t.Fatalf("CreateDetector: %s", create.Body.String())
	}

	listDet := mustGuardDutyJSON(t, handler, "ListDetectors", map[string]any{}, now)
	if listDet.Code != http.StatusOK || !strings.Contains(listDet.Body.String(), detectorID) {
		t.Fatalf("ListDetectors status=%d body=%q", listDet.Code, listDet.Body.String())
	}

	inject := mustGuardDutyJSON(t, handler, "InjectFindings", map[string]any{
		"DetectorId": detectorID,
		"Finding": map[string]any{
			"Type":    "CryptoCurrency:EC2/BitcoinTool.B!DNS",
			"Severity": 8,
		},
	}, now)
	if inject.Code != http.StatusOK {
		t.Fatalf("InjectFindings status=%d body=%q", inject.Code, inject.Body.String())
	}
	var inj map[string]any
	_ = json.Unmarshal(inject.Body.Bytes(), &inj)
	rawIDs, _ := inj["FindingIds"].([]any)
	if len(rawIDs) < 1 {
		rawIDs, _ = inj["findingIds"].([]any)
	}
	if len(rawIDs) < 1 {
		t.Fatalf("inject response=%v", inj)
	}

	list := mustGuardDutyJSON(t, handler, "ListFindings", map[string]any{"DetectorId": detectorID}, now)
	if list.Code != http.StatusOK {
		t.Fatalf("ListFindings status=%d body=%q", list.Code, list.Body.String())
	}
	get := mustGuardDutyJSON(t, handler, "GetFindings", map[string]any{
		"DetectorId": detectorID,
		"FindingIds": rawIDs,
	}, now)
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), "CryptoCurrency") {
		t.Fatalf("GetFindings status=%d body=%q", get.Code, get.Body.String())
	}

	missing := mustGuardDutyJSON(t, handler, "ListFindings", map[string]any{"DetectorId": "missing"}, now)
	if missing.Code == http.StatusOK {
		t.Fatalf("missing detector should fail: %q", missing.Body.String())
	}

	_, denyARN, err := st.CreateUser(testAccountID, "gd-deny")
	if err != nil {
		t.Fatal(err)
	}
	denyAK, denySecret, err := st.CreateUserAccessKey(testAccountID, "gd-deny")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutInlinePolicy(denyARN, "deny", `{
		"Version":"2012-10-17",
		"Statement":[{"Effect":"Deny","Action":"guardduty:ListDetectors","Resource":"*"}]
	}`); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{})
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "AWSGuardDuty.ListDetectors")
	signHeader(t, req, raw, denyAK, denySecret, testRegion, "guardduty", now)
	denyRec := httptest.NewRecorder()
	handler.ServeHTTP(denyRec, req)
	if denyRec.Code != http.StatusForbidden || !strings.Contains(denyRec.Body.String(), "AccessDeniedException") {
		t.Fatalf("authz deny status=%d body=%q", denyRec.Code, denyRec.Body.String())
	}

	unimplemented := mustGuardDutyJSON(t, handler, "DeleteDetector", map[string]any{"DetectorId": detectorID}, now)
	if unimplemented.Code != http.StatusNotImplemented {
		t.Fatalf("unimplemented status=%d body=%q", unimplemented.Code, unimplemented.Body.String())
	}
}

func TestSecurityHubImportAndGetFindings(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	productARN := "arn:aws:securityhub:" + testRegion + ":" + testAccountID + ":product/" + testAccountID + "/default"
	importRec := mustSecurityHubJSON(t, handler, "BatchImportFindings", map[string]any{
		"Findings": []map[string]any{{
			"SchemaVersion": "2018-10-08",
			"Id":            "finding-1",
			"ProductArn":    productARN,
			"GeneratorId":   "lab-generator",
			"AwsAccountId":  testAccountID,
			"Types":         []string{"Software and Configuration Checks"},
			"CreatedAt":     now.Format(time.RFC3339),
			"UpdatedAt":     now.Format(time.RFC3339),
			"Severity":      map[string]any{"Label": "HIGH"},
			"Title":         "lab finding",
			"Description":   "imported finding",
			"Resources": []map[string]any{{
				"Type": "AwsS3Bucket",
				"Id":   "arn:aws:s3:::lab-bucket",
			}},
		}},
	}, now)
	if importRec.Code != http.StatusOK {
		t.Fatalf("BatchImportFindings status=%d body=%q", importRec.Code, importRec.Body.String())
	}

	get := mustSecurityHubJSON(t, handler, "GetFindings", map[string]any{
		"Filters": map[string]any{
			"SeverityLabel": map[string]any{"Value": "HIGH"},
			"ProductArn":    map[string]any{"Value": productARN},
		},
	}, now)
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), "lab finding") {
		t.Fatalf("GetFindings status=%d body=%q", get.Code, get.Body.String())
	}

	flat := mustSecurityHubJSON(t, handler, "GetFindings", map[string]any{
		"SeverityLabel": "HIGH",
		"ResourceType":  "AwsS3Bucket",
	}, now)
	if flat.Code != http.StatusOK || !strings.Contains(flat.Body.String(), "AwsS3Bucket") {
		t.Fatalf("flat GetFindings status=%d body=%q", flat.Code, flat.Body.String())
	}

	bad := mustSecurityHubJSON(t, handler, "BatchImportFindings", map[string]any{
		"Findings": []any{"not-an-object"},
	}, now)
	if bad.Code == http.StatusOK {
		t.Fatalf("invalid findings should fail: %q", bad.Body.String())
	}

	_, denyARN, err := st.CreateUser(testAccountID, "sh-deny")
	if err != nil {
		t.Fatal(err)
	}
	denyAK, denySecret, err := st.CreateUserAccessKey(testAccountID, "sh-deny")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutInlinePolicy(denyARN, "deny", `{
		"Version":"2012-10-17",
		"Statement":[{"Effect":"Deny","Action":"securityhub:GetFindings","Resource":"*"}]
	}`); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{})
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "AWSSecurityHub.GetFindings")
	signHeader(t, req, raw, denyAK, denySecret, testRegion, "securityhub", now)
	denyRec := httptest.NewRecorder()
	handler.ServeHTTP(denyRec, req)
	if denyRec.Code != http.StatusForbidden || !strings.Contains(denyRec.Body.String(), "AccessDeniedException") {
		t.Fatalf("authz deny status=%d body=%q", denyRec.Code, denyRec.Body.String())
	}
}

func TestDetectiveGraphSearchAccept(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustDetectiveJSON(t, handler, "CreateGraph", map[string]any{}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateGraph status=%d body=%q", create.Code, create.Body.String())
	}
	var createOut map[string]any
	_ = json.Unmarshal(create.Body.Bytes(), &createOut)
	graphARN, _ := createOut["GraphArn"].(string)
	if graphARN == "" {
		t.Fatalf("CreateGraph: %s", create.Body.String())
	}

	list := mustDetectiveJSON(t, handler, "ListGraphs", map[string]any{"MaxResults": 10}, now)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), graphARN) {
		t.Fatalf("ListGraphs status=%d body=%q", list.Code, list.Body.String())
	}

	acceptMissing := mustDetectiveJSON(t, handler, "AcceptInvitation", map[string]any{}, now)
	if acceptMissing.Code == http.StatusOK || !strings.Contains(acceptMissing.Body.String(), "ValidationException") {
		t.Fatalf("AcceptInvitation empty status=%d body=%q", acceptMissing.Code, acceptMissing.Body.String())
	}
	acceptGone := mustDetectiveJSON(t, handler, "AcceptInvitation", map[string]any{
		"GraphArn": "arn:aws:detective:us-east-1:" + testAccountID + ":graph/missing",
	}, now)
	if acceptGone.Code == http.StatusOK {
		t.Fatalf("AcceptInvitation missing should fail: %q", acceptGone.Body.String())
	}
	accept := mustDetectiveJSON(t, handler, "AcceptInvitation", map[string]any{"GraphArn": graphARN}, now)
	if accept.Code != http.StatusOK {
		t.Fatalf("AcceptInvitation status=%d body=%q", accept.Code, accept.Body.String())
	}

	search := mustDetectiveJSON(t, handler, "SearchGraph", map[string]any{
		"GraphArn":    graphARN,
		"ResourceArn": "arn:aws:ec2:us-east-1:" + testAccountID + ":instance/i-abc",
		"MaxResults":  5,
	}, now)
	if search.Code != http.StatusOK {
		t.Fatalf("SearchGraph status=%d body=%q", search.Code, search.Body.String())
	}

	unimplemented := mustDetectiveJSON(t, handler, "DeleteGraph", map[string]any{"GraphArn": graphARN}, now)
	if unimplemented.Code != http.StatusNotImplemented {
		t.Fatalf("unimplemented status=%d body=%q", unimplemented.Code, unimplemented.Body.String())
	}

	_, denyARN, err := st.CreateUser(testAccountID, "det-deny")
	if err != nil {
		t.Fatal(err)
	}
	denyAK, denySecret, err := st.CreateUserAccessKey(testAccountID, "det-deny")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutInlinePolicy(denyARN, "deny", `{
		"Version":"2012-10-17",
		"Statement":[{"Effect":"Deny","Action":"detective:CreateGraph","Resource":"*"}]
	}`); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{})
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "AmazonDetective.CreateGraph")
	signHeader(t, req, raw, denyAK, denySecret, testRegion, "detective", now)
	denyRec := httptest.NewRecorder()
	handler.ServeHTTP(denyRec, req)
	if denyRec.Code != http.StatusForbidden || !strings.Contains(denyRec.Body.String(), "AccessDeniedException") {
		t.Fatalf("authz deny status=%d body=%q", denyRec.Code, denyRec.Body.String())
	}
}

func TestMacieDescribeAndListJobs(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	enable := mustMacieJSON(t, handler, "Macie2.EnableMacie", map[string]any{}, now)
	if enable.Code != http.StatusOK {
		t.Fatalf("EnableMacie status=%d body=%q", enable.Code, enable.Body.String())
	}
	createJob := mustMacieJSON(t, handler, "Macie2.CreateClassificationJob", map[string]any{
		"name": "list-job", "jobType": "ONE_TIME",
		"s3JobDefinition": map[string]any{
			"bucketDefinitions": []map[string]any{
				{"accountId": testAccountID, "buckets": []string{"macie-list"}},
			},
		},
	}, now)
	if createJob.Code != http.StatusOK {
		t.Fatalf("CreateClassificationJob status=%d body=%q", createJob.Code, createJob.Body.String())
	}
	var jobResp map[string]any
	_ = json.Unmarshal(createJob.Body.Bytes(), &jobResp)
	jobID, _ := jobResp["jobId"].(string)

	desc := mustMacieJSON(t, handler, "Macie2.DescribeClassificationJob", map[string]any{"jobId": jobID}, now)
	if desc.Code != http.StatusOK || !strings.Contains(desc.Body.String(), "list-job") {
		t.Fatalf("DescribeClassificationJob status=%d body=%q", desc.Code, desc.Body.String())
	}
	list := mustMacieJSON(t, handler, "Macie2.ListClassificationJobs", map[string]any{"maxResults": 10}, now)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), jobID) {
		t.Fatalf("ListClassificationJobs status=%d body=%q", list.Code, list.Body.String())
	}
	missing := mustMacieJSON(t, handler, "Macie2.DescribeClassificationJob", map[string]any{"jobId": "missing"}, now)
	if missing.Code == http.StatusOK {
		t.Fatalf("missing job should fail: %q", missing.Body.String())
	}
}
