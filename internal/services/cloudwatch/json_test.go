package cloudwatch_test

import (
	"encoding/json"
	"testing"

	cloudwatchsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/cloudwatch"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestCloudWatchJSON(t *testing.T) {
	if _, err := cloudwatchsvc.PutMetricDataJSON(); err != nil {
		t.Fatal(err)
	}

	metrics := []store.CWMetricIdentity{{
		Namespace: "AWS/Lambda", MetricName: "Invocations",
		Dimensions: []store.CWDimension{{Name: "FunctionName", Value: "f"}},
	}}
	raw, err := cloudwatchsvc.ListMetricsJSON(metrics)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	_, _ = cloudwatchsvc.ListMetricsJSON(nil)

	dps := []store.CWDatapoint{
		{Timestamp: 1_700_000_000, Unit: "Count", Average: 1, Sum: 2, Minimum: 0, Maximum: 3, SampleCount: 4},
	}
	raw, _ = cloudwatchsvc.GetMetricStatisticsJSON("lbl", []string{"Sum", "Minimum", "Maximum", "SampleCount"}, dps)
	_ = json.Unmarshal(raw, &out)
	raw, _ = cloudwatchsvc.GetMetricStatisticsJSON("lbl", nil, dps)

	mdr := []store.CWMetricDataResult{{
		ID: "m1", Label: "l", Timestamps: []int64{1_700_000_000}, Values: []float64{1}, StatusCode: "Complete",
	}}
	raw, _ = cloudwatchsvc.GetMetricDataJSON(mdr)

	if _, err := cloudwatchsvc.PutMetricAlarmJSON(); err != nil {
		t.Fatal(err)
	}

	alarm := store.CWMetricAlarm{
		AlarmName: "a", AlarmArn: "arn:a", AlarmDescription: "d", MetricName: "m", Namespace: "n",
		Statistic: "Average", Period: 60, EvaluationPeriods: 1, Threshold: 1, ComparisonOperator: "GreaterThanThreshold",
		Dimensions: []store.CWDimension{{Name: "k", Value: "v"}},
		ActionsEnabled: true, StateValue: "OK", StateReason: "r", StateReasonData: "{}",
		StateUpdatedAt: 1_700_000_000, ConfigUpdatedAt: 1_700_000_000,
	}
	raw, _ = cloudwatchsvc.DescribeAlarmsJSON([]store.CWMetricAlarm{alarm})
	_ = json.Unmarshal(raw, &out)

	_, _ = cloudwatchsvc.DeleteAlarmsJSON()
	_, _ = cloudwatchsvc.SetAlarmStateJSON()
}
