# Organizations

**Status:** shipped

Organizations MVP for multi-account labs: create accounts, list accounts, OUs, move accounts into OUs, enable policy types, SCP/RCP create/attach/detach/describe, and list helpers for policies, parents, and accounts-for-parent. Shared authorize uses identity, boundary, SCP, and RCP. SCP and RCP collection walks the OU path from the account parent to the organization root. Mutate and broad-list Organizations APIs require the lab management account (`000000000001`) after IAM Allow. Organization trail flag (`IsOrganizationTrail`) is on CloudTrail CreateTrail for the management account (see [cloudtrail.md](cloudtrail.md)).

## Implemented

| Action | Notes |
|--------|-------|
| `CreateAccount` | Management account only. Sync SUCCEEDED. Member gets `OrganizationAccountAccessRole` trusted by management root. New members sit under root until `MoveAccount` |
| `DescribeCreateAccountStatus` | Caller that requested the create, or management, may read status; others get not-found |
| `ListAccounts` | Management only. Returns management account `000000000001` plus CreateAccount members |
| `CreateOrganizationalUnit` | Management only. OU under a parent (root or nested OU) |
| `ListOrganizationalUnitsForParent` | Management only. List OUs for a parent |
| `MoveAccount` | Management only. Move a member from source parent (root or OU) to destination parent |
| `EnablePolicyType` | Management only. Enable SCP or RCP policy type |
| `CreatePolicy` | Management only. SCP or RCP document |
| `AttachPolicy` | Management only. Attach to root, OU, or account target |
| `DetachPolicy` | Management only. Detach from target |
| `DescribePolicy` | Management only. Describe policy content |
| `ListPolicies` | Management only. List org SCP/RCP policies (SigV4 service `organizations`; IAM `ListPolicies` stays on `iam`) |
| `ListPoliciesForTarget` | Management only. Policies attached to root, OU, or account |
| `ListParents` | Management only. Parent root/OU for an account or OU |
| `ListAccountsForParent` | Management only. Accounts under a root or OU |

### Authz notes

Organizations APIs use `EvaluateFull` through the shared authorize path: identity (including groups), permissions boundary, session policies, SCP, and RCP. SCPs and RCPs never grant alone. Management account is exempt from SCP. After IAM Allow, mutate and list Organizations actions also require the caller account to be the lab management account; member accounts receive AccessDenied (`organizations API requires management account`). `DescribeCreateAccountStatus` stays IAM-scoped and returns status only when `requested_by_account_id` matches the caller or the caller is management.

For each member account, authorize loads SCP and RCP documents attached to the account, each OU on the path from the account parent up to the root, and the organization root. Documents are deduped by policy id. Management account exemption is unchanged.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws organizations create-account \
  --email member@example.com \
  --account-name Member \
  --endpoint-url "$EP"

aws organizations list-accounts --endpoint-url "$EP"

aws organizations create-organizational-unit \
  --parent-id r-root \
  --name Workloads \
  --endpoint-url "$EP"

aws organizations move-account \
  --account-id 000000000002 \
  --source-parent-id r-root \
  --destination-parent-id ou-REPLACE \
  --endpoint-url "$EP"
```

Expect management account `000000000001` plus any `CreateAccount` members. After `move-account`, SCPs attached to the destination OU apply to that member on identity and data-plane authorize paths.

## Out of lab scope

- Account invites and handshake control-plane APIs beyond the lab MoveAccount / OU model (out of lab scope)
