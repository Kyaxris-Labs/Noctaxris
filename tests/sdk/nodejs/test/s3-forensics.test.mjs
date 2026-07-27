import { test } from "node:test";
import assert from "node:assert/strict";
import {
  CreateBucketCommand,
  DeleteBucketCommand,
  DeleteObjectCommand,
  GetBucketLoggingCommand,
  GetObjectCommand,
  ListObjectVersionsCommand,
  ListObjectsV2Command,
  PutBucketLoggingCommand,
  PutBucketVersioningCommand,
  PutObjectCommand,
} from "@aws-sdk/client-s3";
import { newS3, requireReady, uniquePrefix } from "../lib/helpers.mjs";

async function emptyAndDeleteBucket(s3, bucket) {
  try {
    const vers = await s3.send(
      new ListObjectVersionsCommand({ Bucket: bucket }),
    );
    for (const v of vers.Versions || []) {
      if (!v.Key) continue;
      try {
        await s3.send(
          new DeleteObjectCommand({
            Bucket: bucket,
            Key: v.Key,
            VersionId: v.VersionId,
            BypassGovernanceRetention: true,
          }),
        );
      } catch {
        /* ignore */
      }
    }
    for (const m of vers.DeleteMarkers || []) {
      if (!m.Key) continue;
      try {
        await s3.send(
          new DeleteObjectCommand({
            Bucket: bucket,
            Key: m.Key,
            VersionId: m.VersionId,
          }),
        );
      } catch {
        /* ignore */
      }
    }
  } catch {
    /* ignore */
  }
  try {
    const listed = await s3.send(new ListObjectsV2Command({ Bucket: bucket }));
    for (const obj of listed.Contents || []) {
      if (!obj.Key) continue;
      try {
        await s3.send(
          new DeleteObjectCommand({
            Bucket: bucket,
            Key: obj.Key,
            BypassGovernanceRetention: true,
          }),
        );
      } catch {
        /* ignore */
      }
    }
  } catch {
    /* ignore */
  }
  try {
    await s3.send(new DeleteBucketCommand({ Bucket: bucket }));
  } catch {
    /* ignore */
  }
}

test("S3 forensics Object Lock lite", async (t) => {
  if (!(await requireReady(t))) return;
  const s3 = newS3();
  const bucket = `${uniquePrefix()}-lock`.toLowerCase();
  const key = "locked.txt";

  await s3.send(
    new CreateBucketCommand({
      Bucket: bucket,
      ObjectLockEnabledForBucket: true,
    }),
  );
  t.after(() => emptyAndDeleteBucket(s3, bucket));

  const retain = new Date(Date.now() + 24 * 60 * 60 * 1000);
  await s3.send(
    new PutObjectCommand({
      Bucket: bucket,
      Key: key,
      Body: Buffer.from("secret"),
      ObjectLockMode: "GOVERNANCE",
      ObjectLockRetainUntilDate: retain,
    }),
  );

  await assert.rejects(
    () =>
      s3.send(
        new DeleteObjectCommand({
          Bucket: bucket,
          Key: key,
        }),
      ),
    /AccessDenied|Forbidden|denied/i,
  );

  await s3.send(
    new DeleteObjectCommand({
      Bucket: bucket,
      Key: key,
      BypassGovernanceRetention: true,
    }),
  );
});

test("S3 forensics bucket logging", async (t) => {
  if (!(await requireReady(t))) return;
  const s3 = newS3();
  const prefix = uniquePrefix();
  const src = `${prefix}-src`.toLowerCase();
  const logs = `${prefix}-logs`.toLowerCase();
  const key = "a.txt";

  await s3.send(new CreateBucketCommand({ Bucket: src }));
  await s3.send(new CreateBucketCommand({ Bucket: logs }));
  t.after(() => emptyAndDeleteBucket(s3, src));
  t.after(() => emptyAndDeleteBucket(s3, logs));

  await s3.send(
    new PutBucketLoggingCommand({
      Bucket: src,
      BucketLoggingStatus: {
        LoggingEnabled: {
          TargetBucket: logs,
          TargetPrefix: "s3/",
        },
      },
    }),
  );

  const got = await s3.send(new GetBucketLoggingCommand({ Bucket: src }));
  assert.equal(got.LoggingEnabled?.TargetBucket, logs);

  await s3.send(
    new PutObjectCommand({
      Bucket: src,
      Key: key,
      Body: Buffer.from("abc"),
    }),
  );

  const listed = await s3.send(
    new ListObjectsV2Command({ Bucket: logs, Prefix: "s3/" }),
  );
  assert.ok(
    (listed.Contents || []).length >= 1,
    `expected access log under s3/: ${JSON.stringify(listed.Contents)}`,
  );
  const obj = await s3.send(
    new GetObjectCommand({
      Bucket: logs,
      Key: listed.Contents[0].Key,
    }),
  );
  const line = Buffer.from(await obj.Body.transformToByteArray()).toString(
    "utf8",
  );
  assert.ok(line.includes("REST.PUT.OBJECT"), line);
  assert.ok(line.includes(key), line);
});

test("S3 forensics versioning delete markers", async (t) => {
  if (!(await requireReady(t))) return;
  const s3 = newS3();
  const bucket = `${uniquePrefix()}-ver`.toLowerCase();
  const key = "obj.txt";

  await s3.send(new CreateBucketCommand({ Bucket: bucket }));
  t.after(() => emptyAndDeleteBucket(s3, bucket));

  await s3.send(
    new PutBucketVersioningCommand({
      Bucket: bucket,
      VersioningConfiguration: { Status: "Enabled" },
    }),
  );

  const put = await s3.send(
    new PutObjectCommand({
      Bucket: bucket,
      Key: key,
      Body: Buffer.from("payload"),
    }),
  );
  assert.ok(put.VersionId, "PutObject missing VersionId");

  const del = await s3.send(
    new DeleteObjectCommand({ Bucket: bucket, Key: key }),
  );
  assert.equal(del.DeleteMarker, true);

  const listed = await s3.send(
    new ListObjectVersionsCommand({ Bucket: bucket, Prefix: key }),
  );
  assert.ok((listed.Versions || []).length >= 1, "missing object version");
  assert.ok(
    (listed.DeleteMarkers || []).length >= 1,
    "missing delete marker",
  );
});
