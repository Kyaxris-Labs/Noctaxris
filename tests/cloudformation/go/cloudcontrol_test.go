package cfn_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	cctypes "github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
)

func newCloudControl(t *testing.T, cfg aws.Config) *cloudcontrol.Client {
	t.Helper()
	return cloudcontrol.NewFromConfig(cfg, func(o *cloudcontrol.Options) {
		o.BaseEndpoint = aws.String(endpoint())
	})
}

func newIAM(t *testing.T, cfg aws.Config) *iam.Client {
	t.Helper()
	return iam.NewFromConfig(cfg, func(o *iam.Options) {
		o.BaseEndpoint = aws.String(endpoint())
	})
}

func waitCloudControlSuccess(t *testing.T, cc *cloudcontrol.Client, token string) {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(15 * time.Second)
	for {
		st, err := cc.GetResourceRequestStatus(ctx, &cloudcontrol.GetResourceRequestStatusInput{
			RequestToken: aws.String(token),
		})
		if err != nil {
			t.Fatalf("GetResourceRequestStatus: %v", err)
		}
		if st.ProgressEvent == nil {
			t.Fatal("GetResourceRequestStatus missing ProgressEvent")
		}
		if st.ProgressEvent.OperationStatus == cctypes.OperationStatusSuccess {
			return
		}
		if st.ProgressEvent.OperationStatus == cctypes.OperationStatusFailed {
			t.Fatalf("operation failed: %+v", st.ProgressEvent)
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for SUCCESS, status=%v", st.ProgressEvent.OperationStatus)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func TestCloudControlUpdateIAMUser(t *testing.T) {
	requireReady(t)
	cfg := loadCFG(t)
	cc := newCloudControl(t, cfg)
	iamc := newIAM(t, cfg)
	ctx := context.Background()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	short := suffix
	if len(short) > 12 {
		short = short[:12]
	}
	userName := "cc-upd-user-" + short
	groupName := "cc-upd-group-" + short

	// Prefer IAM create over Cloud Control CreateResource so the suite exercises
	// UpdateResource against a live user without requiring CreateResource allowlist depth.
	_, err := iamc.CreateUser(ctx, &iam.CreateUserInput{UserName: aws.String(userName)})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	t.Cleanup(func() {
		_, _ = iamc.DeleteUser(ctx, &iam.DeleteUserInput{UserName: aws.String(userName)})
	})

	_, err = iamc.CreateGroup(ctx, &iam.CreateGroupInput{GroupName: aws.String(groupName)})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	t.Cleanup(func() {
		_, _ = iamc.RemoveUserFromGroup(ctx, &iam.RemoveUserFromGroupInput{
			GroupName: aws.String(groupName),
			UserName:  aws.String(userName),
		})
		_, _ = iamc.DeleteGroup(ctx, &iam.DeleteGroupInput{GroupName: aws.String(groupName)})
	})

	// Lab PatchDocument is a property object (CreateResource DesiredState shape), not RFC6902.
	patch, err := json.Marshal(map[string]any{
		"Groups": []any{groupName},
		"Policies": []any{
			map[string]any{
				"PolicyName": "u-inline",
				"PolicyDocument": map[string]any{
					"Version": "2012-10-17",
					"Statement": []any{
						map[string]any{
							"Effect":   "Allow",
							"Action":   "sqs:SendMessage",
							"Resource": "*",
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal patch: %v", err)
	}

	upd, err := cc.UpdateResource(ctx, &cloudcontrol.UpdateResourceInput{
		TypeName:      aws.String("AWS::IAM::User"),
		Identifier:    aws.String(userName),
		PatchDocument: aws.String(string(patch)),
	})
	if err != nil {
		t.Fatalf("UpdateResource: %v", err)
	}
	if upd.ProgressEvent == nil || upd.ProgressEvent.RequestToken == nil {
		t.Fatal("UpdateResource missing ProgressEvent.RequestToken")
	}
	waitCloudControlSuccess(t, cc, aws.ToString(upd.ProgressEvent.RequestToken))

	got, err := cc.GetResource(ctx, &cloudcontrol.GetResourceInput{
		TypeName:   aws.String("AWS::IAM::User"),
		Identifier: aws.String(userName),
	})
	if err != nil {
		t.Fatalf("GetResource: %v", err)
	}
	if got.ResourceDescription == nil || aws.ToString(got.ResourceDescription.Identifier) != userName {
		t.Fatalf("GetResource identifier=%v", got.ResourceDescription)
	}
	props := aws.ToString(got.ResourceDescription.Properties)
	if !strings.Contains(props, userName) {
		t.Fatalf("GetResource properties missing user: %s", props)
	}

	// ListGroupsForUser is not in the lab IAM surface; GetGroup returns members.
	groupOut2, err := iamc.GetGroup(ctx, &iam.GetGroupInput{GroupName: aws.String(groupName)})
	if err != nil {
		t.Fatalf("GetGroup: %v", err)
	}
	foundMember := false
	for _, u := range groupOut2.Users {
		if aws.ToString(u.UserName) == userName {
			foundMember = true
			break
		}
	}
	if !foundMember {
		t.Fatalf("GetGroup members after UpdateResource: %+v", groupOut2.Users)
	}

	inline, err := iamc.GetUserPolicy(ctx, &iam.GetUserPolicyInput{
		UserName:   aws.String(userName),
		PolicyName: aws.String("u-inline"),
	})
	if err != nil {
		t.Fatalf("GetUserPolicy: %v", err)
	}
	doc := aws.ToString(inline.PolicyDocument)
	if !strings.Contains(doc, "sqs:SendMessage") {
		t.Fatalf("inline policy=%q", doc)
	}

	_, err = cc.UpdateResource(ctx, &cloudcontrol.UpdateResourceInput{
		TypeName:      aws.String("AWS::IAM::User"),
		Identifier:    aws.String(userName),
		PatchDocument: aws.String(`{"Path":"/admin/"}`),
	})
	if err == nil {
		t.Fatal("expected UpdateResource Path patch to fail closed")
	}

	t.Cleanup(func() {
		_, _ = iamc.DeleteUserPolicy(ctx, &iam.DeleteUserPolicyInput{
			UserName:   aws.String(userName),
			PolicyName: aws.String("u-inline"),
		})
	})
}
