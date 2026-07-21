package authz_test

import (
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
)

const (
	testEventsAccount = "000000000001"
	testQueueARN      = "arn:aws:sqs:us-east-1:000000000001:jobs"
	testRuleARN       = "arn:aws:events:us-east-1:000000000001:rule/default/lab-rule"
)

func eventsQueuePolicy(principal string) string {
	return `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"` + principal + `"},"Action":"sqs:SendMessage","Resource":"` + testQueueARN + `"}]}`
}

func eventsQueuePolicyRoot(accountID string) string {
	return `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::` + accountID + `:root"},"Action":"sqs:SendMessage","Resource":"` + testQueueARN + `"}]}`
}

func eventsQueuePolicySourceArn(principal, sourceARN string) string {
	return `{"Version":"2012-10-17","Statement":[{
		"Effect":"Allow",
		"Principal":{"Service":"` + principal + `"},
		"Action":"sqs:SendMessage",
		"Resource":"` + testQueueARN + `",
		"Condition":{"ArnLike":{"aws:SourceArn":"` + sourceARN + `"}}
	}]}`
}

func TestEventTargetResourcePolicyAllowsServicePrincipal(t *testing.T) {
	if !authz.EventTargetResourcePolicyAllows(
		eventsQueuePolicy(authz.ServicePrincipalEvents),
		"sqs:SendMessage",
		testQueueARN,
		authz.ServicePrincipalEvents,
		testEventsAccount,
		nil,
	) {
		t.Fatal("expected service principal allow")
	}
}

func TestEventTargetResourcePolicyAllowsAccountRoot(t *testing.T) {
	if !authz.EventTargetResourcePolicyAllows(
		eventsQueuePolicyRoot(testEventsAccount),
		"sqs:SendMessage",
		testQueueARN,
		authz.ServicePrincipalEvents,
		testEventsAccount,
		nil,
	) {
		t.Fatal("expected account root allow")
	}
}

func TestEventTargetResourcePolicyDenyWithoutPolicy(t *testing.T) {
	if authz.EventTargetResourcePolicyAllows(
		"",
		"sqs:SendMessage",
		testQueueARN,
		authz.ServicePrincipalEvents,
		testEventsAccount,
		nil,
	) {
		t.Fatal("expected deny without policy")
	}
}

func TestEventTargetResourcePolicyDenyWrongAction(t *testing.T) {
	if authz.EventTargetResourcePolicyAllows(
		eventsQueuePolicy(authz.ServicePrincipalEvents),
		"sqs:ReceiveMessage",
		testQueueARN,
		authz.ServicePrincipalEvents,
		testEventsAccount,
		nil,
	) {
		t.Fatal("expected deny for wrong action")
	}
}

func TestEventTargetResourcePolicyExplicitDenyBlocks(t *testing.T) {
	policy := `{"Version":"2012-10-17","Statement":[
		{"Effect":"Allow","Principal":{"Service":"events.amazonaws.com"},"Action":"sqs:SendMessage","Resource":"` + testQueueARN + `"},
		{"Effect":"Deny","Principal":{"Service":"events.amazonaws.com"},"Action":"sqs:SendMessage","Resource":"` + testQueueARN + `"}
	]}`
	if authz.EventTargetResourcePolicyAllows(
		policy,
		"sqs:SendMessage",
		testQueueARN,
		authz.ServicePrincipalEvents,
		testEventsAccount,
		nil,
	) {
		t.Fatal("expected explicit deny to block service path")
	}
}

func TestEventTargetResourcePolicySourceArnCondition(t *testing.T) {
	policy := eventsQueuePolicySourceArn(authz.ServicePrincipalEvents, testRuleARN)
	keysOK := authz.DeliverySourceConditionKeys(testRuleARN, testEventsAccount)
	if !authz.EventTargetResourcePolicyAllows(
		policy, "sqs:SendMessage", testQueueARN, authz.ServicePrincipalEvents, testEventsAccount, keysOK,
	) {
		t.Fatal("expected allow when SourceArn matches")
	}
	wrong := authz.DeliverySourceConditionKeys(
		"arn:aws:events:us-east-1:000000000001:rule/default/other",
		testEventsAccount,
	)
	if authz.EventTargetResourcePolicyAllows(
		policy, "sqs:SendMessage", testQueueARN, authz.ServicePrincipalEvents, testEventsAccount, wrong,
	) {
		t.Fatal("expected deny when SourceArn mismatches")
	}
	if authz.EventTargetResourcePolicyAllows(
		policy, "sqs:SendMessage", testQueueARN, authz.ServicePrincipalEvents, testEventsAccount, nil,
	) {
		t.Fatal("expected deny when SourceArn unpopulated")
	}
}

func TestDeliverySourceConditionKeysParsesAccount(t *testing.T) {
	keys := authz.DeliverySourceConditionKeys(testRuleARN, "")
	if keys["aws:SourceArn"] != testRuleARN {
		t.Fatalf("SourceArn=%q", keys["aws:SourceArn"])
	}
	if keys["aws:SourceAccount"] != testEventsAccount {
		t.Fatalf("SourceAccount=%q", keys["aws:SourceAccount"])
	}
}
