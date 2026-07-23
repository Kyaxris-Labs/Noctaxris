package sdk_test

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/sts"
)

func TestSTSGetCallerIdentity(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	client := newSTS(t, cfg)

	out, err := client.GetCallerIdentity(context.Background(), &sts.GetCallerIdentityInput{})
	if err != nil {
		t.Fatalf("GetCallerIdentity: %v", err)
	}
	if out.Account == nil || *out.Account == "" {
		t.Fatal("expected Account")
	}
	if out.Arn == nil || *out.Arn == "" {
		t.Fatal("expected Arn")
	}
	if out.UserId == nil || *out.UserId == "" {
		t.Fatal("expected UserId")
	}
	t.Logf("caller account=%s arn=%s", *out.Account, *out.Arn)
}
