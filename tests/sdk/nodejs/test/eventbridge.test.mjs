import { test } from "node:test";
import assert from "node:assert/strict";
import {
  CreateEventBusCommand,
  DeleteEventBusCommand,
  DeleteRuleCommand,
  DescribeRuleCommand,
  PutRuleCommand,
} from "@aws-sdk/client-eventbridge";
import { newEvents, requireReady, uniquePrefix } from "../lib/helpers.mjs";

test("EventBridge bus and rule round-trip", async (t) => {
  if (!(await requireReady(t))) return;
  const client = newEvents();
  const prefix = uniquePrefix();
  const busName = `${prefix}-bus`;
  const ruleName = `${prefix}-rule`;

  await client.send(new CreateEventBusCommand({ Name: busName }));
  t.after(async () => {
    try {
      await client.send(new DeleteEventBusCommand({ Name: busName }));
    } catch {
      /* ignore */
    }
  });

  await client.send(
    new PutRuleCommand({
      Name: ruleName,
      EventBusName: busName,
      EventPattern: '{"source":["noctaxris.sdk"]}',
      State: "ENABLED",
    }),
  );
  t.after(async () => {
    try {
      await client.send(
        new DeleteRuleCommand({ Name: ruleName, EventBusName: busName }),
      );
    } catch {
      /* ignore */
    }
  });

  const desc = await client.send(
    new DescribeRuleCommand({ Name: ruleName, EventBusName: busName }),
  );
  assert.equal(desc.Name, ruleName);

  await client.send(
    new DeleteRuleCommand({ Name: ruleName, EventBusName: busName }),
  );
  await client.send(new DeleteEventBusCommand({ Name: busName }));
});
