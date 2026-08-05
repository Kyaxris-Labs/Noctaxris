package store_test

import (
	"testing"
)

func TestCFNDriftMoreResourceTypes(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	tpl := `{
  "Resources": {
    "Role": {
      "Type": "AWS::IAM::Role",
      "Properties": {
        "RoleName": "CfnDriftMoreRole",
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
        "FunctionName": "cfn-drift-fn",
        "Runtime": "nodejs22.x",
        "Handler": "index.handler",
        "Role": {"Fn::GetAtt": ["Role", "Arn"]},
        "Code": { "ZipFile": "exports.handler=async()=>({ok:true})" }
      }
    },
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
        "AliasName": "alias/cfn-drift-alias",
        "TargetKeyId": {"Ref": "Key"}
      }
    },
    "Sec": {
      "Type": "AWS::SecretsManager::Secret",
      "Properties": {
        "Name": "cfn-drift-secret",
        "SecretString": "secret-value"
      }
    },
    "Topic": {
      "Type": "AWS::SNS::Topic",
      "Properties": { "TopicName": "cfn-drift-more-topic" }
    },
    "TopicPolicy": {
      "Type": "AWS::SNS::TopicPolicy",
      "Properties": {
        "Topics": [{"Ref": "Topic"}],
        "PolicyDocument": {
          "Version": "2012-10-17",
          "Statement": [{"Effect":"Allow","Principal":"*","Action":"sns:Publish","Resource":"*"}]
        }
      }
    },
    "Pol": {
      "Type": "AWS::IAM::ManagedPolicy",
      "Properties": {
        "ManagedPolicyName": "CfnDriftPol",
        "PolicyDocument": {
          "Version": "2012-10-17",
          "Statement": [{"Effect":"Allow","Action":"s3:ListBucket","Resource":"*"}]
        }
      }
    }
  }
}`
	if _, err := st.CreateCFNStack(account, "us-east-1", "drift-more", tpl, "", "CAPABILITY_NAMED_IAM"); err != nil {
		t.Fatal(err)
	}
	det, err := st.DetectCFNStackDrift(account, "drift-more")
	if err != nil {
		t.Fatal(err)
	}
	if det.DetectionStatus != "DETECTION_COMPLETE" {
		t.Fatalf("det=%+v", det)
	}
	drifts, err := st.DescribeCFNStackResourceDrifts(account, "drift-more")
	if err != nil {
		t.Fatal(err)
	}
	if len(drifts) < 6 {
		t.Fatalf("drifts=%v", drifts)
	}
	types := map[string]bool{}
	for _, d := range drifts {
		types[d.ResourceType] = true
		if d.StackResourceDriftStatus != "IN_SYNC" && d.StackResourceDriftStatus != "NOT_CHECKED" {
			t.Fatalf("unexpected status %+v", d)
		}
	}
	for _, want := range []string{
		"AWS::Lambda::Function", "AWS::KMS::Key", "AWS::KMS::Alias",
		"AWS::SecretsManager::Secret", "AWS::SNS::Topic", "AWS::IAM::ManagedPolicy",
	} {
		if !types[want] {
			t.Fatalf("missing type %s in %v", want, types)
		}
	}
}
