package sdk_test

import (
	"testing"
	"time"
)

func TestCloudWatchMetricsAlarmsSmoke(t *testing.T) {
	requireReady(t)

	now := time.Now().UTC()
	start := now.Add(-time.Hour).Format(time.RFC3339)
	end := now.Add(time.Minute).Format(time.RFC3339)
	ns := "Sdk/MyApp-" + uniquePrefix(t)
	alarmName := "sdk-alarm-" + uniquePrefix(t)

	putStatus, putBody, _ := signedJSONTarget(t, "monitoring", "GraniteServiceVersion20100801.PutMetricData", map[string]any{
		"Namespace": ns,
		"MetricData": []map[string]any{{
			"MetricName": "RequestCount",
			"Value":      7.0,
			"Unit":       "Count",
			"Timestamp":  float64(now.Unix()),
			"Dimensions": []map[string]any{{"Name": "Service", "Value": "api"}},
		}},
	})
	if putStatus != 200 {
		t.Fatalf("PutMetricData status=%d body=%s", putStatus, putBody)
	}

	listStatus, listBody, listParsed := signedJSONTarget(t, "monitoring", "GraniteServiceVersion20100801.ListMetrics", map[string]any{
		"Namespace": ns,
	})
	if listStatus != 200 {
		t.Fatalf("ListMetrics status=%d body=%s", listStatus, listBody)
	}
	metrics, _ := listParsed["Metrics"].([]any)
	if len(metrics) != 1 {
		t.Fatalf("ListMetrics Metrics=%v", listParsed)
	}

	statsStatus, statsBody, statsParsed := signedJSONTarget(t, "monitoring", "GraniteServiceVersion20100801.GetMetricStatistics", map[string]any{
		"Namespace":  ns,
		"MetricName": "RequestCount",
		"StartTime":  start,
		"EndTime":    end,
		"Period":     60,
		"Statistics": []string{"Sum"},
		"Dimensions": []map[string]any{{"Name": "Service", "Value": "api"}},
	})
	if statsStatus != 200 {
		t.Fatalf("GetMetricStatistics status=%d body=%s", statsStatus, statsBody)
	}
	dps, _ := statsParsed["Datapoints"].([]any)
	if len(dps) == 0 {
		t.Fatalf("GetMetricStatistics Datapoints=%v", statsParsed)
	}
	dp0, _ := dps[0].(map[string]any)
	if sum, ok := dp0["Sum"].(float64); !ok || sum < 7 {
		t.Fatalf("GetMetricStatistics Sum want >=7 got %v", dp0)
	}

	alarmStatus, alarmBody, _ := signedJSONTarget(t, "monitoring", "GraniteServiceVersion20100801.PutMetricAlarm", map[string]any{
		"AlarmName":          alarmName,
		"MetricName":         "RequestCount",
		"Namespace":          ns,
		"Statistic":          "Sum",
		"Period":             60,
		"EvaluationPeriods":  1,
		"Threshold":          1.0,
		"ComparisonOperator": "GreaterThanThreshold",
		"Dimensions":         []map[string]any{{"Name": "Service", "Value": "api"}},
	})
	if alarmStatus != 200 {
		t.Fatalf("PutMetricAlarm status=%d body=%s", alarmStatus, alarmBody)
	}

	descStatus, descBody, descParsed := signedJSONTarget(t, "monitoring", "GraniteServiceVersion20100801.DescribeAlarms", map[string]any{
		"AlarmNames": []string{alarmName},
	})
	if descStatus != 200 {
		t.Fatalf("DescribeAlarms status=%d body=%s", descStatus, descBody)
	}
	alarms, _ := descParsed["MetricAlarms"].([]any)
	if len(alarms) != 1 {
		t.Fatalf("DescribeAlarms=%v", descParsed)
	}

	setStatus, setBody, _ := signedJSONTarget(t, "monitoring", "GraniteServiceVersion20100801.SetAlarmState", map[string]any{
		"AlarmName":   alarmName,
		"StateValue":  "ALARM",
		"StateReason": "sdk smoke",
	})
	if setStatus != 200 {
		t.Fatalf("SetAlarmState status=%d body=%s", setStatus, setBody)
	}

	afterStatus, afterBody, afterParsed := signedJSONTarget(t, "monitoring", "GraniteServiceVersion20100801.DescribeAlarms", map[string]any{
		"AlarmNames": []string{alarmName},
	})
	if afterStatus != 200 {
		t.Fatalf("DescribeAlarms after SetAlarmState status=%d body=%s", afterStatus, afterBody)
	}
	afterAlarms, _ := afterParsed["MetricAlarms"].([]any)
	if len(afterAlarms) != 1 {
		t.Fatalf("DescribeAlarms after set=%v", afterParsed)
	}
	after0, _ := afterAlarms[0].(map[string]any)
	if after0["StateValue"] != "ALARM" {
		t.Fatalf("StateValue want ALARM got %v", after0["StateValue"])
	}

	delStatus, delBody, _ := signedJSONTarget(t, "monitoring", "GraniteServiceVersion20100801.DeleteAlarms", map[string]any{
		"AlarmNames": []string{alarmName},
	})
	if delStatus != 200 {
		t.Fatalf("DeleteAlarms status=%d body=%s", delStatus, delBody)
	}
}
