package store_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestCloudControlUpdateIAMUserGroupManagedPolicy(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"

	if _, _, err := st.CreateUser(account, "cc-upd-user"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.CreateGroup(account, "cc-upd-group"); err != nil {
		t.Fatal(err)
	}
	policyARN, err := st.CreateManagedPolicy(account, "cc-upd-mp", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"}]}`)
	if err != nil {
		t.Fatal(err)
	}

	userPatch := map[string]any{
		"Policies": []any{
			map[string]any{
				"PolicyName": "u-inline",
				"PolicyDocument": map[string]any{
					"Version": "2012-10-17",
					"Statement": []any{
						map[string]any{"Effect": "Allow", "Action": "sqs:SendMessage", "Resource": "*"},
					},
				},
			},
		},
		"ManagedPolicyArns": []any{policyARN},
		"Groups":            []any{"cc-upd-group"},
	}
	raw, _ := json.Marshal(userPatch)
	if _, _, err := st.CloudControlUpdateResource(account, "AWS::IAM::User", "cc-upd-user", string(raw)); err != nil {
		t.Fatal(err)
	}
	userARN := store.UserARN(account, "/", "cc-upd-user")
	pol, err := st.GetInlinePolicy(userARN, "u-inline")
	if err != nil || !strings.Contains(pol.Document, "sqs:SendMessage") {
		t.Fatalf("user inline=%+v err=%v", pol, err)
	}
	groups, err := st.ListGroupsForUser(account, "cc-upd-user")
	if err != nil || len(groups) != 1 || groups[0].GroupName != "cc-upd-group" {
		t.Fatalf("groups=%+v err=%v", groups, err)
	}
	attached, err := st.ListAttachedPolicyRefs(userARN)
	if err != nil || len(attached) != 1 || attached[0].PolicyARN != policyARN {
		t.Fatalf("attached=%+v err=%v", attached, err)
	}

	groupPatch := map[string]any{
		"Policies": []any{
			map[string]any{
				"PolicyName": "g-inline",
				"PolicyDocument": map[string]any{
					"Version": "2012-10-17",
					"Statement": []any{
						map[string]any{"Effect": "Allow", "Action": "sns:Publish", "Resource": "*"},
					},
				},
			},
		},
		"ManagedPolicyArns": []any{policyARN},
	}
	raw, _ = json.Marshal(groupPatch)
	if _, _, err := st.CloudControlUpdateResource(account, "AWS::IAM::Group", "cc-upd-group", string(raw)); err != nil {
		t.Fatal(err)
	}
	groupARN := store.GroupARN(account, "/", "cc-upd-group")
	gpol, err := st.GetInlinePolicy(groupARN, "g-inline")
	if err != nil || !strings.Contains(gpol.Document, "sns:Publish") {
		t.Fatalf("group inline=%+v err=%v", gpol, err)
	}

	mpPatch := map[string]any{
		"PolicyDocument": map[string]any{
			"Version": "2012-10-17",
			"Statement": []any{
				map[string]any{"Effect": "Allow", "Action": "s3:PutObject", "Resource": "*"},
			},
		},
		"Users": []any{"cc-upd-user"},
	}
	raw, _ = json.Marshal(mpPatch)
	if _, _, err := st.CloudControlUpdateResource(account, "AWS::IAM::ManagedPolicy", policyARN, string(raw)); err != nil {
		t.Fatal(err)
	}
	mp, err := st.GetManagedPolicy(policyARN)
	if err != nil || !strings.Contains(mp.Document, "s3:PutObject") {
		t.Fatalf("managed=%+v err=%v", mp, err)
	}

	if _, _, err := st.CloudControlUpdateResource(account, "AWS::IAM::User", "cc-upd-user", `{"Path":"/admin/"}`); err == nil {
		t.Fatal("expected Path patch reject")
	}
}

func TestCloudControlUpdateEventBusPolicy(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	if _, err := st.CreateEventBus(account, "us-east-1", "cc-bus-pol"); err != nil {
		t.Fatal(err)
	}
	policy := `{"Version":"2012-10-17","Statement":[{"Sid":"AllowPut","Effect":"Allow","Principal":{"AWS":"arn:aws:iam::000000000001:root"},"Action":"events:PutEvents","Resource":"*"}]}`
	patch := `{"Policy":` + policy + `}`
	if _, _, err := st.CloudControlUpdateResource(account, "AWS::Events::EventBus", "cc-bus-pol", patch); err != nil {
		t.Fatal(err)
	}
	bus, err := st.DescribeEventBus(account, "cc-bus-pol")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(bus.Policy, "AllowPut") {
		t.Fatalf("policy=%s", bus.Policy)
	}
	if _, _, err := st.CloudControlUpdateResource(account, "AWS::Events::EventBus", "cc-bus-pol", `{"Name":"renamed"}`); err == nil {
		t.Fatal("expected Name patch reject")
	}
}
