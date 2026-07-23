package sdk_test

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
)

func TestIAMUserAndRoleRoundTrip(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	client := newIAM(t, cfg)
	ctx := context.Background()
	prefix := uniquePrefix(t)

	userName := prefix + "-user"
	_, err := client.CreateUser(ctx, &iam.CreateUserInput{UserName: aws.String(userName)})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	t.Cleanup(func() {
		_, _ = client.DeleteUser(ctx, &iam.DeleteUserInput{UserName: aws.String(userName)})
	})

	user, err := client.GetUser(ctx, &iam.GetUserInput{UserName: aws.String(userName)})
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if user.User == nil || user.User.UserName == nil || *user.User.UserName != userName {
		t.Fatalf("GetUser unexpected: %+v", user.User)
	}

	roleName := prefix + "-role"
	trust := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	_, err = client.CreateRole(ctx, &iam.CreateRoleInput{
		RoleName:                 aws.String(roleName),
		AssumeRolePolicyDocument: aws.String(trust),
	})
	if err != nil {
		t.Fatalf("CreateRole: %v", err)
	}
	t.Cleanup(func() {
		_, _ = client.DeleteRole(ctx, &iam.DeleteRoleInput{RoleName: aws.String(roleName)})
	})

	role, err := client.GetRole(ctx, &iam.GetRoleInput{RoleName: aws.String(roleName)})
	if err != nil {
		t.Fatalf("GetRole: %v", err)
	}
	if role.Role == nil || role.Role.Arn == nil || *role.Role.Arn == "" {
		t.Fatalf("GetRole missing ARN: %+v", role.Role)
	}

	_, err = client.DeleteRole(ctx, &iam.DeleteRoleInput{RoleName: aws.String(roleName)})
	if err != nil {
		t.Fatalf("DeleteRole: %v", err)
	}
	_, err = client.DeleteUser(ctx, &iam.DeleteUserInput{UserName: aws.String(userName)})
	if err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
}
