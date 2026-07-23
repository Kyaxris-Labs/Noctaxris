package cfn_test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func endpoint() string {
	if v := strings.TrimSpace(os.Getenv("NOCTAXRIS_ENDPOINT")); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "http://127.0.0.1:4566"
}

func requireReady(t *testing.T) {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(endpoint() + "/_noctaxris/ready")
	if err != nil {
		t.Skipf("Noctaxris not reachable: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Skipf("Noctaxris not ready: status %d", resp.StatusCode)
	}
}

func loadCFG(t *testing.T) aws.Config {
	t.Helper()
	accessKey := envOr("AWS_ACCESS_KEY_ID", "AKIAROOTEXAMPLE01")
	secretKey := envOr("AWS_SECRET_ACCESS_KEY", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")
	region := envOr("AWS_DEFAULT_REGION", "us-east-1")
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
	)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	return cfg
}

func envOr(k, d string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return d
}

func newCFN(t *testing.T, cfg aws.Config) *cloudformation.Client {
	t.Helper()
	return cloudformation.NewFromConfig(cfg, func(o *cloudformation.Options) {
		o.BaseEndpoint = aws.String(endpoint())
	})
}

func templatesDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "templates"))
}

type templateVars struct {
	Bucket   string
	Role     string
	Queue    string
	Table    string
	Function string
}

func renderTemplate(t *testing.T, name string, vars templateVars) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(templatesDir(t), name))
	if err != nil {
		t.Fatalf("read template: %v", err)
	}
	body := string(raw)
	body = strings.ReplaceAll(body, "REPLACE_BUCKET_NAME", vars.Bucket)
	body = strings.ReplaceAll(body, "REPLACE_ROLE_NAME", vars.Role)
	body = strings.ReplaceAll(body, "REPLACE_QUEUE_NAME", vars.Queue)
	body = strings.ReplaceAll(body, "REPLACE_TABLE_NAME", vars.Table)
	body = strings.ReplaceAll(body, "REPLACE_FUNCTION_NAME", vars.Function)
	return body
}

func createDescribeDeleteStack(t *testing.T, cfn *cloudformation.Client, stackName, body string) {
	t.Helper()
	ctx := context.Background()
	_, err := cfn.CreateStack(ctx, &cloudformation.CreateStackInput{
		StackName:    aws.String(stackName),
		TemplateBody: aws.String(body),
	})
	if err != nil {
		t.Fatalf("CreateStack: %v", err)
	}
	t.Cleanup(func() {
		_, _ = cfn.DeleteStack(ctx, &cloudformation.DeleteStackInput{StackName: aws.String(stackName)})
	})

	desc, err := cfn.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{
		StackName: aws.String(stackName),
	})
	if err != nil {
		t.Fatalf("DescribeStacks: %v", err)
	}
	if len(desc.Stacks) != 1 {
		t.Fatalf("DescribeStacks got %d stacks", len(desc.Stacks))
	}
	st := desc.Stacks[0]
	if st.StackStatus != types.StackStatusCreateComplete && string(st.StackStatus) != "CREATE_COMPLETE" {
		if st.StackStatus == "" {
			t.Fatalf("unexpected empty StackStatus")
		}
	}

	_, err = cfn.DeleteStack(ctx, &cloudformation.DeleteStackInput{StackName: aws.String(stackName)})
	if err != nil {
		t.Fatalf("DeleteStack: %v", err)
	}
}

func TestCloudFormationS3AndIAMStack(t *testing.T) {
	requireReady(t)
	cfg := loadCFG(t)
	cfn := newCFN(t, cfg)
	s3c := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint())
		o.UsePathStyle = true
	})
	ctx := context.Background()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	bucket := fmt.Sprintf("cfn-it-%s", suffix)
	if len(bucket) > 63 {
		bucket = bucket[:63]
	}
	role := fmt.Sprintf("CfnItRole%s", suffix[:8])
	stackName := fmt.Sprintf("cfn-it-%s", suffix[:12])
	body := renderTemplate(t, "s3-and-iam.json", templateVars{Bucket: bucket, Role: role})

	_, err := cfn.CreateStack(ctx, &cloudformation.CreateStackInput{
		StackName:    aws.String(stackName),
		TemplateBody: aws.String(body),
	})
	if err != nil {
		t.Fatalf("CreateStack: %v", err)
	}
	t.Cleanup(func() {
		_, _ = cfn.DeleteStack(ctx, &cloudformation.DeleteStackInput{StackName: aws.String(stackName)})
	})

	desc, err := cfn.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{
		StackName: aws.String(stackName),
	})
	if err != nil {
		t.Fatalf("DescribeStacks: %v", err)
	}
	if len(desc.Stacks) != 1 {
		t.Fatalf("DescribeStacks got %d stacks", len(desc.Stacks))
	}

	list, err := cfn.ListStacks(ctx, &cloudformation.ListStacksInput{})
	if err != nil {
		t.Fatalf("ListStacks: %v", err)
	}
	found := false
	for _, sum := range list.StackSummaries {
		if sum.StackName != nil && *sum.StackName == stackName {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("ListStacks missing %s", stackName)
	}

	_, err = s3c.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(bucket)})
	if err != nil {
		t.Fatalf("HeadBucket after CreateStack: %v", err)
	}

	_, err = cfn.DeleteStack(ctx, &cloudformation.DeleteStackInput{StackName: aws.String(stackName)})
	if err != nil {
		t.Fatalf("DeleteStack: %v", err)
	}

	_, err = cfn.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{
		StackName: aws.String(stackName),
	})
	if err == nil {
		t.Fatal("expected DescribeStacks after delete to fail")
	}
}

func TestCloudFormationYAMLStack(t *testing.T) {
	requireReady(t)
	cfg := loadCFG(t)
	cfn := newCFN(t, cfg)

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	short := suffix
	if len(short) > 12 {
		short = short[:12]
	}

	cases := []struct {
		name     string
		template string
		vars     templateVars
	}{
		{
			name:     "sqs",
			template: "sqs-queue.yaml",
			vars:     templateVars{Queue: "cfn-q-" + short},
		},
		{
			name:     "dynamodb",
			template: "dynamodb-table.yaml",
			vars:     templateVars{Table: "cfn-ddb-" + short},
		},
		{
			name:     "lambda-zipfile",
			template: "lambda-zipfile.yaml",
			vars: templateVars{
				Role:     "CfnLamRole" + short[:8],
				Function: "cfn-fn-" + short,
			},
		},
		{
			name:     "multi-resource",
			template: "multi-resource.yaml",
			vars: templateVars{
				Bucket:   "cfn-yml-" + short,
				Role:     "CfnYmlRole" + short[:8],
				Table:    "cfn-yml-ddb-" + short,
				Function: "cfn-yml-fn-" + short,
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if tc.vars.Bucket != "" && len(tc.vars.Bucket) > 63 {
				tc.vars.Bucket = tc.vars.Bucket[:63]
			}
			stackName := fmt.Sprintf("cfn-yaml-%s-%s", tc.name, short)
			body := renderTemplate(t, tc.template, tc.vars)
			createDescribeDeleteStack(t, cfn, stackName, body)
		})
	}
}

func TestCloudFormationRejectsUnsupportedType(t *testing.T) {
	requireReady(t)
	cfg := loadCFG(t)
	cfn := newCFN(t, cfg)
	ctx := context.Background()

	stackName := fmt.Sprintf("cfn-bad-%d", time.Now().UnixNano())
	body := `{"Resources":{"X":{"Type":"AWS::EC2::Instance","Properties":{}}}}`
	_, err := cfn.CreateStack(ctx, &cloudformation.CreateStackInput{
		StackName:    aws.String(stackName),
		TemplateBody: aws.String(body),
	})
	if err == nil {
		t.Cleanup(func() {
			_, _ = cfn.DeleteStack(ctx, &cloudformation.DeleteStackInput{StackName: aws.String(stackName)})
		})
		t.Fatal("expected CreateStack with unsupported type to fail")
	}
}
