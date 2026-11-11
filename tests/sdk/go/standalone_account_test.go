package sdk_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func TestStandaloneAccountSDK(t *testing.T) {
	requireReady(t)
	rootCfg := loadAWSConfig(t)
	ctx := context.Background()

	name := "sdk-go-" + uniquePrefix(t)
	status, body := postStandaloneSeed(t, name)
	if status == http.StatusNotFound {
		t.Skip("NOCTAXRIS_LAB_STANDALONE is off")
	}
	if status != http.StatusOK {
		t.Fatalf("seed standalone status=%d body=%s", status, body)
	}
	seeded := decodeStandaloneSeed(t, body)

	againStatus, againBody := postStandaloneSeed(t, name)
	if againStatus != http.StatusOK {
		t.Fatalf("reseed standalone status=%d body=%s", againStatus, againBody)
	}
	again := decodeStandaloneSeed(t, againBody)
	if again.AccessKeyID != seeded.AccessKeyID {
		t.Fatalf("reseed accessKeyId=%s want %s", again.AccessKeyID, seeded.AccessKeyID)
	}

	homeCfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(rootCfg.Region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(seeded.AccessKeyID, seeded.SecretAccessKey, "")),
	)
	if err != nil {
		t.Fatalf("load home config: %v", err)
	}
	homeCfg.BaseEndpoint = aws.String(endpoint())

	homeS3 := newS3(t, homeCfg)
	rootS3 := newS3(t, rootCfg)
	prefix := uniquePrefix(t)
	homeBucket := prefix + "-home"
	labBucket := prefix + "-lab"
	labKey := "note.txt"
	labBody := []byte("noctaxris-lab-object")

	_, err = homeS3.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(homeBucket)})
	if err != nil {
		t.Fatalf("CreateBucket home: %v", err)
	}
	t.Cleanup(func() {
		_, _ = homeS3.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: aws.String(homeBucket)})
	})

	if bucketListed(t, rootS3, homeBucket) {
		t.Fatalf("root ListBuckets contains %s", homeBucket)
	}
	if !bucketListed(t, homeS3, homeBucket) {
		t.Fatalf("home ListBuckets missing %s", homeBucket)
	}

	_, err = rootS3.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(labBucket)})
	if err != nil {
		t.Fatalf("CreateBucket lab: %v", err)
	}
	t.Cleanup(func() {
		_, _ = rootS3.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(labBucket), Key: aws.String(labKey)})
		_, _ = rootS3.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: aws.String(labBucket)})
	})
	_, err = rootS3.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(labBucket),
		Key:    aws.String(labKey),
		Body:   bytes.NewReader(labBody),
	})
	if err != nil {
		t.Fatalf("PutObject lab: %v", err)
	}

	got, err := homeS3.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(labBucket),
		Key:    aws.String(labKey),
	})
	if err == nil {
		read, readErr := io.ReadAll(got.Body)
		_ = got.Body.Close()
		if readErr != nil {
			t.Fatalf("read lab object: %v", readErr)
		}
		if bytes.Equal(read, labBody) {
			t.Fatal("home key returned lab object bytes")
		}
		t.Fatal("home key GetObject succeeded")
	}
}

type standaloneSeed struct {
	AccountID       string `json:"accountId"`
	AccessKeyID     string `json:"accessKeyId"`
	SecretAccessKey string `json:"secretAccessKey"`
	UserARN         string `json:"userArn"`
}

func postStandaloneSeed(t *testing.T, name string) (int, []byte) {
	t.Helper()
	payload, err := json.Marshal(struct {
		Name string `json:"name"`
	}{Name: name})
	if err != nil {
		t.Fatalf("marshal seed body: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, endpoint()+"/_noctaxris/lab/accounts:seedStandalone", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("seed request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	sum := sha256.Sum256(payload)
	accessKey := envOr("AWS_ACCESS_KEY_ID", "AKIAROOTEXAMPLE01")
	secretKey := envOr("AWS_SECRET_ACCESS_KEY", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")
	region := envOr("AWS_DEFAULT_REGION", "us-east-1")
	// The seed path keeps a colon. The server canonical URI leaves that colon
	// unescaped, so the signer must not encode the path a second time.
	signer := v4.NewSigner(func(o *v4.SignerOptions) {
		o.DisableURIPathEscaping = true
	})
	err = signer.SignHTTP(context.Background(), aws.Credentials{
		AccessKeyID:     accessKey,
		SecretAccessKey: secretKey,
	}, req, hex.EncodeToString(sum[:]), "iam", region, time.Now())
	if err != nil {
		t.Fatalf("sign seed request: %v", err)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("seed request: %v", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read seed response: %v", err)
	}
	return resp.StatusCode, respBody
}

func decodeStandaloneSeed(t *testing.T, body []byte) standaloneSeed {
	t.Helper()
	var seeded standaloneSeed
	if err := json.Unmarshal(body, &seeded); err != nil {
		t.Fatalf("decode seed response: %v", err)
	}
	if seeded.AccountID == "" || seeded.AccessKeyID == "" || seeded.SecretAccessKey == "" || seeded.UserARN == "" {
		t.Fatal("seed response missing accountId, accessKeyId, secretAccessKey, or userArn")
	}
	return seeded
}

func bucketListed(t *testing.T, client *s3.Client, name string) bool {
	t.Helper()
	out, err := client.ListBuckets(context.Background(), &s3.ListBucketsInput{})
	if err != nil {
		t.Fatalf("ListBuckets: %v", err)
	}
	for _, b := range out.Buckets {
		if aws.ToString(b.Name) == name {
			return true
		}
	}
	return false
}
