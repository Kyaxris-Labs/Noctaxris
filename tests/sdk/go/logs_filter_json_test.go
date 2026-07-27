package sdk_test

import (
	"strings"
	"testing"
	"time"
)

const ctAssumeRoleLog = `{"eventVersion":"1.08","userIdentity":{"type":"IAMUser","principalId":"AIDACKCEVSQ6C2EXAMPLE","arn":"arn:aws:iam::000000000001:user/alice"},"eventTime":"2026-01-02T03:04:05Z","eventSource":"sts.amazonaws.com","eventName":"AssumeRole","awsRegion":"us-east-1"}`

func TestLogsFilterLogEventsJSONEventName(t *testing.T) {
	requireReady(t)
	prefix := uniquePrefix(t)
	group := "/lab/sdk-json-" + prefix
	stream := "s1"
	ts := time.Now().UTC().UnixMilli()

	createGroupStatus, createGroupBody, _ := signedJSONTarget(t, "logs", "Logs_20140328.CreateLogGroup", map[string]any{
		"logGroupName": group,
	})
	if createGroupStatus != 200 {
		t.Fatalf("CreateLogGroup status=%d body=%s", createGroupStatus, createGroupBody)
	}
	t.Cleanup(func() {
		_, _, _ = signedJSONTarget(t, "logs", "Logs_20140328.DeleteLogStream", map[string]any{
			"logGroupName": group, "logStreamName": stream,
		})
		_, _, _ = signedJSONTarget(t, "logs", "Logs_20140328.DeleteLogGroup", map[string]any{
			"logGroupName": group,
		})
	})

	createStreamStatus, createStreamBody, _ := signedJSONTarget(t, "logs", "Logs_20140328.CreateLogStream", map[string]any{
		"logGroupName": group, "logStreamName": stream,
	})
	if createStreamStatus != 200 {
		t.Fatalf("CreateLogStream status=%d body=%s", createStreamStatus, createStreamBody)
	}

	putStatus, putBody, _ := signedJSONTarget(t, "logs", "Logs_20140328.PutLogEvents", map[string]any{
		"logGroupName":  group,
		"logStreamName": stream,
		"logEvents": []map[string]any{
			{"timestamp": ts, "message": `{"eventName":"GetObject","eventSource":"s3.amazonaws.com"}`},
			{"timestamp": ts + 1, "message": ctAssumeRoleLog},
			{"timestamp": ts + 2, "message": "plain text noise"},
		},
	})
	if putStatus != 200 {
		t.Fatalf("PutLogEvents status=%d body=%s", putStatus, putBody)
	}

	filterStatus, filterBody, filterParsed := signedJSONTarget(t, "logs", "Logs_20140328.FilterLogEvents", map[string]any{
		"logGroupName":  group,
		"filterPattern": `{ $.eventName = "AssumeRole" }`,
	})
	if filterStatus != 200 {
		t.Fatalf("FilterLogEvents status=%d body=%s", filterStatus, filterBody)
	}
	events, _ := filterParsed["events"].([]any)
	if len(events) != 1 {
		t.Fatalf("FilterLogEvents events=%v body=%s", filterParsed["events"], filterBody)
	}
	msg, _ := events[0].(map[string]any)["message"].(string)
	if !strings.Contains(msg, `"eventName":"AssumeRole"`) {
		t.Fatalf("filtered message=%q", msg)
	}
}
