package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
)

func catalogServiceFromAction(action string) string {
	i := strings.Index(action, ":")
	if i <= 0 {
		return ""
	}
	return strings.ToLower(action[:i])
}

func expectedSigV4Service(action string) string {
	svc := catalogServiceFromAction(action)
	switch svc {
	case "iot-data":
		return "iotdata"
	case "iot-jobs-data":
		return "iot-jobs-data"
	case "states":
		return "states"
	case "wafv2":
		return "wafv2"
	case "cloudwatch":
		return "monitoring"
	case "apigatewayv2":
		return "apigateway"
	case "dynamodbstreams":
		return "dynamodb"
	case "tag":
		return "tagging"
	default:
		if svc == "" {
			return "unknown"
		}
		return svc
	}
}

func sigv4ServiceMatchesAction(verifiedService, action string) bool {
	want := catalogServiceFromAction(action)
	if want == "" {
		// Short names are not auto-allowed; Query/JSON bind via queryShortNameMatchesService.
		return false
	}
	got := strings.ToLower(strings.TrimSpace(verifiedService))
	if got == "" {
		return false
	}
	if got == want {
		return true
	}
	switch want {
	case "iot-data":
		return got == "iotdata" || got == "data.iot" || got == "iotdevicegateway"
	case "iot-jobs-data":
		return got == "iotjobsdata"
	case "iot":
		return strings.HasPrefix(got, "iot")
	case "states":
		return got == "sfn"
	case "wafv2":
		return got == "waf"
	case "cloudwatch":
		return got == "monitoring"
	case "es":
		return got == "opensearch"
	case "ses":
		return got == "email"
	case "kafka":
		return got == "msk"
	case "macie2":
		return got == "macie"
	case "appconfig":
		return got == "appconfigdata"
	case "appconfigdata":
		return got == "appconfig"
	case "bedrock":
		return got == "bedrock-runtime"
	case "bedrock-runtime":
		return got == "bedrock"
	case "cloudcontrol":
		return got == "cloudcontrolapi"
	case "cloudcontrolapi":
		return got == "cloudcontrol"
	case "noctaxris-lab":
		return got == "noctaxris" || got == "noctaxris-lab"
	case "apigatewayv2":
		return got == "apigateway"
	case "dynamodbstreams":
		return got == "dynamodb"
	case "tag":
		return got == "tagging"
	case "rds":
		// DocDB control-plane actions are catalogued as rds:* and may be signed as docdb.
		return got == "docdb"
	default:
		return false
	}
}

// bindOwnedShortAction keeps a service-prefixed mapping, or stamps svc:short when
// the service mapper left an unimplemented short name unbound.
func bindOwnedShortAction(svc, shortAction, mapped string) string {
	if strings.Contains(mapped, ":") {
		return mapped
	}
	if strings.TrimSpace(shortAction) == "" {
		return mapped
	}
	return svc + ":" + shortAction
}

// mapQueryShortAction remaps a Query/JSON short Action using the SigV4 service.
// Returns a service-prefixed catalog action when the service owns the short name.
func mapQueryShortAction(verifiedService, shortAction string) string {
	svc := strings.ToLower(strings.TrimSpace(verifiedService))
	switch svc {
	case "elasticache":
		return bindOwnedShortAction("elasticache", shortAction, elasticacheAction(shortAction))
	case "memorydb":
		return bindOwnedShortAction("memorydb", shortAction, memorydbAction(shortAction))
	case "neptune":
		return bindOwnedShortAction("neptune", shortAction, neptuneAction(shortAction))
	case "docdb":
		return bindOwnedShortAction("docdb", shortAction, docdbAction(shortAction))
	case "rds":
		if mapped := rdsAction(shortAction); strings.Contains(mapped, ":") {
			return mapped
		}
		// DocDB Query is often signed as rds with CreateDBCluster/Describe/Delete.
		return bindOwnedShortAction("rds", shortAction, docdbAction(shortAction))
	case "rds-data":
		return bindOwnedShortAction("rds-data", shortAction, rdsDataAction(shortAction))
	case "mq":
		return bindOwnedShortAction("mq", shortAction, mqAction(shortAction))
	case "ec2":
		return bindOwnedShortAction("ec2", shortAction, ec2Action(shortAction))
	case "transfer":
		return bindOwnedShortAction("transfer", shortAction, transferAction(shortAction))
	case "acm":
		return bindOwnedShortAction("acm", shortAction, acmAction(shortAction))
	case "guardduty":
		return bindOwnedShortAction("guardduty", shortAction, guarddutyAction(shortAction))
	case "securityhub":
		return bindOwnedShortAction("securityhub", shortAction, securityHubAction(shortAction))
	case "macie", "macie2":
		return bindOwnedShortAction("macie2", shortAction, macieAction(shortAction))
	case "detective":
		return bindOwnedShortAction("detective", shortAction, detectiveAction(shortAction))
	case "autoscaling":
		return bindOwnedShortAction("autoscaling", shortAction, asgAction(shortAction))
	case "elasticbeanstalk":
		return bindOwnedShortAction("elasticbeanstalk", shortAction, beanstalkAction(shortAction))
	case "lightsail":
		return bindOwnedShortAction("lightsail", shortAction, lightsailAction(shortAction))
	case "backup":
		return bindOwnedShortAction("backup", shortAction, backupAction(shortAction))
	case "eks":
		return bindOwnedShortAction("eks", shortAction, eksAction(shortAction))
	case "appconfig", "appconfigdata":
		return bindOwnedShortAction("appconfig", shortAction, appconfigAction(shortAction))
	case "athena":
		return bindOwnedShortAction("athena", shortAction, athenaAction(shortAction))
	case "es", "opensearch":
		return bindOwnedShortAction("es", shortAction, opensearchAction(shortAction))
	case "kafka", "msk":
		return bindOwnedShortAction("kafka", shortAction, mskAction(shortAction))
	case "pipes":
		return bindOwnedShortAction("pipes", shortAction, pipesAction(shortAction))
	case "sns":
		return bindOwnedShortAction("sns", shortAction, snsAction(shortAction))
	case "ses", "email":
		return bindOwnedShortAction("ses", shortAction, sesAction(shortAction))
	case "iam", "sts", "organizations":
		return normalizeAction(shortAction)
	case "monitoring", "cloudwatch":
		return bindOwnedShortAction("cloudwatch", shortAction, cloudWatchAction(shortAction))
	case "logs":
		return bindOwnedShortAction("logs", shortAction, logsAction(shortAction))
	case "kms":
		if mapped := normalizeAction(shortAction); strings.Contains(mapped, ":") {
			return mapped
		}
		if shortAction == "ReEncrypt" {
			return "kms:ReEncrypt"
		}
		return "kms:" + shortAction
	case "cloudformation":
		return bindOwnedShortAction("cloudformation", shortAction, cfnAction(shortAction))
	case "config":
		return bindOwnedShortAction("config", shortAction, configAction(shortAction))
	case "elasticloadbalancing":
		return bindOwnedShortAction("elasticloadbalancing", shortAction, elbv2Action(shortAction))
	case "cloudfront":
		return bindOwnedShortAction("cloudfront", shortAction, cloudfrontAction(shortAction))
	case "cloudcontrol", "cloudcontrolapi":
		return bindOwnedShortAction("cloudcontrol", shortAction, cloudControlAction(shortAction))
	case "budgets":
		return bindOwnedShortAction("budgets", shortAction, budgetsAction(shortAction))
	case "bcm-data-exports", "bcm":
		return bindOwnedShortAction("bcm-data-exports", shortAction, bcmExportAction(shortAction))
	case "firehose":
		return bindOwnedShortAction("firehose", shortAction, firehoseAction(shortAction))
	case "kinesis":
		return bindOwnedShortAction("kinesis", shortAction, kinesisAction(shortAction))
	case "route53":
		return bindOwnedShortAction("route53", shortAction, route53Action(shortAction))
	case "s3vectors":
		return bindOwnedShortAction("s3vectors", shortAction, s3vectorsAction(shortAction))
	case "codedeploy":
		return bindOwnedShortAction("codedeploy", shortAction, codeDeployAction(shortAction))
	case "lambda":
		return bindOwnedShortAction("lambda", shortAction, lambdaAction(shortAction))
	case "s3":
		// S3 control/data plane is mostly path-style. Only a few Query Action=
		// names are claimed so s3-scoped CreateCacheCluster stays fail-closed.
		switch shortAction {
		case "ListBuckets", "CreateBucket", "DeleteBucket", "HeadBucket",
			"PutObject", "GetObject", "DeleteObject", "CopyObject",
			"ListObjects", "ListObjectsV2", "HeadObject":
			return "s3:" + shortAction
		default:
			return shortAction
		}
	case "sqs", "dynamodb", "events", "ssm", "secretsmanager",
		"ecs", "ecr", "sfn", "states",
		"codebuild", "codepipeline", "scheduler", "glue", "waf", "wafv2",
		"iot", "emr", "batch", "bedrock", "bedrock-runtime",
		"textract", "transcribe", "pricing", "ce", "cur",
		"appsync", "apigateway", "apigatewayv2", "cognito-idp",
		"tagging", "cloudtrail":
		if mapped := normalizeAction(shortAction); strings.Contains(mapped, ":") {
			return mapped
		}
		return svc + ":" + shortAction
	default:
		return shortAction
	}
}

// queryShortNameMatchesService reports whether a short Action is owned by
// the SigV4 credential service (fail-closed when the mapper cannot bind it).
func queryShortNameMatchesService(verifiedService, shortAction string) bool {
	if strings.Contains(shortAction, ":") || strings.TrimSpace(shortAction) == "" {
		return false
	}
	mapped := mapQueryShortAction(verifiedService, shortAction)
	if !strings.Contains(mapped, ":") {
		return false
	}
	return sigv4ServiceMatchesAction(verifiedService, mapped)
}

func (s *Server) rejectSigV4ServiceMismatch(
	w http.ResponseWriter, r *http.Request, requestID, eventID, action string,
	verified *authn.Verified, readOnly bool,
) bool {
	if verified == nil || action == "" {
		return false
	}
	if isUnauthenticatedSTSAction(action) || isUnauthenticatedCognitoAction(action) {
		return false
	}
	if sigv4ServiceMatchesAction(verified.Service, action) {
		return false
	}
	// Short names (Query Action= or unresolved JSON X-Amz-Target): require the
	// SigV4 service to own the Action via its mapper.
	if !strings.Contains(action, ":") {
		if queryShortNameMatchesService(verified.Service, action) {
			return false
		}
	}
	// Shared short names remapped by normalizeAction (Organizations/SNS/ASG/IoT)
	// still route by verified.Service.
	if querySharedShortNameAllows(verified.Service, action) {
		return false
	}
	want := expectedSigV4Service(action)
	msg := fmt.Sprintf("Credential should be scoped to correct service: '%s'", want)
	s.writeAWSError(w, requestID, http.StatusForbidden, authn.CodeSignatureDoesNotMatch, msg,
		readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
	return true
}

func querySharedShortNameAllows(verifiedService, action string) bool {
	got := strings.ToLower(strings.TrimSpace(verifiedService))
	switch got {
	case "organizations":
		switch action {
		case catalog.ActionIAMCreatePolicy, "CreatePolicy",
			catalog.ActionIAMListPolicies, "ListPolicies":
			return true
		}
	case "sns":
		// normalizeAction maps these shared short names to events:/lambda:/kms:*.
		// SNS Query still signs as sns and remaps inside handleSNS.
		switch action {
		case catalog.ActionEventsRemovePermission, catalog.ActionLambdaRemovePermission,
			catalog.ActionSNSRemovePermission, "RemovePermission",
			catalog.ActionKMSListResourceTags, catalog.ActionKMSTagResource, catalog.ActionKMSUntagResource,
			"ListTagsForResource", "TagResource", "UntagResource",
			catalog.ActionLambdaAddPermission, "AddPermission":
			return true
		}
	case "autoscaling":
		// DeletePolicy remaps to iam:DeletePolicy; ASG Query remaps in asgAction/handleASG.
		switch action {
		case catalog.ActionIAMDeletePolicy, catalog.ActionASGDeletePolicy, "DeletePolicy":
			return true
		}
	case "iot":
		switch action {
		case catalog.ActionIAMDeletePolicy, catalog.ActionIoTDeletePolicy, "DeletePolicy":
			return true
		}
	}
	return false
}
