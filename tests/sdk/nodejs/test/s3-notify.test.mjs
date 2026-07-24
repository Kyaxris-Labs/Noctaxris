import { test } from "node:test";
import assert from "node:assert/strict";
import {
  CreateBucketCommand,
  DeleteBucketCommand,
  DeleteObjectCommand,
  PutBucketNotificationConfigurationCommand,
  PutObjectCommand,
} from "@aws-sdk/client-s3";
import {
  CreateQueueCommand,
  DeleteQueueCommand,
  GetQueueAttributesCommand,
  ReceiveMessageCommand,
  SetQueueAttributesCommand,
} from "@aws-sdk/client-sqs";
import { newS3, newSQS, requireReady, uniquePrefix } from "../lib/helpers.mjs";

async function queueArn(sqs, queueUrl) {
  const attrs = await sqs.send(
    new GetQueueAttributesCommand({
      QueueUrl: queueUrl,
      AttributeNames: ["QueueArn"],
    }),
  );
  const arn = attrs.Attributes?.QueueArn;
  assert.ok(arn, "GetQueueAttributes missing QueueArn");
  return arn;
}

test("S3 notification PutObject to SQS", async (t) => {
  if (!(await requireReady(t))) return;
  const s3 = newS3();
  const sqs = newSQS();
  const prefix = uniquePrefix();
  const bucket = `sdk-s3n-${prefix}`.toLowerCase();
  const qName = `sdk-s3n-q-${prefix}`;
  const key = "notify.txt";

  await s3.send(new CreateBucketCommand({ Bucket: bucket }));
  t.after(async () => {
    try {
      await s3.send(new DeleteObjectCommand({ Bucket: bucket, Key: key }));
    } catch {
      /* ignore */
    }
    try {
      await s3.send(new DeleteBucketCommand({ Bucket: bucket }));
    } catch {
      /* ignore */
    }
  });

  const qOut = await sqs.send(new CreateQueueCommand({ QueueName: qName }));
  const qURL = qOut.QueueUrl;
  assert.ok(qURL, "CreateQueue missing QueueUrl");
  t.after(async () => {
    try {
      await sqs.send(new DeleteQueueCommand({ QueueUrl: qURL }));
    } catch {
      /* ignore */
    }
  });

  const qARN = await queueArn(sqs, qURL);
  const bucketARN = `arn:aws:s3:::${bucket}`;
  const policy = JSON.stringify({
    Version: "2012-10-17",
    Statement: [
      {
        Effect: "Allow",
        Principal: { Service: "s3.amazonaws.com" },
        Action: "sqs:SendMessage",
        Resource: qARN,
        Condition: { ArnLike: { "aws:SourceArn": bucketARN } },
      },
    ],
  });
  await sqs.send(
    new SetQueueAttributesCommand({
      QueueUrl: qURL,
      Attributes: { Policy: policy },
    }),
  );

  await s3.send(
    new PutBucketNotificationConfigurationCommand({
      Bucket: bucket,
      NotificationConfiguration: {
        QueueConfigurations: [
          {
            Id: "q1",
            QueueArn: qARN,
            Events: ["s3:ObjectCreated:*"],
          },
        ],
      },
    }),
  );

  await s3.send(
    new PutObjectCommand({
      Bucket: bucket,
      Key: key,
      Body: Buffer.from("hello-notify"),
    }),
  );

  const deadline = Date.now() + 10_000;
  let body = "";
  while (Date.now() < deadline) {
    const recv = await sqs.send(
      new ReceiveMessageCommand({
        QueueUrl: qURL,
        MaxNumberOfMessages: 1,
        WaitTimeSeconds: 1,
      }),
    );
    if (!recv.Messages?.length) continue;
    body = recv.Messages[0].Body || "";
    break;
  }
  assert.ok(body, "expected S3 notification message on SQS");
  assert.match(body, /"eventSource":"aws:s3"/);
  assert.ok(body.includes(bucket), `want bucket in body=${body}`);
  assert.ok(body.includes(key), `want key in body=${body}`);
});

test("S3 notification deny without queue policy", async (t) => {
  if (!(await requireReady(t))) return;
  const s3 = newS3();
  const sqs = newSQS();
  const prefix = uniquePrefix();
  const bucket = `sdk-s3n-deny-${prefix}`.toLowerCase();
  const qName = `sdk-s3n-deny-q-${prefix}`;
  const key = "deny.txt";

  await s3.send(new CreateBucketCommand({ Bucket: bucket }));
  t.after(async () => {
    try {
      await s3.send(new DeleteObjectCommand({ Bucket: bucket, Key: key }));
    } catch {
      /* ignore */
    }
    try {
      await s3.send(new DeleteBucketCommand({ Bucket: bucket }));
    } catch {
      /* ignore */
    }
  });

  const qOut = await sqs.send(new CreateQueueCommand({ QueueName: qName }));
  const qURL = qOut.QueueUrl;
  assert.ok(qURL, "CreateQueue missing QueueUrl");
  t.after(async () => {
    try {
      await sqs.send(new DeleteQueueCommand({ QueueUrl: qURL }));
    } catch {
      /* ignore */
    }
  });

  const qARN = await queueArn(sqs, qURL);
  const bucketARN = `arn:aws:s3:::${bucket}`;
  const policy = JSON.stringify({
    Version: "2012-10-17",
    Statement: [
      {
        Effect: "Allow",
        Principal: { Service: "s3.amazonaws.com" },
        Action: "sqs:SendMessage",
        Resource: qARN,
        Condition: { ArnLike: { "aws:SourceArn": bucketARN } },
      },
    ],
  });
  await sqs.send(
    new SetQueueAttributesCommand({
      QueueUrl: qURL,
      Attributes: { Policy: policy },
    }),
  );

  await s3.send(
    new PutBucketNotificationConfigurationCommand({
      Bucket: bucket,
      NotificationConfiguration: {
        QueueConfigurations: [
          {
            QueueArn: qARN,
            Events: ["s3:ObjectCreated:Put"],
          },
        ],
      },
    }),
  );

  await sqs.send(
    new SetQueueAttributesCommand({
      QueueUrl: qURL,
      Attributes: { Policy: "" },
    }),
  );

  await s3.send(
    new PutObjectCommand({
      Bucket: bucket,
      Key: key,
      Body: Buffer.from("no-notify"),
    }),
  );

  const recv = await sqs.send(
    new ReceiveMessageCommand({
      QueueUrl: qURL,
      MaxNumberOfMessages: 10,
      WaitTimeSeconds: 2,
    }),
  );
  assert.equal(
    recv.Messages?.length || 0,
    0,
    "emit must re-check queue policy",
  );
});
