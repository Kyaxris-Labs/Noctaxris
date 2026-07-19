# Architecture (Phase 3)

One Go binary runs in the container. There is no sidecar API gateway and no Docker socket mount.

## Tree

```text
Noctaxris/
  cmd/noctaxris/
  docker/
  internal/config/
  internal/store/                 # users, keys, policies, roles, orgs, IdP, temp creds
  internal/catalog/
  internal/kernel/audit/
  internal/kernel/authn/          # SigV4 + session token
  internal/kernel/authz/          # Evaluate, EvaluateCrossAccount, EvaluateWithSession
  internal/kernel/identity/
  internal/kernel/federation/     # SAML / OIDC verification (fail-closed)
  internal/kernel/sts/            # STS XML builders
  internal/services/iam/
  internal/services/organizations/
  internal/server/
  docs/                             # public reference (see docs/index.md)
```

## Request path

```text
HTTP request
  ├─ GET /_noctaxris/health → 200 ok
  └─ else
       ├─ resolve Action
       ├─ AssumeRoleWithSAML / AssumeRoleWithWebIdentity → IdP crypto (no SigV4) then dual-eval
       ├─ else authn.Verify (SigV4, session token if temp key, inactive keys rejected)
       ├─ GetCallerIdentity → XML (no IAM Evaluate)
       ├─ other STS → Evaluate / dual-eval then handler
       ├─ IAM lab APIs → Evaluate iam:* then handler
       ├─ Organizations CreateAccount / DescribeCreateAccountStatus → Evaluate then handler
       └─ unknown → 501 NotImplemented
```

Input validation for account ids, IAM names, emails, policy JSON, and IdP paths uses `internal/validate` (`go-playground/validator`). Request and event ids use `google/uuid`. OIDC JWT checks use `go-jose`.

SigV4 is required for every non-health path except `AssumeRoleWithSAML` and `AssumeRoleWithWebIdentity`, which authenticate with the federation token.

## Authz

- Same-account: `Evaluate` (root Allow, explicit Deny, Allow, implicit Deny)
- Attached managed + inline policy documents are merged for the principal
- Federated / assume sessions with a session policy use `EvaluateWithSession` (intersection)
- AssumeRole and federation role assumption: `EvaluateCrossAccount` (caller identity + trust)
- Federation crypto must succeed against configured IdP before dual-eval mint

## Identity model

- Env-injected keys are management account root
- IAM users get long-lived `AKIA` keys (secret sealed at rest)
- Temporary `ASIA` keys require `X-Amz-Security-Token`
- Optional IdP bootstrap via env paths/URLs into SQLite (see [configuration.md](configuration.md))
