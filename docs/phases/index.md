# Phase history

Completed delivery slices. Smoke commands live in [../verification.md](../verification.md). Deferred depth is in [../deferred.md](../deferred.md).

| Phase | Focus | Doc |
|-------|--------|-----|
| 0 | Scaffold, Docker, sealed secrets, health + auth gate | [phase-0.md](phase-0.md) |
| 1 | SigV4 + `GetCallerIdentity` | [phase-1.md](phase-1.md) |
| 2 | Organizations CreateAccount + cross-account `AssumeRole` | [phase-2.md](phase-2.md) |
| 3 | Lab IAM + all 11 STS actions (fail-closed federation) | [phase-3.md](phase-3.md) |
| 4 | Lab-complete KMS (keys, policies, crypto, grants, aliases) | [phase-4.md](phase-4.md) |
| 5 | Lab-complete S3 (objects, bucket policy, SSE, path-style presign) | [phase-5.md](phase-5.md) |
| 6 | Lab-complete DynamoDB + SQS (items, resource/queue policy, SSE) | [phase-6.md](phase-6.md) |
| 7 | Lab-complete Lambda (zip python3.12, PassRole, nested DinD, sync Invoke) | [phase-7.md](phase-7.md) |
