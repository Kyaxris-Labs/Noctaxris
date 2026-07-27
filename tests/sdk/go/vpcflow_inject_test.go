package sdk_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func TestVPCFlowCreateInjectS3AcceptReject(t *testing.T) {
	requireReady(t)
	if os.Getenv("NOCTAXRIS_VPCFLOW_INJECT") != "1" {
		t.Skip("set NOCTAXRIS_VPCFLOW_INJECT=1 on the API process for lab InjectFlowLogs")
	}

	cfg := loadAWSConfig(t)
	s3c := newS3(t, cfg)
	ctx := context.Background()
	prefix := uniquePrefix(t)
	bucket := strings.ToLower(prefix + "-vpcflow")

	if _, err := s3c.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)}); err != nil {
		t.Fatalf("CreateBucket: %v", err)
	}
	t.Cleanup(func() {
		listed, _ := s3c.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String(bucket)})
		for _, obj := range listed.Contents {
			_, _ = s3c.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: obj.Key})
		}
		_, _ = s3c.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: aws.String(bucket)})
	})

	createStatus, createBody, createParsed := signedJSONTarget(t, "ec2", "AmazonEC2.CreateFlowLogs", map[string]any{
		"ResourceIds":        []string{"vpc-labopaque001"},
		"ResourceType":       "VPC",
		"TrafficType":        "ALL",
		"LogDestinationType": "s3",
		"LogDestination":     fmt.Sprintf("arn:aws:s3:::%s", bucket),
	})
	if createStatus != 200 {
		t.Fatalf("CreateFlowLogs status=%d body=%s", createStatus, createBody)
	}
	flowLogIDs, _ := createParsed["FlowLogIds"].([]any)
	if len(flowLogIDs) != 1 {
		t.Fatalf("CreateFlowLogs FlowLogIds=%v", createParsed)
	}
	flowLogID, _ := flowLogIDs[0].(string)
	if !strings.HasPrefix(flowLogID, "fl-") {
		t.Fatalf("FlowLogId=%q want fl- prefix", flowLogID)
	}

	injStatus, injBody, _ := signedJSONTarget(t, "ec2", "NoctaxrisEC2.InjectFlowLogs", map[string]any{
		"FlowLogId": flowLogID,
	})
	if injStatus != 200 {
		t.Fatalf("InjectFlowLogs status=%d body=%s", injStatus, injBody)
	}

	listed, err := s3c.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String("AWSLogs/"),
	})
	if err != nil {
		t.Fatalf("ListObjectsV2: %v", err)
	}
	if len(listed.Contents) == 0 {
		t.Fatal("no flow log object in S3")
	}
	obj, err := s3c.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    listed.Contents[0].Key,
	})
	if err != nil {
		t.Fatalf("GetObject: %v", err)
	}
	defer obj.Body.Close()
	raw, err := io.ReadAll(obj.Body)
	if err != nil {
		t.Fatalf("read object: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, " ACCEPT OK") || !strings.Contains(text, " REJECT OK") {
		t.Fatalf("s3 body=%q want ACCEPT+REJECT v2 lines", text)
	}
}
