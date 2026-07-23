# DynamoDB

**Status:** shipped

Lab-complete DynamoDB: tables, item CRUD, Query/Scan (including up to two lab GSIs per table), BatchGet/BatchWrite, table resource policies, CMK encryption, and TTL with lazy expiry on read.

## Implemented

| Area | Actions |
|------|---------|
| Tables | `CreateTable`, `DescribeTable`, `DeleteTable`, `ListTables`, `UpdateTable` |
| Continuous backups | `DescribeContinuousBackups` (lab stub: continuous `ENABLED`, PITR `DISABLED`) |
| Tags | `ListTagsOfResource`, `TagResource`, `UntagResource` (table ARN via `ResourceArn`; `Tags` as `{Key,Value}`) |
| GSI | Up to two lab global secondary indexes per table (`CreateTable` or `UpdateTable` `Create` GSI updates) |
| Items | `PutItem`, `GetItem`, `DeleteItem`, `UpdateItem` |
| Query / Scan | `Query`, `Scan` (base table and lab GSIs via `IndexName`; sort-key `EQ`/`BETWEEN`/`begins_with`/comparisons on `KeyConditionExpression`; optional `FilterExpression` with the same lab subset as `ConditionExpression`, fail-closed on unsupported operators) |
| Batch | `BatchGetItem`, `BatchWriteItem` (lab soft cap 25; overflow returned in `UnprocessedKeys` / `UnprocessedItems`) |
| Resource policy | `PutResourcePolicy`, `GetResourcePolicy`, `DeleteResourcePolicy` |
| TTL | `UpdateTimeToLive`, `DescribeTimeToLive` (lazy expiry on `GetItem`, `Query`, `Scan`, and `BatchGetItem`; `Query`/`Scan` over-fetch until `Limit` live items) |
| Encryption | Table SSE with AWS-owned or customer-managed KMS |

Table and item metadata live in SQLite. Item ciphertext uses table SSE. Expired TTL items are omitted on read, not deleted in the background.

### Authz notes

DynamoDB uses `EvaluateDynamoDB` via `authorizeDataplaneOR` with the table owner account from the table ARN. Same-account access: allow if identity **or** table resource policy Allows. Cross-account access: allow only when identity **and** table resource policy both Allow. Empty resource policy denies cross-account callers. Explicit Deny in either wins. A resource policy alone can grant access in the same account (unlike KMS). Org SCP/RCP filters apply before evaluation. When identity Allows, permissions boundary and session intersect.

Pass a full table ARN as `TableName` for cross-account `GetItem` and similar item APIs. SSE-KMS tables resolve the CMK under the table owner account and require `kms:Decrypt` via EvaluateKMS (identity plus key policy), matching S3 SSE-KMS.

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

## Not yet / deferred

- Full DynamoDB SAR beyond the lab set (more than two GSIs, LSI, Transactions, PartiQL, Contributor Insights, export/import, global tables, UpdateContinuousBackups / live PITR restore, on-demand vs provisioned billing depth, full pagination parity)
- Streams depth beyond the lab core in [dynamodbstreams.md](dynamodbstreams.md) (OLD_IMAGE views; DynamoDB Streams Lambda ESM ships in [lambda.md](lambda.md))
