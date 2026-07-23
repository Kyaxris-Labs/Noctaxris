import { test } from "node:test";
import assert from "node:assert/strict";
import {
  CreateSecretCommand,
  DeleteSecretCommand,
  GetSecretValueCommand,
} from "@aws-sdk/client-secrets-manager";
import { newSecrets, requireReady, uniquePrefix } from "../lib/helpers.mjs";

test("Secrets Manager round-trip", async (t) => {
  if (!(await requireReady(t))) return;
  const secrets = newSecrets();
  const prefix = uniquePrefix();
  const name = `lab/${prefix}/api-key`;
  const secret = JSON.stringify({ apiKey: `key-${prefix}` });

  await secrets.send(
    new CreateSecretCommand({
      Name: name,
      SecretString: secret,
    }),
  );
  t.after(async () => {
    try {
      await secrets.send(
        new DeleteSecretCommand({
          SecretId: name,
          ForceDeleteWithoutRecovery: true,
        }),
      );
    } catch {
      /* ignore */
    }
  });

  const got = await secrets.send(
    new GetSecretValueCommand({ SecretId: name }),
  );
  assert.equal(got.SecretString, secret);

  await secrets.send(
    new DeleteSecretCommand({
      SecretId: name,
      ForceDeleteWithoutRecovery: true,
    }),
  );
});
