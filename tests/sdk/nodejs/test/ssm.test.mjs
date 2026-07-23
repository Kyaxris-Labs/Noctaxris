import { test } from "node:test";
import assert from "node:assert/strict";
import {
  DeleteParameterCommand,
  GetParameterCommand,
  PutParameterCommand,
} from "@aws-sdk/client-ssm";
import { newSSM, requireReady, uniquePrefix } from "../lib/helpers.mjs";

test("SSM String parameter round-trip", async (t) => {
  if (!(await requireReady(t))) return;
  const ssm = newSSM();
  const prefix = uniquePrefix();
  const name = `/lab/${prefix}/config`;
  const value = `plain-${prefix}`;

  await ssm.send(
    new PutParameterCommand({
      Name: name,
      Type: "String",
      Value: value,
    }),
  );
  t.after(async () => {
    try {
      await ssm.send(new DeleteParameterCommand({ Name: name }));
    } catch {
      /* ignore */
    }
  });

  const got = await ssm.send(new GetParameterCommand({ Name: name }));
  assert.equal(got.Parameter?.Value, value);

  await ssm.send(new DeleteParameterCommand({ Name: name }));
});
