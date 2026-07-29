# CodePipeline

**Status:** shipped (lab core)

Pipeline CRUD, StartPipelineExecution, GetPipelineState, manual Approval pause/resume, and execution Get/List lite. Pipelines must include at least one CodeBuild action (`ProjectName`). Identity authz. Optional PassRole for `roleArn` with `codepipeline.amazonaws.com` trust.

## Implemented

| Area | Actions |
|------|---------|
| Pipeline | `CreatePipeline`, `GetPipeline`, `DeletePipeline` |
| Execution | `StartPipelineExecution`, `GetPipelineState`, `GetPipelineExecution`, `ListPipelineExecutions` |
| Approval | `PutApprovalResult` (Approved / Rejected) |

Stages with `actionTypeId.category=Approval` (provider `Manual`) pause `StartPipelineExecution` with execution status `InProgress` until `PutApprovalResult`. `GetPipelineState` returns the approval `token` under `actionStates[].latestExecution`. Approved continues subsequent stages (CodeBuild `StartBuild` when present). Rejected marks the execution Failed.

CodeBuild stages call nested `StartBuild` for each `ProjectName`. Without `NOCTAXRIS_DOCKER_HOST`, the build row is created and marked Failed (compute unavailable). With DinD configured, the shared compute client runs the buildspec in a nested container.

### Authz notes

Identity `EvaluateFull` on `codepipeline:*`. When `roleArn` is set, PassRole plus `codepipeline.amazonaws.com` trust is required.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws codepipeline create-pipeline --cli-input-json file://pipeline.json --endpoint-url "$EP"
aws codepipeline start-pipeline-execution --name lab-pipe --endpoint-url "$EP"
aws codepipeline get-pipeline-state --name lab-pipe --endpoint-url "$EP"
# copy token from stageStates[].actionStates[].latestExecution.token, then:
aws codepipeline put-approval-result --cli-input-json file://approval.json --endpoint-url "$EP"
aws codepipeline get-pipeline-execution --pipeline-name lab-pipe --pipeline-execution-id "$EXEC" --endpoint-url "$EP"
aws codepipeline list-pipeline-executions --pipeline-name lab-pipe --endpoint-url "$EP"
aws codebuild list-builds-for-project --project-name proj --endpoint-url "$EP"
```

`pipeline.json` must declare a CodeBuild action with `ProjectName` that already exists as a CodeBuild project. Optional Approval stage example:

```json
{
  "name": "Approve",
  "actions": [{
    "name": "ManualGate",
    "actionTypeId": {
      "category": "Approval",
      "owner": "AWS",
      "provider": "Manual",
      "version": "1"
    }
  }]
}
```

## Not yet / deferred

- Full action catalog, cross-region, stop/retry/rollback
