# SNS

**Status:** shipped

Lab-complete SNS core: topic CRUD (including FIFO), publish, subscribe (SQS, Lambda, and loopback HTTP), topic policies, and best-effort delivery to confirmed subscriptions. Query protocol (`Action=...`, form URL-encoded body, XML responses) for AWS CLI compatibility.

## Implemented

| Area | Actions |
|------|---------|
| Topics | `CreateTopic`, `DeleteTopic`, `ListTopics`, `GetTopicAttributes`, `SetTopicAttributes` |
| FIFO | Topic names ending in `.fifo` (or `FifoTopic=true`). Publish requires `MessageGroupId`. Dedup via `MessageDeduplicationId` or `ContentBasedDeduplication` within a 5-minute window (same as SQS FIFO). SQS FIFO subscriptions receive group and dedup ids |
| Publish | `Publish` (message id plus fan-out to confirmed subscriptions) |
| Subscriptions | `Subscribe`, `ConfirmSubscription`, `Unsubscribe`, `ListSubscriptions`, `ListSubscriptionsByTopic`, `GetSubscriptionAttributes` |
| Topic policy | `AddPermission`, `RemovePermission`, and Policy attribute on create or `SetTopicAttributes` |
| Protocols | `sqs` and `lambda` (lab auto-confirm). `http` and `https` only to the lab catcher on loopback `:4566` (`/_noctaxris/sns-http-catcher`), or exact URLs in `NOCTAXRIS_SNS_HTTP_ALLOWLIST` that resolve to public hosts (private/loopback/metadata rejected; no redirect follow). Arbitrary loopback ports are rejected |
| Delivery | Confirmed `sqs` subscriptions receive the SNS-to-SQS JSON envelope when the queue policy Allows `sns.amazonaws.com`. Confirmed `lambda` subscriptions receive an SNS Records event via the async invoke path when the function policy Allows `sns.amazonaws.com`. Confirmed HTTP subscriptions POST JSON to the allowlisted endpoint. Best-effort with up to two attempts per target |
| Destinations | Lambda async `DestinationConfig.OnFailure` may target an SNS topic ARN (Publish) or an SQS queue ARN |

Topic and subscription metadata live in SQLite.

### Authz notes

SNS uses `EvaluateSNS` via `authorizeDataplaneOR` with the topic owner account from the topic ARN. Same-account access: allow if identity **or** topic policy Allows. Cross-account access: allow only when identity **and** topic policy both Allow. Empty topic policy denies cross-account callers. Explicit Deny in either wins. Org SCP/RCP filters apply before evaluation. When identity Allows, permissions boundary and session intersect.

Subscription delivery to SQS or Lambda also requires the destination resource policy to Allow `sns.amazonaws.com` (EventBridge-style service principal check). Missing policy skips that subscription (logged).

Cross-account `Publish` uses `TopicArn` of the owner account. Cross-account `Subscribe` to a foreign topic ARN is allowed when topic-policy dual-eval Allows `sns:Subscribe`. SQS subscription endpoints may be foreign queue ARNs; delivery uses the queue owner account and requires the queue policy to Allow `sns.amazonaws.com`.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
TOPIC_ARN=$(aws sns create-topic --name "noctaxris-lab-$RANDOM" --endpoint-url "$EP" --query TopicArn --output text)
QUEUE_URL=$(aws sqs create-queue --queue-name "noctaxris-sns-$RANDOM" --endpoint-url "$EP" --query QueueUrl --output text)
QUEUE_ARN=$(aws sqs get-queue-attributes \
  --queue-url "$QUEUE_URL" \
  --attribute-names QueueArn \
  --endpoint-url "$EP" \
  --query Attributes.QueueArn --output text)

SNS_Q_POLICY='{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"sns.amazonaws.com"},"Action":"sqs:SendMessage","Resource":"*"}]}'
aws sqs set-queue-attributes \
  --queue-url "$QUEUE_URL" \
  --attributes Policy="$SNS_Q_POLICY" \
  --endpoint-url "$EP"

aws sns subscribe \
  --topic-arn "$TOPIC_ARN" \
  --protocol sqs \
  --notification-endpoint "$QUEUE_ARN" \
  --endpoint-url "$EP"

aws sns publish \
  --topic-arn "$TOPIC_ARN" \
  --message hello-sns \
  --endpoint-url "$EP"

aws sqs receive-message --queue-url "$QUEUE_URL" --endpoint-url "$EP"
```

Topic policy via attributes:

```bash
POLICY='{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"sns:Publish","Resource":"*","Principal":{"AWS":"*"}}]}'

aws sns set-topic-attributes \
  --topic-arn "$TOPIC_ARN" \
  --attribute-name Policy \
  --attribute-value "$POLICY" \
  --endpoint-url "$EP"
```

Two-account cross-account publish (member account B owns the topic, member account A user publishes via dual eval):

```bash
TOPIC_ARN=$(aws sns create-topic --name "$TOPIC" --endpoint-url "$EP" --profile account-b --query TopicArn --output text)
aws sns set-topic-attributes --topic-arn "$TOPIC_ARN" --endpoint-url "$EP" --profile account-b \
  --attribute-name Policy \
  --attribute-value '{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::ACCOUNT_A:user/publisher"},"Action":"sns:Publish","Resource":"'"$TOPIC_ARN"'"}]}'
aws sns publish --topic-arn "$TOPIC_ARN" --message hello-xa --endpoint-url "$EP" --profile account-a
```

## Not yet / deferred

- Full SNS SAR beyond the lab set (SMS, email, filter policy depth, raw message delivery edge cases)
- Open internet HTTP webhooks (egress remains deny-by-default outside loopback)
- Exact AWS retry and jitter timing for delivery failures
- Foreign Lambda subscription endpoints (SQS foreign delivery is shipped)
- High-throughput FIFO quotas