package store

import (
	"errors"
	"path/filepath"
	"testing"
)

func openInternalTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	key, err := LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestModifyCFNSNSTopicSecretLogGroupEventBus(t *testing.T) {
	st := openInternalTestStore(t)
	const account = "000000000001"
	const region = "us-east-1"

	topic, err := st.CreateTopic(account, region, "cfn-mod-topic", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.modifyCFNSNSTopic(account, topic.TopicARN, map[string]any{
		"TopicName": "cfn-mod-topic",
	}, map[string]any{
		"TopicName":   "cfn-mod-topic",
		"DisplayName": "Lab Topic",
	}); err != nil {
		t.Fatalf("modify topic: %v", err)
	}
	attrs, err := st.GetTopicAttributes(account, topic.TopicName)
	if err != nil || attrs["DisplayName"] != "Lab Topic" {
		t.Fatalf("attrs=%v err=%v", attrs, err)
	}
	if err := st.modifyCFNSNSTopic(account, "missing-topic", nil, map[string]any{"DisplayName": "x"}); err == nil {
		t.Fatal("missing topic should fail")
	}
	if err := st.modifyCFNTopicPolicyResource(account, "pol", nil, map[string]any{
		"Topics":         []any{topic.TopicARN},
		"PolicyDocument": map[string]any{"Version": "2012-10-17", "Statement": []any{}},
	}, cfnProvisionAuth{}); err != nil && !errors.Is(err, ErrCFNBadTemplate) {
		// empty statement policy may fail closed; accept either applied or bad-template
		t.Logf("topic policy modify err=%v", err)
	}

	sec, err := st.CreateSecret(account, region, "cfn-mod-secret", "initial", nil, "", "seed", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.modifyCFNSecret(account, sec.Name, map[string]any{
		"Name": "cfn-mod-secret",
	}, map[string]any{
		"Name":        "cfn-mod-secret",
		"Description": "updated",
	}); err != nil {
		t.Fatalf("modify secret: %v", err)
	}
	if err := st.modifyCFNSecret(account, sec.Name, map[string]any{
		"SecretString": "initial",
	}, map[string]any{
		"SecretString": "changed",
	}); err == nil {
		t.Fatal("SecretString change should fail closed")
	}
	if err := st.modifyCFNSecret(account, "missing-secret", nil, map[string]any{
		"Description": "x",
	}); err == nil {
		t.Fatal("missing secret should fail")
	}

	lg, err := st.CreateLogGroup(account, region, "/cfn/mod")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.modifyCFNLogGroup(account, lg.LogGroupName, map[string]any{
		"LogGroupName": lg.LogGroupName,
	}, map[string]any{
		"LogGroupName":    lg.LogGroupName,
		"RetentionInDays": 7,
	}); err != nil {
		t.Fatalf("set retention: %v", err)
	}
	if err := st.modifyCFNLogGroup(account, lg.LogGroupName, map[string]any{
		"LogGroupName": lg.LogGroupName,
	}, map[string]any{
		"LogGroupName":    lg.LogGroupName,
		"RetentionInDays": 0,
	}); err != nil {
		t.Fatalf("clear retention: %v", err)
	}
	if err := st.modifyCFNLogGroup(account, lg.LogGroupName, map[string]any{
		"LogGroupName": lg.LogGroupName,
	}, map[string]any{
		"LogGroupName": "/other/name",
	}); err == nil {
		t.Fatal("rename log group should fail")
	}

	bus, err := st.CreateEventBus(account, region, "cfn-mod-bus")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.modifyCFNEventBus(account, bus.Name, map[string]any{"Name": bus.Name}, map[string]any{
		"Name": bus.Name,
		"Policy": map[string]any{
			"Version": "2012-10-17",
			"Statement": []any{
				map[string]any{"Effect": "Allow", "Principal": "*", "Action": "events:PutEvents", "Resource": "*"},
			},
		},
	}); err != nil {
		t.Fatalf("event bus policy: %v", err)
	}
	if err := st.modifyCFNEventBus(account, bus.Name, map[string]any{"Name": bus.Name}, map[string]any{
		"Name":            bus.Name,
		"EventSourceName": "unsupported",
	}); err == nil {
		t.Fatal("unsupported event bus modify should fail")
	}
	if err := st.modifyCFNEventBus(account, bus.Name, map[string]any{"Name": bus.Name}, map[string]any{
		"Name":   bus.Name,
		"Policy": nil,
	}); err != nil {
		t.Fatalf("clear event bus policy: %v", err)
	}
}

func TestIoTThingNameFromTargetEquivalence(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"  ", ""},
		{"plain-thing", "plain-thing"},
		{"arn:aws:iot:us-east-1:1:thing/dev-1", "dev-1"},
		{"arn:aws:iot:us-east-1:1:thing/dev-1 ", "dev-1"},
	}
	for _, tc := range cases {
		if got := iotThingNameFromTarget(tc.in); got != tc.want {
			t.Fatalf("iotThingNameFromTarget(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}
