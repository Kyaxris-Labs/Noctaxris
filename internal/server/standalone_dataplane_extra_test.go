package server_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/config"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestStandaloneDataplaneExtra(t *testing.T) {
	srv, st, _ := newTestServerStoreWith(t, func(cfg *config.Config) {
		cfg.LabStandalone = true
		cfg.RootSecretAccessKey = testSecret
	})
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	before := accountIDSet(t, st)
	for _, body := range [][]byte{
		[]byte(`{"name":"dp extra"}`),
		[]byte(`{"name":"dp/extra"}`),
	} {
		rec := postSeed(t, handler, testAccessKey, testSecret, body, now)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("seed body=%q status=%d want 400 response=%q", body, rec.Code, rec.Body.String())
		}
		assertSameAccounts(t, st, before)
	}

	seededRec := postSeed(t, handler, testAccessKey, testSecret, []byte(`{"name":"dp-extra"}`), now)
	if seededRec.Code != http.StatusOK {
		t.Fatalf("seed status=%d body=%q", seededRec.Code, seededRec.Body.String())
	}
	seeded := decodeSeed(t, seededRec)
	home := seeded["accountId"]
	userKey := seeded["accessKeyId"]
	userSecret := seeded["secretAccessKey"]
	userARN := seeded["userArn"]
	if home == "" || home == testAccountID || userKey == "" || userSecret == "" || userSecret == testSecret || userARN == "" {
		t.Fatalf("seeded=%v", seeded)
	}
	key, err := st.LookupAccessKeyRecord(userKey)
	if err != nil {
		t.Fatal(err)
	}
	if key.IsRoot || key.AccountID != home || key.Secret != userSecret {
		t.Fatalf("key=%+v", key)
	}

	const homeQueue = "dp-extra-home"
	const labQueue = "dp-extra-lab"
	const queueBody = "DpExtraQueueBody"
	homeCreate := mustSQSJSONWithCreds(t, handler, "CreateQueue", map[string]any{
		"QueueName": homeQueue,
	}, userKey, userSecret, now)
	if homeCreate.Code != http.StatusOK {
		t.Fatalf("home CreateQueue status=%d body=%q", homeCreate.Code, homeCreate.Body.String())
	}
	homeQueueURL := queueURLFrom(t, homeCreate)
	if !strings.Contains(homeQueueURL, home) || !strings.Contains(homeQueueURL, homeQueue) {
		t.Fatalf("home QueueUrl=%q account=%s", homeQueueURL, home)
	}
	ownQueues := mustSQSJSONWithCreds(t, handler, "ListQueues", map[string]any{}, userKey, userSecret, now)
	if ownQueues.Code != http.StatusOK || !strings.Contains(ownQueues.Body.String(), homeQueueURL) {
		t.Fatalf("home ListQueues status=%d body=%q", ownQueues.Code, ownQueues.Body.String())
	}
	labQueues := mustSQSJSON(t, handler, "ListQueues", map[string]any{}, now)
	if labQueues.Code != http.StatusOK || strings.Contains(labQueues.Body.String(), homeQueueURL) || strings.Contains(labQueues.Body.String(), homeQueue) {
		t.Fatalf("lab ListQueues status=%d body=%q", labQueues.Code, labQueues.Body.String())
	}

	labCreate := mustSQSJSON(t, handler, "CreateQueue", map[string]any{
		"QueueName": labQueue,
	}, now)
	if labCreate.Code != http.StatusOK {
		t.Fatalf("lab CreateQueue status=%d body=%q", labCreate.Code, labCreate.Body.String())
	}
	labQueueURL := queueURLFrom(t, labCreate)
	labQueueARN := store.QueueARN(testRegion, testAccountID, labQueue)
	send := mustSQSJSON(t, handler, "SendMessage", map[string]any{
		"QueueUrl":    labQueueURL,
		"MessageBody": queueBody,
	}, now)
	if send.Code != http.StatusOK {
		t.Fatalf("lab SendMessage status=%d body=%q", send.Code, send.Body.String())
	}
	deniedRecv := mustSQSJSONWithCreds(t, handler, "ReceiveMessage", map[string]any{
		"QueueUrl":            labQueueURL,
		"MaxNumberOfMessages": 1,
	}, userKey, userSecret, now)
	if deniedRecv.Code != http.StatusForbidden || !strings.Contains(deniedRecv.Body.String(), "AccessDenied") || strings.Contains(deniedRecv.Body.String(), queueBody) {
		t.Fatalf("cross ReceiveMessage status=%d body=%q", deniedRecv.Code, deniedRecv.Body.String())
	}
	queuePolicy := fmt.Sprintf(
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"%s"},"Action":"sqs:ReceiveMessage","Resource":"%s"}]}`,
		userARN, labQueueARN,
	)
	setQueue := mustSQSJSON(t, handler, "SetQueueAttributes", map[string]any{
		"QueueUrl": labQueueURL,
		"Attributes": map[string]string{
			"Policy": queuePolicy,
		},
	}, now)
	if setQueue.Code != http.StatusOK {
		t.Fatalf("SetQueueAttributes status=%d body=%q", setQueue.Code, setQueue.Body.String())
	}
	allowedRecv := mustSQSJSONWithCreds(t, handler, "ReceiveMessage", map[string]any{
		"QueueUrl":            labQueueURL,
		"MaxNumberOfMessages": 1,
	}, userKey, userSecret, now)
	if allowedRecv.Code != http.StatusOK || !strings.Contains(allowedRecv.Body.String(), queueBody) {
		t.Fatalf("allowed ReceiveMessage status=%d body=%q", allowedRecv.Code, allowedRecv.Body.String())
	}

	const topicName = "dp-extra-topic"
	const displayName = "DpExtraTopicLabel"
	const publishBody = "dp-extra-notice"
	topicRec := mustSNSQuery(t, handler,
		"Action=CreateTopic&Version=2010-03-31&Name="+url.QueryEscape(topicName),
		testAccessKey, testSecret, now)
	if topicRec.Code != http.StatusOK {
		t.Fatalf("CreateTopic status=%d body=%q", topicRec.Code, topicRec.Body.String())
	}
	topicARN := xmlTag(t, topicRec.Body.String(), "TopicArn")
	if !strings.Contains(topicARN, testAccountID) || !strings.Contains(topicARN, topicName) {
		t.Fatalf("TopicArn=%q", topicARN)
	}
	setDisplay := mustSNSQuery(t, handler,
		"Action=SetTopicAttributes&Version=2010-03-31&TopicArn="+url.QueryEscape(topicARN)+
			"&AttributeName=DisplayName&AttributeValue="+url.QueryEscape(displayName),
		testAccessKey, testSecret, now)
	if setDisplay.Code != http.StatusOK {
		t.Fatalf("SetTopicAttributes DisplayName status=%d body=%q", setDisplay.Code, setDisplay.Body.String())
	}
	deniedAttrs := mustSNSQuery(t, handler,
		"Action=GetTopicAttributes&Version=2010-03-31&TopicArn="+url.QueryEscape(topicARN),
		userKey, userSecret, now)
	if deniedAttrs.Code != http.StatusForbidden || strings.Contains(deniedAttrs.Body.String(), displayName) {
		t.Fatalf("cross GetTopicAttributes status=%d body=%q", deniedAttrs.Code, deniedAttrs.Body.String())
	}
	deniedPublish := mustSNSQuery(t, handler,
		"Action=Publish&Version=2010-03-31&TopicArn="+url.QueryEscape(topicARN)+
			"&Message="+url.QueryEscape(publishBody),
		userKey, userSecret, now)
	if deniedPublish.Code != http.StatusForbidden || strings.Contains(deniedPublish.Body.String(), "<MessageId>") {
		t.Fatalf("cross Publish status=%d body=%q", deniedPublish.Code, deniedPublish.Body.String())
	}
	topicPolicy := fmt.Sprintf(
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"%s"},"Action":["sns:GetTopicAttributes","sns:Publish"],"Resource":"%s"}]}`,
		userARN, topicARN,
	)
	setTopic := mustSNSQuery(t, handler,
		"Action=SetTopicAttributes&Version=2010-03-31&TopicArn="+url.QueryEscape(topicARN)+
			"&AttributeName=Policy&AttributeValue="+url.QueryEscape(topicPolicy),
		testAccessKey, testSecret, now)
	if setTopic.Code != http.StatusOK {
		t.Fatalf("SetTopicAttributes Policy status=%d body=%q", setTopic.Code, setTopic.Body.String())
	}
	allowedAttrs := mustSNSQuery(t, handler,
		"Action=GetTopicAttributes&Version=2010-03-31&TopicArn="+url.QueryEscape(topicARN),
		userKey, userSecret, now)
	if allowedAttrs.Code != http.StatusOK || !strings.Contains(allowedAttrs.Body.String(), displayName) {
		t.Fatalf("allowed GetTopicAttributes status=%d body=%q", allowedAttrs.Code, allowedAttrs.Body.String())
	}
	allowedPublish := mustSNSQuery(t, handler,
		"Action=Publish&Version=2010-03-31&TopicArn="+url.QueryEscape(topicARN)+
			"&Message="+url.QueryEscape(publishBody),
		userKey, userSecret, now)
	if allowedPublish.Code != http.StatusOK || xmlTag(t, allowedPublish.Body.String(), "MessageId") == "" {
		t.Fatalf("allowed Publish status=%d body=%q", allowedPublish.Code, allowedPublish.Body.String())
	}

	const secretName = "dp-extra-secret"
	const secretString = "DpExtraSecretMaterial"
	createSecret := mustSecretsJSON(t, handler, "CreateSecret", map[string]any{
		"Name":         secretName,
		"SecretString": secretString,
	}, now)
	if createSecret.Code != http.StatusOK {
		t.Fatalf("CreateSecret status=%d body=%q", createSecret.Code, createSecret.Body.String())
	}
	var secretOut map[string]any
	if err := json.Unmarshal(createSecret.Body.Bytes(), &secretOut); err != nil {
		t.Fatal(err)
	}
	secretARN, _ := secretOut["ARN"].(string)
	if secretARN == "" || !strings.Contains(secretARN, testAccountID) {
		t.Fatalf("secret ARN=%q body=%q", secretARN, createSecret.Body.String())
	}
	if _, err := st.GetSecretResourcePolicy(testAccountID, secretName); !errors.Is(err, store.ErrNoSuchResourcePolicy) {
		t.Fatalf("lab secret resource policy=%v", err)
	}
	deniedSecret := mustSecretsJSONWithCreds(t, handler, "GetSecretValue", map[string]any{
		"SecretId": secretARN,
	}, userKey, userSecret, now)
	if deniedSecret.Code != http.StatusForbidden || !strings.Contains(deniedSecret.Body.String(), "AccessDenied") || strings.Contains(deniedSecret.Body.String(), secretString) {
		t.Fatalf("cross GetSecretValue status=%d body=%q", deniedSecret.Code, deniedSecret.Body.String())
	}

	ownDoc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"sqs:ListQueues","Resource":"*"}]}`
	ownARN, err := st.CreateManagedPolicy(home, "DpExtraHomePol", ownDoc)
	if err != nil {
		t.Fatal(err)
	}
	ownDelete := stsOrOrg(t, handler, "Action=DeletePolicy&Version=2010-05-08&PolicyArn="+url.QueryEscape(ownARN), userKey, userSecret, "iam", now)
	if ownDelete.Code != http.StatusOK {
		t.Fatalf("own DeletePolicy status=%d body=%q", ownDelete.Code, ownDelete.Body.String())
	}
	if _, err := st.GetManagedPolicy(ownARN); err == nil {
		t.Fatal("own policy still exists after DeletePolicy")
	}

	const labMarker = "DpExtraLabPolicyMarker"
	labDoc := `{"Version":"2012-10-17","Statement":[{"Sid":"` + labMarker + `","Effect":"Allow","Action":"sqs:ListQueues","Resource":"*"}]}`
	labARN, err := st.CreateManagedPolicy(testAccountID, "DpExtraLabPol", labDoc)
	if err != nil {
		t.Fatal(err)
	}
	labDelete := stsOrOrg(t, handler, "Action=DeletePolicy&Version=2010-05-08&PolicyArn="+url.QueryEscape(labARN), userKey, userSecret, "iam", now)
	if labDelete.Code != http.StatusForbidden || !strings.Contains(labDelete.Body.String(), "AccessDenied") || strings.Contains(labDelete.Body.String(), labMarker) {
		t.Fatalf("lab DeletePolicy status=%d body=%q", labDelete.Code, labDelete.Body.String())
	}
	if _, err := st.GetManagedPolicy(labARN); err != nil {
		t.Fatalf("lab policy missing after denied DeletePolicy: %v", err)
	}
}

func queueURLFrom(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	queueURL, _ := out["QueueUrl"].(string)
	if queueURL == "" {
		t.Fatalf("missing QueueUrl body=%q", rec.Body.String())
	}
	return queueURL
}
