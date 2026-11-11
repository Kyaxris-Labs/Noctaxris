import { test } from "node:test";
import assert from "node:assert/strict";
import {
  CreateBucketCommand,
  DeleteBucketCommand,
  DeleteObjectCommand,
  GetObjectCommand,
  ListBucketsCommand,
  PutObjectCommand,
  S3Client,
} from "@aws-sdk/client-s3";
import { Sha256 } from "@aws-crypto/sha256-js";
import { HttpRequest } from "@smithy/protocol-http";
import { SignatureV4 } from "@smithy/signature-v4";
import { credentials, endpoint, newS3, region, uniquePrefix } from "../lib/helpers.mjs";

const SEED_PATH = "/_noctaxris/lab/accounts:seedStandalone";

async function reachabilityMessage() {
  const ep = endpoint();
  const url = `${ep}/_noctaxris/ready`;
  try {
    const resp = await fetch(url, { signal: AbortSignal.timeout(2000) });
    if (!resp.ok) {
      return `Noctaxris not reachable at ${ep}: status ${resp.status}`;
    }
  } catch (err) {
    return `Noctaxris not reachable at ${ep}: ${err}`;
  }
  return "";
}

function bucketNames(list) {
  return (list.Buckets || []).map((bucket) => bucket.Name);
}

function seedFailure(status, text) {
  if (!text || text.includes("secretAccessKey")) {
    return `seed status ${status}`;
  }
  return `seed status ${status} body ${text.slice(0, 400)}`;
}

async function seedStandalone(name) {
  const body = JSON.stringify({ name });
  const ep = new URL(endpoint());
  const req = new HttpRequest({
    method: "POST",
    protocol: ep.protocol,
    hostname: ep.hostname,
    port: ep.port ? Number(ep.port) : undefined,
    path: SEED_PATH,
    headers: {
      host: ep.host,
      "content-type": "application/json",
    },
    body,
  });
  // uriEscapePath stays off so the colon in the seed path is signed as ":".
  const signer = new SignatureV4({
    service: "iam",
    region: region(),
    credentials: credentials(),
    sha256: Sha256,
    uriEscapePath: false,
  });
  const signed = await signer.sign(req);
  const resp = await fetch(`${ep.origin}${SEED_PATH}`, {
    method: "POST",
    headers: signed.headers,
    body,
    signal: AbortSignal.timeout(30000),
  });
  const text = await resp.text();
  if (resp.status !== 200) {
    return { status: resp.status, json: null, text };
  }
  let json;
  try {
    json = JSON.parse(text);
  } catch {
    assert.fail("seed response was not JSON");
  }
  return { status: resp.status, json, text: "" };
}

function homeS3(accessKeyId, secretAccessKey) {
  return new S3Client({
    region: region(),
    endpoint: endpoint(),
    forcePathStyle: true,
    credentials: { accessKeyId, secretAccessKey },
  });
}

test("standalone account S3 isolation", async (t) => {
  const down = await reachabilityMessage();
  if (down) {
    t.skip(down);
    return;
  }

  const name = `sdk-node-${uniquePrefix()}`;
  const homeBucket = name.toLowerCase();
  const labBucket = `${name}-lab`.toLowerCase();
  const objectKey = "note.txt";
  const payload = Buffer.from("lab-root-object");

  const first = await seedStandalone(name);
  if (first.status === 404) {
    t.skip("NOCTAXRIS_LAB_STANDALONE is off");
    return;
  }
  assert.equal(first.status, 200, seedFailure(first.status, first.text));

  const accessKeyId = first.json?.accessKeyId;
  const secretAccessKey = first.json?.secretAccessKey;
  assert.equal(typeof accessKeyId, "string");
  assert.ok(accessKeyId, "seed response missing accessKeyId");
  assert.equal(typeof secretAccessKey, "string");
  assert.ok(secretAccessKey, "seed response missing secret");

  const home = homeS3(accessKeyId, secretAccessKey);
  const root = newS3();

  await home.send(new CreateBucketCommand({ Bucket: homeBucket }));
  t.after(async () => {
    try {
      await home.send(new DeleteBucketCommand({ Bucket: homeBucket }));
    } catch {
      /* ignore */
    }
  });

  const rootNames = bucketNames(await root.send(new ListBucketsCommand({})));
  assert.equal(
    rootNames.includes(homeBucket),
    false,
    `root ListBuckets includes ${homeBucket}`,
  );

  const homeNames = bucketNames(await home.send(new ListBucketsCommand({})));
  assert.equal(
    homeNames.includes(homeBucket),
    true,
    `home ListBuckets missing ${homeBucket}`,
  );

  await root.send(new CreateBucketCommand({ Bucket: labBucket }));
  t.after(async () => {
    try {
      await root.send(
        new DeleteObjectCommand({ Bucket: labBucket, Key: objectKey }),
      );
    } catch {
      /* ignore */
    }
    try {
      await root.send(new DeleteBucketCommand({ Bucket: labBucket }));
    } catch {
      /* ignore */
    }
  });
  await root.send(
    new PutObjectCommand({ Bucket: labBucket, Key: objectKey, Body: payload }),
  );

  let returnedObject = false;
  try {
    const got = await home.send(
      new GetObjectCommand({ Bucket: labBucket, Key: objectKey }),
    );
    const bytes = Buffer.from(await got.Body.transformToByteArray());
    returnedObject = bytes.equals(payload);
  } catch {
    returnedObject = false;
  }
  assert.equal(
    returnedObject,
    false,
    "home GetObject returned the root object",
  );

  const second = await seedStandalone(name);
  assert.equal(second.status, 200, seedFailure(second.status, second.text));
  assert.equal(second.json?.accessKeyId, accessKeyId);
});
