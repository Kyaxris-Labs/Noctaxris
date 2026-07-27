# VPC Flow Logs (lab)

**Status:** shipped (lab seed)

Opaque flow-log metadata and inject of AWS VPC Flow Logs custom format v2 lines. No real VPC/ENI control plane. Instance APIs (`RunInstances`, …) live on the same `ec2` service; see [ec2.md](ec2.md).

## Implemented

| Area | Actions |
|------|---------|
| Create | `ec2:CreateFlowLogs` lite (opaque `fl-` / `vpc-` / `eni-` IDs; destination S3 or CloudWatch Logs) |
| Lab inject | `ec2:InjectFlowLogs` (`NoctaxrisEC2.InjectFlowLogs`) when `NOCTAXRIS_VPCFLOW_INJECT=1` |

Default inject emits one ACCEPT and one REJECT v2 line (`version account-id interface-id srcaddr dstaddr srcport dstport protocol packets bytes start end action log-status`).

### Authz notes

Identity `EvaluateFull` on `ec2:CreateFlowLogs` / `ec2:InjectFlowLogs`. Inject fail-closed unless env enabled.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification). Set `NOCTAXRIS_VPCFLOW_INJECT=1`, create a flow log to an in-account bucket or log group, then InjectFlowLogs and list the S3 object or GetLogEvents.

## Not yet / deferred

- Real ENI attachment or traffic capture
- Transit Gateway / VPC Lattice flow logs
- Aggregation intervals and Athena partition projection helpers
