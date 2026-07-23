import { test } from "node:test";
import assert from "node:assert/strict";
import {
  CreateQueueCommand,
  DeleteMessageCommand,
  DeleteQueueCommand,
  ReceiveMessageCommand,
  SendMessageCommand,
} from "@aws-sdk/client-sqs";
import { newSQS, requireReady, uniquePrefix } from "../lib/helpers.mjs";

test("SQS send receive delete round-trip", async (t) => {
  if (!(await requireReady(t))) return;
  const client = newSQS();
  const name = `${uniquePrefix()}-q`;
  const created = await client.send(new CreateQueueCommand({ QueueName: name }));
  const queueUrl = created.QueueUrl;
  assert.ok(queueUrl, "CreateQueue missing QueueUrl");

  t.after(async () => {
    try {
      await client.send(new DeleteQueueCommand({ QueueUrl: queueUrl }));
    } catch {
      /* ignore */
    }
  });

  await client.send(
    new SendMessageCommand({
      QueueUrl: queueUrl,
      MessageBody: "sdk-sqs-body",
    }),
  );
  const recv = await client.send(
    new ReceiveMessageCommand({
      QueueUrl: queueUrl,
      MaxNumberOfMessages: 1,
      WaitTimeSeconds: 1,
    }),
  );
  assert.equal(recv.Messages?.length, 1);
  assert.equal(recv.Messages[0].Body, "sdk-sqs-body");
  assert.ok(recv.Messages[0].ReceiptHandle, "missing ReceiptHandle");

  await client.send(
    new DeleteMessageCommand({
      QueueUrl: queueUrl,
      ReceiptHandle: recv.Messages[0].ReceiptHandle,
    }),
  );
  await client.send(new DeleteQueueCommand({ QueueUrl: queueUrl }));
});
