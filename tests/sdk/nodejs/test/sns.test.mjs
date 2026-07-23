import { test } from "node:test";
import assert from "node:assert/strict";
import {
  CreateTopicCommand,
  DeleteTopicCommand,
  ListTopicsCommand,
  PublishCommand,
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
