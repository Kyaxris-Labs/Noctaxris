import { test } from "node:test";
import assert from "node:assert/strict";
import { requireReady, signedJsonTarget, uniquePrefix } from "../lib/helpers.mjs";

test("CloudWatch metrics and alarms smoke", async (t) => {
  if (!(await requireReady(t))) return;

  const now = Math.floor(Date.now() / 1000);
  const start = new Date(Date.now() - 3600_000).toISOString();
  const end = new Date(Date.now() + 60_000).toISOString();
  const ns = `Sdk/MyApp-${uniquePrefix()}`;
  const alarmName = `sdk-alarm-${uniquePrefix()}`;

  const put = await signedJsonTarget(
    "monitoring",
    "GraniteServiceVersion20100801.PutMetricData",
    {
      Namespace: ns,
      MetricData: [
        {
          MetricName: "RequestCount",
          Value: 7,
          Unit: "Count",
          Timestamp: now,
          Dimensions: [{ Name: "Service", Value: "api" }],
        },
      ],
    },
  );
  assert.equal(
    put.status,
    200,
    `PutMetricData status=${put.status} body=${put.body}`,
  );

  const listed = await signedJsonTarget(
    "monitoring",
    "GraniteServiceVersion20100801.ListMetrics",
    { Namespace: ns },
  );
  assert.equal(
    listed.status,
    200,
    `ListMetrics status=${listed.status} body=${listed.body}`,
  );
  assert.equal((listed.json?.Metrics || []).length, 1);

  const stats = await signedJsonTarget(
    "monitoring",
    "GraniteServiceVersion20100801.GetMetricStatistics",
    {
      Namespace: ns,
      MetricName: "RequestCount",
      StartTime: start,
      EndTime: end,
      Period: 60,
      Statistics: ["Sum"],
      Dimensions: [{ Name: "Service", Value: "api" }],
    },
  );
  assert.equal(
    stats.status,
    200,
    `GetMetricStatistics status=${stats.status} body=${stats.body}`,
  );
  assert.ok(
    (stats.json?.Datapoints || []).length > 0,
    `Datapoints empty: ${stats.body}`,
  );
  assert.ok(
    (stats.json.Datapoints[0].Sum ?? 0) >= 7,
    `Sum want >=7 got ${JSON.stringify(stats.json.Datapoints[0])}`,
  );

  const alarm = await signedJsonTarget(
    "monitoring",
    "GraniteServiceVersion20100801.PutMetricAlarm",
    {
      AlarmName: alarmName,
      MetricName: "RequestCount",
      Namespace: ns,
      Statistic: "Sum",
      Period: 60,
      EvaluationPeriods: 1,
      Threshold: 1,
      ComparisonOperator: "GreaterThanThreshold",
      Dimensions: [{ Name: "Service", Value: "api" }],
    },
  );
  assert.equal(
    alarm.status,
    200,
    `PutMetricAlarm status=${alarm.status} body=${alarm.body}`,
  );

  const desc = await signedJsonTarget(
    "monitoring",
    "GraniteServiceVersion20100801.DescribeAlarms",
    { AlarmNames: [alarmName] },
  );
  assert.equal(
    desc.status,
    200,
    `DescribeAlarms status=${desc.status} body=${desc.body}`,
  );
  assert.equal((desc.json?.MetricAlarms || []).length, 1);

  const set = await signedJsonTarget(
    "monitoring",
    "GraniteServiceVersion20100801.SetAlarmState",
    {
      AlarmName: alarmName,
      StateValue: "ALARM",
      StateReason: "sdk smoke",
    },
  );
  assert.equal(
    set.status,
    200,
    `SetAlarmState status=${set.status} body=${set.body}`,
  );

  const after = await signedJsonTarget(
    "monitoring",
    "GraniteServiceVersion20100801.DescribeAlarms",
    { AlarmNames: [alarmName] },
  );
  assert.equal(
    after.status,
    200,
    `DescribeAlarms after SetAlarmState status=${after.status} body=${after.body}`,
  );
  assert.equal(after.json?.MetricAlarms?.[0]?.StateValue, "ALARM");

  const del = await signedJsonTarget(
    "monitoring",
    "GraniteServiceVersion20100801.DeleteAlarms",
    { AlarmNames: [alarmName] },
  );
  assert.equal(
    del.status,
    200,
    `DeleteAlarms status=${del.status} body=${del.body}`,
  );
});
