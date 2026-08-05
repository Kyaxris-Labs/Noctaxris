package apigateway_test

import (
	"encoding/json"
	"testing"

	apigwsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/apigateway"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestAPIGatewayRESTJSON(t *testing.T) {
	api := store.RestAPI{APIID: "api1", Name: "lab", RootResourceID: "root"}
	if _, err := apigwsvc.RestApiJSON(api); err != nil {
		t.Fatal(err)
	}
	if _, err := apigwsvc.RestApisJSON([]store.RestAPI{api}); err != nil {
		t.Fatal(err)
	}
	res := store.RestResource{ResourceID: "res1", APIID: api.APIID, ParentID: "root", PathPart: "ping", Path: "/ping"}
	if _, err := apigwsvc.ResourceJSON(res); err != nil {
		t.Fatal(err)
	}
	if _, err := apigwsvc.ResourcesJSON([]store.RestResource{res}); err != nil {
		t.Fatal(err)
	}
	m := store.RestMethod{APIID: api.APIID, ResourceID: res.ResourceID, HTTPMethod: "GET", AuthorizationType: "NONE"}
	if _, err := apigwsvc.MethodJSON(m); err != nil {
		t.Fatal(err)
	}
	in := store.RestIntegration{APIID: api.APIID, ResourceID: res.ResourceID, HTTPMethod: "GET", Type: "MOCK", RequestTemplates: map[string]string{"application/json": "{}"}}
	if _, err := apigwsvc.IntegrationJSON(in); err != nil {
		t.Fatal(err)
	}
	dep := store.RestDeployment{APIID: api.APIID, DeploymentID: "dep1", Description: "lab"}
	if _, err := apigwsvc.DeploymentJSON(dep); err != nil {
		t.Fatal(err)
	}
	st := store.RestStage{APIID: api.APIID, StageName: "prod", DeploymentID: dep.DeploymentID}
	if _, err := apigwsvc.StageJSON(st); err != nil {
		t.Fatal(err)
	}
	auth := store.RestAuthorizer{AuthorizerID: "auth1", APIID: api.APIID, Name: "jwt", Type: "TOKEN", IdentitySource: "method.request.header.Authorization"}
	if _, err := apigwsvc.AuthorizerJSON(auth); err != nil {
		t.Fatal(err)
	}
	if _, err := apigwsvc.AuthorizersJSON([]store.RestAuthorizer{auth}); err != nil {
		t.Fatal(err)
	}
	key := store.RestAPIKey{ID: "key1", Name: "lab-key", Value: "secret", Enabled: true}
	raw, err := apigwsvc.ApiKeyJSON(key)
	if err != nil {
		t.Fatal(err)
	}
	var keyOut map[string]any
	if err := json.Unmarshal(raw, &keyOut); err != nil {
		t.Fatal(err)
	}
	if _, err := apigwsvc.ApiKeysJSON([]store.RestAPIKey{key}); err != nil {
		t.Fatal(err)
	}
	plan := store.RestUsagePlan{ID: "plan1", Name: "lab-plan"}
	if _, err := apigwsvc.UsagePlanJSON(plan); err != nil {
		t.Fatal(err)
	}
	if _, err := apigwsvc.UsagePlansJSON([]store.RestUsagePlan{plan}); err != nil {
		t.Fatal(err)
	}
	planKey := store.RestUsagePlanKey{ID: key.ID, Type: "API_KEY", Value: key.ID}
	if _, err := apigwsvc.UsagePlanKeyJSON(planKey); err != nil {
		t.Fatal(err)
	}
	if _, err := apigwsvc.UsagePlanKeysJSON([]store.RestUsagePlanKey{planKey}); err != nil {
		t.Fatal(err)
	}
}
