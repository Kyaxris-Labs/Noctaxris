# Noctaxris

**Run AWS-shaped security labs on your laptop without a cloud bill or a host Docker socket.**

```bash
docker compose -f docker/compose.yaml --env-file docker/.env up --build
curl http://127.0.0.1:4566/_noctaxris/health
# ok
```

SigV4 endpoint on `127.0.0.1:4566`. Point the AWS CLI at it and exercise the lab services in the table below the way you would against real AWS.

Repo and Go module: [`github.com/Kyaxris-Labs/Noctaxris`](https://github.com/Kyaxris-Labs/Noctaxris). Images ship under [Kyaxris-Labs](https://github.com/Kyaxris-Labs).

## Why this exists

| | |
|---|---|
| Lab fidelity | Identity evaluation with boundaries, SCP/RCP filters, PassRole, and condition keys |
| Secure defaults | Loopback publish only. No host `docker.sock`. Sealed secrets and CMK material at rest |
| Nested compute | Lambda Invoke uses Compose `noctaxris-engine` (DinD), not your host engine |
| CLI-shaped | Latest AWS CLI v2 via `--endpoint-url` |

## Quick start

Copy env, start Compose, hit health, then call STS and S3.

```bash
cp docker/.env.example docker/.env
# edit docker/.env root access keys if you want non-example values

docker compose -f docker/compose.yaml --env-file docker/.env up --build

curl http://127.0.0.1:4566/_noctaxris/health

export AWS_ACCESS_KEY_ID=AKIAROOTEXAMPLE01
export AWS_SECRET_ACCESS_KEY='wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY'
export AWS_DEFAULT_REGION=us-east-1
EP=http://127.0.0.1:4566

aws configure set default.s3.addressing_style path
aws sts get-caller-identity --endpoint-url "$EP"
aws s3 mb s3://lab-bucket --endpoint-url "$EP"
aws kms create-key --endpoint-url "$EP"
```

Use the same root keys you put in `docker/.env`. Full per-service CLI smoke lives under [docs/services/](docs/services/index.md).

## Services

<table>
  <thead>
    <tr>
      <th>Area</th>
      <th>Services</th>
      <th>Detailed actions</th>
      <th>Not implemented</th>
    </tr>
  </thead>
  <tbody>
    <tr>
      <td rowspan="3" align="center" valign="middle">Identity</td>
      <td>IAM</td>
      <td>Users, roles, managed and inline policies, access keys, groups, permissions boundaries, instance profiles, OIDC and SAML IdP CRUD, virtual MFA.</td>
      <td>Service-linked roles, full pagination and tagging parity.</td>
    </tr>
    <tr>
      <td>STS</td>
      <td>All 11 actions (lab MFA on GetSessionToken).</td>
      <td>Deeper AssumeRoot, DecodeAuthorizationMessage, GetDelegatedAccessToken, and GetWebIdentityToken parity.</td>
    </tr>
    <tr>
      <td>Organizations</td>
      <td>CreateAccount, ListAccounts, OUs, EnablePolicyType, SCP and RCP create/attach/detach/describe. Identity, boundary, SCP, and RCP apply on shared authorize and on S3/KMS/DynamoDB/SQS plus Lambda PassRole paths.</td>
      <td>Invites, OU-path SCP/RCP inheritance.</td>
    </tr>
    <tr>
      <td rowspan="1" align="center" valign="middle">Crypto</td>
      <td>KMS</td>
      <td>Customer-managed keys, key policies, Encrypt/Decrypt/GenerateDataKey*/ReEncrypt, grants, aliases (including lab alias/aws/s3|dynamodb|sqs), ScheduleKeyDeletion/CancelKeyDeletion (cancel leaves Disabled), rotation enable/status flags.</td>
      <td>Sign/Verify, MAC, asymmetric/HMAC specs, import, multi-Region, material rotation sweeper, tags, cross-account key policy depth, true AWS-owned managed keys.</td>
    </tr>
    <tr>
      <td rowspan="7" align="center" valign="middle">Data</td>
      <td>S3</td>
      <td>Path-style buckets and objects, bucket policy, SSE-S3/SSE-KMS, presigned GET/PUT, multipart upload (5 MiB min non-final parts), CopyObject (same account), bucket default encryption.</td>
      <td>Versioning, lifecycle, virtual-hosted style, cross-account policy depth, multipart presign.</td>
    </tr>
    <tr>
      <td>DynamoDB</td>
      <td>Tables, item CRUD, Query/Scan with one lab GSI, BatchGet/BatchWrite, table resource policies, CMK encryption, TTL configure and lazy expiry.</td>
      <td>LSI, Streams, Transactions, PartiQL, global tables, multi-GSI.</td>
    </tr>
    <tr>
      <td>SQS</td>
      <td>Standard and FIFO queues, send/receive/delete (batch and visibility), deduplication, queue policies, SSE-SQS and SSE-KMS, RedrivePolicy to DLQ.</td>
      <td>High-throughput FIFO quotas, DLQ redrive allow policies, delay queue depth.</td>
    </tr>
    <tr>
      <td>SSM Parameter Store</td>
      <td>String and SecureString parameters, Put/Get/GetParameters/Delete/Describe, KMS via KeyId or alias/aws/ssm, identity EvaluateFull authz.</td>
      <td>StringList types, parameter policies, hierarchy paths, tags, full pagination parity.</td>
    </tr>
    <tr>
      <td>Secrets Manager</td>
      <td>Create/Get/Put/Delete/Describe/List, resource policies, KMS via alias/aws/secretsmanager, identity or resource policy authz.</td>
      <td>RotateSecret, recovery window, tags, replication, version stages.</td>
    </tr>
    <tr>
      <td>SNS</td>
      <td>Topic CRUD, Publish, Subscribe and Unsubscribe, List*, Get/SetTopicAttributes, Add/RemovePermission, topic policies with identity-or-policy authz, confirmed sqs and lambda delivery (SNS envelope to SQS, Records event to Lambda async queue). Lab auto-confirm for sqs and lambda.</td>
      <td>FIFO topics, SMS, email, HTTP subscriptions, filter policy depth, full SAR, exact AWS retry timing.</td>
    </tr>
    <tr>
      <td>EventBridge</td>
      <td>Default and custom buses, Put/Describe/List/Delete/Enable/Disable Rule, Put/Remove/List Targets, PutEvents with lab pattern match (source, detail-type, simple detail keys). Targets SQS, Lambda, SNS. PassRole plus events.amazonaws.com trust on PutTargets RoleArn. Without RoleArn, target resource policy must Allow events.amazonaws.com or account root.</td>
      <td>Pipes, Scheduler, archive and replay, CloudWatch Logs and Kinesis targets, InputPath and InputTransformer, role session mint at delivery, full pattern language, bus policy dual-eval depth.</td>
    </tr>
    <tr>
      <td rowspan="3" align="center" valign="middle">Compute</td>
      <td>Lambda</td>
      <td>Zip or Image CreateFunction through UpdateConfiguration, PublishVersion and aliases, layers (max 5, <code>/opt</code> on zip Invoke), sync and async Invoke (Event with SQS DLQ/OnFailure), runtimes <code>python3.11</code>/<code>python3.12</code>/<code>nodejs20.x</code>, Invoke qualifiers, same-account AddPermission/GetPolicy/RemovePermission, PassRole plus <code>lambda.amazonaws.com</code> trust, nested DinD with TLS (no host <code>docker.sock</code>), platform egress deny.</td>
      <td>Event source mappings, provisioned concurrency, weighted aliases, Function URLs, cross-account policies, EventBridge failure destinations, rootless engine, full SAR depth. Post-v2: Firecracker microVMs.</td>
    </tr>
    <tr>
      <td>ECR</td>
      <td>Create/Describe/DeleteRepository, GetAuthorizationToken, repository policies, PutImage/BatchGetImage/ListImages/BatchDeleteImage, Registry V2 on <code>127.0.0.1:4566</code> with token auth, DinD sync on manifest put.</td>
      <td>Scanning, replication, lifecycle, OCI referrers depth, chunked PATCH uploads, public galleries, cross-account policy depth.</td>
    </tr>
    <tr>
      <td>ECS</td>
      <td>Register/Describe/List/DeregisterTaskDefinition (requires taskRoleArn and executionRoleArn), RunTask/Describe/List/Stop, DescribeClusters/ListClusters, PassRole plus <code>ecs-tasks.amazonaws.com</code> trust, nested DinD on <code>noctaxris-ecs</code> Internal network, task-role credential injection.</td>
      <td>CreateService/UpdateService, awsvpc, capacity providers, ECS Exec, Service Connect, full SAR depth. Post-v2: Firecracker microVMs.</td>
    </tr>
  </tbody>
</table>

Per-service APIs, authz notes, and CLI smoke: [docs/services/](docs/services/index.md).

## Defaults

| Setting | Value |
|---------|--------|
| Listen | `127.0.0.1:4566` only |
| Docker | No host `docker.sock` (nested `noctaxris-engine` for Lambda) |
| Credentials | Root keys via env injection |
| At rest | Secrets and CMK material sealed under the data volume |
| Authn | SigV4 on non-health paths (SAML/OIDC federation STS excepted) |
| Function egress | Platform deny on `noctaxris-fn` (unlike AWS Lambda default internet) |

## Docs

Architecture, configuration, and security posture: [docs/index.md](docs/index.md).

## Author

[![Kyaxris-Labs](https://img.shields.io/badge/GitHub-Kyaxris--Labs-181717?logo=github)](https://github.com/Kyaxris-Labs)
[![Noctaxris](https://img.shields.io/badge/repo-Noctaxris-0A66C2?logo=go)](https://github.com/Kyaxris-Labs/Noctaxris)

## License

[MIT](LICENSE)
