package store_test

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func openCWStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	key, err := store.LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestCloudWatchMetricsAndAlarmsRoundTrip(t *testing.T) {
	st := openCWStore(t)
	account := "000000000001"
	region := "us-east-1"
	now := time.Now().UTC().Unix()

	err := st.PutMetricData(account, region, "MyApp", []store.CWMetricDatum{
		{
			MetricName: "RequestCount",
			Value:      10,
			Unit:       "Count",
			Timestamp:  now - 30,
			Dimensions: []store.CWDimension{{Name: "Service", Value: "api"}},
		},
		{
			MetricName: "RequestCount",
			Value:      5,
			Unit:       "Count",
			Timestamp:  now - 10,
			Dimensions: []store.CWDimension{{Name: "Service", Value: "api"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	metrics, err := st.ListMetrics(account, region, "MyApp", "RequestCount", []store.CWDimension{
		{Name: "Service", Value: "api"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(metrics) != 1 {
		t.Fatalf("ListMetrics=%+v", metrics)
	}

	stats, err := st.GetMetricStatistics(
		account, region, "MyApp", "RequestCount",
		[]store.CWDimension{{Name: "Service", Value: "api"}},
		now-3600, now+60, 60, "",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) == 0 {
		t.Fatal("expected statistics datapoints")
	}
	var sum float64
	for _, dp := range stats {
		sum += dp.Sum
	}
	if sum != 15 {
		t.Fatalf("sum=%v want 15 stats=%+v", sum, stats)
	}

	results, err := st.GetCloudWatchMetricData(account, region, []store.CWMetricDataQuery{{
		ID: "m1", ReturnData: true, Label: "requests",
		MetricStat: &store.CWMetricStat{
			Namespace:  "MyApp",
			MetricName: "RequestCount",
			Dimensions: []store.CWDimension{{Name: "Service", Value: "api"}},
			Period:     60,
			Stat:       "Sum",
		},
	}}, now-3600, now+60)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].ID != "m1" || len(results[0].Values) == 0 {
		t.Fatalf("GetCloudWatchMetricData=%+v", results)
	}

	alarm, err := st.PutMetricAlarm(account, region, store.CWMetricAlarm{
		AlarmName:          "high-requests",
		MetricName:         "RequestCount",
		Namespace:          "MyApp",
		Statistic:          "Sum",
		Period:             60,
		EvaluationPeriods:  1,
		Threshold:          10,
		ComparisonOperator: "GreaterThanThreshold",
		ActionsEnabled:     true,
		Dimensions:         []store.CWDimension{{Name: "Service", Value: "api"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if alarm.AlarmArn == "" || alarm.StateValue != "OK" {
		t.Fatalf("alarm=%+v", alarm)
	}

	listed, err := st.DescribeAlarms(account, region, nil, "high-")
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 {
		t.Fatalf("DescribeAlarms=%+v", listed)
	}

	if err := st.SetAlarmState(account, region, "high-requests", "ALARM", "manual", `{"lab":true}`); err != nil {
		t.Fatal(err)
	}
	got, err := st.DescribeAlarm(account, region, "high-requests")
	if err != nil {
		t.Fatal(err)
	}
	if got.StateValue != "ALARM" || got.StateReason != "manual" {
		t.Fatalf("state=%+v", got)
	}

	if err := st.DeleteAlarms(account, region, []string{"high-requests"}); err != nil {
		t.Fatal(err)
	}
	_, err = st.DescribeAlarm(account, region, "high-requests")
	if !errors.Is(err, store.ErrCWAlarmNotFound) {
		t.Fatalf("err=%v want not found", err)
	}
}

func TestCloudWatchPutMetricDataValidation(t *testing.T) {
	st := openCWStore(t)
	err := st.PutMetricData("000000000001", "us-east-1", "", nil)
	if !errors.Is(err, store.ErrCWMissingParameter) {
		t.Fatalf("err=%v", err)
	}
}
