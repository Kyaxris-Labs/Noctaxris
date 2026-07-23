package authz

import "strings"

// ServicePrincipalLambda is the AWS Lambda service principal used in role trust.
const ServicePrincipalLambda = "lambda.amazonaws.com"

// ServicePrincipalEvents is the Amazon EventBridge service principal used in role trust.
const ServicePrincipalEvents = "events.amazonaws.com"

// ServicePrincipalScheduler is the Amazon EventBridge Scheduler service principal used in role trust.
const ServicePrincipalScheduler = "scheduler.amazonaws.com"

// ServicePrincipalECSTasks is the Amazon ECS tasks service principal used in role trust.
const ServicePrincipalECSTasks = "ecs-tasks.amazonaws.com"

// ServicePrincipalStates is the AWS Step Functions service principal used in role trust.
const ServicePrincipalStates = "states.amazonaws.com"

// ServicePrincipalCloudFormation is the CloudFormation service principal used in role trust.
const ServicePrincipalCloudFormation = "cloudformation.amazonaws.com"

// ServicePrincipalCodePipeline is the CodePipeline service principal used in role trust.
const ServicePrincipalCodePipeline = "codepipeline.amazonaws.com"

// ServicePrincipalFirehose is the Kinesis Data Firehose service principal used in role trust.
const ServicePrincipalFirehose = "firehose.amazonaws.com"

// ServicePrincipalPipes is the EventBridge Pipes service principal used in role trust.
const ServicePrincipalPipes = "pipes.amazonaws.com"

// ServicePrincipalSNS is the Amazon SNS service principal used for subscription delivery
// to SQS queues and Lambda functions (resource-policy checks).
const ServicePrincipalSNS = "sns.amazonaws.com"

// ServicePrincipalConfig is the AWS Config service principal used in role trust.
const ServicePrincipalConfig = "config.amazonaws.com"

// ServicePrincipalCodeBuild is the AWS CodeBuild service principal used in role trust.
const ServicePrincipalCodeBuild = "codebuild.amazonaws.com"

// ServicePrincipalBatch is the AWS Batch service principal used in role trust.
const ServicePrincipalBatch = "batch.amazonaws.com"

// ServicePrincipalCodeDeploy is the AWS CodeDeploy service principal used in role trust.
const ServicePrincipalCodeDeploy = "codedeploy.amazonaws.com"

// ServicePrincipalCloudControl is the Cloud Control API service principal used in role trust.
const ServicePrincipalCloudControl = "cloudformation.amazonaws.com"

// ServicePrincipalAPIGateway is the API Gateway service principal used in role trust
// for HTTP API integrations, CredentialsArn PassRole, and Lambda resource-policy invoke.
const ServicePrincipalAPIGateway = "apigateway.amazonaws.com"

// ServicePrincipalAppSync is the AppSync service principal used for Lambda
// resource-policy invoke from GraphQL data sources.
const ServicePrincipalAppSync = "appsync.amazonaws.com"

// ServicePrincipalCognitoIDP is the Cognito User Pools service principal used in
// role trust when Cognito configure APIs take RoleArn.
const ServicePrincipalCognitoIDP = "cognito-idp.amazonaws.com"

// ServicePrincipalLogs is the CloudWatch Logs service principal used in role trust
// for subscription filter roleArn (stream destinations / lab SQS).
const ServicePrincipalLogs = "logs.amazonaws.com"

// ServicePrincipalTransfer is the AWS Transfer Family service principal used in
// role trust for CreateUser Role.
const ServicePrincipalTransfer = "transfer.amazonaws.com"

// ServicePrincipalELB is the Elastic Load Balancing service principal used for
// Lambda target resource-policy invoke (ALB RegisterTargets).
const ServicePrincipalELB = "elasticloadbalancing.amazonaws.com"

// ServicePrincipalRDS is reserved for future RDS configure APIs that accept RoleArn
// (for example MonitoringRoleArn). Lab create paths do not expose RoleArn yet.
const ServicePrincipalRDS = "rds.amazonaws.com"

const actionPassRole = "iam:PassRole"
const actionAssumeRole = "sts:AssumeRole"

// PassRoleRequest is input for configure-time PassRole checks.
type PassRoleRequest struct {
	Caller           RequestContext // Action is set by CheckPassRole to iam:PassRole
	IdentityDocs     []string       // used when EvalInputs.IdentityDocs is nil
	EvalInputs       EvalInputs     // boundary, session, SCP/RCP; IdentityDocs wins when non-nil
	RoleARN          string
	TrustPolicyDoc   string
	ServicePrincipal string // e.g. ServicePrincipalLambda
	SourceArn        string // optional configure-time aws:SourceArn for trust Conditions
}

// CheckPassRole applies configure-time PassRole dual evaluation:
//  1. Caller must Allow iam:PassRole on RoleARN via EvaluateFull (SCP, RCP, boundary, session)
//  2. Trust policy must Allow sts:AssumeRole for Principal Service = ServicePrincipal
//
// Both sides must Allow; otherwise Deny.
func CheckPassRole(req PassRoleRequest) Decision {
	ctx := req.Caller
	ctx.Action = actionPassRole
	ctx.Resource = req.RoleARN
	if ctx.ConditionKeys == nil {
		ctx.ConditionKeys = map[string]string{}
	} else {
		// Copy so callers' maps are not mutated.
		copied := make(map[string]string, len(ctx.ConditionKeys)+1)
		for k, v := range ctx.ConditionKeys {
			copied[k] = v
		}
		ctx.ConditionKeys = copied
	}
	if req.ServicePrincipal != "" {
		ctx.ConditionKeys["iam:PassedToService"] = req.ServicePrincipal
	}

	in := req.EvalInputs
	if in.IdentityDocs == nil {
		in.IdentityDocs = req.IdentityDocs
	}
	if EvaluateFull(ctx, in) != Allow {
		return Deny
	}
	trustKeys := passRoleTrustConditionKeys(req)
	if !TrustAllowsServiceWithKeys(req.TrustPolicyDoc, req.ServicePrincipal, trustKeys) {
		return Deny
	}
	return Allow
}

// passRoleTrustConditionKeys builds trust-evaluation keys for configure-time
// PassRole (aws:SourceAccount from caller; aws:SourceArn when provided).
func passRoleTrustConditionKeys(req PassRoleRequest) map[string]string {
	keys := map[string]string{}
	if req.Caller.ConditionKeys != nil {
		for k, v := range req.Caller.ConditionKeys {
			keys[k] = v
		}
	}
	if accountID := strings.TrimSpace(req.Caller.Principal.AccountID); accountID != "" {
		keys["aws:SourceAccount"] = accountID
	}
	if src := strings.TrimSpace(req.SourceArn); src != "" {
		keys["aws:SourceArn"] = src
	}
	return keys
}

// TrustAllowsService reports whether trustDoc explicitly Allows sts:AssumeRole
// for the given service principal (Principal.Service). Explicit Deny for that
// service yields false. Principal "*" matches any service. Trust Conditions are
// evaluated with empty keys (positive operators fail closed when unpopulated).
func TrustAllowsService(trustDoc, servicePrincipal string) bool {
	return TrustAllowsServiceWithKeys(trustDoc, servicePrincipal, nil)
}

// TrustAllowsServiceWithKeys is TrustAllowsService with request condition keys
// for trust Condition evaluation (e.g. aws:SourceAccount, aws:SourceArn).
func TrustAllowsServiceWithKeys(trustDoc, servicePrincipal string, keys map[string]string) bool {
	if trustDoc == "" || servicePrincipal == "" {
		return false
	}
	doc, err := parsePolicyDocument(trustDoc)
	if err != nil {
		return false
	}
	if keys == nil {
		keys = map[string]string{}
	}
	var denyHit, allowHit bool
	for _, st := range doc.Statement {
		if !trustServiceStatementMatches(st, servicePrincipal, keys) {
			continue
		}
		switch {
		case strings.EqualFold(st.Effect, "Deny"):
			denyHit = true
		case strings.EqualFold(st.Effect, "Allow"):
			allowHit = true
		}
	}
	if denyHit {
		return false
	}
	return allowHit
}

func trustServiceStatementMatches(st statement, servicePrincipal string, keys map[string]string) bool {
	effect := strings.EqualFold(st.Effect, "Allow") || strings.EqualFold(st.Effect, "Deny")
	if !effect {
		return false
	}
	if st.Principal == nil || !principalServiceMatches(*st.Principal, servicePrincipal) {
		return false
	}
	if !actionsMatch(st.Action, actionAssumeRole) {
		return false
	}
	if len(st.Resource) > 0 && !resourcesMatch(st.Resource, "*") {
		return false
	}
	if conditionCatalogUnknown(st.Condition) {
		return false
	}
	switch conditionApplies(st.Condition, keys) {
	case condMatch:
		return true
	default:
		return false
	}
}

func principalServiceMatches(spec principalSpec, servicePrincipal string) bool {
	if spec.All {
		return true
	}
	for _, s := range spec.Service {
		if s == "*" || s == servicePrincipal {
			return true
		}
	}
	return false
}
