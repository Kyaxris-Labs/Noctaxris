package server_test

import (
	"encoding/base64"
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

func TestSESHandlersCoverageWave(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	topicARN := snsCreateTopic(t, handler, "ses-bounce", now)

	// Missing EmailAddress
	if rec := mustSESQuery(t, handler, "Action=VerifyEmailIdentity&Version=2010-12-01", now); rec.Code != http.StatusBadRequest {
		t.Fatalf("Verify empty want 400 got %d %s", rec.Code, rec.Body.String())
	}

	email := "cov-ses@example.com"
	if rec := mustSESQuery(t, handler,
		"Action=VerifyEmailIdentity&Version=2010-12-01&EmailAddress="+url.QueryEscape(email), now); rec.Code != http.StatusOK {
		t.Fatalf("VerifyEmailIdentity %d %s", rec.Code, rec.Body.String())
	}

	// SendEmail without Source
	if rec := mustSESQuery(t, handler, "Action=SendEmail&Version=2010-12-01", now); rec.Code != http.StatusBadRequest {
		t.Fatalf("SendEmail no source want 400 got %d", rec.Code)
	}

	// SendEmail unverified source
	unverified := strings.Join([]string{
		"Action=SendEmail", "Version=2010-12-01",
		"Source=" + url.QueryEscape("nope@example.com"),
		"Destination.ToAddresses.member.1=" + url.QueryEscape("to@example.com"),
		"Message.Subject.Data=hi",
		"Message.Body.Text.Data=body",
	}, "&")
	if rec := mustSESQuery(t, handler, unverified, now); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "MessageRejected") {
		t.Fatalf("unverified SendEmail want MessageRejected got %d %s", rec.Code, rec.Body.String())
	}

	sendOK := strings.Join([]string{
		"Action=SendEmail", "Version=2010-12-01",
		"Source=" + url.QueryEscape(email),
		"Destination.ToAddresses.member.1=" + url.QueryEscape("to@example.com"),
		"Destination.ToAddresses.member.2=" + url.QueryEscape("cc@example.com"),
		"Message.Subject.Data=" + url.QueryEscape("subj"),
		"Message.Body.Text.Data=" + url.QueryEscape("text"),
		"Message.Body.Html.Data=" + url.QueryEscape("<b>html</b>"),
	}, "&")
	if rec := mustSESQuery(t, handler, sendOK, now); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "MessageId") {
		t.Fatalf("SendEmail %d %s", rec.Code, rec.Body.String())
	}

	// SendRawEmail negatives + success
	if rec := mustSESQuery(t, handler, "Action=SendRawEmail&Version=2010-12-01&Source="+url.QueryEscape(email), now); rec.Code != http.StatusBadRequest {
		t.Fatalf("SendRawEmail missing raw want 400 got %d", rec.Code)
	}
	raw := base64.StdEncoding.EncodeToString([]byte("From: " + email + "\r\nTo: to@example.com\r\nSubject: raw\r\n\r\nbody"))
	rawBody := strings.Join([]string{
		"Action=SendRawEmail", "Version=2010-12-01",
		"Source=" + url.QueryEscape(email),
		"Destination.ToAddresses.member.1=" + url.QueryEscape("to@example.com"),
		"RawMessage.Data=" + url.QueryEscape(raw),
	}, "&")
	if rec := mustSESQuery(t, handler, rawBody, now); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "MessageId") {
		t.Fatalf("SendRawEmail %d %s", rec.Code, rec.Body.String())
	}

	// ListIdentities with IdentityType filter
	if rec := mustSESQuery(t, handler, "Action=ListIdentities&Version=2010-12-01&IdentityType=EmailAddress", now); rec.Code != http.StatusOK {
		t.Fatalf("ListIdentities %d %s", rec.Code, rec.Body.String())
	}

	if rec := mustSESQuery(t, handler, "Action=GetSendStatistics&Version=2010-12-01", now); rec.Code != http.StatusOK {
		t.Fatalf("GetSendStatistics %d %s", rec.Code, rec.Body.String())
	}

	// SetIdentityNotificationTopic negatives + success
	if rec := mustSESQuery(t, handler, "Action=SetIdentityNotificationTopic&Version=2010-12-01", now); rec.Code != http.StatusBadRequest {
		t.Fatalf("SetIdentityNotificationTopic empty want 400 got %d", rec.Code)
	}
	missingID := strings.Join([]string{
		"Action=SetIdentityNotificationTopic", "Version=2010-12-01",
		"Identity=" + url.QueryEscape("missing@example.com"),
		"NotificationType=Bounce",
		"SnsTopic=" + url.QueryEscape(topicARN),
	}, "&")
	if rec := mustSESQuery(t, handler, missingID, now); rec.Code != http.StatusBadRequest {
		t.Fatalf("SetIdentityNotificationTopic missing identity want 400 got %d %s", rec.Code, rec.Body.String())
	}
	setOK := strings.Join([]string{
		"Action=SetIdentityNotificationTopic", "Version=2010-12-01",
		"Identity=" + url.QueryEscape(email),
		"NotificationType=Bounce",
		"SnsTopic=" + url.QueryEscape(topicARN),
	}, "&")
	if rec := mustSESQuery(t, handler, setOK, now); rec.Code != http.StatusOK {
		t.Fatalf("SetIdentityNotificationTopic %d %s", rec.Code, rec.Body.String())
	}

	// Unimplemented action
	if rec := mustSESQuery(t, handler, "Action=DeleteIdentity&Version=2010-12-01", now); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "InvalidAction") {
		t.Fatalf("DeleteIdentity want InvalidAction got %d %s", rec.Code, rec.Body.String())
	}
}
