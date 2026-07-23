package store

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	filterLogEventsDefaultLimit = 10_000
	filterLogEventsLabCap       = 1_000
)

// FilterLogEventsInput is the lab FilterLogEvents request subset.
type FilterLogEventsInput struct {
	LogGroupName   string
	LogStreamNames []string // optional; empty means all streams in the group
	FilterPattern  string   // lab filter-pattern subset (same matcher as subscription/metric filters)
	StartTime      int64
	EndTime        int64
	Limit          int
	NextToken      string
}

// FilteredLogEvent is one FilterLogEvents result row (includes stream name).
type FilteredLogEvent struct {
	LogStreamName string
	Timestamp     int64
	Message       string
	IngestionTime int64
	EventID       string
}

// FilterLogEvents scans events across streams in a log group with optional
// filter-pattern subset, time bounds, stream names, and offset pagination.
func (s *Store) FilterLogEvents(accountID string, in FilterLogEventsInput) ([]FilteredLogEvent, string, error) {
	group := strings.TrimSpace(in.LogGroupName)
	if group == "" {
		return nil, "", fmt.Errorf("filter log events: log group name is required")
	}
	if _, err := s.getLogGroup(accountID, group); err != nil {
		return nil, "", err
	}
	if _, err := MatchLogFilterPattern(in.FilterPattern, ""); err != nil {
		return nil, "", err
	}

	offset := 0
	if tok := strings.TrimSpace(in.NextToken); tok != "" {
		n, err := strconv.Atoi(tok)
		if err != nil || n < 0 {
			return nil, "", fmt.Errorf("filter log events: invalid nextToken")
		}
		offset = n
	}

	limit := in.Limit
	if limit <= 0 {
		limit = filterLogEventsDefaultLimit
	}
	if limit > filterLogEventsLabCap {
		limit = filterLogEventsLabCap
	}

	q := `SELECT log_stream_name, event_id, timestamp, ingestion_time, message
		 FROM logs_events
		 WHERE account_id = ? AND log_group_name = ?`
	args := []any{accountID, group}

	streams := normalizeLogStreamNames(in.LogStreamNames)
	if len(streams) > 0 {
		placeholders := make([]string, len(streams))
		for i, name := range streams {
			placeholders[i] = "?"
			args = append(args, name)
		}
		q += ` AND log_stream_name IN (` + strings.Join(placeholders, ",") + `)`
	}
	if in.StartTime > 0 {
		q += ` AND timestamp >= ?`
		args = append(args, in.StartTime)
	}
	if in.EndTime > 0 {
		q += ` AND timestamp <= ?`
		args = append(args, in.EndTime)
	}
	q += ` ORDER BY timestamp ASC, event_id ASC`

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, "", fmt.Errorf("filter log events: %w", err)
	}
	defer rows.Close()

	matched := make([]FilteredLogEvent, 0)
	for rows.Next() {
		var ev FilteredLogEvent
		if err := rows.Scan(&ev.LogStreamName, &ev.EventID, &ev.Timestamp, &ev.IngestionTime, &ev.Message); err != nil {
			return nil, "", fmt.Errorf("filter log events: scan: %w", err)
		}
		if !labLogFilterMatches(in.FilterPattern, ev.Message) {
			continue
		}
		matched = append(matched, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("filter log events: %w", err)
	}

	if offset > len(matched) {
		offset = len(matched)
	}
	end := offset + limit
	if end > len(matched) {
		end = len(matched)
	}
	page := matched[offset:end]
	next := ""
	if end < len(matched) {
		next = strconv.Itoa(end)
	}
	return page, next, nil
}

func normalizeLogStreamNames(names []string) []string {
	if len(names) == 0 {
		return nil
	}
	out := make([]string, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	return out
}
