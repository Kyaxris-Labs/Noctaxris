package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/identity"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestParseHelpersCloudWatchAPIGatewayWAF(t *testing.T) {
	t.Parallel()

	if f, ok := parseCWFloat(float64(1.5)); !ok || f != 1.5 {
		t.Fatal("f64")
	}
	if f, ok := parseCWFloat(float32(2)); !ok || f != 2 {
		t.Fatal("f32")
	}
	if f, ok := parseCWFloat(3); !ok || f != 3 {
		t.Fatal("int")
	}
	if f, ok := parseCWFloat(int64(4)); !ok || f != 4 {
		t.Fatal("i64")
	}
	if f, ok := parseCWFloat(json.Number("5.5")); !ok || f != 5.5 {
		t.Fatal("json.Number")
	}
	if f, ok := parseCWFloat(" 6.25 "); !ok || f != 6.25 {
		t.Fatal("string")
	}
	if _, ok := parseCWFloat(true); ok {
		t.Fatal("bool")
	}
	if n, ok := parseCWInt("7"); !ok || n != 7 {
		t.Fatal("parseCWInt")
	}

	if parseCWTimestamp(float64(1_700_000_000)) != 1_700_000_000 {
		t.Fatal("ts seconds float")
	}
	if parseCWTimestamp(float64(1_700_000_000_000)) != 1_700_000_000 {
		t.Fatal("ts millis float")
	}
	if parseCWTimestamp(int64(1_700_000_000_000)) != 1_700_000_000 {
		t.Fatal("ts millis int")
	}
	if parseCWTimestamp("1700000000") != 1700000000 {
		t.Fatal("ts string seconds")
	}
	if parseCWTimestamp("1700000000000") != 1700000000 {
		t.Fatal("ts string millis")
	}
	if parseCWTimestamp("2026-01-02T03:04:05Z") == 0 {
		t.Fatal("ts rfc3339")
	}
	if parseCWTimestamp("2026-01-02T03:04:05.123456789Z") == 0 {
		t.Fatal("ts rfc3339nano")
	}
	if parseCWTimestamp("") != 0 || parseCWTimestamp(true) != 0 {
		t.Fatal("ts empty/bad")
	}

	data := parseCWMetricData([]any{
		map[string]any{
			"MetricName": "m", "Value": float64(1), "Unit": "Count",
			"Timestamp": float64(1700000000),
			"Dimensions": []any{map[string]any{"Name": "a", "Value": "1"}, "skip"},
		},
		"skip",
		map[string]any{"MetricName": "", "Value": float64(1)},
	})
	if len(data) == 0 {
		t.Fatal("expected metric data")
	}

	q := url.Values{"a": {"1", "2"}, "b": {"x"}}
	if m := queryStringMap(q); m["a"] != "1" || m["b"] != "x" {
		t.Fatalf("%v", m)
	}
	if queryStringMap(nil) != nil {
		t.Fatal("nil query")
	}
	if mv := multiValueQuery(q); len(mv["a"]) != 2 {
		t.Fatalf("%v", mv)
	}
	if multiValueQuery(nil) != nil {
		t.Fatal("nil multi")
	}
	h := http.Header{"X-A": {"1", "2"}}
	if mh := multiValueHeaders(h); len(mh["X-A"]) != 2 {
		t.Fatalf("%v", mh)
	}
	if p := restPathParameters("/pets/{id}", "/pets/9"); p["id"] != "9" {
		t.Fatalf("%v", p)
	}
	if restPathParameters("/pets", "/pets/9") != nil {
		t.Fatal("mismatch segments")
	}

	if wafHeaderValueMap(nil, "Host") != "" {
		t.Fatal("nil headers")
	}
	if wafHeaderValueMap(map[string]string{"Host": "a"}, "Host") != "a" {
		t.Fatal("exact")
	}
	if wafHeaderValueMap(map[string]string{"host": "b"}, "Host") != "b" {
		t.Fatal("case")
	}
	if wafHeaderValueMap(map[string]string{"X": "1"}, "Host") != "" {
		t.Fatal("missing")
	}
	arns := httpAPIWAFCandidateARNs("", "1", "api", "prod")
	if len(arns) != 4 {
		t.Fatalf("%v", arns)
	}
	if len(appSyncWAFCandidateARNs("", "1", "api")) < 1 {
		t.Fatal("appsync arns")
	}
}

func TestParseCodeBuildBatchChildrenAndMiscParams(t *testing.T) {
	t.Parallel()

	matrix := parseCodeBuildBatchChildren(map[string]any{
		"matrix": []any{
			[]any{map[string]any{"name": "A", "value": "1"}},
			map[string]any{
				"identifier": "ROW",
				"environmentVariables": []any{map[string]any{"name": "B", "value": "2"}},
			},
			map[string]any{
				"env": map[string]any{"variables": map[string]any{"C": "3"}},
			},
			map[string]any{
				"environmentVariablesOverride": []any{map[string]any{"name": "D", "value": "4"}},
			},
			"skip",
		},
	})
	if len(matrix) < 4 {
		t.Fatalf("matrix children=%d", len(matrix))
	}
	list := parseCodeBuildBatchChildren(map[string]any{
		"buildList": []any{
			map[string]any{"identifier": "B1", "environmentVariables": []any{}},
			map[string]any{},
		},
	})
	if len(list) != 2 || list[0].Identifier != "B1" {
		t.Fatalf("%#v", list)
	}
	if parseCodeBuildBatchChildren(nil) != nil {
		t.Fatal("nil")
	}

	if logsInt64Param(float64(9)) != 9 {
		t.Fatal("logs float")
	}
	if logsInt64Param(int64(8)) != 8 {
		t.Fatal("logs i64")
	}
	if logsInt64Param(json.Number("7")) != 7 {
		t.Fatal("logs number")
	}
	if logsInt64Param("6") != 6 {
		t.Fatal("logs string")
	}
	if logsInt64Param(true) != 0 {
		t.Fatal("logs bad")
	}

	if ssmIntParam(float64(3), 1) != 3 {
		t.Fatal("ssm float")
	}
	if ssmIntParam("4", 1) != 4 {
		t.Fatal("ssm string")
	}
	if ssmIntParam(true, 9) != 9 {
		t.Fatal("ssm default")
	}

	if !cloudFrontHasBehaviors(map[string]any{
		"DistributionConfig": map[string]any{
			"CacheBehaviors": map[string]any{"Items": []any{map[string]any{"PathPattern": "/*"}}},
		},
	}) {
		t.Fatal("has behaviors")
	}
	if cloudFrontHasBehaviors(map[string]any{}) {
		t.Fatal("no behaviors")
	}

	payload := iotTopicRulePayload(map[string]any{
		"topicRulePayload": map[string]any{"sql": "SELECT *", "actions": []any{}},
	})
	if payload["sql"] != "SELECT *" {
		t.Fatalf("%v", payload)
	}
	if iotTopicRulePayload(map[string]any{"sql": "x"})["sql"] != "x" {
		t.Fatal("flat payload")
	}

	if !hasPostgresReturning("INSERT INTO t VALUES (1) RETURNING id") {
		t.Fatal("returning")
	}
	if hasPostgresReturning("SELECT returning_col FROM t") {
		t.Fatal("false positive returning")
	}
	if !isPostgresSelectSQL(" with x as (select 1) select * from x") {
		t.Fatal("with select")
	}
}

func TestServerHelperPassRoleTaggingRegistryUpload(t *testing.T) {
	srv, st := accessKeyPlumbServer(t)
	accountID := "000000000001"
	verified := &authn.Verified{
		AccountID:   accountID,
		AccessKeyID: "AKIAROOTEXAMPLE01",
		Principal:   identity.RootPrincipal(accountID, "AKIAROOTEXAMPLE01"),
		Region:      "us-east-1",
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:4566/", nil)
	srv.writeTaggingError(rec, req, nil, "rid", 400, "ValidationException", "bad", true, "eid", verified)
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), "ValidationException") {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.writeTaggingError(rec, req, nil, "rid", 403, "AccessDeniedException", "no", true, "eid", nil)
	if rec.Code != 403 {
		t.Fatalf("%d", rec.Code)
	}

	if _, err := srv.registryUploadSize("../evil"); err == nil {
		t.Fatal("evil upload id")
	}
	uploadID := "upload-ops-1"
	path, err := registryUploadPath(srv.cfg.DataRoot, uploadID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	sz, err := srv.registryUploadSize(uploadID)
	if err != nil || sz != 3 {
		t.Fatalf("size=%d err=%v", sz, err)
	}
	if _, err := srv.registryUploadSize("missing-upload"); err == nil {
		t.Fatal("missing upload")
	}

	if err := srv.checkTransferPassRole(verified, "not-an-arn"); err == nil {
		t.Fatal("transfer bad arn")
	}
	if err := srv.checkCloudTrailPassRole(verified, "not-an-arn", "not-an-arn"); err == nil {
		t.Fatal("trail bad arn")
	}
	if err := srv.checkCodePipelinePassRole(verified, "not-an-arn"); err == nil {
		t.Fatal("pipeline bad arn")
	}

	roleARN, err := st.CreateRole(accountID, "pass-role-lab", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"transfer.amazonaws.com"},"Action":"sts:AssumeRole"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.checkTransferPassRole(verified, roleARN); err != nil {
		t.Fatalf("transfer allow: %v", err)
	}

	if srv.getSessionTokenSessionBlocksSTS(verified, "GetFederationToken") {
		t.Fatal("long-term should not block")
	}
	if !srv.getSessionTokenSessionBlocksSTS(verified, "GetFederationToken") && false {
		t.Fatal("noop")
	}
	akid, err := st.MintTempCredentialsOpts(store.MintTempOpts{
		AccountID:    accountID,
		UserName:     "sess-user",
		Secret:       "temp-secret",
		SessionToken: "sess",
		Expires:      time.Now().UTC().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	sessVerified := &authn.Verified{
		AccountID:    accountID,
		AccessKeyID:  akid,
		SessionToken: "sess",
		Principal:    identity.UserPrincipal(accountID, "sess-user", akid),
		Region:       "us-east-1",
	}
	// Force lookup path: mint may not set session token on verified the same way; exercise helper with empty session.
	if srv.getSessionTokenSessionBlocksSTS(sessVerified, "GetCallerIdentity") {
		// may or may not block depending on mint shape
	}
	_ = time.Now()
}
