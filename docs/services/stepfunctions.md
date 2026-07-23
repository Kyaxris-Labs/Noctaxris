# Step Functions

**Status:** shipped (lab core)

Standard state machines with a small ASL subset. Executions run synchronously in-process. Identity authz. PassRole for `RoleArn` with `states.amazonaws.com` trust. Definitions that contain Task require `roleArn` at create time. Task delivery uses the state machine `RoleArn` session (or a target resource policy Allow for `states.amazonaws.com`) for Lambda Invoke and built-in SQS/SNS/EventBridge Tasks; foreign SQS/Lambda/SNS targets require RoleArn session Allow **and** destination resource policy. EventBridge PutEvents Tasks also dual-eval the bus resource policy after RoleArn Allow. StartExecution callers are not used for Task resource access. EventBridge Scheduler may target a state machine ARN with RoleArn (`states:StartExecution`). EventBridge rule targets may omit RoleArn when a lab state-machine resource policy Allows `events.amazonaws.com` for `states:StartExecution` (AWS SFN has no resource-based policies; RoleArn remains the AWS-shaped path).

## Implemented

| Area | Actions |
|------|---------|
| State machines | `CreateStateMachine`, `DeleteStateMachine`, `DescribeStateMachine`, `ListStateMachines` |
| Executions | `StartExecution`, `DescribeExecution`, `GetExecutionHistory` |
| Resource policy | Lab `PutResourcePolicy` / `GetResourcePolicy` / `DeleteResourcePolicy` on the state machine (for EventBridge RoleArn-less delivery; empty policy denies) |
| ASL subset | `Pass`, `Succeed`, `Fail`, `Task` |
| Task resources | Lambda (sync Invoke), SQS SendMessage, SNS Publish, EventBridge bus ARN (PutEvents) |

Lambda Task states call the same Invoke path as `lambda:InvokeFunction` after the state machine role (or `states.amazonaws.com` resource policy) Allows Invoke. Without nested compute (`DockerHost` empty), Lambda Task fails with `States.TaskFailed` / compute unavailable. SQS/SNS/EventBridge Tasks do not need Docker and resolve foreign SQS queue accounts from the Task ARN. EventBridge can target a state machine ARN with `RoleArn` or (lab) resource policy for StartExecution.

### Authz notes

Identity `EvaluateFull` on `states:*`. When `roleArn` is set on CreateStateMachine, PassRole plus `states.amazonaws.com` trust is required. Task Invoke also requires `lambda:InvokeFunction` (and function policy when present). Lab state-machine resource policy is fail-closed for EventBridge service-principal delivery.

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
- EventBridge→SFN Lambda Tasks without a wired sync invoker
