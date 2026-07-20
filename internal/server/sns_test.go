package server_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func mustSNSQuery(
	t *testing.T,
	handler http.Handler,
	body string,
	akid, secret string,
	now time.Time,
) *httptest.ResponseRecorder {
	t.Helper()
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(body))
	signHeader(t, req, []byte(body), akid, secret, testRegion, "sns", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func snsCreateTopic(t *testing.T, handler http.Handler, name string, now time.Time) string {
	t.Helper()
	rec := mustSNSQuery(t, handler,
		"Action=CreateTopic&Version=2010-03-31&Name="+url.QueryEscape(name),
		testAccessKey, testSecret, now)
	if rec.Code != http.StatusOK {
		t.Fatalf("CreateTopic status=%d body=%q", rec.Code, rec.Body.String())
	}
	arn := xmlTag(t, rec.Body.String(), "TopicArn")
	if arn == "" {
		t.Fatalf("missing TopicArn in %q", rec.Body.String())
	}
	return arn
}

func TestSNSCreateTopic(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	arn := snsCreateTopic(t, handler, "alerts", now)
	want := "arn:aws:sns:us-east-1:000000000001:alerts"
	if arn != want {
		t.Fatalf("TopicArn=%q want %q", arn, want)
	}
}

func TestSNSPublishNotFound(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	missingARN := "arn:aws:sns:us-east-1:000000000001:missing"
	rec := mustSNSQuery(t, handler,
		"Action=Publish&Version=2010-03-31&TopicArn="+url.QueryEscape(missingARN)+
			"&Message="+url.QueryEscape("hello"),
		testAccessKey, testSecret, now)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("Publish status=%d want 404 body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "NotFound") {
		t.Fatalf("expected NotFound in %q", rec.Body.String())
	}
}

func TestSNSIdentityAllowPublish(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	topicARN := snsCreateTopic(t, handler, "identity-topic", now)

	_, userARN, err := st.CreateUser(testAccountID, "sns-publisher")
	if err != nil {
		t.Fatal(err)
	}
	userAKID, userSecret, err := st.CreateUserAccessKey(testAccountID, "sns-publisher")
	if err != nil {
		t.Fatal(err)
	}
	allowPublish := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"sns:Publish","Resource":"*"}]}`
	if err := st.PutInlinePolicy(userARN, "publish", allowPublish); err != nil {
		t.Fatal(err)
	}

	rec := mustSNSQuery(t, handler,
		"Action=Publish&Version=2010-03-31&TopicArn="+url.QueryEscape(topicARN)+
			"&Message="+url.QueryEscape("via-identity"),
		userAKID, userSecret, now)
	if rec.Code != http.StatusOK {
		t.Fatalf("Publish status=%d want 200 body=%q", rec.Code, rec.Body.String())
	}
	if xmlTag(t, rec.Body.String(), "MessageId") == "" {
		t.Fatalf("missing MessageId in %q", rec.Body.String())
	}
}

func TestSNSTopicPolicyAloneAllowPublish(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	topicARN := snsCreateTopic(t, handler, "policy-topic", now)

	_, guestARN, err := st.CreateUser(testAccountID, "sns-guest")
	if err != nil {
		t.Fatal(err)
	}
	guestAKID, guestSecret, err := st.CreateUserAccessKey(testAccountID, "sns-guest")
	if err != nil {
		t.Fatal(err)
	}

	allowGuest := fmt.Sprintf(
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"%s"},"Action":"sns:Publish","Resource":"*"}]}`,
		guestARN,
	)
	setRec := mustSNSQuery(t, handler,
		"Action=SetTopicAttributes&Version=2010-03-31&TopicArn="+url.QueryEscape(topicARN)+
			"&AttributeName=Policy&AttributeValue="+url.QueryEscape(allowGuest),
		testAccessKey, testSecret, now)
	if setRec.Code != http.StatusOK {
		t.Fatalf("SetTopicAttributes status=%d body=%q", setRec.Code, setRec.Body.String())
	}

	rec := mustSNSQuery(t, handler,
		"Action=Publish&Version=2010-03-31&TopicArn="+url.QueryEscape(topicARN)+
			"&Message="+url.QueryEscape("via-topic-policy"),
		guestAKID, guestSecret, now)
	if rec.Code != http.StatusOK {
		t.Fatalf("Publish via topic policy status=%d want 200 body=%q", rec.Code, rec.Body.String())
	}
	if xmlTag(t, rec.Body.String(), "MessageId") == "" {
		t.Fatalf("missing MessageId in %q", rec.Body.String())
	}
}
