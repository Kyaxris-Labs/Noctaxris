# GuardDuty

**Status:** shipped (lab lite)

Lab detectors and findings for forensic scenarios. Identity authz. Lab inject is env-gated.

## Implemented

| Area | Actions |
|------|---------|
| Detector | `CreateDetector`, `ListDetectors` |
| Findings | `ListFindings`, `GetFindings` |
| Lab inject | `InjectFindings` (`NoctaxrisGuardDuty.InjectFindings`) when `NOCTAXRIS_GUARDDUTY_INJECT=1` |

Finding shape follows AWS Finding lite fields (`accountId`, `arn`, `id`, `type`, `severity`, `title`, `description`, `region`, `schemaVersion`, `createdAt`, `updatedAt`, `resource`, optional `service`).

### Authz notes

Identity `EvaluateFull` on `guardduty:*`. Inject remains AccessDenied unless `NOCTAXRIS_GUARDDUTY_INJECT=1`.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
# Enable inject in the API process env, then signed InjectFindings / GetFindings via SDK or SigV4 JSON.
```

## Not yet / deferred

- Detector feature sets, malware protection, attack sequences
- Finding publishing to Security Hub
- Full filter criteria matrix on ListFindings
