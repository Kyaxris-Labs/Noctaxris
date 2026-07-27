import { test } from "node:test";
import assert from "node:assert/strict";
import {
  CreateTableCommand,
  DeleteTableCommand,
  DescribeTableCommand,
  ExecuteStatementCommand,
  PutItemCommand,
  QueryCommand,
} from "@aws-sdk/client-dynamodb";
import { newDDB, requireReady, uniquePrefix } from "../lib/helpers.mjs";

test("DynamoDB LSI query", async (t) => {
  if (!(await requireReady(t))) return;
  const client = newDDB();
  const table = `${uniquePrefix()}-lsi`;

  await client.send(
    new CreateTableCommand({
      TableName: table,
      AttributeDefinitions: [
        { AttributeName: "pk", AttributeType: "S" },
        { AttributeName: "sk", AttributeType: "S" },
        { AttributeName: "status", AttributeType: "S" },
      ],
      KeySchema: [
        { AttributeName: "pk", KeyType: "HASH" },
        { AttributeName: "sk", KeyType: "RANGE" },
      ],
      LocalSecondaryIndexes: [
        {
          IndexName: "ByStatus",
          KeySchema: [
            { AttributeName: "pk", KeyType: "HASH" },
            { AttributeName: "status", KeyType: "RANGE" },
          ],
          Projection: { ProjectionType: "ALL" },
        },
      ],
      BillingMode: "PAY_PER_REQUEST",
    }),
  );
  t.after(async () => {
    try {
      await client.send(new DeleteTableCommand({ TableName: table }));
    } catch {
      /* ignore */
    }
  });

  const desc = await client.send(new DescribeTableCommand({ TableName: table }));
  assert.equal(desc.Table?.LocalSecondaryIndexes?.length, 1);

  await client.send(
    new PutItemCommand({
      TableName: table,
      Item: {
        pk: { S: "u1" },
        sk: { S: "o1" },
        status: { S: "OPEN" },
      },
    }),
  );

  const out = await client.send(
    new QueryCommand({
      TableName: table,
      IndexName: "ByStatus",
      KeyConditionExpression: "pk = :pk AND #s = :st",
      ExpressionAttributeNames: { "#s": "status" },
      ExpressionAttributeValues: {
        ":pk": { S: "u1" },
        ":st": { S: "OPEN" },
      },
    }),
  );
  assert.equal(out.Items?.length, 1);
  assert.equal(out.Items[0].pk.S, "u1");
  assert.equal(out.Items[0].status.S, "OPEN");
});

test("DynamoDB PartiQL ExecuteStatement", async (t) => {
  if (!(await requireReady(t))) return;
  const client = newDDB();
  const table = `${uniquePrefix()}-partiql`;

  await client.send(
    new CreateTableCommand({
      TableName: table,
      AttributeDefinitions: [{ AttributeName: "pk", AttributeType: "S" }],
      KeySchema: [{ AttributeName: "pk", KeyType: "HASH" }],
      BillingMode: "PAY_PER_REQUEST",
    }),
  );
  t.after(async () => {
    try {
      await client.send(new DeleteTableCommand({ TableName: table }));
    } catch {
      /* ignore */
    }
  });

  await client.send(
    new ExecuteStatementCommand({
      Statement: `INSERT INTO "${table}" VALUE {'pk':?,'data':?}`,
      Parameters: [{ S: "1" }, { S: "partiql" }],
    }),
  );
  const sel = await client.send(
    new ExecuteStatementCommand({
      Statement: `SELECT * FROM "${table}" WHERE pk=?`,
      Parameters: [{ S: "1" }],
    }),
  );
  assert.equal(sel.Items?.length, 1);
  assert.equal(sel.Items[0].data.S, "partiql");
});
