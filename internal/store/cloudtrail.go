package store

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const cloudtrailEventsFile = "events.jsonl"

const DefaultCloudTrailRegion = "us-east-1"

const cloudTrailDeliveryCap = 100

var (
	ErrCloudTrailBadRequest    = errors.New("InvalidTrailNameException")
	ErrCloudTrailNotFound      = errors.New("TrailNotFoundException")
	ErrCloudTrailAlreadyExists = errors.New("TrailAlreadyExistsException")
	ErrCloudTrailS3Bucket      = errors.New("S3BucketDoesNotExistException")
)

const cloudtrailTrailSchema = `
CREATE TABLE IF NOT EXISTS cloudtrail_trails (
  account_id TEXT NOT NULL,
  name TEXT NOT NULL,
  s3_bucket_name TEXT NOT NULL,
  s3_key_prefix TEXT NOT NULL DEFAULT '',
  cloudwatch_logs_log_group_arn TEXT NOT NULL DEFAULT '',
  cloudwatch_logs_role_arn TEXT NOT NULL DEFAULT '',
  is_logging INTEGER NOT NULL DEFAULT 0,
  home_region TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, name)
);
`

// CloudTrailTrail is a CreateTrail resource row.
type CloudTrailTrail struct {
	Name                      string
	S3BucketName              string
	S3KeyPrefix               string
	CloudWatchLogsLogGroupArn string
	CloudWatchLogsRoleArn     string
	IsLogging                 bool
	HomeRegion                string
}

// CloudTrailLookupFilter selects events from the local JSONL audit file.
type CloudTrailLookupFilter struct {
	StartTime *time.Time
	EndTime   *time.Time
	// AttributeKey/Value are the single LookupAttributes entry (AWS allows one).
	AttributeKey   string
	AttributeValue string
	MaxResults     int
}

// CloudTrailEventRecord is one LookupEvents Events[] member.
type CloudTrailEventRecord struct {
	EventId         string
	EventName       string
	EventSource     string
	EventTime       time.Time
	Username        string
	CloudTrailEvent string // raw JSON line
}

// EnsureCloudTrailSchema creates CloudTrail trail tables if missing.
func EnsureCloudTrailSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure cloudtrail schema: db is nil")
	}
	if _, err := db.Exec(cloudtrailTrailSchema); err != nil {
		return fmt.Errorf("ensure cloudtrail schema: %w", err)
	}
	return nil
}

// EnsureCloudTrailSchema ensures CloudTrail tables on an open store.
func (s *Store) EnsureCloudTrailSchema() error {
	return EnsureCloudTrailSchema(s.db)
}

// CloudTrailTrailARN builds arn:aws:cloudtrail:REGION:ACCOUNT:trail/NAME.
func CloudTrailTrailARN(region, accountID, name string) string {
	if region == "" {
		region = DefaultCloudTrailRegion
	}
	return fmt.Sprintf("arn:aws:cloudtrail:%s:%s:trail/%s", region, accountID, name)
}

// CloudTrailLabLogStreamName is the stream used for lab Logs delivery.
func CloudTrailLabLogStreamName(trailName string) string {
	return "noctaxris-trail-" + strings.TrimSpace(trailName)
}

func normalizeCloudTrailS3KeyPrefix(prefix string) string {
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

// CloudTrailDeliveryObjectKey returns the lab-shaped S3 key for a StartLogging snapshot.
func CloudTrailDeliveryObjectKey(accountID, keyPrefix, trailName string, tsMillis int64) string {
	prefix := normalizeCloudTrailS3KeyPrefix(keyPrefix)
	safeTrail := strings.ReplaceAll(strings.TrimSpace(trailName), "/", "-")
	return fmt.Sprintf("%sAWSLogs/%s/CloudTrail/noctaxris-%s-%d.json",
		prefix, accountID, safeTrail, tsMillis)
}

// CreateCloudTrailTrail creates a trail with IsLogging=false. The S3 bucket must exist.
func (s *Store) CreateCloudTrailTrail(accountID string, trail CloudTrailTrail) (CloudTrailTrail, error) {
	if err := s.EnsureCloudTrailSchema(); err != nil {
		return CloudTrailTrail{}, err
	}
	name := strings.TrimSpace(trail.Name)
	bucket := strings.TrimSpace(trail.S3BucketName)
	if name == "" {
		return CloudTrailTrail{}, fmt.Errorf("%w: Name required", ErrCloudTrailBadRequest)
	}
	if bucket == "" {
		return CloudTrailTrail{}, fmt.Errorf("%w: S3BucketName required", ErrCloudTrailBadRequest)
	}
	if _, err := s.GetBucket(accountID, bucket); err != nil {
		if errors.Is(err, ErrNoSuchBucket) {
			return CloudTrailTrail{}, fmt.Errorf("%w: S3 bucket %q does not exist", ErrCloudTrailS3Bucket, bucket)
		}
		return CloudTrailTrail{}, fmt.Errorf("create trail: get bucket: %w", err)
	}
	home := strings.TrimSpace(trail.HomeRegion)
	if home == "" {
		home = DefaultCloudTrailRegion
	}
	cwGroupARN := strings.TrimSpace(trail.CloudWatchLogsLogGroupArn)
	cwRoleARN := strings.TrimSpace(trail.CloudWatchLogsRoleArn)
	if cwRoleARN != "" && cwGroupARN == "" {
		return CloudTrailTrail{}, fmt.Errorf("%w: CloudWatchLogsLogGroupArn required when CloudWatchLogsRoleArn is set", ErrCloudTrailBadRequest)
	}
	if cwGroupARN != "" {
		group, err := parseLogGroupNameFromARN(cwGroupARN)
		if err != nil {
			return CloudTrailTrail{}, fmt.Errorf("%w: CloudWatchLogsLogGroupArn is invalid", ErrCloudTrailBadRequest)
		}
		if _, err := s.getLogGroup(accountID, group); err != nil {
			if errors.Is(err, ErrLogGroupNotFound) {
				return CloudTrailTrail{}, fmt.Errorf("%w: CloudWatch Logs log group does not exist", ErrCloudTrailBadRequest)
			}
			return CloudTrailTrail{}, fmt.Errorf("create trail: get log group: %w", err)
		}
	}
	prefix := strings.TrimSpace(trail.S3KeyPrefix)
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO cloudtrail_trails (
			account_id, name, s3_bucket_name, s3_key_prefix,
			cloudwatch_logs_log_group_arn, cloudwatch_logs_role_arn,
			is_logging, home_region, created_at
		) VALUES (?, ?, ?, ?, ?, ?, 0, ?, ?)`,
		accountID, name, bucket, prefix, cwGroupARN, cwRoleARN, home, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "constraint") {
			return CloudTrailTrail{}, ErrCloudTrailAlreadyExists
		}
		return CloudTrailTrail{}, fmt.Errorf("create trail: %w", err)
	}
	return CloudTrailTrail{
		Name:                      name,
		S3BucketName:              bucket,
		S3KeyPrefix:               prefix,
		CloudWatchLogsLogGroupArn: cwGroupARN,
		CloudWatchLogsRoleArn:     cwRoleARN,
		IsLogging:                 false,
		HomeRegion:                home,
	}, nil
}

// GetCloudTrailTrail returns a trail by name.
func (s *Store) GetCloudTrailTrail(accountID, name string) (CloudTrailTrail, error) {
	if err := s.EnsureCloudTrailSchema(); err != nil {
		return CloudTrailTrail{}, err
	}
	name = strings.TrimSpace(name)
	var t CloudTrailTrail
	var logging int
	err := s.db.QueryRow(
		`SELECT name, s3_bucket_name, s3_key_prefix, cloudwatch_logs_log_group_arn,
		        cloudwatch_logs_role_arn, is_logging, home_region
		 FROM cloudtrail_trails WHERE account_id = ? AND name = ?`,
		accountID, name,
	).Scan(
		&t.Name, &t.S3BucketName, &t.S3KeyPrefix, &t.CloudWatchLogsLogGroupArn,
		&t.CloudWatchLogsRoleArn, &logging, &t.HomeRegion,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return CloudTrailTrail{}, ErrCloudTrailNotFound
	}
	if err != nil {
		return CloudTrailTrail{}, fmt.Errorf("get trail: %w", err)
	}
	t.IsLogging = logging == 1
	return t, nil
}

// DescribeCloudTrailTrails lists trails, optionally filtered by name list.
func (s *Store) DescribeCloudTrailTrails(accountID string, names []string) ([]CloudTrailTrail, error) {
	if err := s.EnsureCloudTrailSchema(); err != nil {
		return nil, err
	}
	want := make(map[string]struct{}, len(names))
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if i := strings.LastIndex(n, "/"); i >= 0 && i+1 < len(n) && strings.Contains(n, ":trail/") {
			n = n[i+1:]
		}
		want[n] = struct{}{}
	}
	rows, err := s.db.Query(
		`SELECT name, s3_bucket_name, s3_key_prefix, cloudwatch_logs_log_group_arn,
		        cloudwatch_logs_role_arn, is_logging, home_region
		 FROM cloudtrail_trails WHERE account_id = ? ORDER BY name`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("describe trails: %w", err)
	}
	defer rows.Close()
	var out []CloudTrailTrail
	for rows.Next() {
		var t CloudTrailTrail
		var logging int
		if err := rows.Scan(
			&t.Name, &t.S3BucketName, &t.S3KeyPrefix, &t.CloudWatchLogsLogGroupArn,
			&t.CloudWatchLogsRoleArn, &logging, &t.HomeRegion,
		); err != nil {
			return nil, fmt.Errorf("describe trails scan: %w", err)
		}
		t.IsLogging = logging == 1
		if len(want) > 0 {
			if _, ok := want[t.Name]; !ok {
				continue
			}
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// DeleteCloudTrailTrail deletes a trail.
func (s *Store) DeleteCloudTrailTrail(accountID, name string) error {
	if err := s.EnsureCloudTrailSchema(); err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	if _, err := s.GetCloudTrailTrail(accountID, name); err != nil {
		return err
	}
	_, err := s.db.Exec(
		`DELETE FROM cloudtrail_trails WHERE account_id = ? AND name = ?`,
		accountID, name,
	)
	if err != nil {
		return fmt.Errorf("delete trail: %w", err)
	}
	return nil
}

// StartCloudTrailLogging delivers a lab JSONL snapshot to S3 (and optional Logs), then sets IsLogging=true.
// If any configured destination Put fails, IsLogging stays false.
func (s *Store) StartCloudTrailLogging(accountID, name, dataRoot string) error {
	if err := s.EnsureCloudTrailSchema(); err != nil {
		return err
	}
	trail, err := s.GetCloudTrailTrail(accountID, name)
	if err != nil {
		return err
	}
	if _, err := s.GetBucket(accountID, trail.S3BucketName); err != nil {
		if errors.Is(err, ErrNoSuchBucket) {
			return fmt.Errorf("%w: S3 bucket %q does not exist", ErrCloudTrailS3Bucket, trail.S3BucketName)
		}
		return fmt.Errorf("start logging: get bucket: %w", err)
	}
	lines, err := readCloudTrailJSONLTail(dataRoot, cloudTrailDeliveryCap)
	if err != nil {
		return fmt.Errorf("start logging: read events: %w", err)
	}
	ts := time.Now().UTC().UnixMilli()
	if err := s.putCloudTrailS3Delivery(accountID, trail, lines, ts); err != nil {
		return err
	}
	if strings.TrimSpace(trail.CloudWatchLogsLogGroupArn) != "" {
		if err := s.putCloudTrailLogsDelivery(accountID, trail, lines, ts); err != nil {
			return err
		}
	}
	_, err = s.db.Exec(
		`UPDATE cloudtrail_trails SET is_logging = 1 WHERE account_id = ? AND name = ?`,
		accountID, trail.Name,
	)
	if err != nil {
		return fmt.Errorf("start logging: set flag: %w", err)
	}
	return nil
}

// StopCloudTrailLogging sets IsLogging=false without delivery.
func (s *Store) StopCloudTrailLogging(accountID, name string) error {
	if err := s.EnsureCloudTrailSchema(); err != nil {
		return err
	}
	trail, err := s.GetCloudTrailTrail(accountID, name)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`UPDATE cloudtrail_trails SET is_logging = 0 WHERE account_id = ? AND name = ?`,
		accountID, trail.Name,
	)
	if err != nil {
		return fmt.Errorf("stop logging: %w", err)
	}
	return nil
}

func (s *Store) putCloudTrailS3Delivery(accountID string, trail CloudTrailTrail, lines []string, tsMillis int64) error {
	records := make([]json.RawMessage, 0, len(lines))
	for _, line := range lines {
		if !json.Valid([]byte(line)) {
			continue
		}
		records = append(records, json.RawMessage(line))
	}
	body, err := json.Marshal(map[string]any{"Records": records})
	if err != nil {
		return fmt.Errorf("start logging: marshal s3 body: %w", err)
	}
	key := CloudTrailDeliveryObjectKey(accountID, trail.S3KeyPrefix, trail.Name, tsMillis)
	if _, err := s.PutObject(accountID, trail.S3BucketName, key, PutObjectMeta{
		Data:        body,
		PlainSize:   int64(len(body)),
		ContentType: "application/json",
	}); err != nil {
		return fmt.Errorf("start logging: put s3 object: %w", err)
	}
	return nil
}

func (s *Store) putCloudTrailLogsDelivery(accountID string, trail CloudTrailTrail, lines []string, tsMillis int64) error {
	group, err := parseLogGroupNameFromARN(trail.CloudWatchLogsLogGroupArn)
	if err != nil {
		return fmt.Errorf("%w: CloudWatchLogsLogGroupArn is invalid", ErrCloudTrailBadRequest)
	}
	region := trail.HomeRegion
	if region == "" {
		region = DefaultCloudTrailRegion
	}
	if _, err := s.getLogGroup(accountID, group); err != nil {
		if errors.Is(err, ErrLogGroupNotFound) {
			return fmt.Errorf("%w: CloudWatch Logs log group does not exist", ErrCloudTrailBadRequest)
		}
		return fmt.Errorf("start logging: get log group: %w", err)
	}
	stream := CloudTrailLabLogStreamName(trail.Name)
	if _, err := s.getLogStream(accountID, group, stream); errors.Is(err, ErrLogStreamNotFound) {
		if _, err := s.CreateLogStream(accountID, region, group, stream); err != nil && !errors.Is(err, ErrLogStreamAlreadyExists) {
			return fmt.Errorf("start logging: create log stream: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("start logging: get log stream: %w", err)
	}
	st, err := s.getLogStream(accountID, group, stream)
	if err != nil {
		return fmt.Errorf("start logging: get log stream: %w", err)
	}
	events := make([]LogEvent, 0, len(lines))
	for i, line := range lines {
		events = append(events, LogEvent{
			Timestamp: tsMillis,
			Message:   line,
			EventID:   fmt.Sprintf("ct-%d-%d", tsMillis, i),
		})
	}
	if len(events) == 0 {
		events = append(events, LogEvent{
			Timestamp: tsMillis,
			Message:   `{"Records":[]}`,
			EventID:   fmt.Sprintf("ct-%d-empty", tsMillis),
		})
	}
	if _, _, err := s.PutLogEvents(accountID, group, stream, st.UploadSequenceToken, events); err != nil {
		return fmt.Errorf("start logging: put log events: %w", err)
	}
	return nil
}

func readCloudTrailJSONLTail(dataRoot string, max int) ([]string, error) {
	if max <= 0 {
		max = cloudTrailDeliveryCap
	}
	path := filepath.Join(dataRoot, "cloudtrail", cloudtrailEventsFile)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var all []string
	sc := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		all = append(all, line)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(all) > max {
		all = all[len(all)-max:]
	}
	return all, nil
}

// LookupCloudTrailEvents reads dataRoot/cloudtrail/events.jsonl and returns matching events
// newest-first (AWS LookupEvents order). Missing file yields an empty slice.
func LookupCloudTrailEvents(dataRoot string, filter CloudTrailLookupFilter) ([]CloudTrailEventRecord, error) {
	path := filepath.Join(dataRoot, "cloudtrail", cloudtrailEventsFile)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("lookup cloudtrail events: open: %w", err)
	}
	defer f.Close()

	max := filter.MaxResults
	if max <= 0 {
		max = 50
	}
	if max > 50 {
		max = 50
	}

	var matched []CloudTrailEventRecord
	sc := bufio.NewScanner(f)
	// Audit lines can be moderately large; raise buffer beyond default 64KiB.
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 1024*1024)

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		rec, ok, err := matchCloudTrailLine(line, filter)
		if err != nil {
			return nil, err
		}
		if ok {
			matched = append(matched, rec)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("lookup cloudtrail events: scan: %w", err)
	}

	sort.SliceStable(matched, func(i, j int) bool {
		return matched[i].EventTime.After(matched[j].EventTime)
	})
	if len(matched) > max {
		matched = matched[:max]
	}
	return matched, nil
}

func matchCloudTrailLine(line string, filter CloudTrailLookupFilter) (CloudTrailEventRecord, bool, error) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return CloudTrailEventRecord{}, false, fmt.Errorf("lookup cloudtrail events: unmarshal: %w", err)
	}

	eventTimeStr, _ := raw["eventTime"].(string)
	eventTime, err := time.Parse(time.RFC3339, eventTimeStr)
	if err != nil {
		// Soft-skip malformed timestamps rather than failing the whole lookup.
		eventTime = time.Time{}
	}
	if filter.StartTime != nil && !eventTime.IsZero() && eventTime.Before(*filter.StartTime) {
		return CloudTrailEventRecord{}, false, nil
	}
	if filter.EndTime != nil && !eventTime.IsZero() && eventTime.After(*filter.EndTime) {
		return CloudTrailEventRecord{}, false, nil
	}

	eventName, _ := raw["eventName"].(string)
	eventSource, _ := raw["eventSource"].(string)
	eventID, _ := raw["eventID"].(string)
	username := cloudTrailUsername(raw)
	accessKeyID := cloudTrailAccessKeyID(raw)
	readOnly := cloudTrailReadOnlyString(raw)

	if key := strings.TrimSpace(filter.AttributeKey); key != "" {
		want := filter.AttributeValue
		var got string
		switch strings.ToLower(key) {
		case "eventname":
			got = eventName
		case "username":
			got = username
		case "eventid":
			got = eventID
		case "eventsource":
			got = eventSource
		case "accesskeyid":
			got = accessKeyID
		case "readonly":
			got = readOnly
		default:
			return CloudTrailEventRecord{}, false, nil
		}
		if !strings.EqualFold(got, want) {
			return CloudTrailEventRecord{}, false, nil
		}
	}

	return CloudTrailEventRecord{
		EventId:         eventID,
		EventName:       eventName,
		EventSource:     eventSource,
		EventTime:       eventTime,
		Username:        username,
		CloudTrailEvent: line,
	}, true, nil
}

func cloudTrailUsername(raw map[string]any) string {
	ui, _ := raw["userIdentity"].(map[string]any)
	if ui == nil {
		return ""
	}
	if v, _ := ui["userName"].(string); v != "" {
		return v
	}
	if v, _ := ui["principalId"].(string); v != "" {
		return v
	}
	if arn, _ := ui["arn"].(string); arn != "" {
		if i := strings.LastIndex(arn, "/"); i >= 0 && i+1 < len(arn) {
			return arn[i+1:]
		}
	}
	if v, _ := ui["accessKeyId"].(string); v != "" {
		return v
	}
	return ""
}

func cloudTrailAccessKeyID(raw map[string]any) string {
	ui, _ := raw["userIdentity"].(map[string]any)
	if ui == nil {
		return ""
	}
	v, _ := ui["accessKeyId"].(string)
	return v
}

func cloudTrailReadOnlyString(raw map[string]any) string {
	switch v := raw["readOnly"].(type) {
	case bool:
		return strconv.FormatBool(v)
	case string:
		return v
	default:
		return ""
	}
}
