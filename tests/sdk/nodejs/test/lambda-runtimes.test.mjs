import { test } from "node:test";
import assert from "node:assert/strict";
import {
  CreateFunctionCommand,
  DeleteFunctionCommand,
  GetFunctionCommand,
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

function zipEntry(nameStr, dataStr) {
  const name = Buffer.from(nameStr);
  const data = Buffer.from(dataStr);
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

const pythonZip = () =>
  zipEntry(
    "index.py",
    "def handler(event, context):\n    return {'ok': True}\n",
  );
const nodeZip = () =>
  zipEntry("index.js", "exports.handler = async () => ({ ok: true });\n");
const javaZip = () =>
  zipEntry(
    "example/Handler.java",
    `package example;
public class Handler {
  public static String handleRequest(String in) { return "{\\"ok\\":true}"; }
}
`,
  );

const cases = [
  { suffix: "py311", runtime: "python3.11", handler: "index.handler", zip: pythonZip },
  { suffix: "py312", runtime: "python3.12", handler: "index.handler", zip: pythonZip },
  { suffix: "py313", runtime: "python3.13", handler: "index.handler", zip: pythonZip },
  { suffix: "py314", runtime: "python3.14", handler: "index.handler", zip: pythonZip },
  { suffix: "node20", runtime: "nodejs20.x", handler: "index.handler", zip: nodeZip },
  { suffix: "node22", runtime: "nodejs22.x", handler: "index.handler", zip: nodeZip },
  { suffix: "node24", runtime: "nodejs24.x", handler: "index.handler", zip: nodeZip },
  {
    suffix: "java21",
    runtime: "java21",
    handler: "example.Handler::handleRequest",
    zip: javaZip,
  },
  {
    suffix: "java25",
    runtime: "java25",
    handler: "example.Handler::handleRequest",
    zip: javaZip,
  },
];

test("Lambda create get delete for lab zip runtimes", async (t) => {
  if (!(await requireReady(t))) return;
  const iam = newIAM();
  const lam = newLambda();
  const prefix = uniquePrefix();
  const roleName = `${prefix}-lambda-rt-role`;

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

  for (const tc of cases) {
    await t.test(tc.runtime, async (st) => {
      const fnName = `${prefix}-${tc.suffix}`;
      await lam.send(
        new CreateFunctionCommand({
          FunctionName: fnName,
          Runtime: tc.runtime,
          Role: roleArn,
          Handler: tc.handler,
          Code: { ZipFile: tc.zip() },
        }),
      );
      st.after(async () => {
        try {
          await lam.send(new DeleteFunctionCommand({ FunctionName: fnName }));
        } catch {
          /* ignore */
        }
      });

      const got = await lam.send(
        new GetFunctionCommand({ FunctionName: fnName }),
      );
      assert.equal(got.Configuration?.Runtime, tc.runtime);
      assert.equal(got.Configuration?.Handler, tc.handler);

      await lam.send(new DeleteFunctionCommand({ FunctionName: fnName }));
    });
  }
});

test("Lambda CreateFunction rejects unsupported runtime", async (t) => {
  if (!(await requireReady(t))) return;
  const iam = newIAM();
  const lam = newLambda();
  const prefix = uniquePrefix();
  const roleName = `${prefix}-lambda-bad-rt`;

  const roleOut = await iam.send(
    new CreateRoleCommand({
      RoleName: roleName,
      AssumeRolePolicyDocument: TRUST,
    }),
  );
  const roleArn = roleOut.Role?.Arn;
  assert.ok(roleArn);
  t.after(async () => {
    try {
      await iam.send(new DeleteRoleCommand({ RoleName: roleName }));
    } catch {
      /* ignore */
    }
  });

  await assert.rejects(
    () =>
      lam.send(
        new CreateFunctionCommand({
          FunctionName: `${prefix}-bad`,
          Runtime: "ruby3.4",
          Role: roleArn,
          Handler: "index.handler",
          Code: { ZipFile: pythonZip() },
        }),
      ),
    /./,
  );
});
