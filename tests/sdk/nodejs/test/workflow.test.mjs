import { test } from "node:test";
import assert from "node:assert/strict";
import {
  CreateApiKeyCommand,
  CreateDataSourceCommand,
  CreateGraphqlApiCommand,
  CreateResolverCommand,
  StartSchemaCreationCommand,
} from "@aws-sdk/client-appsync";
import {
  CreateTrailCommand,
  DeleteTrailCommand,
  LookupEventsCommand,
  StartLoggingCommand,
  StopLoggingCommand,
} from "@aws-sdk/client-cloudtrail";
import {
  CreateRoleCommand,
  DeleteRoleCommand,
} from "@aws-sdk/client-iam";
import {
  CreateFunctionCommand,
  DeleteFunctionCommand,
} from "@aws-sdk/client-lambda";
import {
  CreateBucketCommand,
  DeleteBucketCommand,
  DeleteObjectCommand,
  ListObjectsV2Command,
} from "@aws-sdk/client-s3";
import {
  CreateStateMachineCommand,
  DeleteStateMachineCommand,
  DescribeExecutionCommand,
  StartExecutionCommand,
} from "@aws-sdk/client-sfn";
import {
  newAppSync,
  newCloudTrail,
  newIAM,
  newLambda,
  newS3,
  newSFN,
  requireReady,
  signedJsonTarget,
  uniquePrefix,
} from "../lib/helpers.mjs";

const LAMBDA_TRUST =
  '{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"}]}';
const APPSYNC_TRUST =
  '{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"appsync.amazonaws.com"},"Action":"sts:AssumeRole"}]}';

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

test("SFN Choice ASL StartExecution SUCCEEDED", async (t) => {
  if (!(await requireReady(t))) return;
  const sfn = newSFN();
  const prefix = uniquePrefix();
  const name = `${prefix}-choice`.slice(0, 64);
  const definition = JSON.stringify({
    StartAt: "Pick",
    States: {
      Pick: {
        Type: "Choice",
        Choices: [
          {
            Variable: "$.color",
            StringEquals: "red",
            Next: "Red",
          },
        ],
        Default: "Other",
      },
      Red: {
        Type: "Pass",
        Result: { branch: "red" },
        End: true,
      },
      Other: {
        Type: "Pass",
        Result: { branch: "other" },
        End: true,
      },
    },
  });

  // Choice/Pass ASL needs no Task RoleArn (omit so PassRole is not required).
  const sm = await sfn.send(
    new CreateStateMachineCommand({
      name,
      definition,
    }),
  );
  const smArn = sm.stateMachineArn;
  assert.ok(smArn);
  t.after(async () => {
    try {
      await sfn.send(
        new DeleteStateMachineCommand({ stateMachineArn: smArn }),
      );
    } catch {
      /* ignore */
    }
  });

  const started = await sfn.send(
    new StartExecutionCommand({
      stateMachineArn: smArn,
      name: `${prefix}-run`.slice(0, 64),
      input: JSON.stringify({ color: "red" }),
    }),
  );
  const execArn = started.executionArn;
  assert.ok(execArn);

  const desc = await sfn.send(
    new DescribeExecutionCommand({ executionArn: execArn }),
  );
  assert.equal(desc.status, "SUCCEEDED", JSON.stringify(desc));
  assert.match(desc.output || "", /"branch"\s*:\s*"red"/);
});

test("CloudTrail CreateTrail StartLogging S3 delivery list", async (t) => {
  if (!(await requireReady(t))) return;
  const s3 = newS3();
  const ct = newCloudTrail();
  const prefix = uniquePrefix();
  const bucket = `${prefix}-ct`.toLowerCase();
  const trailName = `${prefix}-trail`.slice(0, 64);

  await s3.send(new CreateBucketCommand({ Bucket: bucket }));
  t.after(async () => {
    try {
      const listed = await s3.send(
        new ListObjectsV2Command({ Bucket: bucket, Prefix: "AWSLogs/" }),
      );
      for (const obj of listed.Contents || []) {
        if (!obj.Key) continue;
        try {
          await s3.send(
            new DeleteObjectCommand({ Bucket: bucket, Key: obj.Key }),
          );
        } catch {
          /* ignore */
        }
      }
    } catch {
      /* ignore */
    }
    try {
      await s3.send(new DeleteBucketCommand({ Bucket: bucket }));
    } catch {
      /* ignore */
    }
  });

  await ct.send(
    new CreateTrailCommand({
      Name: trailName,
      S3BucketName: bucket,
    }),
  );
  await ct.send(new StartLoggingCommand({ Name: trailName }));

  const listed = await s3.send(
    new ListObjectsV2Command({ Bucket: bucket, Prefix: "AWSLogs/" }),
  );
  const delivered = (listed.Contents || []).some((o) =>
    (o.Key || "").includes("/CloudTrail/"),
  );
  assert.ok(
    delivered,
    `expected CloudTrail delivery object under AWSLogs/: ${JSON.stringify(listed.Contents)}`,
  );
});

test("CloudTrail PutEventSelectors GetEventSelectors round-trip", async (t) => {
  if (!(await requireReady(t))) return;
  const s3 = newS3();
  const ct = newCloudTrail();
  const prefix = uniquePrefix();
  const bucket = `${prefix}-ctsel`.toLowerCase();
  const trailName = `${prefix}-sel`.slice(0, 64);

  await s3.send(new CreateBucketCommand({ Bucket: bucket }));
  t.after(async () => {
    try {
      await ct.send(new DeleteTrailCommand({ Name: trailName }));
    } catch {
      /* ignore */
    }
    try {
      const listed = await s3.send(
        new ListObjectsV2Command({ Bucket: bucket }),
      );
      for (const obj of listed.Contents || []) {
        if (!obj.Key) continue;
        try {
          await s3.send(
            new DeleteObjectCommand({ Bucket: bucket, Key: obj.Key }),
          );
        } catch {
          /* ignore */
        }
      }
    } catch {
      /* ignore */
    }
    try {
      await s3.send(new DeleteBucketCommand({ Bucket: bucket }));
    } catch {
      /* ignore */
    }
  });

  await ct.send(
    new CreateTrailCommand({
      Name: trailName,
      S3BucketName: bucket,
    }),
  );

  const put = await signedJsonTarget(
    "cloudtrail",
    "CloudTrail_20131101.PutEventSelectors",
    {
      TrailName: trailName,
      EventSelectors: [
        {
          ReadWriteType: "All",
          IncludeManagementEvents: true,
          DataResources: [
            { Type: "AWS::S3::Object", Values: ["arn:aws:s3:::"] },
          ],
        },
      ],
    },
  );
  assert.equal(put.status, 200, put.body);

  const got = await signedJsonTarget(
    "cloudtrail",
    "CloudTrail_20131101.GetEventSelectors",
    { TrailName: trailName },
  );
  assert.equal(got.status, 200, got.body);
  const selectors = got.json?.EventSelectors || [];
  assert.equal(selectors.length, 1, got.body);
  const resources = selectors[0]?.DataResources || [];
  assert.ok(resources.length > 0, `expected S3 DataResources in ${JSON.stringify(selectors[0])}`);
});

test("CloudTrail InjectInsightsEvents LookupEvents EventCategory=insight", async (t) => {
  if (!(await requireReady(t))) return;
  if (process.env.NOCTAXRIS_CLOUDTRAIL_INJECT !== "1") {
    t.skip(
      "set NOCTAXRIS_CLOUDTRAIL_INJECT=1 on the API process for lab InjectInsightsEvents",
    );
    return;
  }
  const prefix = uniquePrefix();
  const eventID = `sdk-insight-${prefix}`;

  const inj = await signedJsonTarget(
    "cloudtrail",
    "NoctaxrisCloudTrail.InjectInsightsEvents",
    {
      Event: {
        eventTime: "2026-07-20T12:05:00Z",
        eventID,
        insightDetails: {
          state: "Start",
          eventSource: "sts.amazonaws.com",
          eventName: "AssumeRole",
          insightType: "ApiCallRateInsight",
          sourceEventCategory: "Management",
          insightContext: {
            statistics: {
              baseline: { average: 0.1 },
              insight: { average: 12.0 },
              insightDuration: 5,
              baselineDuration: 1000,
            },
          },
        },
      },
    },
  );
  assert.equal(inj.status, 200, inj.body);

  const looked = await signedJsonTarget(
    "cloudtrail",
    "CloudTrail_20131101.LookupEvents",
    { EventCategory: "insight", MaxResults: 50 },
  );
  assert.equal(looked.status, 200, looked.body);
  assert.ok(
    (looked.body || "").includes(eventID),
    `insight LookupEvents missing ${eventID}: ${looked.body}`,
  );
});

test("CloudTrail InjectEvents LookupEvents SourceIP/EventName + ValidateLogs", async (t) => {
  if (!(await requireReady(t))) return;
  if (process.env.NOCTAXRIS_CLOUDTRAIL_INJECT !== "1") {
    t.skip(
      "set NOCTAXRIS_CLOUDTRAIL_INJECT=1 on the API process for lab InjectEvents",
    );
    return;
  }
  const s3 = newS3();
  const ct = newCloudTrail();
  const prefix = uniquePrefix();
  const bucket = `${prefix}-ctinj`.toLowerCase();
  const trailName = `${prefix}-inj`.slice(0, 64);
  const eventID = `sdk-inj-${prefix}`;
  const sourceIP = "198.51.100.44";

  const inj = await signedJsonTarget(
    "cloudtrail",
    "NoctaxrisCloudTrail.InjectEvents",
    {
      Events: [
        {
          eventTime: "2026-07-20T15:00:00Z",
          sourceIPAddress: sourceIP,
          userIdentity: { type: "IAMUser", userName: "sdk-forensic" },
          eventSource: "signin.amazonaws.com",
          eventName: "ConsoleLogin",
          eventID,
          readOnly: false,
        },
      ],
    },
  );
  assert.equal(inj.status, 200, inj.body);

  const byIP = await ct.send(
    new LookupEventsCommand({
      LookupAttributes: [
        { AttributeKey: "SourceIPAddress", AttributeValue: sourceIP },
      ],
      MaxResults: 10,
    }),
  );
  const ipHit = (byIP.Events || []).some(
    (ev) =>
      ev.EventId === eventID ||
      (ev.CloudTrailEvent || "").includes(eventID),
  );
  assert.ok(ipHit, `LookupEvents SourceIPAddress missing ${eventID}`);

  const byName = await ct.send(
    new LookupEventsCommand({
      LookupAttributes: [
        { AttributeKey: "EventName", AttributeValue: "ConsoleLogin" },
      ],
      MaxResults: 10,
    }),
  );
  const nameHit = (byName.Events || []).some(
    (ev) =>
      ev.EventId === eventID ||
      (ev.CloudTrailEvent || "").includes(eventID),
  );
  assert.ok(nameHit, `LookupEvents EventName missing ${eventID}`);

  await s3.send(new CreateBucketCommand({ Bucket: bucket }));
  t.after(async () => {
    try {
      await ct.send(new StopLoggingCommand({ Name: trailName }));
    } catch {
      /* ignore */
    }
    try {
      await ct.send(new DeleteTrailCommand({ Name: trailName }));
    } catch {
      /* ignore */
    }
    try {
      const listed = await s3.send(
        new ListObjectsV2Command({ Bucket: bucket, Prefix: "AWSLogs/" }),
      );
      for (const obj of listed.Contents || []) {
        if (!obj.Key) continue;
        try {
          await s3.send(
            new DeleteObjectCommand({ Bucket: bucket, Key: obj.Key }),
          );
        } catch {
          /* ignore */
        }
      }
    } catch {
      /* ignore */
    }
    try {
      await s3.send(new DeleteBucketCommand({ Bucket: bucket }));
    } catch {
      /* ignore */
    }
  });

  await ct.send(
    new CreateTrailCommand({
      Name: trailName,
      S3BucketName: bucket,
    }),
  );
  await ct.send(new StartLoggingCommand({ Name: trailName }));

  const listed = await s3.send(
    new ListObjectsV2Command({ Bucket: bucket, Prefix: "AWSLogs/" }),
  );
  const logKey = (listed.Contents || [])
    .map((o) => o.Key || "")
    .find(
      (k) =>
        k.includes("/CloudTrail/") && !k.includes("CloudTrail-Digest"),
    );
  assert.ok(
    logKey,
    `missing CloudTrail log object: ${JSON.stringify(listed.Contents)}`,
  );

  const validated = await signedJsonTarget(
    "cloudtrail",
    "CloudTrail_20131101.ValidateLogs",
    { S3BucketName: bucket, S3ObjectKey: logKey },
  );
  assert.equal(validated.status, 200, validated.body);
  assert.equal(validated.json?.Valid, true, validated.body);
});

test("ELBv2 CreateRule path-pattern (no /alb/ invoke)", async (t) => {
  if (!(await requireReady(t))) return;
  const prefix = uniquePrefix();
  const lbName = `${prefix}-alb`.replace(/_/g, "-").slice(0, 32);
  const tgName = `${prefix}-tg`.replace(/_/g, "-").slice(0, 32);

  const lb = await signedJsonTarget(
    "elasticloadbalancing",
    "ElasticLoadBalancing_v2.CreateLoadBalancer",
    { Name: lbName },
  );
  assert.equal(lb.status, 200, lb.body);
  const lbArn = lb.json?.LoadBalancers?.[0]?.LoadBalancerArn;
  assert.ok(lbArn, lb.body);

  const tg = await signedJsonTarget(
    "elasticloadbalancing",
    "ElasticLoadBalancing_v2.CreateTargetGroup",
    { Name: tgName, TargetType: "lambda" },
  );
  assert.equal(tg.status, 200, tg.body);
  const tgArn = tg.json?.TargetGroups?.[0]?.TargetGroupArn;
  assert.ok(tgArn, tg.body);

  const listener = await signedJsonTarget(
    "elasticloadbalancing",
    "ElasticLoadBalancing_v2.CreateListener",
    {
      LoadBalancerArn: lbArn,
      Protocol: "HTTP",
      Port: 80,
      DefaultActions: [{ Type: "forward", TargetGroupArn: tgArn }],
    },
  );
  assert.equal(listener.status, 200, listener.body);
  const listenerArn = listener.json?.Listeners?.[0]?.ListenerArn;
  assert.ok(listenerArn, listener.body);

  const rule = await signedJsonTarget(
    "elasticloadbalancing",
    "ElasticLoadBalancing_v2.CreateRule",
    {
      ListenerArn: listenerArn,
      Priority: 5,
      Conditions: [
        { Field: "path-pattern", Values: ["/api*"] },
      ],
      Actions: [{ Type: "forward", TargetGroupArn: tgArn }],
    },
  );
  assert.equal(rule.status, 200, rule.body);
  assert.match(rule.body, /\/api\*/);

  const desc = await signedJsonTarget(
    "elasticloadbalancing",
    "ElasticLoadBalancing_v2.DescribeRules",
    { ListenerArn: listenerArn },
  );
  assert.equal(desc.status, 200, desc.body);
  assert.match(desc.body, /\/api\*/);
});

test("AppSync CreateDataSource serviceRoleArn + two CreateResolver fields", async (t) => {
  if (!(await requireReady(t))) return;
  const iam = newIAM();
  const lam = newLambda();
  const appsync = newAppSync();
  const prefix = uniquePrefix();
  const roleName = `${prefix}-appsync-ds`;
  const lambdaRoleName = `${prefix}-appsync-fn`;
  const fnHello = `${prefix}-hello`.slice(0, 64);
  const fnWorld = `${prefix}-world`.slice(0, 64);

  const dsRole = await iam.send(
    new CreateRoleCommand({
      RoleName: roleName,
      AssumeRolePolicyDocument: APPSYNC_TRUST,
    }),
  );
  const serviceRoleArn = dsRole.Role?.Arn;
  assert.ok(serviceRoleArn);
  t.after(async () => {
    try {
      await iam.send(new DeleteRoleCommand({ RoleName: roleName }));
    } catch {
      /* ignore */
    }
  });

  const fnRole = await iam.send(
    new CreateRoleCommand({
      RoleName: lambdaRoleName,
      AssumeRolePolicyDocument: LAMBDA_TRUST,
    }),
  );
  const fnRoleArn = fnRole.Role?.Arn;
  assert.ok(fnRoleArn);
  t.after(async () => {
    try {
      await iam.send(new DeleteRoleCommand({ RoleName: lambdaRoleName }));
    } catch {
      /* ignore */
    }
  });

  for (const fnName of [fnHello, fnWorld]) {
    await lam.send(
      new CreateFunctionCommand({
        FunctionName: fnName,
        Runtime: "python3.12",
        Role: fnRoleArn,
        Handler: "index.handler",
        Code: { ZipFile: minimalPythonZip() },
      }),
    );
    t.after(async () => {
      try {
        await lam.send(new DeleteFunctionCommand({ FunctionName: fnName }));
      } catch {
        /* ignore */
      }
    });
  }

  const api = await appsync.send(
    new CreateGraphqlApiCommand({
      name: `${prefix}-gql`.slice(0, 50),
      authenticationType: "API_KEY",
    }),
  );
  const apiId = api.graphqlApi?.apiId;
  assert.ok(apiId);

  await appsync.send(
    new StartSchemaCreationCommand({
      apiId,
      definition: "type Query { hello: String world: String }",
    }),
  );
  await appsync.send(new CreateApiKeyCommand({ apiId }));

  const helloArn = `arn:aws:lambda:us-east-1:000000000001:function:${fnHello}`;
  const worldArn = `arn:aws:lambda:us-east-1:000000000001:function:${fnWorld}`;

  await appsync.send(
    new CreateDataSourceCommand({
      apiId,
      name: "HelloDS",
      type: "AWS_LAMBDA",
      serviceRoleArn,
      lambdaConfig: { lambdaFunctionArn: helloArn },
    }),
  );
  await appsync.send(
    new CreateDataSourceCommand({
      apiId,
      name: "WorldDS",
      type: "AWS_LAMBDA",
      serviceRoleArn,
      lambdaConfig: { lambdaFunctionArn: worldArn },
    }),
  );
  await appsync.send(
    new CreateResolverCommand({
      apiId,
      typeName: "Query",
      fieldName: "hello",
      dataSourceName: "HelloDS",
    }),
  );
  await appsync.send(
    new CreateResolverCommand({
      apiId,
      typeName: "Query",
      fieldName: "world",
      dataSourceName: "WorldDS",
    }),
  );

  // GraphQL invoke needs nested Lambda; skip unless NOCTAXRIS_NESTED=1.
  if (process.env.NOCTAXRIS_NESTED !== "1") {
    t.diagnostic(
      "AppSync GraphQL invoke skipped (set NOCTAXRIS_NESTED=1 for nested Lambda)",
    );
    return;
  }
});
