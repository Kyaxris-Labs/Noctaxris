package store_test

import (
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestCFNChangeSetAddExecuteAndUpdateStack(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	base := `{
  "Resources": {
    "LabBucket": {
      "Type": "AWS::S3::Bucket",
      "Properties": { "BucketName": "cfn-cs-bucket-1" }
    }
  }
}`
	if _, err := st.CreateCFNStack(account, "us-east-1", "cs-stack", base, ""); err != nil {
		t.Fatal(err)
	}
	updated := `{
  "Resources": {
    "LabBucket": {
      "Type": "AWS::S3::Bucket",
      "Properties": { "BucketName": "cfn-cs-bucket-1" }
    },
    "LabQueue": {
      "Type": "AWS::SQS::Queue",
      "Properties": { "QueueName": "cfn-cs-queue-1" }
    }
  }
}`
	cs, err := st.CreateCFNChangeSet(account, "us-east-1", "cs-stack", "add-queue", updated)
	if err != nil {
		t.Fatal(err)
	}
	if len(cs.Changes) != 1 || cs.Changes[0].Action != "Add" {
		t.Fatalf("changes=%+v", cs.Changes)
	}
	got, err := st.ExecuteCFNChangeSet(account, "us-east-1", cs.ChangeSetID, "cs-stack")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Resources) != 2 {
		t.Fatalf("resources=%d", len(got.Resources))
	}
	if _, err := st.GetQueue(account, "cfn-cs-queue-1"); err != nil {
		t.Fatalf("queue: %v", err)
	}

	withTopic := `{
  "Resources": {
    "LabBucket": {
      "Type": "AWS::S3::Bucket",
      "Properties": { "BucketName": "cfn-cs-bucket-1" }
    },
    "LabQueue": {
      "Type": "AWS::SQS::Queue",
      "Properties": { "QueueName": "cfn-cs-queue-1" }
    },
    "LabTopic": {
      "Type": "AWS::SNS::Topic",
      "Properties": { "TopicName": "cfn-cs-topic-1" }
    }
  }
}`
	if _, err := st.UpdateCFNStack(account, "us-east-1", "cs-stack", withTopic); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetTopic(account, "cfn-cs-topic-1"); err != nil {
		t.Fatalf("topic: %v", err)
	}
}

func TestCFNChangeSetModifySSMParameter(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	base := `{
  "Resources": {
    "LabParam": {
      "Type": "AWS::SSM::Parameter",
      "Properties": {
        "Name": "/lab/cfn-mod-param",
        "Type": "String",
        "Value": "v1"
      }
    }
  }
}`
	if _, err := st.CreateCFNStack(account, "us-east-1", "mod-ssm-stack", base, ""); err != nil {
		t.Fatal(err)
	}
	changed := `{
  "Resources": {
    "LabParam": {
      "Type": "AWS::SSM::Parameter",
      "Properties": {
        "Name": "/lab/cfn-mod-param",
        "Type": "String",
        "Value": "v2"
      }
    }
  }
}`
	cs, err := st.CreateCFNChangeSet(account, "us-east-1", "mod-ssm-stack", "bump-value", changed)
	if err != nil {
		t.Fatal(err)
	}
	if len(cs.Changes) != 1 || cs.Changes[0].Action != "Modify" {
		t.Fatalf("changes=%+v", cs.Changes)
	}
	if _, err := st.ExecuteCFNChangeSet(account, "us-east-1", cs.ChangeSetID, "mod-ssm-stack"); err != nil {
		t.Fatal(err)
	}
	p, err := st.GetParameter(account, "/lab/cfn-mod-param", true)
	if err != nil {
		t.Fatal(err)
	}
	if p.Value != "v2" {
		t.Fatalf("value=%q", p.Value)
	}
}

func TestCFNChangeSetModifyUnsupportedFailsClosed(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	base := `{
  "Resources": {
    "LabTable": {
      "Type": "AWS::DynamoDB::Table",
      "Properties": {
        "TableName": "cfn-mod-ddb",
        "BillingMode": "PAY_PER_REQUEST",
        "AttributeDefinitions": [{"AttributeName":"pk","AttributeType":"S"}],
        "KeySchema": [{"AttributeName":"pk","KeyType":"HASH"}]
      }
    }
  }
}`
	if _, err := st.CreateCFNStack(account, "us-east-1", "mod-ddb-stack", base, ""); err != nil {
		t.Fatal(err)
	}
	changed := `{
  "Resources": {
    "LabTable": {
      "Type": "AWS::DynamoDB::Table",
      "Properties": {
        "TableName": "cfn-mod-ddb",
        "BillingMode": "PAY_PER_REQUEST",
        "AttributeDefinitions": [
          {"AttributeName":"pk","AttributeType":"S"},
          {"AttributeName":"sk","AttributeType":"S"}
        ],
        "KeySchema": [
          {"AttributeName":"pk","KeyType":"HASH"},
          {"AttributeName":"sk","KeyType":"RANGE"}
        ]
      }
    }
  }
}`
	cs, err := st.CreateCFNChangeSet(account, "us-east-1", "mod-ddb-stack", "bad-key", changed)
	if err != nil {
		t.Fatal(err)
	}
	if len(cs.Changes) != 1 || cs.Changes[0].Action != "Modify" {
		t.Fatalf("changes=%+v", cs.Changes)
	}
	_, err = st.ExecuteCFNChangeSet(account, "us-east-1", cs.ChangeSetID, "mod-ddb-stack")
	if err == nil || (!strings.Contains(err.Error(), "fail-closed") && !strings.Contains(err.Error(), "immutable")) {
		t.Fatalf("expected fail-closed key schema reject, err=%v", err)
	}
}

func TestCFNChangeSetModifyBucketRenameFailsClosed(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	base := `{
  "Resources": {
    "LabBucket": {
      "Type": "AWS::S3::Bucket",
      "Properties": { "BucketName": "cfn-mod-bucket" }
    }
  }
}`
	if _, err := st.CreateCFNStack(account, "us-east-1", "mod-stack", base, ""); err != nil {
		t.Fatal(err)
	}
	changed := `{
  "Resources": {
    "LabBucket": {
      "Type": "AWS::S3::Bucket",
      "Properties": { "BucketName": "cfn-mod-bucket-renamed" }
    }
  }
}`
	cs, err := st.CreateCFNChangeSet(account, "us-east-1", "mod-stack", "rename", changed)
	if err != nil {
		t.Fatal(err)
	}
	if len(cs.Changes) != 1 || cs.Changes[0].Action != "Modify" {
		t.Fatalf("changes=%+v", cs.Changes)
	}
	_, err = st.ExecuteCFNChangeSet(account, "us-east-1", cs.ChangeSetID, "mod-stack")
	if err == nil || (!strings.Contains(err.Error(), "fail-closed") && !strings.Contains(err.Error(), "cannot change")) {
		t.Fatalf("expected bucket rename reject, err=%v", err)
	}
}

func TestCFNFullstackTypesAndDependsOn(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	tpl := `{
  "Resources": {
    "LabKey": {
      "Type": "AWS::KMS::Key",
      "Properties": {
        "Description": "lab",
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
    "LabRole": {
      "Type": "AWS::IAM::Role",
      "Properties": {
        "RoleName": "CfnFullRole",
        "AssumeRolePolicyDocument": {
          "Version": "2012-10-17",
          "Statement": [{
            "Effect": "Allow",
            "Principal": {"Service": "lambda.amazonaws.com"},
            "Action": "sts:AssumeRole"
          }]
        },
        "Policies": [{
          "PolicyName": "inline",
          "PolicyDocument": {
            "Version": "2012-10-17",
            "Statement": [{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"}]
          }
        }]
      }
    },
    "LabBucket": {
      "Type": "AWS::S3::Bucket",
      "DependsOn": ["LabKey"],
      "Properties": {
        "BucketName": "cfn-full-bucket",
        "BucketEncryption": {
          "ServerSideEncryptionConfiguration": [{
            "ServerSideEncryptionByDefault": {
              "SSEAlgorithm": "aws:kms",
              "KMSMasterKeyID": {"Fn::GetAtt": ["LabKey", "Arn"]}
            }
          }]
        }
      }
    },
    "LabTable": {
      "Type": "AWS::DynamoDB::Table",
      "Properties": {
        "TableName": "cfn-full-table",
        "BillingMode": "PAY_PER_REQUEST",
        "AttributeDefinitions": [{"AttributeName":"pk","AttributeType":"S"}],
        "KeySchema": [{"AttributeName":"pk","KeyType":"HASH"}],
        "SSESpecification": {
          "SSEEnabled": true,
          "KMSMasterKeyId": {"Fn::GetAtt": ["LabKey", "Arn"]}
        }
      }
    },
    "LabQueue": {
      "Type": "AWS::SQS::Queue",
      "Properties": { "QueueName": "cfn-full-queue" }
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
            "Principal": "*",
            "Action": "sqs:SendMessage",
            "Resource": "*"
          }]
        }
      }
    },
    "LabTopic": {
      "Type": "AWS::SNS::Topic",
      "Properties": { "TopicName": "cfn-full-topic" }
    },
    "LabBus": {
      "Type": "AWS::Events::EventBus",
      "Properties": { "Name": "cfn-full-bus" }
    },
    "LabRule": {
      "Type": "AWS::Events::Rule",
      "DependsOn": ["LabBus", "LabQueue"],
      "Properties": {
        "Name": "cfn-full-rule",
        "EventBusName": {"Ref": "LabBus"},
        "EventPattern": {"source": ["noctaxris.lab"]},
        "Targets": [{
          "Id": "q",
          "Arn": {"Fn::GetAtt": ["LabQueue", "Arn"]}
        }]
      }
    },
    "LabParam": {
      "Type": "AWS::SSM::Parameter",
      "Properties": {
        "Name": "/lab/cfn-full",
        "Type": "String",
        "Value": "ok"
      }
    },
    "LabSecret": {
      "Type": "AWS::SecretsManager::Secret",
      "Properties": {
        "Name": "cfn-full-secret",
        "SecretString": "{\"token\":\"x\"}"
      }
    },
    "LabFn": {
      "Type": "AWS::Lambda::Function",
      "DependsOn": ["LabRole"],
      "Properties": {
        "FunctionName": "cfn-full-fn",
        "Role": {"Fn::GetAtt": ["LabRole", "Arn"]},
        "Runtime": "python3.12",
        "Handler": "index.handler",
        "Code": { "ZipFile": "def handler(event, context):\n    return event\n" }
      }
    }
  }
}`
	created, err := st.CreateCFNStack(account, "us-east-1", "full-stack", tpl, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(created.Resources) < 12 {
		t.Fatalf("resources=%d", len(created.Resources))
	}
	if _, err := st.GetParameter(account, "/lab/cfn-full", true); err != nil {
		t.Fatalf("param: %v", err)
	}
	if err := st.DeleteCFNStack(account, "full-stack"); err != nil {
		t.Fatal(err)
	}
}

func TestCFNNestedTemplateURLDriftAndAllowlist(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	if _, err := st.CreateBucket(account, "cfn-tpl-bucket"); err != nil {
		t.Fatal(err)
	}
	childTpl := `{"Resources":{"ChildBucket":{"Type":"AWS::S3::Bucket","Properties":{"BucketName":"cfn-nested-child-bucket"}}}}`
	if _, err := st.PutObject(account, "cfn-tpl-bucket", "child.json", store.PutObjectMeta{
		ContentType: "application/json",
		Data:        []byte(childTpl),
		PlainSize:   int64(len(childTpl)),
	}); err != nil {
		t.Fatal(err)
	}

	_, err := st.CreateCFNStack(account, "us-east-1", "nested-bad", `{
  "Resources": {
    "Nested": {
      "Type": "AWS::CloudFormation::Stack",
      "Properties": { "TemplateURL": "https://example.com/evil.json" }
    }
  }
}`, "")
	if err == nil || !strings.Contains(err.Error(), "allowlisted") {
		t.Fatalf("expected allowlist reject, err=%v", err)
	}

	parent := `{
  "Resources": {
    "Nested": {
      "Type": "AWS::CloudFormation::Stack",
      "Properties": { "TemplateURL": "s3://cfn-tpl-bucket/child.json" }
    }
  }
}`
	created, err := st.CreateCFNStack(account, "us-east-1", "nested-parent", parent, "")
	if err != nil {
		t.Fatal(err)
	}
	if created.Resources[0].ResourceType != "AWS::CloudFormation::Stack" {
		t.Fatalf("resource=%+v", created.Resources[0])
	}
	if _, err := st.GetBucket(account, "cfn-nested-child-bucket"); err != nil {
		t.Fatalf("child bucket: %v", err)
	}
	det, err := st.DetectCFNStackDrift(account, "nested-parent")
	if err != nil {
		t.Fatal(err)
	}
	drifts, err := st.DescribeCFNStackResourceDrifts(account, "nested-parent")
	if err != nil {
		t.Fatal(err)
	}
	if len(drifts) == 0 || drifts[0].StackResourceDriftStatus != "NOT_CHECKED" {
		t.Fatalf("drift=%+v det=%+v", drifts, det)
	}
	if err := st.DeleteCFNStack(account, "nested-parent"); err != nil {
		t.Fatal(err)
	}
}

func TestCFNUnknownPropertyRejected(t *testing.T) {
	st := openTestStore(t)
	_, err := st.CreateCFNStack("000000000001", "us-east-1", "bad-props", `{
  "Resources": {
    "LabBucket": {
      "Type": "AWS::S3::Bucket",
      "Properties": {
        "BucketName": "cfn-bad-props",
        "WebsiteConfiguration": { "IndexDocument": "index.html" }
      }
    }
  }
}`, "")
	if err == nil || !strings.Contains(err.Error(), "unsupported property") {
		t.Fatalf("expected property reject, err=%v", err)
	}
}

func TestCloudControlBucket(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	_, _, err := st.CloudControlCreateResource(account, "AWS::EC2::Instance", `{"InstanceType":"t3.micro"}`)
	if err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("unknown type err=%v", err)
	}
	bucket, token, err := st.CloudControlCreateResource(account, "AWS::S3::Bucket", `{"BucketName":"cc-lab-bucket"}`)
	if err != nil {
		t.Fatal(err)
	}
	if bucket.Identifier != "cc-lab-bucket" || token == "" {
		t.Fatalf("bucket=%+v token=%q", bucket, token)
	}
	stStatus, err := st.CloudControlGetResourceRequestStatus(account, token)
	if err != nil || stStatus.Status != "SUCCESS" {
		t.Fatalf("status=%+v err=%v", stStatus, err)
	}
	if _, err := st.CloudControlDeleteResource(account, "AWS::S3::Bucket", "cc-lab-bucket"); err != nil {
		t.Fatal(err)
	}
}