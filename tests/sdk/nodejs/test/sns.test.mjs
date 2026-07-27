import { test } from "node:test";
import assert from "node:assert/strict";
import {
  CreateTopicCommand,
  DeleteTopicCommand,
  ListTopicsCommand,
  PublishCommand,
  SubscribeCommand,
} from "@aws-sdk/client-sns";
import { newSNS, requireReady, uniquePrefix } from "../lib/helpers.mjs";

test("SNS topic publish round-trip", async (t) => {
  if (!(await requireReady(t))) return;
  const client = newSNS();
  const name = `${uniquePrefix()}-topic`;
  const created = await client.send(new CreateTopicCommand({ Name: name }));
  const topicArn = created.TopicArn;
  assert.ok(topicArn, "CreateTopic missing TopicArn");

  t.after(async () => {
    try {
      await client.send(new DeleteTopicCommand({ TopicArn: topicArn }));
    } catch {
      /* ignore */
    }
  });

  const list = await client.send(new ListTopicsCommand({}));
  const arns = (list.Topics || []).map((x) => x.TopicArn);
  assert.ok(arns.includes(topicArn), `ListTopics missing ${topicArn}`);

  const pub = await client.send(
    new PublishCommand({ TopicArn: topicArn, Message: "sdk-sns" }),
  );
  assert.ok(pub.MessageId, "Publish missing MessageId");

  await client.send(new DeleteTopicCommand({ TopicArn: topicArn }));
});

test("SNS HTTP egress deny without opt-in", async (t) => {
  if (!(await requireReady(t))) return;
  if (process.env.NOCTAXRIS_SNS_HTTP_EGRESS === "1") {
    t.skip(
      "NOCTAXRIS_SNS_HTTP_EGRESS=1; deny-by-default assertion soft-skipped",
    );
    return;
  }
  const client = newSNS();
  const name = `${uniquePrefix()}-http-deny`;
  const created = await client.send(new CreateTopicCommand({ Name: name }));
  const topicArn = created.TopicArn;
  assert.ok(topicArn, "CreateTopic missing TopicArn");

  t.after(async () => {
    try {
      await client.send(new DeleteTopicCommand({ TopicArn: topicArn }));
    } catch {
      /* ignore */
    }
  });

  await assert.rejects(
    () =>
      client.send(
        new SubscribeCommand({
          TopicArn: topicArn,
          Protocol: "https",
          Endpoint: "https://example.com/noctaxris-sns-deny",
        }),
      ),
    /InvalidParameter|not allowlisted|HTTP endpoint/i,
  );
});
