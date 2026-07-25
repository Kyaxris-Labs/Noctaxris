import { test } from "node:test";
import assert from "node:assert/strict";
import {
  CreateBucketCommand,
  DeleteBucketCommand,
  DeleteObjectCommand,
  PutObjectCommand,
} from "@aws-sdk/client-s3";
import {
  newS3,
  requireReady,
  signedFetch,
  signedJsonTarget,
  uniquePrefix,
} from "../lib/helpers.mjs";

test("CloudFront CreateDistribution Deployed + optional edge GET", async (t) => {
  if (!(await requireReady(t))) return;
  const s3 = newS3();
  const prefix = uniquePrefix();
  const bucket = `${prefix}-cf-origin`.toLowerCase();
  const key = "docs/hi.txt";
  const payload = "edge-object-bytes";

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
    new PutObjectCommand({
      Bucket: bucket,
      Key: key,
      Body: Buffer.from(payload),
      ContentType: "text/plain",
    }),
  );

  const create = await signedJsonTarget(
    "cloudfront",
    "CloudFront_2016_01_28.CreateDistribution",
    {
      DistributionConfig: {
        CallerReference: `${prefix}-cf-ref`,
        Comment: "lab",
        Enabled: true,
        Origins: {
          Items: [{ Id: "o1", DomainName: bucket, OriginType: "s3" }],
        },
      },
    },
  );
  assert.equal(create.status, 200, create.body);
  const dist = create.json?.Distribution;
  assert.ok(dist?.Id, "missing Distribution.Id");
  assert.equal(dist.Status, "Deployed");
  assert.ok(
    dist.DomainName && String(dist.DomainName).includes("cloudfront"),
    `DomainName=${dist.DomainName}`,
  );

  const edge = await signedFetch(
    "cloudfront",
    "GET",
    `/cloudfront/${dist.Id}/${key}`,
  );
  const edgeBody = await edge.text();
  assert.equal(edge.status, 200, edgeBody);
  assert.equal(edgeBody, payload);
});

test("Transfer CreateServer ONLINE + CreateUser + lab PutFile/GetFile", async (t) => {
  if (!(await requireReady(t))) return;
  const prefix = uniquePrefix();
  const userName = "alice";

  const create = await signedJsonTarget(
    "transfer",
    "TransferService.CreateServer",
    { Protocols: ["SFTP"] },
  );
  assert.equal(create.status, 200, create.body);
  const serverId = create.json?.ServerId;
  assert.ok(serverId, "missing ServerId");
  t.after(async () => {
    try {
      await signedJsonTarget("transfer", "TransferService.DeleteUser", {
        ServerId: serverId,
        UserName: userName,
      });
    } catch {
      /* ignore */
    }
    try {
      await signedJsonTarget("transfer", "TransferService.DeleteServer", {
        ServerId: serverId,
      });
    } catch {
      /* ignore */
    }
  });

  const desc = await signedJsonTarget(
    "transfer",
    "TransferService.DescribeServer",
    { ServerId: serverId },
  );
  assert.equal(desc.status, 200, desc.body);
  const state = desc.json?.Server?.State ?? desc.json?.State;
  assert.equal(state, "ONLINE", desc.body);

  const user = await signedJsonTarget("transfer", "TransferService.CreateUser", {
    ServerId: serverId,
    UserName: userName,
  });
  assert.equal(user.status, 200, user.body);

  // Lab file API: TransferService.PutFile / GetFile (also available as
  // PUT/GET /transfer/{serverId}/home/{user}/path with SigV4 transfer).
  const relPath = `inbox/${prefix}.txt`;
  const put = await signedJsonTarget("transfer", "TransferService.PutFile", {
    ServerId: serverId,
    UserName: userName,
    Path: relPath,
    Body: "hello-transfer",
  });
  assert.equal(put.status, 200, put.body);

  const get = await signedJsonTarget("transfer", "TransferService.GetFile", {
    ServerId: serverId,
    UserName: userName,
    Path: relPath,
  });
  assert.equal(get.status, 200, get.body);
  const b64 = get.json?.BodyBase64;
  assert.ok(b64, "missing BodyBase64");
  assert.equal(Buffer.from(b64, "base64").toString("utf8"), "hello-transfer");

  const pathPut = await signedFetch(
    "transfer",
    "PUT",
    `/transfer/${serverId}/home/${userName}/path-style.txt`,
    "via-path",
    "application/octet-stream",
  );
  const pathPutBody = await pathPut.text();
  assert.equal(pathPut.status, 200, pathPutBody);
  const pathGet = await signedFetch(
    "transfer",
    "GET",
    `/transfer/${serverId}/home/${userName}/path-style.txt`,
  );
  const pathGetBody = await pathGet.text();
  assert.equal(pathGet.status, 200, pathGetBody);
  assert.equal(pathGetBody, "via-path");
});

test("Route53 Alias to CloudFront DomainName", async (t) => {
  if (!(await requireReady(t))) return;
  const s3 = newS3();
  const prefix = uniquePrefix();
  const bucket = `${prefix}-r53-cf`.toLowerCase();
  const zoneName = `${prefix.replace(/_/g, "-")}.example.com`;

  await s3.send(new CreateBucketCommand({ Bucket: bucket }));
  t.after(async () => {
    try {
      await s3.send(new DeleteBucketCommand({ Bucket: bucket }));
    } catch {
      /* ignore */
    }
  });

  const cf = await signedJsonTarget(
    "cloudfront",
    "CloudFront_2016_01_28.CreateDistribution",
    {
      DistributionConfig: {
        CallerReference: `${prefix}-r53-cf`,
        Comment: "lab",
        Enabled: true,
        Origins: {
          Items: [{ Id: "o1", DomainName: bucket, OriginType: "s3" }],
        },
      },
    },
  );
  assert.equal(cf.status, 200, cf.body);
  const domain = cf.json?.Distribution?.DomainName;
  assert.ok(domain, "missing CloudFront DomainName");

  const zone = await signedJsonTarget(
    "route53",
    "AWSRoute53.CreateHostedZone",
    {
      Name: zoneName,
      CallerReference: `${prefix}-hz`,
    },
  );
  assert.equal(zone.status, 200, zone.body);
  let zoneId = zone.json?.HostedZone?.Id || "";
  zoneId = zoneId.replace(/^\/hostedzone\//, "");
  assert.ok(zoneId, "missing HostedZone.Id");

  const change = await signedJsonTarget(
    "route53",
    "AWSRoute53.ChangeResourceRecordSets",
    {
      HostedZoneId: zoneId,
      ChangeBatch: {
        Changes: [
          {
            Action: "CREATE",
            ResourceRecordSet: {
              Name: `www.${zoneName}`,
              Type: "A",
              AliasTarget: {
                DNSName: domain,
                HostedZoneId: "Z2FDTNDATAQYW2",
                EvaluateTargetHealth: false,
              },
            },
          },
        ],
      },
    },
  );
  assert.equal(change.status, 200, change.body);

  const list = await signedJsonTarget(
    "route53",
    "AWSRoute53.ListResourceRecordSets",
    { HostedZoneId: zoneId },
  );
  assert.equal(list.status, 200, list.body);
  assert.match(list.body, /AliasTarget/i);
  assert.ok(
    list.body.toLowerCase().includes(String(domain).toLowerCase()),
    `list missing domain ${domain}: ${list.body}`,
  );
});
