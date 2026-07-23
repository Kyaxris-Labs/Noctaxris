package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var (
	ErrLogGroupAlreadyExists  = errors.New("ResourceAlreadyExistsException")
	ErrLogStreamAlreadyExists = errors.New("ResourceAlreadyExistsException")
	ErrLogGroupNotFound       = errors.New("ResourceNotFoundException")
	ErrLogStreamNotFound      = errors.New("ResourceNotFoundException")
	ErrInvalidSequenceToken   = errors.New("InvalidSequenceTokenException")
)

// LogGroup is a CloudWatch Logs log group row.
type LogGroup struct {
	LogGroupName      string
	Arn               string
	CreationTime      int64 // epoch millis
	StoredBytes       int64
	MetricFilterCount int
	// RetentionInDays is 0 when unset (never expire). AWS-shaped values otherwise.
	RetentionInDays int
}

// LogStream is a CloudWatch Logs log stream row.
type LogStream struct {
	LogGroupName        string
	LogStreamName       string
	Arn                 string
	CreationTime        int64
	FirstEventTimestamp int64
	LastEventTimestamp  int64
	LastIngestionTime   int64
	UploadSequenceToken string
	StoredBytes         int64
}

// LogEvent is one ingested log event.
type LogEvent struct {
	Timestamp     int64
	Message       string
	IngestionTime int64
	EventID       string
}

const logsSchema = `
CREATE TABLE IF NOT EXISTS logs_groups (
  account_id TEXT NOT NULL,
  log_group_name TEXT NOT NULL,
  arn TEXT NOT NULL,
  creation_time INTEGER NOT NULL,
  PRIMARY KEY (account_id, log_group_name)
);
CREATE TABLE IF NOT EXISTS logs_streams (
  account_id TEXT NOT NULL,
  log_group_name TEXT NOT NULL,
  log_stream_name TEXT NOT NULL,
  arn TEXT NOT NULL,
  creation_time INTEGER NOT NULL,
  first_event_timestamp INTEGER NOT NULL DEFAULT 0,
  last_event_timestamp INTEGER NOT NULL DEFAULT 0,
  last_ingestion_time INTEGER NOT NULL DEFAULT 0,
  upload_sequence_token TEXT NOT NULL DEFAULT '',
  stored_bytes INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (account_id, log_group_name, log_stream_name)
);
CREATE TABLE IF NOT EXISTS logs_events (
  account_id TEXT NOT NULL,
  log_group_name TEXT NOT NULL,
  log_stream_name TEXT NOT NULL,
  event_id TEXT NOT NULL,
  timestamp INTEGER NOT NULL,
  ingestion_time INTEGER NOT NULL,
  message TEXT NOT NULL,
  PRIMARY KEY (account_id, log_group_name, log_stream_name, event_id)
);
CREATE INDEX IF NOT EXISTS idx_logs_events_ts ON logs_events(account_id, log_group_name, log_stream_name, timestamp);
`

// EnsureLogsSchema creates CloudWatch Logs tables if missing.
func EnsureLogsSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure logs schema: db is nil")
	}
	if _, err := db.Exec(logsSchema); err != nil {
		return fmt.Errorf("ensure logs schema: %w", err)
	}
	if _, err := db.Exec(`ALTER TABLE logs_groups ADD COLUMN retention_in_days INTEGER NOT NULL DEFAULT 0`); err != nil {
		if !isDuplicateColumnErr(err) {
			return fmt.Errorf("ensure logs schema: retention column: %w", err)
		}
	}
	return nil
}

var allowedLogRetentionDays = map[int]struct{}{
	1: {}, 3: {}, 5: {}, 7: {}, 14: {}, 30: {}, 60: {}, 90: {}, 120: {}, 150: {}, 180: {},
	365: {}, 400: {}, 545: {}, 731: {}, 1096: {}, 1827: {}, 2192: {}, 2557: {}, 2922: {}, 3288: {}, 3653: {},
}

// PutRetentionPolicy sets RetentionInDays on a log group (AWS-allowed values only).
func (s *Store) PutRetentionPolicy(accountID, name string, retentionInDays int) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("put retention policy: name is required")
	}
	if _, err := s.getLogGroup(accountID, name); err != nil {
		return err
	}
	if _, ok := allowedLogRetentionDays[retentionInDays]; !ok {
		return fmt.Errorf("InvalidParameterException: RetentionInDays must be a valid CloudWatch Logs retention value")
	}
	_, err := s.db.Exec(
		`UPDATE logs_groups SET retention_in_days = ? WHERE account_id = ? AND log_group_name = ?`,
		retentionInDays, accountID, name,
	)
	if err != nil {
		return fmt.Errorf("put retention policy: %w", err)
	}
	_ = s.purgeExpiredLogEvents(accountID, name, retentionInDays)
	return nil
}

// DeleteRetentionPolicy clears retention so events never expire.
func (s *Store) DeleteRetentionPolicy(accountID, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("delete retention policy: name is required")
	}
	if _, err := s.getLogGroup(accountID, name); err != nil {
		return err
	}
	_, err := s.db.Exec(
		`UPDATE logs_groups SET retention_in_days = 0 WHERE account_id = ? AND log_group_name = ?`,
		accountID, name,
	)
	if err != nil {
		return fmt.Errorf("delete retention policy: %w", err)
	}
	return nil
}

func (s *Store) purgeExpiredLogEvents(accountID, group string, retentionInDays int) error {
	if retentionInDays <= 0 {
		return nil
	}
	cutoff := time.Now().UTC().Add(-time.Duration(retentionInDays) * 24 * time.Hour).UnixMilli()
	_, err := s.db.Exec(
		`DELETE FROM logs_events WHERE account_id = ? AND log_group_name = ? AND timestamp < ?`,
		accountID, group, cutoff,
	)
	if err != nil {
		return fmt.Errorf("purge expired log events: %w", err)
	}
	return nil
}

// EnsureLogsSchema ensures logs tables on an open store.
func (s *Store) EnsureLogsSchema() error {
	return EnsureLogsSchema(s.db)
}

// LogGroupARN builds arn:aws:logs:REGION:ACCOUNT:log-group:NAME
func LogGroupARN(region, accountID, name string) string {
	if region == "" {
		region = DefaultSSMRegion
	}
	return fmt.Sprintf("arn:aws:logs:%s:%s:log-group:%s", region, accountID, name)
}

// LogStreamARN builds arn:aws:logs:REGION:ACCOUNT:log-group:GROUP:log-stream:STREAM
func LogStreamARN(region, accountID, group, stream string) string {
	if region == "" {
		region = DefaultSSMRegion
	}
	return fmt.Sprintf("arn:aws:logs:%s:%s:log-group:%s:log-stream:%s", region, accountID, group, stream)
}

// CreateLogGroup creates a log group.
func (s *Store) CreateLogGroup(accountID, region, name string) (LogGroup, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return LogGroup{}, fmt.Errorf("create log group: name is required")
	}
	if _, err := s.getLogGroup(accountID, name); err == nil {
		return LogGroup{}, ErrLogGroupAlreadyExists
	} else if !errors.Is(err, ErrLogGroupNotFound) {
		return LogGroup{}, err
	}
	now := time.Now().UTC().UnixMilli()
	arn := LogGroupARN(region, accountID, name)
	_, err := s.db.Exec(
		`INSERT INTO logs_groups (account_id, log_group_name, arn, creation_time) VALUES (?, ?, ?, ?)`,
		accountID, name, arn, now,
	)
	if err != nil {
		return LogGroup{}, fmt.Errorf("create log group: %w", err)
	}
	return LogGroup{LogGroupName: name, Arn: arn, CreationTime: now}, nil
}

// CreateLogStream creates a log stream under an existing group.
func (s *Store) CreateLogStream(accountID, region, group, stream string) (LogStream, error) {
	group = strings.TrimSpace(group)
	stream = strings.TrimSpace(stream)
	if group == "" || stream == "" {
		return LogStream{}, fmt.Errorf("create log stream: group and stream names are required")
	}
	if _, err := s.getLogGroup(accountID, group); err != nil {
		return LogStream{}, err
	}
	if _, err := s.getLogStream(accountID, group, stream); err == nil {
		return LogStream{}, ErrLogStreamAlreadyExists
	} else if !errors.Is(err, ErrLogStreamNotFound) {
		return LogStream{}, err
	}
	now := time.Now().UTC().UnixMilli()
	arn := LogStreamARN(region, accountID, group, stream)
	_, err := s.db.Exec(
		`INSERT INTO logs_streams (
			account_id, log_group_name, log_stream_name, arn, creation_time,
			first_event_timestamp, last_event_timestamp, last_ingestion_time, upload_sequence_token, stored_bytes
		) VALUES (?, ?, ?, ?, ?, 0, 0, 0, '', 0)`,
		accountID, group, stream, arn, now,
	)
	if err != nil {
		return LogStream{}, fmt.Errorf("create log stream: %w", err)
	}
	return LogStream{
		LogGroupName: group, LogStreamName: stream, Arn: arn, CreationTime: now,
	}, nil
}

// DeleteLogGroup deletes a log group and all streams and events under it.
func (s *Store) DeleteLogGroup(accountID, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("delete log group: name is required")
	}
	if _, err := s.getLogGroup(accountID, name); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("delete log group: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(
		`DELETE FROM logs_events WHERE account_id = ? AND log_group_name = ?`,
		accountID, name,
	); err != nil {
		return fmt.Errorf("delete log group: events: %w", err)
	}
	if _, err := tx.Exec(
		`DELETE FROM logs_streams WHERE account_id = ? AND log_group_name = ?`,
		accountID, name,
	); err != nil {
		return fmt.Errorf("delete log group: streams: %w", err)
	}
	if _, err := tx.Exec(
		`DELETE FROM logs_groups WHERE account_id = ? AND log_group_name = ?`,
		accountID, name,
	); err != nil {
		return fmt.Errorf("delete log group: group: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("delete log group: commit: %w", err)
	}
	return nil
}

// DeleteLogStream deletes a log stream and its events.
func (s *Store) DeleteLogStream(accountID, group, stream string) error {
	group = strings.TrimSpace(group)
	stream = strings.TrimSpace(stream)
	if group == "" || stream == "" {
		return fmt.Errorf("delete log stream: group and stream names are required")
	}
	if _, err := s.getLogStream(accountID, group, stream); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("delete log stream: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(
		`DELETE FROM logs_events WHERE account_id = ? AND log_group_name = ? AND log_stream_name = ?`,
		accountID, group, stream,
	); err != nil {
		return fmt.Errorf("delete log stream: events: %w", err)
	}
	if _, err := tx.Exec(
		`DELETE FROM logs_streams WHERE account_id = ? AND log_group_name = ? AND log_stream_name = ?`,
		accountID, group, stream,
	); err != nil {
		return fmt.Errorf("delete log stream: stream: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("delete log stream: commit: %w", err)
	}
	return nil
}

// DescribeLogStreams lists streams under a group, optional stream name prefix.
func (s *Store) DescribeLogStreams(accountID, group, prefix string) ([]LogStream, error) {
	group = strings.TrimSpace(group)
	if group == "" {
		return nil, fmt.Errorf("describe log streams: group name is required")
	}
	if _, err := s.getLogGroup(accountID, group); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT log_group_name, log_stream_name, arn, creation_time,
			first_event_timestamp, last_event_timestamp, last_ingestion_time,
			upload_sequence_token, stored_bytes
		 FROM logs_streams WHERE account_id = ? AND log_group_name = ?
		 ORDER BY log_stream_name`,
		accountID, group,
	)
	if err != nil {
		return nil, fmt.Errorf("describe log streams: %w", err)
	}
	defer rows.Close()

	var out []LogStream
	for rows.Next() {
		var st LogStream
		if err := rows.Scan(
			&st.LogGroupName, &st.LogStreamName, &st.Arn, &st.CreationTime,
			&st.FirstEventTimestamp, &st.LastEventTimestamp, &st.LastIngestionTime,
			&st.UploadSequenceToken, &st.StoredBytes,
		); err != nil {
			return nil, fmt.Errorf("describe log streams: scan: %w", err)
		}
		if prefix != "" && !strings.HasPrefix(st.LogStreamName, prefix) {
			continue
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// DescribeLogGroups lists log groups, optional prefix filter.
func (s *Store) DescribeLogGroups(accountID, prefix string) ([]LogGroup, error) {
	rows, err := s.db.Query(
		`SELECT log_group_name, arn, creation_time, COALESCE(retention_in_days, 0)
		 FROM logs_groups WHERE account_id = ? ORDER BY log_group_name`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("describe log groups: %w", err)
	}
	var out []LogGroup
	for rows.Next() {
		var g LogGroup
		if err := rows.Scan(&g.LogGroupName, &g.Arn, &g.CreationTime, &g.RetentionInDays); err != nil {
			rows.Close()
			return nil, fmt.Errorf("describe log groups: scan: %w", err)
		}
		if prefix != "" && !strings.HasPrefix(g.LogGroupName, prefix) {
			continue
		}
		out = append(out, g)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("describe log groups: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("describe log groups: %w", err)
	}
	// Sum bytes after closing the listing cursor so nested queries cannot deadlock
	// a single-connection pool.
	for i := range out {
		if out[i].RetentionInDays > 0 {
			_ = s.purgeExpiredLogEvents(accountID, out[i].LogGroupName, out[i].RetentionInDays)
		}
		bytes, err := s.sumLogGroupBytes(accountID, out[i].LogGroupName)
		if err != nil {
			return nil, err
		}
		out[i].StoredBytes = bytes
		n, err := s.countMetricFilters(accountID, out[i].LogGroupName)
		if err != nil {
			return nil, err
		}
		out[i].MetricFilterCount = n
	}
	return out, nil
}

// PutLogEvents appends events and returns the next sequence token.
// sequenceToken may be empty on the first put for a stream.
func (s *Store) PutLogEvents(
	accountID, group, stream, sequenceToken string, events []LogEvent,
) (nextToken string, rejected []LogEvent, err error) {
	if g, gerr := s.getLogGroup(accountID, group); gerr == nil && g.RetentionInDays > 0 {
		_ = s.purgeExpiredLogEvents(accountID, group, g.RetentionInDays)
	}
	st, err := s.getLogStream(accountID, group, stream)
	if err != nil {
		return "", nil, err
	}
	if st.UploadSequenceToken != "" && sequenceToken != st.UploadSequenceToken {
		return st.UploadSequenceToken, nil, ErrInvalidSequenceToken
	}
	if len(events) == 0 {
		return st.UploadSequenceToken, nil, nil
	}

	now := time.Now().UTC().UnixMilli()
	tx, err := s.db.Begin()
	if err != nil {
		return "", nil, fmt.Errorf("put log events: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	first := st.FirstEventTimestamp
	last := st.LastEventTimestamp
	stored := st.StoredBytes
	for i, ev := range events {
		ts := ev.Timestamp
		if ts == 0 {
			ts = now
		}
		msg := ev.Message
		id := ev.EventID
		if id == "" {
			id = fmt.Sprintf("%d-%d", ts, i)
		}
		_, err := tx.Exec(
			`INSERT INTO logs_events (
				account_id, log_group_name, log_stream_name, event_id, timestamp, ingestion_time, message
			) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			accountID, group, stream, id, ts, now, msg,
		)
		if err != nil {
			return "", nil, fmt.Errorf("put log events: insert: %w", err)
		}
		stored += int64(len(msg))
		if first == 0 || ts < first {
			first = ts
		}
		if ts > last {
			last = ts
		}
	}

	next := strconv.FormatInt(now, 10) + "-" + strconv.Itoa(len(events))
	_, err = tx.Exec(
		`UPDATE logs_streams SET
			first_event_timestamp = ?, last_event_timestamp = ?, last_ingestion_time = ?,
			upload_sequence_token = ?, stored_bytes = ?
		 WHERE account_id = ? AND log_group_name = ? AND log_stream_name = ?`,
		first, last, now, next, stored, accountID, group, stream,
	)
	if err != nil {
		return "", nil, fmt.Errorf("put log events: update stream: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", nil, fmt.Errorf("put log events: commit: %w", err)
	}
	s.fanOutLogSubscriptionFilters(accountID, group, events)
	s.fanOutLogMetricFilters(accountID, group, events)
	return next, nil, nil
}

// GetLogEvents returns events for a stream, optionally bounded by start/end millis.
func (s *Store) GetLogEvents(
	accountID, group, stream string, startTime, endTime int64, startFromHead bool, limit int,
) ([]LogEvent, error) {
	if g, err := s.getLogGroup(accountID, group); err == nil && g.RetentionInDays > 0 {
		_ = s.purgeExpiredLogEvents(accountID, group, g.RetentionInDays)
	}
	if _, err := s.getLogStream(accountID, group, stream); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 10000
	}
	order := "ASC"
	if !startFromHead {
		order = "DESC"
	}
	q := fmt.Sprintf(
		`SELECT event_id, timestamp, ingestion_time, message FROM logs_events
		 WHERE account_id = ? AND log_group_name = ? AND log_stream_name = ?`,
	)
	args := []any{accountID, group, stream}
	if startTime > 0 {
		q += ` AND timestamp >= ?`
		args = append(args, startTime)
	}
	if endTime > 0 {
		q += ` AND timestamp <= ?`
		args = append(args, endTime)
	}
	q += ` ORDER BY timestamp ` + order + ` LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("get log events: %w", err)
	}
	defer rows.Close()

	var out []LogEvent
	for rows.Next() {
		var ev LogEvent
		if err := rows.Scan(&ev.EventID, &ev.Timestamp, &ev.IngestionTime, &ev.Message); err != nil {
			return nil, fmt.Errorf("get log events: scan: %w", err)
		}
		out = append(out, ev)
	}
	if !startFromHead {
		// AWS returns chronological when startFromHead=false after reverse fetch; keep DESC as lab lite.
	}
	return out, rows.Err()
}

func (s *Store) getLogGroup(accountID, name string) (LogGroup, error) {
	var g LogGroup
	err := s.db.QueryRow(
		`SELECT log_group_name, arn, creation_time, COALESCE(retention_in_days, 0)
		 FROM logs_groups WHERE account_id = ? AND log_group_name = ?`,
		accountID, name,
	).Scan(&g.LogGroupName, &g.Arn, &g.CreationTime, &g.RetentionInDays)
	if errors.Is(err, sql.ErrNoRows) {
		return LogGroup{}, ErrLogGroupNotFound
	}
	if err != nil {
		return LogGroup{}, fmt.Errorf("get log group: %w", err)
	}
	return g, nil
}

func (s *Store) getLogStream(accountID, group, stream string) (LogStream, error) {
	var st LogStream
	err := s.db.QueryRow(
		`SELECT log_group_name, log_stream_name, arn, creation_time,
			first_event_timestamp, last_event_timestamp, last_ingestion_time,
			upload_sequence_token, stored_bytes
		 FROM logs_streams WHERE account_id = ? AND log_group_name = ? AND log_stream_name = ?`,
		accountID, group, stream,
	).Scan(
		&st.LogGroupName, &st.LogStreamName, &st.Arn, &st.CreationTime,
		&st.FirstEventTimestamp, &st.LastEventTimestamp, &st.LastIngestionTime,
		&st.UploadSequenceToken, &st.StoredBytes,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return LogStream{}, ErrLogStreamNotFound
	}
	if err != nil {
		return LogStream{}, fmt.Errorf("get log stream: %w", err)
	}
	return st, nil
}

func (s *Store) sumLogGroupBytes(accountID, group string) (int64, error) {
	var n sql.NullInt64
	err := s.db.QueryRow(
		`SELECT COALESCE(SUM(stored_bytes), 0) FROM logs_streams WHERE account_id = ? AND log_group_name = ?`,
		accountID, group,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("sum log group bytes: %w", err)
	}
	return n.Int64, nil
}
