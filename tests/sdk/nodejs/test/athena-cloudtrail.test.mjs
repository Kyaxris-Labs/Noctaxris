import { test } from "node:test";
import assert from "node:assert/strict";
import {
  CreateDatabaseCommand,
  CreateTableCommand,
  DeleteDatabaseCommand,
  DeleteTableCommand,
} from "@aws-sdk/client-glue";
import {
  CreateBucketCommand,
  DeleteBucketCommand,
  DeleteObjectCommand,
  PutObjectCommand,
} from "@aws-sdk/client-s3";
import {
  newGlue,
  newS3,
  requireReady,
  signedJsonTarget,
  uniquePrefix,
} from "../lib/helpers.mjs";

async function athenaQueryRows(query, database) {
  const start = await signedJsonTarget(
    "athena",
    "AmazonAthena.StartQueryExecution",
    {
      QueryString: query,
      QueryExecutionContext: { Database: database },
    },
  );
  assert.equal(start.status, 200, start.body);
  const qid = start.json?.QueryExecutionId;
  assert.ok(qid, start.body);

  const exec = await signedJsonTarget(
    "athena",
    "AmazonAthena.GetQueryExecution",
    { QueryExecutionId: qid },
  );
  assert.equal(exec.status, 200, exec.body);
  assert.equal(
    exec.json?.QueryExecution?.Status?.State,
    "SUCCEEDED",
    exec.body,
  );

  const results = await signedJsonTarget(
    "athena",
    "AmazonAthena.GetQueryResults",
    { QueryExecutionId: qid },
  );
  assert.equal(results.status, 200, results.body);
  const rows = results.json?.ResultSet?.Rows || [];
  return rows.map((r) =>
    (r.Data || []).map((d) => d.VarCharValue || ""),
  );
}

test("Athena CloudTrail Records unwrap LIKE json_extract", async (t) => {
  if (!(await requireReady(t))) return;
  const s3 = newS3();
  const glue = newGlue();
  const prefix = uniquePrefix();
  const bucket = `${prefix}-athct`.toLowerCase();
  let dbName = `${prefix.replace(/-/g, "_")}_ctdb`;
  if (dbName.length > 48) dbName = dbName.slice(0, 48);
  const tableName = "events";
  const objKey = "trail/delivery.json";

  const payload = JSON.stringify({
    Records: [
      {
        eventName: "AssumeRole",
        eventID: "e1",
        userIdentity: { type: "IAMUser", userName: "alice" },
      },
      {
        eventName: "PutObject",
        eventID: "e2",
        userIdentity: { type: "AWSService", userName: "s3" },
      },
      {
        eventName: "AssumeRoleWithSAML",
        eventID: "e3",
        userIdentity: { type: "IAMUser", userName: "bob" },
      },
    ],
  });

  await s3.send(new CreateBucketCommand({ Bucket: bucket }));
  t.after(async () => {
    try {
      await glue.send(
        new DeleteTableCommand({ DatabaseName: dbName, Name: tableName }),
      );
    } catch {
      /* ignore */
    }
    try {
      await glue.send(new DeleteDatabaseCommand({ Name: dbName }));
    } catch {
      /* ignore */
    }
    try {
      await s3.send(
        new DeleteObjectCommand({ Bucket: bucket, Key: objKey }),
      );
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
      Key: objKey,
      Body: payload,
    }),
  );
  await glue.send(
    new CreateDatabaseCommand({ DatabaseInput: { Name: dbName } }),
  );
  await glue.send(
    new CreateTableCommand({
      DatabaseName: dbName,
      TableInput: {
        Name: tableName,
        StorageDescriptor: {
          Location: `s3://${bucket}/trail/`,
          Columns: [
            { Name: "eventName", Type: "string" },
            { Name: "eventID", Type: "string" },
            { Name: "userIdentity", Type: "string" },
          ],
          InputFormat: "org.apache.hive.hcatalog.data.JsonSerDe",
          SerdeInfo: {
            SerializationLibrary: "org.openx.data.jsonserde.JsonSerDe",
          },
        },
      },
    }),
  );

  const unwrap = await athenaQueryRows(
    `SELECT eventName, eventID FROM ${dbName}.events ORDER BY eventID`,
    dbName,
  );
  assert.equal(unwrap.length, 4, JSON.stringify(unwrap));
  assert.equal(unwrap[1][0], "AssumeRole");
  assert.equal(unwrap[3][0], "AssumeRoleWithSAML");

  const likeRows = await athenaQueryRows(
    `SELECT eventName FROM ${dbName}.events WHERE eventName LIKE 'Assume%'`,
    dbName,
  );
  assert.equal(likeRows.length, 3, JSON.stringify(likeRows));
  const likeJoined = `${likeRows[1][0]}${likeRows[2][0]}`;
  assert.match(likeJoined, /AssumeRole/);
  assert.match(likeJoined, /AssumeRoleWithSAML/);

  const jxRows = await athenaQueryRows(
    `SELECT eventName FROM ${dbName}.events WHERE json_extract(userIdentity, '$.type') = 'IAMUser'`,
    dbName,
  );
  assert.equal(jxRows.length, 3, JSON.stringify(jxRows));
  const jxJoined = `${jxRows[1][0]}${jxRows[2][0]}`;
  assert.match(jxJoined, /AssumeRole/);
  assert.match(jxJoined, /AssumeRoleWithSAML/);
});
