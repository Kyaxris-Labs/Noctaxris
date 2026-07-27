package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func mustCWJSON(t *testing.T, handler http.Handler, target string, payload map[string]any, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.0")
	req.Header.Set("X-Amz-Target", "GraniteServiceVersion20100801."+target)
	signHeader(t, req, raw, testAccessKey, testSecret, testRegion, "monitoring", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestCloudWatchMetricsAlarmsRoundTrip(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)
	start := now.Add(-time.Hour).Format(time.RFC3339)
	end := now.Add(time.Minute).Format(time.RFC3339)

	put := mustCWJSON(t, handler, "PutMetricData", map[string]any{
		"Namespace": "MyApp",
		"MetricData": []map[string]any{{
			"MetricName": "RequestCount",
			"Value":      42.0,
			"Unit":       "Count",
			"Timestamp":  float64(now.Unix()),
			"Dimensions": []map[string]any{{"Name": "Service", "Value": "api"}},
		}},
	}, now)
	if put.Code != http.StatusOK {
		t.Fatalf("PutMetricData status=%d body=%q", put.Code, put.Body.String())
	}

	list := mustCWJSON(t, handler, "ListMetrics", map[string]any{
		"Namespace": "MyApp",
	}, now)
	if list.Code != http.StatusOK {
		t.Fatalf("ListMetrics status=%d body=%q", list.Code, list.Body.String())
	}
	var listOut map[string]any
	if err := json.Unmarshal(list.Body.Bytes(), &listOut); err != nil {
		t.Fatal(err)
	}
	metrics, _ := listOut["Metrics"].([]any)
	if len(metrics) != 1 {
		t.Fatalf("Metrics=%v", listOut)
	}

	stats := mustCWJSON(t, handler, "GetMetricStatistics", map[string]any{
		"Namespace":  "MyApp",
		"MetricName": "RequestCount",
		"StartTime":  start,
		"EndTime":    end,
		"Period":     60,
		"Statistics": []string{"Sum", "Average"},
		"Dimensions": []map[string]any{{"Name": "Service", "Value": "api"}},
	}, now)
	if stats.Code != http.StatusOK {
		t.Fatalf("GetMetricStatistics status=%d body=%q", stats.Code, stats.Body.String())
	}
	var statsOut map[string]any
	if err := json.Unmarshal(stats.Body.Bytes(), &statsOut); err != nil {
		t.Fatal(err)
	}
	dps, _ := statsOut["Datapoints"].([]any)
	if len(dps) == 0 {
		t.Fatalf("Datapoints empty: %v", statsOut)
	}

	gmd := mustCWJSON(t, handler, "GetMetricData", map[string]any{
		"StartTime": start,
		"EndTime":   end,
		"MetricDataQueries": []map[string]any{{
			"Id": "m1",
			"MetricStat": map[string]any{
				"Metric": map[string]any{
					"Namespace":  "MyApp",
					"MetricName": "RequestCount",
					"Dimensions": []map[string]any{{"Name": "Service", "Value": "api"}},
				},
				"Period": 60,
				"Stat":   "Sum",
			},
			"ReturnData": true,
		}},
	}, now)
	if gmd.Code != http.StatusOK {
		t.Fatalf("GetMetricData status=%d body=%q", gmd.Code, gmd.Body.String())
	}

	alarm := mustCWJSON(t, handler, "PutMetricAlarm", map[string]any{
		"AlarmName":          "high-requests",
		"MetricName":         "RequestCount",
		"Namespace":          "MyApp",
		"Statistic":          "Sum",
		"Period":             60,
		"EvaluationPeriods":  1,
		"Threshold":          10.0,
		"ComparisonOperator": "GreaterThanThreshold",
		"Dimensions":         []map[string]any{{"Name": "Service", "Value": "api"}},
	}, now)
	if alarm.Code != http.StatusOK {
		t.Fatalf("PutMetricAlarm status=%d body=%q", alarm.Code, alarm.Body.String())
	}

	desc := mustCWJSON(t, handler, "DescribeAlarms", map[string]any{
		"AlarmNames": []string{"high-requests"},
	}, now)
	if desc.Code != http.StatusOK {
		t.Fatalf("DescribeAlarms status=%d body=%q", desc.Code, desc.Body.String())
	}
	var descOut map[string]any
	if err := json.Unmarshal(desc.Body.Bytes(), &descOut); err != nil {
		t.Fatal(err)
	}
	alarms, _ := descOut["MetricAlarms"].([]any)
	if len(alarms) != 1 {
		t.Fatalf("MetricAlarms=%v", descOut)
	}

	set := mustCWJSON(t, handler, "SetAlarmState", map[string]any{
		"AlarmName":  "high-requests",
		"StateValue": "ALARM",
		"StateReason": "manual set",
	}, now)
	if set.Code != http.StatusOK {
		t.Fatalf("SetAlarmState status=%d body=%q", set.Code, set.Body.String())
	}

	afterSet := mustCWJSON(t, handler, "DescribeAlarms", map[string]any{
		"AlarmNames": []string{"high-requests"},
	}, now)
	if afterSet.Code != http.StatusOK {
		t.Fatalf("DescribeAlarms after SetAlarmState status=%d body=%q", afterSet.Code, afterSet.Body.String())
	}
	var afterOut map[string]any
	if err := json.Unmarshal(afterSet.Body.Bytes(), &afterOut); err != nil {
		t.Fatal(err)
	}
	afterAlarms, _ := afterOut["MetricAlarms"].([]any)
	if len(afterAlarms) != 1 {
		t.Fatalf("after SetAlarmState MetricAlarms=%v", afterOut)
	}
	alarmRow, _ := afterAlarms[0].(map[string]any)
	if alarmRow["StateValue"] != "ALARM" {
		t.Fatalf("StateValue after SetAlarmState=%v", alarmRow["StateValue"])
	}

	del := mustCWJSON(t, handler, "DeleteAlarms", map[string]any{
		"AlarmNames": []string{"high-requests"},
	}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteAlarms status=%d body=%q", del.Code, del.Body.String())
	}
}
