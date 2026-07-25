# Detective

**Status:** shipped (lab lite)

Behavior graph lite over CloudTrail JSONL and GuardDuty findings for forensic correlation. Identity authz.

## Implemented

| Area | Actions |
|------|---------|
| Graph | `CreateGraph`, `ListGraphs`, `AcceptInvitation` (noop lite) |
| Search | Lab `SearchGraph` (Noctaxris lab extension; not a full AWS Detective API) joins CT events and GuardDuty findings by resource ARN or source IP |

### Authz notes

Identity `EvaluateFull` on `detective:*`.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification). Create a graph, inject CT/GuardDuty data, then SearchGraph via SigV4 JSON.

## Not yet / deferred

- Member invitations and organization admin accounts
- Full entity timeline UI APIs
- Finding groups / attack sequences beyond lab search
