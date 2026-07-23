import { test } from "node:test";
import assert from "node:assert/strict";
import {
  CreateBucketCommand,
  DeleteBucketCommand,
  DeleteObjectCommand,
  GetObjectCommand,
  ListBucketsCommand,
  PutObjectCommand,
} from "@aws-sdk/client-s3";
import { newS3, requireReady, uniquePrefix } from "../lib/helpers.mjs";

test("S3 bucket object round-trip", async (t) => {
  if (!(await requireReady(t))) return;
  const client = newS3();
  const bucket = `${uniquePrefix()}-bucket`.toLowerCase();
  const key = "hello.txt";
  const body = Buffer.from("noctaxris-sdk-s3");

  await client.send(new CreateBucketCommand({ Bucket: bucket }));
  t.after(async () => {
    try {
      await client.send(new DeleteObjectCommand({ Bucket: bucket, Key: key }));
    } catch {
      /* ignore */
    }
    try {
      await client.send(new DeleteBucketCommand({ Bucket: bucket }));
    } catch {
      /* ignore */
    }
  });

  const list = await client.send(new ListBucketsCommand({}));
  const names = (list.Buckets || []).map((b) => b.Name);
  assert.ok(names.includes(bucket), `ListBuckets missing ${bucket}`);

  await client.send(
    new PutObjectCommand({ Bucket: bucket, Key: key, Body: body }),
  );
  const got = await client.send(new GetObjectCommand({ Bucket: bucket, Key: key }));
  const bytes = Buffer.from(await got.Body.transformToByteArray());
  assert.deepEqual(bytes, body);

  await client.send(new DeleteObjectCommand({ Bucket: bucket, Key: key }));
  await assert.rejects(() =>
    client.send(new GetObjectCommand({ Bucket: bucket, Key: key })),
  );
});
