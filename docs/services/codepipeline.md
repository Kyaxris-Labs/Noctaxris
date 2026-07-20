# CodePipeline

**Status:** shipped (lab core)

Pipeline CRUD, StartPipelineExecution, and GetPipelineState lite. Pipelines must include at least one CodeBuild action (`ProjectName`). Identity authz. Optional PassRole for `roleArn` with `codepipeline.amazonaws.com` trust.

## Implemented

| Area | Actions |
|------|---------|
| Pipeline | `CreatePipeline`, `GetPipeline`, `DeletePipeline` |
| Execution | `StartPipelineExecution`, `GetPipelineState` |

CodeBuild stages call nested `StartBuild` for each `ProjectName`. Without `NOCTAXRIS_DOCKER_HOST`, the build row is created and marked SUCCEEDED for unit labs. With DinD configured, the shared compute client runs the buildspec in a nested container.

### Authz notes

Identity `EvaluateFull` on `codepipeline:*`. When `roleArn` is set, PassRole plus `codepipeline.amazonaws.com` trust is required.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws codepipeline create-pipeline --cli-input-json file://pipeline.json --endpoint-url "$EP"
aws codepipeline start-pipeline-execution --name lab-pipe --endpoint-url "$EP"
aws codepipeline get-pipeline-state --name lab-pipe --endpoint-url "$EP"
aws codebuild list-builds-for-project --project-name proj --endpoint-url "$EP"
```

`pipeline.json` must declare a CodeBuild action with `ProjectName` that already exists as a CodeBuild project.

## Not yet / deferred

- Full action catalog, manual approvals, cross-region
