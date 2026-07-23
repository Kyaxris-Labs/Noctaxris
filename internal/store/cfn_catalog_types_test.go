package store_test

import (
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestCFNTopicPolicySubscriptionAndLogGroup(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	tpl := `{
	  "Resources": {
	    "LabTopic": {
	      "Type": "AWS::SNS::Topic",
	      "Properties": { "TopicName": "cfn-tp-topic" }
	    },
	    "LabQueue": {
	      "Type": "AWS::SQS::Queue",
	      "Properties": {
	        "QueueName": "cfn-tp-queue",
	        "DelaySeconds": 5
	      }
	    },
	    "LabTopicPolicy": {
	      "Type": "AWS::SNS::TopicPolicy",
	      "DependsOn": ["LabTopic"],
	      "Properties": {
	        "Topics": [{"Ref": "LabTopic"}],
	        "PolicyDocument": {
	          "Version": "2012-10-17",
	          "Statement": [{
	            "Effect": "Allow",
	            "Principal": {"Service": "s3.amazonaws.com"},
	            "Action": "sns:Publish",
	            "Resource": "*"
	          }]
	        }
	      }
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
	            "Principal": {"Service": "sns.amazonaws.com"},
	            "Action": "sqs:SendMessage",
	            "Resource": "*"
	          }]
	        }
	      }
	    },
	    "LabSub": {
	      "Type": "AWS::SNS::Subscription",
	      "DependsOn": ["LabTopicPolicy", "LabQueuePolicy"],
	      "Properties": {
	        "TopicArn": {"Ref": "LabTopic"},
	        "Protocol": "sqs",
	        "Endpoint": {"Fn::GetAtt": ["LabQueue", "Arn"]}
	      }
	    },
	    "LabLogs": {
	      "Type": "AWS::Logs::LogGroup",
	      "Properties": { "LogGroupName": "/cfn/lab-tp" }
	    }
	  }
	}`
	created, err := st.CreateCFNStack(account, "us-east-1", "cfn-tp-stack", tpl, "")
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != "CREATE_COMPLETE" {
		t.Fatalf("status=%s", created.Status)
	}

	topic, err := st.GetTopic(account, "cfn-tp-topic")
	if err != nil {
		t.Fatalf("topic: %v", err)
	}
	attrs, err := st.GetTopicAttributes(account, topic.TopicName)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(attrs["Policy"], "s3.amazonaws.com") {
		t.Fatalf("topic policy=%s", attrs["Policy"])
	}

	q, err := st.GetQueue(account, "cfn-tp-queue")
	if err != nil {
		t.Fatalf("queue: %v", err)
	}
	if q.Attributes["DelaySeconds"] != "5" {
		t.Fatalf("DelaySeconds=%q", q.Attributes["DelaySeconds"])
	}

	subs, err := st.ListSubscriptionsByTopic(account, "cfn-tp-topic")
	if err != nil || len(subs) != 1 {
		t.Fatalf("subs: %v %#v", err, subs)
	}
	if subs[0].Protocol != "sqs" || !strings.Contains(subs[0].Endpoint, "cfn-tp-queue") {
		t.Fatalf("sub=%+v", subs[0])
	}

	groups, err := st.DescribeLogGroups(account, "/cfn/lab-tp")
	if err != nil || len(groups) != 1 {
		t.Fatalf("log groups: %v %#v", err, groups)
	}

	drift, err := st.DetectCFNStackDrift(account, "cfn-tp-stack")
	if err != nil {
		t.Fatal(err)
	}
	if drift.StackDriftStatus != "IN_SYNC" {
		t.Fatalf("drift=%s", drift.StackDriftStatus)
	}

	if err := st.DeleteCFNStack(account, "cfn-tp-stack"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetTopic(account, "cfn-tp-topic"); err == nil {
		t.Fatal("topic should be deleted")
	}
	if _, err := st.GetSubscription(subs[0].SubscriptionARN); err == nil {
		t.Fatal("subscription should be deleted")
	}
	if groups, err := st.DescribeLogGroups(account, "/cfn/lab-tp"); err != nil || len(groups) != 0 {
		t.Fatalf("log group should be deleted: %v %#v", err, groups)
	}
}

func TestCFNKMSAliasAndIAMUserGroup(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	tpl := `{
	  "Resources": {
	    "LabKey": {
	      "Type": "AWS::KMS::Key",
	      "Properties": {
	        "KeyPolicy": {
	          "Version": "2012-10-17",
	          "Statement": [{
	            "Effect": "Allow",
	            "Principal": {"AWS": "*"},
	            "Action": "kms:*",
	            "Resource": "*"
	          }]
	        }
	      }
	    },
	    "LabAlias": {
	      "Type": "AWS::KMS::Alias",
	      "DependsOn": ["LabKey"],
	      "Properties": {
	        "AliasName": "alias/cfn-lab-key",
	        "TargetKeyId": {"Ref": "LabKey"}
	      }
	    },
	    "LabGroup": {
	      "Type": "AWS::IAM::Group",
	      "Properties": { "GroupName": "cfn-lab-group" }
	    },
	    "LabUser": {
	      "Type": "AWS::IAM::User",
	      "DependsOn": ["LabGroup"],
	      "Properties": {
	        "UserName": "cfn-lab-user",
	        "Groups": ["cfn-lab-group"]
	      }
	    },
	    "LabMP": {
	      "Type": "AWS::IAM::ManagedPolicy",
	      "DependsOn": ["LabUser", "LabGroup"],
	      "Properties": {
	        "ManagedPolicyName": "cfn-lab-ug-mp",
	        "PolicyDocument": {
	          "Version": "2012-10-17",
	          "Statement": [{"Effect":"Allow","Action":"s3:ListBucket","Resource":"*"}]
	        },
	        "Users": ["cfn-lab-user"],
	        "Groups": ["cfn-lab-group"]
	      }
	    }
	  }
	}`
	created, err := st.CreateCFNStack(account, "us-east-1", "cfn-alias-ug", tpl, "")
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != "CREATE_COMPLETE" {
		t.Fatalf("status=%s", created.Status)
	}

	keyID, err := st.ResolveKeyID(account, "alias/cfn-lab-key")
	if err != nil || keyID == "" {
		t.Fatalf("alias resolve: %v %q", err, keyID)
	}
	if _, err := st.GetUser(account, "cfn-lab-user"); err != nil {
		t.Fatalf("user: %v", err)
	}
	if _, err := st.GetGroup(account, "cfn-lab-group"); err != nil {
		t.Fatalf("group: %v", err)
	}
	userARN := store.UserARN(account, "/", "cfn-lab-user")
	docs, err := st.ListAttachedPolicyDocuments(userARN)
	if err != nil || len(docs) == 0 {
		t.Fatalf("user policies: %v %#v", err, docs)
	}

	if err := st.DeleteCFNStack(account, "cfn-alias-ug"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ResolveKeyID(account, "alias/cfn-lab-key"); err == nil {
		t.Fatal("alias should be deleted")
	}
	if _, err := st.GetUser(account, "cfn-lab-user"); err == nil {
		t.Fatal("user should be deleted")
	}
}

func TestCFNRejectsScheduleExpressionAndRetention(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"

	_, err := st.CreateCFNStack(account, "us-east-1", "bad-sched", `{
	  "Resources": {
	    "R": {
	      "Type": "AWS::Events::Rule",
	      "Properties": {
	        "Name": "sched",
	        "ScheduleExpression": "rate(5 minutes)",
	        "EventPattern": {"source": ["noctaxris.lab"]}
	      }
	    }
	  }
	}`, "")
	if err == nil || !strings.Contains(err.Error(), "ScheduleExpression") {
		t.Fatalf("want ScheduleExpression reject, got %v", err)
	}

	_, err = st.CreateCFNStack(account, "us-east-1", "bad-retention", `{
	  "Resources": {
	    "G": {
	      "Type": "AWS::Logs::LogGroup",
	      "Properties": {
	        "LogGroupName": "/cfn/bad",
	        "RetentionInDays": 7
	      }
	    }
	  }
	}`, "")
	if err == nil || !strings.Contains(err.Error(), "RetentionInDays") {
		t.Fatalf("want RetentionInDays reject, got %v", err)
	}
}

func TestCFNSubscriptionRejectsUnknownProperty(t *testing.T) {
	st := openTestStore(t)
	_, err := st.CreateCFNStack("000000000001", "us-east-1", "bad-sub", `{
	  "Resources": {
	    "T": { "Type": "AWS::SNS::Topic", "Properties": { "TopicName": "cfn-bad-sub-topic" } },
	    "S": {
	      "Type": "AWS::SNS::Subscription",
	      "DependsOn": ["T"],
	      "Properties": {
	        "TopicArn": {"Ref": "T"},
	        "Protocol": "sqs",
	        "Endpoint": "arn:aws:sqs:us-east-1:000000000001:missing",
	        "FilterPolicy": {"a": ["b"]}
	      }
	    }
	  }
	}`, "")
	if err == nil || !strings.Contains(err.Error(), "FilterPolicy") {
		t.Fatalf("want FilterPolicy reject, got %v", err)
	}
}
