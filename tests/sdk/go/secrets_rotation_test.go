package sdk_test

import (
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
)

func TestSecretsRotationRulesDeferDescribe(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	client := newSecrets(t, cfg)
	ctx := context.Background()
	prefix := uniquePrefix(t)
	name := "sdk-rot-" + prefix

	_, err := client.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
		Name:         aws.String(name),
		SecretString: aws.String("v1"),
	})
	if err != nil {
		t.Fatalf("CreateSecret: %v", err)
	}
	t.Cleanup(func() {
		_, _ = client.DeleteSecret(ctx, &secretsmanager.DeleteSecretInput{
			SecretId:                   aws.String(name),
			ForceDeleteWithoutRecovery: aws.Bool(true),
		})
	})

	_, err = client.RotateSecret(ctx, &secretsmanager.RotateSecretInput{
		SecretId: aws.String(name),
		RotationRules: &types.RotationRulesType{
			AutomaticallyAfterDays: aws.Int64(7),
		},
		RotateImmediately: aws.Bool(false),
	})
	if err != nil {
		t.Fatalf("RotateSecret rules defer: %v", err)
	}

	got, err := client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{SecretId: aws.String(name)})
	if err != nil {
		t.Fatalf("GetSecretValue: %v", err)
	}
	if aws.ToString(got.SecretString) != "v1" {
		t.Fatalf("deferred rotate changed value: %q", aws.ToString(got.SecretString))
	}

	desc, err := client.DescribeSecret(ctx, &secretsmanager.DescribeSecretInput{SecretId: aws.String(name)})
	if err != nil {
		t.Fatalf("DescribeSecret: %v", err)
	}
	if !aws.ToBool(desc.RotationEnabled) {
		t.Fatal("want RotationEnabled")
	}
	if desc.RotationRules == nil || aws.ToInt64(desc.RotationRules.AutomaticallyAfterDays) != 7 {
		t.Fatalf("rules=%+v", desc.RotationRules)
	}
	if desc.NextRotationDate == nil {
		t.Fatal("want NextRotationDate")
	}
}

func TestSecretsRotationRulesScheduleExpression(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	client := newSecrets(t, cfg)
	ctx := context.Background()
	prefix := uniquePrefix(t)
	name := "sdk-rot-sched-" + prefix

	_, err := client.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
		Name:         aws.String(name),
		SecretString: aws.String("sched-v1"),
	})
	if err != nil {
		t.Fatalf("CreateSecret: %v", err)
	}
	t.Cleanup(func() {
		_, _ = client.DeleteSecret(ctx, &secretsmanager.DeleteSecretInput{
			SecretId:                   aws.String(name),
			ForceDeleteWithoutRecovery: aws.Bool(true),
		})
	})

	_, err = client.RotateSecret(ctx, &secretsmanager.RotateSecretInput{
		SecretId: aws.String(name),
		RotationRules: &types.RotationRulesType{
			ScheduleExpression: aws.String("rate(4 hours)"),
		},
		RotateImmediately: aws.Bool(false),
	})
	if err != nil {
		t.Fatalf("RotateSecret schedule: %v", err)
	}

	desc, err := client.DescribeSecret(ctx, &secretsmanager.DescribeSecretInput{SecretId: aws.String(name)})
	if err != nil {
		t.Fatalf("DescribeSecret: %v", err)
	}
	if !aws.ToBool(desc.RotationEnabled) {
		t.Fatal("want RotationEnabled")
	}
	if desc.RotationRules == nil || aws.ToString(desc.RotationRules.ScheduleExpression) != "rate(4 hours)" {
		t.Fatalf("rules=%+v", desc.RotationRules)
	}
	if desc.NextRotationDate == nil {
		t.Fatal("want NextRotationDate")
	}
}

func TestSecretsRotationRulesRejectBothDaysAndSchedule(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	client := newSecrets(t, cfg)
	ctx := context.Background()
	prefix := uniquePrefix(t)
	name := "sdk-rot-both-" + prefix

	_, err := client.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
		Name:         aws.String(name),
		SecretString: aws.String("both-v1"),
	})
	if err != nil {
		t.Fatalf("CreateSecret: %v", err)
	}
	t.Cleanup(func() {
		_, _ = client.DeleteSecret(ctx, &secretsmanager.DeleteSecretInput{
			SecretId:                   aws.String(name),
			ForceDeleteWithoutRecovery: aws.Bool(true),
		})
	})

	_, err = client.RotateSecret(ctx, &secretsmanager.RotateSecretInput{
		SecretId: aws.String(name),
		RotationRules: &types.RotationRulesType{
			AutomaticallyAfterDays: aws.Int64(7),
			ScheduleExpression:     aws.String("rate(7 days)"),
		},
		RotateImmediately: aws.Bool(false),
	})
	if err == nil {
		t.Fatal("expected ValidationException when both AutomaticallyAfterDays and ScheduleExpression set")
	}
	if !strings.Contains(err.Error(), "ValidationException") {
		t.Fatalf("want ValidationException, got %v", err)
	}
}
