# lab-parity-observe

Terraform apply+destroy smoke for DynamoDB with a local secondary index.

```bash
STACK=lab-parity-observe bash tests/terraform/run.sh
```

Or via the suite gate:

```bash
TF_PARITY=1 bash tests/run-all.sh
```

| Resource | Included | Notes |
|----------|----------|-------|
| `aws_dynamodb_table` + LSI (Projection ALL) | Yes | hash `pk`, range `sk`, LSI `ByStatus` on `status` |
| `aws_cloudwatch_metric_alarm` | No | Provider Query shape vs Noctaxris `monitoring` Granite JSON; use SDK |

Endpoints wired: `dynamodb`, `cloudwatch` (provider key for monitoring), `sts`, `iam`. Soft-skip when the API is down or when WSL cannot reach Windows loopback publish stays in `tests/terraform/run.sh`.
