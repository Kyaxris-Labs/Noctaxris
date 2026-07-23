import { test } from "node:test";
import assert from "node:assert/strict";
import {
  CreateTableCommand,
  DeleteItemCommand,
  DeleteTableCommand,
  DescribeTableCommand,
  GetItemCommand,
  PutItemCommand,
} from "@aws-sdk/client-dynamodb";
import { newDDB, requireReady, uniquePrefix } from "../lib/helpers.mjs";

test("DynamoDB table item round-trip", async (t) => {
  if (!(await requireReady(t))) return;
  const client = newDDB();
  const table = `${uniquePrefix()}-ddb`;

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

  const desc = await client.send(new DescribeTableCommand({ TableName: table }));
  assert.equal(desc.Table?.TableName, table);

  await client.send(
    new PutItemCommand({
      TableName: table,
      Item: {
        pk: { S: "1" },
        data: { S: "sdk-ddb" },
      },
    }),
  );
  const got = await client.send(
    new GetItemCommand({
      TableName: table,
      Key: { pk: { S: "1" } },
    }),
  );
  assert.equal(got.Item?.data?.S, "sdk-ddb");

  await client.send(
    new DeleteItemCommand({
      TableName: table,
      Key: { pk: { S: "1" } },
    }),
  );
  await client.send(new DeleteTableCommand({ TableName: table }));
});
