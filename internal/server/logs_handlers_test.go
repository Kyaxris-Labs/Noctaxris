package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

const logsTrustOK = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"logs.amazonaws.com"},"Action":"sts:AssumeRole"}]}`

const logsTrustBad = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::000000000001:root"},"Action":"sts:AssumeRole"}]}`

func TestLogsFiltersPoliciesRetentionAndErrors(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)
	group := "/lab/cov-logs"

	if rec := mustLogsJSON(t, handler, "CreateLogGroup", map[string]any{"logGroupName": group}, now); rec.Code != http.StatusOK {
		t.Fatalf("CreateLogGroup %d %s", rec.Code, rec.Body.String())
	}

	// Retention policy put/delete + negatives
	if rec := mustLogsJSON(t, handler, "PutRetentionPolicy", map[string]any{
		"logGroupName":    group,
		"retentionInDays": 7,
	}, now); rec.Code != http.StatusOK {
		t.Fatalf("PutRetentionPolicy %d %s", rec.Code, rec.Body.String())
	}
	if rec := mustLogsJSON(t, handler, "PutRetentionPolicy", map[string]any{
		"logGroupName":    "",
		"retentionInDays": 7,
	}, now); rec.Code != http.StatusBadRequest {
		t.Fatalf("PutRetentionPolicy empty name want 400 %d %s", rec.Code, rec.Body.String())
	}
	if rec := mustLogsJSON(t, handler, "PutRetentionPolicy", map[string]any{
		"logGroupName":    "/lab/missing-ret",
		"retentionInDays": 7,
	}, now); rec.Code != http.StatusBadRequest {
		t.Fatalf("PutRetentionPolicy missing group want 400 %d %s", rec.Code, rec.Body.String())
	}
	if rec := mustLogsJSON(t, handler, "DeleteRetentionPolicy", map[string]any{
		"logGroupName": group,
	}, now); rec.Code != http.StatusOK {
		t.Fatalf("DeleteRetentionPolicy %d %s", rec.Code, rec.Body.String())
	}

	// Metric filter CRUD
	if rec := mustLogsJSON(t, handler, "PutMetricFilter", map[string]any{
		"logGroupName":  group,
		"filterName":    "err-count",
		"filterPattern": "ERROR",
		"metricTransformations": []map[string]any{{
			"metricName":      "ErrorCount",
			"metricNamespace": "Lab",
			"metricValue":     "1",
		}},
	}, now); rec.Code != http.StatusOK {
		t.Fatalf("PutMetricFilter %d %s", rec.Code, rec.Body.String())
	}
	if rec := mustLogsJSON(t, handler, "DescribeMetricFilters", map[string]any{
		"logGroupName": group,
	}, now); rec.Code != http.StatusOK {
		t.Fatalf("DescribeMetricFilters %d %s", rec.Code, rec.Body.String())
	}
	if rec := mustLogsJSON(t, handler, "DeleteMetricFilter", map[string]any{
		"logGroupName": group,
		"filterName":   "err-count",
	}, now); rec.Code != http.StatusOK {
		t.Fatalf("DeleteMetricFilter %d %s", rec.Code, rec.Body.String())
	}

	// Resource policy CRUD + list
	policyDoc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":"*","Action":"logs:PutLogEvents","Resource":"*"}]}`
	if rec := mustLogsJSON(t, handler, "PutResourcePolicy", map[string]any{
		"policyName":     "lab-policy",
		"policyDocument": policyDoc,
	}, now); rec.Code != http.StatusOK {
		t.Fatalf("PutResourcePolicy %d %s", rec.Code, rec.Body.String())
	}
	if rec := mustLogsJSON(t, handler, "GetResourcePolicy", map[string]any{
		"policyName": "lab-policy",
	}, now); rec.Code != http.StatusOK {
		t.Fatalf("GetResourcePolicy %d %s", rec.Code, rec.Body.String())
	}
	if rec := mustLogsJSON(t, handler, "DescribeResourcePolicies", map[string]any{}, now); rec.Code != http.StatusOK {
		t.Fatalf("DescribeResourcePolicies %d %s", rec.Code, rec.Body.String())
	}
	if rec := mustLogsJSON(t, handler, "DeleteResourcePolicy", map[string]any{
		"policyName": "lab-policy",
	}, now); rec.Code != http.StatusOK {
		t.Fatalf("DeleteResourcePolicy %d %s", rec.Code, rec.Body.String())
	}

	// Lambda destination ignores roleArn (no checkLogsPassRole).
	subLambda := mustLogsJSON(t, handler, "PutSubscriptionFilter", map[string]any{
		"logGroupName":   group,
		"filterName":     "to-lambda",
		"filterPattern":  "",
		"destinationArn": "arn:aws:lambda:us-east-1:000000000001:function:missing",
		"roleArn":        "arn:aws:iam::000000000001:role/LogsDelivery",
	}, now)
	if subLambda.Code != http.StatusOK && subLambda.Code != http.StatusBadRequest && subLambda.Code != http.StatusForbidden {
		t.Fatalf("PutSubscriptionFilter lambda unexpected %d %s", subLambda.Code, subLambda.Body.String())
	}
	_ = mustLogsJSON(t, handler, "DescribeSubscriptionFilters", map[string]any{"logGroupName": group}, now)
	_ = mustLogsJSON(t, handler, "DeleteSubscriptionFilter", map[string]any{
		"logGroupName": group,
		"filterName":   "to-lambda",
	}, now)

	// SQS destination with roleArn exercises checkLogsPassRole.
	q, err := st.CreateQueue(testAccountID, testRegion, "127.0.0.1:4566", "logs-sub-q", nil)
	if err != nil {
		t.Fatal(err)
	}
	mustCreateIAMRole(t, handler, "logs-delivery-ok", logsTrustOK, now)
	mustCreateIAMRole(t, handler, "logs-delivery-bad", logsTrustBad, now)
	okRole := "arn:aws:iam::" + testAccountID + ":role/logs-delivery-ok"
	badRole := "arn:aws:iam::" + testAccountID + ":role/logs-delivery-bad"
	sqsARN := "arn:aws:sqs:" + testRegion + ":" + testAccountID + ":" + q.QueueName

	denyPass := mustLogsJSON(t, handler, "PutSubscriptionFilter", map[string]any{
		"logGroupName":   group,
		"filterName":     "to-sqs-deny",
		"filterPattern":  "",
		"destinationArn": sqsARN,
		"roleArn":        badRole,
	}, now)
	if denyPass.Code != http.StatusForbidden || !strings.Contains(denyPass.Body.String(), "AccessDeniedException") {
		t.Fatalf("PutSubscriptionFilter bad trust want 403 status=%d body=%q", denyPass.Code, denyPass.Body.String())
	}

	subSQS := mustLogsJSON(t, handler, "PutSubscriptionFilter", map[string]any{
		"logGroupName":   group,
		"filterName":     "to-sqs",
		"filterPattern":  "",
		"destinationArn": sqsARN,
		"roleArn":        okRole,
	}, now)
	if subSQS.Code != http.StatusOK {
		t.Fatalf("PutSubscriptionFilter SQS status=%d body=%q", subSQS.Code, subSQS.Body.String())
	}
	descSub := mustLogsJSON(t, handler, "DescribeSubscriptionFilters", map[string]any{"logGroupName": group}, now)
	if descSub.Code != http.StatusOK || !strings.Contains(descSub.Body.String(), "to-sqs") {
		t.Fatalf("DescribeSubscriptionFilters status=%d body=%q", descSub.Code, descSub.Body.String())
	}
	delSub := mustLogsJSON(t, handler, "DeleteSubscriptionFilter", map[string]any{
		"logGroupName": group,
		"filterName":   "to-sqs",
	}, now)
	if delSub.Code != http.StatusOK {
		t.Fatalf("DeleteSubscriptionFilter status=%d body=%q", delSub.Code, delSub.Body.String())
	}
	delSubGone := mustLogsJSON(t, handler, "DeleteSubscriptionFilter", map[string]any{
		"logGroupName": group,
		"filterName":   "to-sqs",
	}, now)
	if delSubGone.Code == http.StatusOK {
		t.Fatal("expected delete missing subscription filter failure")
	}

	// Negative: unknown action → writeLogsError / not implemented
	unknown := mustLogsJSON(t, handler, "NotARealLogsAction", map[string]any{}, now)
	if unknown.Code == http.StatusOK {
		t.Fatal("expected error for unknown logs action")
	}
	var errBody map[string]any
	_ = json.Unmarshal(unknown.Body.Bytes(), &errBody)
	if unknown.Body.Len() == 0 {
		t.Fatal("expected error body")
	}

	// Negative: delete missing group
	missing := mustLogsJSON(t, handler, "DeleteLogGroup", map[string]any{"logGroupName": "/lab/missing-xx"}, now)
	if missing.Code == http.StatusOK {
		t.Fatal("expected delete missing failure")
	}
}
