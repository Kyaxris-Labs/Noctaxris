# Elastic Beanstalk

**Status:** shipped (lab core)

Query-protocol control-plane lab (`Action=` or `Operation=`, credential scope `elasticbeanstalk`). Applications, versions, and environments are stored state. Environments create as `Ready` / `Green` with no real platform deploy.

## Implemented

| Area | Actions |
|------|---------|
| Applications | `CreateApplication`, `DescribeApplications`, `DeleteApplication` |
| Versions | `CreateApplicationVersion` |
| Environments | `CreateEnvironment`, `DescribeEnvironments`, `TerminateEnvironment` |
| Platforms | `ListAvailableSolutionStacks` (static lab list) |

### Authz notes

Identity `EvaluateFull` on `elasticbeanstalk:*`.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws elasticbeanstalk list-available-solution-stacks --endpoint-url "$EP"
aws elasticbeanstalk create-application --application-name sample-app --endpoint-url "$EP"
aws elasticbeanstalk create-application-version \
  --application-name sample-app --version-label v1 \
  --source-bundle S3Bucket=src,S3Key=app.zip \
  --endpoint-url "$EP"
aws elasticbeanstalk create-environment \
  --application-name sample-app --environment-name sample-env --version-label v1 \
  --endpoint-url "$EP"
aws elasticbeanstalk describe-environments --application-name sample-app --endpoint-url "$EP"
aws elasticbeanstalk terminate-environment --environment-name sample-env --endpoint-url "$EP"
```

Unit tests cover application create, Ready environment, and terminate.

## Not yet / deferred

- Real platform provision or nested compute
- Configuration settings / option settings depth
- UpdateEnvironment, CheckDNSAvailability
