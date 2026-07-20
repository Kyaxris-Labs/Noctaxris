# CodeDeploy

**Status:** shipped (lab core)

CreateApplication, CreateDeploymentGroup, CreateDeployment, GetDeployment, and ListDeployments. Deployments record `Succeeded` synchronously. Optional PassRole when `serviceRoleArn` is set (`codedeploy.amazonaws.com` trust). When a deployment group references an ECS service name, CreateDeployment best-effort refreshes DesiredCount via existing ECS helpers. When it references `lambdaFunctionName`, CreateDeployment best-effort calls Lambda `PublishVersion`. No host docker.sock. No EC2 agent.

## Implemented

| Area | Actions |
|------|---------|
| Application | `CreateApplication` |
| Deployment group | `CreateDeploymentGroup` |
| Deployment | `CreateDeployment`, `GetDeployment`, `ListDeployments` |

### Authz notes

Identity `EvaluateFull` on `codedeploy:*`. When `serviceRoleArn` is set, PassRole plus `codedeploy.amazonaws.com` trust is required.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws deploy create-application --application-name LabApp --compute-platform ECS --endpoint-url "$EP"
aws deploy create-deployment-group --application-name LabApp --deployment-group-name LabDG --endpoint-url "$EP"
aws deploy create-deployment --application-name LabApp --deployment-group-name LabDG --endpoint-url "$EP"
aws deploy get-deployment --deployment-id d-... --endpoint-url "$EP"
```

Live Compose smoke skipped when Docker is unavailable.

## Not yet / deferred

- Blue/green traffic shifting depth
- CodeDeploy agent on EC2
- On-premises instances
- Weighted alias traffic shifting beyond PublishVersion on named function
