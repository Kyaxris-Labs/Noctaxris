package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestACMDescribeCertificateRoundTrip(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	empty := mustJSONTarget(t, handler, "CertificateManager.ListCertificates", "acm", map[string]any{}, now)
	if empty.Code != http.StatusOK {
		t.Fatalf("empty ListCertificates status=%d body=%q", empty.Code, empty.Body.String())
	}
	if !strings.Contains(empty.Body.String(), "CertificateSummaryList") {
		t.Fatalf("expected CertificateSummaryList in empty list body=%q", empty.Body.String())
	}

	req := mustJSONTarget(t, handler, "CertificateManager.RequestCertificate", "acm", map[string]any{
		"DomainName": "describe.example.com",
	}, now)
	if req.Code != http.StatusOK {
		t.Fatalf("RequestCertificate status=%d body=%q", req.Code, req.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(req.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	arn, _ := created["CertificateArn"].(string)
	if arn == "" {
		t.Fatalf("missing arn: %s", req.Body.String())
	}

	desc := mustJSONTarget(t, handler, "CertificateManager.DescribeCertificate", "acm", map[string]any{
		"CertificateArn": arn,
	}, now)
	if desc.Code != http.StatusOK {
		t.Fatalf("DescribeCertificate status=%d body=%q", desc.Code, desc.Body.String())
	}
	if !strings.Contains(desc.Body.String(), "describe.example.com") {
		t.Fatalf("DescribeCertificate missing domain body=%q", desc.Body.String())
	}

	del := mustJSONTarget(t, handler, "CertificateManager.DeleteCertificate", "acm", map[string]any{
		"CertificateArn": arn,
	}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteCertificate status=%d body=%q", del.Code, del.Body.String())
	}

	gone := mustJSONTarget(t, handler, "CertificateManager.DescribeCertificate", "acm", map[string]any{
		"CertificateArn": arn,
	}, now)
	if gone.Code != http.StatusBadRequest || !strings.Contains(gone.Body.String(), "ResourceNotFoundException") {
		t.Fatalf("DescribeCertificate missing want ResourceNotFound status=%d body=%q", gone.Code, gone.Body.String())
	}

	delGone := mustJSONTarget(t, handler, "CertificateManager.DeleteCertificate", "acm", map[string]any{
		"CertificateArn": arn,
	}, now)
	if delGone.Code != http.StatusBadRequest || !strings.Contains(delGone.Body.String(), "ResourceNotFoundException") {
		t.Fatalf("DeleteCertificate missing want ResourceNotFound status=%d body=%q", delGone.Code, delGone.Body.String())
	}
}

func TestACMRequestCertificateBadDomain(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	rec := mustJSONTarget(t, handler, "CertificateManager.RequestCertificate", "acm", map[string]any{
		"DomainName": "",
	}, now)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "InvalidParameterException") {
		t.Fatalf("empty DomainName want InvalidParameter status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestACMUnknownActionError(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	rec := mustJSONTarget(t, handler, "CertificateManager.ImportCertificate", "acm", map[string]any{}, now)
	if rec.Code != http.StatusNotImplemented || !strings.Contains(rec.Body.String(), "InvalidAction") {
		t.Fatalf("unknown ACM action want InvalidAction status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestACMAccessDeniedWithoutPolicy(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	_, userARN, err := st.CreateUser(testAccountID, "acm-deny")
	if err != nil {
		t.Fatal(err)
	}
	akid, secret, err := st.CreateUserAccessKey(testAccountID, "acm-deny")
	if err != nil {
		t.Fatal(err)
	}
	deny := `{"Version":"2012-10-17","Statement":[{"Effect":"Deny","Action":"acm:*","Resource":"*"}]}`
	if err := st.PutInlinePolicy(userARN, "noacm", deny); err != nil {
		t.Fatal(err)
	}

	raw, err := json.Marshal(map[string]any{"DomainName": "denied.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "CertificateManager.RequestCertificate")
	signHeader(t, req, raw, akid, secret, testRegion, "acm", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "AccessDeniedException") {
		t.Fatalf("denied RequestCertificate want 403 AccessDenied status=%d body=%q", rec.Code, rec.Body.String())
	}
}
