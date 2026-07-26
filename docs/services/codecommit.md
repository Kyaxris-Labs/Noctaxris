# CodeCommit

**Status:** shipped (lab subset)

CodeCommit-shaped lab store for repositories and files under the data root (`codecommit/<account>/<repo>/tree`). JSON protocol via `X-Amz-Target: CodeCommit_20150413.*`. This is not git smart-HTTP, SSH, or AWS CodeCommit protocol parity. Clone URLs in metadata are decorative. CodeBuild `StartBuild` with `source.type=CODECOMMIT` materializes the lab tree (`MaterializeCodeCommitRepo`) and injects it into the nested build under `/codebuild/src` (see [codebuild.md](codebuild.md)). Store helpers `ExportCodeCommitRepoTree` / `MaterializeCodeCommitRepo` / `BatchPutCodeCommitFiles` remain available for operators and tests.

## Implemented

| Area | Actions |
|------|---------|
| Repositories | `CreateRepository`, `GetRepository`, `ListRepositories`, `DeleteRepository` |
| Files | `PutFile` (base64 `fileContent`), `GetFile`, `GetFolder` |
| Clone helpers (store) | `ExportCodeCommitRepoTree(account, repo)`, `MaterializeCodeCommitRepo(account, repo, destDir)`, `BatchPutCodeCommitFiles` |

### Authz notes

Identity `EvaluateFull` on repository ARNs (`arn:aws:codecommit:REGION:ACCOUNT:NAME`) where applicable. List uses `*`.

### Lab limits

- Single default branch (`main`); other branch names are rejected.
- Files live on a simple filesystem tree (content-addressed blob/commit IDs for API responses only).
- No git objects, refs protocol, merge, or pull/push over HTTPS/SSH.
- Optional `parentCommitId` is checked when provided; otherwise PutFile advances head without requiring it.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws codecommit create-repository --repository-name lab-repo --repository-description 'lab' --endpoint-url "$EP"
aws codecommit list-repositories --endpoint-url "$EP"
aws codecommit get-repository --repository-name lab-repo --endpoint-url "$EP"

# PutFile expects base64 file content on the JSON API; CLI put-file may encode for you:
printf 'hello' | aws codecommit put-file \
  --repository-name lab-repo \
  --branch-name main \
  --file-path README.md \
  --file-content fileb://- \
  --endpoint-url "$EP"

aws codecommit get-file --repository-name lab-repo --file-path README.md --endpoint-url "$EP"
aws codecommit get-folder --repository-name lab-repo --folder-path "" --endpoint-url "$EP"
aws codecommit delete-repository --repository-name lab-repo --endpoint-url "$EP"
```

Unit coverage: `go test ./internal/store/ ./internal/server/ -run CodeCommit -count=1`.

## Not yet / deferred

- Git smart-HTTP / SSH clone and push
- Multi-branch refs, merge, pull requests
- Full CreateCommit / GetCommit / GetDifferences depth
