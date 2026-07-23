package store

import (
	"bytes"
	"compress/gzip"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

const servicePrincipalLogs = "logs.amazonaws.com"

const logsSubscriptionSchema = `
CREATE TABLE IF NOT EXISTS logs_subscription_filters (
  account_id TEXT NOT NULL,
  log_group_name TEXT NOT NULL,
  filter_name TEXT NOT NULL,
  filter_pattern TEXT NOT NULL DEFAULT '',
  destination_arn TEXT NOT NULL,
  role_arn TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, log_group_name, filter_name)
);
`

// LogsSubscriptionFilter is a lab PutSubscriptionFilter row.
type LogsSubscriptionFilter struct {
	LogGroupName   string
	FilterName     string
	FilterPattern  string
	DestinationARN string
	RoleARN        string
	CreatedAt      int64
}

// EnsureLogsSubscriptionSchema creates subscription filter tables.
func EnsureLogsSubscriptionSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure logs subscription schema: db is nil")
	}
	if _, err := db.Exec(logsSubscriptionSchema); err != nil {
		return fmt.Errorf("ensure logs subscription schema: %w", err)
	}
	return nil
}

// EnsureLogsSubscriptionSchema ensures subscription tables on an open store.
func (s *Store) EnsureLogsSubscriptionSchema() error {
	return EnsureLogsSubscriptionSchema(s.db)
}

// PutSubscriptionFilter upserts a subscription filter (Lambda or SQS destination).
// Lambda destinations ignore roleARN (resource-policy delivery path). SQS is lab-only.
func (s *Store) PutSubscriptionFilter(accountID, group, filterName, pattern, destinationARN, roleARN string) (LogsSubscriptionFilter, error) {
	group = strings.TrimSpace(group)
	filterName = strings.TrimSpace(filterName)
	destinationARN = strings.TrimSpace(destinationARN)
	roleARN = strings.TrimSpace(roleARN)
	if group == "" || filterName == "" || destinationARN == "" {
		return LogsSubscriptionFilter{}, fmt.Errorf("put subscription filter: LogGroupName, FilterName, and DestinationArn are required")
	}
	if _, err := s.getLogGroup(accountID, group); err != nil {
		return LogsSubscriptionFilter{}, err
	}
	if !strings.HasPrefix(destinationARN, "arn:aws:lambda:") && !strings.HasPrefix(destinationARN, "arn:aws:sqs:") {
		return LogsSubscriptionFilter{}, fmt.Errorf("put subscription filter: DestinationArn must be Lambda or SQS")
	}
	// AWS Lambda subscription filters use destination resource policy, not roleArn.
	if strings.HasPrefix(destinationARN, "arn:aws:lambda:") {
		roleARN = ""
	}
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO logs_subscription_filters
		 (account_id, log_group_name, filter_name, filter_pattern, destination_arn, role_arn, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(account_id, log_group_name, filter_name) DO UPDATE SET
		   filter_pattern = excluded.filter_pattern,
		   destination_arn = excluded.destination_arn,
		   role_arn = excluded.role_arn`,
		accountID, group, filterName, strings.TrimSpace(pattern), destinationARN, roleARN, now,
	)
	if err != nil {
		return LogsSubscriptionFilter{}, fmt.Errorf("put subscription filter: %w", err)
	}
	return LogsSubscriptionFilter{
		LogGroupName: group, FilterName: filterName, FilterPattern: pattern,
		DestinationARN: destinationARN, RoleARN: roleARN, CreatedAt: now,
	}, nil
}

// DeleteSubscriptionFilter removes a filter.
func (s *Store) DeleteSubscriptionFilter(accountID, group, filterName string) error {
	res, err := s.db.Exec(
		`DELETE FROM logs_subscription_filters WHERE account_id = ? AND log_group_name = ? AND filter_name = ?`,
		accountID, strings.TrimSpace(group), strings.TrimSpace(filterName),
	)
	if err != nil {
		return fmt.Errorf("delete subscription filter: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrLogGroupNotFound
	}
	return nil
}

// DescribeSubscriptionFilters lists filters for a log group.
func (s *Store) DescribeSubscriptionFilters(accountID, group string) ([]LogsSubscriptionFilter, error) {
	if _, err := s.getLogGroup(accountID, group); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT log_group_name, filter_name, filter_pattern, destination_arn, role_arn, created_at
		 FROM logs_subscription_filters WHERE account_id = ? AND log_group_name = ? ORDER BY filter_name`,
		accountID, strings.TrimSpace(group),
	)
	if err != nil {
		return nil, fmt.Errorf("describe subscription filters: %w", err)
	}
	defer rows.Close()
	var out []LogsSubscriptionFilter
	for rows.Next() {
		var f LogsSubscriptionFilter
		if err := rows.Scan(&f.LogGroupName, &f.FilterName, &f.FilterPattern, &f.DestinationARN, &f.RoleARN, &f.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *Store) fanOutLogSubscriptionFilters(accountID, group string, events []LogEvent) {
	filters, err := s.DescribeSubscriptionFilters(accountID, group)
	if err != nil || len(filters) == 0 {
		return
	}
	for _, f := range filters {
		matched := make([]LogEvent, 0, len(events))
		for _, ev := range events {
			if labLogFilterMatches(f.FilterPattern, ev.Message) {
				matched = append(matched, ev)
			}
		}
		if len(matched) == 0 {
			continue
		}
		if err := s.deliverLogSubscription(accountID, group, f, matched); err != nil {
			log.Printf("logs subscription delivery failed group=%s filter=%s err=%v", group, f.FilterName, err)
		}
	}
}

func labLogFilterMatches(pattern, message string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return true
	}
	// Lab lite: substring match (not CloudWatch Logs filter syntax).
	return strings.Contains(message, pattern)
}

func (s *Store) deliverLogSubscription(accountID, group string, f LogsSubscriptionFilter, events []LogEvent) error {
	lg, err := s.getLogGroup(accountID, group)
	if err != nil {
		return err
	}
	sourceARN := strings.TrimSpace(lg.Arn)
	payload, err := json.Marshal(map[string]any{
		"messageType":         "DATA_MESSAGE",
		"owner":               accountID,
		"logGroup":            group,
		"subscriptionFilters": []string{f.FilterName},
		"logEvents":           events,
	})
	if err != nil {
		return err
	}
	arn := strings.TrimSpace(f.DestinationARN)
	action, ok := deliveryActionForARN(arn)
	if !ok {
		return fmt.Errorf("unsupported destination")
	}
	destOwner := resourceOwnerAccountFromARN(arn)
	if destOwner == "" {
		destOwner = accountID
	}
	roleARN := strings.TrimSpace(f.RoleARN)
	isLambda := strings.HasPrefix(arn, "arn:aws:lambda:")

	// Destination resource policy always required (Lambda path; SQS with or without role).
	if !s.deliveryTargetResourcePolicyAllows(accountID, arn, action, servicePrincipalLogs, sourceARN) {
		return fmt.Errorf("destination policy denied")
	}
	// When roleArn is set (lab SQS path), also require role session Allow (AND).
	if roleARN != "" && !isLambda {
		if !s.deliveryRoleSessionAllows(accountID, roleARN, action, arn, "logs-subscription", DefaultEventsRegion, sourceARN) {
			return fmt.Errorf("role session denied")
		}
	}

	switch {
	case strings.HasPrefix(arn, "arn:aws:sqs:"):
		qName, err := queueNameFromARN(arn)
		if err != nil {
			return err
		}
		// SQS destinations are lab-only: raw DATA_MESSAGE JSON (not awslogs envelope).
		_, err = s.SendMessage(destOwner, qName, payload, false, nil, "", nil)
		return err
	case isLambda:
		fn, qual := ParseFunctionQualifier(arn)
		envelope, err := encodeAwslogsSubscriptionEnvelope(payload)
		if err != nil {
			return err
		}
		_, err = s.EnqueueAsyncInvoke(destOwner, fn, qual, string(envelope))
		return err
	default:
		return fmt.Errorf("unsupported destination")
	}
}

// EncodeAwslogsSubscriptionEnvelope wraps DATA_MESSAGE JSON as AWS Lambda subscription
// shape: {"awslogs":{"data":"<base64(gzip(json))>"}}.
func EncodeAwslogsSubscriptionEnvelope(dataMessageJSON []byte) ([]byte, error) {
	return encodeAwslogsSubscriptionEnvelope(dataMessageJSON)
}

func encodeAwslogsSubscriptionEnvelope(dataMessageJSON []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(dataMessageJSON); err != nil {
		_ = zw.Close()
		return nil, fmt.Errorf("awslogs gzip: %w", err)
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("awslogs gzip close: %w", err)
	}
	return json.Marshal(map[string]any{
		"awslogs": map[string]string{
			"data": base64.StdEncoding.EncodeToString(buf.Bytes()),
		},
	})
}

// DecodeAwslogsSubscriptionEnvelope extracts and gunzips awslogs.data for tests.
func DecodeAwslogsSubscriptionEnvelope(envelope []byte) ([]byte, error) {
	var wrap struct {
		Awslogs struct {
			Data string `json:"data"`
		} `json:"awslogs"`
	}
	if err := json.Unmarshal(envelope, &wrap); err != nil {
		return nil, err
	}
	if wrap.Awslogs.Data == "" {
		return nil, fmt.Errorf("awslogs.data missing")
	}
	raw, err := base64.StdEncoding.DecodeString(wrap.Awslogs.Data)
	if err != nil {
		return nil, err
	}
	zr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	var out bytes.Buffer
	if _, err := out.ReadFrom(zr); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
