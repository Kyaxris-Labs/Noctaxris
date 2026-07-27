package cloudwatch

import (
	"encoding/json"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func dimsMaps(dims []store.CWDimension) []map[string]string {
	out := make([]map[string]string, 0, len(dims))
	for _, d := range dims {
		out = append(out, map[string]string{"Name": d.Name, "Value": d.Value})
	}
	return out
}

// PutMetricDataJSON is empty OK.
func PutMetricDataJSON() ([]byte, error) { return []byte(`{}`), nil }

// ListMetricsJSON builds ListMetrics response.
func ListMetricsJSON(metrics []store.CWMetricIdentity) ([]byte, error) {
	items := make([]map[string]any, 0, len(metrics))
	for _, m := range metrics {
		items = append(items, map[string]any{
			"Namespace":  m.Namespace,
			"MetricName": m.MetricName,
			"Dimensions": dimsMaps(m.Dimensions),
		})
	}
	return json.Marshal(map[string]any{"Metrics": items})
}

// GetMetricStatisticsJSON builds GetMetricStatistics response.
func GetMetricStatisticsJSON(label string, stats []string, dps []store.CWDatapoint) ([]byte, error) {
	want := map[string]bool{}
	for _, s := range stats {
		want[s] = true
	}
	if len(want) == 0 {
		want["Average"] = true
	}
	items := make([]map[string]any, 0, len(dps))
	for _, dp := range dps {
		m := map[string]any{
			"Timestamp": time.Unix(dp.Timestamp, 0).UTC().Format(time.RFC3339),
			"Unit":      dp.Unit,
		}
		if want["Average"] {
			m["Average"] = dp.Average
		}
		if want["Sum"] {
			m["Sum"] = dp.Sum
		}
		if want["Minimum"] {
			m["Minimum"] = dp.Minimum
		}
		if want["Maximum"] {
			m["Maximum"] = dp.Maximum
		}
		if want["SampleCount"] {
			m["SampleCount"] = dp.SampleCount
		}
		items = append(items, m)
	}
	return json.Marshal(map[string]any{"Label": label, "Datapoints": items})
}

// GetMetricDataJSON builds GetMetricData response.
func GetMetricDataJSON(results []store.CWMetricDataResult) ([]byte, error) {
	items := make([]map[string]any, 0, len(results))
	for _, r := range results {
		ts := make([]string, 0, len(r.Timestamps))
		for _, t := range r.Timestamps {
			ts = append(ts, time.Unix(t, 0).UTC().Format(time.RFC3339))
		}
		items = append(items, map[string]any{
			"Id":         r.ID,
			"Label":      r.Label,
			"Timestamps": ts,
			"Values":     r.Values,
			"StatusCode": r.StatusCode,
		})
	}
	return json.Marshal(map[string]any{"MetricDataResults": items})
}

// PutMetricAlarmJSON is empty OK.
func PutMetricAlarmJSON() ([]byte, error) { return []byte(`{}`), nil }

func alarmMap(a store.CWMetricAlarm) map[string]any {
	return map[string]any{
		"AlarmName":            a.AlarmName,
		"AlarmArn":             a.AlarmArn,
		"AlarmDescription":     a.AlarmDescription,
		"MetricName":           a.MetricName,
		"Namespace":            a.Namespace,
		"Statistic":            a.Statistic,
		"Period":               a.Period,
		"EvaluationPeriods":    a.EvaluationPeriods,
		"Threshold":            a.Threshold,
		"ComparisonOperator":   a.ComparisonOperator,
		"Dimensions":           dimsMaps(a.Dimensions),
		"ActionsEnabled":       a.ActionsEnabled,
		"StateValue":           a.StateValue,
		"StateReason":          a.StateReason,
		"StateReasonData":      a.StateReasonData,
		"StateUpdatedTimestamp": time.Unix(a.StateUpdatedAt, 0).UTC().Format(time.RFC3339),
		"AlarmConfigurationUpdatedTimestamp": time.Unix(a.ConfigUpdatedAt, 0).UTC().Format(time.RFC3339),
	}
}

// DescribeAlarmsJSON builds DescribeAlarms response.
func DescribeAlarmsJSON(alarms []store.CWMetricAlarm) ([]byte, error) {
	items := make([]map[string]any, 0, len(alarms))
	for _, a := range alarms {
		items = append(items, alarmMap(a))
	}
	return json.Marshal(map[string]any{"MetricAlarms": items, "CompositeAlarms": []any{}})
}

// DeleteAlarmsJSON is empty OK.
func DeleteAlarmsJSON() ([]byte, error) { return []byte(`{}`), nil }

// SetAlarmStateJSON is empty OK.
func SetAlarmStateJSON() ([]byte, error) { return []byte(`{}`), nil }
