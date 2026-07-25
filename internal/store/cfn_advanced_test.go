package store_test

import (
	"strings"
	"testing"
)

func TestCFNYAMLTemplateWithRef(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	tpl := `
AWSTemplateFormatVersion: "2010-09-09"
Resources:
  LabBucket:
    Type: AWS::S3::Bucket
    Properties:
      BucketName: cfn-yaml-bucket-1
  LabRole:
    Type: AWS::IAM::Role
    Properties:
      RoleName: CfnYamlRole1
      AssumeRolePolicyDocument:
        Version: "2012-10-17"
        Statement:
          - Effect: Allow
            Principal:
              Service: lambda.amazonaws.com
            Action: sts:AssumeRole
  LabQueue:
    Type: AWS::SQS::Queue
    Properties:
      QueueName: !Sub "${LabBucket}-q"
`
	created, err := st.CreateCFNStack(account, "us-east-1", "yaml-stack", tpl, "", "CAPABILITY_NAMED_IAM")
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != "CREATE_COMPLETE" {
		t.Fatalf("status=%s", created.Status)
	}
	if _, err := st.GetBucketByName("cfn-yaml-bucket-1"); err != nil {
		t.Fatalf("bucket: %v", err)
	}
	if _, _, err := st.GetRole(account, "CfnYamlRole1"); err != nil {
		t.Fatalf("role: %v", err)
	}
	q, err := st.GetQueue(account, "cfn-yaml-bucket-1-q")
	if err != nil {
		t.Fatalf("queue: %v", err)
	}
	if !strings.Contains(q.QueueURL, "cfn-yaml-bucket-1-q") {
		t.Fatalf("queue url=%q", q.QueueURL)
	}
	if err := st.DeleteCFNStack(account, "yaml-stack"); err != nil {
		t.Fatal(err)
	}
}

func TestCFNIntrinsicsGetAttAndJoin(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	tpl := `{
  "Resources": {
    "LabRole": {
      "Type": "AWS::IAM::Role",
      "Properties": {
        "RoleName": "CfnGetAttRole",
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
    "LabFn": {
      "Type": "AWS::Lambda::Function",
      "Properties": {
        "FunctionName": "cfn-getatt-fn",
        "Role": {"Fn::GetAtt": ["LabRole", "Arn"]},
        "Runtime": "python3.12",
        "Handler": "index.handler",
        "Code": {
          "ZipFile": "def handler(event, context):\n    return event\n"
        },
        "Description": {"Fn::Join": ["-", [{"Ref": "AWS::StackName"}, "fn"]]}
      }
    }
  }
}`
	if _, err := st.CreateCFNStack(account, "us-east-1", "getatt-stack", tpl, "", "CAPABILITY_NAMED_IAM"); err != nil {
		t.Fatal(err)
	}
	fn, err := st.GetFunction(account, "cfn-getatt-fn")
	if err != nil {
		t.Fatal(err)
	}
	if fn.RoleARN != "arn:aws:iam::000000000001:role/CfnGetAttRole" {
		t.Fatalf("role=%q", fn.RoleARN)
	}
	if err := st.DeleteCFNStack(account, "getatt-stack"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetFunction(account, "cfn-getatt-fn"); err == nil {
		t.Fatal("expected function deleted with stack")
	}
}

func TestCFNDynamoDBTableType(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	tpl := `{
  "Resources": {
    "LabTable": {
      "Type": "AWS::DynamoDB::Table",
      "Properties": {
        "TableName": "cfn-ddb-1",
        "BillingMode": "PAY_PER_REQUEST",
        "AttributeDefinitions": [
          {"AttributeName": "pk", "AttributeType": "S"}
        ],
        "KeySchema": [
          {"AttributeName": "pk", "KeyType": "HASH"}
        ]
      }
    }
  }
}`
	if _, err := st.CreateCFNStack(account, "us-east-1", "ddb-stack", tpl, "", "CAPABILITY_NAMED_IAM"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetTable(account, "cfn-ddb-1"); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteCFNStack(account, "ddb-stack"); err != nil {
		t.Fatal(err)
	}
}
