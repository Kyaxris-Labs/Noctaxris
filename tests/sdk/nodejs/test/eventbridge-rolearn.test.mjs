import { test } from "node:test";
import assert from "node:assert/strict";
import {
  CreateEventBusCommand,
  DeleteEventBusCommand,
  DeleteRuleCommand,
  PutEventsCommand,
  PutRuleCommand,
  PutTargetsCommand,
  RemoveTargetsCommand,
} from "@aws-sdk/client-eventbridge";
import {
  CreateQueueCommand,
  DeleteMessageCommand,
  DeleteQueueCommand,
  GetQueueAttributesCommand,
  ReceiveMessageCommand,
  SetQueueAttributesCommand,
} from "@aws-sdk/client-sqs";
import { GetCallerIdentityCommand } from "@aws-sdk/client-sts";
import {
  newEvents,
  newSQS,
  newSTS,
  requireReady,
  uniquePrefix,
} from "../lib/helpers.mjs";

const EB_SOURCE = "noctaxris.sdk.eb.rolearn";

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

async function drainQueue(sqs, queueUrl) {
  const deadline = Date.now() + 3000;
  while (Date.now() < deadline) {
    const recv = await sqs.send(
      new ReceiveMessageCommand({
        QueueUrl: queueUrl,
        MaxNumberOfMessages: 10,
        WaitTimeSeconds: 1,
        VisibilityTimeout: 0,
      }),
    );
    if (!recv.Messages?.length) return;
    for (const m of recv.Messages) {
      if (!m.ReceiptHandle) continue;
      await sqs.send(
        new DeleteMessageCommand({
          QueueUrl: queueUrl,
          ReceiptHandle: m.ReceiptHandle,
        }),
      );
    }
  }
}

async function receiveOneBody(sqs, queueUrl, timeoutMs) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const recv = await sqs.send(
      new ReceiveMessageCommand({
        QueueUrl: queueUrl,
        MaxNumberOfMessages: 1,
        WaitTimeSeconds: 1,
      }),
    );
    if (!recv.Messages?.length) continue;
    const body = recv.Messages[0].Body || "";
    if (recv.Messages[0].ReceiptHandle) {
      await sqs.send(
        new DeleteMessageCommand({
          QueueUrl: queueUrl,
          ReceiptHandle: recv.Messages[0].ReceiptHandle,
        }),
      );
    }
    return body;
  }
  throw new Error(`ReceiveMessage timed out on ${queueUrl}`);
}

async function receiveOptionalBody(sqs, queueUrl, timeoutMs) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const recv = await sqs.send(
      new ReceiveMessageCommand({
        QueueUrl: queueUrl,
        MaxNumberOfMessages: 1,
        WaitTimeSeconds: 1,
      }),
    );
    if (!recv.Messages?.length) continue;
    const body = recv.Messages[0].Body || "";
    if (recv.Messages[0].ReceiptHandle) {
      await sqs.send(
        new DeleteMessageCommand({
          QueueUrl: queueUrl,
          ReceiptHandle: recv.Messages[0].ReceiptHandle,
        }),
      );
    }
    return body;
  }
  return "";
}

test("EventBridge deliver SQS without RoleArn", async (t) => {
  if (!(await requireReady(t))) return;
  const eb = newEvents();
  const sqs = newSQS();
  const sts = newSTS();
  const prefix = uniquePrefix();

  const caller = await sts.send(new GetCallerIdentityCommand({}));
  const accountID = caller.Account;
  assert.ok(accountID, "GetCallerIdentity missing Account");

  const busName = `${prefix}-bus`;
  const ruleName = `${prefix}-rule`;
  const qName = `${prefix}-eb-q`;

  const qOut = await sqs.send(new CreateQueueCommand({ QueueName: qName }));
  const qURL = qOut.QueueUrl;
  assert.ok(qURL);
  t.after(async () => {
    try {
      await sqs.send(new DeleteQueueCommand({ QueueUrl: qURL }));
    } catch {
      /* ignore */
    }
  });
  const qARN = await queueArn(sqs, qURL);

  await eb.send(new CreateEventBusCommand({ Name: busName }));
  t.after(async () => {
    try {
      await eb.send(new DeleteEventBusCommand({ Name: busName }));
    } catch {
      /* ignore */
    }
  });

  const pattern = JSON.stringify({ source: [EB_SOURCE] });
  await eb.send(
    new PutRuleCommand({
      Name: ruleName,
      EventBusName: busName,
      EventPattern: pattern,
      State: "ENABLED",
    }),
  );
  t.after(async () => {
    try {
      await eb.send(
        new RemoveTargetsCommand({
          Rule: ruleName,
          EventBusName: busName,
          Ids: ["sqs"],
        }),
      );
    } catch {
      /* ignore */
    }
    try {
      await eb.send(
        new DeleteRuleCommand({ Name: ruleName, EventBusName: busName }),
      );
    } catch {
      /* ignore */
    }
  });

  const policy = JSON.stringify({
    Version: "2012-10-17",
    Statement: [
      {
        Effect: "Allow",
        Principal: { Service: "events.amazonaws.com" },
        Action: "sqs:SendMessage",
        Resource: qARN,
        Condition: {
          ArnLike: {
            "aws:SourceArn": `arn:aws:events:us-east-1:${accountID}:rule/${busName}/${ruleName}`,
          },
        },
      },
    ],
  });
  await sqs.send(
    new SetQueueAttributesCommand({
      QueueUrl: qURL,
      Attributes: { Policy: policy },
    }),
  );

  const putTargets = await eb.send(
    new PutTargetsCommand({
      Rule: ruleName,
      EventBusName: busName,
      Targets: [{ Id: "sqs", Arn: qARN }],
    }),
  );
  assert.equal(putTargets.FailedEntryCount || 0, 0);

  await drainQueue(sqs, qURL);
  const marker = `eb-rolearn-${prefix}`;
  const putOut = await eb.send(
    new PutEventsCommand({
      Entries: [
        {
          EventBusName: busName,
          Source: EB_SOURCE,
          DetailType: "RoleArnLessDelivery",
          Detail: JSON.stringify({ marker }),
        },
      ],
    }),
  );
  assert.equal(putOut.FailedEntryCount || 0, 0);

  const body = await receiveOneBody(sqs, qURL, 8000);
  assert.ok(body.includes(EB_SOURCE), `missing source in ${body}`);
  assert.ok(body.includes(marker), `missing marker in ${body}`);
});

test("EventBridge skip SQS without RoleArn or policy", async (t) => {
  if (!(await requireReady(t))) return;
  const eb = newEvents();
  const sqs = newSQS();
  const prefix = uniquePrefix();

  const busName = `${prefix}-bus`;
  const ruleName = `${prefix}-rule`;
  const qName = `${prefix}-deny-q`;

  const qOut = await sqs.send(new CreateQueueCommand({ QueueName: qName }));
  const qURL = qOut.QueueUrl;
  assert.ok(qURL);
  t.after(async () => {
    try {
      await sqs.send(new DeleteQueueCommand({ QueueUrl: qURL }));
    } catch {
      /* ignore */
    }
  });
  const qARN = await queueArn(sqs, qURL);

  await eb.send(new CreateEventBusCommand({ Name: busName }));
  t.after(async () => {
    try {
      await eb.send(new DeleteEventBusCommand({ Name: busName }));
    } catch {
      /* ignore */
    }
  });

  await eb.send(
    new PutRuleCommand({
      Name: ruleName,
      EventBusName: busName,
      EventPattern: JSON.stringify({ source: [EB_SOURCE] }),
      State: "ENABLED",
    }),
  );
  t.after(async () => {
    try {
      await eb.send(
        new RemoveTargetsCommand({
          Rule: ruleName,
          EventBusName: busName,
          Ids: ["deny"],
        }),
      );
    } catch {
      /* ignore */
    }
    try {
      await eb.send(
        new DeleteRuleCommand({ Name: ruleName, EventBusName: busName }),
      );
    } catch {
      /* ignore */
    }
  });

  const putTargets = await eb.send(
    new PutTargetsCommand({
      Rule: ruleName,
      EventBusName: busName,
      Targets: [{ Id: "deny", Arn: qARN }],
    }),
  );
  assert.equal(putTargets.FailedEntryCount || 0, 0);

  await drainQueue(sqs, qURL);
  await eb.send(
    new PutEventsCommand({
      Entries: [
        {
          EventBusName: busName,
          Source: EB_SOURCE,
          DetailType: "RoleArnLessSkip",
          Detail: JSON.stringify({ probe: "no-policy" }),
        },
      ],
    }),
  );
  const msg = await receiveOptionalBody(sqs, qURL, 2000);
  assert.equal(msg, "", `queue without policy unexpectedly received: ${msg}`);
});
