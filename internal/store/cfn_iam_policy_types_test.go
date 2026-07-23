package store_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestCFNManagedPolicyAndPolicyAttachRole(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	tpl := `{
	  "Resources": {
	    "R": {
	      "Type": "AWS::IAM::Role",
	      "Properties": {
	        "RoleName": "cfn-pol-role",
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
	    "P": {
	      "Type": "AWS::IAM::ManagedPolicy",
	      "DependsOn": ["R"],
	      "Properties": {
	        "ManagedPolicyName": "cfn-lab-mp",
	        "PolicyDocument": {
	          "Version": "2012-10-17",
	          "Statement": [{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"}]
	        },
	        "Roles": ["cfn-pol-role"]
	      }
	    },
	    "InlineStyle": {
	      "Type": "AWS::IAM::Policy",
	      "DependsOn": ["R"],
	      "Properties": {
	        "PolicyName": "cfn-lab-policy",
	        "PolicyDocument": {
	          "Version": "2012-10-17",
	          "Statement": [{"Effect":"Allow","Action":"sqs:SendMessage","Resource":"*"}]
	        },
	        "Roles": [{"Ref": "R"}]
	      }
	    }
	  }
	}`
	created, err := st.CreateCFNStack(account, "us-east-1", "pol-stack", tpl, "")
	if err != nil {
		t.Fatal(err)
	}
	roleARN := "arn:aws:iam::" + account + ":role/cfn-pol-role"
	docs, err := st.ListAttachedPolicyDocuments(roleARN)
	if err != nil || len(docs) < 2 {
		t.Fatalf("attached=%v err=%v", docs, err)
	}
	mpARN := store.PolicyARN(account, "/", "cfn-lab-mp")
	if _, err := st.GetManagedPolicy(mpARN); err != nil {
		t.Fatalf("managed policy: %v", err)
	}
	policyARN := store.PolicyARN(account, "/", "cfn-lab-policy")
	if _, err := st.GetManagedPolicy(policyARN); err != nil {
		t.Fatalf("iam policy resource: %v", err)
	}
	if err := st.DeleteCFNStack(account, created.StackName); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetManagedPolicy(mpARN); err == nil {
		t.Fatal("expected managed policy deleted with stack")
	}
}

func TestCFNManagedPolicyAttachGroupAndUser(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	if err := st.EnsureRoot(account, "AKIAROOTEXAMPLE01", "secret"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.CreateUser(account, "cfn-pol-user"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.CreateGroup(account, "cfn-pol-group"); err != nil {
		t.Fatal(err)
	}
	tpl := `{
	  "Resources": {
	    "P": {
	      "Type": "AWS::IAM::ManagedPolicy",
	      "Properties": {
	        "ManagedPolicyName": "cfn-lab-mp-ug",
	        "PolicyDocument": {
	          "Version": "2012-10-17",
	          "Statement": [{"Effect":"Allow","Action":"s3:ListBucket","Resource":"*"}]
	        },
	        "Users": ["cfn-pol-user"],
	        "Groups": ["cfn-pol-group"]
	      }
	    }
	  }
	}`
	if _, err := st.CreateCFNStack(account, "us-east-1", "pol-ug-stack", tpl, ""); err != nil {
		t.Fatal(err)
	}
	userARN := store.UserARN(account, "/", "cfn-pol-user")
	docs, err := st.ListAttachedPolicyDocuments(userARN)
	if err != nil || len(docs) == 0 {
		t.Fatalf("user attached=%v err=%v", docs, err)
	}
	groupARN := store.GroupARN(account, "/", "cfn-pol-group")
	docs, err = st.ListAttachedPolicyDocuments(groupARN)
	if err != nil || len(docs) == 0 {
		t.Fatalf("group attached=%v err=%v", docs, err)
	}
}

func TestCFNBucketPolicyAndLambdaPermission(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	tpl := `{
	  "Resources": {
	    "Bucket": {
	      "Type": "AWS::S3::Bucket",
	      "Properties": { "BucketName": "cfn-bp-bucket" }
	    },
	    "BucketPolicy": {
	      "Type": "AWS::S3::BucketPolicy",
	      "DependsOn": ["Bucket"],
	      "Properties": {
	        "Bucket": { "Ref": "Bucket" },
	        "PolicyDocument": {
	          "Version": "2012-10-17",
	          "Statement": [{
	            "Effect": "Allow",
	            "Principal": { "AWS": "arn:aws:iam::000000000001:root" },
	            "Action": "s3:GetObject",
	            "Resource": "arn:aws:s3:::cfn-bp-bucket/*"
	          }]
	        }
	      }
	    },
	    "Role": {
	      "Type": "AWS::IAM::Role",
	      "Properties": {
	        "RoleName": "cfn-perm-role",
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
	      "DependsOn": ["Role"],
	      "Properties": {
	        "FunctionName": "cfn-perm-fn",
	        "Role": { "Fn::GetAtt": ["Role", "Arn"] },
	        "Runtime": "python3.12",
	        "Handler": "index.handler",
	        "Code": { "ZipFile": "def handler(event, context):\n    return event\n" }
	      }
	    },
	    "Perm": {
	      "Type": "AWS::Lambda::Permission",
	      "DependsOn": ["Fn", "Bucket"],
	      "Properties": {
	        "FunctionName": { "Ref": "Fn" },
	        "Action": "lambda:InvokeFunction",
	        "Principal": "s3.amazonaws.com",
	        "SourceArn": "arn:aws:s3:::cfn-bp-bucket",
	        "SourceAccount": "000000000001",
	        "StatementId": "AllowS3Invoke"
	      }
	    }
	  }
	}`
	created, err := st.CreateCFNStack(account, "us-east-1", "bp-perm-stack", tpl, "")
	if err != nil {
		t.Fatal(err)
	}
	policy, err := st.GetBucketPolicy(account, "cfn-bp-bucket")
	if err != nil {
		t.Fatalf("bucket policy: %v", err)
	}
	if !strings.Contains(policy, "s3:GetObject") {
		t.Fatalf("policy=%s", policy)
	}
	fnPolicy, err := st.GetFunctionPolicy(account, "cfn-perm-fn")
	if err != nil {
		t.Fatalf("function policy: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(fnPolicy), &doc); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(doc)
	if !strings.Contains(string(raw), "s3.amazonaws.com") || !strings.Contains(string(raw), "AllowS3Invoke") {
		t.Fatalf("function policy=%s", fnPolicy)
	}
	if !strings.Contains(string(raw), "arn:aws:s3:::cfn-bp-bucket") {
		t.Fatalf("missing SourceArn condition in %s", fnPolicy)
	}
	if err := st.DeleteCFNStack(account, created.StackName); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetBucketPolicy(account, "cfn-bp-bucket"); err == nil {
		t.Fatal("expected bucket policy cleared on stack delete")
	}
	if _, err := st.GetFunctionPolicy(account, "cfn-perm-fn"); err == nil {
		t.Fatal("expected function permission removed on stack delete")
	}
}

func TestCFNLambdaPermissionRejectsUnknownProperty(t *testing.T) {
	st := openTestStore(t)
	_, err := st.CreateCFNStack("000000000001", "us-east-1", "bad-perm", `{
	  "Resources": {
	    "Perm": {
	      "Type": "AWS::Lambda::Permission",
	      "Properties": {
	        "FunctionName": "missing",
	        "Action": "lambda:InvokeFunction",
	        "Principal": "s3.amazonaws.com",
	        "PrincipalOrgID": "o-abc1234567"
	      }
	    }
	  }
	}`, "")
	if err == nil || !strings.Contains(err.Error(), "unsupported property") {
		t.Fatalf("expected unknown property reject, err=%v", err)
	}
}
