# EventBridge Scheduler

**Status:** shipped (lab core)

Distinct Scheduler API (not EventBridge Rules `ScheduleExpression`). Create and manage schedules with rate or a small cron subset, deliver to Lambda, SQS, SNS, or Step Functions state machines via an in-process ticker.

## Implemented

| Area | Actions |
|------|---------|
| Schedules | `CreateSchedule`, `GetSchedule`, `UpdateSchedule`, `DeleteSchedule`, `ListSchedules` |
| Expressions | `rate(n minutes\|hours\|days)`, AWS-shaped `cron(minutes hours day-of-month month day-of-week year)` subset (digits, `*`, `?`), optional one-time `at(yyyy-mm-ddThh:mm:ss)` |
| Targets | SQS `SendMessage`, Lambda async Invoke enqueue, SNS `Publish`, Step Functions `StartExecution` (RoleArn) |
| Authz | Identity `EvaluateFull` on `scheduler:*`. PassRole when `Target.RoleArn` is set (`scheduler.amazonaws.com` trust). Delivery without RoleArn requires a target resource policy Allow for `scheduler.amazonaws.com`. Foreign SQS/Lambda/SNS targets with RoleArn require role session Allow **and** destination resource policy. Target I/O uses the account embedded in the target ARN |
| Ticker | In-process worker advances `next_run` and delivers due ENABLED schedules (at-least-once lab best-effort) |

Schedule metadata lives in SQLite under the default group when `GroupName` is omitted.

### Authz notes

PassRole uses service principal `scheduler.amazonaws.com`. Target delivery with RoleArn mints a role session and evaluates identity policies for the target action. Same-account RoleArn-only delivery remains the lab path for SQS/Lambda/SNS; foreign targets also require the destination resource policy. Without RoleArn, delivery checks the SQS, Lambda, or SNS resource policy for the Scheduler service principal. Step Functions targets require RoleArn (`states:StartExecution`).

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
QUEUE_URL=$(aws sqs create-queue --queue-name "noctaxris-sched-$RANDOM" --endpoint-url "$EP" --query QueueUrl --output text)
QUEUE_ARN=$(aws sqs get-queue-attributes \
  --queue-url "$QUEUE_URL" \
  --attribute-names QueueArn \
  --endpoint-url "$EP" \
  --query Attributes.QueueArn --output text)

aws sqs set-queue-attributes --queue-url "$QUEUE_URL" --endpoint-url "$EP" \
  --attributes "{\"Policy\":\"{\\\"Version\\\":\\\"2012-10-17\\\",\\\"Statement\\\":[{\\\"Effect\\\":\\\"Allow\\\",\\\"Principal\\\":{\\\"Service\\\":\\\"scheduler.amazonaws.com\\\"},\\\"Action\\\":\\\"sqs:SendMessage\\\",\\\"Resource\\\":\\\"$QUEUE_ARN\\\"}]}\"}"

aws scheduler create-schedule \
  --name "noctaxris-rate-$RANDOM" \
  --schedule-expression "rate(1 minutes)" \
  --flexible-time-window Mode=OFF \
  --target "Arn=$QUEUE_ARN,Input={\\\"hello\\\":true}" \
  --endpoint-url "$EP"

aws scheduler list-schedules --endpoint-url "$EP"
```

Skip live ticker wait in CI when Docker is unavailable. Unit tests call `ProcessDueSchedules` directly.

## Not yet / deferred

- Flexible time windows beyond Mode=OFF
- Full retry and DLQ matrix
- Schedule groups beyond the default group depth
- Universal targets beyond Lambda, SQS, SNS, and Step Functions
- Legacy EventBridge scheduled rules (`ScheduleExpression` on Rules)
