package server

import (
	"net/http"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
)

func TestSigV4ServiceMatchesAction(t *testing.T) {
	t.Parallel()
	if !sigv4ServiceMatchesAction("dynamodb", "dynamodb:GetItem") {
		t.Fatal("dynamodb GetItem")
	}
	if sigv4ServiceMatchesAction("sts", "dynamodb:GetItem") {
		t.Fatal("sts must not run dynamodb")
	}
	if !sigv4ServiceMatchesAction("kms", "kms:Decrypt") {
		t.Fatal("kms Decrypt")
	}
	if !sigv4ServiceMatchesAction("iam", "iam:CreateUser") {
		t.Fatal("iam CreateUser")
	}
	if sigv4ServiceMatchesAction("sts", "iam:CreateUser") {
		t.Fatal("sts must not run iam")
	}
	if !sigv4ServiceMatchesAction("iotdevicegateway", "iot-data:GetThingShadow") {
		t.Fatal("shadow REST signing name")
	}
	if !sigv4ServiceMatchesAction("iot-jobs-data", "iot-jobs-data:GetPendingJobExecutions") {
		t.Fatal("jobs data")
	}
	if !sigv4ServiceMatchesAction("states", "states:StartExecution") {
		t.Fatal("states")
	}
	if !sigv4ServiceMatchesAction("sfn", "states:StartExecution") {
		t.Fatal("sfn alias")
	}
	if !sigv4ServiceMatchesAction("apigateway", "apigatewayv2:CreateApi") {
		t.Fatal("HTTP API management signing name")
	}
	if sigv4ServiceMatchesAction("sts", "apigatewayv2:CreateApi") {
		t.Fatal("sts must not run apigatewayv2")
	}
	if !sigv4ServiceMatchesAction("dynamodb", "dynamodbstreams:ListStreams") {
		t.Fatal("streams signing name")
	}
	if sigv4ServiceMatchesAction("dynamodbstreams", "dynamodb:GetItem") {
		t.Fatal("streams scope must not run GetItem")
	}
	if !sigv4ServiceMatchesAction("tagging", "tag:TagResources") {
		t.Fatal("tagging signing name")
	}
	if !sigv4ServiceMatchesAction("sts", "CreateUser") {
		t.Fatal("short name leaves routing to verified.Service")
	}
	if expectedSigV4Service("dynamodb:GetItem") != "dynamodb" {
		t.Fatalf("expected service %q", expectedSigV4Service("dynamodb:GetItem"))
	}
}

func TestIoTRESTAfterAuthBindsDataPlaneService(t *testing.T) {
	t.Parallel()
	req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:4566/things/lab/shadow", nil)
	if err != nil {
		t.Fatal(err)
	}
	if isIoTRESTAfterAuth(req, &authn.Verified{Service: "s3"}) {
		t.Fatal("s3 must not claim shadow REST")
	}
	if isIoTRESTAfterAuth(req, &authn.Verified{Service: "sts"}) {
		t.Fatal("sts must not claim shadow REST")
	}
	if !isIoTRESTAfterAuth(req, &authn.Verified{Service: "iotdevicegateway"}) {
		t.Fatal("iotdevicegateway shadow REST")
	}
	if !isIoTRESTAfterAuth(req, &authn.Verified{Service: "iotdata"}) {
		t.Fatal("iotdata shadow REST")
	}
	jobs, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:4566/things/lab/jobs", nil)
	if err != nil {
		t.Fatal(err)
	}
	if isIoTRESTAfterAuth(jobs, &authn.Verified{Service: "s3"}) {
		t.Fatal("s3 must not claim jobs REST")
	}
	if !isIoTRESTAfterAuth(jobs, &authn.Verified{Service: "iot-jobs-data"}) {
		t.Fatal("iot-jobs-data jobs REST")
	}
	named, err := http.NewRequest(http.MethodGet,
		"http://127.0.0.1:4566/api/things/shadow/ListNamedShadowsForThing/lab", nil)
	if err != nil {
		t.Fatal(err)
	}
	if isIoTRESTAfterAuth(named, &authn.Verified{Service: "iam"}) {
		t.Fatal("iam must not claim named-shadow REST")
	}
	if !isIoTRESTAfterAuth(named, &authn.Verified{Service: "iot-data"}) {
		t.Fatal("iot-data named-shadow REST")
	}
	ep, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:4566/endpoint", nil)
	if err != nil {
		t.Fatal(err)
	}
	if isIoTRESTAfterAuth(ep, &authn.Verified{Service: "s3"}) {
		t.Fatal("s3 GET /endpoint stays S3")
	}
	if !isIoTRESTAfterAuth(ep, &authn.Verified{Service: "iot"}) {
		t.Fatal("iot GET /endpoint")
	}
}
