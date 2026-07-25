# Macie

**Status:** shipped (lab lite)

Session, classification-job lite, and sensitive-data findings for forensic labs. Identity authz. Lab inject is env-gated and uses canned pattern matches against existing S3 object bytes (no ML engine).

## Implemented

| Area | Actions |
|------|---------|
| Session | `EnableMacie`, `GetMacieSession` |
| Jobs | `CreateClassificationJob`, `DescribeClassificationJob`, `ListClassificationJobs` (jobs complete immediately; no scanning engine) |
| Findings | `ListFindings`, `GetFindings` |
| Lab inject | `InjectFindings` (`NoctaxrisMacie.InjectFindings`) when `NOCTAXRIS_MACIE_INJECT=1` |

### Lab inject

Fail-closed unless `NOCTAXRIS_MACIE_INJECT=1`. Body accepts either:

- `Findings` / `Finding` — Macie2 Finding lite objects
- `S3Objects` — `[{Bucket, Key}]` against existing lab objects; canned matchers for AWS access-key, US SSN-shaped, and credit-card-shaped patterns produce `SensitiveData:S3Object/*` findings

Finding shape follows Macie2 Finding lite fields (`accountId`, `id`, `type`, `category`, `severity`, `title`, `description`, `region`, `resourcesAffected`, optional `classificationDetails`).

### Authz notes

Identity `EvaluateFull` on `macie2:*`. Inject remains AccessDenied unless `NOCTAXRIS_MACIE_INJECT=1`. Macie must be enabled before jobs/findings/inject.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
# Enable inject in the API process env, then signed EnableMacie / CreateClassificationJob /
# InjectFindings (S3Objects) / GetFindings via SDK or SigV4 JSON (X-Amz-Target Macie2.* / NoctaxrisMacie.InjectFindings).
```

## Not yet / deferred

- Managed/custom data identifiers and automated discovery
- Policy findings and bucket inventory
- Finding export to Security Hub / EventBridge
- Real ML classification (intentionally out of lab scope; inject is seeded/canned only)
