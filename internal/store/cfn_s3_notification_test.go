package store_test

import (
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestCFNS3BucketNotificationConfiguration(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	bucketName := "cfn-s3-notify-bucket"
	tpl := `{
	  "Resources": {
	    "LabQueue": {
	      "Type": "AWS::SQS::Queue",
	      "Properties": { "QueueName": "cfn-s3-notify-q" }
	    },
	    "LabQueuePolicy": {
	      "Type": "AWS::SQS::QueuePolicy",
	      "DependsOn": ["LabQueue"],
	      "Properties": {
	        "Queues": [{"Ref": "LabQueue"}],
	        "PolicyDocument": {
	          "Version": "2012-10-17",
	          "Statement": [{
	            "Effect": "Allow",
	            "Principal": {"Service": "s3.amazonaws.com"},
	            "Action": "sqs:SendMessage",
	            "Resource": "*",
	            "Condition": {"ArnLike": {"aws:SourceArn": "arn:aws:s3:::cfn-s3-notify-bucket"}}
	          }]
	        }
	      }
	    },
	    "LabBucket": {
	      "Type": "AWS::S3::Bucket",
	      "DependsOn": ["LabQueuePolicy"],
	      "Properties": {
	        "BucketName": "cfn-s3-notify-bucket",
	        "NotificationConfiguration": {
	          "EventBridgeConfiguration": { "EventBridgeEnabled": true },
	          "QueueConfigurations": [{
	            "Event": "s3:ObjectCreated:*",
	            "Queue": {"Fn::GetAtt": ["LabQueue", "Arn"]},
	            "Filter": {
	              "S3Key": {
	                "Rules": [
	                  {"Name": "prefix", "Value": "inbox/"},
	                  {"Name": "suffix", "Value": ".json"}
	                ]
	              }
	            }
	          }]
	        }
	      }
	    }
	  }
	}`
	if _, err := st.CreateCFNStack(account, "us-east-1", "s3-notify-stack", tpl, ""); err != nil {
		t.Fatal(err)
	}
	cfg, err := st.GetBucketNotificationConfiguration(account, bucketName)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.EventBridgeEnabled {
		t.Fatal("want EventBridgeEnabled")
	}
	if len(cfg.QueueConfigs) != 1 {
		t.Fatalf("queue configs=%d", len(cfg.QueueConfigs))
	}
	qc := cfg.QueueConfigs[0]
	if qc.FilterPrefix != "inbox/" || qc.FilterSuffix != ".json" {
		t.Fatalf("filter=%+v", qc)
	}
	if len(qc.Events) != 1 || qc.Events[0] != "s3:ObjectCreated:*" {
		t.Fatalf("events=%v", qc.Events)
	}
	if _, err := st.PutObject(account, bucketName, "inbox/a.json", store.PutObjectMeta{Data: []byte("{}"), PlainSize: 2}); err != nil {
		t.Fatal(err)
	}
	msgs, err := st.ReceiveMessages(account, "cfn-s3-notify-q", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("want notify delivery, got %d", len(msgs))
	}
}
