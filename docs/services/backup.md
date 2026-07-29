# AWS Backup

**Status:** shipped (lab core)

REST JSON control-plane lab (credential scope `backup`). Vaults, plans, selections, and jobs are stored metadata. `StartBackupJob` completes immediately and writes a recovery-point row for S3/DynamoDB ARN strings; there is no real snapshot engine.

## Implemented

| Area | Method / path | Actions |
|------|---------------|---------|
| Vaults | `PUT/GET/DELETE /backup-vaults/{name}`, `GET /backup-vaults/` | Create/Describe/List/DeleteBackupVault |
| Plans | `PUT/GET /backup/plans/`, `GET/DELETE /backup/plans/{id}` | Create/Get/List/DeleteBackupPlan |
| Selections | `PUT/GET /backup/plans/{id}/selections/`, `GET/DELETE .../selections/{selectionId}` | Create/Get/List/DeleteBackupSelection |
| Jobs | `PUT /backup-jobs`, `GET /backup-jobs/`, `GET/POST /backup-jobs/{id}` | StartBackupJob, ListBackupJobs, DescribeBackupJob, StopBackupJob |
| Recovery points | `GET .../recovery-points/`, `GET/DELETE .../recovery-points/{arn}` | ListRecoveryPointsByBackupVault, DescribeRecoveryPoint, DeleteRecoveryPoint |

Selections store `IamRoleArn` plus a `Resources` ARN list under a plan. `ListBackupJobs` honors `backupVaultName`, `resourceArn`, `resourceType`, and `state` query filters. `StopBackupJob` on a completed/aborted/failed job is a no-op (200). `DeleteRecoveryPoint` removes metadata and decrements the vault recovery-point count.

### Authz notes

Identity `EvaluateFull` on `backup:*`.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws backup create-backup-vault --backup-vault-name lab-vault --endpoint-url "$EP"
aws backup create-backup-plan --backup-plan file://plan.json --endpoint-url "$EP"
aws backup create-backup-selection \
  --backup-plan-id "$PLAN_ID" \
  --backup-selection SelectionName=lab-sel,IamRoleArn=arn:aws:iam::000000000001:role/Backup,Resources=arn:aws:s3:::lab-bucket \
  --endpoint-url "$EP"
aws backup start-backup-job \
  --backup-vault-name lab-vault \
  --resource-arn arn:aws:s3:::lab-bucket \
  --iam-role-arn arn:aws:iam::000000000001:role/Backup \
  --endpoint-url "$EP"
aws backup list-backup-jobs --by-backup-vault-name lab-vault --by-state COMPLETED --endpoint-url "$EP"
aws backup list-recovery-points-by-backup-vault --backup-vault-name lab-vault --endpoint-url "$EP"
```

Unit tests cover selection CRUD, job list filters, StopBackupJob no-op on COMPLETED, and DeleteRecoveryPoint metadata removal.

## Not yet / deferred

- Real snapshot or copy engines
- Vault notifications / access policies
- Async job delay (jobs still complete immediately)
