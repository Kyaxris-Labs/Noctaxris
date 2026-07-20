# SNS

**Status:** shipped

Lab-complete SNS core: topic CRUD, publish, subscribe, topic policies, and best-effort delivery to confirmed SQS and Lambda subscriptions. Query protocol (`Action=...`, form URL-encoded body, XML responses) for AWS CLI compatibility.

## Implemented

| Area | Actions |
|------|---------|
| Topics | `CreateTopic`, `DeleteTopic`, `ListTopics`, `GetTopicAttributes`, `SetTopicAttributes` |
| Publish | `Publish` (message id plus fan-out to confirmed subscriptions) |
| Subscriptions | `Subscribe`, `Unsubscribe`, `ListSubscriptions`, `ListSubscriptionsByTopic`, `GetSubscriptionAttributes` |
| Topic policy | `AddPermission`, `RemovePermission`, and Policy attribute on create or `SetTopicAttributes` |
| Protocols | `sqs` and `lambda` (lab auto-confirm on subscribe). HTTP, email, and SMS deferred |
| Delivery | Confirmed `sqs` subscriptions receive the SNS-to-SQS JSON envelope. Confirmed `lambda` subscriptions receive an SNS Records event via the async invoke path. Best-effort with up to two attempts per target. Delivery failures are logged after retries. SQS subscription ARNs must match the subscription owner account |
| Destinations | Lambda async `DestinationConfig.OnFailure` may target an SNS topic ARN (Publish) or an SQS queue ARN |

Topic and subscription metadata live in SQLite.

### Authz notes

SNS uses `EvaluateSNS` via `authorizeDataplaneOR` with the topic owner account from the topic ARN. Same-account access: allow if identity **or** topic policy Allows. Cross-account access: allow only when identity **and** topic policy both Allow. Empty topic policy denies cross-account callers. Explicit Deny in either wins. Org SCP/RCP filters apply before evaluation. When identity Allows, permissions boundary and session intersect.

Cross-account `Publish` uses `TopicArn` of the owner account. Lab subscription delivery still requires same-account SQS or Lambda endpoints unless already supported.

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

- Full SNS SAR beyond the lab set (FIFO topics, SMS, email, HTTP and HTTPS subscriptions, filter policy depth, raw message delivery edge cases)
- HTTP, email, and SMS subscription protocols and confirmation token flows
- Exact AWS retry and jitter timing for delivery failures
- Cross-account Subscribe and foreign-account subscription delivery depth