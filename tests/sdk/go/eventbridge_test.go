package sdk_test

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge/types"
)

func TestEventBridgeBusAndRuleRoundTrip(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	client := newEvents(t, cfg)
	ctx := context.Background()
	prefix := uniquePrefix(t)

	busName := prefix + "-bus"
	_, err := client.CreateEventBus(ctx, &eventbridge.CreateEventBusInput{Name: aws.String(busName)})
	if err != nil {
		t.Fatalf("CreateEventBus: %v", err)
	}
	t.Cleanup(func() {
		_, _ = client.DeleteEventBus(ctx, &eventbridge.DeleteEventBusInput{Name: aws.String(busName)})
	})

	ruleName := prefix + "-rule"
	_, err = client.PutRule(ctx, &eventbridge.PutRuleInput{
		Name:         aws.String(ruleName),
		EventBusName: aws.String(busName),
		EventPattern: aws.String(`{"source":["noctaxris.sdk"]}`),
		State:        types.RuleStateEnabled,
	})
	if err != nil {
		t.Fatalf("PutRule: %v", err)
	}
	t.Cleanup(func() {
		_, _ = client.DeleteRule(ctx, &eventbridge.DeleteRuleInput{
			Name:         aws.String(ruleName),
			EventBusName: aws.String(busName),
		})
	})

	desc, err := client.DescribeRule(ctx, &eventbridge.DescribeRuleInput{
		Name:         aws.String(ruleName),
		EventBusName: aws.String(busName),
	})
	if err != nil {
		t.Fatalf("DescribeRule: %v", err)
	}
	if desc.Name == nil || *desc.Name != ruleName {
		t.Fatalf("DescribeRule unexpected: %+v", desc)
	}

	_, err = client.DeleteRule(ctx, &eventbridge.DeleteRuleInput{
		Name:         aws.String(ruleName),
		EventBusName: aws.String(busName),
	})
	if err != nil {
		t.Fatalf("DeleteRule: %v", err)
	}
	_, err = client.DeleteEventBus(ctx, &eventbridge.DeleteEventBusInput{Name: aws.String(busName)})
	if err != nil {
		t.Fatalf("DeleteEventBus: %v", err)
	}
}
