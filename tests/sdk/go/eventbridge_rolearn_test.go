package sdk_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
	ebtypes "github.com/aws/aws-sdk-go-v2/service/eventbridge/types"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

const ebRoleArnLessSource = "noctaxris.sdk.eb.rolearn"

func TestEventBridgeDeliverSQSWithoutRoleArn(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	ebClient := newEvents(t, cfg)
	sqsClient := newSQS(t, cfg)
	stsClient := newSTS(t, cfg)
	ctx := context.Background()
	prefix := uniquePrefix(t)

	caller, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		t.Fatalf("GetCallerIdentity: %v", err)
	}
	accountID := aws.ToString(caller.Account)
	if accountID == "" {
		t.Fatal("GetCallerIdentity missing Account")
	}

	busName := prefix + "-bus"
	ruleName := prefix + "-rule"
	qName := prefix + "-eb-q"

	qOut, err := sqsClient.CreateQueue(ctx, &sqs.CreateQueueInput{QueueName: aws.String(qName)})
	if err != nil {
		t.Fatalf("CreateQueue: %v", err)
	}
	qURL := aws.ToString(qOut.QueueUrl)
	t.Cleanup(func() {
		_, _ = sqsClient.DeleteQueue(ctx, &sqs.DeleteQueueInput{QueueUrl: aws.String(qURL)})
	})
	qARN := queueARN(t, ctx, sqsClient, qURL)

	_, err = ebClient.CreateEventBus(ctx, &eventbridge.CreateEventBusInput{Name: aws.String(busName)})
	if err != nil {
		t.Fatalf("CreateEventBus: %v", err)
	}
	t.Cleanup(func() {
		_, _ = ebClient.DeleteEventBus(ctx, &eventbridge.DeleteEventBusInput{Name: aws.String(busName)})
	})

	pattern := fmt.Sprintf(`{"source":[%q]}`, ebRoleArnLessSource)
	_, err = ebClient.PutRule(ctx, &eventbridge.PutRuleInput{
		Name:         aws.String(ruleName),
		EventBusName: aws.String(busName),
		EventPattern: aws.String(pattern),
		State:        ebtypes.RuleStateEnabled,
	})
	if err != nil {
		t.Fatalf("PutRule: %v", err)
	}
	t.Cleanup(func() {
		_, _ = ebClient.RemoveTargets(ctx, &eventbridge.RemoveTargetsInput{
			Rule:         aws.String(ruleName),
			EventBusName: aws.String(busName),
			Ids:          []string{"sqs"},
		})
		_, _ = ebClient.DeleteRule(ctx, &eventbridge.DeleteRuleInput{
			Name:         aws.String(ruleName),
			EventBusName: aws.String(busName),
		})
	})

	policy := fmt.Sprintf(
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"events.amazonaws.com"},"Action":"sqs:SendMessage","Resource":"%s","Condition":{"ArnLike":{"aws:SourceArn":"arn:aws:events:us-east-1:%s:rule/%s/%s"}}}]}`,
		qARN, accountID, busName, ruleName,
	)
	_, err = sqsClient.SetQueueAttributes(ctx, &sqs.SetQueueAttributesInput{
		QueueUrl:   aws.String(qURL),
		Attributes: map[string]string{"Policy": policy},
	})
	if err != nil {
		t.Fatalf("SetQueueAttributes Policy: %v", err)
	}

	putTargets, err := ebClient.PutTargets(ctx, &eventbridge.PutTargetsInput{
		Rule:         aws.String(ruleName),
		EventBusName: aws.String(busName),
		Targets: []ebtypes.Target{
			{Id: aws.String("sqs"), Arn: aws.String(qARN)},
		},
	})
	if err != nil {
		t.Fatalf("PutTargets: %v", err)
	}
	if putTargets.FailedEntryCount > 0 {
		t.Fatalf("PutTargets FailedEntryCount=%d entries=%+v", putTargets.FailedEntryCount, putTargets.FailedEntries)
	}

	drainQueue(t, ctx, sqsClient, qURL)
	marker := "eb-rolearn-" + prefix
	detail := fmt.Sprintf(`{"marker":%q}`, marker)
	putOut, err := ebClient.PutEvents(ctx, &eventbridge.PutEventsInput{
		Entries: []ebtypes.PutEventsRequestEntry{{
			EventBusName: aws.String(busName),
			Source:       aws.String(ebRoleArnLessSource),
			DetailType:   aws.String("RoleArnLessDelivery"),
			Detail:       aws.String(detail),
		}},
	})
	if err != nil {
		t.Fatalf("PutEvents: %v", err)
	}
	if putOut.FailedEntryCount > 0 {
		t.Fatalf("PutEvents FailedEntryCount=%d entries=%+v", putOut.FailedEntryCount, putOut.Entries)
	}

	body := receiveOneBody(t, ctx, sqsClient, qURL, 8*time.Second)
	if !strings.Contains(body, ebRoleArnLessSource) || !strings.Contains(body, marker) {
		t.Fatalf("SQS body missing delivery markers: %s", body)
	}
}

func TestEventBridgeSkipSQSWithoutRoleArnOrPolicy(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	ebClient := newEvents(t, cfg)
	sqsClient := newSQS(t, cfg)
	ctx := context.Background()
	prefix := uniquePrefix(t)

	busName := prefix + "-bus"
	ruleName := prefix + "-rule"
	qName := prefix + "-deny-q"

	qOut, err := sqsClient.CreateQueue(ctx, &sqs.CreateQueueInput{QueueName: aws.String(qName)})
	if err != nil {
		t.Fatalf("CreateQueue: %v", err)
	}
	qURL := aws.ToString(qOut.QueueUrl)
	t.Cleanup(func() {
		_, _ = sqsClient.DeleteQueue(ctx, &sqs.DeleteQueueInput{QueueUrl: aws.String(qURL)})
	})
	qARN := queueARN(t, ctx, sqsClient, qURL)

	_, err = ebClient.CreateEventBus(ctx, &eventbridge.CreateEventBusInput{Name: aws.String(busName)})
	if err != nil {
		t.Fatalf("CreateEventBus: %v", err)
	}
	t.Cleanup(func() {
		_, _ = ebClient.DeleteEventBus(ctx, &eventbridge.DeleteEventBusInput{Name: aws.String(busName)})
	})

	pattern := fmt.Sprintf(`{"source":[%q]}`, ebRoleArnLessSource)
	_, err = ebClient.PutRule(ctx, &eventbridge.PutRuleInput{
		Name:         aws.String(ruleName),
		EventBusName: aws.String(busName),
		EventPattern: aws.String(pattern),
		State:        ebtypes.RuleStateEnabled,
	})
	if err != nil {
		t.Fatalf("PutRule: %v", err)
	}
	t.Cleanup(func() {
		_, _ = ebClient.RemoveTargets(ctx, &eventbridge.RemoveTargetsInput{
			Rule:         aws.String(ruleName),
			EventBusName: aws.String(busName),
			Ids:          []string{"deny"},
		})
		_, _ = ebClient.DeleteRule(ctx, &eventbridge.DeleteRuleInput{
			Name:         aws.String(ruleName),
			EventBusName: aws.String(busName),
		})
	})

	putTargets, err := ebClient.PutTargets(ctx, &eventbridge.PutTargetsInput{
		Rule:         aws.String(ruleName),
		EventBusName: aws.String(busName),
		Targets: []ebtypes.Target{
			{Id: aws.String("deny"), Arn: aws.String(qARN)},
		},
	})
	if err != nil {
		t.Fatalf("PutTargets: %v", err)
	}
	if putTargets.FailedEntryCount > 0 {
		t.Fatalf("PutTargets FailedEntryCount=%d entries=%+v", putTargets.FailedEntryCount, putTargets.FailedEntries)
	}

	drainQueue(t, ctx, sqsClient, qURL)
	_, err = ebClient.PutEvents(ctx, &eventbridge.PutEventsInput{
		Entries: []ebtypes.PutEventsRequestEntry{{
			EventBusName: aws.String(busName),
			Source:       aws.String(ebRoleArnLessSource),
			DetailType:   aws.String("RoleArnLessSkip"),
			Detail:       aws.String(`{"probe":"no-policy"}`),
		}},
	})
	if err != nil {
		t.Fatalf("PutEvents: %v", err)
	}
	if msg := receiveOptionalBody(t, ctx, sqsClient, qURL, 2*time.Second); msg != "" {
		t.Fatalf("queue without events.amazonaws.com policy unexpectedly received: %s", msg)
	}
}
