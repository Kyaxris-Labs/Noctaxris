# Organizations

**Status:** shipped

Organizations MVP for multi-account labs: create accounts, list accounts, OUs, enable policy types, and SCP/RCP create/attach/detach/describe. Shared authorize uses identity, boundary, SCP, and RCP.

## Implemented

| Action | Notes |
|--------|-------|
| `CreateAccount` | Sync SUCCEEDED. Member gets `OrganizationAccountAccessRole` trusted by management root |
| `DescribeCreateAccountStatus` | Status for create requests |
| `ListAccounts` | Management account `000000000001` plus CreateAccount members |
| `CreateOrganizationalUnit` | OU under a parent |
| `ListOrganizationalUnitsForParent` | List OUs for a parent |
| `EnablePolicyType` | Enable SCP or RCP policy type |
| `CreatePolicy` | SCP or RCP document |
| `AttachPolicy` | Attach to root, OU, or account target |
| `DetachPolicy` | Detach from target |
| `DescribePolicy` | Describe policy content |

### Authz notes

Organizations APIs use `EvaluateFull` through the shared authorize path: identity (including groups), permissions boundary, session policies, SCP, and RCP. SCPs and RCPs never grant alone. Management account is exempt from SCP.

Attached SCP/RCP documents also participate in identity and data-plane authorize paths (org filters deny before resource-or-identity allow). OU-path inheritance is not stored yet. Account placement in OUs does not drive inherited filter sets.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws organizations create-account \
  --email member@example.com \
  --account-name Member \
  --endpoint-url "$EP"

aws organizations list-accounts --endpoint-url "$EP"
```

Expect management account `000000000001` plus any `CreateAccount` members.

## Not yet / deferred

- Account invites and related control-plane APIs beyond the lab subset
- OU-path SCP and RCP inheritance (account placement in OUs is not stored yet)
