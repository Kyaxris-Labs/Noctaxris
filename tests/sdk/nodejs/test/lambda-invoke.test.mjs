import { test } from "node:test";
import assert from "node:assert/strict";
import {
  CreateFunctionCommand,
  DeleteFunctionCommand,
  InvokeCommand,
} from "@aws-sdk/client-lambda";
import {
  CreateRoleCommand,
  DeleteRoleCommand,
} from "@aws-sdk/client-iam";
import {
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

test("Lambda nested Invoke", async (t) => {
  if (process.env.NOCTAXRIS_NESTED !== "1") {
    t.skip(
      "nested Lambda Invoke — set NOCTAXRIS_NESTED=1 (Compose noctaxris-engine healthy)",
    );
    return;
  }
  if (!(await requireReady(t))) return;

  const iam = newIAM();
  const lam = newLambda();
  const prefix = uniquePrefix();
  const roleName = `${prefix}-lambda-role`;
  const fnName = `${prefix}-invoke-fn`;

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

  await lam.send(
    new CreateFunctionCommand({
      FunctionName: fnName,
      Runtime: "python3.12",
      Role: roleArn,
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

  const inv = await lam.send(
    new InvokeCommand({
      FunctionName: fnName,
      Payload: Buffer.from("{}"),
    }),
  );
  assert.ok(!inv.FunctionError, `FunctionError=${inv.FunctionError}`);
  const raw = Buffer.from(inv.Payload || []).toString("utf8");
  const body = JSON.parse(raw);
  assert.equal(body.ok, true);
});
