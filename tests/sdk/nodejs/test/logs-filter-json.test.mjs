import { test } from "node:test";
import assert from "node:assert/strict";
import {
  requireReady,
  signedJsonTarget,
  uniquePrefix,
} from "../lib/helpers.mjs";

const ctAssumeRoleLog =
  '{"eventVersion":"1.08","userIdentity":{"type":"IAMUser","principalId":"AIDACKCEVSQ6C2EXAMPLE","arn":"arn:aws:iam::000000000001:user/alice"},"eventTime":"2026-01-02T03:04:05Z","eventSource":"sts.amazonaws.com","eventName":"AssumeRole","awsRegion":"us-east-1"}';

test("Logs FilterLogEvents JSON eventName AssumeRole", async (t) => {
  if (!(await requireReady(t))) return;
  const prefix = uniquePrefix();
  const group = `/lab/sdk-json-${prefix}`;
  const stream = "s1";
  const ts = Date.now();

  const createGroup = await signedJsonTarget(
    "logs",
    "Logs_20140328.CreateLogGroup",
    { logGroupName: group },
  );
  assert.equal(createGroup.status, 200, createGroup.body);
  t.after(async () => {
    try {
      await signedJsonTarget("logs", "Logs_20140328.DeleteLogStream", {
        logGroupName: group,
        logStreamName: stream,
      });
    } catch {
      /* ignore */
    }
    try {
      await signedJsonTarget("logs", "Logs_20140328.DeleteLogGroup", {
        logGroupName: group,
      });
    } catch {
      /* ignore */
    }
  });

  const createStream = await signedJsonTarget(
    "logs",
    "Logs_20140328.CreateLogStream",
    { logGroupName: group, logStreamName: stream },
  );
  assert.equal(createStream.status, 200, createStream.body);

  const put = await signedJsonTarget("logs", "Logs_20140328.PutLogEvents", {
    logGroupName: group,
    logStreamName: stream,
    logEvents: [
      {
        timestamp: ts,
        message: '{"eventName":"GetObject","eventSource":"s3.amazonaws.com"}',
      },
      { timestamp: ts + 1, message: ctAssumeRoleLog },
      { timestamp: ts + 2, message: "plain text noise" },
    ],
  });
  assert.equal(put.status, 200, put.body);

  const filtered = await signedJsonTarget(
    "logs",
    "Logs_20140328.FilterLogEvents",
    {
      logGroupName: group,
      filterPattern: '{ $.eventName = "AssumeRole" }',
    },
  );
  assert.equal(filtered.status, 200, filtered.body);
  const events = filtered.json?.events || [];
  assert.equal(events.length, 1, filtered.body);
  assert.match(events[0]?.message || "", /"eventName":"AssumeRole"/);
});
