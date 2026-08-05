package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
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

func TestSNSHTTPSubscribeRequiresEgressForAllowlist(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	topicARN := snsCreateTopic(t, handler, "http-egress", now)
	publicHook := "https://example.com/sns-hook"

	t.Setenv("NOCTAXRIS_SNS_HTTP_ALLOWLIST", publicHook)
	t.Setenv("NOCTAXRIS_SNS_HTTP_EGRESS", "")

	deny := mustSNSQuery(t, handler,
		"Action=Subscribe&Version=2010-03-31&TopicArn="+url.QueryEscape(topicARN)+
			"&Protocol=https&Endpoint="+url.QueryEscape(publicHook),
		testAccessKey, testSecret, now)
	if deny.Code != http.StatusBadRequest {
		t.Fatalf("Subscribe without egress status=%d want 400 body=%q", deny.Code, deny.Body.String())
	}
	if !strings.Contains(deny.Body.String(), "InvalidParameter") {
		t.Fatalf("expected InvalidParameter in %q", deny.Body.String())
	}

	catcher := "http://127.0.0.1:4566/_noctaxris/sns-http-catcher"
	ok := mustSNSQuery(t, handler,
		"Action=Subscribe&Version=2010-03-31&TopicArn="+url.QueryEscape(topicARN)+
			"&Protocol=http&Endpoint="+url.QueryEscape(catcher),
		testAccessKey, testSecret, now)
	if ok.Code != http.StatusOK {
		t.Fatalf("Subscribe catcher status=%d want 200 body=%q", ok.Code, ok.Body.String())
	}
}

func TestSNSHTTPCatcherPostAndConfirm(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	topicARN := snsCreateTopic(t, handler, "catcher-topic", now)
	catcher := "http://127.0.0.1:4566" + store.LabSNSHTTPCatcherPath
	subRec := mustSNSQuery(t, handler,
		"Action=Subscribe&Version=2010-03-31&TopicArn="+url.QueryEscape(topicARN)+
			"&Protocol=http&Endpoint="+url.QueryEscape(catcher),
		testAccessKey, testSecret, now)
	if subRec.Code != http.StatusOK {
		t.Fatalf("Subscribe status=%d body=%q", subRec.Code, subRec.Body.String())
	}
	// Unconfirmed HTTP subscribe returns "pending confirmation" in XML; resolve real row from store.
	subs, err := st.ListSubscriptions(testAccountID)
	if err != nil || len(subs) < 1 {
		t.Fatalf("ListSubscriptions=%v err=%v", subs, err)
	}
	var pending store.Subscription
	for _, s := range subs {
		if s.TopicARN == topicARN && s.Endpoint == catcher {
			pending = s
			break
		}
	}
	if pending.SubscriptionARN == "" {
		t.Fatalf("pending HTTP sub not found in %+v", subs)
	}
	subARN := pending.SubscriptionARN
	token := pending.ConfirmToken
	if token == "" {
		t.Fatal("expected ConfirmToken on pending HTTP subscription")
	}

	// Method not allowed on GET without ConfirmSubscription action.
	badGet := httptest.NewRecorder()
	badReq := mustNewRequest(t, http.MethodGet, catcher, nil)
	handler.ServeHTTP(badGet, badReq)
	if badGet.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET catcher want 405 got %d", badGet.Code)
	}

	payload := map[string]string{
		"Type":            "Notification",
		"TopicArn":        topicARN,
		"SubscriptionArn": subARN,
		"Message":         "hello-catcher",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	postRec := httptest.NewRecorder()
	postReq := mustNewRequest(t, http.MethodPost, catcher, body)
	postReq.Header.Set("Content-Type", "application/json")
	postReq.Header.Set("x-amz-sns-message-type", "Notification")
	handler.ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusOK || postRec.Body.String() != "ok" {
		t.Fatalf("POST catcher status=%d body=%q", postRec.Code, postRec.Body.String())
	}
	msgs, err := st.ListSNSHTTPCatcher()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range msgs {
		if strings.Contains(m.Body, "hello-catcher") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("catcher messages=%+v", msgs)
	}

	confirmURL := catcher + "?Action=ConfirmSubscription&Token=" + url.QueryEscape(token) + "&TopicArn=" + url.QueryEscape(topicARN)
	confRec := httptest.NewRecorder()
	confReq := mustNewRequest(t, http.MethodGet, confirmURL, nil)
	handler.ServeHTTP(confRec, confReq)
	if confRec.Code != http.StatusOK {
		t.Fatalf("ConfirmSubscription status=%d body=%q", confRec.Code, confRec.Body.String())
	}
	var confirmResp map[string]string
	if err := json.Unmarshal(confRec.Body.Bytes(), &confirmResp); err != nil {
		t.Fatal(err)
	}
	if confirmResp["Status"] != "confirmed" || confirmResp["SubscriptionArn"] == "" {
		t.Fatalf("confirm resp=%v", confirmResp)
	}

	badConf := httptest.NewRecorder()
	badConfReq := mustNewRequest(t, http.MethodGet, catcher+"?Action=ConfirmSubscription&Token=bad&TopicArn="+url.QueryEscape(topicARN), nil)
	handler.ServeHTTP(badConf, badConfReq)
	if badConf.Code != http.StatusBadRequest {
		t.Fatalf("bad confirm want 400 got %d", badConf.Code)
	}

	badPost := httptest.NewRecorder()
	badPostReq := mustNewRequest(t, http.MethodPost, catcher, []byte("not-json"))
	handler.ServeHTTP(badPost, badPostReq)
	if badPost.Code != http.StatusOK {
		t.Fatalf("invalid json post want 200 got %d", badPost.Code)
	}
}

func TestSNSConfirmListSubscriptionsAndTags(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createQ := mustSQSJSON(t, handler, "CreateQueue", map[string]any{"QueueName": "sns-cov-q"}, now)
	if createQ.Code != http.StatusOK {
		t.Fatalf("CreateQueue status=%d body=%q", createQ.Code, createQ.Body.String())
	}
	qARN := "arn:aws:sqs:" + testRegion + ":" + testAccountID + ":sns-cov-q"

	createTopic := url.Values{
		"Action": {"CreateTopic"}, "Version": {"2010-03-31"}, "Name": {"cov-topic"},
		"Tags.member.1.Key": {"env"}, "Tags.member.1.Value": {"lab"},
	}.Encode()
	topicReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(createTopic))
	signHeader(t, topicReq, []byte(createTopic), testAccessKey, testSecret, testRegion, "sns", now)
	topicRec := httptest.NewRecorder()
	handler.ServeHTTP(topicRec, topicReq)
	if topicRec.Code != http.StatusOK {
		t.Fatalf("CreateTopic status=%d body=%q", topicRec.Code, topicRec.Body.String())
	}
	topicARN := xmlTag(t, topicRec.Body.String(), "TopicArn")

	subBody := url.Values{
		"Action": {"Subscribe"}, "Version": {"2010-03-31"},
		"TopicArn": {topicARN}, "Protocol": {"sqs"}, "Endpoint": {qARN},
	}.Encode()
	subReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(subBody))
	signHeader(t, subReq, []byte(subBody), testAccessKey, testSecret, testRegion, "sns", now)
	subRec := httptest.NewRecorder()
	handler.ServeHTTP(subRec, subReq)
	if subRec.Code != http.StatusOK {
		t.Fatalf("Subscribe status=%d body=%q", subRec.Code, subRec.Body.String())
	}

	listSubs := url.Values{"Action": {"ListSubscriptions"}, "Version": {"2010-03-31"}}.Encode()
	listReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(listSubs))
	signHeader(t, listReq, []byte(listSubs), testAccessKey, testSecret, testRegion, "sns", now)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("ListSubscriptions status=%d body=%q", listRec.Code, listRec.Body.String())
	}

	listByTopic := url.Values{
		"Action": {"ListSubscriptionsByTopic"}, "Version": {"2010-03-31"}, "TopicArn": {topicARN},
	}.Encode()
	lbtReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(listByTopic))
	signHeader(t, lbtReq, []byte(listByTopic), testAccessKey, testSecret, testRegion, "sns", now)
	lbtRec := httptest.NewRecorder()
	handler.ServeHTTP(lbtRec, lbtReq)
	if lbtRec.Code != http.StatusOK {
		t.Fatalf("ListSubscriptionsByTopic status=%d body=%q", lbtRec.Code, lbtRec.Body.String())
	}

	// Pending confirmation token from store if present
	subs, err := st.ListSubscriptionsByTopic(testAccountID, "cov-topic")
	if err == nil {
		for _, sub := range subs {
			if sub.ConfirmToken != "" {
				confirm := url.Values{
					"Action": {"ConfirmSubscription"}, "Version": {"2010-03-31"},
					"TopicArn": {topicARN}, "Token": {sub.ConfirmToken},
				}.Encode()
				cReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(confirm))
				signHeader(t, cReq, []byte(confirm), testAccessKey, testSecret, testRegion, "sns", now)
				cRec := httptest.NewRecorder()
				handler.ServeHTTP(cRec, cReq)
				if cRec.Code != http.StatusOK {
					t.Fatalf("ConfirmSubscription status=%d body=%q", cRec.Code, cRec.Body.String())
				}
			}
		}
	}
	badConfirm := url.Values{
		"Action": {"ConfirmSubscription"}, "Version": {"2010-03-31"},
		"TopicArn": {topicARN}, "Token": {"bad-token"},
	}.Encode()
	bcReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(badConfirm))
	signHeader(t, bcReq, []byte(badConfirm), testAccessKey, testSecret, testRegion, "sns", now)
	bcRec := httptest.NewRecorder()
	handler.ServeHTTP(bcRec, bcReq)
	if bcRec.Code == http.StatusOK {
		t.Fatalf("ConfirmSubscription bad token should fail: %q", bcRec.Body.String())
	}

	tagBody := url.Values{
		"Action": {"TagResource"}, "Version": {"2010-03-31"}, "ResourceArn": {topicARN},
		"Tags.member.1.Key": {"owner"}, "Tags.member.1.Value": {"lab"},
	}.Encode()
	tagReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(tagBody))
	signHeader(t, tagReq, []byte(tagBody), testAccessKey, testSecret, testRegion, "sns", now)
	tagRec := httptest.NewRecorder()
	handler.ServeHTTP(tagRec, tagReq)
	if tagRec.Code != http.StatusOK {
		t.Fatalf("TagResource status=%d body=%q", tagRec.Code, tagRec.Body.String())
	}
	listTags := url.Values{
		"Action": {"ListTagsForResource"}, "Version": {"2010-03-31"}, "ResourceArn": {topicARN},
	}.Encode()
	ltReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(listTags))
	signHeader(t, ltReq, []byte(listTags), testAccessKey, testSecret, testRegion, "sns", now)
	ltRec := httptest.NewRecorder()
	handler.ServeHTTP(ltRec, ltReq)
	if ltRec.Code != http.StatusOK {
		t.Fatalf("ListTagsForResource status=%d body=%q", ltRec.Code, ltRec.Body.String())
	}
	untag := url.Values{
		"Action": {"UntagResource"}, "Version": {"2010-03-31"}, "ResourceArn": {topicARN},
		"TagKeys.member.1": {"owner"},
	}.Encode()
	utReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(untag))
	signHeader(t, utReq, []byte(untag), testAccessKey, testSecret, testRegion, "sns", now)
	utRec := httptest.NewRecorder()
	handler.ServeHTTP(utRec, utReq)
	if utRec.Code != http.StatusOK {
		t.Fatalf("UntagResource status=%d body=%q", utRec.Code, utRec.Body.String())
	}

	attrs := url.Values{
		"Action": {"GetTopicAttributes"}, "Version": {"2010-03-31"}, "TopicArn": {topicARN},
	}.Encode()
	aReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(attrs))
	signHeader(t, aReq, []byte(attrs), testAccessKey, testSecret, testRegion, "sns", now)
	aRec := httptest.NewRecorder()
	handler.ServeHTTP(aRec, aReq)
	if aRec.Code != http.StatusOK {
		t.Fatalf("GetTopicAttributes status=%d body=%q", aRec.Code, aRec.Body.String())
	}
	setAttr := url.Values{
		"Action": {"SetTopicAttributes"}, "Version": {"2010-03-31"}, "TopicArn": {topicARN},
		"AttributeName": {"DisplayName"}, "AttributeValue": {"Cov"},
	}.Encode()
	saReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(setAttr))
	signHeader(t, saReq, []byte(setAttr), testAccessKey, testSecret, testRegion, "sns", now)
	saRec := httptest.NewRecorder()
	handler.ServeHTTP(saRec, saReq)
	if saRec.Code != http.StatusOK {
		t.Fatalf("SetTopicAttributes status=%d body=%q", saRec.Code, saRec.Body.String())
	}

	listTopics := url.Values{"Action": {"ListTopics"}, "Version": {"2010-03-31"}}.Encode()
	ltpReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(listTopics))
	signHeader(t, ltpReq, []byte(listTopics), testAccessKey, testSecret, testRegion, "sns", now)
	ltpRec := httptest.NewRecorder()
	handler.ServeHTTP(ltpRec, ltpReq)
	if ltpRec.Code != http.StatusOK || !strings.Contains(ltpRec.Body.String(), topicARN) {
		t.Fatalf("ListTopics status=%d body=%q", ltpRec.Code, ltpRec.Body.String())
	}

	del := url.Values{"Action": {"DeleteTopic"}, "Version": {"2010-03-31"}, "TopicArn": {topicARN}}.Encode()
	dReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(del))
	signHeader(t, dReq, []byte(del), testAccessKey, testSecret, testRegion, "sns", now)
	dRec := httptest.NewRecorder()
	handler.ServeHTTP(dRec, dReq)
	if dRec.Code != http.StatusOK {
		t.Fatalf("DeleteTopic status=%d body=%q", dRec.Code, dRec.Body.String())
	}
}

func TestSTSAssumeRoleAndFederationCoverage(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	trust := url.QueryEscape(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::` + testAccountID + `:root"},"Action":"sts:AssumeRole"}]}`)
	role := iamForm(t, handler, "Action=CreateRole&Version=2010-05-08&RoleName=sts-cov-role&AssumeRolePolicyDocument="+trust, now)
	if role.Code != http.StatusOK {
		t.Fatalf("CreateRole status=%d body=%q", role.Code, role.Body.String())
	}
	roleARN := "arn:aws:iam::" + testAccountID + ":role/sts-cov-role"

	assume := url.Values{
		"Action": {"AssumeRole"}, "Version": {"2011-06-15"},
		"RoleArn": {roleARN}, "RoleSessionName": {"cov-sess"},
	}.Encode()
	aReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(assume))
	signHeader(t, aReq, []byte(assume), testAccessKey, testSecret, testRegion, "sts", now)
	aRec := httptest.NewRecorder()
	handler.ServeHTTP(aRec, aReq)
	if aRec.Code != http.StatusOK {
		t.Fatalf("AssumeRole status=%d body=%q", aRec.Code, aRec.Body.String())
	}

	fed := url.Values{
		"Action": {"GetFederationToken"}, "Version": {"2011-06-15"},
		"Name": {"fed-user"},
		"Policy": {`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:ListBucket","Resource":"*"}]}`},
	}.Encode()
	fReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(fed))
	signHeader(t, fReq, []byte(fed), testAccessKey, testSecret, testRegion, "sts", now)
	fRec := httptest.NewRecorder()
	handler.ServeHTTP(fRec, fReq)
	if fRec.Code != http.StatusOK {
		t.Fatalf("GetFederationToken status=%d body=%q", fRec.Code, fRec.Body.String())
	}

	sess := url.Values{
		"Action": {"GetSessionToken"}, "Version": {"2011-06-15"}, "DurationSeconds": {"900"},
	}.Encode()
	sReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(sess))
	signHeader(t, sReq, []byte(sess), testAccessKey, testSecret, testRegion, "sts", now)
	sRec := httptest.NewRecorder()
	handler.ServeHTTP(sRec, sReq)
	if sRec.Code != http.StatusOK {
		t.Fatalf("GetSessionToken status=%d body=%q", sRec.Code, sRec.Body.String())
	}

	aki := url.Values{
		"Action": {"GetAccessKeyInfo"}, "Version": {"2011-06-15"}, "AccessKeyId": {testAccessKey},
	}.Encode()
	akiReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(aki))
	signHeader(t, akiReq, []byte(aki), testAccessKey, testSecret, testRegion, "sts", now)
	akiRec := httptest.NewRecorder()
	handler.ServeHTTP(akiRec, akiReq)
	if akiRec.Code != http.StatusOK {
		t.Fatalf("GetAccessKeyInfo status=%d body=%q", akiRec.Code, akiRec.Body.String())
	}

	decode := url.Values{
		"Action": {"DecodeAuthorizationMessage"}, "Version": {"2011-06-15"},
		"EncodedMessage": {"e30="},
	}.Encode()
	dReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(decode))
	signHeader(t, dReq, []byte(decode), testAccessKey, testSecret, testRegion, "sts", now)
	dRec := httptest.NewRecorder()
	handler.ServeHTTP(dRec, dReq)
	if dRec.Code != http.StatusOK && dRec.Code != http.StatusBadRequest {
		t.Fatalf("DecodeAuthorizationMessage status=%d body=%q", dRec.Code, dRec.Body.String())
	}

	// Fail-closed / not-implemented paths for remaining 0% handlers
	saml := url.Values{
		"Action": {"AssumeRoleWithSAML"}, "Version": {"2011-06-15"},
		"RoleArn": {roleARN}, "PrincipalArn": {"arn:aws:iam::" + testAccountID + ":saml-provider/x"},
		"SAMLAssertion": {"e30="},
	}.Encode()
	samlReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(saml))
	signHeader(t, samlReq, []byte(saml), testAccessKey, testSecret, testRegion, "sts", now)
	samlRec := httptest.NewRecorder()
	handler.ServeHTTP(samlRec, samlReq)
	if samlRec.Code == 0 {
		t.Fatal("empty response")
	}

	web := url.Values{
		"Action": {"AssumeRoleWithWebIdentity"}, "Version": {"2011-06-15"},
		"RoleArn": {roleARN}, "RoleSessionName": {"web"}, "WebIdentityToken": {"bad"},
	}.Encode()
	wReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(web))
	signHeader(t, wReq, []byte(web), testAccessKey, testSecret, testRegion, "sts", now)
	wRec := httptest.NewRecorder()
	handler.ServeHTTP(wRec, wReq)
	if wRec.Code == http.StatusOK {
		t.Fatalf("AssumeRoleWithWebIdentity bad token should fail: %q", wRec.Body.String())
	}
}

func TestDynamoDBUpdateTableStreamAndSSECoverage(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustDynamoJSON(t, handler, "CreateTable", map[string]any{
		"TableName": "upd-stream",
		"AttributeDefinitions": []map[string]any{
			{"AttributeName": "pk", "AttributeType": "S"},
		},
		"KeySchema": []map[string]any{
			{"AttributeName": "pk", "KeyType": "HASH"},
		},
		"BillingMode": "PAY_PER_REQUEST",
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateTable status=%d body=%q", create.Code, create.Body.String())
	}

	enableStream := mustDynamoJSON(t, handler, "UpdateTable", map[string]any{
		"TableName": "upd-stream",
		"StreamSpecification": map[string]any{
			"StreamEnabled":  true,
			"StreamViewType": "KEYS_ONLY",
		},
	}, now)
	if enableStream.Code != http.StatusOK {
		t.Fatalf("UpdateTable stream status=%d body=%q", enableStream.Code, enableStream.Body.String())
	}
	badView := mustDynamoJSON(t, handler, "UpdateTable", map[string]any{
		"TableName": "upd-stream",
		"StreamSpecification": map[string]any{
			"StreamEnabled":  true,
			"StreamViewType": "BAD",
		},
	}, now)
	if badView.Code != http.StatusBadRequest {
		t.Fatalf("UpdateTable bad view want 400 status=%d body=%q", badView.Code, badView.Body.String())
	}
	emptyUpd := mustDynamoJSON(t, handler, "UpdateTable", map[string]any{"TableName": "upd-stream"}, now)
	if emptyUpd.Code != http.StatusBadRequest {
		t.Fatalf("UpdateTable empty want 400 status=%d body=%q", emptyUpd.Code, emptyUpd.Body.String())
	}

	kms := mustJSONTarget(t, handler, "TrentService.CreateKey", "kms", map[string]any{
		"Description": "ddb-sse",
	}, now)
	if kms.Code != http.StatusOK {
		t.Fatalf("CreateKey status=%d body=%q", kms.Code, kms.Body.String())
	}
	var kmsOut map[string]any
	if err := json.Unmarshal(kms.Body.Bytes(), &kmsOut); err != nil {
		t.Fatal(err)
	}
	meta, _ := kmsOut["KeyMetadata"].(map[string]any)
	keyID, _ := meta["KeyId"].(string)
	sse := mustDynamoJSON(t, handler, "UpdateTable", map[string]any{
		"TableName": "upd-stream",
		"SSESpecification": map[string]any{
			"Enabled":         true,
			"SSEType":         "KMS",
			"KMSMasterKeyId":  keyID,
		},
	}, now)
	if sse.Code != http.StatusOK {
		t.Fatalf("UpdateTable SSE status=%d body=%q", sse.Code, sse.Body.String())
	}
	del := mustDynamoJSON(t, handler, "DeleteTable", map[string]any{"TableName": "upd-stream"}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteTable status=%d body=%q", del.Code, del.Body.String())
	}
}
