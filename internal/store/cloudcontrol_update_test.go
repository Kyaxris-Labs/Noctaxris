package store_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestCloudControlUpdateRoleAndQueue(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	trust := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	roleARN, err := st.CreateRole(account, "CCUpdateRole", trust)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutInlinePolicy(roleARN, "inline", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"}]}`); err != nil {
		t.Fatal(err)
	}
	patch := map[string]any{
		"Policies": []any{
			map[string]any{
				"PolicyName": "inline",
				"PolicyDocument": map[string]any{
					"Version": "2012-10-17",
					"Statement": []any{
						map[string]any{"Effect": "Allow", "Action": "s3:PutObject", "Resource": "*"},
					},
				},
			},
		},
	}
	raw, _ := json.Marshal(patch)
	_, _, err = st.CloudControlUpdateResource(account, "AWS::IAM::Role", "CCUpdateRole", string(raw))
	if err != nil {
		t.Fatal(err)
	}
	pol, err := st.GetInlinePolicy(roleARN, "inline")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(pol.Document, "s3:PutObject") {
		t.Fatalf("policy=%s", pol.Document)
	}

	q, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "cc-update-queue", map[string]string{
		"VisibilityTimeout": "30",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = st.CloudControlUpdateResource(account, "AWS::SQS::Queue", q.QueueURL, `{"VisibilityTimeout":60}`)
	if err != nil {
		t.Fatal(err)
	}
	attrs, err := st.GetQueueAttributes(account, q.QueueName)
	if err != nil {
		t.Fatal(err)
	}
	if attrs["VisibilityTimeout"] != "60" {
		t.Fatalf("attrs=%v", attrs)
	}
}

func TestCloudControlUpdateRejectsUnknownProperty(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	if _, err := st.CreateQueue(account, "us-east-1", "127.0.0.1:4566", "cc-bad-patch-q", nil); err != nil {
		t.Fatal(err)
	}
	q, err := st.GetQueue(account, "cc-bad-patch-q")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = st.CloudControlUpdateResource(account, "AWS::SQS::Queue", q.QueueURL, `{"VisibilityTimeout":10,"WebsiteConfiguration":{}}`)
	if err == nil || !strings.Contains(err.Error(), "unsupported patch") {
		t.Fatalf("expected unsupported patch reject, err=%v", err)
	}
}

func TestCloudControlUpdateLabFullstackTypes(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	if _, err := st.PutParameter(account, "us-east-1", "/lab/cc-upd", store.ParamTypeString, "a", "", true); err != nil {
		t.Fatal(err)
	}
	_, _, err := st.CloudControlUpdateResource(account, "AWS::SSM::Parameter", "/lab/cc-upd", `{"Value":"b"}`)
	if err != nil {
		t.Fatal(err)
	}
	p, err := st.GetParameter(account, "/lab/cc-upd", true)
	if err != nil || p.Value != "b" {
		t.Fatalf("param=%+v err=%v", p, err)
	}

	if _, err := st.CreateBucket(account, "cc-upd-bucket"); err != nil {
		t.Fatal(err)
	}
	encPatch := `{"BucketEncryption":{"ServerSideEncryptionConfiguration":[{"ServerSideEncryptionByDefault":{"SSEAlgorithm":"AES256"}}]}}`
	_, _, err = st.CloudControlUpdateResource(account, "AWS::S3::Bucket", "cc-upd-bucket", encPatch)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := st.CreateEventBus(account, "us-east-1", "cc-upd-bus"); err != nil {
		t.Fatal(err)
	}
	_, err = st.PutRule(account, "us-east-1", "cc-upd-bus", "cc-upd-rule", `{"source":["a"]}`, "", "ENABLED")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = st.CloudControlUpdateResource(account, "AWS::Events::Rule", "cc-upd-bus|cc-upd-rule", `{"State":"DISABLED","EventPattern":{"source":["b"]}}`)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = st.CloudControlUpdateResource(account, "AWS::Events::EventBus", "cc-upd-bus", `{"Name":"other"}`)
	if err == nil {
		t.Fatal("expected EventBus update reject")
	}
}
