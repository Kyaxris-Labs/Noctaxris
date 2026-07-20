package server_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// TestSNSPublishStartsAsyncWorker verifies CS-016: SNS→Lambda delivery not only
// enqueues but runs the async worker (status leaves pending).
func TestSNSPublishStartsAsyncWorker(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "sns-async-exec", lambdaTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/sns-async-exec"
	createRec := mustLambdaJSON(t, handler, "CreateFunction", map[string]any{
		"FunctionName": "sns-async-fn",
		"Runtime":      "python3.12",
		"Role":         roleARN,
		"Handler":      "app.handler",
		"Code":         map[string]any{"ZipFile": testLambdaZipB64(t)},
	}, now)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateFunction status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	fnARN := "arn:aws:lambda:us-east-1:" + testAccountID + ":function:sns-async-fn"
	addRec := mustLambdaJSON(t, handler, "AddPermission", map[string]any{
		"FunctionName": "sns-async-fn",
		"StatementId":  "sns-allow",
		"Action":       "lambda:InvokeFunction",
		"Principal":    "sns.amazonaws.com",
	}, now)
	if addRec.Code != http.StatusOK {
		t.Fatalf("AddPermission status=%d body=%q", addRec.Code, addRec.Body.String())
	}

	topicARN := snsCreateTopic(t, handler, "sns-async-topic", now)
	subRec := mustSNSQuery(t, handler,
		"Action=Subscribe&Version=2010-03-31&TopicArn="+url.QueryEscape(topicARN)+
			"&Protocol=lambda&Endpoint="+url.QueryEscape(fnARN),
		testAccessKey, testSecret, now)
	if subRec.Code != http.StatusOK {
		t.Fatalf("Subscribe status=%d body=%q", subRec.Code, subRec.Body.String())
	}

	pubRec := mustSNSQuery(t, handler,
		"Action=Publish&Version=2010-03-31&TopicArn="+url.QueryEscape(topicARN)+
			"&Message="+url.QueryEscape("async-worker-payload"),
		testAccessKey, testSecret, now)
	if pubRec.Code != http.StatusOK {
		t.Fatalf("Publish status=%d body=%q", pubRec.Code, pubRec.Body.String())
	}

	job, err := st.LatestAsyncInvocation(testAccountID, "sns-async-fn")
	if err != nil {
		t.Fatal(err)
	}
	if job.Status == "pending" {
		t.Fatalf("async job still pending; OnAsyncEnqueue worker did not run: %+v", job)
	}
	if !strings.Contains(job.EventJSON, "async-worker-payload") {
		t.Fatalf("EventJSON=%q", job.EventJSON)
	}
}
