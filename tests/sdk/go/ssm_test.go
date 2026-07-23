package sdk_test

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

func TestSSMStringParameterRoundTrip(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	client := newSSM(t, cfg)
	ctx := context.Background()
	prefix := uniquePrefix(t)
	name := "/lab/" + prefix + "/config"
	value := "plain-" + prefix

	_, err := client.PutParameter(ctx, &ssm.PutParameterInput{
		Name:  aws.String(name),
		Type:  types.ParameterTypeString,
		Value: aws.String(value),
	})
	if err != nil {
		t.Fatalf("PutParameter: %v", err)
	}
	t.Cleanup(func() {
		_, _ = client.DeleteParameter(ctx, &ssm.DeleteParameterInput{Name: aws.String(name)})
	})

	got, err := client.GetParameter(ctx, &ssm.GetParameterInput{Name: aws.String(name)})
	if err != nil {
		t.Fatalf("GetParameter: %v", err)
	}
	if got.Parameter == nil || aws.ToString(got.Parameter.Value) != value {
		t.Fatalf("GetParameter unexpected: %+v", got.Parameter)
	}

	_, err = client.DeleteParameter(ctx, &ssm.DeleteParameterInput{Name: aws.String(name)})
	if err != nil {
		t.Fatalf("DeleteParameter: %v", err)
	}
}
