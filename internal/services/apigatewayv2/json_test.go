package apigatewayv2_test

import (
	"encoding/json"
	"testing"

	apigwv2svc "github.com/Kyaxris-Labs/Noctaxris/internal/services/apigatewayv2"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestAPIGatewayV2JSON(t *testing.T) {
	api := store.APIGatewayAPI{
		APIID: "api1", Name: "lab-http", ProtocolType: "HTTP",
		APIEndpoint: "https://api1.noctaxris.local", CreatedAt: 1_700_000_000_000,
		CORS: store.APIGatewayCORS{
			AllowOrigins: []string{"*"}, AllowMethods: []string{"GET"}, AllowHeaders: []string{"h"},
			ExposeHeaders: []string{"x"}, MaxAge: 3600, AllowCredentials: true,
		},
	}
	raw, err := apigwv2svc.CreateApiJSON(api)
	if err != nil {
		t.Fatal(err)
	}
	var created map[string]any
	if err := json.Unmarshal(raw, &created); err != nil {
		t.Fatal(err)
	}
	if _, err := apigwv2svc.GetApiJSON(api); err != nil {
		t.Fatal(err)
	}
	if _, err := apigwv2svc.DeleteApiJSON(); err != nil {
		t.Fatal(err)
	}
	if _, err := apigwv2svc.GetApisJSON([]store.APIGatewayAPI{api}); err != nil {
		t.Fatal(err)
	}
	in := store.APIGatewayIntegration{APIID: api.APIID, IntegrationID: "int1", IntegrationType: "AWS_PROXY", IntegrationURI: "arn:aws:lambda:us-east-1:1:function:fn", CredentialsArn: "arn:aws:iam::1:role/lambda"}
	if _, err := apigwv2svc.CreateIntegrationJSON(in); err != nil {
		t.Fatal(err)
	}
	jwtAuth := store.APIGatewayAuthorizer{APIID: api.APIID, AuthorizerID: "auth-jwt", AuthorizerType: store.APIGatewayAuthorizerJWT, Name: "jwt", JWTIssuer: "iss", JWTAudience: []string{"aud"}}
	if _, err := apigwv2svc.CreateAuthorizerJSON(jwtAuth); err != nil {
		t.Fatal(err)
	}
	reqAuth := store.APIGatewayAuthorizer{
		APIID: api.APIID, AuthorizerID: "auth-req", AuthorizerType: store.APIGatewayAuthorizerREQUEST, Name: "req",
		AuthorizerURI: "arn:aws:apigateway:us-east-1:lambda:path/2015-03-31/functions/fn/invocations",
		AuthorizerPayloadFormatVersion: "2.0", EnableSimpleResponses: true, AuthorizerCredentialsArn: "arn:aws:iam::1:role/auth",
	}
	if _, err := apigwv2svc.CreateAuthorizerJSON(reqAuth); err != nil {
		t.Fatal(err)
	}
	auth := jwtAuth
	route := store.APIGatewayRoute{APIID: api.APIID, RouteID: "r1", RouteKey: "GET /ping", Target: "integrations/int1", AuthorizationType: "JWT", AuthorizerID: auth.AuthorizerID}
	if _, err := apigwv2svc.CreateRouteJSON(route); err != nil {
		t.Fatal(err)
	}
	stage := store.APIGatewayStage{APIID: api.APIID, StageName: "$default", AutoDeploy: true}
	if _, err := apigwv2svc.CreateStageJSON(stage); err != nil {
		t.Fatal(err)
	}
	if _, err := apigwv2svc.GetIntegrationsJSON([]store.APIGatewayIntegration{in}); err != nil {
		t.Fatal(err)
	}
	if _, err := apigwv2svc.GetRoutesJSON([]store.APIGatewayRoute{route}); err != nil {
		t.Fatal(err)
	}
	if _, err := apigwv2svc.GetAuthorizersJSON([]store.APIGatewayAuthorizer{jwtAuth, reqAuth}); err != nil {
		t.Fatal(err)
	}
}
