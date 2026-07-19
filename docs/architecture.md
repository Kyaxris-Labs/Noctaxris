# Architecture (Phase 2)

One Go binary runs in the container. There is no sidecar API gateway and no Docker socket mount.

## Tree

```text
Noctaxris/
  cmd/noctaxris/
  docker/
  internal/config/
  internal/store/                 # identity, policies, orgs, roles, temp creds
  internal/catalog/
  internal/kernel/audit/
  internal/kernel/authn/          # SigV4 + session token
  internal/kernel/authz/          # single-account + EvaluateCrossAccount
  internal/kernel/identity/
  internal/kernel/sts/            # GetCallerIdentity, AssumeRole XML
  internal/services/organizations/
  internal/server/
  docs/
```

## Request path

```text
HTTP request
  ├─ GET /_noctaxris/health → 200 ok
  └─ else
       ├─ authn.Verify (SigV4, session token if temp key)
       ├─ GetCallerIdentity → XML (no IAM Evaluate)
       ├─ CreateAccount / DescribeCreateAccountStatus → orgs authz then handler
       ├─ AssumeRole → EvaluateCrossAccount then mint temp creds
       └─ other → 501 NotImplemented
```

CreateAccount completes synchronously with state `SUCCEEDED` and seeds `OrganizationAccountAccessRole` in the member account, trusted by the management account root.

## Authz

- Same-account: `Evaluate` (root Allow, explicit Deny, Allow, implicit Deny)
- AssumeRole: `EvaluateCrossAccount` requires caller identity Allow and trust policy Allow

