import { test } from "node:test";
import assert from "node:assert/strict";
import {
  CreateBucketCommand,
  DeleteBucketCommand,
  DeleteObjectCommand,
  GetObjectCommand,
  ListObjectsV2Command,
} from "@aws-sdk/client-s3";
import {
  newS3,
  requireReady,
  signedJsonTarget,
  uniquePrefix,
} from "../lib/helpers.mjs";

async function streamToString(body) {
  if (!body) return "";
  if (typeof body.transformToString === "function") {
    return body.transformToString();
  }
  const chunks = [];
  for await (const chunk of body) {
    chunks.push(chunk);
  }
  return Buffer.concat(chunks).toString("utf8");
}

test("VPC Flow CreateFlowLogs InjectFlowLogs S3 ACCEPT REJECT", async (t) => {
  if (!(await requireReady(t))) return;
  if (process.env.NOCTAXRIS_VPCFLOW_INJECT !== "1") {
    t.skip(
      "set NOCTAXRIS_VPCFLOW_INJECT=1 on the API process for lab InjectFlowLogs",
    );
    return;
  }

  const s3 = newS3();
  const prefix = uniquePrefix();
  const bucket = `${prefix}-vpcflow`.toLowerCase();

  await s3.send(new CreateBucketCommand({ Bucket: bucket }));
  t.after(async () => {
    try {
      const listed = await s3.send(
        new ListObjectsV2Command({ Bucket: bucket }),
      );
      for (const obj of listed.Contents || []) {
        await s3.send(
          new DeleteObjectCommand({ Bucket: bucket, Key: obj.Key }),
        );
      }
    } catch {
      // best-effort cleanup
    }
    try {
      await s3.send(new DeleteBucketCommand({ Bucket: bucket }));
    } catch {
      // best-effort cleanup
    }
  });

  const created = await signedJsonTarget("ec2", "AmazonEC2.CreateFlowLogs", {
    ResourceIds: ["vpc-labopaque001"],
    ResourceType: "VPC",
    TrafficType: "ALL",
    LogDestinationType: "s3",
    LogDestination: `arn:aws:s3:::${bucket}`,
  });
  assert.equal(
    created.status,
    200,
    `CreateFlowLogs status=${created.status} body=${created.body}`,
  );
  const flowLogIds = created.json?.FlowLogIds || [];
  assert.equal(flowLogIds.length, 1);
  const flowLogId = flowLogIds[0];
  assert.ok(
    String(flowLogId).startsWith("fl-"),
    `FlowLogId=${flowLogId} want fl- prefix`,
  );

  const injected = await signedJsonTarget(
    "ec2",
    "NoctaxrisEC2.InjectFlowLogs",
    { FlowLogId: flowLogId },
  );
  assert.equal(
    injected.status,
    200,
    `InjectFlowLogs status=${injected.status} body=${injected.body}`,
  );

  const listed = await s3.send(
    new ListObjectsV2Command({ Bucket: bucket, Prefix: "AWSLogs/" }),
  );
  assert.ok((listed.Contents || []).length > 0, "no flow log object in S3");
  const key = listed.Contents[0].Key;
  const obj = await s3.send(
    new GetObjectCommand({ Bucket: bucket, Key: key }),
  );
  const text = await streamToString(obj.Body);
  assert.ok(text.includes(" ACCEPT OK"), `s3 body missing ACCEPT: ${text}`);
  assert.ok(text.includes(" REJECT OK"), `s3 body missing REJECT: ${text}`);
});
