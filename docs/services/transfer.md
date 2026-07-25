# Transfer Family

**Status:** shipped (lab core)

Server and user CRUD with an SFTP-shaped sandbox filesystem under the data root. Servers create as `State=ONLINE`. There is no real SFTP/SSH listener and no extra Compose port: file Put/Get/List is a lab HTTP API on `:4566` only. Homes live under `transfer/ACCOUNT/SERVER/home/USER/` inside the lab data directory. Describe omits `EndpointType` (no VPC theatre). `EndpointDetails` and client `EndpointType` are rejected.

## Implemented

| Area | Actions / paths |
|------|-----------------|
| Server | `CreateServer`, `DescribeServer`, `ListServers`, `DeleteServer` (`State=ONLINE`) |
| User | `CreateUser`, `DeleteUser` |
| Protocol | SFTP only (label; not a live SFTP daemon) |
| Storage | Per-user sandbox directory under data root |
| Lab files | `PutFile` / `GetFile` / `ListDirectory` (JSON targets) and `PUT|GET /transfer/{serverId}/home/{user}/{path...}` |

### Authz notes

Identity `EvaluateFull` on `transfer:*` against the server ARN (or `*` for list/create). Lab file actions use `transfer:PutFile`, `transfer:GetFile`, `transfer:ListDirectory`. Non-empty CreateUser `Role` requires `iam:PassRole` plus `transfer.amazonaws.com` trust. Paths are confined with `filepath.Clean`; `../` escapes fail closed.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
SID=$(aws transfer create-server --protocols SFTP --endpoint-url "$EP" --query ServerId --output text)
aws transfer describe-server --server-id "$SID" --endpoint-url "$EP"
aws transfer create-user --server-id "$SID" --user-name alice --endpoint-url "$EP"
aws transfer list-servers --endpoint-url "$EP"
```

Lab file API (JSON targets via any SigV4 client for service `transfer`):

```bash
# PutFile / GetFile / ListDirectory with ServerId, UserName, Path
# Or: PUT/GET $EP/transfer/$SID/home/alice/inbox/hello.txt
```

No live SFTP port is published. Unit tests assert ONLINE create, put/get roundtrip, and path-traversal rejection.

## Not yet / deferred

- Real SFTP listener even on loopback
- AS2, FTPS, IdP integration
- WAN expose
