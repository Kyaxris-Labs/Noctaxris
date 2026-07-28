# Transfer Family

**Status:** shipped (lab core)

Server and user CRUD with an SFTP-shaped sandbox filesystem under the data root. Servers create as `State=ONLINE`. There is no real SFTP/SSH listener and no extra Compose port: file Put/Get/List is a lab HTTP API on the same `:4566` API port only (SigV4 service `transfer`). Homes live under `transfer/ACCOUNT/SERVER/home/USER/` inside the lab data directory. Describe omits `EndpointType` (no VPC theatre). `EndpointDetails` and client `EndpointType` are rejected.

## Implemented

| Area | Actions / paths |
|------|-----------------|
| Server | `CreateServer`, `DescribeServer`, `ListServers`, `DeleteServer` (`State=ONLINE`) |
| User | `CreateUser`, `DeleteUser` |
| Protocol | SFTP only (label; not a live SFTP daemon) |
| Storage | Per-user sandbox directory under data root |
| Lab files | `PutFile` / `GetFile` / `ListDirectory` (JSON `X-Amz-Target` on `:4566`) and `PUT|GET /transfer/{serverId}/home/{user}/{path...}` on `:4566` |

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

Lab file API on the API port (not SFTP):

```bash
# JSON targets (service transfer, X-Amz-Target TransferService.PutFile / GetFile / ListDirectory):
#   ServerId, UserName, Path; PutFile Body or BodyBase64
# HTTP path (same host:port as the AWS API, SigV4 service transfer):
#   PUT|GET $EP/transfer/$SID/home/alice/inbox/hello.txt
```

No live SFTP port is published. Unit tests assert ONLINE create, JSON PutFile/GetFile roundtrip, HTTP `/transfer/.../home/...` Put/Get roundtrip, and path-traversal rejection.

## Not yet / deferred

- Real SFTP listener (even on loopback); lab files stay HTTP on `:4566`
- AS2, FTPS, IdP integration
- WAN expose
