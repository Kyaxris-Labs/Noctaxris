# Bedrock Runtime

**Status:** shipped (lab core, canned stub)

`InvokeModel` and `Converse` over allowlisted model IDs return deterministic canned JSON. No real foundation model calls and no WAN egress to model providers.

## Implemented

| Area | Actions |
|------|---------|
| Inference | `InvokeModel` (REST `POST /model/{modelId}/invoke`) |
| Converse | `Converse` (REST `POST /model/{modelId}/converse`); non-empty `messages` required |
| Streaming | `ConverseStream` returns `501 UnsupportedOperationException` |
| Allowlist | `amazon.titan-text-express-v1`, `amazon.titan-embed-text-v1`, `anthropic.claude-3-haiku-20240307-v1:0` |
| Authz | `bedrock:InvokeModel`, `bedrock:Converse` via identity `EvaluateFull` |

Unknown model IDs fail closed with `ResourceNotFoundException`.

Converse accepts `system`, `inferenceConfig`, and `toolConfig` in the request body; only `messages` is validated. Tool-use round-tripping is not implemented.

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

aws bedrock-runtime converse \
  --model-id anthropic.claude-3-haiku-20240307-v1:0 \
  --messages '[{"role":"user","content":[{"text":"hello from noctaxris"}]}]' \
  --endpoint-url "$EP"
```

Live Compose smoke skipped when Docker is unavailable.

## Not yet / deferred

- `InvokeModelWithResponseStream` and live `ConverseStream` bodies
- Agents, Guardrails, Provisioned Throughput
- Real model runtimes or cloud Bedrock calls
