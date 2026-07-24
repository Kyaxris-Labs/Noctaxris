import { test } from "node:test";
import assert from "node:assert/strict";
import {
  CreateApiCommand,
  CreateIntegrationCommand,
  CreateRouteCommand,
  CreateStageCommand,
  DeleteApiCommand,
  UpdateApiCommand,
} from "@aws-sdk/client-apigatewayv2";
import {
  CreateRoleCommand,
  DeleteRoleCommand,
} from "@aws-sdk/client-iam";
import {
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

test("HTTP API CORS preflight and UpdateApi", async (t) => {
  if (!(await requireReady(t))) return;
  const iam = newIAM();
  const lam = newLambda();
  const apigw = newAPIGWv2();
  const prefix = uniquePrefix();

  const roleName = `${prefix}-cors-role`;
  const roleOut = await iam.send(
    new CreateRoleCommand({
      RoleName: roleName,
      AssumeRolePolicyDocument: TRUST,
    }),
  );
  const roleArn = roleOut.Role?.Arn;
  assert.ok(roleArn, "CreateRole missing Arn");
  t.after(async () => {
    try {
      await iam.send(new DeleteRoleCommand({ RoleName: roleName }));
    } catch {
      /* ignore */
    }
  });

  const fnName = `${prefix}-cors-fn`;
  const fnOut = await lam.send(
    new CreateFunctionCommand({
      FunctionName: fnName,
      Runtime: "python3.12",
      Role: roleArn,
      Handler: "index.handler",
      Code: { ZipFile: minimalPythonZip() },
    }),
  );
  const lambdaArn = fnOut.FunctionArn;
  assert.ok(lambdaArn, "CreateFunction missing FunctionArn");
  t.after(async () => {
    try {
      await lam.send(new DeleteFunctionCommand({ FunctionName: fnName }));
    } catch {
      /* ignore */
    }
  });

  const apiOut = await apigw.send(
    new CreateApiCommand({
      Name: `${prefix}-cors-api`,
      ProtocolType: "HTTP",
      CorsConfiguration: {
        AllowOrigins: ["https://lab.example"],
        AllowMethods: ["GET", "OPTIONS"],
        AllowHeaders: ["authorization", "content-type"],
        MaxAge: 600,
      },
    }),
  );
  const apiId = apiOut.ApiId;
  assert.ok(apiId, "CreateApi missing ApiId");
  assert.ok(apiOut.CorsConfiguration, "CreateApi missing CorsConfiguration");
  t.after(async () => {
    try {
      await apigw.send(new DeleteApiCommand({ ApiId: apiId }));
    } catch {
      /* ignore */
    }
  });

  const intOut = await apigw.send(
    new CreateIntegrationCommand({
      ApiId: apiId,
      IntegrationType: "AWS_PROXY",
      IntegrationUri: lambdaArn,
    }),
  );
  const integrationId = intOut.IntegrationId;
  assert.ok(integrationId, "CreateIntegration missing IntegrationId");

  await apigw.send(
    new CreateRouteCommand({
      ApiId: apiId,
      RouteKey: "GET /hello",
      Target: `integrations/${integrationId}`,
      AuthorizationType: "NONE",
    }),
  );

  await apigw.send(
    new CreateStageCommand({
      ApiId: apiId,
      StageName: "$default",
    }),
  );

  const helloURL = `${endpoint()}/http-api/${apiId}/$default/hello`;

  const optResp = await fetch(helloURL, {
    method: "OPTIONS",
    headers: {
      Origin: "https://lab.example",
      "Access-Control-Request-Method": "GET",
      "Access-Control-Request-Headers": "authorization",
    },
  });
  const optBody = await optResp.text();
  assert.equal(optResp.status, 204, `preflight status body=${optBody}`);
  assert.equal(optResp.headers.get("access-control-allow-origin"), "https://lab.example");
  assert.match(
    optResp.headers.get("access-control-allow-methods") || "",
    /GET/,
  );
  assert.equal(optResp.headers.get("access-control-max-age"), "600");

  const denyResp = await fetch(helloURL, {
    method: "OPTIONS",
    headers: {
      Origin: "https://evil.example",
      "Access-Control-Request-Method": "GET",
    },
  });
  assert.equal(denyResp.status, 403, "foreign origin preflight want 403");

  await apigw.send(
    new UpdateApiCommand({
      ApiId: apiId,
      CorsConfiguration: {
        AllowOrigins: ["https://other.example"],
        AllowMethods: ["GET", "POST", "OPTIONS"],
      },
    }),
  );

  const opt2Resp = await fetch(helloURL, {
    method: "OPTIONS",
    headers: {
      Origin: "https://other.example",
      "Access-Control-Request-Method": "POST",
    },
  });
  const opt2Body = await opt2Resp.text();
  assert.equal(opt2Resp.status, 204, `updated preflight body=${opt2Body}`);
  assert.equal(
    opt2Resp.headers.get("access-control-allow-origin"),
    "https://other.example",
  );
});
