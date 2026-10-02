package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestKMSCreateGrantWithConstraintsAndNegatives(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustKMSJSON(t, handler, "CreateKey", map[string]any{}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateKey %d %s", create.Code, create.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(create.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	meta, _ := out["KeyMetadata"].(map[string]any)
	keyID, _ := meta["KeyId"].(string)

	mustCreateIAMRole(t, handler, "kms-grantee", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"*"},"Action":"sts:AssumeRole"}]}`, now)
	grantee := "arn:aws:iam::" + testAccountID + ":role/kms-grantee"

	okGrant := mustKMSJSON(t, handler, "CreateGrant", map[string]any{
		"KeyId":            keyID,
		"GranteePrincipal": grantee,
		"Operations":       []string{"Decrypt", "Encrypt"},
		"Constraints": map[string]any{
			"EncryptionContextEquals": map[string]string{"dept": "lab"},
			"EncryptionContextSubset": map[string]any{"env": "dev"},
		},
	}, now)
	if okGrant.Code != http.StatusOK || !strings.Contains(okGrant.Body.String(), "GrantId") {
		t.Fatalf("CreateGrant %d %s", okGrant.Code, okGrant.Body.String())
	}

	badConstraint := mustKMSJSON(t, handler, "CreateGrant", map[string]any{
		"KeyId":            keyID,
		"GranteePrincipal": grantee,
		"Operations":       []string{"Decrypt"},
		"Constraints":      map[string]any{"NotAField": map[string]string{"a": "b"}},
	}, now)
	if badConstraint.Code == http.StatusOK {
		t.Fatalf("bad constraints should fail: %s", badConstraint.Body.String())
	}
	badType := mustKMSJSON(t, handler, "CreateGrant", map[string]any{
		"KeyId":            keyID,
		"GranteePrincipal": grantee,
		"Operations":       []string{"Decrypt"},
		"Constraints":      "nope",
	}, now)
	if badType.Code == http.StatusOK {
		t.Fatalf("constraints type should fail: %s", badType.Body.String())
	}
}

func TestCURModifyReportDefinitionPositiveNegative(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	_ = mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/cur-mod-bucket", nil, "s3", now, nil)
	put := mustJSONTarget(t, handler, "AWSOrigamiServiceGatewayService.PutReportDefinition", "cur", map[string]any{
		"ReportDefinition": map[string]any{
			"ReportName":               "lab-cur-mod",
			"TimeUnit":                 "MONTHLY",
			"Format":                   "textORcsv",
			"Compression":              "GZIP",
			"S3Bucket":                 "cur-mod-bucket",
			"S3Prefix":                 "reports",
			"S3Region":                 "us-east-1",
			"AdditionalSchemaElements": []any{"RESOURCES"},
			"ReportVersioning":         "OVERWRITE_REPORT",
		},
	}, now)
	if put.Code != http.StatusOK {
		t.Fatalf("Put %d %s", put.Code, put.Body.String())
	}

	mod := mustJSONTarget(t, handler, "AWSOrigamiServiceGatewayService.ModifyReportDefinition", "cur", map[string]any{
		"ReportName": "lab-cur-mod",
		"ReportDefinition": map[string]any{
			"ReportName":               "lab-cur-mod",
			"TimeUnit":                 "DAILY",
			"Format":                   "textORcsv",
			"Compression":              "GZIP",
			"S3Bucket":                 "cur-mod-bucket",
			"S3Prefix":                 "reports-v2",
			"S3Region":                 "us-east-1",
			"AdditionalSchemaElements": []any{"RESOURCES"},
			"AdditionalArtifacts":      []any{"ATHENA"},
			"ReportVersioning":         "OVERWRITE_REPORT",
			"RefreshClosedReports":     true,
		},
	}, now)
	if mod.Code != http.StatusOK {
		t.Fatalf("Modify %d %s", mod.Code, mod.Body.String())
	}
	miss := mustJSONTarget(t, handler, "AWSOrigamiServiceGatewayService.ModifyReportDefinition", "cur", map[string]any{
		"ReportName": "missing-cur",
		"ReportDefinition": map[string]any{
			"ReportName": "missing-cur",
			"TimeUnit":   "DAILY",
			"Format":     "textORcsv",
			"Compression": "GZIP",
			"S3Bucket":   "cur-mod-bucket",
			"S3Prefix":   "x",
			"S3Region":   "us-east-1",
			"AdditionalSchemaElements": []any{"RESOURCES"},
			"ReportVersioning":         "OVERWRITE_REPORT",
		},
	}, now)
	if miss.Code == http.StatusOK {
		t.Fatalf("missing modify should fail: %s", miss.Body.String())
	}
	emptyPut := mustJSONTarget(t, handler, "AWSOrigamiServiceGatewayService.PutReportDefinition", "cur", map[string]any{}, now)
	if emptyPut.Code == http.StatusOK {
		t.Fatalf("empty put should fail: %s", emptyPut.Body.String())
	}
}
