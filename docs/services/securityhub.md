# Security Hub

**Status:** shipped (lab lite)

ASFF-lite import and get for forensic lab findings. Identity authz.

## Implemented

| Area | Actions |
|------|---------|
| Import | `BatchImportFindings` (cap 100; required ASFF fields) |
| Read | `GetFindings` with lite filters (`ProductArn`, `GeneratorId`, `SeverityLabel`, `ResourceType`) |

Required import fields follow AWS BatchImportFindings / AwsSecurityFinding: `SchemaVersion`, `Id`, `ProductArn`, `GeneratorId`, `AwsAccountId`, `Types`, `CreatedAt`, `UpdatedAt`, `Severity`, `Title`, `Description`, `Resources`.

### Authz notes

Identity `EvaluateFull` on `securityhub:*`.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification). Use SigV4 JSON (`X-Amz-Target`) or SDK against `:4566`.

## Not yet / deferred

- Security standards and controls
- Insights / custom actions
- Finding aggregator and cross-Region
- Full ASFF update / FindingProviderFields rules
