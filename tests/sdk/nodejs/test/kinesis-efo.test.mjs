import { test } from "node:test";
import assert from "node:assert/strict";
import {
  CreateStreamCommand,
  DeleteStreamCommand,
  DeregisterStreamConsumerCommand,
  DescribeStreamCommand,
  RegisterStreamConsumerCommand,
  UpdateShardCountCommand,
} from "@aws-sdk/client-kinesis";
import { newKinesis, requireReady, uniquePrefix } from "../lib/helpers.mjs";

test("Kinesis RegisterStreamConsumer and UpdateShardCount", async (t) => {
  if (!(await requireReady(t))) return;
  const client = newKinesis();
  const stream = `${uniquePrefix()}-kinesis`;

  await client.send(
    new CreateStreamCommand({ StreamName: stream, ShardCount: 1 }),
  );
  t.after(async () => {
    try {
      await client.send(new DeleteStreamCommand({ StreamName: stream }));
    } catch {
      /* ignore */
    }
  });

  const desc = await client.send(
    new DescribeStreamCommand({ StreamName: stream }),
  );
  const streamARN = desc.StreamDescription?.StreamARN;
  assert.ok(streamARN);

  const reg = await client.send(
    new RegisterStreamConsumerCommand({
      StreamARN: streamARN,
      ConsumerName: "lab-consumer",
    }),
  );
  assert.ok(reg.Consumer?.ConsumerARN);

  await client.send(
    new UpdateShardCountCommand({
      StreamName: stream,
      TargetShardCount: 2,
      ScalingType: "UNIFORM_SCALING",
    }),
  );
  const after = await client.send(
    new DescribeStreamCommand({ StreamName: stream }),
  );
  assert.equal(after.StreamDescription?.Shards?.length, 2);

  await client.send(
    new DeregisterStreamConsumerCommand({
      ConsumerARN: reg.Consumer.ConsumerARN,
    }),
  );
});
