# Amazon MQ

**Status:** shipped (lab core; nested RabbitMQ when DinD is up)

Broker CRUD for ActiveMQ or RabbitMQ engine strings. `PubliclyAccessible=true` is rejected. Broker ports are never published on the host.

| Engine | Behavior |
|--------|----------|
| `RABBITMQ` | Nested DinD `rabbitmq:3.13-alpine` on Internal `noctaxris-data`. `CREATION_IN_PROGRESS` → `RUNNING` when the container is healthy; `CREATION_FAILED` when DinD is unset or start/wait fails. Nested AMQP endpoint (`amqp://noctaxris-mq-<broker-id>:5672`). Lab user `noctaxris` / `noctaxris-mq-lab`. |
| `ACTIVEMQ` | Control-plane only: `CREATION_FAILED` with `stub://127.0.0.1/mq/...` (no nested ActiveMQ image). |

## Implemented

| Area | Actions |
|------|---------|
| CRUD | `CreateBroker`, `DescribeBroker`, `ListBrokers`, `DeleteBroker` |
| Engines | `ACTIVEMQ` (stub), `RABBITMQ` (nested when DinD up) |
| Network | Internal nested network only; no host publish of 5672 |

### Authz notes

Identity `EvaluateFull` on `mq:*`.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification). Compose must include `noctaxris-engine` for nested RabbitMQ.

```bash
aws mq create-broker \
  --broker-name "noctaxris-mq-$RANDOM" \
  --engine-type RabbitMQ \
  --engine-version 3.13 \
  --host-instance-type mq.t3.micro \
  --deployment-mode SINGLE_INSTANCE \
  --no-publicly-accessible \
  --users Username=lab,Password=lab-password-1 \
  --endpoint-url "$EP"

aws mq list-brokers --endpoint-url "$EP"
```

With DinD healthy, DescribeBroker shows `RUNNING` and a nested `amqp://noctaxris-mq-...:5672` endpoint (reachable only from other nested containers on `noctaxris-data`, not from the host). Without DinD, state is `CREATION_FAILED` and the endpoint falls back to `stub://`.

AMQP smoke from a nested peer (same DinD network), not from the operator host:

```bash
# Example: exec into a nested lab container attached to noctaxris-data, then:
# amqp://noctaxris:noctaxris-mq-lab@noctaxris-mq-<broker-id>:5672
```

## Not yet / deferred

- Nested ActiveMQ engine
- Full broker admin APIs and public endpoints
- MSK / Kafka (deferred product line)
