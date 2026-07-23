# Transfer Family

**Status:** shipped (lab core)

Server and user CRUD with an SFTP-shaped sandbox filesystem under the data root. No WAN listener and no loopback SFTP listener. Homes live under `transfer/ACCOUNT/SERVER/home/USER/` inside the lab data directory. Describe reports `State=OFFLINE` and omits `EndpointType` (no VPC theatre). `EndpointDetails` and client `EndpointType` are rejected.

## Implemented

| Area | Actions |
|------|---------|
| Server | `CreateServer`, `DescribeServer`, `ListServers`, `DeleteServer` |
| User | `CreateUser`, `DeleteUser` |
| Protocol | SFTP only |
| Storage | Per-user sandbox directory under data root |

### Authz notes

Identity `EvaluateFull` on `transfer:*` against the server ARN (or `*` for list/create). Non-empty CreateUser `Role` requires `iam:PassRole` plus `transfer.amazonaws.com` trust.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
SID=$(aws transfer create-server --protocols SFTP --endpoint-url "$EP" --query ServerId --output text)
aws transfer describe-server --server-id "$SID" --endpoint-url "$EP"
aws transfer create-user --server-id "$SID" --user-name alice --endpoint-url "$EP"
aws transfer list-servers --endpoint-url "$EP"
aws transfer delete-user --server-id "$SID" --user-name alice --endpoint-url "$EP"
aws transfer delete-server --server-id "$SID" --endpoint-url "$EP"
```

No live SFTP port is published. Unit tests assert the sandbox home directory is created and removed with the user.

## Not yet / deferred

- Real SFTP listener even on loopback
- AS2, FTPS, IdP integration
- WAN expose
