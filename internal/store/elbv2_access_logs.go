package store

import (
	"database/sql"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// ELBv2 access log attribute keys (Application Load Balancer).
const (
	ELBv2AttrAccessLogsS3Enabled = "access_logs.s3.enabled"
	ELBv2AttrAccessLogsS3Bucket  = "access_logs.s3.bucket"
	ELBv2AttrAccessLogsS3Prefix  = "access_logs.s3.prefix"
)

// ELBv2AccessLogsConfig is the lab access log destination for a load balancer.
type ELBv2AccessLogsConfig struct {
	Enabled bool
	Bucket  string
	Prefix  string
}

// ELBv2AccessLogInput captures one lab /alb/ request for log emission.
type ELBv2AccessLogInput struct {
	Region           string
	ClientIP         string
	ClientPort       int
	RequestMethod    string
	RequestPath      string
	HTTPVersion      string
	UserAgent        string
	TargetGroupARN   string
	ElbStatusCode    int
	TargetStatusCode int
	ReceivedBytes    int64
	SentBytes        int64
	RequestTime      time.Time
}

// ELBv2AccessLogsConfigForARN returns access log settings for a load balancer ARN.
func (s *Store) ELBv2AccessLogsConfigForARN(accountID, lbARN string) (ELBv2AccessLogsConfig, error) {
	lbARN = strings.TrimSpace(lbARN)
	var enabled int
	var bucket, prefix string
	err := s.db.QueryRow(
		`SELECT access_logs_enabled, access_logs_s3_bucket, access_logs_s3_prefix
		 FROM elbv2_load_balancers WHERE account_id = ? AND arn = ?`,
		accountID, lbARN,
	).Scan(&enabled, &bucket, &prefix)
	if errors.Is(err, sql.ErrNoRows) {
		return ELBv2AccessLogsConfig{}, ErrELBv2NotFound
	}
	if err != nil {
		return ELBv2AccessLogsConfig{}, fmt.Errorf("access logs config: %w", err)
	}
	return ELBv2AccessLogsConfig{
		Enabled: enabled != 0,
		Bucket:  strings.TrimSpace(bucket),
		Prefix:  strings.TrimSpace(prefix),
	}, nil
}

// SetELBv2LoadBalancerAttributes applies lite load balancer attributes (access logs).
func (s *Store) SetELBv2LoadBalancerAttributes(accountID, lbARN string, attrs map[string]string) error {
	if len(attrs) == 0 {
		return fmt.Errorf("%w: Attributes required", ErrELBv2BadRequest)
	}
	lbARN = strings.TrimSpace(lbARN)
	var name string
	err := s.db.QueryRow(
		`SELECT name FROM elbv2_load_balancers WHERE account_id = ? AND arn = ?`,
		accountID, lbARN,
	).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrELBv2NotFound
	}
	if err != nil {
		return fmt.Errorf("set lb attributes: %w", err)
	}
	_ = name

	cur, err := s.ELBv2AccessLogsConfigForARN(accountID, lbARN)
	if err != nil {
		return err
	}
	next := cur
	for k, v := range attrs {
		switch strings.TrimSpace(k) {
		case ELBv2AttrAccessLogsS3Enabled:
			switch strings.ToLower(strings.TrimSpace(v)) {
			case "true", "1":
				next.Enabled = true
			case "false", "0", "":
				next.Enabled = false
			default:
				return fmt.Errorf("%w: %s must be true or false", ErrELBv2BadRequest, ELBv2AttrAccessLogsS3Enabled)
			}
		case ELBv2AttrAccessLogsS3Bucket:
			next.Bucket = strings.TrimSpace(v)
		case ELBv2AttrAccessLogsS3Prefix:
			next.Prefix = strings.TrimSpace(v)
		}
	}
	if next.Enabled {
		if next.Bucket == "" {
			return fmt.Errorf("%w: %s required when access logs are enabled", ErrELBv2BadRequest, ELBv2AttrAccessLogsS3Bucket)
		}
		if _, err := s.GetBucket(accountID, next.Bucket); err != nil {
			if errors.Is(err, ErrNoSuchBucket) {
				return fmt.Errorf("%w: S3 bucket %q does not exist", ErrELBv2BadRequest, next.Bucket)
			}
			return fmt.Errorf("set lb attributes: get bucket: %w", err)
		}
	}
	enabledInt := 0
	if next.Enabled {
		enabledInt = 1
	}
	res, err := s.db.Exec(
		`UPDATE elbv2_load_balancers SET access_logs_enabled = ?, access_logs_s3_bucket = ?, access_logs_s3_prefix = ?
		 WHERE account_id = ? AND arn = ?`,
		enabledInt, next.Bucket, next.Prefix, accountID, lbARN,
	)
	if err != nil {
		return fmt.Errorf("set lb attributes: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrELBv2NotFound
	}
	return nil
}

// DescribeELBv2LoadBalancerAttributes returns standard access log attribute keys.
func (s *Store) DescribeELBv2LoadBalancerAttributes(accountID, lbARN string) ([]map[string]string, error) {
	cfg, err := s.ELBv2AccessLogsConfigForARN(accountID, lbARN)
	if err != nil {
		return nil, err
	}
	enabled := "false"
	if cfg.Enabled {
		enabled = "true"
	}
	return []map[string]string{
		{"Key": ELBv2AttrAccessLogsS3Enabled, "Value": enabled},
		{"Key": ELBv2AttrAccessLogsS3Bucket, "Value": cfg.Bucket},
		{"Key": ELBv2AttrAccessLogsS3Prefix, "Value": cfg.Prefix},
	}, nil
}

// ELBv2AccessLogObjectKey returns a hive-layout object key for appended ALB access log lines.
// Shape: {prefix}AWSLogs/{account}/elasticloadbalancing/{region}/{yyyy}/{mm}/{dd}/{account}_elasticloadbalancing_{region}_{lbName}.log
func ELBv2AccessLogObjectKey(accountID, region, lbName, keyPrefix string, ts time.Time) string {
	if region == "" {
		region = DefaultELBv2Region
	}
	accountID = strings.TrimSpace(accountID)
	lbName = strings.TrimSpace(lbName)
	prefix := normalizeAccessLogS3KeyPrefix(keyPrefix)
	ts = ts.UTC()
	yyyy := ts.Format("2006")
	mm := ts.Format("01")
	dd := ts.Format("02")
	filename := fmt.Sprintf("%s_elasticloadbalancing_%s_%s.log", accountID, region, lbName)
	return fmt.Sprintf("%sAWSLogs/%s/elasticloadbalancing/%s/%s/%s/%s/%s",
		prefix, accountID, region, yyyy, mm, dd, filename)
}

func normalizeAccessLogS3KeyPrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	prefix = strings.TrimPrefix(prefix, "/")
	if prefix == "" {
		return ""
	}
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return prefix
}

// FormatELBv2AccessLogLine builds a space-separated ALB access log line (lite field set).
func FormatELBv2AccessLogLine(lb ELBv2LoadBalancer, in ELBv2AccessLogInput) string {
	if in.RequestTime.IsZero() {
		in.RequestTime = time.Now().UTC()
	}
	if in.Region == "" {
		in.Region = DefaultELBv2Region
	}
	if in.HTTPVersion == "" {
		in.HTTPVersion = "HTTP/1.1"
	}
	client := formatHostPort(in.ClientIP, in.ClientPort)
	request := fmt.Sprintf("%s %s %s", strings.ToUpper(in.RequestMethod), in.RequestPath, in.HTTPVersion)
	request = quoteALBLogField(request)
	ua := quoteALBLogField(in.UserAgent)
	elbStatus := strconv.Itoa(in.ElbStatusCode)
	targetStatus := strconv.Itoa(in.TargetStatusCode)
	if in.TargetStatusCode <= 0 {
		targetStatus = "-"
	}
	fields := []string{
		"http",
		in.RequestTime.UTC().Format("2006-01-02T15:04:05.000000Z"),
		lb.ARN,
		client,
		"-",
		"0.000",
		"0.000",
		"0.000",
		elbStatus,
		targetStatus,
		strconv.FormatInt(in.ReceivedBytes, 10),
		strconv.FormatInt(in.SentBytes, 10),
		request,
		ua,
		"-",
		"-",
		in.TargetGroupARN,
	}
	return strings.Join(fields, " ")
}

func formatHostPort(host string, port int) string {
	host = strings.TrimSpace(host)
	if host == "" {
		host = "-"
	}
	if port <= 0 {
		if h, p, err := net.SplitHostPort(host); err == nil {
			return net.JoinHostPort(h, p)
		}
		return host + ":0"
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}

func quoteALBLogField(v string) string {
	v = strings.ReplaceAll(v, `"`, `""`)
	return `"` + v + `"`
}

// AppendELBv2AccessLog appends one access log line when logging is enabled for the load balancer.
func (s *Store) AppendELBv2AccessLog(accountID string, lb ELBv2LoadBalancer, in ELBv2AccessLogInput) error {
	cfg, err := s.ELBv2AccessLogsConfigForARN(accountID, lb.ARN)
	if err != nil {
		return err
	}
	if !cfg.Enabled {
		return nil
	}
	if cfg.Bucket == "" {
		return fmt.Errorf("%w: access logs enabled without bucket", ErrELBv2BadRequest)
	}
	if _, err := s.GetBucket(accountID, cfg.Bucket); err != nil {
		return err
	}
	if in.RequestTime.IsZero() {
		in.RequestTime = time.Now().UTC()
	}
	key := ELBv2AccessLogObjectKey(accountID, in.Region, lb.Name, cfg.Prefix, in.RequestTime)
	line := FormatELBv2AccessLogLine(lb, in)
	return s.appendS3TextLine(accountID, cfg.Bucket, key, []byte(line))
}

func (s *Store) appendS3TextLine(accountID, bucket, key string, line []byte) error {
	var existing []byte
	if meta, data, err := s.GetObject(accountID, bucket, key); err == nil {
		existing = data
		_ = meta
	} else if !errors.Is(err, ErrNoSuchKey) {
		return err
	}
	var buf []byte
	if len(existing) > 0 {
		buf = append(buf, existing...)
		if existing[len(existing)-1] != '\n' {
			buf = append(buf, '\n')
		}
	}
	buf = append(buf, line...)
	buf = append(buf, '\n')
	_, err := s.PutObject(accountID, bucket, key, PutObjectMeta{
		Data:        buf,
		PlainSize:   int64(len(buf)),
		ContentType: "text/plain",
	})
	return err
}
