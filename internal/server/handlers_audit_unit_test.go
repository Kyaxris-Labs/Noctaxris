package server

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAuditEventSourceFromJSONTarget(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"TrentService.CreateKey":                         "kms.amazonaws.com",
		"AWSKMS.Decrypt":                                 "kms.amazonaws.com",
		"AWSSecretsManager.GetSecretValue":               "secretsmanager.amazonaws.com",
		"secretsmanager.GetSecretValue":                  "secretsmanager.amazonaws.com",
		"AmazonSSM.GetParameter":                         "ssm.amazonaws.com",
		"AWSLambda.Invoke":                               "lambda.amazonaws.com",
		"DynamoDB_20120810.PutItem":                      "dynamodb.amazonaws.com",
		"Logs_20140328.CreateLogGroup":                   "logs.amazonaws.com",
		"Logs.PutLogEvents":                              "logs.amazonaws.com",
		"CloudTrail_20131101.LookupEvents":               "cloudtrail.amazonaws.com",
		"AmazonAthena.StartQueryExecution":               "athena.amazonaws.com",
		"AWSEvents.PutEvents":                            "events.amazonaws.com",
		"AmazonSNS.Publish":                              "sns.amazonaws.com",
		"AmazonSQS.SendMessage":                          "sqs.amazonaws.com",
		"AWSOrganizationsV20161128.ListAccounts":         "organizations.amazonaws.com",
		"AmazonEC2.DescribeInstances":                    "ec2.amazonaws.com",
		"AWSEC2.RunInstances":                            "ec2.amazonaws.com",
		"AWSCognitoIdentityProviderService.InitiateAuth": "cognito-idp.amazonaws.com",
		"APIGateway.CreateApi":                           "apigateway.amazonaws.com",
		"AWSAppSync.CreateGraphqlApi":                    "appsync.amazonaws.com",
		"ElasticMapReduce.RunJobFlow":                    "elasticmapreduce.amazonaws.com",
		"AWSGlue.GetTable":                               "glue.amazonaws.com",
		"AWSStepFunctions.StartExecution":                "states.amazonaws.com",
		"Firehose_20150804.PutRecord":                    "firehose.amazonaws.com",
		"Kinesis_20131202.PutRecord":                     "kinesis.amazonaws.com",
		"AmazonEC2ContainerServiceV20141113.RunTask":     "ecs.amazonaws.com",
		"AmazonEC2ContainerRegistry_V20150921.PutImage":  "ecr.amazonaws.com",
		"AmazonRDS.DescribeDBInstances":                  "rds.amazonaws.com",
		"CloudFormation.CreateStack":                     "cloudformation.amazonaws.com",
		"CloudApiService.CreateResource":                 "cloudcontrolapi.amazonaws.com",
		"AWSWAF_20150824.CreateWebACL":                   "wafv2.amazonaws.com",
		"ConfigService.PutConfigRule":                    "config.amazonaws.com",
		"AWSIAM20100508.CreateUser":                      "iam.amazonaws.com",
		"AWSSTS_20110615.AssumeRole":                     "sts.amazonaws.com",
		"AmazonS3.ListBuckets":                           "s3.amazonaws.com",
		"no-dot-target":                                  "",
		".EmptyPrefix":                                   "",
	}
	for target, want := range cases {
		if got := auditEventSourceFromJSONTarget(target); got != want {
			t.Fatalf("%q got %q want %q", target, got, want)
		}
	}
}

func TestAuditEventSourceForRequest(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest("POST", "http://127.0.0.1:4566/", strings.NewReader(`{}`))
	req.Header.Set("X-Amz-Target", "AmazonSQS.ListQueues")
	if src := auditEventSourceForRequest(req, ""); src != "sqs.amazonaws.com" {
		t.Fatalf("sqs got %q", src)
	}
	req2 := httptest.NewRequest("GET", "http://s3.local/bucket/key", nil)
	if src := auditEventSourceForRequest(req2, ""); src != "s3.amazonaws.com" {
		t.Fatalf("path s3 got %q", src)
	}
	if src := auditEventSourceForRequest(nil, "custom.amazonaws.com"); src != "custom.amazonaws.com" {
		t.Fatalf("fallback got %q", src)
	}
}

func TestWantsJSONAndLooksLikeJSON(t *testing.T) {
	t.Parallel()
	if !looksLikeJSON([]byte(` {"a":1}`)) || looksLikeJSON([]byte("plain")) {
		t.Fatal("looksLikeJSON")
	}
	req := httptest.NewRequest("POST", "/", nil)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	if !wantsJSON(req, nil) {
		t.Fatal("content-type json")
	}
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("X-Amz-Target", "AWSOrganizations.ListAccounts")
	if !wantsJSON(req, []byte("not json")) {
		t.Fatal("organizations target")
	}
}

func TestRequestParamsAndFormParams(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest("POST", "/?Action=ListTables&Version=2012-08-10", nil)
	body := []byte("TableName=items")
	m := requestParams(req, body)
	if m["Action"] != "ListTables" || m["TableName"] != "items" {
		t.Fatalf("%#v", m)
	}
	vals := formParams(req, body)
	if vals.Get("TableName") != "items" {
		t.Fatal("form")
	}
}

func TestServiceResourceTagKeyPrefix(t *testing.T) {
	t.Parallel()
	if serviceResourceTagKeyPrefix("arn:aws:ecr:us-east-1:1:repository/x") != "ecr:ResourceTag/" {
		t.Fatal("ecr arn")
	}
	if serviceResourceTagKeyPrefix("not-an-arn") != "" {
		t.Fatal("invalid")
	}
}
