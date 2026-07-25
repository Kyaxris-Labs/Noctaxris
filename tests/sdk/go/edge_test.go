package sdk_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func TestCloudFrontDeployedAndEdgeGET(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	s3c := newS3(t, cfg)
	ctx := context.Background()
	prefix := uniquePrefix(t)
	bucket := strings.ToLower(prefix + "-cf-origin")
	key := "docs/hi.txt"
	payload := []byte("edge-object-bytes")

	if _, err := s3c.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)}); err != nil {
		t.Fatalf("CreateBucket: %v", err)
	}
	t.Cleanup(func() {
		_, _ = s3c.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
		_, _ = s3c.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: aws.String(bucket)})
	})
	if _, err := s3c.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket), Key: aws.String(key), Body: bytes.NewReader(payload),
		ContentType: aws.String("text/plain"),
	}); err != nil {
		t.Fatalf("PutObject: %v", err)
	}

	status, body, parsed := signedJSONTarget(t, "cloudfront", "CloudFront_2016_01_28.CreateDistribution", map[string]any{
		"DistributionConfig": map[string]any{
			"CallerReference": prefix + "-cf-ref",
			"Comment":         "lab",
			"Enabled":         true,
			"Origins": map[string]any{
				"Items": []map[string]any{
					{"Id": "o1", "DomainName": bucket, "OriginType": "s3"},
				},
			},
		},
	})
	if status != 200 {
		t.Fatalf("CreateDistribution status=%d body=%s", status, body)
	}
	dist, _ := parsed["Distribution"].(map[string]any)
	id, _ := dist["Id"].(string)
	if id == "" {
		t.Fatalf("missing Id: %s", body)
	}
	if dist["Status"] != "Deployed" {
		t.Fatalf("Status=%v want Deployed body=%s", dist["Status"], body)
	}
	domain, _ := dist["DomainName"].(string)
	if domain == "" || !strings.Contains(domain, "cloudfront.noctaxris.local") {
		t.Fatalf("DomainName=%q body=%s", domain, body)
	}

	edgeStatus, edgeBody := signedHTTP(t, "cloudfront", "GET", "/cloudfront/"+id+"/"+key, nil, "")
	if edgeStatus != 200 {
		t.Fatalf("edge GET status=%d body=%s", edgeStatus, edgeBody)
	}
	if !bytes.Equal(edgeBody, payload) {
		t.Fatalf("edge body=%q want=%q", edgeBody, payload)
	}
}

func TestTransferLabPutGetFile(t *testing.T) {
	requireReady(t)
	prefix := uniquePrefix(t)

	status, body, parsed := signedJSONTarget(t, "transfer", "TransferService.CreateServer", map[string]any{
		"Protocols": []string{"SFTP"},
	})
	if status != 200 {
		t.Fatalf("CreateServer status=%d body=%s", status, body)
	}
	serverID, _ := parsed["ServerId"].(string)
	if serverID == "" {
		t.Fatalf("missing ServerId: %s", body)
	}
	t.Cleanup(func() {
		_, _, _ = signedJSONTarget(t, "transfer", "TransferService.DeleteUser", map[string]any{
			"ServerId": serverID, "UserName": "alice",
		})
		_, _, _ = signedJSONTarget(t, "transfer", "TransferService.DeleteServer", map[string]any{
			"ServerId": serverID,
		})
	})

	descStatus, descBody, descParsed := signedJSONTarget(t, "transfer", "TransferService.DescribeServer", map[string]any{
		"ServerId": serverID,
	})
	if descStatus != 200 {
		t.Fatalf("DescribeServer status=%d body=%s", descStatus, descBody)
	}
	state := ""
	if srv, ok := descParsed["Server"].(map[string]any); ok {
		state, _ = srv["State"].(string)
	}
	if state == "" {
		state, _ = descParsed["State"].(string)
	}
	if state != "ONLINE" {
		t.Fatalf("State=%q want ONLINE body=%s", state, descBody)
	}

	userStatus, userBody, _ := signedJSONTarget(t, "transfer", "TransferService.CreateUser", map[string]any{
		"ServerId": serverID, "UserName": "alice",
	})
	if userStatus != 200 {
		t.Fatalf("CreateUser status=%d body=%s", userStatus, userBody)
	}

	rel := "inbox/" + prefix + ".txt"
	putStatus, putBody, _ := signedJSONTarget(t, "transfer", "TransferService.PutFile", map[string]any{
		"ServerId": serverID, "UserName": "alice", "Path": rel, "Body": "hello-transfer",
	})
	if putStatus != 200 {
		t.Fatalf("PutFile status=%d body=%s", putStatus, putBody)
	}
	getStatus, getBody, getParsed := signedJSONTarget(t, "transfer", "TransferService.GetFile", map[string]any{
		"ServerId": serverID, "UserName": "alice", "Path": rel,
	})
	if getStatus != 200 {
		t.Fatalf("GetFile status=%d body=%s", getStatus, getBody)
	}
	b64, _ := getParsed["BodyBase64"].(string)
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatalf("BodyBase64: %v", err)
	}
	if string(raw) != "hello-transfer" {
		t.Fatalf("GetFile body=%q", raw)
	}
}

func TestRoute53AliasCloudFront(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	s3c := newS3(t, cfg)
	ctx := context.Background()
	prefix := uniquePrefix(t)
	bucket := strings.ToLower(prefix + "-r53-cf")
	zoneName := strings.ReplaceAll(prefix, "_", "-") + ".example.com"

	if _, err := s3c.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)}); err != nil {
		t.Fatalf("CreateBucket: %v", err)
	}
	t.Cleanup(func() {
		_, _ = s3c.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: aws.String(bucket)})
	})

	cfStatus, cfBody, cfParsed := signedJSONTarget(t, "cloudfront", "CloudFront_2016_01_28.CreateDistribution", map[string]any{
		"DistributionConfig": map[string]any{
			"CallerReference": prefix + "-r53-cf",
			"Comment":         "lab",
			"Enabled":         true,
			"Origins": map[string]any{
				"Items": []map[string]any{
					{"Id": "o1", "DomainName": bucket, "OriginType": "s3"},
				},
			},
		},
	})
	if cfStatus != 200 {
		t.Fatalf("CreateDistribution status=%d body=%s", cfStatus, cfBody)
	}
	dist, _ := cfParsed["Distribution"].(map[string]any)
	domain, _ := dist["DomainName"].(string)
	if domain == "" {
		t.Fatalf("missing DomainName: %s", cfBody)
	}

	zoneStatus, zoneBody, zoneParsed := signedJSONTarget(t, "route53", "AWSRoute53.CreateHostedZone", map[string]any{
		"Name": zoneName, "CallerReference": prefix + "-hz",
	})
	if zoneStatus != 200 {
		t.Fatalf("CreateHostedZone status=%d body=%s", zoneStatus, zoneBody)
	}
	hz, _ := zoneParsed["HostedZone"].(map[string]any)
	zoneID, _ := hz["Id"].(string)
	zoneID = strings.TrimPrefix(zoneID, "/hostedzone/")
	if zoneID == "" {
		t.Fatalf("missing zone id: %s", zoneBody)
	}

	changeStatus, changeBody, _ := signedJSONTarget(t, "route53", "AWSRoute53.ChangeResourceRecordSets", map[string]any{
		"HostedZoneId": zoneID,
		"ChangeBatch": map[string]any{
			"Changes": []map[string]any{{
				"Action": "CREATE",
				"ResourceRecordSet": map[string]any{
					"Name": "www." + zoneName,
					"Type": "A",
					"AliasTarget": map[string]any{
						"DNSName": domain, "HostedZoneId": "Z2FDTNDATAQYW2", "EvaluateTargetHealth": false,
					},
				},
			}},
		},
	})
	if changeStatus != 200 {
		t.Fatalf("ChangeResourceRecordSets status=%d body=%s", changeStatus, changeBody)
	}

	listStatus, listBody, _ := signedJSONTarget(t, "route53", "AWSRoute53.ListResourceRecordSets", map[string]any{
		"HostedZoneId": zoneID,
	})
	if listStatus != 200 {
		t.Fatalf("ListResourceRecordSets status=%d body=%s", listStatus, listBody)
	}
	if !strings.Contains(string(listBody), "AliasTarget") ||
		!strings.Contains(strings.ToLower(string(listBody)), strings.ToLower(domain)) {
		t.Fatalf("list missing AliasTarget/domain: %s", listBody)
	}
}
