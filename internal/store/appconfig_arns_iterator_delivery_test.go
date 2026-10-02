package store_test

import (
	"errors"
	"sync/atomic"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestAppConfigARNBuilders(t *testing.T) {
	app := store.AppConfigApplicationARN("", "000000000001", "app-1")
	if app != "arn:aws:appconfig:us-east-1:000000000001:application/app-1" {
		t.Fatalf("app ARN=%q", app)
	}
	env := store.AppConfigEnvironmentARN("eu-west-1", "000000000001", "app-1", "env-1")
	if env != "arn:aws:appconfig:eu-west-1:000000000001:application/app-1/environment/env-1" {
		t.Fatalf("env ARN=%q", env)
	}
	prof := store.AppConfigConfigurationProfileARN("eu-west-1", "000000000001", "app-1", "prof-1")
	if prof != "arn:aws:appconfig:eu-west-1:000000000001:application/app-1/configurationprofile/prof-1" {
		t.Fatalf("profile ARN=%q", prof)
	}
}

func TestRoute53HostedZoneARNAndMQTTBrokerAuthHook(t *testing.T) {
	if got := store.Route53HostedZoneARN(""); got != "*" {
		t.Fatalf("empty zone=%q", got)
	}
	if got := store.Route53HostedZoneARN("/hostedzone/Z123"); got != "arn:aws:route53:::hostedzone/Z123" {
		t.Fatalf("zone ARN=%q", got)
	}
	if got := store.Route53HostedZoneARN("Z456"); got != "arn:aws:route53:::hostedzone/Z456" {
		t.Fatalf("bare zone ARN=%q", got)
	}

	st := openTestStore(t)
	var n atomic.Int32
	st.SetMQTTBrokerAuthHook(func() { n.Add(1) })
	st.SetMQTTBrokerAuthHook(nil)
	var nilStore *store.Store
	nilStore.SetMQTTBrokerAuthHook(func() {})
	_ = n
}

func TestLookupKinesisShardIteratorPositiveNegative(t *testing.T) {
	st := openTestStore(t)
	const account = "000000000001"
	const region = "us-east-1"

	if _, _, _, err := st.LookupKinesisShardIterator(""); err == nil {
		t.Fatal("empty iterator should fail")
	}
	if _, _, _, err := st.LookupKinesisShardIterator("missing-it"); !errors.Is(err, store.ErrKinesisExpiredIterator) {
		t.Fatalf("missing iterator err=%v", err)
	}

	if _, err := st.CreateKinesisStream(account, region, "lookup-stream", 1); err != nil {
		t.Fatal(err)
	}
	it, err := st.GetKinesisShardIterator(account, "lookup-stream", store.LabKinesisShardID(0), "TRIM_HORIZON", "")
	if err != nil {
		t.Fatal(err)
	}
	acct, name, arn, err := st.LookupKinesisShardIterator(it)
	if err != nil {
		t.Fatal(err)
	}
	if acct != account || name != "lookup-stream" || arn == "" {
		t.Fatalf("lookup acct=%q name=%q arn=%q", acct, name, arn)
	}
}

func TestPeekDynamoStreamIteratorPositiveNegative(t *testing.T) {
	st := openTestStore(t)
	const account = "000000000001"

	if _, _, err := st.PeekDynamoStreamIterator(""); err == nil {
		t.Fatal("empty peek should fail")
	}
	if _, _, err := st.PeekDynamoStreamIterator("missing"); !errors.Is(err, store.ErrDynamoStreamExpiredIter) {
		t.Fatalf("missing peek err=%v", err)
	}

	table, err := st.CreateTable(account, "us-east-1", "PeekStream", "pk", store.KeyTypeString, "", "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpdateTableStreamSpec(account, table.TableName, true, store.StreamViewNewImage); err != nil {
		t.Fatal(err)
	}
	it, err := st.GetDynamoStreamShardIterator(account, table.TableName, store.LabDynamoStreamShardID, "TRIM_HORIZON", "")
	if err != nil {
		t.Fatal(err)
	}
	acct, name, err := st.PeekDynamoStreamIterator(it)
	if err != nil {
		t.Fatal(err)
	}
	if acct != account || name != table.TableName {
		t.Fatalf("peek acct=%q name=%q", acct, name)
	}
}

func TestDeliveryAuthorizedRoleAndResourceSameAccount(t *testing.T) {
	st := openTestStore(t)
	const account = "000000000001"
	const region = "us-east-1"

	if st.DeliveryAuthorizedRoleAndResource(account, "", "sqs:SendMessage", "arn:aws:sqs:"+region+":"+account+":q", "events.amazonaws.com", "", "sess", region) {
		t.Fatal("empty role without resource policy path should deny for missing queue")
	}

	q, err := st.CreateQueue(account, region, "127.0.0.1:4566", "deliver-authz-q", map[string]string{
		"Policy": `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"events.amazonaws.com"},"Action":"sqs:SendMessage","Resource":"*"}]}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !st.DeliveryAuthorizedRoleAndResource(account, "", "sqs:SendMessage", q.QueueARN, "events.amazonaws.com", "arn:aws:events:"+region+":"+account+":rule/r", "sess", region) {
		t.Fatal("resource-policy-only delivery should allow")
	}

	trust := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"events.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	roleARN, err := st.CreateRole(account, "deliver-authz-role", trust)
	if err != nil {
		t.Fatal(err)
	}
	allow := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"sqs:SendMessage","Resource":"*"}]}`
	if err := st.PutInlinePolicy(roleARN, "send", allow); err != nil {
		t.Fatal(err)
	}
	if !st.DeliveryAuthorizedRoleAndResource(account, roleARN, "sqs:SendMessage", q.QueueARN, "events.amazonaws.com", "arn:aws:events:"+region+":"+account+":rule/r", "sess", region) {
		t.Fatal("same-account role session delivery should allow")
	}
	if st.DeliveryAuthorizedRoleAndResource(account, "arn:aws:iam::"+account+":role/missing-role", "sqs:SendMessage", q.QueueARN, "events.amazonaws.com", "", "sess", region) {
		t.Fatal("missing role should deny")
	}
	if st.RoleSessionAllows(account, "arn:aws:iam::999999999999:role/x", "sqs:SendMessage", q.QueueARN, "sess", region) {
		t.Fatal("foreign role ARN should deny")
	}
}
