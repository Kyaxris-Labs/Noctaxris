# Noctaxris

Local AWS emulator for cloud security labs. IAM-faithful and secure by default.

Repo and Go module: `github.com/Kyaxris-Labs/Noctaxris` (PascalCase `Noctaxris`, not all-lowercase).

Primary distribution: Docker image under [Kyaxris-Labs](https://github.com/Kyaxris-Labs).

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
      <td>CreateAccount, ListAccounts, OUs, EnablePolicyType, SCP and RCP create/attach/detach/describe. Shared authorize uses identity, boundary, SCP, and RCP.</td>
      <td>Invites, OU-path SCP/RCP inheritance, SCP/boundary on data-plane authorize paths.</td>
    </tr>
    <tr>
      <td rowspan="1" align="center" valign="middle">Crypto</td>
      <td>KMS</td>
      <td>Customer-managed keys, key policies (explicit allow), Encrypt, Decrypt, GenerateDataKey*, grants, aliases.</td>
      <td>Full KMS SAR (ReEncrypt, Sign/Verify, MAC, asymmetric, import, multi-Region, rotation APIs, tags). Cross-account key policy depth. AWS-managed key convenience aliases.</td>
    </tr>
    <tr>
      <td rowspan="7" align="center" valign="middle">Data</td>
      <td>S3</td>
      <td>Path-style buckets and objects, bucket policy, SSE-S3/SSE-KMS, presigned GET/PUT.</td>
      <td>Multipart, CopyObject, versioning, lifecycle, virtual-hosted style, cross-account policy depth.</td>
    </tr>
    <tr>
      <td>DynamoDB</td>
      <td>Tables, item CRUD, Query/Scan, BatchGet/BatchWrite, table resource policies, CMK encryption.</td>
      <td>GSI/LSI, Streams, Transactions, PartiQL, global tables.</td>
    </tr>
    <tr>
      <td>SQS</td>
      <td>Standard queues, send/receive/delete (batch and visibility), queue policies, SSE-SQS and SSE-KMS.</td>
      <td>FIFO and DLQ redrive depth.</td>
    </tr>
    <tr>
      <td>SSM Parameter Store</td>
      <td>not shipped</td>
      <td>Planned lab-complete Parameter Store (not full SAR).</td>
    </tr>
    <tr>
      <td>Secrets Manager</td>
      <td>not shipped</td>
      <td>Planned lab-complete Secrets Manager (not full SAR).</td>
    </tr>
    <tr>
      <td>SNS</td>
      <td>not shipped</td>
      <td>Planned lab-complete SNS (not full SAR).</td>
    </tr>
    <tr>
      <td>EventBridge</td>
      <td>not shipped</td>
      <td>Planned lab-complete EventBridge (not full SAR).</td>
    </tr>
    <tr>
      <td rowspan="3" align="center" valign="middle">Compute</td>
      <td>Lambda</td>
      <td>Zip <code>python3.12</code> CreateFunction, Get, Delete, List, UpdateCode, UpdateConfiguration, sync Invoke, PassRole plus <code>lambda.amazonaws.com</code> trust, nested DinD (no host <code>docker.sock</code>), platform egress deny.</td>
      <td>Layers, versions/aliases depth, event source mappings, async invoke and DLQ, image packaging, other runtimes, cross-account resource policies, engine TLS/rootless hardening. Post-v2: Firecracker microVMs.</td>
    </tr>
    <tr>
      <td>ECR</td>
      <td>not shipped</td>
      <td>Planned lab-complete ECR (not full SAR).</td>
    </tr>
    <tr>
      <td>ECS</td>
      <td>not shipped</td>
      <td>Planned lab-complete ECS (not full SAR).</td>
    </tr>
  </tbody>
</table>

More detail: [docs/deferred.md](docs/deferred.md).

## Quick start

1. Copy `docker/.env.example` to `docker/.env` and set root access keys.
2. `docker compose -f docker/compose.yaml --env-file docker/.env up --build`
3. `curl http://127.0.0.1:4566/_noctaxris/health` (expect `ok`)
4. Point AWS CLI at the endpoint with the same root keys.

```bash
export AWS_ACCESS_KEY_ID=...          # from docker/.env
export AWS_SECRET_ACCESS_KEY=...
export AWS_DEFAULT_REGION=us-east-1
EP=http://127.0.0.1:4566

aws configure set default.s3.addressing_style path
aws sts get-caller-identity --endpoint-url "$EP"
aws s3 mb s3://lab-bucket --endpoint-url "$EP"
aws kms create-key --endpoint-url "$EP"
```

Full smoke (IAM, Orgs, KMS, S3, DynamoDB, SQS, Lambda): [docs/verification.md](docs/verification.md).

## Defaults

- Host publish `127.0.0.1:4566` only
- No host `docker.sock` (nested `noctaxris-engine` for Lambda only)
- Root credentials via env injection
- Secrets and CMK material encrypted at rest under the data volume
- SigV4 required on non-health paths (except SAML/OIDC federation STS)
- Function containers: platform egress deny (unlike AWS Lambda default internet)

## Docs

See [docs/index.md](docs/index.md) for architecture, configuration, security, and verification.

## License

[MIT](LICENSE)
