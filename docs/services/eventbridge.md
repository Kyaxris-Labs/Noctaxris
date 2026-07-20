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
| Pattern | Lab match on `source`, `detail-type`, and simple `detail` key equality |
| Targets | SQS (`sqs:SendMessage`), Lambda (async invoke), SNS (`sns:Publish`) |
| Input | Constant `Input` JSON on a target overrides the generated EventBridge envelope when set. Otherwise lab `InputPath` JSONPath subset (`$.a.b`, hyphenated keys like `$.detail-type`) extracts a portion of the event |

Bus, rule, and target metadata live in SQLite.

### Authz notes

EventBridge control-plane APIs use identity `EvaluateFull` on bus and rule ARNs.

`PutTargets` with `RoleArn` requires `iam:PassRole` on the role and role trust must Allow `sts:AssumeRole` for `events.amazonaws.com` (`CheckPassRole` at PutTargets). At delivery time the lab mints a temporary role session and requires the role identity policies to Allow the target action (`sqs:SendMessage`, `lambda:InvokeFunction`, or `sns:Publish`). Roles without an Allow skip that target.

`PutTargets` without `RoleArn` delivers only when the target resource policy Allows `events.amazonaws.com` or the account root for the required action. Missing or insufficient policy skips that target (best-effort). Rule or target matches are recorded only when delivery is authorized.

Delivery failures after authorization are logged. PutEvents still succeeds (best-effort fan-out).

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

## Not yet / deferred

- Full EventBridge SAR (partner buses, archive and replay, API Destinations). Pipes lab core: [pipes.md](pipes.md)
- Legacy scheduled rules (`ScheduleExpression` on Rules). Prefer the Scheduler service for time-based labs
- Full EventBridge pattern language beyond source, detail-type, and simple detail key equality
- CloudWatch Logs and Kinesis targets
- `InputTransformer` (and InputPath bracket/wildcard notation)
- Bus resource policy dual-eval depth beyond same-account lab paths
- Exact AWS retry and jitter timing for delivery failures
- Cross-account bus policies beyond same-account lab paths
