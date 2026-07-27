package sdk_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
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

func TestWebSocketAPICreateRoutesAndSoftSkipConnect(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	iamClient := newIAM(t, cfg)
	lam := newLambda(t, cfg)
	apigw := newAPIGWv2(t, cfg)
	ctx := context.Background()
	prefix := uniquePrefix(t)
	httpClient := &http.Client{Timeout: 30 * time.Second}

	roleName := prefix + "-ws-role"
	trust := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	roleOut, err := iamClient.CreateRole(ctx, &iam.CreateRoleInput{
		RoleName:                 aws.String(roleName),
		AssumeRolePolicyDocument: aws.String(trust),
	})
	if err != nil {
		t.Fatalf("CreateRole: %v", err)
	}
	roleARN := *roleOut.Role.Arn
	t.Cleanup(func() {
		_, _ = iamClient.DeleteRole(context.Background(), &iam.DeleteRoleInput{RoleName: aws.String(roleName)})
	})

	fnName := prefix + "-ws-fn"
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
	lambdaARN := *fnOut.FunctionArn
	t.Cleanup(func() {
		_, _ = lam.DeleteFunction(context.Background(), &lambda.DeleteFunctionInput{FunctionName: aws.String(fnName)})
	})
	_, err = lam.AddPermission(ctx, &lambda.AddPermissionInput{
		FunctionName: aws.String(fnName),
		StatementId:  aws.String(prefix + "-ws-perm"),
		Action:       aws.String("lambda:InvokeFunction"),
		Principal:    aws.String("apigateway.amazonaws.com"),
	})
	if err != nil {
		t.Fatalf("AddPermission: %v", err)
	}

	apiOut, err := apigw.CreateApi(ctx, &apigatewayv2.CreateApiInput{
		Name:         aws.String(prefix + "-ws-api"),
		ProtocolType: apitypes.ProtocolTypeWebsocket,
	})
	if err != nil {
		t.Fatalf("CreateApi WEBSOCKET: %v", err)
	}
	apiID := *apiOut.ApiId
	t.Cleanup(func() {
		_, _ = apigw.DeleteApi(context.Background(), &apigatewayv2.DeleteApiInput{ApiId: aws.String(apiID)})
	})
	if apiOut.ProtocolType != apitypes.ProtocolTypeWebsocket {
		t.Fatalf("ProtocolType=%v", apiOut.ProtocolType)
	}

	intOut, err := apigw.CreateIntegration(ctx, &apigatewayv2.CreateIntegrationInput{
		ApiId:           aws.String(apiID),
		IntegrationType: apitypes.IntegrationTypeAwsProxy,
		IntegrationUri:  aws.String(lambdaARN),
	})
	if err != nil {
		t.Fatalf("CreateIntegration: %v", err)
	}
	integrationID := *intOut.IntegrationId
	for _, rk := range []string{"$connect", "$disconnect", "$default"} {
		_, err := apigw.CreateRoute(ctx, &apigatewayv2.CreateRouteInput{
			ApiId:             aws.String(apiID),
			RouteKey:          aws.String(rk),
			Target:            aws.String("integrations/" + integrationID),
			AuthorizationType: apitypes.AuthorizationTypeNone,
		})
		if err != nil {
			t.Fatalf("CreateRoute %s: %v", rk, err)
		}
	}
	_, err = apigw.CreateStage(ctx, &apigatewayv2.CreateStageInput{
		ApiId:      aws.String(apiID),
		StageName:  aws.String("$default"),
		AutoDeploy: aws.Bool(true),
	})
	if err != nil {
		t.Fatalf("CreateStage: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, endpoint()+"/ws-api/"+apiID+"/$default/$connect", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("lab $connect: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusServiceUnavailable && strings.Contains(string(body), "compute unavailable") {
		t.Skip("WebSocket $connect soft-skip: nested Lambda unavailable (set NOCTAXRIS_NESTED=1)")
	}
	if os.Getenv("NOCTAXRIS_NESTED") != "1" && resp.StatusCode == http.StatusServiceUnavailable {
		t.Skip("WebSocket $connect soft-skip: compute unavailable without nested engine")
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("$connect status=%d body=%s", resp.StatusCode, body)
	}
	var connectResp map[string]any
	if err := json.Unmarshal(body, &connectResp); err != nil {
		t.Fatalf("connect json: %v body=%s", err, body)
	}
	if connectResp["connectionId"] == nil || connectResp["connectionId"] == "" {
		t.Fatalf("missing connectionId: %v", connectResp)
	}
}
