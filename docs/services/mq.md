# Amazon MQ

**Status:** shipped (lab core, control-plane stub)

Broker CRUD for ActiveMQ or RabbitMQ engine strings. Returns a loopback-only stub endpoint (`stub://127.0.0.1/mq/...`). BrokerState is `CREATION_FAILED` (stub-only; no nested broker). `PubliclyAccessible=true` is rejected. No nested broker process and no WAN-published broker ports.

## Implemented

| Area | Actions |
|------|---------|
| CRUD | `CreateBroker`, `DescribeBroker`, `ListBrokers`, `DeleteBroker` |
| Engines | `ACTIVEMQ`, `RABBITMQ` |
| Endpoint | Stub ConsoleURL / Endpoints on `127.0.0.1` (not a live listener) |

### Authz notes

Identity `EvaluateFull` on `mq:*`.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

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

DescribeBroker shows the stub endpoint and `CREATION_FAILED`. Do not expect a live AMQP/MQTT connection on that URL.

## Not yet / deferred

- Nested DinD ActiveMQ or RabbitMQ broker
- Full broker admin APIs and public endpoints
- MSK / Kafka (deferred product line)
