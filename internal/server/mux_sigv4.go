package server

import (
	"fmt"
	"net/http"
	"strings"

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
		return true
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
	default:
		return false
	}
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
	if r.Header.Get("X-Amz-Target") == "" {
		// Query IAM/STS bind only when the caller signed as iam or sts.
		// Short names such as CreatePolicy / DeletePolicy remap to iam:* but
		// Organizations and Auto Scaling Query still use their own scope.
		got := strings.ToLower(strings.TrimSpace(verified.Service))
		if got != "iam" && got != "sts" {
			return false
		}
	}
	if sigv4ServiceMatchesAction(verified.Service, action) {
		return false
	}
	want := expectedSigV4Service(action)
	msg := fmt.Sprintf("Credential should be scoped to correct service: '%s'", want)
	s.writeAWSError(w, requestID, http.StatusForbidden, authn.CodeSignatureDoesNotMatch, msg,
		readOnly, r, eventID, verified.AccessKeyID, verified.AccountID, true)
	return true
}
