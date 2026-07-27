package sdk_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func TestS3ForensicsObjectLockLite(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	client := newS3(t, cfg)
	ctx := context.Background()
	bucket := strings.ToLower(uniquePrefix(t) + "-lock")
	key := "locked.txt"

	_, err := client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket:                     aws.String(bucket),
		ObjectLockEnabledForBucket: aws.Bool(true),
	})
	if err != nil {
		t.Fatalf("CreateBucket ObjectLock: %v", err)
	}
	t.Cleanup(func() {
		_, _ = client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket:                    aws.String(bucket),
			Key:                       aws.String(key),
			BypassGovernanceRetention: aws.Bool(true),
		})
		_, _ = client.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: aws.String(bucket)})
	})

	retain := time.Now().UTC().Add(24 * time.Hour)
	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:                    aws.String(bucket),
		Key:                       aws.String(key),
		Body:                      bytes.NewReader([]byte("secret")),
		ObjectLockMode:            s3types.ObjectLockModeGovernance,
		ObjectLockRetainUntilDate: aws.Time(retain),
	})
	if err != nil {
		t.Fatalf("PutObject with Object Lock: %v", err)
	}

	_, err = client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err == nil {
		t.Fatal("DeleteObject without bypass expected AccessDenied")
	}

	_, err = client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket:                    aws.String(bucket),
		Key:                       aws.String(key),
		BypassGovernanceRetention: aws.Bool(true),
	})
	if err != nil {
		t.Fatalf("DeleteObject bypass governance: %v", err)
	}
}

func TestS3ForensicsBucketLogging(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	client := newS3(t, cfg)
	ctx := context.Background()
	prefix := uniquePrefix(t)
	src := strings.ToLower(prefix + "-src")
	logs := strings.ToLower(prefix + "-logs")
	key := "a.txt"

	for _, b := range []string{src, logs} {
		bucket := b
		_, err := client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)})
		if err != nil {
			t.Fatalf("CreateBucket %s: %v", bucket, err)
		}
		t.Cleanup(func() {
			listed, _ := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String(bucket)})
			if listed != nil {
				for _, obj := range listed.Contents {
					_, _ = client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: obj.Key})
				}
			}
			_, _ = client.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: aws.String(bucket)})
		})
	}

	_, err := client.PutBucketLogging(ctx, &s3.PutBucketLoggingInput{
		Bucket: aws.String(src),
		BucketLoggingStatus: &s3types.BucketLoggingStatus{
			LoggingEnabled: &s3types.LoggingEnabled{
				TargetBucket: aws.String(logs),
				TargetPrefix: aws.String("s3/"),
			},
		},
	})
	if err != nil {
		t.Fatalf("PutBucketLogging: %v", err)
	}

	gotLogging, err := client.GetBucketLogging(ctx, &s3.GetBucketLoggingInput{Bucket: aws.String(src)})
	if err != nil {
		t.Fatalf("GetBucketLogging: %v", err)
	}
	if gotLogging.LoggingEnabled == nil || aws.ToString(gotLogging.LoggingEnabled.TargetBucket) != logs {
		t.Fatalf("GetBucketLogging unexpected: %+v", gotLogging.LoggingEnabled)
	}

	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(src),
		Key:    aws.String(key),
		Body:   bytes.NewReader([]byte("abc")),
	})
	if err != nil {
		t.Fatalf("PutObject: %v", err)
	}

	listed, err := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(logs),
		Prefix: aws.String("s3/"),
	})
	if err != nil {
		t.Fatalf("ListObjectsV2 logs: %v", err)
	}
	if len(listed.Contents) < 1 {
		t.Fatalf("expected access log object under s3/, got %+v", listed.Contents)
	}
	obj, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(logs),
		Key:    listed.Contents[0].Key,
	})
	if err != nil {
		t.Fatalf("GetObject log: %v", err)
	}
	raw, err := io.ReadAll(obj.Body)
	_ = obj.Body.Close()
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	line := string(raw)
	if !strings.Contains(line, "REST.PUT.OBJECT") || !strings.Contains(line, key) {
		t.Fatalf("access log line=%q", line)
	}
}

func TestS3ForensicsVersioningDeleteMarkers(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	client := newS3(t, cfg)
	ctx := context.Background()
	bucket := strings.ToLower(uniquePrefix(t) + "-ver")
	key := "obj.txt"

	_, err := client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)})
	if err != nil {
		t.Fatalf("CreateBucket: %v", err)
	}
	t.Cleanup(func() {
		vers, _ := client.ListObjectVersions(ctx, &s3.ListObjectVersionsInput{Bucket: aws.String(bucket)})
		if vers != nil {
			for _, v := range vers.Versions {
				_, _ = client.DeleteObject(ctx, &s3.DeleteObjectInput{
					Bucket: aws.String(bucket), Key: v.Key, VersionId: v.VersionId,
				})
			}
			for _, m := range vers.DeleteMarkers {
				_, _ = client.DeleteObject(ctx, &s3.DeleteObjectInput{
					Bucket: aws.String(bucket), Key: m.Key, VersionId: m.VersionId,
				})
			}
		}
		_, _ = client.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: aws.String(bucket)})
	})

	_, err = client.PutBucketVersioning(ctx, &s3.PutBucketVersioningInput{
		Bucket: aws.String(bucket),
		VersioningConfiguration: &s3types.VersioningConfiguration{
			Status: s3types.BucketVersioningStatusEnabled,
		},
	})
	if err != nil {
		t.Fatalf("PutBucketVersioning: %v", err)
	}

	put, err := client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader([]byte("payload")),
	})
	if err != nil {
		t.Fatalf("PutObject: %v", err)
	}
	if put.VersionId == nil || *put.VersionId == "" {
		t.Fatal("PutObject missing VersionId")
	}

	del, err := client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		t.Fatalf("DeleteObject: %v", err)
	}
	if del.DeleteMarker == nil || !*del.DeleteMarker {
		t.Fatalf("expected DeleteMarker=true, got %+v", del)
	}

	listed, err := client.ListObjectVersions(ctx, &s3.ListObjectVersionsInput{
		Bucket: aws.String(bucket),
		Prefix: aws.String(key),
	})
	if err != nil {
		t.Fatalf("ListObjectVersions: %v", err)
	}
	if len(listed.Versions) < 1 {
		t.Fatalf("expected object version, got %+v", listed.Versions)
	}
	if len(listed.DeleteMarkers) < 1 {
		t.Fatalf("expected delete marker, got %+v", listed.DeleteMarkers)
	}
}