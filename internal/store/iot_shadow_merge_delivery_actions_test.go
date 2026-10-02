package store_test

import (
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestIoTThingShadowMergeNestedAndDelete(t *testing.T) {
	st := openTestStore(t)
	const account = "000000000001"
	const region = "us-east-1"

	thing, err := st.CreateIoTThing(account, region, "shadow-merge", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpdateIoTThingShadow(account, region, thing.ThingName, "", map[string]any{
		"state": map[string]any{
			"desired": map[string]any{
				"nested": map[string]any{"a": 1.0, "b": "keep"},
				"flag":   true,
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	sh, err := st.UpdateIoTThingShadow(account, region, thing.ThingName, "", map[string]any{
		"state": map[string]any{
			"desired": map[string]any{
				"nested": map[string]any{"a": 2.0, "c": "new"},
				"flag":   nil,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if sh.Version < 2 {
		t.Fatalf("version=%d", sh.Version)
	}
	got, err := st.GetIoTThingShadow(account, region, thing.ThingName, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.PayloadJSON, `"a":2`) && !strings.Contains(got.PayloadJSON, `"a": 2`) {
		t.Fatalf("merged nested a missing: %s", got.PayloadJSON)
	}
	if !strings.Contains(got.PayloadJSON, "keep") || !strings.Contains(got.PayloadJSON, "new") {
		t.Fatalf("nested merge incomplete: %s", got.PayloadJSON)
	}
	if strings.Contains(got.PayloadJSON, `"flag"`) {
		t.Fatalf("nil flag should delete key: %s", got.PayloadJSON)
	}
}

func TestDeliveryAuthorizedAcrossTargetFamilies(t *testing.T) {
	st := openTestStore(t)
	const account = "000000000001"
	const region = "us-east-1"

	trust := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"events.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	roleARN, err := st.CreateRole(account, "multi-deliver", trust)
	if err != nil {
		t.Fatal(err)
	}
	allow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["sqs:SendMessage","lambda:InvokeFunction","sns:Publish","s3:PutObject","logs:PutLogEvents","kinesis:PutRecord","states:StartExecution"],"Resource":"*"}]}`
	if err := st.PutInlinePolicy(roleARN, "all", allow); err != nil {
		t.Fatal(err)
	}

	topic, err := st.CreateTopic(account, region, "deliver-topic", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateBucket(account, "deliver-bucket"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateLogGroup(account, region, "/deliver/logs"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateKinesisStream(account, region, "deliver-stream", 1); err != nil {
		t.Fatal(err)
	}
	zip := testZip(t, map[string]string{"app.py": "def handler(e, c):\n    return e\n"})
	fn, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID: account, Region: region, FunctionName: "deliver-fn",
		Runtime: store.LambdaRuntimePython312, RoleARN: roleARN,
		Handler: "app.handler", Timeout: 3, Memory: 128, Zip: zip,
	})
	if err != nil {
		t.Fatal(err)
	}

	targets := []struct {
		action string
		arn    string
	}{
		{"sns:Publish", topic.TopicARN},
		{"s3:PutObject", "arn:aws:s3:::deliver-bucket/key"},
		{"logs:PutLogEvents", store.LogGroupARN(region, account, "/deliver/logs")},
		{"kinesis:PutRecord", store.KinesisStreamARN(region, account, "deliver-stream")},
		{"lambda:InvokeFunction", fn.FunctionARN},
	}
	source := "arn:aws:events:" + region + ":" + account + ":rule/r"
	for _, tc := range targets {
		if !st.DeliveryAuthorizedRoleAndResource(account, roleARN, tc.action, tc.arn, "events.amazonaws.com", source, "sess", region) {
			t.Fatalf("want allow action=%s arn=%s", tc.action, tc.arn)
		}
	}
	_ = st.DeliveryAuthorizedRoleAndResource(account, roleARN, "sqs:SendMessage", "arn:aws:elasticache:"+region+":"+account+":cluster:x", "events.amazonaws.com", source, "sess", region)
}
