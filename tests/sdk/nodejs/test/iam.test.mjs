import { test } from "node:test";
import assert from "node:assert/strict";
import {
  CreateAccessKeyCommand,
  CreateRoleCommand,
  CreateUserCommand,
  DeleteAccessKeyCommand,
  DeleteRoleCommand,
  DeleteUserCommand,
  GenerateCredentialReportCommand,
  GetAccessKeyLastUsedCommand,
  GetCredentialReportCommand,
  GetRoleCommand,
  GetUserCommand,
} from "@aws-sdk/client-iam";
import { GetCallerIdentityCommand, STSClient } from "@aws-sdk/client-sts";
import {
  endpoint,
  newIAM,
  region,
  requireReady,
  uniquePrefix,
} from "../lib/helpers.mjs";

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

test("IAM access key last-used and credential report", async (t) => {
  if (!(await requireReady(t))) return;
  const client = newIAM();
  const userName = `${uniquePrefix()}-lastused`;

  await client.send(new CreateUserCommand({ UserName: userName }));
  t.after(async () => {
    try {
      await client.send(new DeleteUserCommand({ UserName: userName }));
    } catch {
      /* ignore */
    }
  });

  const keyOut = await client.send(
    new CreateAccessKeyCommand({ UserName: userName }),
  );
  const accessKeyId = keyOut.AccessKey?.AccessKeyId;
  const secretAccessKey = keyOut.AccessKey?.SecretAccessKey;
  assert.ok(accessKeyId && secretAccessKey, "CreateAccessKey missing key");
  t.after(async () => {
    try {
      await client.send(
        new DeleteAccessKeyCommand({
          UserName: userName,
          AccessKeyId: accessKeyId,
        }),
      );
    } catch {
      /* ignore */
    }
  });

  const userSTS = new STSClient({
    region: region(),
    endpoint: endpoint(),
    credentials: {
      accessKeyId,
      secretAccessKey,
    },
  });
  await userSTS.send(new GetCallerIdentityCommand({}));

  const lastUsed = await client.send(
    new GetAccessKeyLastUsedCommand({ AccessKeyId: accessKeyId }),
  );
  assert.equal(lastUsed.UserName, userName);
  assert.equal(lastUsed.AccessKeyLastUsed?.ServiceName, "sts");
  assert.equal(lastUsed.AccessKeyLastUsed?.Region, region());

  const gen = await client.send(new GenerateCredentialReportCommand({}));
  assert.equal(String(gen.State || "").toUpperCase(), "COMPLETE");

  const report = await client.send(new GetCredentialReportCommand({}));
  assert.ok(report.Content?.length, "GetCredentialReport empty Content");
  const csvText = Buffer.from(report.Content).toString("utf8");
  const found = csvText
    .split(/\r?\n/)
    .slice(1)
    .some((line) => line.split(",")[0] === userName);
  assert.ok(found, `credential report missing user ${userName}: ${csvText}`);
});
