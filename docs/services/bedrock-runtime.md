# Bedrock Runtime

**Status:** shipped (lab core, canned stub)

`InvokeModel` over allowlisted model IDs returns deterministic canned JSON. No real foundation model calls and no WAN egress to model providers.

## Implemented

| Area | Actions |
|------|---------|
| Inference | `InvokeModel` (REST `POST /model/{modelId}/invoke`) |
| Allowlist | `amazon.titan-text-express-v1`, `amazon.titan-embed-text-v1`, `anthropic.claude-3-haiku-20240307-v1:0` |
| Authz | Identity `EvaluateFull` on `bedrock:InvokeModel` |

Unknown model IDs fail closed with `ResourceNotFoundException`.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
printf '%s' '{"inputText":"hello from noctaxris"}' > /tmp/bedrock-in.json
aws bedrock-runtime invoke-model \
  --model-id amazon.titan-text-express-v1 \
  --body fileb:///tmp/bedrock-in.json \
  --cli-binary-format raw-in-base64-out \
  --endpoint-url "$EP" \
  /tmp/bedrock-out.json
cat /tmp/bedrock-out.json
```

Live Compose smoke skipped when Docker is unavailable.

## Not yet / deferred

- Converse API and streaming invoke
- Agents, Guardrails, Provisioned Throughput
- Real model runtimes or cloud Bedrock calls
