package sdk_test

import (
	"context"
	"encoding/csv"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/sts"
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

func TestIAMAccessKeyLastUsedAndCredentialReport(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	client := newIAM(t, cfg)
	ctx := context.Background()
	userName := uniquePrefix(t) + "-lastused"

	_, err := client.CreateUser(ctx, &iam.CreateUserInput{UserName: aws.String(userName)})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	t.Cleanup(func() {
		_, _ = client.DeleteUser(ctx, &iam.DeleteUserInput{UserName: aws.String(userName)})
	})

	keyOut, err := client.CreateAccessKey(ctx, &iam.CreateAccessKeyInput{UserName: aws.String(userName)})
	if err != nil {
		t.Fatalf("CreateAccessKey: %v", err)
	}
	if keyOut.AccessKey == nil || keyOut.AccessKey.AccessKeyId == nil || keyOut.AccessKey.SecretAccessKey == nil {
		t.Fatalf("CreateAccessKey missing key: %+v", keyOut.AccessKey)
	}
	accessKeyID := *keyOut.AccessKey.AccessKeyId
	secret := *keyOut.AccessKey.SecretAccessKey
	t.Cleanup(func() {
		_, _ = client.DeleteAccessKey(ctx, &iam.DeleteAccessKeyInput{
			UserName:    aws.String(userName),
			AccessKeyId: aws.String(accessKeyID),
		})
	})

	userCfg := cfg.Copy()
	userCfg.Credentials = credentials.NewStaticCredentialsProvider(accessKeyID, secret, "")
	userSTS := sts.NewFromConfig(userCfg, func(o *sts.Options) {
		o.BaseEndpoint = aws.String(endpoint())
	})
	if _, err := userSTS.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{}); err != nil {
		t.Fatalf("GetCallerIdentity with user key: %v", err)
	}

	lastUsed, err := client.GetAccessKeyLastUsed(ctx, &iam.GetAccessKeyLastUsedInput{
		AccessKeyId: aws.String(accessKeyID),
	})
	if err != nil {
		t.Fatalf("GetAccessKeyLastUsed: %v", err)
	}
	if lastUsed.UserName == nil || *lastUsed.UserName != userName {
		t.Fatalf("UserName=%v want %s", lastUsed.UserName, userName)
	}
	if lastUsed.AccessKeyLastUsed == nil {
		t.Fatal("AccessKeyLastUsed missing")
	}
	if aws.ToString(lastUsed.AccessKeyLastUsed.ServiceName) != "sts" {
		t.Fatalf("ServiceName=%q want sts", aws.ToString(lastUsed.AccessKeyLastUsed.ServiceName))
	}
	region := envOr("AWS_DEFAULT_REGION", "us-east-1")
	if aws.ToString(lastUsed.AccessKeyLastUsed.Region) != region {
		t.Fatalf("Region=%q want %s", aws.ToString(lastUsed.AccessKeyLastUsed.Region), region)
	}

	gen, err := client.GenerateCredentialReport(ctx, &iam.GenerateCredentialReportInput{})
	if err != nil {
		t.Fatalf("GenerateCredentialReport: %v", err)
	}
	if gen.State == "" || !strings.EqualFold(string(gen.State), "COMPLETE") {
		t.Fatalf("GenerateCredentialReport State=%q", gen.State)
	}

	report, err := client.GetCredentialReport(ctx, &iam.GetCredentialReportInput{})
	if err != nil {
		t.Fatalf("GetCredentialReport: %v", err)
	}
	if len(report.Content) == 0 {
		t.Fatal("GetCredentialReport empty Content")
	}
	rows, err := csv.NewReader(strings.NewReader(string(report.Content))).ReadAll()
	if err != nil {
		t.Fatalf("parse credential report CSV: %v", err)
	}
	found := false
	for _, row := range rows[1:] {
		if len(row) > 0 && row[0] == userName {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("credential report missing user %s: %s", userName, report.Content)
	}
}
