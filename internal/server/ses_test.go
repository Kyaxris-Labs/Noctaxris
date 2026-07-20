package server_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func mustSESQuery(t *testing.T, handler http.Handler, body string, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(body))
	signHeader(t, req, []byte(body), testAccessKey, testSecret, testRegion, "ses", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestSESCatcherRoundTrip(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	verify := mustSESQuery(t, handler,
		"Action=VerifyEmailIdentity&Version=2010-12-01&EmailAddress="+url.QueryEscape("lab@example.com"),
		now)
	if verify.Code != http.StatusOK {
		t.Fatalf("VerifyEmailIdentity status=%d body=%q", verify.Code, verify.Body.String())
	}

	sendBody := strings.Join([]string{
		"Action=SendEmail",
		"Version=2010-12-01",
		"Source=" + url.QueryEscape("lab@example.com"),
		"Destination.ToAddresses.member.1=" + url.QueryEscape("to@example.com"),
		"Message.Subject.Data=" + url.QueryEscape("hello"),
		"Message.Body.Text.Data=" + url.QueryEscape("caught"),
	}, "&")
	send := mustSESQuery(t, handler, sendBody, now)
	if send.Code != http.StatusOK {
		t.Fatalf("SendEmail status=%d body=%q", send.Code, send.Body.String())
	}
	if !strings.Contains(send.Body.String(), "MessageId") {
		t.Fatalf("missing MessageId in %q", send.Body.String())
	}

	list := mustSESQuery(t, handler, "Action=ListIdentities&Version=2010-12-01", now)
	if list.Code != http.StatusOK {
		t.Fatalf("ListIdentities status=%d body=%q", list.Code, list.Body.String())
	}
	if !strings.Contains(list.Body.String(), "lab@example.com") {
		t.Fatalf("identity missing in %q", list.Body.String())
	}

	stats := mustSESQuery(t, handler, "Action=GetSendStatistics&Version=2010-12-01", now)
	if stats.Code != http.StatusOK {
		t.Fatalf("GetSendStatistics status=%d body=%q", stats.Code, stats.Body.String())
	}
}
