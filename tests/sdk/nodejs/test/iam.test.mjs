import { test } from "node:test";
import assert from "node:assert/strict";
import {
  CreateRoleCommand,
  CreateUserCommand,
  DeleteRoleCommand,
  DeleteUserCommand,
  GetRoleCommand,
  GetUserCommand,
} from "@aws-sdk/client-iam";
import { newIAM, requireReady, uniquePrefix } from "../lib/helpers.mjs";

const TRUST =
  '{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"}]}';

test("IAM user and role round-trip", async (t) => {
  if (!(await requireReady(t))) return;
  const client = newIAM();
  const prefix = uniquePrefix();
  const userName = `${prefix}-user`;
  const roleName = `${prefix}-role`;

  await client.send(new CreateUserCommand({ UserName: userName }));
  t.after(async () => {
    try {
      await client.send(new DeleteUserCommand({ UserName: userName }));
    } catch {
      /* ignore */
    }
  });

  const user = await client.send(new GetUserCommand({ UserName: userName }));
  assert.equal(user.User?.UserName, userName);

  await client.send(
    new CreateRoleCommand({
      RoleName: roleName,
      AssumeRolePolicyDocument: TRUST,
    }),
  );
  t.after(async () => {
    try {
      await client.send(new DeleteRoleCommand({ RoleName: roleName }));
    } catch {
      /* ignore */
    }
  });

  const role = await client.send(new GetRoleCommand({ RoleName: roleName }));
  assert.ok(role.Role?.Arn, "GetRole missing ARN");

  await client.send(new DeleteRoleCommand({ RoleName: roleName }));
  await client.send(new DeleteUserCommand({ UserName: userName }));
});
