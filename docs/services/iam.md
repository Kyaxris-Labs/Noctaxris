# IAM

**Status:** shipped

Lab IAM control plane for users, roles, managed and inline policies, access keys, groups, permissions boundaries, instance profiles, OIDC and SAML IdP CRUD, and virtual MFA.

## Implemented

| Area | Actions |
|------|---------|
| Users | `CreateUser`, `GetUser`, `ListUsers`, `DeleteUser` |
| Access keys | `CreateAccessKey`, `DeleteAccessKey`, `ListAccessKeys`, `UpdateAccessKey` |
| Managed policies | `CreatePolicy`, `GetPolicy`, `ListPolicies`, `DeletePolicy`, `CreatePolicyVersion`, `GetPolicyVersion`, `ListPolicyVersions`, `DeletePolicyVersion`, `SetDefaultPolicyVersion` (max five versions; default document feeds Evaluate) |
| Attachments | `AttachUserPolicy`, `DetachUserPolicy`, `AttachRolePolicy`, `DetachRolePolicy`, `ListAttachedUserPolicies`, `ListAttachedRolePolicies` |
| Inline user | `PutUserPolicy`, `GetUserPolicy`, `DeleteUserPolicy`, `ListUserPolicies` |
| Inline role | `PutRolePolicy`, `GetRolePolicy`, `DeleteRolePolicy`, `ListRolePolicies` |
| Roles | `CreateRole`, `GetRole`, `ListRoles`, `DeleteRole`, `UpdateAssumeRolePolicy` |
| Groups | `CreateGroup`, `DeleteGroup`, `GetGroup`, `ListGroups`, `AddUserToGroup`, `RemoveUserFromGroup` |
| Group policies | `AttachGroupPolicy`, `DetachGroupPolicy`, `ListAttachedGroupPolicies`, `PutGroupPolicy`, `GetGroupPolicy`, `DeleteGroupPolicy`, `ListGroupPolicies` |
| Boundaries | `PutUserPermissionsBoundary`, `DeleteUserPermissionsBoundary`, `PutRolePermissionsBoundary`, `DeleteRolePermissionsBoundary`. `GetUser` / `GetRole` embed `PermissionsBoundary` and `CreateDate`. Lab-only: `GetUserPermissionsBoundary` / `GetRolePermissionsBoundary` |
| Instance profiles | `CreateInstanceProfile`, `DeleteInstanceProfile`, `GetInstanceProfile`, `AddRoleToInstanceProfile`, `RemoveRoleFromInstanceProfile`, `ListInstanceProfiles`, `ListInstanceProfilesForRole`. Role members include `Arn` and `RoleId` |
| OIDC IdP | `CreateOpenIDConnectProvider`, `DeleteOpenIDConnectProvider`, `ListOpenIDConnectProviders`, `GetOpenIDConnectProvider` |
| SAML IdP | `CreateSAMLProvider`, `DeleteSAMLProvider`, `ListSAMLProviders`, `GetSAMLProvider` |
| Virtual MFA | `CreateVirtualMFADevice`, `EnableMFADevice`, `ListMFADevices`, `DeactivateMFADevice` |

Group-attached and inline policies feed identity documents for authorization. Access-key secrets are sealed at rest.

### Authz notes

IAM APIs authorize through `EvaluateFull`: identity policies (including group docs), optional permissions boundary, session policies, SCP, and RCP. Boundaries intersect with identity. SCPs and RCPs never grant on their own. Management account is exempt from SCP. Root skips the boundary intersection. Assumed-role sessions resolve identity documents from the IAM role ARN (attachments and inline role policies), not the STS session ARN.

Condition-key catalogs for lab IAM (plus global keys) are loaded from the service catalog. Broader request-context population for every global key remains open. See [index.md](index.md#cross-cutting).

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws iam create-user --user-name labuser --endpoint-url "$EP"
aws iam create-access-key --user-name labuser --endpoint-url "$EP"

TRUST='{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::000000000001:root"},"Action":"sts:AssumeRole"}]}'
aws iam create-role --role-name LabRole --assume-role-policy-document "$TRUST" --endpoint-url "$EP"
```

Groups inherit attached and inline policies. Permissions boundaries intersect with identity policies in `EvaluateFull`.

```bash
aws iam create-group --group-name Admins --endpoint-url "$EP"
aws iam create-user --user-name alice --endpoint-url "$EP"
aws iam add-user-to-group --group-name Admins --user-name alice --endpoint-url "$EP"

LIST_DOC='{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"iam:ListUsers","Resource":"*"}]}'
aws iam create-policy --policy-name GroupListUsers --policy-document "$LIST_DOC" --endpoint-url "$EP"
POLICY_ARN=$(aws iam list-policies --scope Local --query "Policies[?PolicyName=='GroupListUsers'].Arn" --output text --endpoint-url "$EP")
aws iam attach-group-policy --group-name Admins --policy-arn "$POLICY_ARN" --endpoint-url "$EP"
```

Managed policy versioning (lab subset; `SetAsDefault` updates the operative document for attached principals):

```bash
ADMIN_DOC='{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"*","Resource":"*"}]}'
aws iam create-policy-version \
  --policy-arn "$POLICY_ARN" \
  --policy-document "$ADMIN_DOC" \
  --set-as-default \
  --endpoint-url "$EP"
aws iam list-policy-versions --policy-arn "$POLICY_ARN" --endpoint-url "$EP"
```

Boundary deny: identity allows `ListUsers` but the boundary allows only `GetUser`.

```bash
aws iam create-user --user-name bounded --endpoint-url "$EP"
BOUNDED_KEYS=$(aws iam create-access-key --user-name bounded --endpoint-url "$EP" --output json)
BOUNDED_AKID=$(echo "$BOUNDED_KEYS" | python3 -c 'import sys,json; print(json.load(sys.stdin)["AccessKey"]["AccessKeyId"])')
BOUNDED_SECRET=$(echo "$BOUNDED_KEYS" | python3 -c 'import sys,json; print(json.load(sys.stdin)["AccessKey"]["SecretAccessKey"])')

BOUND_DOC='{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"iam:GetUser","Resource":"*"}]}'
aws iam create-policy --policy-name UserBoundary --policy-document "$BOUND_DOC" --endpoint-url "$EP"
BOUND_ARN=$(aws iam list-policies --scope Local --query "Policies[?PolicyName=='UserBoundary'].Arn" --output text --endpoint-url "$EP")
aws iam put-user-permissions-boundary --user-name bounded --permissions-boundary "$BOUND_ARN" --endpoint-url "$EP"
aws iam attach-user-policy --user-name bounded --policy-arn "$POLICY_ARN" --endpoint-url "$EP"

AWS_ACCESS_KEY_ID="$BOUNDED_AKID" AWS_SECRET_ACCESS_KEY="$BOUNDED_SECRET" \
  aws iam list-users --endpoint-url "$EP"
```

Expect `AccessDenied` for the last command.

Virtual MFA device setup (lab token used by STS `GetSessionToken`): after `CreateVirtualMFADevice`, `Base32StringSeed` is hex-encoded seed bytes. Token is the first 6 hex chars of `sha256(seed + ":" + unixMinute)` (see `internal/kernel/sts/mfa.go`). `EnableMFADevice` requires `AuthenticationCode1` and `AuthenticationCode2` as consecutive lab tokens (current and next minute). Full MFA session smoke is on [sts.md](sts.md).

```bash
aws iam create-user --user-name mfa-user --endpoint-url "$EP"
MFA_JSON=$(aws iam create-virtual-mfa-device --endpoint-url "$EP" --output json)
SERIAL=$(echo "$MFA_JSON" | python3 -c 'import sys,json; print(json.load(sys.stdin)["VirtualMFADevice"]["SerialNumber"])')
SEED_HEX=$(echo "$MFA_JSON" | python3 -c 'import sys,json; print(json.load(sys.stdin)["VirtualMFADevice"]["Base32StringSeed"])')
CODES=$(SEED_HEX="$SEED_HEX" python3 - <<'PY'
import hashlib, time, os, binascii
seed = binascii.unhexlify(os.environ["SEED_HEX"])
m = int(time.time()) // 60
def code(minute):
    return hashlib.sha256(seed + (":%d" % minute).encode("ascii")).hexdigest()[:6]
print(code(m), code(m + 1))
PY
)
CODE1=$(echo "$CODES" | awk '{print $1}')
CODE2=$(echo "$CODES" | awk '{print $2}')
aws iam enable-mfa-device \
  --user-name mfa-user \
  --serial-number "$SERIAL" \
  --authentication-code1 "$CODE1" \
  --authentication-code2 "$CODE2" \
  --endpoint-url "$EP"
```

## Not yet / deferred

- Service-linked roles
- Full IAM pagination, tagging, and API parity beyond the lab subset
- PassRole trust Conditions beyond StringEquals on common keys (`aws:SourceAccount` / `aws:SourceArn` when populated); full operator matrix (ArnLike, Bool, …)
