# Step Functions

**Status:** shipped (lab core)

Standard state machines with a small ASL subset. Executions run synchronously in-process. Identity authz. Optional PassRole for `RoleArn` with `states.amazonaws.com` trust.

## Implemented

| Area | Actions |
|------|---------|
| State machines | `CreateStateMachine`, `DeleteStateMachine`, `DescribeStateMachine`, `ListStateMachines` |
| Executions | `StartExecution`, `DescribeExecution`, `GetExecutionHistory` |
| ASL subset | `Pass`, `Succeed`, `Fail`, `Task` (Resource = Lambda function ARN or name, sync Invoke) |

Task states call the same Lambda Invoke path as `lambda:InvokeFunction`. Without nested compute (`DockerHost` empty), Task fails with `States.TaskFailed` / compute unavailable. Pass/Succeed/Fail do not need Docker.

### Authz notes

Identity `EvaluateFull` on `states:*`. When `roleArn` is set on CreateStateMachine, PassRole plus `states.amazonaws.com` trust is required. Task Invoke also requires `lambda:InvokeFunction` (and function policy when present).

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
DEF='{"StartAt":"Hello","States":{"Hello":{"Type":"Pass","Result":{"ok":true},"End":true}}}'
SM=$(aws stepfunctions create-state-machine \
  --name "lab-sm-$RANDOM" \
  --definition "$DEF" \
  --endpoint-url "$EP" \
  --query stateMachineArn --output text)

EXEC=$(aws stepfunctions start-execution \
  --state-machine-arn "$SM" \
  --input '{"n":1}' \
  --endpoint-url "$EP" \
  --query executionArn --output text)

aws stepfunctions describe-execution --execution-arn "$EXEC" --endpoint-url "$EP"
aws stepfunctions get-execution-history --execution-arn "$EXEC" --endpoint-url "$EP"
aws stepfunctions delete-state-machine --state-machine-arn "$SM" --endpoint-url "$EP"
```

Task to Lambda needs Compose DinD for a successful Invoke. Without Docker, expect FAILED status with compute unavailable in the cause.

## Not yet / deferred

- Choice, Wait, Parallel, Map, Callback / activity patterns
- Express workflows, Map distributed mode
- InputPath / ResultPath / OutputPath depth beyond Pass Result
