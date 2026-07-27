package sdk_test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/kinesis"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

func endpoint() string {
	if v := strings.TrimSpace(os.Getenv("NOCTAXRIS_ENDPOINT")); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "http://127.0.0.1:4566"
}

func requireReady(t *testing.T) {
	t.Helper()
	ep := endpoint()
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(ep + "/_noctaxris/ready")
	if err != nil {
		t.Skipf("Noctaxris not reachable at %s: %v", ep, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Skipf("Noctaxris not ready at %s: status %d", ep, resp.StatusCode)
	}
}

func uniquePrefix(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("it-%d", time.Now().UnixNano())
}

func loadAWSConfig(t *testing.T) aws.Config {
	t.Helper()
	accessKey := envOr("AWS_ACCESS_KEY_ID", "AKIAROOTEXAMPLE01")
	secretKey := envOr("AWS_SECRET_ACCESS_KEY", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")
	region := envOr("AWS_DEFAULT_REGION", "us-east-1")
	ep := endpoint()

	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
	)
	if err != nil {
		t.Fatalf("load aws config: %v", err)
	}
	cfg.BaseEndpoint = aws.String(ep)
	return cfg
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func newSTS(t *testing.T, cfg aws.Config) *sts.Client {
	t.Helper()
	return sts.NewFromConfig(cfg, func(o *sts.Options) {
		o.BaseEndpoint = aws.String(endpoint())
	})
}

func newS3(t *testing.T, cfg aws.Config) *s3.Client {
	t.Helper()
	return s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint())
		o.UsePathStyle = true
	})
}

func newDDB(t *testing.T, cfg aws.Config) *dynamodb.Client {
	t.Helper()
	return dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		o.BaseEndpoint = aws.String(endpoint())
	})
}

func newKinesis(t *testing.T, cfg aws.Config) *kinesis.Client {
	t.Helper()
	return kinesis.NewFromConfig(cfg, func(o *kinesis.Options) {
		o.BaseEndpoint = aws.String(endpoint())
	})
}

func newIAM(t *testing.T, cfg aws.Config) *iam.Client {
	t.Helper()
	return iam.NewFromConfig(cfg, func(o *iam.Options) {
		o.BaseEndpoint = aws.String(endpoint())
	})
}

func newKMS(t *testing.T, cfg aws.Config) *kms.Client {
	t.Helper()
	return kms.NewFromConfig(cfg, func(o *kms.Options) {
		o.BaseEndpoint = aws.String(endpoint())
	})
}

func newSQS(t *testing.T, cfg aws.Config) *sqs.Client {
	t.Helper()
	return sqs.NewFromConfig(cfg, func(o *sqs.Options) {
		o.BaseEndpoint = aws.String(endpoint())
	})
}

func newSNS(t *testing.T, cfg aws.Config) *sns.Client {
	t.Helper()
	return sns.NewFromConfig(cfg, func(o *sns.Options) {
		o.BaseEndpoint = aws.String(endpoint())
	})
}

func newLambda(t *testing.T, cfg aws.Config) *lambda.Client {
	t.Helper()
	return lambda.NewFromConfig(cfg, func(o *lambda.Options) {
		o.BaseEndpoint = aws.String(endpoint())
	})
}

func newEvents(t *testing.T, cfg aws.Config) *eventbridge.Client {
	t.Helper()
	return eventbridge.NewFromConfig(cfg, func(o *eventbridge.Options) {
		o.BaseEndpoint = aws.String(endpoint())
	})
}

func newSSM(t *testing.T, cfg aws.Config) *ssm.Client {
	t.Helper()
	return ssm.NewFromConfig(cfg, func(o *ssm.Options) {
		o.BaseEndpoint = aws.String(endpoint())
	})
}

func newSecrets(t *testing.T, cfg aws.Config) *secretsmanager.Client {
	t.Helper()
	return secretsmanager.NewFromConfig(cfg, func(o *secretsmanager.Options) {
		o.BaseEndpoint = aws.String(endpoint())
	})
}
