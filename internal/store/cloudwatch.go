package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

var (
	ErrCWBadRequest      = errors.New("InvalidParameterValue")
	ErrCWAlarmNotFound   = errors.New("ResourceNotFound")
	ErrCWMissingParameter = errors.New("MissingParameter")
)

const cloudwatchSchema = `
CREATE TABLE IF NOT EXISTS cw_metric_datapoints (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  account_id TEXT NOT NULL,
  region TEXT NOT NULL,
  namespace TEXT NOT NULL,
  metric_name TEXT NOT NULL,
  dim_key TEXT NOT NULL DEFAULT '',
  dimensions_json TEXT NOT NULL DEFAULT '[]',
  timestamp INTEGER NOT NULL,
  sample_count REAL NOT NULL,
  sum_value REAL NOT NULL,
  minimum REAL NOT NULL,
  maximum REAL NOT NULL,
  unit TEXT NOT NULL DEFAULT 'None'
);
CREATE INDEX IF NOT EXISTS idx_cw_metric_dp
  ON cw_metric_datapoints(account_id, region, namespace, metric_name, dim_key, timestamp);

CREATE TABLE IF NOT EXISTS cw_metric_alarms (
  account_id TEXT NOT NULL,
  region TEXT NOT NULL,
  alarm_name TEXT NOT NULL,
  alarm_arn TEXT NOT NULL,
  alarm_description TEXT NOT NULL DEFAULT '',
  metric_name TEXT NOT NULL DEFAULT '',
  namespace TEXT NOT NULL DEFAULT '',
  statistic TEXT NOT NULL DEFAULT 'Average',
  period INTEGER NOT NULL DEFAULT 60,
  evaluation_periods INTEGER NOT NULL DEFAULT 1,
  threshold REAL NOT NULL DEFAULT 0,
  comparison_operator TEXT NOT NULL DEFAULT 'GreaterThanThreshold',
  dimensions_json TEXT NOT NULL DEFAULT '[]',
  actions_enabled INTEGER NOT NULL DEFAULT 1,
  state_value TEXT NOT NULL DEFAULT 'OK',
  state_reason TEXT NOT NULL DEFAULT 'Unchecked: Initial alarm creation',
  state_reason_data TEXT NOT NULL DEFAULT '',
  state_updated_at INTEGER NOT NULL DEFAULT 0,
  config_updated_at INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (account_id, region, alarm_name)
);
`

// CWDimension is a CloudWatch metric dimension.
type CWDimension struct {
	Name  string `json:"Name"`
	Value string `json:"Value"`
}

// CWMetricDatum is one PutMetricData point.
type CWMetricDatum struct {
	MetricName  string
	Dimensions  []CWDimension
	Timestamp   int64 // unix seconds; 0 means now
	Value       float64
	SampleCount float64
	Sum         float64
	Minimum     float64
	Maximum     float64
	Unit        string
	HasStats    bool
}

// CWMetricIdentity is a unique metric listing row.
type CWMetricIdentity struct {
	Namespace  string
	MetricName string
	Dimensions []CWDimension
}

// CWDatapoint is an aggregated GetMetricStatistics bucket.
type CWDatapoint struct {
	Timestamp   int64
	SampleCount float64
	Sum         float64
	Average     float64
	Minimum     float64
	Maximum     float64
	Unit        string
}

// CWMetricAlarm is a stored metric alarm.
type CWMetricAlarm struct {
	AccountID          string
	Region             string
	AlarmName          string
	AlarmArn           string
	AlarmDescription   string
	MetricName         string
	Namespace          string
	Statistic          string
	Period             int
	EvaluationPeriods  int
	Threshold          float64
	ComparisonOperator string
	Dimensions         []CWDimension
	ActionsEnabled     bool
	StateValue         string
	StateReason        string
	StateReasonData    string
	StateUpdatedAt     int64
	ConfigUpdatedAt    int64
}

// CWMetricStat describes a GetMetricData MetricStat query.
type CWMetricStat struct {
	Namespace  string
	MetricName string
	Dimensions []CWDimension
	Period     int
	Stat       string
	Unit       string
}

// CWMetricDataQuery is one GetMetricData query (MetricStat only; expressions unsupported).
type CWMetricDataQuery struct {
	ID         string
	MetricStat *CWMetricStat
	Label      string
	ReturnData bool
}

// CWMetricDataResult is one GetMetricData result series.
type CWMetricDataResult struct {
	ID          string
	Label       string
	Timestamps  []int64
	Values      []float64
	StatusCode  string
}

// EnsureCloudWatchSchema creates CloudWatch Metrics/Alarms tables if missing.
func EnsureCloudWatchSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure cloudwatch schema: db is nil")
	}
	if _, err := db.Exec(cloudwatchSchema); err != nil {
		return fmt.Errorf("ensure cloudwatch schema: %w", err)
	}
	return nil
}

// EnsureCloudWatchSchema ensures CloudWatch tables on an open store.
func (s *Store) EnsureCloudWatchSchema() error {
	return EnsureCloudWatchSchema(s.db)
}

// BuildCWDimKey returns a stable sorted dimension key.
func BuildCWDimKey(dims []CWDimension) string {
	if len(dims) == 0 {
		return ""
	}
	cp := append([]CWDimension(nil), dims...)
	sort.Slice(cp, func(i, j int) bool {
		if cp[i].Name != cp[j].Name {
			return cp[i].Name < cp[j].Name
		}
		return cp[i].Value < cp[j].Value
	})
	parts := make([]string, 0, len(cp))
	for _, d := range cp {
		parts = append(parts, d.Name+"="+d.Value)
	}
	return strings.Join(parts, ",")
}

func marshalCWDimensions(dims []CWDimension) (string, error) {
	if dims == nil {
		dims = []CWDimension{}
	}
	raw, err := json.Marshal(dims)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func unmarshalCWDimensions(raw string) ([]CWDimension, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var dims []CWDimension
	if err := json.Unmarshal([]byte(raw), &dims); err != nil {
		return nil, err
	}
	return dims, nil
}

// PutMetricData stores metric datapoints for an account/region.
func (s *Store) PutMetricData(accountID, region, namespace string, datums []CWMetricDatum) error {
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return fmt.Errorf("%w: Namespace required", ErrCWMissingParameter)
	}
	if len(datums) == 0 {
		return fmt.Errorf("%w: MetricData required", ErrCWMissingParameter)
	}
	region = strings.TrimSpace(region)
	if region == "" {
		region = "us-east-1"
	}
	now := time.Now().UTC().Unix()
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("put metric data begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare(
		`INSERT INTO cw_metric_datapoints
		 (account_id, region, namespace, metric_name, dim_key, dimensions_json, timestamp,
		  sample_count, sum_value, minimum, maximum, unit)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
	)
	if err != nil {
		return fmt.Errorf("put metric data prepare: %w", err)
	}
	defer stmt.Close()

	for i, d := range datums {
		name := strings.TrimSpace(d.MetricName)
		if name == "" {
			return fmt.Errorf("%w: MetricData[%d].MetricName required", ErrCWMissingParameter, i)
		}
		ts := d.Timestamp
		if ts <= 0 {
			ts = now
		}
		sc, sum, minV, maxV := d.SampleCount, d.Sum, d.Minimum, d.Maximum
		if !d.HasStats {
			sc, sum, minV, maxV = 1, d.Value, d.Value, d.Value
		}
		if sc <= 0 {
			sc = 1
			sum, minV, maxV = d.Value, d.Value, d.Value
		}
		unit := strings.TrimSpace(d.Unit)
		if unit == "" {
			unit = "None"
		}
		dimJSON, err := marshalCWDimensions(d.Dimensions)
		if err != nil {
			return fmt.Errorf("%w: dimensions", ErrCWBadRequest)
		}
		dimKey := BuildCWDimKey(d.Dimensions)
		if _, err := stmt.Exec(
			accountID, region, namespace, name, dimKey, dimJSON, ts,
			sc, sum, minV, maxV, unit,
		); err != nil {
			return fmt.Errorf("put metric data insert: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("put metric data commit: %w", err)
	}
	return nil
}

// ListMetrics returns unique metric identities, optionally filtered.
func (s *Store) ListMetrics(accountID, region, namespace, metricName string, dims []CWDimension) ([]CWMetricIdentity, error) {
	region = strings.TrimSpace(region)
	if region == "" {
		region = "us-east-1"
	}
	q := `SELECT namespace, metric_name, dimensions_json FROM cw_metric_datapoints
		 WHERE account_id = ? AND region = ?`
	args := []any{accountID, region}
	if ns := strings.TrimSpace(namespace); ns != "" {
		q += ` AND namespace = ?`
		args = append(args, ns)
	}
	if mn := strings.TrimSpace(metricName); mn != "" {
		q += ` AND metric_name = ?`
		args = append(args, mn)
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("list metrics: %w", err)
	}
	defer rows.Close()

	filterKey := BuildCWDimKey(dims)
	seen := map[string]CWMetricIdentity{}
	for rows.Next() {
		var ns, mn, dimJSON string
		if err := rows.Scan(&ns, &mn, &dimJSON); err != nil {
			return nil, fmt.Errorf("list metrics scan: %w", err)
		}
		parsed, err := unmarshalCWDimensions(dimJSON)
		if err != nil {
			return nil, fmt.Errorf("list metrics dimensions: %w", err)
		}
		if filterKey != "" {
			actual := BuildCWDimKey(parsed)
			if !strings.Contains(actual, filterKey) {
				continue
			}
		}
		key := ns + "::" + mn + "::" + BuildCWDimKey(parsed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = CWMetricIdentity{Namespace: ns, MetricName: mn, Dimensions: parsed}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]CWMetricIdentity, 0, len(seen))
	for _, v := range seen {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace != out[j].Namespace {
			return out[i].Namespace < out[j].Namespace
		}
		if out[i].MetricName != out[j].MetricName {
			return out[i].MetricName < out[j].MetricName
		}
		return BuildCWDimKey(out[i].Dimensions) < BuildCWDimKey(out[j].Dimensions)
	})
	return out, nil
}

// GetMetricStatistics aggregates datapoints into period buckets.
func (s *Store) GetMetricStatistics(
	accountID, region, namespace, metricName string,
	dims []CWDimension,
	startTime, endTime int64,
	periodSeconds int,
	unit string,
) ([]CWDatapoint, error) {
	namespace = strings.TrimSpace(namespace)
	metricName = strings.TrimSpace(metricName)
	if namespace == "" || metricName == "" {
		return nil, fmt.Errorf("%w: Namespace and MetricName required", ErrCWMissingParameter)
	}
	if periodSeconds <= 0 {
		periodSeconds = 60
	}
	region = strings.TrimSpace(region)
	if region == "" {
		region = "us-east-1"
	}
	if endTime <= 0 {
		endTime = time.Now().UTC().Unix()
	}
	if startTime <= 0 {
		startTime = endTime - 3600
	}
	if startTime >= endTime {
		return nil, fmt.Errorf("%w: StartTime must be before EndTime", ErrCWBadRequest)
	}
	dimKey := BuildCWDimKey(dims)
	q := `SELECT timestamp, sample_count, sum_value, minimum, maximum, unit
		 FROM cw_metric_datapoints
		 WHERE account_id = ? AND region = ? AND namespace = ? AND metric_name = ?
		   AND dim_key = ? AND timestamp >= ? AND timestamp <= ?`
	args := []any{accountID, region, namespace, metricName, dimKey, startTime, endTime}
	if u := strings.TrimSpace(unit); u != "" && !strings.EqualFold(u, "None") {
		q += ` AND unit = ?`
		args = append(args, u)
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("get metric statistics: %w", err)
	}
	defer rows.Close()

	type rawDP struct {
		ts, sc, sum, min, max float64
		unit                  string
	}
	var raw []rawDP
	for rows.Next() {
		var r rawDP
		var ts int64
		if err := rows.Scan(&ts, &r.sc, &r.sum, &r.min, &r.max, &r.unit); err != nil {
			return nil, fmt.Errorf("get metric statistics scan: %w", err)
		}
		r.ts = float64(ts)
		raw = append(raw, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	buckets := map[int64][]rawDP{}
	var keys []int64
	for _, r := range raw {
		bucket := (int64(r.ts) / int64(periodSeconds)) * int64(periodSeconds)
		if _, ok := buckets[bucket]; !ok {
			keys = append(keys, bucket)
		}
		buckets[bucket] = append(buckets[bucket], r)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

	out := make([]CWDatapoint, 0, len(keys))
	for _, k := range keys {
		group := buckets[k]
		var sc, sum float64
		minV := math.Inf(1)
		maxV := math.Inf(-1)
		unitOut := "None"
		for _, g := range group {
			sc += g.sc
			sum += g.sum
			if g.min < minV {
				minV = g.min
			}
			if g.max > maxV {
				maxV = g.max
			}
			if g.unit != "" {
				unitOut = g.unit
			}
		}
		avg := 0.0
		if sc > 0 {
			avg = sum / sc
		}
		if math.IsInf(minV, 1) {
			minV = 0
		}
		if math.IsInf(maxV, -1) {
			maxV = 0
		}
		out = append(out, CWDatapoint{
			Timestamp: k, SampleCount: sc, Sum: sum, Average: avg,
			Minimum: minV, Maximum: maxV, Unit: unitOut,
		})
	}
	return out, nil
}

func resolveCWStat(dp CWDatapoint, stat string) float64 {
	switch strings.TrimSpace(stat) {
	case "Sum":
		return dp.Sum
	case "Minimum":
		return dp.Minimum
	case "Maximum":
		return dp.Maximum
	case "SampleCount":
		return dp.SampleCount
	case "Average", "":
		return dp.Average
	default:
		if strings.HasPrefix(stat, "p") || strings.HasPrefix(stat, "P") {
			return dp.Maximum
		}
		return dp.Average
	}
}

// GetCloudWatchMetricData evaluates simple MetricStat queries (no math expressions).
func (s *Store) GetCloudWatchMetricData(
	accountID, region string,
	queries []CWMetricDataQuery,
	startTime, endTime int64,
) ([]CWMetricDataResult, error) {
	region = strings.TrimSpace(region)
	if region == "" {
		region = "us-east-1"
	}
	if endTime <= 0 {
		endTime = time.Now().UTC().Unix()
	}
	if startTime <= 0 {
		startTime = endTime - 3600
	}
	var out []CWMetricDataResult
	for _, q := range queries {
		if !q.ReturnData {
			continue
		}
		if q.MetricStat == nil {
			continue
		}
		st := q.MetricStat
		period := st.Period
		if period <= 0 {
			period = 60
		}
		dps, err := s.GetMetricStatistics(
			accountID, region, st.Namespace, st.MetricName, st.Dimensions,
			startTime, endTime, period, st.Unit,
		)
		if err != nil {
			return nil, err
		}
		label := q.Label
		if label == "" {
			label = st.MetricName
		}
		res := CWMetricDataResult{
			ID: q.ID, Label: label, StatusCode: "Complete",
			Timestamps: make([]int64, 0, len(dps)),
			Values:     make([]float64, 0, len(dps)),
		}
		for _, dp := range dps {
			res.Timestamps = append(res.Timestamps, dp.Timestamp)
			res.Values = append(res.Values, resolveCWStat(dp, st.Stat))
		}
		out = append(out, res)
	}
	return out, nil
}

// PutMetricAlarm creates or replaces a metric alarm.
func (s *Store) PutMetricAlarm(accountID, region string, alarm CWMetricAlarm) (CWMetricAlarm, error) {
	name := strings.TrimSpace(alarm.AlarmName)
	if name == "" {
		return CWMetricAlarm{}, fmt.Errorf("%w: AlarmName required", ErrCWMissingParameter)
	}
	region = strings.TrimSpace(region)
	if region == "" {
		region = "us-east-1"
	}
	now := time.Now().UTC().Unix()
	if alarm.Period <= 0 {
		alarm.Period = 60
	}
	if alarm.EvaluationPeriods <= 0 {
		alarm.EvaluationPeriods = 1
	}
	if strings.TrimSpace(alarm.Statistic) == "" {
		alarm.Statistic = "Average"
	}
	if strings.TrimSpace(alarm.ComparisonOperator) == "" {
		alarm.ComparisonOperator = "GreaterThanThreshold"
	}
	if strings.TrimSpace(alarm.StateValue) == "" {
		alarm.StateValue = "OK"
	}
	if strings.TrimSpace(alarm.StateReason) == "" {
		alarm.StateReason = "Unchecked: Initial alarm creation"
	}
	dimJSON, err := marshalCWDimensions(alarm.Dimensions)
	if err != nil {
		return CWMetricAlarm{}, fmt.Errorf("%w: dimensions", ErrCWBadRequest)
	}
	arn := fmt.Sprintf("arn:aws:cloudwatch:%s:%s:alarm:%s", region, accountID, name)
	actions := 0
	if alarm.ActionsEnabled {
		actions = 1
	}
	_, err = s.db.Exec(
		`INSERT INTO cw_metric_alarms (
		  account_id, region, alarm_name, alarm_arn, alarm_description, metric_name, namespace,
		  statistic, period, evaluation_periods, threshold, comparison_operator, dimensions_json,
		  actions_enabled, state_value, state_reason, state_reason_data, state_updated_at, config_updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(account_id, region, alarm_name) DO UPDATE SET
		  alarm_description=excluded.alarm_description,
		  metric_name=excluded.metric_name,
		  namespace=excluded.namespace,
		  statistic=excluded.statistic,
		  period=excluded.period,
		  evaluation_periods=excluded.evaluation_periods,
		  threshold=excluded.threshold,
		  comparison_operator=excluded.comparison_operator,
		  dimensions_json=excluded.dimensions_json,
		  actions_enabled=excluded.actions_enabled,
		  config_updated_at=excluded.config_updated_at`,
		accountID, region, name, arn, alarm.AlarmDescription, alarm.MetricName, alarm.Namespace,
		alarm.Statistic, alarm.Period, alarm.EvaluationPeriods, alarm.Threshold, alarm.ComparisonOperator, dimJSON,
		actions, alarm.StateValue, alarm.StateReason, alarm.StateReasonData, now, now,
	)
	if err != nil {
		return CWMetricAlarm{}, fmt.Errorf("put metric alarm: %w", err)
	}
	return s.DescribeAlarm(accountID, region, name)
}

// DescribeAlarm returns one alarm by name.
func (s *Store) DescribeAlarm(accountID, region, name string) (CWMetricAlarm, error) {
	region = strings.TrimSpace(region)
	if region == "" {
		region = "us-east-1"
	}
	var a CWMetricAlarm
	var dimJSON string
	var actions int
	err := s.db.QueryRow(
		`SELECT account_id, region, alarm_name, alarm_arn, alarm_description, metric_name, namespace,
		        statistic, period, evaluation_periods, threshold, comparison_operator, dimensions_json,
		        actions_enabled, state_value, state_reason, state_reason_data, state_updated_at, config_updated_at
		 FROM cw_metric_alarms WHERE account_id = ? AND region = ? AND alarm_name = ?`,
		accountID, region, strings.TrimSpace(name),
	).Scan(
		&a.AccountID, &a.Region, &a.AlarmName, &a.AlarmArn, &a.AlarmDescription, &a.MetricName, &a.Namespace,
		&a.Statistic, &a.Period, &a.EvaluationPeriods, &a.Threshold, &a.ComparisonOperator, &dimJSON,
		&actions, &a.StateValue, &a.StateReason, &a.StateReasonData, &a.StateUpdatedAt, &a.ConfigUpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return CWMetricAlarm{}, ErrCWAlarmNotFound
	}
	if err != nil {
		return CWMetricAlarm{}, fmt.Errorf("describe alarm: %w", err)
	}
	a.ActionsEnabled = actions != 0
	a.Dimensions, err = unmarshalCWDimensions(dimJSON)
	if err != nil {
		return CWMetricAlarm{}, fmt.Errorf("describe alarm dimensions: %w", err)
	}
	return a, nil
}

// DescribeAlarms lists alarms filtered by names or prefix.
func (s *Store) DescribeAlarms(accountID, region string, names []string, namePrefix string) ([]CWMetricAlarm, error) {
	region = strings.TrimSpace(region)
	if region == "" {
		region = "us-east-1"
	}
	rows, err := s.db.Query(
		`SELECT account_id, region, alarm_name, alarm_arn, alarm_description, metric_name, namespace,
		        statistic, period, evaluation_periods, threshold, comparison_operator, dimensions_json,
		        actions_enabled, state_value, state_reason, state_reason_data, state_updated_at, config_updated_at
		 FROM cw_metric_alarms WHERE account_id = ? AND region = ? ORDER BY alarm_name`,
		accountID, region,
	)
	if err != nil {
		return nil, fmt.Errorf("describe alarms: %w", err)
	}
	defer rows.Close()

	nameSet := map[string]struct{}{}
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n != "" {
			nameSet[n] = struct{}{}
		}
	}
	prefix := strings.TrimSpace(namePrefix)
	var out []CWMetricAlarm
	for rows.Next() {
		var a CWMetricAlarm
		var dimJSON string
		var actions int
		if err := rows.Scan(
			&a.AccountID, &a.Region, &a.AlarmName, &a.AlarmArn, &a.AlarmDescription, &a.MetricName, &a.Namespace,
			&a.Statistic, &a.Period, &a.EvaluationPeriods, &a.Threshold, &a.ComparisonOperator, &dimJSON,
			&actions, &a.StateValue, &a.StateReason, &a.StateReasonData, &a.StateUpdatedAt, &a.ConfigUpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("describe alarms scan: %w", err)
		}
		if len(nameSet) > 0 {
			if _, ok := nameSet[a.AlarmName]; !ok {
				continue
			}
		}
		if prefix != "" && !strings.HasPrefix(a.AlarmName, prefix) {
			continue
		}
		a.ActionsEnabled = actions != 0
		a.Dimensions, err = unmarshalCWDimensions(dimJSON)
		if err != nil {
			return nil, fmt.Errorf("describe alarms dimensions: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// DeleteAlarms deletes alarms by name (missing names are ignored).
func (s *Store) DeleteAlarms(accountID, region string, names []string) error {
	region = strings.TrimSpace(region)
	if region == "" {
		region = "us-east-1"
	}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, err := s.db.Exec(
			`DELETE FROM cw_metric_alarms WHERE account_id = ? AND region = ? AND alarm_name = ?`,
			accountID, region, name,
		); err != nil {
			return fmt.Errorf("delete alarms: %w", err)
		}
	}
	return nil
}

// SetAlarmState updates alarm state fields.
func (s *Store) SetAlarmState(accountID, region, name, stateValue, stateReason, stateReasonData string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("%w: AlarmName required", ErrCWMissingParameter)
	}
	stateValue = strings.TrimSpace(stateValue)
	switch stateValue {
	case "OK", "ALARM", "INSUFFICIENT_DATA":
	default:
		return fmt.Errorf("%w: StateValue must be OK, ALARM, or INSUFFICIENT_DATA", ErrCWBadRequest)
	}
	if strings.TrimSpace(stateReason) == "" {
		return fmt.Errorf("%w: StateReason required", ErrCWMissingParameter)
	}
	region = strings.TrimSpace(region)
	if region == "" {
		region = "us-east-1"
	}
	now := time.Now().UTC().Unix()
	res, err := s.db.Exec(
		`UPDATE cw_metric_alarms SET state_value = ?, state_reason = ?, state_reason_data = ?, state_updated_at = ?
		 WHERE account_id = ? AND region = ? AND alarm_name = ?`,
		stateValue, stateReason, stateReasonData, now, accountID, region, name,
	)
	if err != nil {
		return fmt.Errorf("set alarm state: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("set alarm state: %w", err)
	}
	if n == 0 {
		return ErrCWAlarmNotFound
	}
	return nil
}
