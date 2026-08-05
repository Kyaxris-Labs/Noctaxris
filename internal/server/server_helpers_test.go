package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/identity"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestAuditEventSourceFromQueryAction(t *testing.T) {
	t.Parallel()
	if auditEventSourceFromQueryAction("") != "" {
		t.Fatal("empty")
	}
	cases := map[string]string{
		"AssumeRole":                       "sts.amazonaws.com",
		"GetCallerIdentity":                "sts.amazonaws.com",
		"GetSessionToken":                  "sts.amazonaws.com",
		"GetFederationToken":               "sts.amazonaws.com",
		"CreateAccount":                    "organizations.amazonaws.com",
		"ListAccounts":                     "organizations.amazonaws.com",
		"CreateOrganizationalUnit":         "organizations.amazonaws.com",
		"EnablePolicyType":                 "organizations.amazonaws.com",
		"CreatePolicy":                     "organizations.amazonaws.com",
		"AttachPolicy":                     "organizations.amazonaws.com",
		"DetachPolicy":                     "organizations.amazonaws.com",
		"DescribePolicy":                   "organizations.amazonaws.com",
		"MoveAccount":                      "organizations.amazonaws.com",
		"ListOrganizationalUnitsForParent": "organizations.amazonaws.com",
		"ListBuckets":                      "s3.amazonaws.com",
		"CreateBucket":                     "s3.amazonaws.com",
		"PutObject":                        "s3.amazonaws.com",
		"GetObject":                        "s3.amazonaws.com",
		"DeleteObject":                     "s3.amazonaws.com",
		"HeadBucket":                       "s3.amazonaws.com",
		"CreateUser":                       "iam.amazonaws.com",
		"ListUsers":                        "iam.amazonaws.com",
		"CreateAccessKey":                  "iam.amazonaws.com",
		"CreateRole":                       "iam.amazonaws.com",
		"AttachRolePolicy":                 "iam.amazonaws.com",
		"PutRolePolicy":                    "iam.amazonaws.com",
		"UnknownAction":                    "",
	}
	for in, want := range cases {
		if got := auditEventSourceFromQueryAction(in); got != want {
			t.Fatalf("%q got %q want %q", in, got, want)
		}
	}
}

func TestUnauthenticatedActionHelpers(t *testing.T) {
	t.Parallel()
	if !isUnauthenticatedSTSAction("AssumeRoleWithSAML") {
		t.Fatal("saml")
	}
	if !isUnauthenticatedSTSAction("AssumeRoleWithWebIdentity") {
		t.Fatal("webid")
	}
	if isUnauthenticatedSTSAction("AssumeRole") {
		t.Fatal("assume role should require auth")
	}
	if !isUnauthenticatedCognitoAction("InitiateAuth") {
		t.Fatal("initiate")
	}
	if !isUnauthenticatedCognitoAction("ConfirmForgotPassword") {
		t.Fatal("confirm forgot")
	}
	if !isUnauthenticatedCognitoAction("RevokeToken") {
		t.Fatal("revoke")
	}
	if isUnauthenticatedCognitoAction("AdminCreateUser") {
		t.Fatal("admin create requires auth")
	}
}

func TestLayerVersionAndAliasParams(t *testing.T) {
	t.Parallel()
	n, err := layerVersionNumberParam(map[string]any{"VersionNumber": "3"})
	if err != nil || n != 3 {
		t.Fatalf("string %d %v", n, err)
	}
	n, err = layerVersionNumberParam(map[string]any{"VersionNumber": float64(2)})
	if err != nil || n != 2 {
		t.Fatalf("float %d %v", n, err)
	}
	n, err = layerVersionNumberParam(map[string]any{"VersionNumber": 4})
	if err != nil || n != 4 {
		t.Fatalf("int %d %v", n, err)
	}
	if _, err := layerVersionNumberParam(map[string]any{"VersionNumber": "0"}); err == nil {
		t.Fatal("zero string")
	}
	if _, err := layerVersionNumberParam(map[string]any{"VersionNumber": float64(-1)}); err == nil {
		t.Fatal("neg float")
	}
	if _, err := layerVersionNumberParam(map[string]any{"VersionNumber": 0}); err == nil {
		t.Fatal("zero int")
	}
	if _, err := layerVersionNumberParam(map[string]any{}); err == nil {
		t.Fatal("missing")
	}
	if layerNameParam(map[string]any{"LayerName": "  L  "}) != "L" {
		t.Fatal("layer name")
	}

	av, err := lambdaAliasVersionParam(map[string]any{"FunctionVersion": "1"})
	if err != nil || av != 1 {
		t.Fatalf("alias ver %d %v", av, err)
	}
	av, err = lambdaAliasVersionParam(map[string]any{"FunctionVersion": float64(2)})
	if err != nil || av != 2 {
		t.Fatalf("alias float %d %v", av, err)
	}
	av, err = lambdaAliasVersionParam(map[string]any{"FunctionVersion": 3})
	if err != nil || av != 3 {
		t.Fatalf("alias int %d %v", av, err)
	}
	if _, err := lambdaAliasVersionParam(map[string]any{}); err == nil {
		t.Fatal("missing alias ver")
	}
}

func TestLambdaPackageAndImageHelpers(t *testing.T) {
	t.Parallel()
	if lambdaPackageTypeFromParams(map[string]any{"PackageType": "Image"}) != store.LambdaPackageTypeImage {
		t.Fatal("image")
	}
	if lambdaPackageTypeFromParams(nil) != store.LambdaPackageTypeZip {
		t.Fatal("default zip")
	}
	uri, err := lambdaImageURIFromCode(map[string]any{
		"Code": map[string]any{"ImageUri": " 111.dkr.ecr.us-east-1.amazonaws.com/repo:tag "},
	})
	if err != nil || !strings.Contains(uri, "repo:tag") {
		t.Fatalf("uri %q %v", uri, err)
	}
	uri, err = lambdaImageURIFromCode(map[string]any{"ImageUri": "direct"})
	if err != nil || uri != "direct" {
		t.Fatalf("top-level %q %v", uri, err)
	}
	if _, err := lambdaImageURIFromCode(nil); err == nil {
		t.Fatal("missing image")
	}
	got := lambdaLayersFromParams(map[string]any{"Layers": []string{"a", " ", "b"}})
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("string slice layers %#v", got)
	}
	if lambdaLayersFromParams(map[string]any{"Layers": 42}) != nil {
		t.Fatal("bad layers type")
	}
}

func TestSimpleErrorWriters(t *testing.T) {
	s, _ := accessKeyPlumbServer(t)
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/", nil)
	verified := &authn.Verified{
		AccountID:   "000000000001",
		AccessKeyID: "AKIAROOTEXAMPLE01",
		Region:      "us-east-1",
		Principal:   identity.RootPrincipal("000000000001", "AKIAROOTEXAMPLE01"),
	}

	rec := httptest.NewRecorder()
	s.writeEMRError(rec, "rid", 400, "ValidationException", "bad")
	if rec.Code != 400 || rec.Body.Len() == 0 {
		t.Fatalf("emr %d %q", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	s.writeMemoryDBError(rec, req, nil, "rid", 400, "ValidationException", "bad", false, "eid", verified)
	if rec.Code != 400 || rec.Body.Len() == 0 {
		t.Fatalf("memorydb %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	s.writeOpenSearchError(rec, req, nil, "rid", 400, "ValidationException", "bad", false, "eid", verified)
	if rec.Code != 400 || rec.Body.Len() == 0 {
		t.Fatalf("opensearch %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	s.writeElastiCacheError(rec, req, "rid", 400, "ValidationException", "bad", false, "eid", verified)
	if rec.Code != 400 || rec.Body.Len() == 0 {
		t.Fatalf("elasticache %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	s.writeTranscribeError(rec, "rid", 400, "ValidationException", "bad")
	if rec.Code != 400 || rec.Body.Len() == 0 {
		t.Fatalf("transcribe %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	s.writeTextractError(rec, "rid", 400, "ValidationException", "bad")
	if rec.Code != 400 || rec.Body.Len() == 0 {
		t.Fatalf("textract %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	s.writeBedrockError(rec, "rid", 400, "ValidationException", "bad")
	if rec.Code != 400 || rec.Body.Len() == 0 {
		t.Fatalf("bedrock %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	s.writeSESV2Error(rec, "rid", 400, "ValidationException", "bad")
	if rec.Code != 400 || rec.Body.Len() == 0 {
		t.Fatalf("sesv2 %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	s.writeDynamoStreamsError(rec, req, nil, "rid", 400, "ValidationException", "bad", false, "eid", verified)
	if rec.Code != 400 || rec.Body.Len() == 0 {
		t.Fatalf("streams %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	s.writeNeptuneError(rec, req, "rid", 400, "ValidationException", "bad", false, "eid", verified)
	if rec.Code != 400 || rec.Body.Len() == 0 {
		t.Fatalf("neptune %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	s.writeOpenSearchLabError(rec, "rid", 400, "ValidationException", "bad")
	if rec.Code != 400 || rec.Body.Len() == 0 {
		t.Fatalf("os lab %d", rec.Code)
	}
}

func TestControlPlaneActionPredicates(t *testing.T) {
	t.Parallel()
	if !isNeptuneControlPlaneAction("CreateDBCluster") {
		t.Fatal("neptune create")
	}
	if !isNeptuneControlPlaneAction("DescribeDBClusters") {
		t.Fatal("neptune describe")
	}
	if isNeptuneControlPlaneAction("NoSuch") {
		t.Fatal("neptune unknown")
	}
	if !isDocDBControlPlaneAction("CreateDBCluster") {
		t.Fatal("docdb create")
	}
	_ = isDocDBControlPlaneAction("DescribeDBClusters")
	_ = isDocDBControlPlaneAction("DeleteDBCluster")
	_ = isDocDBControlPlaneAction("NoSuch")
	_ = isControlTowerAction("ListLandingZones", "controltower")
	_ = isControlTowerAction("GetLandingZone", "controltower")
	_ = isControlTowerAction("NoSuch", "controltower")
	_ = isOrgsDepthAction("ListAccounts", "organizations")
	_ = isOrgsDepthAction("CreateAccount", "organizations")
	_ = isOrgsDepthAction("NoSuch", "organizations")
	if !isAPIGatewayRESTAction("CreateRestApi") {
		t.Fatal("apigw rest")
	}
	_ = isAPIGatewayRESTAction("NoSuch")
}

func TestDefaultAuthnAndParseHelpers(t *testing.T) {
	t.Parallel()
	for _, code := range []string{
		authn.CodeMissingAuthenticationToken,
		authn.CodeInvalidClientTokenId,
		authn.CodeSignatureDoesNotMatch,
		authn.CodeRequestTimeTooSkewed,
		"Other",
	} {
		if defaultAuthnMessage(code) == "" {
			t.Fatalf("empty msg for %s", code)
		}
	}
	ak, ok := parseAccessKeyID("AWS4-HMAC-SHA256 Credential=AKIAEXAMPLE/20200101/us-east-1/s3/aws4_request")
	if !ok || ak != "AKIAEXAMPLE" {
		t.Fatalf("parse ak %q %v", ak, ok)
	}
	if _, ok := parseAccessKeyID("no-cred"); ok {
		t.Fatal("expected fail")
	}
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/", nil)
	req.Header.Set("X-Amz-Target", "DynamoDB_20120810.PutItem")
	if eventNameForRequest(req) != "PutItem" {
		t.Fatalf("target event %q", eventNameForRequest(req))
	}
	req2 := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/?Action=ListBuckets", nil)
	if eventNameForRequest(req2) != "ListBuckets" {
		t.Fatalf("query event %q", eventNameForRequest(req2))
	}
	req3 := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/foo", nil)
	if !strings.Contains(eventNameForRequest(req3), "GET") {
		t.Fatalf("method path event %q", eventNameForRequest(req3))
	}
	req.RemoteAddr = "203.0.113.9:1234"
	if peerClientIP(req) != "203.0.113.9" {
		t.Fatalf("peer %q", peerClientIP(req))
	}
	if clientIP(req) != "203.0.113.9" {
		t.Fatalf("clientIP %q", clientIP(req))
	}
	if federationRegion(req) != "us-east-1" {
		t.Fatal("federation region")
	}
	if newRequestID() == "" {
		t.Fatal("request id")
	}
}
