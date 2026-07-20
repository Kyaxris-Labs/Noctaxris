# Lab-complete DynamoDB and SQS (historical)

Historical ship snapshot: lab-complete DynamoDB and SQS cores with tables and item CRUD, basic Query/Scan and batch APIs, table resource policies, encryption at rest with customer-managed KMS, standard queues with send/receive/delete (including batch and visibility), queue policies via attributes, SSE-SQS, and SSE-KMS.

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
| Deferred DynamoDB and SQS depth | [../services/dynamodb.md](../services/dynamodb.md), [../services/sqs.md](../services/sqs.md) |

## Authz note

Same-account DynamoDB uses identity **or** table resource policy Allow. Same-account SQS uses identity **or** queue policy Allow. Explicit Deny in either wins. Unlike KMS, a resource or queue policy alone can grant access.

## Smoke (Compose)

See [../services/dynamodb.md](../services/dynamodb.md) and [../services/sqs.md](../services/sqs.md) for create-table/put-item/get-item and create-queue/send/receive/delete.

## Explicitly deferred at this ship

Current deferred depth: [../services/dynamodb.md](../services/dynamodb.md), [../services/sqs.md](../services/sqs.md). At this ship there was no lab GSI/TTL, FIFO/RedrivePolicy, or Lambda (those landed later).
