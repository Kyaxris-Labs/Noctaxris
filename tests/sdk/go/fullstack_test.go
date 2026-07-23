package sdk_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
	ebtypes "github.com/aws/aws-sdk-go-v2/service/eventbridge/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	lamtypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

const labEventSource = "noctaxris.lab.orders"

// labOrderPipeline holds shared ARNs/URLs for the advanced fullstack scenario.
type labOrderPipeline struct {
	prefix string

	accountID string
	keyID     string
	keyARN    string

	lambdaRoleName string
	lambdaRoleARN  string

	bucket     string
	objectKey  string
	orderBody  []byte
	orderID    string
	tableName  string

	ebQueueURL string
	ebQueueARN string
	denyQueueURL string
	denyQueueARN string
	snsQueueURL  string
	snsQueueARN  string

	topicARN         string
	subscriptionARN  string
	functionName     string
	busName          string
	ruleName         string
	stringParam      string
	secureParam      string
	secureValue      string
	secretName       string
	secretString     string
	provisioned      bool
	eventbridgeWired bool
}

// TestFullstackLabOrderPipeline exercises a multi-service lab app end-to-end.
//
// Scenario: IAM roles + KMS + S3 + DynamoDB + SQS + SNS + Lambda CRUD + EventBridge
// rule → queue/topic + SSM + Secrets Manager.
//
// Enable with NOCTAXRIS_ADVANCED=1.
func TestFullstackLabOrderPipeline(t *testing.T) {
	if os.Getenv("NOCTAXRIS_ADVANCED") != "1" {
		t.Skip("advanced fullstack suite — set NOCTAXRIS_ADVANCED=1")
	}
	requireReady(t)
	cfg := loadAWSConfig(t)
	ctx := context.Background()
	prefix := uniquePrefix(t)

	stsClient := newSTS(t, cfg)
	caller, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		t.Fatalf("GetCallerIdentity: %v", err)
	}
	if caller.Account == nil || *caller.Account == "" {
		t.Fatal("GetCallerIdentity missing Account")
	}

	lab := &labOrderPipeline{
		prefix:       prefix,
		accountID:    *caller.Account,
		objectKey:    "orders/sample.json",
		orderID:      "ord-" + prefix,
		orderBody:    []byte(`{"orderId":"ord-` + prefix + `","item":"widget","qty":2}`),
		stringParam:  "/lab/" + prefix + "/config",
		secureParam:  "/lab/" + prefix + "/secure-config",
		secureValue:  "secure-" + prefix,
		secretName:   "lab/" + prefix + "/api-key",
		secretString: `{"apiKey":"key-` + prefix + `"}`,
		bucket:       strings.ToLower(prefix + "-orders"),
		tableName:    prefix + "-orders",
		busName:      prefix + "-bus",
		ruleName:     prefix + "-orders-rule",
		functionName: prefix + "-fn",
		lambdaRoleName: prefix + "-lambda",
	}

	kmsClient := newKMS(t, cfg)
	iamClient := newIAM(t, cfg)
	s3Client := newS3(t, cfg)
	ddbClient := newDDB(t, cfg)
	sqsClient := newSQS(t, cfg)
	snsClient := newSNS(t, cfg)
	ssmClient := newSSM(t, cfg)
	secretsClient := newSecrets(t, cfg)
	lamClient := newLambda(t, cfg)
	ebClient := newEvents(t, cfg)

	// Cleanups must register on the parent test so resources survive across subtests.
	t.Run("provision", func(st *testing.T) {
		provisionLabOrderPipeline(st, t, ctx, lab, kmsClient, iamClient, s3Client, ddbClient, sqsClient, snsClient, ssmClient, secretsClient, lamClient)
	})
	t.Run("wire_eventbridge", func(st *testing.T) {
		requireLabProvisioned(st, lab)
		wireEventBridge(st, t, ctx, lab, ebClient)
	})
	t.Run("put_events_to_sqs", func(st *testing.T) {
		requireEventBridgeWired(st, lab)
		putEventsToSQS(st, ctx, lab, ebClient, sqsClient)
	})
	t.Run("sns_fanout", func(st *testing.T) {
		requireEventBridgeWired(st, lab)
		snsFanout(st, ctx, lab, snsClient, sqsClient)
	})
	t.Run("data_plane_reads", func(st *testing.T) {
		requireLabProvisioned(st, lab)
		dataPlaneReads(st, ctx, lab, s3Client, ddbClient, ssmClient, secretsClient, lamClient, iamClient)
	})
	t.Run("authz_denies", func(st *testing.T) {
		requireEventBridgeWired(st, lab)
		authzDenies(st, t, ctx, lab, ebClient, sqsClient, kmsClient, ssmClient)
	})
}

func requireLabProvisioned(t *testing.T, lab *labOrderPipeline) {
	t.Helper()
	if !lab.provisioned {
		t.Skip("provision incomplete")
	}
}

func requireEventBridgeWired(t *testing.T, lab *labOrderPipeline) {
	t.Helper()
	requireLabProvisioned(t, lab)
	if !lab.eventbridgeWired {
		t.Skip("eventbridge wire incomplete")
	}
}

func provisionLabOrderPipeline(
	t *testing.T,
	parent *testing.T,
	ctx context.Context,
	lab *labOrderPipeline,
	kmsClient *kms.Client,
	iamClient *iam.Client,
	s3Client *s3.Client,
	ddbClient *dynamodb.Client,
	sqsClient *sqs.Client,
	snsClient *sns.Client,
	ssmClient *ssm.Client,
	secretsClient *secretsmanager.Client,
	lamClient *lambda.Client,
) {
	t.Helper()

	createdKey, err := kmsClient.CreateKey(ctx, &kms.CreateKeyInput{
		Description: aws.String(lab.prefix + "-lab-cmk"),
	})
	if err != nil {
		t.Fatalf("CreateKey: %v", err)
	}
	if createdKey.KeyMetadata == nil || createdKey.KeyMetadata.KeyId == nil {
		t.Fatal("CreateKey missing KeyId")
	}
	lab.keyID = *createdKey.KeyMetadata.KeyId
	if createdKey.KeyMetadata.Arn != nil {
		lab.keyARN = *createdKey.KeyMetadata.Arn
	} else {
		lab.keyARN = lab.keyID
	}
	parent.Cleanup(func() {
		_, _ = kmsClient.ScheduleKeyDeletion(ctx, &kms.ScheduleKeyDeletionInput{
			KeyId:               aws.String(lab.keyID),
			PendingWindowInDays: aws.Int32(7),
		})
	})

	trust := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	roleOut, err := iamClient.CreateRole(ctx, &iam.CreateRoleInput{
		RoleName:                 aws.String(lab.lambdaRoleName),
		AssumeRolePolicyDocument: aws.String(trust),
	})
	if err != nil {
		t.Fatalf("CreateRole: %v", err)
	}
	if roleOut.Role == nil || roleOut.Role.Arn == nil {
		t.Fatal("CreateRole missing Arn")
	}
	lab.lambdaRoleARN = *roleOut.Role.Arn
	parent.Cleanup(func() {
		_, _ = iamClient.DeleteRolePolicy(ctx, &iam.DeleteRolePolicyInput{
			RoleName:   aws.String(lab.lambdaRoleName),
			PolicyName: aws.String("lab-inline"),
		})
		_, _ = iamClient.DeleteRole(ctx, &iam.DeleteRoleInput{RoleName: aws.String(lab.lambdaRoleName)})
	})

	keyPolicy := fmt.Sprintf(`{
		"Version":"2012-10-17",
		"Statement":[
			{"Sid":"Root","Effect":"Allow","Principal":{"AWS":"arn:aws:iam::%s:root"},"Action":"kms:*","Resource":"*"},
			{"Sid":"Lambda","Effect":"Allow","Principal":{"AWS":"%s"},"Action":["kms:Encrypt","kms:Decrypt","kms:GenerateDataKey","kms:DescribeKey"],"Resource":"*"}
		]
	}`, lab.accountID, lab.lambdaRoleARN)
	_, err = kmsClient.PutKeyPolicy(ctx, &kms.PutKeyPolicyInput{
		KeyId:      aws.String(lab.keyID),
		PolicyName: aws.String("default"),
		Policy:     aws.String(keyPolicy),
	})
	if err != nil {
		t.Fatalf("PutKeyPolicy: %v", err)
	}

	inline := fmt.Sprintf(`{
		"Version":"2012-10-17",
		"Statement":[
			{"Effect":"Allow","Action":["s3:GetObject","s3:PutObject"],"Resource":"arn:aws:s3:::%s/*"},
			{"Effect":"Allow","Action":["dynamodb:GetItem","dynamodb:PutItem"],"Resource":"arn:aws:dynamodb:*:%s:table/%s"},
			{"Effect":"Allow","Action":["ssm:GetParameter","ssm:GetParameters"],"Resource":"arn:aws:ssm:*:%s:parameter/lab/%s/*"},
			{"Effect":"Allow","Action":["secretsmanager:GetSecretValue"],"Resource":"arn:aws:secretsmanager:*:%s:secret:lab/%s/*"},
			{"Effect":"Allow","Action":["kms:Decrypt","kms:Encrypt","kms:GenerateDataKey"],"Resource":"%s"},
			{"Effect":"Allow","Action":["logs:CreateLogGroup","logs:CreateLogStream","logs:PutLogEvents"],"Resource":"*"}
		]
	}`, lab.bucket, lab.accountID, lab.tableName, lab.accountID, lab.prefix, lab.accountID, lab.prefix, lab.keyARN)
	_, err = iamClient.PutRolePolicy(ctx, &iam.PutRolePolicyInput{
		RoleName:       aws.String(lab.lambdaRoleName),
		PolicyName:     aws.String("lab-inline"),
		PolicyDocument: aws.String(inline),
	})
	if err != nil {
		t.Fatalf("PutRolePolicy: %v", err)
	}

	_, err = s3Client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(lab.bucket)})
	if err != nil {
		t.Fatalf("CreateBucket: %v", err)
	}
	parent.Cleanup(func() {
		_, _ = s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(lab.bucket), Key: aws.String(lab.objectKey)})
		_, _ = s3Client.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: aws.String(lab.bucket)})
	})

	_, err = s3Client.PutBucketEncryption(ctx, &s3.PutBucketEncryptionInput{
		Bucket: aws.String(lab.bucket),
		ServerSideEncryptionConfiguration: &s3types.ServerSideEncryptionConfiguration{
			Rules: []s3types.ServerSideEncryptionRule{{
				ApplyServerSideEncryptionByDefault: &s3types.ServerSideEncryptionByDefault{
					SSEAlgorithm:   s3types.ServerSideEncryptionAwsKms,
					KMSMasterKeyID: aws.String(lab.keyID),
				},
			}},
		},
	})
	if err != nil {
		t.Fatalf("PutBucketEncryption: %v", err)
	}

	_, err = s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:               aws.String(lab.bucket),
		Key:                  aws.String(lab.objectKey),
		Body:                 bytes.NewReader(lab.orderBody),
		ServerSideEncryption: s3types.ServerSideEncryptionAwsKms,
		SSEKMSKeyId:          aws.String(lab.keyID),
	})
	if err != nil {
		t.Fatalf("PutObject: %v", err)
	}

	_, err = ddbClient.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(lab.tableName),
		AttributeDefinitions: []ddbtypes.AttributeDefinition{
			{AttributeName: aws.String("orderId"), AttributeType: ddbtypes.ScalarAttributeTypeS},
		},
		KeySchema: []ddbtypes.KeySchemaElement{
			{AttributeName: aws.String("orderId"), KeyType: ddbtypes.KeyTypeHash},
		},
		BillingMode: ddbtypes.BillingModePayPerRequest,
		SSESpecification: &ddbtypes.SSESpecification{
			Enabled:        aws.Bool(true),
			SSEType:        ddbtypes.SSETypeKms,
			KMSMasterKeyId: aws.String(lab.keyID),
		},
	})
	if err != nil {
		t.Fatalf("CreateTable: %v", err)
	}
	parent.Cleanup(func() {
		_, _ = ddbClient.DeleteTable(ctx, &dynamodb.DeleteTableInput{TableName: aws.String(lab.tableName)})
	})

	_, err = ddbClient.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(lab.tableName),
		Item: map[string]ddbtypes.AttributeValue{
			"orderId": &ddbtypes.AttributeValueMemberS{Value: lab.orderID},
			"status":  &ddbtypes.AttributeValueMemberS{Value: "NEW"},
			"item":    &ddbtypes.AttributeValueMemberS{Value: "widget"},
		},
	})
	if err != nil {
		t.Fatalf("PutItem: %v", err)
	}

	ebQ, err := sqsClient.CreateQueue(ctx, &sqs.CreateQueueInput{QueueName: aws.String(lab.prefix + "-eb-q")})
	if err != nil {
		t.Fatalf("CreateQueue eb: %v", err)
	}
	lab.ebQueueURL = aws.ToString(ebQ.QueueUrl)
	lab.ebQueueARN = queueARN(t, ctx, sqsClient, lab.ebQueueURL)
	parent.Cleanup(func() {
		_, _ = sqsClient.DeleteQueue(ctx, &sqs.DeleteQueueInput{QueueUrl: aws.String(lab.ebQueueURL)})
	})

	denyQ, err := sqsClient.CreateQueue(ctx, &sqs.CreateQueueInput{QueueName: aws.String(lab.prefix + "-deny-q")})
	if err != nil {
		t.Fatalf("CreateQueue deny: %v", err)
	}
	lab.denyQueueURL = aws.ToString(denyQ.QueueUrl)
	lab.denyQueueARN = queueARN(t, ctx, sqsClient, lab.denyQueueURL)
	parent.Cleanup(func() {
		_, _ = sqsClient.DeleteQueue(ctx, &sqs.DeleteQueueInput{QueueUrl: aws.String(lab.denyQueueURL)})
	})

	snsQ, err := sqsClient.CreateQueue(ctx, &sqs.CreateQueueInput{QueueName: aws.String(lab.prefix + "-sns-q")})
	if err != nil {
		t.Fatalf("CreateQueue sns: %v", err)
	}
	lab.snsQueueURL = aws.ToString(snsQ.QueueUrl)
	lab.snsQueueARN = queueARN(t, ctx, sqsClient, lab.snsQueueURL)
	parent.Cleanup(func() {
		_, _ = sqsClient.DeleteQueue(ctx, &sqs.DeleteQueueInput{QueueUrl: aws.String(lab.snsQueueURL)})
	})

	// ArnLike (not ArnEquals) for SourceArn locks; region must match delivery context.
	ebPolicy := fmt.Sprintf(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"events.amazonaws.com"},"Action":"sqs:SendMessage","Resource":"%s","Condition":{"ArnLike":{"aws:SourceArn":"arn:aws:events:us-east-1:%s:rule/%s/%s"}}}]}`,
		lab.ebQueueARN, lab.accountID, lab.busName, lab.ruleName)
	_, err = sqsClient.SetQueueAttributes(ctx, &sqs.SetQueueAttributesInput{
		QueueUrl: aws.String(lab.ebQueueURL),
		Attributes: map[string]string{
			"Policy": ebPolicy,
		},
	})
	if err != nil {
		t.Fatalf("SetQueueAttributes eb policy: %v", err)
	}

	snsQPolicy := fmt.Sprintf(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"sns.amazonaws.com"},"Action":"sqs:SendMessage","Resource":"%s"}]}`, lab.snsQueueARN)
	_, err = sqsClient.SetQueueAttributes(ctx, &sqs.SetQueueAttributesInput{
		QueueUrl: aws.String(lab.snsQueueURL),
		Attributes: map[string]string{
			"Policy": snsQPolicy,
		},
	})
	if err != nil {
		t.Fatalf("SetQueueAttributes sns policy: %v", err)
	}

	topicOut, err := snsClient.CreateTopic(ctx, &sns.CreateTopicInput{Name: aws.String(lab.prefix + "-orders")})
	if err != nil {
		t.Fatalf("CreateTopic: %v", err)
	}
	lab.topicARN = aws.ToString(topicOut.TopicArn)
	parent.Cleanup(func() {
		_, _ = snsClient.DeleteTopic(ctx, &sns.DeleteTopicInput{TopicArn: aws.String(lab.topicARN)})
	})

	topicPolicy := fmt.Sprintf(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"events.amazonaws.com"},"Action":"sns:Publish","Resource":"%s"}]}`, lab.topicARN)
	_, err = snsClient.SetTopicAttributes(ctx, &sns.SetTopicAttributesInput{
		TopicArn:       aws.String(lab.topicARN),
		AttributeName:  aws.String("Policy"),
		AttributeValue: aws.String(topicPolicy),
	})
	if err != nil {
		t.Fatalf("SetTopicAttributes Policy: %v", err)
	}

	subOut, err := snsClient.Subscribe(ctx, &sns.SubscribeInput{
		TopicArn: aws.String(lab.topicARN),
		Protocol: aws.String("sqs"),
		Endpoint: aws.String(lab.snsQueueARN),
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	lab.subscriptionARN = aws.ToString(subOut.SubscriptionArn)
	parent.Cleanup(func() {
		if lab.subscriptionARN != "" {
			_, _ = snsClient.Unsubscribe(ctx, &sns.UnsubscribeInput{SubscriptionArn: aws.String(lab.subscriptionARN)})
		}
	})

	_, err = ssmClient.PutParameter(ctx, &ssm.PutParameterInput{
		Name:  aws.String(lab.stringParam),
		Type:  ssmtypes.ParameterTypeString,
		Value: aws.String("config-" + lab.prefix),
	})
	if err != nil {
		t.Fatalf("PutParameter String: %v", err)
	}
	parent.Cleanup(func() {
		_, _ = ssmClient.DeleteParameter(ctx, &ssm.DeleteParameterInput{Name: aws.String(lab.stringParam)})
	})

	_, err = ssmClient.PutParameter(ctx, &ssm.PutParameterInput{
		Name:  aws.String(lab.secureParam),
		Type:  ssmtypes.ParameterTypeSecureString,
		Value: aws.String(lab.secureValue),
		KeyId: aws.String(lab.keyID),
	})
	if err != nil {
		t.Fatalf("PutParameter SecureString: %v", err)
	}
	parent.Cleanup(func() {
		_, _ = ssmClient.DeleteParameter(ctx, &ssm.DeleteParameterInput{Name: aws.String(lab.secureParam)})
	})

	secOut, err := secretsClient.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
		Name:         aws.String(lab.secretName),
		SecretString: aws.String(lab.secretString),
		KmsKeyId:     aws.String(lab.keyID),
	})
	if err != nil {
		t.Fatalf("CreateSecret: %v", err)
	}
	_ = secOut
	parent.Cleanup(func() {
		_, _ = secretsClient.DeleteSecret(ctx, &secretsmanager.DeleteSecretInput{
			SecretId:                   aws.String(lab.secretName),
			ForceDeleteWithoutRecovery: aws.Bool(true),
		})
	})

	_, err = lamClient.CreateFunction(ctx, &lambda.CreateFunctionInput{
		FunctionName: aws.String(lab.functionName),
		Runtime:      lamtypes.RuntimePython312,
		Role:         aws.String(lab.lambdaRoleARN),
		Handler:      aws.String("index.handler"),
		Code:         &lamtypes.FunctionCode{ZipFile: minimalPythonZip(t)},
	})
	if err != nil {
		t.Fatalf("CreateFunction: %v", err)
	}
	parent.Cleanup(func() {
		_, _ = lamClient.DeleteFunction(ctx, &lambda.DeleteFunctionInput{FunctionName: aws.String(lab.functionName)})
	})

	lab.provisioned = true
}

func wireEventBridge(
	t *testing.T,
	parent *testing.T,
	ctx context.Context,
	lab *labOrderPipeline,
	ebClient *eventbridge.Client,
) {
	t.Helper()

	_, err := ebClient.CreateEventBus(ctx, &eventbridge.CreateEventBusInput{Name: aws.String(lab.busName)})
	if err != nil {
		t.Fatalf("CreateEventBus: %v", err)
	}
	parent.Cleanup(func() {
		_, _ = ebClient.DeleteEventBus(ctx, &eventbridge.DeleteEventBusInput{Name: aws.String(lab.busName)})
	})

	pattern := fmt.Sprintf(`{"source":[%q]}`, labEventSource)
	_, err = ebClient.PutRule(ctx, &eventbridge.PutRuleInput{
		Name:         aws.String(lab.ruleName),
		EventBusName: aws.String(lab.busName),
		EventPattern: aws.String(pattern),
		State:        ebtypes.RuleStateEnabled,
	})
	if err != nil {
		t.Fatalf("PutRule: %v", err)
	}
	parent.Cleanup(func() {
		_, _ = ebClient.RemoveTargets(ctx, &eventbridge.RemoveTargetsInput{
			Rule:         aws.String(lab.ruleName),
			EventBusName: aws.String(lab.busName),
			Ids:          []string{"sqs", "sns", "deny"},
		})
		_, _ = ebClient.DeleteRule(ctx, &eventbridge.DeleteRuleInput{
			Name:         aws.String(lab.ruleName),
			EventBusName: aws.String(lab.busName),
		})
	})

	putTargets, err := ebClient.PutTargets(ctx, &eventbridge.PutTargetsInput{
		Rule:         aws.String(lab.ruleName),
		EventBusName: aws.String(lab.busName),
		Targets: []ebtypes.Target{
			{Id: aws.String("sqs"), Arn: aws.String(lab.ebQueueARN)},
			{Id: aws.String("sns"), Arn: aws.String(lab.topicARN)},
			{Id: aws.String("deny"), Arn: aws.String(lab.denyQueueARN)},
		},
	})
	if err != nil {
		t.Fatalf("PutTargets: %v", err)
	}
	if putTargets.FailedEntryCount > 0 {
		t.Fatalf("PutTargets FailedEntryCount=%d entries=%+v", putTargets.FailedEntryCount, putTargets.FailedEntries)
	}

	listed, err := ebClient.ListTargetsByRule(ctx, &eventbridge.ListTargetsByRuleInput{
		Rule:         aws.String(lab.ruleName),
		EventBusName: aws.String(lab.busName),
	})
	if err != nil {
		t.Fatalf("ListTargetsByRule: %v", err)
	}
	foundSQS, foundSNS := false, false
	for _, tgt := range listed.Targets {
		arn := aws.ToString(tgt.Arn)
		if arn == lab.ebQueueARN {
			foundSQS = true
		}
		if arn == lab.topicARN {
			foundSNS = true
		}
	}
	if !foundSQS || !foundSNS {
		t.Fatalf("ListTargetsByRule missing targets: %+v", listed.Targets)
	}

	lab.eventbridgeWired = true
}

func putEventsToSQS(t *testing.T, ctx context.Context, lab *labOrderPipeline, ebClient *eventbridge.Client, sqsClient *sqs.Client) {
	t.Helper()
	drainQueue(t, ctx, sqsClient, lab.ebQueueURL)

	detail := fmt.Sprintf(`{"orderId":%q,"marker":"eb-delivery"}`, lab.orderID)
	putOut, err := ebClient.PutEvents(ctx, &eventbridge.PutEventsInput{
		Entries: []ebtypes.PutEventsRequestEntry{{
			EventBusName: aws.String(lab.busName),
			Source:       aws.String(labEventSource),
			DetailType:   aws.String("OrderReceived"),
			Detail:       aws.String(detail),
		}},
	})
	if err != nil {
		t.Fatalf("PutEvents: %v", err)
	}
	if putOut.FailedEntryCount > 0 {
		t.Fatalf("PutEvents FailedEntryCount=%d entries=%+v", putOut.FailedEntryCount, putOut.Entries)
	}

	body := receiveOneBody(t, ctx, sqsClient, lab.ebQueueURL, 8*time.Second)
	if !strings.Contains(body, labEventSource) {
		t.Fatalf("SQS body missing source %q: %s", labEventSource, body)
	}
	if !strings.Contains(body, lab.orderID) && !strings.Contains(body, "eb-delivery") {
		t.Fatalf("SQS body missing order markers: %s", body)
	}
}

func snsFanout(t *testing.T, ctx context.Context, lab *labOrderPipeline, snsClient *sns.Client, sqsClient *sqs.Client) {
	t.Helper()
	drainQueue(t, ctx, sqsClient, lab.snsQueueURL)

	marker := "sns-fanout-" + lab.prefix
	pub, err := snsClient.Publish(ctx, &sns.PublishInput{
		TopicArn: aws.String(lab.topicARN),
		Message:  aws.String(marker),
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if pub.MessageId == nil || *pub.MessageId == "" {
		t.Fatal("Publish missing MessageId")
	}

	body := receiveOneBody(t, ctx, sqsClient, lab.snsQueueURL, 8*time.Second)
	if !strings.Contains(body, marker) {
		t.Fatalf("SNS→SQS body missing %q: %s", marker, body)
	}
}

func dataPlaneReads(
	t *testing.T,
	ctx context.Context,
	lab *labOrderPipeline,
	s3Client *s3.Client,
	ddbClient *dynamodb.Client,
	ssmClient *ssm.Client,
	secretsClient *secretsmanager.Client,
	lamClient *lambda.Client,
	iamClient *iam.Client,
) {
	t.Helper()

	getObj, err := s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(lab.bucket),
		Key:    aws.String(lab.objectKey),
	})
	if err != nil {
		t.Fatalf("GetObject: %v", err)
	}
	gotBody, err := io.ReadAll(getObj.Body)
	_ = getObj.Body.Close()
	if err != nil {
		t.Fatalf("read GetObject: %v", err)
	}
	if !bytes.Equal(gotBody, lab.orderBody) {
		t.Fatalf("GetObject body=%q want=%q", gotBody, lab.orderBody)
	}

	item, err := ddbClient.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(lab.tableName),
		Key: map[string]ddbtypes.AttributeValue{
			"orderId": &ddbtypes.AttributeValueMemberS{Value: lab.orderID},
		},
	})
	if err != nil {
		t.Fatalf("GetItem: %v", err)
	}
	status, ok := item.Item["status"].(*ddbtypes.AttributeValueMemberS)
	if !ok || status.Value != "NEW" {
		t.Fatalf("GetItem status=%v", item.Item["status"])
	}

	strParam, err := ssmClient.GetParameter(ctx, &ssm.GetParameterInput{
		Name: aws.String(lab.stringParam),
	})
	if err != nil {
		t.Fatalf("GetParameter String: %v", err)
	}
	if strParam.Parameter == nil || aws.ToString(strParam.Parameter.Value) != "config-"+lab.prefix {
		t.Fatalf("GetParameter String unexpected: %+v", strParam.Parameter)
	}

	secParam, err := ssmClient.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           aws.String(lab.secureParam),
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		t.Fatalf("GetParameter SecureString: %v", err)
	}
	if secParam.Parameter == nil || aws.ToString(secParam.Parameter.Value) != lab.secureValue {
		t.Fatalf("GetParameter SecureString unexpected: %+v", secParam.Parameter)
	}

	secret, err := secretsClient.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(lab.secretName),
	})
	if err != nil {
		t.Fatalf("GetSecretValue: %v", err)
	}
	if aws.ToString(secret.SecretString) != lab.secretString {
		t.Fatalf("GetSecretValue=%q want=%q", aws.ToString(secret.SecretString), lab.secretString)
	}

	fn, err := lamClient.GetFunction(ctx, &lambda.GetFunctionInput{FunctionName: aws.String(lab.functionName)})
	if err != nil {
		t.Fatalf("GetFunction: %v", err)
	}
	if fn.Configuration == nil || aws.ToString(fn.Configuration.Role) != lab.lambdaRoleARN {
		t.Fatalf("GetFunction Role=%v want=%s", fn.Configuration, lab.lambdaRoleARN)
	}

	role, err := iamClient.GetRole(ctx, &iam.GetRoleInput{RoleName: aws.String(lab.lambdaRoleName)})
	if err != nil {
		t.Fatalf("GetRole: %v", err)
	}
	if role.Role == nil || role.Role.AssumeRolePolicyDocument == nil {
		t.Fatal("GetRole missing AssumeRolePolicyDocument")
	}
	var trustDoc map[string]any
	if err := json.Unmarshal([]byte(aws.ToString(role.Role.AssumeRolePolicyDocument)), &trustDoc); err != nil {
		// IAM may URL-encode the document; accept non-empty parseable-or-raw.
		if !strings.Contains(aws.ToString(role.Role.AssumeRolePolicyDocument), "lambda.amazonaws.com") {
			t.Fatalf("AssumeRolePolicyDocument not parseable and missing trust: %q", aws.ToString(role.Role.AssumeRolePolicyDocument))
		}
	}
}

func authzDenies(
	t *testing.T,
	parent *testing.T,
	ctx context.Context,
	lab *labOrderPipeline,
	ebClient *eventbridge.Client,
	sqsClient *sqs.Client,
	kmsClient *kms.Client,
	ssmClient *ssm.Client,
) {
	t.Helper()

	// deny-q has no events.amazonaws.com queue policy → delivery skipped.
	drainQueue(t, ctx, sqsClient, lab.denyQueueURL)
	_, err := ebClient.PutEvents(ctx, &eventbridge.PutEventsInput{
		Entries: []ebtypes.PutEventsRequestEntry{{
			EventBusName: aws.String(lab.busName),
			Source:       aws.String(labEventSource),
			DetailType:   aws.String("OrderDeniedProbe"),
			Detail:       aws.String(`{"probe":"deny-q"}`),
		}},
	})
	if err != nil {
		t.Fatalf("PutEvents deny probe: %v", err)
	}
	if msg := receiveOptionalBody(t, ctx, sqsClient, lab.denyQueueURL, 2*time.Second); msg != "" {
		t.Fatalf("deny-q unexpectedly received message: %s", msg)
	}

	// Mismatched source must not match the rule.
	drainQueue(t, ctx, sqsClient, lab.ebQueueURL)
	_, err = ebClient.PutEvents(ctx, &eventbridge.PutEventsInput{
		Entries: []ebtypes.PutEventsRequestEntry{{
			EventBusName: aws.String(lab.busName),
			Source:       aws.String("noctaxris.lab.other"),
			DetailType:   aws.String("OrderReceived"),
			Detail:       aws.String(`{"probe":"mismatch"}`),
		}},
	})
	if err != nil {
		t.Fatalf("PutEvents mismatch: %v", err)
	}
	if msg := receiveOptionalBody(t, ctx, sqsClient, lab.ebQueueURL, 2*time.Second); msg != "" {
		t.Fatalf("eb-q received mismatched-source event: %s", msg)
	}

	// SecureString decrypt denied when CMK policy omits kms:Decrypt (parity with Node/Python).
	denyCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	denyKey, err := kmsClient.CreateKey(denyCtx, &kms.CreateKeyInput{
		Description: aws.String(lab.prefix + "-deny-cmk"),
	})
	if err != nil {
		t.Fatalf("CreateKey deny CMK: %v", err)
	}
	denyKeyID := aws.ToString(denyKey.KeyMetadata.KeyId)
	parent.Cleanup(func() {
		_, _ = kmsClient.ScheduleKeyDeletion(ctx, &kms.ScheduleKeyDeletionInput{
			KeyId:               aws.String(denyKeyID),
			PendingWindowInDays: aws.Int32(7),
		})
	})
	denyParam := "/lab/" + lab.prefix + "/kms-deny"
	_, err = ssmClient.PutParameter(denyCtx, &ssm.PutParameterInput{
		Name:  aws.String(denyParam),
		Type:  ssmtypes.ParameterTypeSecureString,
		Value: aws.String("locked"),
		KeyId: aws.String(denyKeyID),
	})
	if err != nil {
		t.Fatalf("PutParameter deny SecureString: %v", err)
	}
	parent.Cleanup(func() {
		_, _ = ssmClient.DeleteParameter(ctx, &ssm.DeleteParameterInput{Name: aws.String(denyParam)})
	})
	encryptOnly := fmt.Sprintf(`{
		"Version":"2012-10-17",
		"Statement":[{
			"Sid":"EncryptOnly",
			"Effect":"Allow",
			"Principal":{"AWS":"arn:aws:iam::%s:root"},
			"Action":["kms:Encrypt","kms:GenerateDataKey*","kms:DescribeKey"],
			"Resource":"*"
		}]
	}`, lab.accountID)
	_, err = kmsClient.PutKeyPolicy(denyCtx, &kms.PutKeyPolicyInput{
		KeyId:      aws.String(denyKeyID),
		PolicyName: aws.String("default"),
		Policy:     aws.String(encryptOnly),
	})
	if err != nil {
		t.Fatalf("PutKeyPolicy encrypt-only: %v", err)
	}
	_, err = ssmClient.GetParameter(denyCtx, &ssm.GetParameterInput{
		Name:           aws.String(denyParam),
		WithDecryption: aws.Bool(true),
	})
	if err == nil {
		t.Fatal("expected SecureString decrypt deny when CMK omits kms:Decrypt")
	}
	msg := err.Error()
	if !strings.Contains(msg, "AccessDenied") &&
		!strings.Contains(msg, "AccessDeniedException") &&
		!strings.Contains(strings.ToLower(msg), "not authorized") &&
		!strings.Contains(msg, "KMS") {
		t.Fatalf("unexpected deny error: %v", err)
	}
}

func queueARN(t *testing.T, ctx context.Context, client *sqs.Client, queueURL string) string {
	t.Helper()
	attrs, err := client.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl:       aws.String(queueURL),
		AttributeNames: []sqstypes.QueueAttributeName{sqstypes.QueueAttributeNameQueueArn},
	})
	if err != nil {
		t.Fatalf("GetQueueAttributes: %v", err)
	}
	arn := attrs.Attributes[string(sqstypes.QueueAttributeNameQueueArn)]
	if arn == "" {
		t.Fatal("GetQueueAttributes missing QueueArn")
	}
	return arn
}

func drainQueue(t *testing.T, ctx context.Context, client *sqs.Client, queueURL string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		recv, err := client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:            aws.String(queueURL),
			MaxNumberOfMessages: 10,
			WaitTimeSeconds:     1,
			VisibilityTimeout:   0,
		})
		if err != nil {
			t.Fatalf("drain ReceiveMessage: %v", err)
		}
		if len(recv.Messages) == 0 {
			return
		}
		for _, m := range recv.Messages {
			if m.ReceiptHandle == nil {
				continue
			}
			_, _ = client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
				QueueUrl:      aws.String(queueURL),
				ReceiptHandle: m.ReceiptHandle,
			})
		}
	}
}

func receiveOneBody(t *testing.T, ctx context.Context, client *sqs.Client, queueURL string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		recv, err := client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:            aws.String(queueURL),
			MaxNumberOfMessages: 1,
			WaitTimeSeconds:     1,
		})
		if err != nil {
			t.Fatalf("ReceiveMessage: %v", err)
		}
		if len(recv.Messages) == 0 {
			continue
		}
		body := aws.ToString(recv.Messages[0].Body)
		if recv.Messages[0].ReceiptHandle != nil {
			_, _ = client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
				QueueUrl:      aws.String(queueURL),
				ReceiptHandle: recv.Messages[0].ReceiptHandle,
			})
		}
		return body
	}
	t.Fatalf("ReceiveMessage timed out on %s", queueURL)
	return ""
}

func receiveOptionalBody(t *testing.T, ctx context.Context, client *sqs.Client, queueURL string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		recv, err := client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:            aws.String(queueURL),
			MaxNumberOfMessages: 1,
			WaitTimeSeconds:     1,
		})
		if err != nil {
			t.Fatalf("ReceiveMessage: %v", err)
		}
		if len(recv.Messages) == 0 {
			continue
		}
		body := aws.ToString(recv.Messages[0].Body)
		if recv.Messages[0].ReceiptHandle != nil {
			_, _ = client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
				QueueUrl:      aws.String(queueURL),
				ReceiptHandle: recv.Messages[0].ReceiptHandle,
			})
		}
		return body
	}
	return ""
}
