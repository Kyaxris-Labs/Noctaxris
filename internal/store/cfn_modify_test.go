package store_test

import (
	"strings"
	"testing"
)

func TestCFNModifyBucketPolicyQueuePolicyAndAlias(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	base := `{
  "Resources": {
    "Key": {
      "Type": "AWS::KMS::Key",
      "Properties": {
        "KeyPolicy": {
          "Version": "2012-10-17",
          "Statement": [{"Effect":"Allow","Principal":{"AWS":"*"},"Action":"kms:*","Resource":"*"}]
        }
      }
    },
    "Alias": {
      "Type": "AWS::KMS::Alias",
      "Properties": {
        "AliasName": "alias/cfn-mod-alias",
        "TargetKeyId": {"Ref": "Key"}
      }
    },
    "Bucket": {
      "Type": "AWS::S3::Bucket",
      "Properties": { "BucketName": "cfn-mod-bp-bucket" }
    },
    "BucketPolicy": {
      "Type": "AWS::S3::BucketPolicy",
      "Properties": {
        "Bucket": {"Ref": "Bucket"},
        "PolicyDocument": {
          "Version": "2012-10-17",
          "Statement": [{"Effect":"Allow","Principal":"*","Action":"s3:GetObject","Resource":"arn:aws:s3:::cfn-mod-bp-bucket/*"}]
        }
      }
    },
    "Queue": {
      "Type": "AWS::SQS::Queue",
      "Properties": { "QueueName": "cfn-mod-qp" }
    },
    "QueuePolicy": {
      "Type": "AWS::SQS::QueuePolicy",
      "Properties": {
        "Queues": [{"Ref": "Queue"}],
        "PolicyDocument": {
          "Version": "2012-10-17",
          "Statement": [{"Effect":"Allow","Principal":"*","Action":"sqs:SendMessage","Resource":"*"}]
        }
      }
    }
  }
}`
	if _, err := st.CreateCFNStack(account, "us-east-1", "mod-pol", base, "", "CAPABILITY_NAMED_IAM"); err != nil {
		t.Fatal(err)
	}
	changed := `{
  "Resources": {
    "Key": {
      "Type": "AWS::KMS::Key",
      "Properties": {
        "EnableKeyRotation": true,
        "KeyPolicy": {
          "Version": "2012-10-17",
          "Statement": [{"Effect":"Allow","Principal":{"AWS":"*"},"Action":"kms:*","Resource":"*"}]
        }
      }
    },
    "Alias": {
      "Type": "AWS::KMS::Alias",
      "Properties": {
        "AliasName": "alias/cfn-mod-alias",
        "TargetKeyId": {"Ref": "Key"}
      }
    },
    "Bucket": {
      "Type": "AWS::S3::Bucket",
      "Properties": {
        "BucketName": "cfn-mod-bp-bucket",
        "BucketEncryption": {
          "ServerSideEncryptionConfiguration": [{
            "ServerSideEncryptionByDefault": { "SSEAlgorithm": "AES256" }
          }]
        }
      }
    },
    "BucketPolicy": {
      "Type": "AWS::S3::BucketPolicy",
      "Properties": {
        "Bucket": {"Ref": "Bucket"},
        "PolicyDocument": {
          "Version": "2012-10-17",
          "Statement": [{"Effect":"Allow","Principal":"*","Action":"s3:PutObject","Resource":"arn:aws:s3:::cfn-mod-bp-bucket/*"}]
        }
      }
    },
    "Queue": {
      "Type": "AWS::SQS::Queue",
      "Properties": { "QueueName": "cfn-mod-qp", "VisibilityTimeout": 45 }
    },
    "QueuePolicy": {
      "Type": "AWS::SQS::QueuePolicy",
      "Properties": {
        "Queues": [{"Ref": "Queue"}],
        "PolicyDocument": {
          "Version": "2012-10-17",
          "Statement": [{"Effect":"Allow","Principal":"*","Action":"sqs:ReceiveMessage","Resource":"*"}]
        }
      }
    }
  }
}`
	if _, err := st.UpdateCFNStack(account, "us-east-1", "mod-pol", changed, "CAPABILITY_NAMED_IAM"); err != nil {
		t.Fatal(err)
	}
	pol, err := st.GetBucketPolicy(account, "cfn-mod-bp-bucket")
	if err != nil || !strings.Contains(pol, "s3:PutObject") {
		t.Fatalf("bucket policy=%q err=%v", pol, err)
	}
}

func TestCFNModifyLambdaPermissionAndEventRule(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	base := `{
  "Resources": {
    "Role": {
      "Type": "AWS::IAM::Role",
      "Properties": {
        "RoleName": "CfnModPermRole",
        "AssumeRolePolicyDocument": {
          "Version": "2012-10-17",
          "Statement": [{
            "Effect": "Allow",
            "Principal": {"Service": "lambda.amazonaws.com"},
            "Action": "sts:AssumeRole"
          }]
        }
      }
    },
    "Fn": {
      "Type": "AWS::Lambda::Function",
      "Properties": {
        "FunctionName": "cfn-mod-perm-fn",
        "Runtime": "nodejs22.x",
        "Handler": "index.handler",
        "Role": {"Fn::GetAtt": ["Role", "Arn"]},
        "Code": { "ZipFile": "exports.handler=async()=>({ok:true})" }
      }
    },
    "Perm": {
      "Type": "AWS::Lambda::Permission",
      "Properties": {
        "FunctionName": {"Ref": "Fn"},
        "Action": "lambda:InvokeFunction",
        "Principal": "events.amazonaws.com",
        "SourceArn": "arn:aws:events:us-east-1:000000000001:rule/cfn-mod-rule"
      }
    },
    "Bus": {
      "Type": "AWS::Events::EventBus",
      "Properties": { "Name": "cfn-mod-rule-bus" }
    },
    "Rule": {
      "Type": "AWS::Events::Rule",
      "Properties": {
        "Name": "cfn-mod-rule",
        "EventBusName": {"Ref": "Bus"},
        "EventPattern": { "source": ["lab"] },
        "State": "ENABLED",
        "Targets": [{
          "Id": "t1",
          "Arn": {"Fn::GetAtt": ["Fn", "Arn"]}
        }]
      }
    }
  }
}`
	if _, err := st.CreateCFNStack(account, "us-east-1", "mod-perm", base, "", "CAPABILITY_NAMED_IAM"); err != nil {
		t.Fatal(err)
	}
	changed := `{
  "Resources": {
    "Role": {
      "Type": "AWS::IAM::Role",
      "Properties": {
        "RoleName": "CfnModPermRole",
        "AssumeRolePolicyDocument": {
          "Version": "2012-10-17",
          "Statement": [{
            "Effect": "Allow",
            "Principal": {"Service": "lambda.amazonaws.com"},
            "Action": "sts:AssumeRole"
          }]
        }
      }
    },
    "Fn": {
      "Type": "AWS::Lambda::Function",
      "Properties": {
        "FunctionName": "cfn-mod-perm-fn",
        "Runtime": "nodejs22.x",
        "Handler": "index.handler",
        "Role": {"Fn::GetAtt": ["Role", "Arn"]},
        "Timeout": 15,
        "Code": { "ZipFile": "exports.handler=async()=>({ok:true})" }
      }
    },
    "Perm": {
      "Type": "AWS::Lambda::Permission",
      "Properties": {
        "FunctionName": {"Ref": "Fn"},
        "Action": "lambda:InvokeFunction",
        "Principal": "apigateway.amazonaws.com",
        "SourceArn": "arn:aws:execute-api:us-east-1:000000000001:api/*"
      }
    },
    "Bus": {
      "Type": "AWS::Events::EventBus",
      "Properties": { "Name": "cfn-mod-rule-bus" }
    },
    "Rule": {
      "Type": "AWS::Events::Rule",
      "Properties": {
        "Name": "cfn-mod-rule",
        "EventBusName": {"Ref": "Bus"},
        "EventPattern": { "source": ["lab2"] },
        "State": "DISABLED",
        "Targets": [{
          "Id": "t1",
          "Arn": {"Fn::GetAtt": ["Fn", "Arn"]}
        }]
      }
    }
  }
}`
	if _, err := st.UpdateCFNStack(account, "us-east-1", "mod-perm", changed, "CAPABILITY_NAMED_IAM"); err != nil {
		t.Fatal(err)
	}
	rule, err := st.DescribeRule(account, "cfn-mod-rule-bus", "cfn-mod-rule")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(rule.State, "DISABLED") {
		t.Fatalf("rule state=%q", rule.State)
	}
}

func TestCFNModifyDynamoSSEAndManagedPolicy(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	base := `{
  "Resources": {
    "Key": {
      "Type": "AWS::KMS::Key",
      "Properties": {
        "KeyPolicy": {
          "Version": "2012-10-17",
          "Statement": [{"Effect":"Allow","Principal":{"AWS":"*"},"Action":"kms:*","Resource":"*"}]
        }
      }
    },
    "Table": {
      "Type": "AWS::DynamoDB::Table",
      "Properties": {
        "TableName": "cfn-mod-sse-ddb",
        "BillingMode": "PAY_PER_REQUEST",
        "AttributeDefinitions": [{"AttributeName":"pk","AttributeType":"S"}],
        "KeySchema": [{"AttributeName":"pk","KeyType":"HASH"}]
      }
    },
    "Pol": {
      "Type": "AWS::IAM::ManagedPolicy",
      "Properties": {
        "ManagedPolicyName": "CfnModManaged",
        "PolicyDocument": {
          "Version": "2012-10-17",
          "Statement": [{"Effect":"Allow","Action":"s3:ListBucket","Resource":"*"}]
        }
      }
    }
  }
}`
	if _, err := st.CreateCFNStack(account, "us-east-1", "mod-ddb-sse", base, "", "CAPABILITY_NAMED_IAM"); err != nil {
		t.Fatal(err)
	}
	changed := `{
  "Resources": {
    "Key": {
      "Type": "AWS::KMS::Key",
      "Properties": {
        "KeyPolicy": {
          "Version": "2012-10-17",
          "Statement": [{"Effect":"Allow","Principal":{"AWS":"*"},"Action":"kms:*","Resource":"*"}]
        }
      }
    },
    "Table": {
      "Type": "AWS::DynamoDB::Table",
      "Properties": {
        "TableName": "cfn-mod-sse-ddb",
        "BillingMode": "PAY_PER_REQUEST",
        "AttributeDefinitions": [{"AttributeName":"pk","AttributeType":"S"}],
        "KeySchema": [{"AttributeName":"pk","KeyType":"HASH"}],
        "SSESpecification": {
          "SSEEnabled": true,
          "KMSMasterKeyId": {"Fn::GetAtt": ["Key", "Arn"]}
        }
      }
    },
    "Pol": {
      "Type": "AWS::IAM::ManagedPolicy",
      "Properties": {
        "ManagedPolicyName": "CfnModManaged",
        "PolicyDocument": {
          "Version": "2012-10-17",
          "Statement": [{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"}]
        }
      }
    }
  }
}`
	if _, err := st.UpdateCFNStack(account, "us-east-1", "mod-ddb-sse", changed, "CAPABILITY_NAMED_IAM"); err != nil {
		t.Fatal(err)
	}
}
