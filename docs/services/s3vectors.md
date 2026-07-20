# S3 Vectors

**Status:** shipped (lab core)

Vector bucket and index CRUD lite plus PutVectors / QueryVectors with an in-process cosine or euclidean distance stub. Identity authz. No host GPU.

## Implemented

| Area | Actions |
|------|---------|
| Bucket | `CreateVectorBucket`, `ListVectorBuckets`, `DeleteVectorBucket` |
| Index | `CreateIndex`, `ListIndexes`, `DeleteIndex` |
| Data | `PutVectors`, `QueryVectors` |

Distance metrics: `cosine` or `euclidean`. Dimension 1..4096. Vectors use `data.float32` arrays matching the index dimension.

### Authz notes

Identity `EvaluateFull` on `s3vectors:*`.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws s3vectors create-vector-bucket --vector-bucket-name vec-lab --endpoint-url "$EP"
aws s3vectors create-index \
  --vector-bucket-name vec-lab --index-name idx \
  --data-type float32 --dimension 3 --distance-metric cosine \
  --endpoint-url "$EP"
```

Skip live smoke when Docker is unavailable (unit tests cover put/query ranking).

## Not yet / deferred

- Full S3 Vectors IAM condition matrix
- Huge dimensional indexes and metadata filter depth
- Encryption configuration beyond defaults
