package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var ErrCloudTrailDigestMismatch = errors.New("CloudTrail log digest mismatch")

// CloudTrailDigestObjectKey maps a delivered log object key to its sibling digest key.
// AWSLogs/.../CloudTrail/.../{account}_CloudTrail_{region}_{stamp}_{id}.json[.gz]
// -> AWSLogs/.../CloudTrail-Digest/.../{account}_CloudTrail-Digest_{region}_{stamp}_{id}.json
func CloudTrailDigestObjectKey(logObjectKey string) string {
	key := strings.Replace(logObjectKey, "/CloudTrail/", "/CloudTrail-Digest/", 1)
	key = strings.Replace(key, "_CloudTrail_", "_CloudTrail-Digest_", 1)
	if strings.HasSuffix(key, ".json.gz") {
		key = strings.TrimSuffix(key, ".json.gz") + ".json"
	}
	return key
}

// CloudTrailDigestDocument is the lab-lite digest sidecar written after S3 log delivery.
type CloudTrailDigestDocument struct {
	HashAlgorithm string `json:"hashAlgorithm"`
	HashValue     string `json:"hashValue"`
	LogS3Bucket   string `json:"logS3Bucket"`
	LogS3Key      string `json:"logS3Key"`
	LogFileSize   int64  `json:"logFileSize"`
}

func cloudTrailSHA256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (s *Store) putCloudTrailDigestSidecar(accountID, bucket, logKey string, logBody []byte) error {
	digestKey := CloudTrailDigestObjectKey(logKey)
	doc := CloudTrailDigestDocument{
		HashAlgorithm: "SHA256",
		HashValue:     cloudTrailSHA256Hex(logBody),
		LogS3Bucket:   bucket,
		LogS3Key:      logKey,
		LogFileSize:   int64(len(logBody)),
	}
	body, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("cloudtrail digest: marshal: %w", err)
	}
	if _, err := s.PutObject(accountID, bucket, digestKey, PutObjectMeta{
		Data:        body,
		PlainSize:   int64(len(body)),
		ContentType: "application/json",
	}); err != nil {
		return fmt.Errorf("cloudtrail digest: put s3 object: %w", err)
	}
	return nil
}

// ValidateCloudTrailLogFile checks the log object bytes against the lab digest sidecar in the same bucket.
func (s *Store) ValidateCloudTrailLogFile(accountID, bucket, logKey string) error {
	bucket = strings.TrimSpace(bucket)
	logKey = strings.TrimSpace(logKey)
	if bucket == "" || logKey == "" {
		return fmt.Errorf("%w: bucket and key required", ErrCloudTrailBadRequest)
	}
	_, logBody, err := s.GetObject(accountID, bucket, logKey)
	if err != nil {
		return fmt.Errorf("validate cloudtrail log: get log object: %w", err)
	}
	digestKey := CloudTrailDigestObjectKey(logKey)
	_, digestBody, err := s.GetObject(accountID, bucket, digestKey)
	if err != nil {
		return fmt.Errorf("validate cloudtrail log: get digest object: %w", err)
	}
	var doc CloudTrailDigestDocument
	if err := json.Unmarshal(digestBody, &doc); err != nil {
		return fmt.Errorf("validate cloudtrail log: parse digest: %w", err)
	}
	if !strings.EqualFold(doc.HashAlgorithm, "SHA256") {
		return fmt.Errorf("%w: unsupported hash algorithm %q", ErrCloudTrailDigestMismatch, doc.HashAlgorithm)
	}
	got := cloudTrailSHA256Hex(logBody)
	if !strings.EqualFold(strings.TrimSpace(doc.HashValue), got) {
		return ErrCloudTrailDigestMismatch
	}
	if doc.LogS3Key != "" && doc.LogS3Key != logKey {
		return fmt.Errorf("%w: digest references key %q", ErrCloudTrailDigestMismatch, doc.LogS3Key)
	}
	return nil
}
