# Amazon MQ

**Status:** shipped (lab core; nested RabbitMQ and ActiveMQ when DinD is up)

Broker CRUD for ActiveMQ or RabbitMQ engine strings. `PubliclyAccessible=true` is rejected. Broker ports are never published on the host.

| Engine | Behavior |
|--------|----------|
| `RABBITMQ` | Nested DinD `rabbitmq:3.13-alpine` on Internal `noctaxris-data`. `CREATION_IN_PROGRESS` → `RUNNING` when the container is healthy; `CREATION_FAILED` when DinD is unset or start/wait fails. Nested AMQP endpoint (`amqp://noctaxris-mq-<broker-id>:5672`). Lab user `noctaxris` / `noctaxris-mq-lab`. |
| `ACTIVEMQ` | Nested DinD `apache/activemq-classic:5.18.3` on Internal `noctaxris-data` (AMQP on 5672; OpenWire 61616 nested-only). Same `CREATION_IN_PROGRESS` → `RUNNING` / fail-closed `CREATION_FAILED` + `stub://` path as RabbitMQ. |

## Implemented

| Area | Actions |
|------|---------|
| CRUD | `CreateBroker`, `DescribeBroker`, `ListBrokers`, `DeleteBroker` |
| Engines | `ACTIVEMQ` (nested when DinD up), `RABBITMQ` (nested when DinD up) |
| Network | Internal nested network only; no host publish of 5672/61616 |
| Lambda ESM | `CreateEventSourceMapping` accepts broker ARNs when the broker is `RUNNING` with a nested non-`stub://` endpoint; poll dials allowlisted `noctaxris-mq-*` hosts only (never operator/WAN hosts). RabbitMQ: AMQP 0-9-1 Dial + `basic.get` on queue `noctaxris` (lab user `noctaxris` / `noctaxris-mq-lab`) returns real bodies when `RUNNING`. ActiveMQ: classic AMQP on 5672 is AMQP 1.0 — TCP-probe then empty batch (no AMQP 1.0/JMS client). Injectable `MQReceiveFunc` remains for unit tests (see [lambda.md](lambda.md)). Skip live MQ→Lambda smoke without DinD/broker. |

### Authz notes

Identity `EvaluateFull` on `mq:*`.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification). Compose must include `noctaxris-engine` for nested brokers.

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

aws mq create-broker \
  --broker-name "noctaxris-amq-$RANDOM" \
  --engine-type ActiveMQ \
  --engine-version 5.18 \
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
# RabbitMQ (AMQP 0-9-1): amqp://noctaxris:noctaxris-mq-lab@noctaxris-mq-<broker-id>:5672
# ActiveMQ (AMQP 1.0 on classic connector): amqp://noctaxris-mq-<broker-id>:5672
```

Lambda ESM poll allowlists the nested host only. RabbitMQ ESM runs AMQP 0-9-1 `basic.get` on queue `noctaxris` when the broker is `RUNNING`. ActiveMQ remains dial-only empty (AMQP 1.0/JMS). Unit tests may inject `MQReceiveFunc`.

## Not yet / deferred

- Full broker admin APIs and public endpoints
- MSK / Kafka (out of lab scope)
- ActiveMQ AMQP 1.0/JMS consumer for Lambda ESM (dial-only empty after allowlisted probe; RabbitMQ `basic.get` is shipped — see [lambda.md](lambda.md))
