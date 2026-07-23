# EventBridge

**Status:** shipped

Lab-complete EventBridge core: default and custom event buses, rules, targets, and `PutEvents` routing into SQS, Lambda, and SNS. JSON protocol via `X-Amz-Target: AWSEvents.*` or service `events` with JSON body.

## Implemented

| Area | Actions |
|------|---------|
| Buses | Default bus per account (`default`), `CreateEventBus`, `DeleteEventBus`, `ListEventBuses`, `DescribeEventBus` |
| Rules | `PutRule`, `DescribeRule`, `ListRules`, `DeleteRule`, `EnableRule`, `DisableRule` |
| Targets | `PutTargets`, `RemoveTargets`, `ListTargetsByRule` |
| Events | `PutEvents` matches enabled rules and fans out to targets |
| Pattern | Content-based match on `source`, `detail-type`, and nested `detail`: exact/OR lists, `prefix`, `suffix`, `exists`, `anything-but` (value, list, or prefix/suffix), `numeric`, `equals-ignore-case`. Unsupported operators rejected at `PutRule` |
| Targets | SQS, Lambda (async invoke), SNS via RoleArn **or** destination resource policy (`events.amazonaws.com` / account root + SourceArn). CloudWatch Logs, Kinesis, Step Functions require RoleArn (`PutTargets` rejects omit) |
| Input | Constant `Input` overrides the envelope. Else lab `InputTransformer` (`InputPathsMap` + `InputTemplate` with `<var>` placeholders). Else lab `InputPath` JSONPath subset (`$.a.b`, hyphenated keys). InputTransformer cannot combine with Input/InputPath |
| Bus policy | `PutPermission` / `RemovePermission` maintain bus `Policy`. `PutEvents` uses identity **or** bus policy (same account) and identity **and** bus policy (cross-account ARN). `EventBusName` may be a bus ARN |
| Tags | `ListTagsForResource`, `TagResource`, `UntagResource` |

Bus, rule, and target metadata live in SQLite.

### Authz notes

EventBridge control-plane APIs use identity `EvaluateFull` on bus and rule ARNs.

`PutEvents` uses dataplane dual-eval against the bus resource policy (`EvaluateResourceAccess`). Cross-account PutEvents requires a bus ARN in `EventBusName` and a bus policy Allow for the caller.

`PutTargets` with `RoleArn` requires `iam:PassRole` on the role and role trust must Allow `sts:AssumeRole` for `events.amazonaws.com` (`CheckPassRole` at PutTargets). At delivery time the lab mints a temporary role session and requires the role identity policies to Allow the target action. Same-account RoleArn-only delivery remains the lab path for SQS/Lambda/SNS; foreign SQS/Lambda/SNS targets require RoleArn session Allow **and** destination resource policy. Logs, Kinesis, and Step Functions targets require `RoleArn` at `PutTargets` (no resource-policy delivery path; omit fails validation).

`PutTargets` without `RoleArn` (SQS/Lambda/SNS only) is the AWS-shaped resource-based path: delivery runs only when the target resource policy Allows `events.amazonaws.com` or the account root for the target action. Empty or insufficient policy skips that target (not an open proxy). Condition keys `aws:SourceArn` (rule ARN) and `aws:SourceAccount` are populated so SourceArn locks work (Lambda: `AddPermission` with `SourceArn` / `SourceAccount`; SQS queue `Policy`; SNS topic `Policy`). SQS, Lambda, SNS, and Step Functions target ARNs may be cross-account (resource owner account for policy load and I/O). `PutPermission` may include an optional `Condition` object on the bus statement. Rule or target matches are recorded only when delivery is authorized.

CloudWatch Logs targets write to stream `eventbridge` (auto-created). Delivery failures after authorization are logged. PutEvents still succeeds (best-effort fan-out).

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
QUEUE_URL=$(aws sqs create-queue --queue-name "noctaxris-eb-$RANDOM" --endpoint-url "$EP" --query QueueUrl --output text)
QUEUE_ARN=$(aws sqs get-queue-attributes \
  --queue-url "$QUEUE_URL" \
  --attribute-names QueueArn \
  --endpoint-url "$EP" \
  --query Attributes.QueueArn --output text)

# Without RoleArn, the queue policy must Allow events.amazonaws.com (or account root) for sqs:SendMessage.
EB_POLICY='{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"events.amazonaws.com"},"Action":"sqs:SendMessage","Resource":"*"}]}'
aws sqs set-queue-attributes \
  --queue-url "$QUEUE_URL" \
  --attributes Policy="$EB_POLICY" \
  --endpoint-url "$EP"

RULE="noctaxris-lab-$RANDOM"
aws events put-rule \
  --name "$RULE" \
  --event-pattern '{"source":["noctaxris.lab"]}' \
  --endpoint-url "$EP"

aws events put-targets \
  --rule "$RULE" \
  --targets "Id=1,Arn=$QUEUE_ARN" \
  --endpoint-url "$EP"

aws events put-events \
  --entries "Source=noctaxris.lab,DetailType=demo,Detail={\"ok\":true}" \
  --endpoint-url "$EP"

aws sqs receive-message --queue-url "$QUEUE_URL" --endpoint-url "$EP"
```

### Event pattern operators

| Operator | Lab support |
|----------|-------------|
| Exact / OR lists | `"state": ["A","B"]` |
| Nested objects | `"detail": {"a": {"b": ["x"]}}` |
| `prefix` / `suffix` | String prefix/suffix (case-sensitive) |
| `exists` | `true` / `false` on leaf fields |
| `anything-but` | Scalar, list, or nested `prefix` / `suffix` |
| `numeric` | Comparisons `=`, `>`, `>=`, `<`, `<=` (paired arrays) |
| `equals-ignore-case` | Case-insensitive string equality |

## Not yet / deferred

- Full EventBridge SAR (partner buses, archive and replay, API Destinations). Pipes lab core: [pipes.md](pipes.md)
- Legacy scheduled rules (`ScheduleExpression` on Rules). Prefer the Scheduler service for time-based labs
- Pattern operators: `wildcard`, `$or`, IP/`cidr`, `anything-but`+`wildcard`, and `prefix`/`suffix` nested `equals-ignore-case` combos (`PutRule` rejects these)
- InputPath / InputTransformer bracket and wildcard notation
- Resource-policy delivery path for Logs/Kinesis/SFN targets (RoleArn required)
- Exact AWS retry and jitter timing for delivery failures
