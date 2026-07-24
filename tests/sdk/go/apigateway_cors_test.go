package sdk_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/apigatewayv2"
	apitypes "github.com/aws/aws-sdk-go-v2/service/apigatewayv2/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/lambda/types"
)

func newAPIGWv2(t *testing.T, cfg aws.Config) *apigatewayv2.Client {
	t.Helper()
	return apigatewayv2.NewFromConfig(cfg, func(o *apigatewayv2.Options) {
		o.BaseEndpoint = aws.String(endpoint())
	})
}

func TestHTTPAPICORSPreflightAndUpdate(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	iamClient := newIAM(t, cfg)
	lam := newLambda(t, cfg)
	apigw := newAPIGWv2(t, cfg)
	ctx := context.Background()
	prefix := uniquePrefix(t)
	httpClient := &http.Client{Timeout: 30 * time.Second}

	roleName := prefix + "-cors-role"
	trust := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	roleOut, err := iamClient.CreateRole(ctx, &iam.CreateRoleInput{
		RoleName:                 aws.String(roleName),
		AssumeRolePolicyDocument: aws.String(trust),
	})
	if err != nil {
		t.Fatalf("CreateRole: %v", err)
	}
	if roleOut.Role == nil || roleOut.Role.Arn == nil {
		t.Fatal("CreateRole missing Arn")
	}
	roleARN := *roleOut.Role.Arn
	t.Cleanup(func() {
		_, _ = iamClient.DeleteRole(context.Background(), &iam.DeleteRoleInput{RoleName: aws.String(roleName)})
	})

	fnName := prefix + "-cors-fn"
	fnOut, err := lam.CreateFunction(ctx, &lambda.CreateFunctionInput{
		FunctionName: aws.String(fnName),
		Runtime:      types.RuntimePython312,
		Role:         aws.String(roleARN),
		Handler:      aws.String("index.handler"),
		Code:         &types.FunctionCode{ZipFile: minimalPythonZip(t)},
	})
	if err != nil {
		t.Fatalf("CreateFunction: %v", err)
	}
	if fnOut.FunctionArn == nil || *fnOut.FunctionArn == "" {
		t.Fatal("CreateFunction missing FunctionArn")
	}
	lambdaARN := *fnOut.FunctionArn
	t.Cleanup(func() {
		_, _ = lam.DeleteFunction(context.Background(), &lambda.DeleteFunctionInput{FunctionName: aws.String(fnName)})
	})

	apiOut, err := apigw.CreateApi(ctx, &apigatewayv2.CreateApiInput{
		Name:         aws.String(prefix + "-cors-api"),
		ProtocolType: apitypes.ProtocolTypeHttp,
		CorsConfiguration: &apitypes.Cors{
			AllowOrigins: []string{"https://lab.example"},
			AllowMethods: []string{"GET", "OPTIONS"},
			AllowHeaders: []string{"authorization", "content-type"},
			MaxAge:       aws.Int32(600),
		},
	})
	if err != nil {
		t.Fatalf("CreateApi: %v", err)
	}
	if apiOut.ApiId == nil || *apiOut.ApiId == "" {
		t.Fatal("CreateApi missing ApiId")
	}
	if apiOut.CorsConfiguration == nil {
		t.Fatal("CreateApi missing CorsConfiguration")
	}
	apiID := *apiOut.ApiId
	t.Cleanup(func() {
		_, _ = apigw.DeleteApi(context.Background(), &apigatewayv2.DeleteApiInput{ApiId: aws.String(apiID)})
	})

	intOut, err := apigw.CreateIntegration(ctx, &apigatewayv2.CreateIntegrationInput{
		ApiId:           aws.String(apiID),
		IntegrationType: apitypes.IntegrationTypeAwsProxy,
		IntegrationUri:  aws.String(lambdaARN),
	})
	if err != nil {
		t.Fatalf("CreateIntegration: %v", err)
	}
	if intOut.IntegrationId == nil || *intOut.IntegrationId == "" {
		t.Fatal("CreateIntegration missing IntegrationId")
	}
	integrationID := *intOut.IntegrationId

	_, err = apigw.CreateRoute(ctx, &apigatewayv2.CreateRouteInput{
		ApiId:             aws.String(apiID),
		RouteKey:          aws.String("GET /hello"),
		Target:            aws.String("integrations/" + integrationID),
		AuthorizationType: apitypes.AuthorizationTypeNone,
	})
	if err != nil {
		t.Fatalf("CreateRoute: %v", err)
	}

	_, err = apigw.CreateStage(ctx, &apigatewayv2.CreateStageInput{
		ApiId:     aws.String(apiID),
		StageName: aws.String("$default"),
	})
	if err != nil {
		t.Fatalf("CreateStage: %v", err)
	}

	helloURL := fmt.Sprintf("%s/http-api/%s/$default/hello", endpoint(), apiID)

	optReq, err := http.NewRequest(http.MethodOptions, helloURL, nil)
	if err != nil {
		t.Fatalf("OPTIONS NewRequest: %v", err)
	}
	optReq.Header.Set("Origin", "https://lab.example")
	optReq.Header.Set("Access-Control-Request-Method", "GET")
	optReq.Header.Set("Access-Control-Request-Headers", "authorization")
	optResp, err := httpClient.Do(optReq)
	if err != nil {
		t.Fatalf("OPTIONS preflight: %v", err)
	}
	body, _ := io.ReadAll(optResp.Body)
	_ = optResp.Body.Close()
	if optResp.StatusCode != http.StatusNoContent {
		t.Fatalf("preflight status=%d body=%q", optResp.StatusCode, body)
	}
	if got := optResp.Header.Get("Access-Control-Allow-Origin"); got != "https://lab.example" {
		t.Fatalf("ACAO=%q", got)
	}
	if got := optResp.Header.Get("Access-Control-Allow-Methods"); !strings.Contains(got, "GET") {
		t.Fatalf("Allow-Methods=%q", got)
	}
	if got := optResp.Header.Get("Access-Control-Max-Age"); got != "600" {
		t.Fatalf("Max-Age=%q", got)
	}

	denyReq, err := http.NewRequest(http.MethodOptions, helloURL, nil)
	if err != nil {
		t.Fatalf("deny OPTIONS NewRequest: %v", err)
	}
	denyReq.Header.Set("Origin", "https://evil.example")
	denyReq.Header.Set("Access-Control-Request-Method", "GET")
	denyResp, err := httpClient.Do(denyReq)
	if err != nil {
		t.Fatalf("foreign origin OPTIONS: %v", err)
	}
	_, _ = io.Copy(io.Discard, denyResp.Body)
	_ = denyResp.Body.Close()
	if denyResp.StatusCode != http.StatusForbidden {
		t.Fatalf("foreign origin preflight status=%d want 403", denyResp.StatusCode)
	}

	_, err = apigw.UpdateApi(ctx, &apigatewayv2.UpdateApiInput{
		ApiId: aws.String(apiID),
		CorsConfiguration: &apitypes.Cors{
			AllowOrigins: []string{"https://other.example"},
			AllowMethods: []string{"GET", "POST", "OPTIONS"},
		},
	})
	if err != nil {
		t.Fatalf("UpdateApi: %v", err)
	}

	opt2Req, err := http.NewRequest(http.MethodOptions, helloURL, nil)
	if err != nil {
		t.Fatalf("updated OPTIONS NewRequest: %v", err)
	}
	opt2Req.Header.Set("Origin", "https://other.example")
	opt2Req.Header.Set("Access-Control-Request-Method", "POST")
	opt2Resp, err := httpClient.Do(opt2Req)
	if err != nil {
		t.Fatalf("updated OPTIONS: %v", err)
	}
	body2, _ := io.ReadAll(opt2Resp.Body)
	_ = opt2Resp.Body.Close()
	if opt2Resp.StatusCode != http.StatusNoContent {
		t.Fatalf("updated preflight status=%d body=%q", opt2Resp.StatusCode, body2)
	}
	if got := opt2Resp.Header.Get("Access-Control-Allow-Origin"); got != "https://other.example" {
		t.Fatalf("updated ACAO=%q", got)
	}
}
