import { test } from "node:test";
import assert from "node:assert/strict";
import {
  ChangeMessageVisibilityCommand,
  CreateQueueCommand,
  DeleteMessageCommand,
  DeleteQueueCommand,
  GetQueueAttributesCommand,
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

test("SQS RedrivePolicy NoctaxrisDlqSourceArn", async (t) => {
  if (!(await requireReady(t))) return;
  const client = newSQS();
  const prefix = uniquePrefix();
  const dlqName = `${prefix}-dlq`;
  const srcName = `${prefix}-src`;

  const dlqCreated = await client.send(
    new CreateQueueCommand({ QueueName: dlqName }),
  );
  const dlqUrl = dlqCreated.QueueUrl;
  assert.ok(dlqUrl, "CreateQueue dlq missing QueueUrl");
  t.after(async () => {
    try {
      await client.send(new DeleteQueueCommand({ QueueUrl: dlqUrl }));
    } catch {
      /* ignore */
    }
  });

  const dlqAttrs = await client.send(
    new GetQueueAttributesCommand({
      QueueUrl: dlqUrl,
      AttributeNames: ["QueueArn"],
    }),
  );
  const dlqArn = dlqAttrs.Attributes?.QueueArn;
  assert.ok(dlqArn, "GetQueueAttributes missing QueueArn");

  const srcCreated = await client.send(
    new CreateQueueCommand({
      QueueName: srcName,
      Attributes: {
        VisibilityTimeout: "0",
        RedrivePolicy: JSON.stringify({
          deadLetterTargetArn: dlqArn,
          maxReceiveCount: "1",
        }),
      },
    }),
  );
  const srcUrl = srcCreated.QueueUrl;
  assert.ok(srcUrl, "CreateQueue src missing QueueUrl");
  t.after(async () => {
    try {
      await client.send(new DeleteQueueCommand({ QueueUrl: srcUrl }));
    } catch {
      /* ignore */
    }
  });

  const srcAttrs = await client.send(
    new GetQueueAttributesCommand({
      QueueUrl: srcUrl,
      AttributeNames: ["QueueArn"],
    }),
  );
  const srcArn = srcAttrs.Attributes?.QueueArn;
  assert.ok(srcArn, "src QueueArn missing");

  await client.send(
    new SendMessageCommand({ QueueUrl: srcUrl, MessageBody: "poison" }),
  );

  const first = await client.send(
    new ReceiveMessageCommand({
      QueueUrl: srcUrl,
      MaxNumberOfMessages: 1,
      WaitTimeSeconds: 1,
    }),
  );
  assert.equal(first.Messages?.length, 1);
  assert.ok(first.Messages[0].ReceiptHandle);

  await client.send(
    new ChangeMessageVisibilityCommand({
      QueueUrl: srcUrl,
      ReceiptHandle: first.Messages[0].ReceiptHandle,
      VisibilityTimeout: 0,
    }),
  );

  const second = await client.send(
    new ReceiveMessageCommand({
      QueueUrl: srcUrl,
      MaxNumberOfMessages: 1,
      WaitTimeSeconds: 1,
    }),
  );
  assert.equal(second.Messages?.length || 0, 0);

  const dlqRecv = await client.send(
    new ReceiveMessageCommand({
      QueueUrl: dlqUrl,
      MaxNumberOfMessages: 1,
      WaitTimeSeconds: 1,
      MessageAttributeNames: ["All"],
    }),
  );
  assert.equal(dlqRecv.Messages?.length, 1);
  assert.equal(dlqRecv.Messages[0].Body, "poison");
  const prov = dlqRecv.Messages[0].MessageAttributes?.NoctaxrisDlqSourceArn;
  assert.ok(prov, `missing NoctaxrisDlqSourceArn: ${JSON.stringify(dlqRecv.Messages[0].MessageAttributes)}`);
  assert.equal(prov.StringValue, srcArn);
  assert.equal(prov.DataType, "String");
});
