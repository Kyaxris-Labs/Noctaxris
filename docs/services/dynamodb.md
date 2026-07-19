# DynamoDB

**Status:** shipped

Lab-complete DynamoDB: tables, item CRUD, Query/Scan (including one lab GSI per table), BatchGet/BatchWrite, table resource policies, CMK encryption, and TTL with lazy expiry on read.

## Implemented

| Area | Actions |
|------|---------|
| Tables | `CreateTable`, `DescribeTable`, `DeleteTable`, `ListTables`, `UpdateTable` |
| GSI | One lab global secondary index per table (`CreateTable` or `UpdateTable` `Create` GSI update) |
| Items | `PutItem`, `GetItem`, `DeleteItem`, `UpdateItem` |
| Query / Scan | `Query`, `Scan` (base table and lab GSI via `IndexName`) |
| Batch | `BatchGetItem`, `BatchWriteItem` |
| Resource policy | `PutResourcePolicy`, `GetResourcePolicy`, `DeleteResourcePolicy` |
| TTL | `UpdateTimeToLive`, `DescribeTimeToLive` (lazy expiry on `GetItem`, `Query`, `Scan`, and `BatchGetItem`) |
| Encryption | Table SSE with AWS-owned or customer-managed KMS |

Table and item metadata live in SQLite. Item ciphertext uses table SSE. Expired TTL items are omitted on read, not deleted in the background.

### Authz notes

DynamoDB uses `EvaluateDynamoDB`: allow if identity **or** table resource policy Allows. Explicit Deny in either wins. A resource policy alone can grant access (unlike KMS). Org SCP/RCP filters apply before the union. When identity Allows, permissions boundary and session intersect.

Cross-account table resource policy depth beyond same-account lab paths is deferred.

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

## Not yet / deferred

- Full DynamoDB SAR beyond the lab set (additional GSIs, LSI, Streams, Transactions, PartiQL, Contributor Insights, export/import, global tables, continuous backups, PITR, on-demand vs provisioned billing depth, tags, full pagination parity)
- Cross-account table resource policy depth beyond same-account lab paths
