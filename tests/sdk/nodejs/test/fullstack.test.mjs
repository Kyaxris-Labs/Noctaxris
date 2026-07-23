import { test } from "node:test";
import assert from "node:assert/strict";
import {
  CreateTableCommand,
  DeleteTableCommand,
  GetItemCommand,
  PutItemCommand,
} from "@aws-sdk/client-dynamodb";
import {
  CreateEventBusCommand,
  DeleteEventBusCommand,
  DeleteRuleCommand,
  ListTargetsByRuleCommand,
  PutEventsCommand,
  PutRuleCommand,
  PutTargetsCommand,
  RemoveTargetsCommand,
} from "@aws-sdk/client-eventbridge";
import {
  CreateRoleCommand,
  DeleteRoleCommand,
  DeleteRolePolicyCommand,
  GetRoleCommand,
  PutRolePolicyCommand,
} from "@aws-sdk/client-iam";
import {
  CreateKeyCommand,
  PutKeyPolicyCommand,
  ScheduleKeyDeletionCommand,
} from "@aws-sdk/client-kms";
import {
  CreateFunctionCommand,
  DeleteFunctionCommand,
  GetFunctionCommand,
} from "@aws-sdk/client-lambda";
import {
  CreateBucketCommand,
  DeleteBucketCommand,
  DeleteObjectCommand,
  GetObjectCommand,
  PutBucketEncryptionCommand,
  PutObjectCommand,
} from "@aws-sdk/client-s3";
import {
  CreateSecretCommand,
  DeleteSecretCommand,
  GetSecretValueCommand,
} from "@aws-sdk/client-secrets-manager";
import {
  CreateTopicCommand,
  DeleteTopicCommand,
  PublishCommand,
  SetTopicAttributesCommand,
  SubscribeCommand,
  UnsubscribeCommand,
} from "@aws-sdk/client-sns";
import {
  CreateQueueCommand,
  DeleteMessageCommand,
  DeleteQueueCommand,
  GetQueueAttributesCommand,
  ReceiveMessageCommand,
  SetQueueAttributesCommand,
} from "@aws-sdk/client-sqs";
import {
  DeleteParameterCommand,
  GetParameterCommand,
  PutParameterCommand,
} from "@aws-sdk/client-ssm";
import { GetCallerIdentityCommand } from "@aws-sdk/client-sts";
import {
  newDDB,
  newEvents,
  newIAM,
  newKMS,
  newLambda,
  newS3,
  newSecrets,
  newSNS,
  newSQS,
  newSSM,
  newSTS,
  requireReady,
  uniquePrefix,
} from "../lib/helpers.mjs";

const LAMBDA_TRUST =
  '{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"}]}';

const ORDER_BODY = Buffer.from(
  JSON.stringify({ orderId: "ord-1001", sku: "widget", qty: 2 }),
);
const ORDER_ID = "ord-1001";
const CONFIG_VALUE = "lab-config-v1";
const SECURE_VALUE = "lab-secure-config";
const SECRET_VALUE = "lab-api-key-42";
const EVENT_SOURCE = "noctaxris.lab.orders";

function crc32(buf) {
  let c = 0xffffffff;
  for (let i = 0; i < buf.length; i++) {
    c ^= buf[i];
    for (let k = 0; k < 8; k++) {
      c = c & 1 ? (c >>> 1) ^ 0xedb88320 : c >>> 1;
    }
  }
  return (c ^ 0xffffffff) >>> 0;
}

function minimalPythonZip() {
  const name = Buffer.from("index.py");
  const data = Buffer.from(
    "def handler(event, context):\n    return {'ok': True}\n",
  );
  const crc = crc32(data);
  const local = Buffer.alloc(30 + name.length);
  local.writeUInt32LE(0x04034b50, 0);
  local.writeUInt16LE(20, 4);
  local.writeUInt16LE(0, 6);
  local.writeUInt16LE(0, 8);
  local.writeUInt16LE(0, 10);
  local.writeUInt16LE(0, 12);
  local.writeUInt32LE(crc >>> 0, 14);
  local.writeUInt32LE(data.length, 18);
  local.writeUInt32LE(data.length, 22);
  local.writeUInt16LE(name.length, 26);
  local.writeUInt16LE(0, 28);
  name.copy(local, 30);

  const central = Buffer.alloc(46 + name.length);
  central.writeUInt32LE(0x02014b50, 0);
  central.writeUInt16LE(20, 4);
  central.writeUInt16LE(20, 6);
  central.writeUInt16LE(0, 8);
  central.writeUInt16LE(0, 10);
  central.writeUInt16LE(0, 12);
  central.writeUInt16LE(0, 14);
  central.writeUInt32LE(crc >>> 0, 16);
  central.writeUInt32LE(data.length, 20);
  central.writeUInt32LE(data.length, 24);
  central.writeUInt16LE(name.length, 28);
  central.writeUInt16LE(0, 30);
  central.writeUInt16LE(0, 32);
  central.writeUInt16LE(0, 34);
  central.writeUInt16LE(0, 36);
  central.writeUInt32LE(0, 38);
  central.writeUInt32LE(0, 42);
  name.copy(central, 46);

  const eocd = Buffer.alloc(22);
  eocd.writeUInt32LE(0x06054b50, 0);
  eocd.writeUInt16LE(0, 4);
  eocd.writeUInt16LE(0, 6);
  eocd.writeUInt16LE(1, 8);
  eocd.writeUInt16LE(1, 10);
  eocd.writeUInt32LE(central.length, 12);
  eocd.writeUInt32LE(local.length + data.length, 16);
  eocd.writeUInt16LE(0, 20);

  return Buffer.concat([local, data, central, eocd]);
}

async function receiveOne(sqs, queueUrl, { maxAttempts = 8, wait = 1 } = {}) {
  for (let i = 0; i < maxAttempts; i++) {
    const recv = await sqs.send(
      new ReceiveMessageCommand({
        QueueUrl: queueUrl,
        MaxNumberOfMessages: 1,
        WaitTimeSeconds: wait,
      }),
    );
    if (recv.Messages?.length) {
      return recv.Messages[0];
    }
  }
  return null;
}

async function drainQueue(sqs, queueUrl) {
  for (let i = 0; i < 20; i++) {
    const recv = await sqs.send(
      new ReceiveMessageCommand({
        QueueUrl: queueUrl,
        MaxNumberOfMessages: 10,
        WaitTimeSeconds: 0,
      }),
    );
    const msgs = recv.Messages || [];
    if (!msgs.length) return;
    for (const m of msgs) {
      await sqs.send(
        new DeleteMessageCommand({
          QueueUrl: queueUrl,
          ReceiptHandle: m.ReceiptHandle,
        }),
      );
    }
  }
}

/**
 * Full-stack lab order pipeline.
 * Gate: NOCTAXRIS_ADVANCED=1
 */
test("fullstack lab order pipeline", async (t) => {
  if (process.env.NOCTAXRIS_ADVANCED !== "1") {
    t.skip("set NOCTAXRIS_ADVANCED=1 to run advanced fullstack suite");
    return;
  }
  if (!(await requireReady(t))) return;

  const prefix = uniquePrefix();
  const sts = newSTS();
  const kms = newKMS();
  const iam = newIAM();
  const s3 = newS3();
  const ddb = newDDB();
  const sqs = newSQS();
  const sns = newSNS();
  const ssm = newSSM();
  const secrets = newSecrets();
  const lam = newLambda();
  const events = newEvents();

  const ctx = {
    prefix,
    account: "",
    keyId: "",
    roleName: "",
    roleArn: "",
    bucket: "",
    table: "",
    ebQueueUrl: "",
    ebQueueArn: "",
    fanoutQueueUrl: "",
    fanoutQueueArn: "",
    topicArn: "",
    subscriptionArn: "",
    stringParam: "",
    secureParam: "",
    secretName: "",
    secretArn: "",
    fnName: "",
    busName: "",
    ruleName: "",
  };

  await t.test("provision", async () => {
    const ident = await sts.send(new GetCallerIdentityCommand({}));
    ctx.account = ident.Account;
    assert.ok(ctx.account, "GetCallerIdentity missing Account");

    const keyOut = await kms.send(
      new CreateKeyCommand({ Description: `${prefix}-lab-cmk` }),
    );
    ctx.keyId = keyOut.KeyMetadata?.KeyId;
    assert.ok(ctx.keyId, "CreateKey missing KeyId");
    t.after(async () => {
      try {
        await kms.send(
          new ScheduleKeyDeletionCommand({
            KeyId: ctx.keyId,
            PendingWindowInDays: 7,
          }),
        );
      } catch {
        /* ignore */
      }
    });

    const keyPolicy = JSON.stringify({
      Version: "2012-10-17",
      Statement: [
        {
          Sid: "RootAdmin",
          Effect: "Allow",
          Principal: { AWS: `arn:aws:iam::${ctx.account}:root` },
          Action: "kms:*",
          Resource: "*",
        },
      ],
    });
    await kms.send(
      new PutKeyPolicyCommand({
        KeyId: ctx.keyId,
        PolicyName: "default",
        Policy: keyPolicy,
      }),
    );

    ctx.roleName = `${prefix}-lambda`;
    const roleOut = await iam.send(
      new CreateRoleCommand({
        RoleName: ctx.roleName,
        AssumeRolePolicyDocument: LAMBDA_TRUST,
      }),
    );
    ctx.roleArn = roleOut.Role?.Arn;
    assert.ok(ctx.roleArn, "CreateRole missing Arn");
    t.after(async () => {
      try {
        await iam.send(
          new DeleteRolePolicyCommand({
            RoleName: ctx.roleName,
            PolicyName: "lab-access",
          }),
        );
      } catch {
        /* ignore */
      }
      try {
        await iam.send(new DeleteRoleCommand({ RoleName: ctx.roleName }));
      } catch {
        /* ignore */
      }
    });

    const rolePolicy = JSON.stringify({
      Version: "2012-10-17",
      Statement: [
        {
          Effect: "Allow",
          Action: [
            "s3:GetObject",
            "s3:PutObject",
            "dynamodb:GetItem",
            "dynamodb:PutItem",
            "ssm:GetParameter",
            "ssm:GetParameters",
            "secretsmanager:GetSecretValue",
            "kms:Decrypt",
            "kms:Encrypt",
            "logs:CreateLogGroup",
            "logs:CreateLogStream",
            "logs:PutLogEvents",
          ],
          Resource: "*",
        },
      ],
    });
    await iam.send(
      new PutRolePolicyCommand({
        RoleName: ctx.roleName,
        PolicyName: "lab-access",
        PolicyDocument: rolePolicy,
      }),
    );
    const gotRole = await iam.send(
      new GetRoleCommand({ RoleName: ctx.roleName }),
    );
    assert.ok(gotRole.Role?.AssumeRolePolicyDocument, "role trust missing");

    ctx.bucket = `${prefix}-orders`.toLowerCase();
    await s3.send(new CreateBucketCommand({ Bucket: ctx.bucket }));
    t.after(async () => {
      try {
        await s3.send(
          new DeleteObjectCommand({ Bucket: ctx.bucket, Key: "order.json" }),
        );
      } catch {
        /* ignore */
      }
      try {
        await s3.send(new DeleteBucketCommand({ Bucket: ctx.bucket }));
      } catch {
        /* ignore */
      }
    });
    await s3.send(
      new PutBucketEncryptionCommand({
        Bucket: ctx.bucket,
        ServerSideEncryptionConfiguration: {
          Rules: [
            {
              ApplyServerSideEncryptionByDefault: {
                SSEAlgorithm: "aws:kms",
                KMSMasterKeyID: ctx.keyId,
              },
            },
          ],
        },
      }),
    );
    await s3.send(
      new PutObjectCommand({
        Bucket: ctx.bucket,
        Key: "order.json",
        Body: ORDER_BODY,
      }),
    );

    ctx.table = `${prefix}-orders`;
    await ddb.send(
      new CreateTableCommand({
        TableName: ctx.table,
        AttributeDefinitions: [{ AttributeName: "orderId", AttributeType: "S" }],
        KeySchema: [{ AttributeName: "orderId", KeyType: "HASH" }],
        BillingMode: "PAY_PER_REQUEST",
      }),
    );
    t.after(async () => {
      try {
        await ddb.send(new DeleteTableCommand({ TableName: ctx.table }));
      } catch {
        /* ignore */
      }
    });
    await ddb.send(
      new PutItemCommand({
        TableName: ctx.table,
        Item: {
          orderId: { S: ORDER_ID },
          sku: { S: "widget" },
          qty: { N: "2" },
        },
      }),
    );

    const ebQ = await sqs.send(
      new CreateQueueCommand({ QueueName: `${prefix}-eb-q` }),
    );
    ctx.ebQueueUrl = ebQ.QueueUrl;
    assert.ok(ctx.ebQueueUrl);
    t.after(async () => {
      try {
        await sqs.send(new DeleteQueueCommand({ QueueUrl: ctx.ebQueueUrl }));
      } catch {
        /* ignore */
      }
    });
    const ebAttrs = await sqs.send(
      new GetQueueAttributesCommand({
        QueueUrl: ctx.ebQueueUrl,
        AttributeNames: ["QueueArn"],
      }),
    );
    ctx.ebQueueArn = ebAttrs.Attributes?.QueueArn;
    assert.ok(ctx.ebQueueArn);
    await sqs.send(
      new SetQueueAttributesCommand({
        QueueUrl: ctx.ebQueueUrl,
        Attributes: {
          Policy: JSON.stringify({
            Version: "2012-10-17",
            Statement: [
              {
                Effect: "Allow",
                Principal: { Service: "events.amazonaws.com" },
                Action: "sqs:SendMessage",
                Resource: ctx.ebQueueArn,
                Condition: {
                  ArnEquals: {
                    "aws:SourceArn": `arn:aws:events:us-east-1:${ctx.account}:rule/${prefix}-bus/${prefix}-rule`,
                  },
                },
              },
            ],
          }),
        },
      }),
    );

    const fanQ = await sqs.send(
      new CreateQueueCommand({ QueueName: `${prefix}-sns-q` }),
    );
    ctx.fanoutQueueUrl = fanQ.QueueUrl;
    assert.ok(ctx.fanoutQueueUrl);
    t.after(async () => {
      try {
        await sqs.send(
          new DeleteQueueCommand({ QueueUrl: ctx.fanoutQueueUrl }),
        );
      } catch {
        /* ignore */
      }
    });
    const fanAttrs = await sqs.send(
      new GetQueueAttributesCommand({
        QueueUrl: ctx.fanoutQueueUrl,
        AttributeNames: ["QueueArn"],
      }),
    );
    ctx.fanoutQueueArn = fanAttrs.Attributes?.QueueArn;
    assert.ok(ctx.fanoutQueueArn);
    await sqs.send(
      new SetQueueAttributesCommand({
        QueueUrl: ctx.fanoutQueueUrl,
        Attributes: {
          Policy: JSON.stringify({
            Version: "2012-10-17",
            Statement: [
              {
                Effect: "Allow",
                Principal: { Service: "sns.amazonaws.com" },
                Action: "sqs:SendMessage",
                Resource: ctx.fanoutQueueArn,
              },
            ],
          }),
        },
      }),
    );

    const topic = await sns.send(
      new CreateTopicCommand({ Name: `${prefix}-orders` }),
    );
    ctx.topicArn = topic.TopicArn;
    assert.ok(ctx.topicArn);
    t.after(async () => {
      try {
        await sns.send(new DeleteTopicCommand({ TopicArn: ctx.topicArn }));
      } catch {
        /* ignore */
      }
    });
    await sns.send(
      new SetTopicAttributesCommand({
        TopicArn: ctx.topicArn,
        AttributeName: "Policy",
        AttributeValue: JSON.stringify({
          Version: "2012-10-17",
          Statement: [
            {
              Effect: "Allow",
              Principal: { Service: "events.amazonaws.com" },
              Action: "sns:Publish",
              Resource: ctx.topicArn,
            },
          ],
        }),
      }),
    );
    const sub = await sns.send(
      new SubscribeCommand({
        TopicArn: ctx.topicArn,
        Protocol: "sqs",
        Endpoint: ctx.fanoutQueueArn,
      }),
    );
    ctx.subscriptionArn = sub.SubscriptionArn;
    t.after(async () => {
      if (!ctx.subscriptionArn) return;
      try {
        await sns.send(
          new UnsubscribeCommand({ SubscriptionArn: ctx.subscriptionArn }),
        );
      } catch {
        /* ignore */
      }
    });

    ctx.stringParam = `/lab/${prefix}/config`;
    ctx.secureParam = `/lab/${prefix}/secure`;
    await ssm.send(
      new PutParameterCommand({
        Name: ctx.stringParam,
        Value: CONFIG_VALUE,
        Type: "String",
      }),
    );
    t.after(async () => {
      try {
        await ssm.send(
          new DeleteParameterCommand({ Name: ctx.stringParam }),
        );
      } catch {
        /* ignore */
      }
    });
    await ssm.send(
      new PutParameterCommand({
        Name: ctx.secureParam,
        Value: SECURE_VALUE,
        Type: "SecureString",
        KeyId: ctx.keyId,
      }),
    );
    t.after(async () => {
      try {
        await ssm.send(
          new DeleteParameterCommand({ Name: ctx.secureParam }),
        );
      } catch {
        /* ignore */
      }
    });

    ctx.secretName = `${prefix}-api-key`;
    const secretOut = await secrets.send(
      new CreateSecretCommand({
        Name: ctx.secretName,
        SecretString: SECRET_VALUE,
        KmsKeyId: ctx.keyId,
      }),
    );
    ctx.secretArn = secretOut.ARN;
    assert.ok(ctx.secretArn);
    t.after(async () => {
      try {
        await secrets.send(
          new DeleteSecretCommand({
            SecretId: ctx.secretName,
            ForceDeleteWithoutRecovery: true,
          }),
        );
      } catch {
        /* ignore */
      }
    });

    ctx.fnName = `${prefix}-fn`;
    await lam.send(
      new CreateFunctionCommand({
        FunctionName: ctx.fnName,
        Runtime: "python3.12",
        Role: ctx.roleArn,
        Handler: "index.handler",
        Code: { ZipFile: minimalPythonZip() },
      }),
    );
    t.after(async () => {
      try {
        await lam.send(new DeleteFunctionCommand({ FunctionName: ctx.fnName }));
      } catch {
        /* ignore */
      }
    });
    const fn = await lam.send(
      new GetFunctionCommand({ FunctionName: ctx.fnName }),
    );
    assert.equal(fn.Configuration?.Role, ctx.roleArn);
  });

  await t.test("wire_eventbridge", async () => {
    ctx.busName = `${prefix}-bus`;
    ctx.ruleName = `${prefix}-rule`;
    await events.send(new CreateEventBusCommand({ Name: ctx.busName }));
    t.after(async () => {
      try {
        await events.send(new DeleteEventBusCommand({ Name: ctx.busName }));
      } catch {
        /* ignore */
      }
    });

    await events.send(
      new PutRuleCommand({
        Name: ctx.ruleName,
        EventBusName: ctx.busName,
        EventPattern: JSON.stringify({ source: [EVENT_SOURCE] }),
        State: "ENABLED",
      }),
    );
    t.after(async () => {
      try {
        await events.send(
          new RemoveTargetsCommand({
            Rule: ctx.ruleName,
            EventBusName: ctx.busName,
            Ids: ["sqs", "sns"],
          }),
        );
      } catch {
        /* ignore */
      }
      try {
        await events.send(
          new DeleteRuleCommand({
            Name: ctx.ruleName,
            EventBusName: ctx.busName,
          }),
        );
      } catch {
        /* ignore */
      }
    });

    await events.send(
      new PutTargetsCommand({
        Rule: ctx.ruleName,
        EventBusName: ctx.busName,
        Targets: [
          { Id: "sqs", Arn: ctx.ebQueueArn },
          { Id: "sns", Arn: ctx.topicArn },
        ],
      }),
    );
    const listed = await events.send(
      new ListTargetsByRuleCommand({
        Rule: ctx.ruleName,
        EventBusName: ctx.busName,
      }),
    );
    const arns = (listed.Targets || []).map((x) => x.Arn);
    assert.ok(arns.includes(ctx.ebQueueArn), "SQS target missing");
    assert.ok(arns.includes(ctx.topicArn), "SNS target missing");
  });

  await t.test("put_events_to_sqs", async () => {
    await drainQueue(sqs, ctx.ebQueueUrl);
    const put = await events.send(
      new PutEventsCommand({
        Entries: [
          {
            EventBusName: ctx.busName,
            Source: EVENT_SOURCE,
            DetailType: "OrderCreated",
            Detail: JSON.stringify({
              orderId: ORDER_ID,
              marker: "eb-order-pipeline",
            }),
          },
        ],
      }),
    );
    assert.equal(put.FailedEntryCount ?? 0, 0);

    const msg = await receiveOne(sqs, ctx.ebQueueUrl);
    assert.ok(msg, "expected EventBridge delivery to SQS");
    assert.match(msg.Body || "", /noctaxris\.lab\.orders/);
    assert.match(msg.Body || "", /eb-order-pipeline/);
    assert.match(msg.Body || "", /ord-1001/);
    await sqs.send(
      new DeleteMessageCommand({
        QueueUrl: ctx.ebQueueUrl,
        ReceiptHandle: msg.ReceiptHandle,
      }),
    );

    await events.send(
      new PutEventsCommand({
        Entries: [
          {
            EventBusName: ctx.busName,
            Source: "noctaxris.lab.other",
            DetailType: "OrderCreated",
            Detail: JSON.stringify({ orderId: "no-match" }),
          },
        ],
      }),
    );
    const miss = await receiveOne(sqs, ctx.ebQueueUrl, {
      maxAttempts: 3,
      wait: 0,
    });
    assert.equal(miss, null, "mismatched source must not deliver");
  });

  await t.test("sns_fanout", async () => {
    await drainQueue(sqs, ctx.fanoutQueueUrl);
    const marker = `sns-fanout-${prefix}`;
    const pub = await sns.send(
      new PublishCommand({
        TopicArn: ctx.topicArn,
        Message: marker,
        Subject: "lab-order",
      }),
    );
    assert.ok(pub.MessageId, "Publish missing MessageId");

    const msg = await receiveOne(sqs, ctx.fanoutQueueUrl);
    assert.ok(msg, "expected SNS→SQS fan-out message");
    assert.match(msg.Body || "", new RegExp(marker));
    await sqs.send(
      new DeleteMessageCommand({
        QueueUrl: ctx.fanoutQueueUrl,
        ReceiptHandle: msg.ReceiptHandle,
      }),
    );
  });

  await t.test("data_plane_reads", async () => {
    const obj = await s3.send(
      new GetObjectCommand({ Bucket: ctx.bucket, Key: "order.json" }),
    );
    const bytes = Buffer.from(await obj.Body.transformToByteArray());
    assert.deepEqual(bytes, ORDER_BODY);

    const item = await ddb.send(
      new GetItemCommand({
        TableName: ctx.table,
        Key: { orderId: { S: ORDER_ID } },
      }),
    );
    assert.equal(item.Item?.sku?.S, "widget");
    assert.equal(item.Item?.qty?.N, "2");

    const strParam = await ssm.send(
      new GetParameterCommand({ Name: ctx.stringParam }),
    );
    assert.equal(strParam.Parameter?.Value, CONFIG_VALUE);

    const secParam = await ssm.send(
      new GetParameterCommand({
        Name: ctx.secureParam,
        WithDecryption: true,
      }),
    );
    assert.equal(secParam.Parameter?.Value, SECURE_VALUE);

    const secret = await secrets.send(
      new GetSecretValueCommand({ SecretId: ctx.secretName }),
    );
    assert.equal(secret.SecretString, SECRET_VALUE);
  });

  await t.test("authz_denies", async () => {
    const denyQ = await sqs.send(
      new CreateQueueCommand({ QueueName: `${prefix}-deny-q` }),
    );
    const denyUrl = denyQ.QueueUrl;
    t.after(async () => {
      try {
        await sqs.send(new DeleteQueueCommand({ QueueUrl: denyUrl }));
      } catch {
        /* ignore */
      }
    });
    const denyAttrs = await sqs.send(
      new GetQueueAttributesCommand({
        QueueUrl: denyUrl,
        AttributeNames: ["QueueArn"],
      }),
    );
    const denyArn = denyAttrs.Attributes?.QueueArn;

    const denyRule = `${prefix}-deny-rule`;
    await events.send(
      new PutRuleCommand({
        Name: denyRule,
        EventBusName: ctx.busName,
        EventPattern: JSON.stringify({ source: [EVENT_SOURCE] }),
        State: "ENABLED",
      }),
    );
    t.after(async () => {
      try {
        await events.send(
          new RemoveTargetsCommand({
            Rule: denyRule,
            EventBusName: ctx.busName,
            Ids: ["deny-sqs"],
          }),
        );
      } catch {
        /* ignore */
      }
      try {
        await events.send(
          new DeleteRuleCommand({
            Name: denyRule,
            EventBusName: ctx.busName,
          }),
        );
      } catch {
        /* ignore */
      }
    });
    await events.send(
      new PutTargetsCommand({
        Rule: denyRule,
        EventBusName: ctx.busName,
        Targets: [{ Id: "deny-sqs", Arn: denyArn }],
      }),
    );
    await events.send(
      new PutEventsCommand({
        Entries: [
          {
            EventBusName: ctx.busName,
            Source: EVENT_SOURCE,
            DetailType: "OrderCreated",
            Detail: JSON.stringify({ marker: "should-not-land" }),
          },
        ],
      }),
    );
    const blocked = await receiveOne(sqs, denyUrl, { maxAttempts: 3, wait: 0 });
    assert.equal(
      blocked,
      null,
      "delivery without events principal on queue must not land",
    );

    const denyKey = await kms.send(
      new CreateKeyCommand({ Description: `${prefix}-deny-cmk` }),
    );
    const denyKeyId = denyKey.KeyMetadata?.KeyId;
    t.after(async () => {
      try {
        await kms.send(
          new ScheduleKeyDeletionCommand({
            KeyId: denyKeyId,
            PendingWindowInDays: 7,
          }),
        );
      } catch {
        /* ignore */
      }
    });
    const denyParam = `/lab/${prefix}/kms-deny`;
    await ssm.send(
      new PutParameterCommand({
        Name: denyParam,
        Value: "locked",
        Type: "SecureString",
        KeyId: denyKeyId,
      }),
    );
    t.after(async () => {
      try {
        await ssm.send(new DeleteParameterCommand({ Name: denyParam }));
      } catch {
        /* ignore */
      }
    });
    await kms.send(
      new PutKeyPolicyCommand({
        KeyId: denyKeyId,
        PolicyName: "default",
        Policy: JSON.stringify({
          Version: "2012-10-17",
          Statement: [
            {
              Sid: "EncryptOnly",
              Effect: "Allow",
              Principal: { AWS: `arn:aws:iam::${ctx.account}:root` },
              Action: ["kms:Encrypt", "kms:GenerateDataKey*", "kms:DescribeKey"],
              Resource: "*",
            },
          ],
        }),
      }),
    );
    await assert.rejects(
      () =>
        ssm.send(
          new GetParameterCommand({
            Name: denyParam,
            WithDecryption: true,
          }),
        ),
      (err) => {
        const name = err?.name || "";
        const msg = String(err?.message || err);
        return (
          /AccessDenied|AccessDeniedException|UnauthorizedOperation/i.test(
            name,
          ) ||
          /AccessDenied|not authorized|KMS/i.test(msg)
        );
      },
    );
  });
});
