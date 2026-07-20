package store

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/google/uuid"
)

var (
	ErrFirehoseExists   = errors.New("ResourceInUseException")
	ErrFirehoseNotFound = errors.New("ResourceNotFoundException")
	ErrFirehoseBadReq   = errors.New("InvalidArgumentException")
)

const DefaultFirehoseRegion = "us-east-1"

const firehoseSchema = `
CREATE TABLE IF NOT EXISTS firehose_streams (
  account_id TEXT NOT NULL,
  name TEXT NOT NULL,
  stream_arn TEXT NOT NULL,
  dest_type TEXT NOT NULL,
  dest_bucket TEXT NOT NULL DEFAULT '',
  dest_prefix TEXT NOT NULL DEFAULT '',
  dest_lambda_arn TEXT NOT NULL DEFAULT '',
  role_arn TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, name)
);
CREATE TABLE IF NOT EXISTS firehose_records (
  account_id TEXT NOT NULL,
  stream_name TEXT NOT NULL,
  record_id TEXT NOT NULL,
  data BLOB NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (record_id)
);
CREATE INDEX IF NOT EXISTS idx_fh_records ON firehose_records(account_id, stream_name);
`

// FirehoseStream is a delivery stream row.
type FirehoseStream struct {
	Name          string
	StreamARN     string
	DestType      string // S3 or Lambda
	DestBucket    string
	DestPrefix    string
	DestLambdaARN string
	RoleARN       string
	CreatedAt     int64
}

// EnsureFirehoseSchema creates Firehose tables if missing.
func EnsureFirehoseSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure firehose schema: db is nil")
	}
	if _, err := db.Exec(firehoseSchema); err != nil {
		return fmt.Errorf("ensure firehose schema: %w", err)
	}
	return nil
}

// EnsureFirehoseSchema ensures Firehose tables on an open store.
func (s *Store) EnsureFirehoseSchema() error {
	return EnsureFirehoseSchema(s.db)
}

// FirehoseStreamARN builds arn:aws:firehose:REGION:ACCOUNT:deliverystream/NAME
func FirehoseStreamARN(region, accountID, name string) string {
	if region == "" {
		region = DefaultFirehoseRegion
	}
	return fmt.Sprintf("arn:aws:firehose:%s:%s:deliverystream/%s", region, accountID, name)
}

// CreateFirehoseStream creates a delivery stream with S3 and/or Lambda destination.
func (s *Store) CreateFirehoseStream(accountID, region, name, roleARN, destType, bucket, prefix, lambdaARN string) (FirehoseStream, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return FirehoseStream{}, fmt.Errorf("%w: DeliveryStreamName required", ErrFirehoseBadReq)
	}
	destType = strings.TrimSpace(destType)
	switch strings.ToUpper(destType) {
	case "S3", "EXTENDED_S3", "":
		destType = "S3"
		bucket = strings.TrimSpace(bucket)
		if bucket == "" {
			return FirehoseStream{}, fmt.Errorf("%w: S3 BucketARN/Bucket required", ErrFirehoseBadReq)
		}
		if strings.HasPrefix(bucket, "arn:") {
			parts := strings.Split(bucket, ":::")
			if len(parts) == 2 {
				bucket = parts[1]
			}
		}
	case "LAMBDA":
		destType = "Lambda"
		lambdaARN = strings.TrimSpace(lambdaARN)
		if lambdaARN == "" {
			return FirehoseStream{}, fmt.Errorf("%w: Lambda ARN required", ErrFirehoseBadReq)
		}
	default:
		return FirehoseStream{}, fmt.Errorf("%w: unsupported destination type %q", ErrFirehoseBadReq, destType)
	}
	arn := FirehoseStreamARN(region, accountID, name)
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO firehose_streams (account_id, name, stream_arn, dest_type, dest_bucket, dest_prefix, dest_lambda_arn, role_arn, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		accountID, name, arn, destType, bucket, prefix, lambdaARN, roleARN, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "constraint") {
			return FirehoseStream{}, ErrFirehoseExists
		}
		return FirehoseStream{}, fmt.Errorf("create delivery stream: %w", err)
	}
	return FirehoseStream{
		Name: name, StreamARN: arn, DestType: destType, DestBucket: bucket,
		DestPrefix: prefix, DestLambdaARN: lambdaARN, RoleARN: roleARN, CreatedAt: now,
	}, nil
}

// GetFirehoseStream returns a delivery stream.
func (s *Store) GetFirehoseStream(accountID, name string) (FirehoseStream, error) {
	var st FirehoseStream
	err := s.db.QueryRow(
		`SELECT name, stream_arn, dest_type, dest_bucket, dest_prefix, dest_lambda_arn, role_arn, created_at
		 FROM firehose_streams WHERE account_id = ? AND name = ?`,
		accountID, name,
	).Scan(&st.Name, &st.StreamARN, &st.DestType, &st.DestBucket, &st.DestPrefix, &st.DestLambdaARN, &st.RoleARN, &st.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return FirehoseStream{}, ErrFirehoseNotFound
	}
	if err != nil {
		return FirehoseStream{}, fmt.Errorf("get delivery stream: %w", err)
	}
	return st, nil
}

// ListFirehoseStreams lists delivery stream names.
func (s *Store) ListFirehoseStreams(accountID string) ([]FirehoseStream, error) {
	rows, err := s.db.Query(
		`SELECT name, stream_arn, dest_type, dest_bucket, dest_prefix, dest_lambda_arn, role_arn, created_at
		 FROM firehose_streams WHERE account_id = ? ORDER BY created_at`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list delivery streams: %w", err)
	}
	defer rows.Close()
	var out []FirehoseStream
	for rows.Next() {
		var st FirehoseStream
		if err := rows.Scan(&st.Name, &st.StreamARN, &st.DestType, &st.DestBucket, &st.DestPrefix, &st.DestLambdaARN, &st.RoleARN, &st.CreatedAt); err != nil {
			return nil, fmt.Errorf("list delivery streams scan: %w", err)
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// DeleteFirehoseStream deletes a delivery stream.
func (s *Store) DeleteFirehoseStream(accountID, name string) error {
	res, err := s.db.Exec(`DELETE FROM firehose_streams WHERE account_id = ? AND name = ?`, accountID, name)
	if err != nil {
		return fmt.Errorf("delete delivery stream: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrFirehoseNotFound
	}
	_, _ = s.db.Exec(`DELETE FROM firehose_records WHERE account_id = ? AND stream_name = ?`, accountID, name)
	return nil
}

// PutFirehoseRecord delivers one record to S3 and/or enqueues a Lambda async invoke.
// Delivery requires RoleARN session Allow (like Scheduler) or a destination resource
// policy Allow for firehose.amazonaws.com when RoleARN is omitted.
func (s *Store) PutFirehoseRecord(accountID, streamName string, data []byte) (recordID string, err error) {
	st, err := s.GetFirehoseStream(accountID, streamName)
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "", fmt.Errorf("%w: Data required", ErrFirehoseBadReq)
	}
	if !s.firehoseDeliveryAuthorized(accountID, st) {
		return "", fmt.Errorf("%w: delivery not authorized for destination", ErrFirehoseBadReq)
	}
	recordID = uuid.NewString()
	now := time.Now().UTC().UnixMilli()
	_, err = s.db.Exec(
		`INSERT INTO firehose_records (account_id, stream_name, record_id, data, created_at) VALUES (?, ?, ?, ?, ?)`,
		accountID, streamName, recordID, data, now,
	)
	if err != nil {
		return "", fmt.Errorf("put record insert: %w", err)
	}
	if st.DestType == "S3" && st.DestBucket != "" {
		prefix := st.DestPrefix
		if prefix != "" && !strings.HasSuffix(prefix, "/") {
			prefix += "/"
		}
		key := fmt.Sprintf("%s%s", prefix, recordID)
		_, err = s.PutObject(accountID, st.DestBucket, key, PutObjectMeta{
			Data: data, PlainSize: int64(len(data)), ContentType: "application/octet-stream",
		})
		if err != nil {
			return "", fmt.Errorf("put record s3: %w", err)
		}
	}
	if st.DestType == "Lambda" && strings.TrimSpace(st.DestLambdaARN) != "" {
		if err := s.deliverFirehoseToLambda(accountID, st, recordID, data, now); err != nil {
			return "", err
		}
	}
	return recordID, nil
}

func (s *Store) firehoseDeliveryAuthorized(accountID string, st FirehoseStream) bool {
	targetARN, action, ok := firehoseDestinationTarget(st)
	if !ok {
		return false
	}
	roleARN := strings.TrimSpace(st.RoleARN)
	if roleARN == "" {
		return s.deliveryTargetResourcePolicyAllows(accountID, targetARN, action, authz.ServicePrincipalFirehose)
	}
	return s.deliveryRoleSessionAllows(accountID, roleARN, action, targetARN, "firehose-delivery", DefaultFirehoseRegion)
}

func firehoseDestinationTarget(st FirehoseStream) (targetARN, action string, ok bool) {
	switch st.DestType {
	case "S3":
		bucket := strings.TrimSpace(st.DestBucket)
		if bucket == "" {
			return "", "", false
		}
		return BucketARN(bucket), actionS3PutObject, true
	case "Lambda":
		arn := strings.TrimSpace(st.DestLambdaARN)
		if arn == "" {
			return "", "", false
		}
		return arn, actionLambdaInvokeFunction, true
	default:
		return "", "", false
	}
}

func (s *Store) deliverFirehoseToLambda(accountID string, st FirehoseStream, recordID string, data []byte, arrivalMs int64) error {
	functionName, qualifier := ParseFunctionQualifier(st.DestLambdaARN)
	if functionName == "" {
		return fmt.Errorf("%w: Lambda destination function is empty", ErrFirehoseBadReq)
	}
	if _, _, err := s.ResolveFunction(accountID, functionName, qualifier); err != nil {
		return fmt.Errorf("put record lambda: %w", err)
	}
	eventJSON, err := firehoseLambdaEventJSON(st.StreamARN, recordID, data, arrivalMs)
	if err != nil {
		return fmt.Errorf("put record lambda event: %w", err)
	}
	if _, err := s.EnqueueAsyncInvoke(accountID, functionName, qualifier, eventJSON); err != nil {
		return fmt.Errorf("put record lambda invoke: %w", err)
	}
	return nil
}

func firehoseLambdaEventJSON(streamARN, recordID string, data []byte, arrivalMs int64) (string, error) {
	payload := map[string]any{
		"deliveryStreamArn":            streamARN,
		"recordId":                     recordID,
		"approximateArrivalTimestamp":  arrivalMs,
		"data":                         base64.StdEncoding.EncodeToString(data),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// PutFirehoseRecordBatch puts multiple records. Returns failed indexes.
func (s *Store) PutFirehoseRecordBatch(accountID, streamName string, records [][]byte) (failed []int, err error) {
	if _, err := s.GetFirehoseStream(accountID, streamName); err != nil {
		return nil, err
	}
	for i, data := range records {
		if _, err := s.PutFirehoseRecord(accountID, streamName, data); err != nil {
			failed = append(failed, i)
		}
	}
	return failed, nil
}

// DecodeFirehoseData decodes base64 Firehose Data fields.
func DecodeFirehoseData(v any) ([]byte, error) {
	switch t := v.(type) {
	case string:
		return base64.StdEncoding.DecodeString(t)
	case []byte:
		return t, nil
	default:
		return nil, fmt.Errorf("data must be base64 string")
	}
}
