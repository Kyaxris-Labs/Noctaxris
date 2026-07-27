# AWS Backup

**Status:** shipped (lab core)

REST JSON control-plane lab (credential scope `backup`). Vaults, plans, and jobs are stored metadata. `StartBackupJob` completes immediately and writes a recovery-point row for S3/DynamoDB ARN strings; there is no real snapshot engine.

## Implemented

| Area | Method / path | Actions |
|------|---------------|---------|
| Vaults | `PUT/GET/DELETE /backup-vaults/{name}`, `GET /backup-vaults/` | Create/Describe/List/DeleteBackupVault |
| Plans | `PUT/GET /backup/plans/`, `GET/DELETE /backup/plans/{id}` | Create/Get/List/DeleteBackupPlan |
| Jobs | `PUT /backup-jobs`, `GET /backup-jobs/{id}` | StartBackupJob, DescribeBackupJob |
| Recovery points | `GET .../recovery-points/`, `GET .../recovery-points/{arn}` | ListRecoveryPointsByBackupVault, DescribeRecoveryPoint |

### Authz notes

Identity `EvaluateFull` on `backup:*`.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws backup create-backup-vault --backup-vault-name lab-vault --endpoint-url "$EP"
aws backup create-backup-plan --backup-plan file://plan.json --endpoint-url "$EP"
aws backup start-backup-job \
  --backup-vault-name lab-vault \
  --resource-arn arn:aws:s3:::lab-bucket \
  --iam-role-arn arn:aws:iam::000000000001:role/Backup \
  --endpoint-url "$EP"
aws backup list-recovery-points-by-backup-vault --backup-vault-name lab-vault --endpoint-url "$EP"
```

Unit tests cover vault create, plan create, completed job, and DynamoDB/S3 resource-type labels.

## Not yet / deferred

- Real snapshot or copy engines
- Backup selections, vault notifications / access policies
- Async job delay / StopBackupJob
