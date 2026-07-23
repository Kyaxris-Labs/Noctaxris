package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const logsMetricFilterSchema = `
CREATE TABLE IF NOT EXISTS logs_metric_filters (
  account_id TEXT NOT NULL,
  log_group_name TEXT NOT NULL,
  filter_name TEXT NOT NULL,
  filter_pattern TEXT NOT NULL DEFAULT '',
  metric_name TEXT NOT NULL,
  metric_namespace TEXT NOT NULL,
  metric_value TEXT NOT NULL DEFAULT '1',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, log_group_name, filter_name)
);
CREATE TABLE IF NOT EXISTS logs_metric_datapoints (
  account_id TEXT NOT NULL,
  namespace TEXT NOT NULL,
  metric_name TEXT NOT NULL,
  timestamp INTEGER NOT NULL,
  value REAL NOT NULL,
  id INTEGER PRIMARY KEY AUTOINCREMENT
);
CREATE INDEX IF NOT EXISTS idx_logs_metric_dp ON logs_metric_datapoints(account_id, namespace, metric_name, timestamp);
`

// LogsMetricFilter is a lab PutMetricFilter row.
type LogsMetricFilter struct {
	LogGroupName    string
	FilterName      string
	FilterPattern   string
	MetricName      string
	MetricNamespace string
	MetricValue     string
	CreatedAt       int64
}

// MetricDatapoint is a lab metric sample emitted by a metric filter.
type MetricDatapoint struct {
	Namespace  string
	MetricName string
	Timestamp  int64
	Value      float64
}

// EnsureLogsMetricFilterSchema creates metric filter tables.
func EnsureLogsMetricFilterSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure logs metric filter schema: db is nil")
	}
	if _, err := db.Exec(logsMetricFilterSchema); err != nil {
		return fmt.Errorf("ensure logs metric filter schema: %w", err)
	}
	return nil
}

// EnsureLogsMetricFilterSchema ensures metric filter tables on an open store.
func (s *Store) EnsureLogsMetricFilterSchema() error {
	return EnsureLogsMetricFilterSchema(s.db)
}

// PutMetricFilter upserts a metric filter transformation.
func (s *Store) PutMetricFilter(accountID, group, filterName, pattern, metricName, metricNamespace, metricValue string) (LogsMetricFilter, error) {
	group = strings.TrimSpace(group)
	filterName = strings.TrimSpace(filterName)
	metricName = strings.TrimSpace(metricName)
	metricNamespace = strings.TrimSpace(metricNamespace)
	if group == "" || filterName == "" || metricName == "" || metricNamespace == "" {
		return LogsMetricFilter{}, fmt.Errorf("put metric filter: LogGroupName, FilterName, MetricName, and MetricNamespace are required")
	}
	if _, err := s.getLogGroup(accountID, group); err != nil {
		return LogsMetricFilter{}, err
	}
	if strings.TrimSpace(metricValue) == "" {
		metricValue = "1"
	}
	pattern = strings.TrimSpace(pattern)
	if _, err := MatchLogFilterPattern(pattern, ""); err != nil {
		return LogsMetricFilter{}, err
	}
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO logs_metric_filters
		 (account_id, log_group_name, filter_name, filter_pattern, metric_name, metric_namespace, metric_value, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(account_id, log_group_name, filter_name) DO UPDATE SET
		   filter_pattern = excluded.filter_pattern,
		   metric_name = excluded.metric_name,
		   metric_namespace = excluded.metric_namespace,
		   metric_value = excluded.metric_value`,
		accountID, group, filterName, pattern, metricName, metricNamespace, metricValue, now,
	)
	if err != nil {
		return LogsMetricFilter{}, fmt.Errorf("put metric filter: %w", err)
	}
	return LogsMetricFilter{
		LogGroupName: group, FilterName: filterName, FilterPattern: pattern,
		MetricName: metricName, MetricNamespace: metricNamespace, MetricValue: metricValue, CreatedAt: now,
	}, nil
}

// DeleteMetricFilter removes a metric filter.
func (s *Store) DeleteMetricFilter(accountID, group, filterName string) error {
	res, err := s.db.Exec(
		`DELETE FROM logs_metric_filters WHERE account_id = ? AND log_group_name = ? AND filter_name = ?`,
		accountID, strings.TrimSpace(group), strings.TrimSpace(filterName),
	)
	if err != nil {
		return fmt.Errorf("delete metric filter: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrLogGroupNotFound
	}
	return nil
}

// DescribeMetricFilters lists metric filters for a log group.
func (s *Store) DescribeMetricFilters(accountID, group string) ([]LogsMetricFilter, error) {
	if _, err := s.getLogGroup(accountID, group); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT log_group_name, filter_name, filter_pattern, metric_name, metric_namespace, metric_value, created_at
		 FROM logs_metric_filters WHERE account_id = ? AND log_group_name = ? ORDER BY filter_name`,
		accountID, strings.TrimSpace(group),
	)
	if err != nil {
		return nil, fmt.Errorf("describe metric filters: %w", err)
	}
	defer rows.Close()
	var out []LogsMetricFilter
	for rows.Next() {
		var f LogsMetricFilter
		if err := rows.Scan(&f.LogGroupName, &f.FilterName, &f.FilterPattern, &f.MetricName, &f.MetricNamespace, &f.MetricValue, &f.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *Store) countMetricFilters(accountID, group string) (int, error) {
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM logs_metric_filters WHERE account_id = ? AND log_group_name = ?`,
		accountID, group,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count metric filters: %w", err)
	}
	return n, nil
}

func (s *Store) fanOutLogMetricFilters(accountID, group string, events []LogEvent) {
	filters, err := s.DescribeMetricFilters(accountID, group)
	if err != nil || len(filters) == 0 {
		return
	}
	now := time.Now().UTC().UnixMilli()
	for _, f := range filters {
		for _, ev := range events {
			if !labLogFilterMatches(f.FilterPattern, ev.Message) {
				continue
			}
			val := 1.0
			if parsed, err := parseMetricFilterValue(f.MetricValue); err == nil {
				val = parsed
			}
			ts := ev.Timestamp
			if ts == 0 {
				ts = now
			}
			_, _ = s.db.Exec(
				`INSERT INTO logs_metric_datapoints (account_id, namespace, metric_name, timestamp, value)
				 VALUES (?, ?, ?, ?, ?)`,
				accountID, f.MetricNamespace, f.MetricName, ts, val,
			)
		}
	}
}

func parseMetricFilterValue(raw string) (float64, error) {
	raw = strings.TrimSpace(raw)
	var v float64
	_, err := fmt.Sscanf(raw, "%f", &v)
	return v, err
}

// GetMetricData returns stored lab metric datapoints (CloudWatch Metrics lite stub).
func (s *Store) GetMetricData(accountID, namespace, metricName string, startTime, endTime int64) ([]MetricDatapoint, error) {
	namespace = strings.TrimSpace(namespace)
	metricName = strings.TrimSpace(metricName)
	if namespace == "" || metricName == "" {
		return nil, fmt.Errorf("get metric data: Namespace and MetricName are required")
	}
	q := `SELECT namespace, metric_name, timestamp, value FROM logs_metric_datapoints
		 WHERE account_id = ? AND namespace = ? AND metric_name = ?`
	args := []any{accountID, namespace, metricName}
	if startTime > 0 {
		q += ` AND timestamp >= ?`
		args = append(args, startTime)
	}
	if endTime > 0 {
		q += ` AND timestamp <= ?`
		args = append(args, endTime)
	}
	q += ` ORDER BY timestamp`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("get metric data: %w", err)
	}
	defer rows.Close()
	var out []MetricDatapoint
	for rows.Next() {
		var dp MetricDatapoint
		if err := rows.Scan(&dp.Namespace, &dp.MetricName, &dp.Timestamp, &dp.Value); err != nil {
			return nil, err
		}
		out = append(out, dp)
	}
	return out, rows.Err()
}
