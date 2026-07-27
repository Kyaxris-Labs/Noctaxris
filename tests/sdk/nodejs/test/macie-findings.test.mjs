import { test } from "node:test";
import assert from "node:assert/strict";
import {
  CreateBucketCommand,
  DeleteBucketCommand,
  DeleteObjectCommand,
  PutObjectCommand,
} from "@aws-sdk/client-s3";
import { newS3, requireReady, signedJsonTarget, uniquePrefix } from "../lib/helpers.mjs";

test("Macie EnableMacie InjectFindings List/GetFindings", async (t) => {
  if (!(await requireReady(t))) return;
  if (process.env.NOCTAXRIS_MACIE_INJECT !== "1") {
    t.skip(
      "set NOCTAXRIS_MACIE_INJECT=1 on the API process for lab InjectFindings",
    );
    return;
  }

  const s3 = newS3();
  const prefix = uniquePrefix();
  const bucket = `${prefix}-macie`.toLowerCase();
  const key = "pii.csv";
  const body = Buffer.from("ssn 987-65-4321\n");

  const enable = await signedJsonTarget("macie2", "Macie2.EnableMacie", {});
  assert.equal(enable.status, 200, enable.body);

  await s3.send(new CreateBucketCommand({ Bucket: bucket }));
  t.after(async () => {
    try {
      await s3.send(new DeleteObjectCommand({ Bucket: bucket, Key: key }));
    } catch {
      /* ignore */
    }
    try {
      await s3.send(new DeleteBucketCommand({ Bucket: bucket }));
    } catch {
      /* ignore */
    }
  });
  await s3.send(
    new PutObjectCommand({ Bucket: bucket, Key: key, Body: body }),
  );

  const inj = await signedJsonTarget(
    "macie2",
    "NoctaxrisMacie.InjectFindings",
    {
      S3Objects: [{ Bucket: bucket, Key: key }],
    },
  );
  assert.equal(inj.status, 200, inj.body);
  const findingIds = inj.json?.findingIds || [];
  assert.ok(findingIds.length >= 1, `InjectFindings findingIds empty: ${inj.body}`);

  const listed = await signedJsonTarget("macie2", "Macie2.ListFindings", {});
  assert.equal(listed.status, 200, listed.body);
  const listedIds = listed.json?.findingIds || [];
  assert.ok(
    listedIds.includes(findingIds[0]),
    `ListFindings missing ${findingIds[0]}: ${listed.body}`,
  );

  const got = await signedJsonTarget("macie2", "Macie2.GetFindings", {
    findingIds,
  });
  assert.equal(got.status, 200, got.body);
  assert.ok(
    got.body.includes("SensitiveData"),
    `GetFindings missing SensitiveData: ${got.body}`,
  );
  assert.ok(
    (got.json?.findings || []).length >= 1,
    `GetFindings findings empty: ${got.body}`,
  );
});
