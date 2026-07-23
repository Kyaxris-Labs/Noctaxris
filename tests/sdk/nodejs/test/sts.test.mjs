import { test } from "node:test";
import assert from "node:assert/strict";
import { GetCallerIdentityCommand } from "@aws-sdk/client-sts";
import { newSTS, requireReady } from "../lib/helpers.mjs";

test("STS GetCallerIdentity", async (t) => {
  if (!(await requireReady(t))) return;
  const client = newSTS();
  const out = await client.send(new GetCallerIdentityCommand({}));
  assert.ok(out.Account, "expected Account");
  assert.ok(out.Arn, "expected Arn");
  assert.ok(out.UserId, "expected UserId");
});
