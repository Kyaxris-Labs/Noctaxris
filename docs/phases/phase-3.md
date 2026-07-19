# Phase 3 status

Phase 3 delivers the full STS 11-action surface with fail-closed SAML/OIDC federation, plus a lab-complete IAM user/role/policy/access-key control plane.

## Delivered

| Area | Status |
|------|--------|
| Lab IAM users, access keys, managed/inline policies, roles | Done |
| IAM APIs under `Evaluate` (`iam:*`) | Done |
| All 11 STS action routes | Done |
| GetSessionToken / GetFederationToken | Done |
| Session policy intersection (`EvaluateWithSession`) | Done |
| AssumeRoleWithSAML / AssumeRoleWithWebIdentity (crypto vs configured IdP) | Done (fail-closed without IdP) |
| AssumeRoot (member account only when org relationship exists) | Done |
| DecodeAuthorizationMessage / GetAccessKeyInfo | Done |
| GetDelegatedAccessToken / GetWebIdentityToken | Fail-closed stubs |
| Deferred backlog | [../deferred.md](../deferred.md) |

## Lab IAM (in scope)

Users: CreateUser, GetUser, ListUsers, DeleteUser

Access keys: CreateAccessKey, DeleteAccessKey, ListAccessKeys, UpdateAccessKey

Managed policies: CreatePolicy, GetPolicy, ListPolicies, DeletePolicy, Attach/Detach user and role, ListAttached*

Inline policies: Put/Get/Delete/List for user and role

Roles: CreateRole, GetRole, ListRoles, DeleteRole, UpdateAssumeRolePolicy

## STS (all 11)

Already from earlier phases: GetCallerIdentity, AssumeRole

Added: GetSessionToken, GetFederationToken, AssumeRoleWithSAML, AssumeRoleWithWebIdentity, AssumeRoot, DecodeAuthorizationMessage, GetAccessKeyInfo, GetDelegatedAccessToken, GetWebIdentityToken

Federation never accepts arbitrary assertions. Without configured IdP metadata or OIDC issuer, those paths deny.

## Smoke (Compose)

See [../verification.md](../verification.md) for IAM user/key, CreateRole/AssumeRole, GetSessionToken, and fail-closed web identity.

## Explicitly not in Phase 3

See [../deferred.md](../deferred.md). No KMS, S3, DynamoDB, SQS, or Lambda. No full IAM parity, SCPs/RCPs, or MFA device registry.
