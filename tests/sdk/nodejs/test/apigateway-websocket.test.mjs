import { test } from "node:test";
import assert from "node:assert/strict";
import {
  CreateApiCommand,
  CreateIntegrationCommand,
  CreateRouteCommand,
  CreateStageCommand,
  DeleteApiCommand,
} from "@aws-sdk/client-apigatewayv2";
import {
  CreateRoleCommand,
  DeleteRoleCommand,
} from "@aws-sdk/client-iam";
import {
  AddPermissionCommand,
  CreateFunctionCommand,
  DeleteFunctionCommand,
} from "@aws-sdk/client-lambda";
import {
  endpoint,
  newAPIGWv2,
  newIAM,
  newLambda,
  requireReady,
  uniquePrefix,
} from "../lib/helpers.mjs";

const TRUST =
  '{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"}]}';

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
  const payload = Buffer.from(
    "def handler(event, context):\n    return {'ok': True}\n",
    "utf8",
  );
  const name = Buffer.from("index.py");
  const size = payload.length;
  const crc = crc32(payload);
  const local = Buffer.alloc(30 + name.length);
  local.writeUInt32LE(0x04034b50, 0);
  local.writeUInt16LE(20, 4);
  local.writeUInt16LE(0, 6);
  local.writeUInt16LE(0, 8);
  local.writeUInt16LE(0, 10);
  local.writeUInt16LE(0, 12);
  local.writeUInt32LE(crc, 14);
  local.writeUInt32LE(size, 18);
  local.writeUInt32LE(size, 22);
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
  central.writeUInt32LE(crc, 16);
  central.writeUInt32LE(size, 20);
  central.writeUInt32LE(size, 24);
  central.writeUInt16LE(name.length, 28);
  central.writeUInt16LE(0, 30);
  central.writeUInt16LE(0, 32);
  central.writeUInt16LE(0, 34);
  central.writeUInt16LE(0, 36);
  central.writeUInt32LE(0, 38);
  central.writeUInt32LE(0, 42);
  name.copy(central, 46);
  const end = Buffer.alloc(22);
  end.writeUInt32LE(0x06054b50, 0);
  end.writeUInt16LE(0, 4);
  end.writeUInt16LE(0, 6);
  end.writeUInt16LE(1, 8);
  end.writeUInt16LE(1, 10);
  end.writeUInt32LE(central.length, 12);
  end.writeUInt32LE(local.length + size, 16);
  end.writeUInt16LE(0, 20);
  return Buffer.concat([local, payload, central, end]);
}

test("WebSocket API create routes soft-skip connect without nested", async (t) => {
  await requireReady();
  const prefix = uniquePrefix();
  const iam = newIAM();
  const lam = newLambda();
  const apigw = newAPIGWv2();

  const roleName = `${prefix}-ws-role`;
  const role = await iam.send(
    new CreateRoleCommand({
      RoleName: roleName,
      AssumeRolePolicyDocument: TRUST,
    }),
  );
  t.after(async () => {
    await iam.send(new DeleteRoleCommand({ RoleName: roleName })).catch(() => {});
  });
  const roleArn = role.Role.Arn;

  const fnName = `${prefix}-ws-fn`;
  const fn = await lam.send(
    new CreateFunctionCommand({
      FunctionName: fnName,
      Runtime: "python3.12",
      Role: roleArn,
      Handler: "index.handler",
      Code: { ZipFile: minimalPythonZip() },
    }),
  );
  t.after(async () => {
    await lam
      .send(new DeleteFunctionCommand({ FunctionName: fnName }))
      .catch(() => {});
  });
  await lam.send(
    new AddPermissionCommand({
      FunctionName: fnName,
      StatementId: `${prefix}-ws-perm`,
      Action: "lambda:InvokeFunction",
      Principal: "apigateway.amazonaws.com",
    }),
  );

  const api = await apigw.send(
    new CreateApiCommand({
      Name: `${prefix}-ws-api`,
      ProtocolType: "WEBSOCKET",
    }),
  );
  assert.equal(api.ProtocolType, "WEBSOCKET");
  const apiId = api.ApiId;
  t.after(async () => {
    await apigw.send(new DeleteApiCommand({ ApiId: apiId })).catch(() => {});
  });

  const integ = await apigw.send(
    new CreateIntegrationCommand({
      ApiId: apiId,
      IntegrationType: "AWS_PROXY",
      IntegrationUri: fn.FunctionArn,
    }),
  );
  for (const routeKey of ["$connect", "$disconnect", "$default"]) {
    await apigw.send(
      new CreateRouteCommand({
        ApiId: apiId,
        RouteKey: routeKey,
        Target: `integrations/${integ.IntegrationId}`,
        AuthorizationType: "NONE",
      }),
    );
  }
  await apigw.send(
    new CreateStageCommand({
      ApiId: apiId,
      StageName: "$default",
      AutoDeploy: true,
    }),
  );

  const res = await fetch(
    `${endpoint()}/ws-api/${apiId}/$default/$connect`,
    { method: "POST" },
  );
  const text = await res.text();
  if (
    res.status === 503 &&
    (text.includes("compute unavailable") ||
      process.env.NOCTAXRIS_NESTED !== "1")
  ) {
    t.skip(
      "WebSocket $connect soft-skip: nested Lambda unavailable (set NOCTAXRIS_NESTED=1)",
    );
    return;
  }
  assert.equal(res.status, 200, text);
  const body = JSON.parse(text);
  assert.ok(body.connectionId);
});
