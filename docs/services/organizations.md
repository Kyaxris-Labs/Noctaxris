# Organizations

**Status:** shipped

Organizations MVP for multi-account labs: create accounts, list accounts, OUs, move accounts into OUs, enable policy types, and SCP/RCP create/attach/detach/describe. Shared authorize uses identity, boundary, SCP, and RCP. SCP and RCP collection walks the OU path from the account parent to the organization root.

## Implemented

| Action | Notes |
|--------|-------|
| `CreateAccount` | Sync SUCCEEDED. Member gets `OrganizationAccountAccessRole` trusted by management root. New members sit under root until `MoveAccount` |
| `DescribeCreateAccountStatus` | Status for create requests |
| `ListAccounts` | Management account `000000000001` plus CreateAccount members |
| `CreateOrganizationalUnit` | OU under a parent (root or nested OU) |
| `ListOrganizationalUnitsForParent` | List OUs for a parent |
| `MoveAccount` | Move a member from source parent (root or OU) to destination parent |
| `EnablePolicyType` | Enable SCP or RCP policy type |
| `CreatePolicy` | SCP or RCP document |
| `AttachPolicy` | Attach to root, OU, or account target |
| `DetachPolicy` | Detach from target |
| `DescribePolicy` | Describe policy content |

### Authz notes

Organizations APIs use `EvaluateFull` through the shared authorize path: identity (including groups), permissions boundary, session policies, SCP, and RCP. SCPs and RCPs never grant alone. Management account is exempt from SCP.

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

## Not yet / deferred

- Account invites and related control-plane APIs beyond the lab subset
