# Changelog

## v2 (lab cores shipped)

Cleared in-scope deferred depth for the lab core (except microVMs), then shipped lab-complete SSM Parameter Store, Secrets Manager, SNS, EventBridge, ECR, and ECS. Verification bar is `go test ./...`. Per-service Compose CLI smoke lives on each `docs/services/` page (operator-run, not claimed as CI).

### Included

- Condition-key catalogs for lab-core services from servicereference JSON plus global seed
- Authz Condition rules per ADR-0005 §7 (catalog-unknown deny, unpopulated known fail-closed on positive operators, AWS-faithful Null / StringNotEquals / IfExists)
- IAM groups (membership, managed and inline group policies), permissions boundaries for users and roles, instance profiles, OIDC and SAML IdP CRUD
- `EvaluateFull` identity plus boundary plus SCP and RCP on shared `authorize` and on S3/KMS/DynamoDB/SQS authorize plus Lambda PassRole
- Organizations ListAccounts, OUs, EnablePolicyType, SCP and RCP CreatePolicy / AttachPolicy / DetachPolicy / DescribePolicy
- IAM virtual MFA devices, lab MFA token validation, `GetSessionToken` without IAM permission gate, MFA sessions set `aws:MultiFactorAuthPresent`
- KMS ReEncrypt, ScheduleKeyDeletion/CancelKeyDeletion/PendingDeletion (cancel restores `Disabled` per AWS), rotation enable/status flags, lab `alias/aws/s3|dynamodb|sqs` convenience aliases
- S3 multipart upload (5 MiB minimum for non-final parts on Complete), CopyObject (same account), Put/Get/DeleteBucketEncryption with default SSE inheritance
- DynamoDB one lab GSI, Update/DescribeTimeToLive with lazy expiry on read
- SQS FIFO queues with deduplication and RedrivePolicy move-to-DLQ
- SSM Parameter Store String and SecureString parameters, Put/Get/GetParameters/Delete/Describe, KMS via KeyId or lab `alias/aws/ssm`, identity EvaluateFull authz
- Secrets Manager create/get/put/delete/describe/list, secret resource policies, KMS via lab `alias/aws/secretsmanager`, identity-or-resource-policy authz (immediate delete, no recovery window)
- Lambda versions and aliases with Invoke qualifier resolution (`name`, `name:version`, `name:alias`)
- Lambda layers (max 5 same-account ARNs, merged at `/opt` on zip Invoke)
- Lambda async Invoke (`InvocationType=Event`) with two lab retries and SQS DLQ / OnFailure destinations
- Lambda container image packaging (`PackageType=Image`, `Code.ImageUri`)
- Lambda zip runtimes `python3.11`, `python3.12`, `nodejs20.x`
- Nested DinD engine TLS on port 2376 (`NOCTAXRIS_DOCKER_CERT_PATH`)
- Same-account Lambda function resource policies (`AddPermission`, `RemovePermission`, `GetPolicy`) with identity-or-policy Invoke authz
- SNS topic CRUD, publish, subscribe, topic policies (`EvaluateSNS` identity-or-policy authz), confirmed sqs and lambda delivery with best-effort retry
- EventBridge buses, rules, targets, and PutEvents routing to SQS, Lambda, and SNS. PassRole on PutTargets RoleArn with `events.amazonaws.com` trust. Without RoleArn, target resource policy must Allow `events.amazonaws.com` or account root
- ECR repository CRUD, GetAuthorizationToken, repository policies, image metadata APIs, and Docker Registry V2 on loopback with token auth. DinD sync on manifest put for ECS and Lambda Image pulls
- ECS task definitions (taskRoleArn and executionRoleArn required), RunTask/Describe/List/Stop, default cluster, PassRole with `ecs-tasks.amazonaws.com` trust, nested DinD on Internal `noctaxris-ecs` network, task-role credential injection

Deferred depth: [docs/services/index.md](docs/services/index.md).

## Lab core

First shippable service set for Docker-first local AWS-shaped labs.

### Included

- Secure Compose defaults: loopback publish, no host `docker.sock`, sealed secrets
- SigV4 (header and query), IAM lab APIs, all 11 STS actions, Organizations CreateAccount
- Lab-complete KMS (keys, policies, crypto, grants, aliases)
- Lab-complete S3 (path-style objects, bucket policy, SSE-S3/SSE-KMS, presigned GET/PUT)
- Lab-complete DynamoDB and SQS (items/messages, resource/queue policies, encryption options)
- Lab-complete Lambda (zip/Image, versions/aliases, layers, sync+async Invoke, resource policies, TLS DinD)

### Explicitly later

See [docs/services/](docs/services/index.md) for remaining SAR depth, new lab cores, and post-v2 microVMs.
