package store

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// CloudFrontLoggingConfig is the lab standard logging destination for a distribution.
type CloudFrontLoggingConfig struct {
	Enabled bool
	Bucket  string
	Prefix  string
}

// ParseCloudFrontLoggingBucket extracts a lab bucket name from Logging.Bucket values.
func ParseCloudFrontLoggingBucket(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "s3://")
	if raw == "" {
		return ""
	}
	if idx := strings.Index(raw, "/"); idx >= 0 {
		raw = raw[:idx]
	}
	raw = strings.TrimSuffix(raw, ".s3.amazonaws.com")
	raw = strings.TrimSuffix(raw, ".s3.amazonaws.com.cn")
	return strings.TrimSpace(raw)
}

// SetCloudFrontDistributionLogging updates logging on an existing distribution.
func (s *Store) SetCloudFrontDistributionLogging(accountID, distributionID string, cfg CloudFrontLoggingConfig) error {
	distributionID = strings.TrimSpace(distributionID)
	if cfg.Enabled {
		cfg.Bucket = ParseCloudFrontLoggingBucket(cfg.Bucket)
		if cfg.Bucket == "" {
			return fmt.Errorf("%w: Logging.Bucket required when logging is enabled", ErrCloudFrontBadRequest)
		}
		if _, err := s.GetBucket(accountID, cfg.Bucket); err != nil {
			if errors.Is(err, ErrNoSuchBucket) {
				return fmt.Errorf("%w: S3 bucket %q does not exist", ErrCloudFrontBadRequest, cfg.Bucket)
			}
			return fmt.Errorf("set cloudfront logging: get bucket: %w", err)
		}
	}
	enabledInt := 0
	if cfg.Enabled {
		enabledInt = 1
	}
	res, err := s.db.Exec(
		`UPDATE cloudfront_distributions SET logging_enabled = ?, logging_bucket = ?, logging_prefix = ?
		 WHERE account_id = ? AND id = ?`,
		enabledInt, strings.TrimSpace(cfg.Bucket), strings.TrimSpace(cfg.Prefix), accountID, distributionID,
	)
	if err != nil {
		return fmt.Errorf("set cloudfront logging: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrCloudFrontNotFound
	}
	return nil
}

// CloudFrontAccessLogObjectKey returns a hive-layout key for appended CloudFront access log lines.
func CloudFrontAccessLogObjectKey(accountID, distributionID, keyPrefix string, ts time.Time) string {
	prefix := normalizeAccessLogS3KeyPrefix(keyPrefix)
	ts = ts.UTC()
	yyyy := ts.Format("2006")
	mm := ts.Format("01")
	dd := ts.Format("02")
	filename := fmt.Sprintf("%s_cloudfront_%s.log", strings.TrimSpace(accountID), strings.TrimSpace(distributionID))
	return fmt.Sprintf("%sAWSLogs/%s/CloudFront/%s/%s/%s/%s",
		prefix, accountID, yyyy, mm, dd, filename)
}

// CloudFrontAccessLogInput captures one edge GET for log emission.
type CloudFrontAccessLogInput struct {
	ClientIP      string
	Method        string
	Host          string
	URIStem       string
	Status        int
	UserAgent     string
	BytesSent     int64
	BytesReceived int64
	TimeTaken     float64
	RequestTime   time.Time
	RequestID     string
}

// FormatCloudFrontAccessLogLine builds a tab-separated CloudFront standard log line (lite).
func FormatCloudFrontAccessLogLine(in CloudFrontAccessLogInput) string {
	if in.RequestTime.IsZero() {
		in.RequestTime = time.Now().UTC()
	}
	ts := in.RequestTime.UTC()
	date := ts.Format("2006-01-02")
	timeStr := ts.Format("15:04:05")
	if in.Method == "" {
		in.Method = "GET"
	}
	if in.URIStem == "" {
		in.URIStem = "/"
	}
	if in.RequestID == "" {
		in.RequestID = "-"
	}
	fields := []string{
		date,
		timeStr,
		"LAB",
		strconv.FormatInt(in.BytesSent, 10),
		in.ClientIP,
		in.Method,
		in.Host,
		in.URIStem,
		strconv.Itoa(in.Status),
		"-",
		in.UserAgent,
		"-",
		"-",
		"Hit",
		in.RequestID,
		in.Host,
		"http",
		strconv.FormatInt(in.BytesReceived, 10),
		strconv.FormatFloat(in.TimeTaken, 'f', 3, 64),
		"-",
		"-",
		"-",
		"Hit",
	}
	return strings.Join(fields, "\t")
}

// AppendCloudFrontAccessLog appends one access log line when distribution logging is enabled.
func (s *Store) AppendCloudFrontAccessLog(accountID string, d CloudFrontDistribution, in CloudFrontAccessLogInput) error {
	if !d.LoggingEnabled {
		return nil
	}
	bucket := ParseCloudFrontLoggingBucket(d.LoggingBucket)
	if bucket == "" {
		return fmt.Errorf("%w: logging enabled without bucket", ErrCloudFrontBadRequest)
	}
	if _, err := s.GetBucket(accountID, bucket); err != nil {
		return err
	}
	if in.RequestTime.IsZero() {
		in.RequestTime = time.Now().UTC()
	}
	key := CloudFrontAccessLogObjectKey(accountID, d.ID, d.LoggingPrefix, in.RequestTime)
	line := FormatCloudFrontAccessLogLine(in)
	return s.appendS3TextLine(accountID, bucket, key, []byte(line))
}
