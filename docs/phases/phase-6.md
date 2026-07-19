# Phase 6 status

Phase 6 delivers lab-complete DynamoDB and SQS cores: tables and item CRUD with basic Query/Scan and batch APIs, table resource policies, encryption at rest with customer-managed KMS, standard queues with send/receive/delete (including batch and visibility), queue policies via attributes, SSE-SQS, and SSE-KMS.

## Delivered

| Area | Status |
|------|--------|
| CreateTable, DescribeTable, DeleteTable, ListTables, UpdateTable | Done |
| PutItem, GetItem, DeleteItem, UpdateItem | Done |
| Query, Scan, BatchGetItem, BatchWriteItem | Done |
| PutResourcePolicy, GetResourcePolicy, DeleteResourcePolicy | Done |
| Table encryption at rest (AWS-owned or customer-managed KMS) | Done |
| `EvaluateDynamoDB` (identity or table resource policy union, Deny-overrides) | Done |
| CreateQueue, GetQueueUrl, GetQueueAttributes, SetQueueAttributes | Done |
| DeleteQueue, ListQueues, PurgeQueue | Done |
| SendMessage, ReceiveMessage, DeleteMessage | Done |
| SendMessageBatch, DeleteMessageBatch, ChangeMessageVisibility | Done |
| Queue policy via attributes, SSE-SQS, SSE-KMS | Done |
| `EvaluateSQS` (identity or queue policy union, Deny-overrides) | Done |
| Deferred DynamoDB and SQS depth | [../deferred.md](../deferred.md) |

## Authz note

Same-account DynamoDB uses identity **or** table resource policy Allow. Same-account SQS uses identity **or** queue policy Allow. Explicit Deny in either wins. Unlike KMS, a resource or queue policy alone can grant access.

## Smoke (Compose)

See [../verification.md](../verification.md) for `aws dynamodb create-table/put-item/get-item` and `aws sqs create-queue/send/receive/delete`.

## Explicitly not in Phase 6

See [../deferred.md](../deferred.md). No GSI/LSI, Streams, Transactions, FIFO queues, or Lambda.
