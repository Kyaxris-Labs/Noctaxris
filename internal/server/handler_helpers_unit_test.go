package server

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/compute"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/audit"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/identity"
)

func TestParseFunctionURLCorsAndHeaders(t *testing.T) {
	t.Parallel()
	if got := parseFunctionURLCorsAllowOrigins(nil); got != nil {
		t.Fatalf("nil params: %#v", got)
	}
	if got := parseFunctionURLCorsAllowOrigins(map[string]any{"Cors": map[string]any{}}); len(got) != 0 {
		t.Fatalf("empty cors: %#v", got)
	}
	got := parseFunctionURLCorsAllowOrigins(map[string]any{
		"Cors": map[string]any{"AllowOrigins": []any{"https://a.example", "", 1, "https://b.example"}},
	})
	if len(got) != 2 || got[0] != "https://a.example" {
		t.Fatalf("AllowOrigins any: %#v", got)
	}
	got = parseFunctionURLCorsAllowOrigins(map[string]any{
		"cors": map[string]any{"allowOrigins": []string{"*"}},
	})
	if len(got) != 1 || got[0] != "*" {
		t.Fatalf("allowOrigins strings: %#v", got)
	}
	if got := parseFunctionURLCorsAllowOrigins(map[string]any{
		"Cors": map[string]any{"AllowOrigins": "bad"},
	}); got != nil {
		t.Fatalf("bad type: %#v", got)
	}

	rec := httptest.NewRecorder()
	setFunctionURLCORSHeaders(rec, "", nil)
	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("default ACAO=%q", rec.Header().Get("Access-Control-Allow-Origin"))
	}
	rec = httptest.NewRecorder()
	setFunctionURLCORSHeaders(rec, "https://a.example", []string{"https://a.example", "https://b.example"})
	if rec.Header().Get("Access-Control-Allow-Origin") != "https://a.example" {
		t.Fatalf("matched ACAO=%q", rec.Header().Get("Access-Control-Allow-Origin"))
	}
	rec = httptest.NewRecorder()
	setFunctionURLCORSHeaders(rec, "https://z.example", []string{"https://a.example"})
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("unmatched should omit ACAO, got %q", rec.Header().Get("Access-Control-Allow-Origin"))
	}
	if rec.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Fatal("expected methods when Origin unmatched")
	}
	rec = httptest.NewRecorder()
	setFunctionURLCORSHeaders(rec, "https://x.example", []string{"*"})
	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("star ACAO=%q", rec.Header().Get("Access-Control-Allow-Origin"))
	}

	if !isFunctionURLPath("/lambda-url/acc/fn") {
		t.Fatal("path should match")
	}
	if isFunctionURLPath("/other") {
		t.Fatal("path should not match")
	}
	acc, fn, ok := parseFunctionURLPath("/lambda-url/123/my-fn")
	if !ok || acc != "123" || fn != "my-fn" {
		t.Fatalf("parse got %q %q %v", acc, fn, ok)
	}
	if _, _, ok := parseFunctionURLPath("/lambda-url/only"); ok {
		t.Fatal("short path should fail")
	}
	if _, _, ok := parseFunctionURLPath("/lambda-url//fn"); ok {
		t.Fatal("empty account should fail")
	}
}

func TestLightsailIntAndEMRProperties(t *testing.T) {
	t.Parallel()
	if n, ok := lightsailInt(map[string]any{"sizeInGb": float64(32)}, "sizeInGb"); !ok || n != 32 {
		t.Fatalf("float64: %d %v", n, ok)
	}
	if n, ok := lightsailInt(map[string]any{"sizeInGb": 8}, "sizeInGb"); !ok || n != 8 {
		t.Fatalf("int: %d %v", n, ok)
	}
	if n, ok := lightsailInt(map[string]any{"sizeInGb": int64(16)}, "sizeInGb"); !ok || n != 16 {
		t.Fatalf("int64: %d %v", n, ok)
	}
	if n, ok := lightsailInt(map[string]any{"sizeInGb": json.Number("64")}, "sizeInGb"); !ok || n != 64 {
		t.Fatalf("json.Number: %d %v", n, ok)
	}
	if _, ok := lightsailInt(map[string]any{"sizeInGb": "nope"}, "sizeInGb"); ok {
		t.Fatal("string should fail")
	}
	if lightsailPortInfo(nil) != nil {
		t.Fatal("nil portInfo")
	}
	if lightsailPortInfo(map[string]any{"portInfo": map[string]any{"fromPort": 80}})["fromPort"] != 80 {
		t.Fatal("portInfo missing")
	}

	if emrProperties(nil) != nil {
		t.Fatal("nil emr")
	}
	props := emrProperties([]any{
		map[string]any{"Key": "a", "Value": "1"},
		"skip",
		map[string]any{"Key": "", "Value": "x"},
		map[string]any{"Key": "b", "Value": "2"},
	})
	if props["a"] != "1" || props["b"] != "2" || len(props) != 2 {
		t.Fatalf("emrProperties=%#v", props)
	}
	if stringField(map[string]any{"Key": 1}, "Key") != "" {
		t.Fatal("stringField non-string")
	}
}

func TestParseWAFSizeAndIPSetHelpers(t *testing.T) {
	t.Parallel()
	if parseWAFSizeConstraintStatement(nil) != nil {
		t.Fatal("nil rule")
	}
	if parseWAFSizeConstraintStatement(map[string]any{}) != nil {
		t.Fatal("missing statement")
	}
	uri := parseWAFSizeConstraintStatement(map[string]any{
		"SizeConstraintStatement": map[string]any{
			"ComparisonOperator": "GT",
			"Size":               float64(100),
			"FieldToMatch":       map[string]any{"UriPath": map[string]any{}},
		},
	})
	if uri == nil || uri.FieldToMatchType != "UriPath" || uri.Size != 100 {
		t.Fatalf("uri path: %#v", uri)
	}
	hdr := parseWAFSizeConstraintStatement(map[string]any{
		"SizeConstraintStatement": map[string]any{
			"ComparisonOperator": "EQ",
			"Size":               json.Number("10"),
			"FieldToMatch":       map[string]any{"SingleHeader": map[string]any{"Name": "Host"}},
		},
	})
	if hdr == nil || hdr.HeaderName != "Host" || hdr.Size != 10 {
		t.Fatalf("header: %#v", hdr)
	}
	if parseWAFSizeConstraintStatement(map[string]any{
		"SizeConstraintStatement": map[string]any{
			"Size": "bad", "FieldToMatch": map[string]any{"UriPath": map[string]any{}},
		},
	}) != nil {
		t.Fatal("bad size should nil")
	}
	ip := parseWAFIPSetReferenceStatement(map[string]any{
		"IPSetReferenceStatement": map[string]any{
			"ARN":       "arn:aws:wafv2:us-east-1:1:regional/ipset/x",
			"Addresses": []any{"1.2.3.4/32", 9, "  ", "5.6.7.8/32"},
		},
	})
	if ip == nil || len(ip.Addresses) != 2 {
		t.Fatalf("ipset ref: %#v", ip)
	}
	if parseWAFIPSetReferenceStatement(map[string]any{
		"IPSetReferenceStatement": map[string]any{},
	}) != nil {
		t.Fatal("empty ipset should nil")
	}
	addrs := parseWAFIPSetAddresses(map[string]any{"Addresses": []any{"10.0.0.0/8", ""}})
	if len(addrs) != 1 || addrs[0] != "10.0.0.0/8" {
		t.Fatalf("addrs=%#v", addrs)
	}
}

func TestCloudTrailInjectHelpersCoverage(t *testing.T) {
	t.Parallel()
	verified := &authn.Verified{
		AccountID: "000000000001",
		Principal: identity.Principal{Kind: identity.KindUser, AccountID: "000000000001", UserName: "root"},
	}

	list, err := cloudtrailInjectEventList(map[string]any{
		"Events": []any{map[string]any{"eventName": "A"}, "bad"},
	})
	if err == nil || list != nil {
		t.Fatalf("bad Events entry want error got %v %v", list, err)
	}
	list, err = cloudtrailInjectEventList(map[string]any{
		"Event": map[string]any{"eventName": "Solo"},
	})
	if err != nil || len(list) != 1 {
		t.Fatalf("Event object: %v %v", list, err)
	}
	list, err = cloudtrailInjectEventList(map[string]any{"eventName": "Flat"})
	if err != nil || len(list) != 1 {
		t.Fatalf("flat eventName: %v %v", list, err)
	}
	list, err = cloudtrailInjectEventList(map[string]any{"EventName": "Flat2"})
	if err != nil || len(list) != 1 {
		t.Fatalf("flat EventName: %v %v", list, err)
	}
	list, err = cloudtrailInjectEventList(map[string]any{})
	if err != nil || list != nil {
		t.Fatalf("empty: %v %v", list, err)
	}

	ilist, err := cloudtrailInjectInsightsEventList(map[string]any{
		"Events": []any{map[string]any{"eventName": "Insight"}, 1},
	})
	if err == nil {
		t.Fatalf("insights bad entry: %v", ilist)
	}
	ilist, err = cloudtrailInjectInsightsEventList(map[string]any{
		"Event": map[string]any{"eventName": "I"},
	})
	if err != nil || len(ilist) != 1 {
		t.Fatalf("insights Event: %v %v", ilist, err)
	}

	if cloudtrailInjectBoolPtr(true) == nil || !*cloudtrailInjectBoolPtr(true) {
		t.Fatal("bool true")
	}
	if cloudtrailInjectBoolPtr("TRUE") == nil || !*cloudtrailInjectBoolPtr("TRUE") {
		t.Fatal("string true")
	}
	if cloudtrailInjectBoolPtr(1) != nil {
		t.Fatal("non-bool")
	}
	if cloudtrailInjectString(map[string]any{"A": " x "}, "A") != "x" {
		t.Fatal("inject string")
	}
	res := cloudtrailInjectResources(map[string]any{
		"resources": []any{
			map[string]any{"ARN": "arn:aws:s3:::b", "type": "AWS::S3::Bucket", "accountId": "1"},
			map[string]any{"name": "skip"},
			"skip",
		},
	})
	if len(res) != 1 || res[0].ARN == "" {
		t.Fatalf("resources=%#v", res)
	}

	long := strings.Repeat("z", cloudTrailInjectMaxStrLen+10)
	redacted := redactCloudTrailInjectMap(map[string]any{
		"password": "secret",
		"token":    "t",
		"nested": map[string]any{
			"accessKeyId": "AKIA",
			"ok":          "v",
			"arr":         []any{long, map[string]any{"secretValue": "x"}},
		},
	}, cloudTrailInjectMaxDepth)
	if redacted["password"] != "[REDACTED]" {
		t.Fatalf("password redact: %#v", redacted["password"])
	}
	if redactCloudTrailInjectMap(nil, 1) != nil {
		t.Fatal("nil map")
	}
	if redactCloudTrailInjectMap(map[string]any{"a": 1}, 0)["_redacted"] != "depth" {
		t.Fatal("depth zero")
	}
	big := map[string]any{}
	for i := 0; i < cloudTrailInjectMaxEntries+5; i++ {
		big["k"+strconv.Itoa(i)] = i
	}
	trunc := redactCloudTrailInjectMap(big, 2)
	if trunc["_truncated"] != true {
		t.Fatalf("expected truncation keys=%d", len(trunc))
	}

	now := time.Date(2026, 7, 20, 15, 0, 0, 0, time.UTC)
	ev, err := buildInjectedAuditEvent(map[string]any{
		"eventName":   "ConsoleLogin",
		"eventSource": "signin.amazonaws.com",
		"eventTime":   "2026-07-20T15:00:00Z",
		"readOnly":    "true",
		"userIdentity": map[string]any{
			"type":     "IAMUser",
			"userName": "alice",
		},
		"sessionContext":     map[string]any{"mfa": true},
		"requestParameters":  map[string]any{"password": "x", "bucket": "b"},
		"responseElements":   map[string]any{"ok": true},
		"resources":          []any{map[string]any{"arn": "arn:aws:s3:::b", "Type": "AWS::S3::Bucket"}},
		"managementEvent":    true,
		"sourceIPAddress":    "198.51.100.1",
		"errorCode":          "AccessDenied",
		"errorMessage":       "nope",
		"eventCategory":      "Management",
		"awsRegion":          "us-west-2",
	}, verified, "req-1", "us-east-1", now)
	if err != nil {
		t.Fatal(err)
	}
	if ev.EventName != "ConsoleLogin" || ev.AWSRegion != "us-west-2" || !ev.ReadOnly {
		t.Fatalf("event=%#v", ev)
	}
	if ev.RequestParameters["password"] != "[REDACTED]" {
		t.Fatalf("req redact=%#v", ev.RequestParameters)
	}
	if ev.ManagementEvent == nil || !*ev.ManagementEvent {
		t.Fatal("managementEvent")
	}
	if _, err := buildInjectedAuditEvent(map[string]any{"eventName": "X"}, verified, "r", "us-east-1", now); err == nil {
		t.Fatal("missing source want error")
	}
	if _, err := buildInjectedAuditEvent(map[string]any{
		"eventName": "X", "eventSource": "s3.amazonaws.com", "eventTime": "not-a-time",
	}, verified, "r", "us-east-1", now); err == nil {
		t.Fatal("bad time want error")
	}
	iev, err := buildInjectedInsightAuditEvent(map[string]any{
		"insightDetails": map[string]any{"state": "Start", "eventSource": "s3.amazonaws.com"},
		"eventName":      "Insight",
		"eventTime":      "2026-07-20T15:00:00Z",
	}, verified, "req-i", "us-east-1", now)
	if err != nil {
		t.Fatal(err)
	}
	if iev.EventName != "Insight" {
		t.Fatalf("insight event=%#v", iev)
	}
	if _, err := buildInjectedInsightAuditEvent(map[string]any{}, verified, "r", "us-east-1", now); err == nil {
		t.Fatal("missing insightDetails")
	}
}

func TestDecodeRDSBlobAndNumericHelpers(t *testing.T) {
	t.Parallel()
	raw, err := decodeRDSDataBlob(base64.StdEncoding.EncodeToString([]byte("hello")))
	if err != nil || string(raw) != "hello" {
		t.Fatalf("decode: %q %v", raw, err)
	}
	if _, err := decodeRDSDataBlob("!!!"); err == nil {
		t.Fatal("bad b64")
	}
	if n, ok := asInt64(float64(3)); !ok || n != 3 {
		t.Fatal("float64")
	}
	if n, ok := asInt64(int64(4)); !ok || n != 4 {
		t.Fatal("int64")
	}
	if n, ok := asInt64(5); !ok || n != 5 {
		t.Fatal("int")
	}
	if n, ok := asInt64(json.Number("6")); !ok || n != 6 {
		t.Fatal("json.Number")
	}
	if _, ok := asInt64("x"); ok {
		t.Fatal("bad")
	}
	if n, ok := asFloat64(float64(1.5)); !ok || n != 1.5 {
		t.Fatal("f64")
	}
	if n, ok := asFloat64(int64(2)); !ok || n != 2 {
		t.Fatal("i64")
	}
	if n, ok := asFloat64(3); !ok || n != 3 {
		t.Fatal("int")
	}
	if _, ok := asFloat64(true); ok {
		t.Fatal("bool")
	}
}

func TestDuckResultToAthenaAndAuditSession(t *testing.T) {
	t.Parallel()
	cols := duckResultToAthena(compute.DuckQueryResult{Columns: []string{"a", "b"}, Rows: nil})
	if len(cols) != 2 || cols[0].Name != "a" || cols[0].Type != "varchar" {
		t.Fatalf("%#v", cols)
	}
	if jsonString(`say "hi"`) == "" {
		t.Fatal("jsonString empty")
	}
	opt := WithAuditSessionContext(nil)
	ev := &audit.Event{UserIdentity: map[string]any{"type": "IAMUser"}}
	opt(ev)
	opt = WithAuditSessionContext(map[string]any{"mfaAuthenticated": true})
	opt(ev)
	uid := ev.UserIdentity.(map[string]any)
	sc := uid["sessionContext"].(map[string]any)
	if sc["mfaAuthenticated"] != true {
		t.Fatalf("%#v", sc)
	}
	opt = WithAuditSessionContext(map[string]any{"source": "lab"})
	opt(ev)
	if b := auditBoolPtr(true); b == nil || !*b {
		t.Fatal("auditBoolPtr")
	}
}

func TestSQSEndpointHostHelper(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:4566/", nil)
	req.Host = "127.0.0.1:4566"
	if got := sqsEndpointHost(req); !strings.HasPrefix(got, "http://") {
		t.Fatalf("%q", got)
	}
	req = httptest.NewRequest(http.MethodPost, "http://example/", nil)
	req.Host = ""
	if got := sqsEndpointHost(req); !strings.Contains(got, "127.0.0.1:4566") {
		t.Fatalf("%q", got)
	}
}

func TestCreateBucketObjectLockEnabledHelper(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodPut, "http://127.0.0.1:4566/b", nil)
	req.Header.Set("x-amz-bucket-object-lock-enabled", "true")
	if !createBucketObjectLockEnabled(req, nil) {
		t.Fatal("header true")
	}
	req = httptest.NewRequest(http.MethodPut, "http://127.0.0.1:4566/b", nil)
	body := []byte(`<CreateBucketConfiguration><ObjectLockEnabledForBucket>true</ObjectLockEnabledForBucket></CreateBucketConfiguration>`)
	if !createBucketObjectLockEnabled(req, body) {
		t.Fatal("xml enabled")
	}
	req = httptest.NewRequest(http.MethodPut, "http://127.0.0.1:4566/b", nil)
	if createBucketObjectLockEnabled(req, []byte(`<CreateBucketConfiguration></CreateBucketConfiguration>`)) {
		t.Fatal("should be false")
	}
	if s3BypassGovernanceRetention(nil) {
		t.Fatal("nil request")
	}
	req = httptest.NewRequest(http.MethodDelete, "http://127.0.0.1:4566/b/k", nil)
	req.Header.Set("x-amz-bypass-governance-retention", "true")
	if !s3BypassGovernanceRetention(req) {
		t.Fatal("bypass header")
	}
}
