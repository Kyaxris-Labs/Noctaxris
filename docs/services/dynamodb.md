# DynamoDB

**Status:** shipped

Lab-complete DynamoDB: tables, item CRUD, Query/Scan (including up to two lab GSIs and two lab LSIs per table), BatchGet/BatchWrite, TransactWrite/TransactGet (same-account lab subset), PartiQL `ExecuteStatement` / `BatchExecuteStatement` lite, table resource policies, CMK encryption, and TTL with lazy expiry on read.

## Implemented

| Area | Actions |
|------|---------|
| Tables | `CreateTable`, `DescribeTable`, `DeleteTable`, `ListTables`, `UpdateTable` |
| Continuous backups | `DescribeContinuousBackups` (lab stub: continuous `ENABLED`, PITR `DISABLED`) |
| Tags | `ListTagsOfResource`, `TagResource`, `UntagResource` (table ARN via `ResourceArn`; `Tags` as `{Key,Value}`) |
| GSI | Up to two lab global secondary indexes per table (`CreateTable` or `UpdateTable` `Create` GSI updates) |
| LSI | Up to two lab local secondary indexes on `CreateTable` only (same HASH as table; alternate RANGE; table must have a RANGE key; `ProjectionType` ALL) |
| Items | `PutItem`, `GetItem`, `DeleteItem`, `UpdateItem` |
| Query / Scan | `Query`, `Scan` (base table and lab GSI/LSI via `IndexName`; sort-key `EQ`/`BETWEEN`/`begins_with`/comparisons on `KeyConditionExpression`; optional `FilterExpression` with the same lab subset as `ConditionExpression`, fail-closed on unsupported operators) |
| PartiQL | `ExecuteStatement`, `BatchExecuteStatement` (lab soft cap 25): `INSERT INTO … VALUE`, `SELECT * FROM … WHERE` key equality, `UPDATE … SET` key equality, `DELETE FROM … WHERE` key equality; `?` Parameters; fail closed on JOIN, nested SELECT, IN, GROUP BY, ORDER BY, LIMIT |
| Batch | `BatchGetItem`, `BatchWriteItem` (lab soft cap 25; overflow returned in `UnprocessedKeys` / `UnprocessedItems`) |
| Transactions | `TransactWriteItems` (`Put` / `Delete` / `Update` SET/REMOVE / `ConditionCheck` existence; optional `ClientRequestToken` idempotency, 10-minute window; stream append on successful Put/Delete/Update), `TransactGetItems` (same-account tables; lab soft cap 25; duplicate item keys cancel the write) |
| Resource policy | `PutResourcePolicy`, `GetResourcePolicy`, `DeleteResourcePolicy` |
| TTL | `UpdateTimeToLive`, `DescribeTimeToLive` (lazy expiry on `GetItem`, `Query`, `Scan`, and `BatchGetItem`; `Query`/`Scan` over-fetch until `Limit` live items) |
| Encryption | Table SSE with AWS-owned or customer-managed KMS |

Table and item metadata live in SQLite. Item ciphertext uses table SSE. Expired TTL items are omitted on read, not deleted in the background.

### Authz notes

DynamoDB uses `EvaluateDynamoDB` via `authorizeDataplaneOR` with the table owner account from the table ARN. Same-account access: allow if identity **or** table resource policy Allows. Cross-account access: allow only when identity **and** table resource policy both Allow. Empty resource policy denies cross-account callers. Explicit Deny in either wins. A resource policy alone can grant access in the same account (unlike KMS). Org SCP/RCP filters apply before evaluation. When identity Allows, permissions boundary and session intersect.

Pass a full table ARN as `TableName` for cross-account `GetItem` and similar item APIs. SSE-KMS tables resolve the CMK under the table owner account and require `kms:Decrypt` via EvaluateKMS (identity plus key policy), matching S3 SSE-KMS. Item DEK seal/unseal and KMS authz bind EncryptionContext `aws:dynamodb:tableName` and `aws:dynamodb:subscriberId` (table owner account id). `CreateTable` / `UpdateTable` that enable SSE-KMS also require caller `kms:DescribeKey` and `kms:CreateGrant` on that CMK with the same context (deny attach when the key policy lacks Allow). `PutResourcePolicy` requires every statement to name a `Principal`.

### Transactions notes

`TransactWriteItems` and `TransactGetItems` run against same-account lab tables only (cross-account table ARNs fail closed). Writes are all-or-nothing in a SQLite transaction; failed `ConditionCheck`, failed `ConditionExpression` on `Put`/`Delete`/`Update`, or duplicate primary keys in one request return `TransactionCanceledException` with `CancellationReasons`. Lab cap is 25 actions (AWS allows 100).

`Update` uses the same lab `UpdateExpression` SET/REMOVE subset as standalone `UpdateItem`. `Put` / `Delete` / `Update` honor the lab `ConditionExpression` subset (`attribute_exists` / `attribute_not_exists`, top-level comparisons, AND/OR); unsupported operators return `ValidationException` before apply. `ConditionCheck` remains existence / `attribute_exists(...)` only. Transact `Update` and body-reading conditions require unsealed (non-SSE-KMS ciphertext) items in this lab. Optional `ClientRequestToken` makes a successful write idempotent for 10 minutes; reuse with different parameters returns `IdempotentParameterMismatchException`. Successful Put/Delete/Update append DynamoDB Streams records when the table stream is enabled (see [dynamodbstreams.md](dynamodbstreams.md)). AWS 100-item limit and cross-account / XA transact targets are out of lab scope (lab soft cap 25; same-account only).

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
TABLE="noctaxris-lab-$RANDOM"
aws dynamodb create-table \
  --table-name "$TABLE" \
  --attribute-definitions AttributeName=pk,AttributeType=S \
  --key-schema AttributeName=pk,KeyType=HASH \
  --billing-mode PAY_PER_REQUEST \
  --endpoint-url "$EP"

aws dynamodb put-item \
  --table-name "$TABLE" \
  --item '{"pk":{"S":"1"},"data":{"S":"hello-ddb"}}' \
  --endpoint-url "$EP"

aws dynamodb get-item \
  --table-name "$TABLE" \
  --key '{"pk":{"S":"1"}}' \
  --endpoint-url "$EP"
```

GSI query and TTL:

```bash
TABLE="noctaxris-gsi-$RANDOM"
aws dynamodb create-table \
  --table-name "$TABLE" \
  --attribute-definitions AttributeName=pk,AttributeType=S AttributeName=gsi1pk,AttributeType=S \
  --key-schema AttributeName=pk,KeyType=HASH \
  --global-secondary-indexes '[{"IndexName":"Gsi1","KeySchema":[{"AttributeName":"gsi1pk","KeyType":"HASH"}],"Projection":{"ProjectionType":"ALL"}}]' \
  --billing-mode PAY_PER_REQUEST \
  --endpoint-url "$EP"
aws dynamodb update-time-to-live \
  --table-name "$TABLE" \
  --time-to-live-specification Enabled=true,AttributeName=expires \
  --endpoint-url "$EP"
```

Two-account cross-account GetItem (member account B owns the table, member account A user reads via dual eval):

```bash
TABLE_ARN=$(aws dynamodb create-table --table-name "$TABLE" \
  --attribute-definitions AttributeName=pk,AttributeType=S \
  --key-schema AttributeName=pk,KeyType=HASH \
  --billing-mode PAY_PER_REQUEST \
  --endpoint-url "$EP" --profile account-b --query TableDescription.TableArn --output text)
aws dynamodb put-item --table-name "$TABLE" --item '{"pk":{"S":"1"},"data":{"S":"xa"}}' \
  --endpoint-url "$EP" --profile account-b
aws dynamodb put-resource-policy --resource-arn "$TABLE_ARN" --endpoint-url "$EP" --profile account-b \
  --policy '{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::ACCOUNT_A:user/reader"},"Action":"dynamodb:GetItem","Resource":"*"}]}'
aws dynamodb get-item --table-name "$TABLE_ARN" --key '{"pk":{"S":"1"}}' \
  --endpoint-url "$EP" --profile account-a
```

Transactions:

```bash
TABLE="noctaxris-txn-$RANDOM"
aws dynamodb create-table \
  --table-name "$TABLE" \
  --attribute-definitions AttributeName=pk,AttributeType=S \
  --key-schema AttributeName=pk,KeyType=HASH \
  --billing-mode PAY_PER_REQUEST \
  --endpoint-url "$EP"

aws dynamodb transact-write-items --endpoint-url "$EP" --transact-items "[
  {\"Put\":{\"TableName\":\"$TABLE\",\"Item\":{\"pk\":{\"S\":\"a\"},\"v\":{\"S\":\"1\"}}}},
  {\"Put\":{\"TableName\":\"$TABLE\",\"Item\":{\"pk\":{\"S\":\"b\"},\"v\":{\"S\":\"2\"}}}}
]"

aws dynamodb put-item --table-name "$TABLE" --item '{"pk":{"S":"acct"},"balance":{"N":"10"}}' --endpoint-url "$EP"
aws dynamodb transact-write-items --endpoint-url "$EP" --transact-items "[
  {\"Update\":{\"TableName\":\"$TABLE\",\"Key\":{\"pk\":{\"S\":\"acct\"}},\"UpdateExpression\":\"SET balance = :n\",\"ConditionExpression\":\"balance = :old\",\"ExpressionAttributeValues\":{\":n\":{\"N\":\"20\"},\":old\":{\"N\":\"10\"}}}}
]"

aws dynamodb transact-get-items --endpoint-url "$EP" --transact-items "[
  {\"Get\":{\"TableName\":\"$TABLE\",\"Key\":{\"pk\":{\"S\":\"a\"}}}},
  {\"Get\":{\"TableName\":\"$TABLE\",\"Key\":{\"pk\":{\"S\":\"b\"}}}}
]"
```

LSI create + query:

```bash
TABLE="noctaxris-lsi-$RANDOM"
aws dynamodb create-table \
  --table-name "$TABLE" \
  --attribute-definitions AttributeName=pk,AttributeType=S AttributeName=sk,AttributeType=S AttributeName=status,AttributeType=S \
  --key-schema AttributeName=pk,KeyType=HASH AttributeName=sk,KeyType=RANGE \
  --local-secondary-indexes '[{"IndexName":"ByStatus","KeySchema":[{"AttributeName":"pk","KeyType":"HASH"},{"AttributeName":"status","KeyType":"RANGE"}],"Projection":{"ProjectionType":"ALL"}}]' \
  --billing-mode PAY_PER_REQUEST \
  --endpoint-url "$EP"
aws dynamodb put-item --table-name "$TABLE" --item '{"pk":{"S":"u1"},"sk":{"S":"o1"},"status":{"S":"OPEN"}}' --endpoint-url "$EP"
aws dynamodb query --table-name "$TABLE" --index-name ByStatus \
  --key-condition-expression "pk = :pk AND #s = :st" \
  --expression-attribute-names '{"#s":"status"}' \
  --expression-attribute-values '{":pk":{"S":"u1"},":st":{"S":"OPEN"}}' \
  --endpoint-url "$EP"
```

SDK LSI Query soft-skips when the API is down. Terraform: `STACK=lab-parity-observe` (`TF_PARITY=1` / `NOCTAXRIS_ADVANCED=1`).

PartiQL:

```bash
TABLE="noctaxris-partiql-$RANDOM"
aws dynamodb create-table \
  --table-name "$TABLE" \
  --attribute-definitions AttributeName=pk,AttributeType=S \
  --key-schema AttributeName=pk,KeyType=HASH \
  --billing-mode PAY_PER_REQUEST \
  --endpoint-url "$EP"
aws dynamodb execute-statement \
  --statement "INSERT INTO \"$TABLE\" VALUE {'pk':?,'data':?}" \
  --parameters '[{"S":"1"},{"S":"hello"}]' \
  --endpoint-url "$EP"
aws dynamodb execute-statement \
  --statement "SELECT * FROM \"$TABLE\" WHERE pk=?" \
  --parameters '[{"S":"1"}]' \
  --endpoint-url "$EP"
```

## Not yet / deferred

None for lab-core TransactWrite (stream view types and ESM OldImage filters: see [dynamodbstreams.md](dynamodbstreams.md) and [lambda.md](lambda.md)).

## Out of lab scope

- Full DynamoDB SAR beyond the lab set (more than two GSIs, more than two LSIs, Contributor Insights, export/import, global tables, UpdateContinuousBackups / live PITR restore, on-demand vs provisioned billing depth, full pagination parity, richer PartiQL) (out of lab scope; Transact lab subset + two GSIs + two LSIs + PartiQL lite cover marketed core)
- AWS 100-item TransactItems limit (lab soft cap 25), cross-account / XA transact, Transact Update/conditions on SSE-KMS sealed items, richer `ConditionCheck` beyond existence (out of lab scope)
