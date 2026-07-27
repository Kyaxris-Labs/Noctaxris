import { test } from "node:test";
import assert from "node:assert/strict";
import {
  CreateApplicationCommand,
  CreateConfigurationProfileCommand,
  CreateEnvironmentCommand,
  CreateHostedConfigurationVersionCommand,
  GetConfigurationCommand,
  StartDeploymentCommand,
} from "@aws-sdk/client-appconfig";
import {
  CreateBucketCommand,
  DeleteBucketCommand,
  DeleteObjectCommand,
  GetObjectCommand,
  ListObjectsV2Command,
  PutObjectCommand,
} from "@aws-sdk/client-s3";
import {
  CreateCrawlerCommand,
  CreateDatabaseCommand,
  GetCrawlerCommand,
  GetTableCommand,
  StartCrawlerCommand,
} from "@aws-sdk/client-glue";
import {
  PutConfigurationRecorderCommand,
  PutDeliveryChannelCommand,
  StartConfigurationRecorderCommand,
} from "@aws-sdk/client-config-service";
import {
  newAppConfig,
  newConfig,
  newGlue,
  newS3,
  requireReady,
  signedFetch,
  uniquePrefix,
} from "../lib/helpers.mjs";

test("Glue CreateCrawler StartCrawler GetTable from S3 CSV", async (t) => {
  if (!(await requireReady(t))) return;
  const s3 = newS3();
  const glue = newGlue();
  const prefix = uniquePrefix();
  const bucket = `${prefix}-glue`.toLowerCase();
  const dbName = `${prefix.replace(/-/g, "_")}_db`.slice(0, 48);
  const crawlerName = `${prefix}-crawler`.slice(0, 64);
  const csvKey = "data/people.csv";

  await s3.send(new CreateBucketCommand({ Bucket: bucket }));
  t.after(async () => {
    try {
      await s3.send(new DeleteObjectCommand({ Bucket: bucket, Key: csvKey }));
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
      Key: csvKey,
      Body: Buffer.from("id,name\n1,alice\n"),
    }),
  );

  await glue.send(
    new CreateDatabaseCommand({
      DatabaseInput: { Name: dbName },
    }),
  );
  await glue.send(
    new CreateCrawlerCommand({
      Name: crawlerName,
      DatabaseName: dbName,
      Targets: {
        S3Targets: [{ Path: `s3://${bucket}/data/` }],
      },
    }),
  );
  await glue.send(new StartCrawlerCommand({ Name: crawlerName }));

  const crawler = await glue.send(
    new GetCrawlerCommand({ Name: crawlerName }),
  );
  assert.equal(crawler.Crawler?.State, "READY", JSON.stringify(crawler.Crawler));

  const table = await glue.send(
    new GetTableCommand({ DatabaseName: dbName, Name: "people" }),
  );
  const cols = table.Table?.StorageDescriptor?.Columns || table.Table?.Columns || [];
  const names = cols.map((c) => c.Name);
  assert.ok(names.includes("id"), `columns=${JSON.stringify(cols)}`);
  assert.ok(names.includes("name"), `columns=${JSON.stringify(cols)}`);
});

test("AppConfig StartDeployment + GetConfiguration", async (t) => {
  if (!(await requireReady(t))) return;
  const appconfig = newAppConfig();
  const prefix = uniquePrefix();

  const app = await appconfig.send(
    new CreateApplicationCommand({ Name: `${prefix}-app` }),
  );
  const appId = app.Id;
  assert.ok(appId);

  const env = await appconfig.send(
    new CreateEnvironmentCommand({
      ApplicationId: appId,
      Name: "dev",
    }),
  );
  const envId = env.Id;
  assert.ok(envId);

  const profile = await appconfig.send(
    new CreateConfigurationProfileCommand({
      ApplicationId: appId,
      Name: "flags",
      LocationUri: "hosted",
    }),
  );
  const profileId = profile.Id;
  assert.ok(profileId);

  const content = Buffer.from(JSON.stringify({ feature: true }));
  const hosted = await appconfig.send(
    new CreateHostedConfigurationVersionCommand({
      ApplicationId: appId,
      ConfigurationProfileId: profileId,
      ContentType: "application/json",
      Content: content,
    }),
  );
  const version = String(hosted.VersionNumber);
  assert.ok(version && version !== "undefined");

  const dep = await appconfig.send(
    new StartDeploymentCommand({
      ApplicationId: appId,
      EnvironmentId: envId,
      ConfigurationProfileId: profileId,
      ConfigurationVersion: version,
      DeploymentStrategyId: "AppConfig.AllAtOnce",
    }),
  );
  assert.ok(
    dep.State === "DEPLOYED" || dep.DeploymentNumber != null || dep.Id,
    JSON.stringify(dep),
  );

  const got = await appconfig.send(
    new GetConfigurationCommand({
      Application: appId,
      Environment: envId,
      Configuration: profileId,
      ClientId: "lab",
    }),
  );
  const raw = Buffer.from(got.Content || []).toString("utf8");
  assert.equal(raw, '{"feature":true}');
});

test("Config StartConfigurationRecorder + S3 snapshot GetObject", async (t) => {
  if (!(await requireReady(t))) return;
  const s3 = newS3();
  const config = newConfig();
  const prefix = uniquePrefix();
  const bucket = `${prefix}-config`.toLowerCase();
  const recorder = `${prefix}-rec`.slice(0, 64);

  await s3.send(new CreateBucketCommand({ Bucket: bucket }));
  t.after(async () => {
    try {
      const listed = await s3.send(
        new ListObjectsV2Command({ Bucket: bucket, Prefix: "AWSLogs/" }),
      );
      for (const obj of listed.Contents || []) {
        if (!obj.Key) continue;
        try {
          await s3.send(
            new DeleteObjectCommand({ Bucket: bucket, Key: obj.Key }),
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
  });

  await config.send(
    new PutConfigurationRecorderCommand({
      ConfigurationRecorder: { name: recorder },
    }),
  );
  await config.send(
    new PutDeliveryChannelCommand({
      DeliveryChannel: { name: recorder, s3BucketName: bucket },
    }),
  );
  await config.send(
    new StartConfigurationRecorderCommand({
      ConfigurationRecorderName: recorder,
    }),
  );

  const listed = await s3.send(
    new ListObjectsV2Command({ Bucket: bucket, Prefix: "AWSLogs/" }),
  );
  const snap = (listed.Contents || []).find((o) =>
    (o.Key || "").includes(`noctaxris-config-snapshot-${recorder}-`),
  );
  assert.ok(snap?.Key, `missing snapshot under AWSLogs/: ${JSON.stringify(listed.Contents)}`);

  const obj = await s3.send(
    new GetObjectCommand({ Bucket: bucket, Key: snap.Key }),
  );
  const body = Buffer.from(await obj.Body.transformToByteArray()).toString(
    "utf8",
  );
  assert.ok(body.length > 0, "empty snapshot");
  JSON.parse(body);

  const tracked = `${prefix}-tracked`.toLowerCase();
  await s3.send(new CreateBucketCommand({ Bucket: tracked }));
  t.after(async () => {
    try {
      await s3.send(new DeleteBucketCommand({ Bucket: tracked }));
    } catch {
      /* ignore */
    }
  });

  // Lab Config history is query/XML (see server config handlers), not AWS JSON.
  const histParams = new URLSearchParams({
    Action: "GetResourceConfigHistory",
    Version: "2014-11-12",
    resourceType: "AWS::S3::Bucket",
    resourceId: tracked,
  });
  let histResp = await signedFetch(
    "config",
    "POST",
    "/",
    histParams.toString(),
    "application/x-www-form-urlencoded",
  );
  let histXML = await histResp.text();
  assert.equal(histResp.status, 200, histXML);
  assert.ok(histXML.includes(tracked), histXML);
  assert.ok(
    histXML.includes("<configurationItemStatus>OK</configurationItemStatus>"),
    histXML,
  );

  await s3.send(new DeleteBucketCommand({ Bucket: tracked }));

  histResp = await signedFetch(
    "config",
    "POST",
    "/",
    histParams.toString(),
    "application/x-www-form-urlencoded",
  );
  histXML = await histResp.text();
  assert.equal(histResp.status, 200, histXML);
  assert.ok(
    histXML.includes(
      "<configurationItemStatus>ResourceDeleted</configurationItemStatus>",
    ),
    histXML,
  );
});
