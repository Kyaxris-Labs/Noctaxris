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

func mustSESV2REST(t *testing.T, handler http.Handler, method, path string, payload map[string]any, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	var raw []byte
	if payload != nil {
		var err error
		raw, err = json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
	} else {
		raw = []byte{}
	}
	req := mustNewRequest(t, method, "http://127.0.0.1:4566"+path, raw)
	req.Header.Set("Content-Type", "application/json")
	signHeader(t, req, raw, testAccessKey, testSecret, testRegion, "ses", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestSESV2MinimalREST(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustSESV2REST(t, handler, http.MethodPost, "/v2/email/identities", map[string]any{
		"EmailIdentity": "v2sender@example.com",
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateEmailIdentity status=%d body=%q", create.Code, create.Body.String())
	}
	if !strings.Contains(create.Body.String(), "EMAIL_ADDRESS") {
		t.Fatalf("unexpected create body %q", create.Body.String())
	}

	dup := mustSESV2REST(t, handler, http.MethodPost, "/v2/email/identities", map[string]any{
		"EmailIdentity": "v2sender@example.com",
	}, now)
	if dup.Code != http.StatusBadRequest || !strings.Contains(dup.Body.String(), "AlreadyExistsException") {
		t.Fatalf("duplicate create status=%d body=%q", dup.Code, dup.Body.String())
	}

	send := mustSESV2REST(t, handler, http.MethodPost, "/v2/email/outbound-emails", map[string]any{
		"FromEmailAddress": "v2sender@example.com",
		"Destination": map[string]any{
			"ToAddresses": []string{"recipient@example.com"},
		},
		"Content": map[string]any{
			"Simple": map[string]any{
				"Subject": map[string]any{"Data": "V2 Test Subject"},
				"Body": map[string]any{
					"Text": map[string]any{"Data": "Hello from SES V2!"},
				},
			},
		},
	}, now)
	if send.Code != http.StatusOK || !strings.Contains(send.Body.String(), "MessageId") {
		t.Fatalf("SendEmail status=%d body=%q", send.Code, send.Body.String())
	}

	list := mustSESV2REST(t, handler, http.MethodGet, "/v2/email/identities", nil, now)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "v2sender@example.com") {
		t.Fatalf("ListEmailIdentities status=%d body=%q", list.Code, list.Body.String())
	}

	encoded := url.PathEscape("v2sender@example.com")
	get := mustSESV2REST(t, handler, http.MethodGet, "/v2/email/identities/"+encoded, nil, now)
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), "VerifiedForSendingStatus") {
		t.Fatalf("GetEmailIdentity status=%d body=%q", get.Code, get.Body.String())
	}

	acct := mustSESV2REST(t, handler, http.MethodGet, "/v2/email/account", nil, now)
	if acct.Code != http.StatusOK || !strings.Contains(acct.Body.String(), "SendQuota") {
		t.Fatalf("GetAccount status=%d body=%q", acct.Code, acct.Body.String())
	}

	del := mustSESV2REST(t, handler, http.MethodDelete, "/v2/email/identities/"+encoded, nil, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteEmailIdentity status=%d body=%q", del.Code, del.Body.String())
	}

	missing := mustSESV2REST(t, handler, http.MethodGet, "/v2/email/identities/"+encoded, nil, now)
	if missing.Code != http.StatusNotFound || !strings.Contains(missing.Body.String(), "NotFoundException") {
		t.Fatalf("Get missing identity status=%d body=%q", missing.Code, missing.Body.String())
	}
}

func TestSESV2SharesStoreWithV1(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	verify := mustSESQuery(t, handler,
		"Action=VerifyEmailIdentity&Version=2010-12-01&EmailAddress="+url.QueryEscape("shared@example.com"),
		now)
	if verify.Code != http.StatusOK {
		t.Fatalf("v1 verify status=%d", verify.Code)
	}

	list := mustSESV2REST(t, handler, http.MethodGet, "/v2/email/identities", nil, now)
	if !strings.Contains(list.Body.String(), "shared@example.com") {
		t.Fatalf("v2 list missing v1 identity: %q", list.Body.String())
	}
}
