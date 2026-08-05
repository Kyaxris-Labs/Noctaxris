package codedeploy_test

import (
	"encoding/json"
	"testing"

	cdsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/codedeploy"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestCodeDeployJSON(t *testing.T) {
	app := store.CodeDeployApplication{ApplicationID: "app-1"}
	appRaw, err := cdsvc.CreateApplicationJSON(app)
	if err != nil {
		t.Fatal(err)
	}
	var appOut map[string]string
	if err := json.Unmarshal(appRaw, &appOut); err != nil {
		t.Fatal(err)
	}
	if appOut["applicationId"] != "app-1" {
		t.Fatalf("app=%v", appOut)
	}

	grp := store.CodeDeployDeploymentGroup{DeploymentGroupID: "dg-1"}
	grpRaw, err := cdsvc.CreateDeploymentGroupJSON(grp)
	if err != nil {
		t.Fatal(err)
	}
	var grpOut map[string]string
	if err := json.Unmarshal(grpRaw, &grpOut); err != nil {
		t.Fatal(err)
	}
	if grpOut["deploymentGroupId"] != "dg-1" {
		t.Fatalf("group=%v", grpOut)
	}

	dep := store.CodeDeployDeployment{
		DeploymentID: "d-1", ApplicationName: "a", DeploymentGroupName: "g",
		Status: "Succeeded", Description: "lab",
	}
	depCreateRaw, err := cdsvc.CreateDeploymentJSON(dep)
	if err != nil {
		t.Fatal(err)
	}
	var depCreate map[string]string
	if err := json.Unmarshal(depCreateRaw, &depCreate); err != nil {
		t.Fatal(err)
	}
	if depCreate["deploymentId"] != "d-1" {
		t.Fatalf("dep create=%v", depCreate)
	}

	getRaw, err := cdsvc.GetDeploymentJSON(dep)
	if err != nil {
		t.Fatal(err)
	}
	var getOut map[string]any
	if err := json.Unmarshal(getRaw, &getOut); err != nil {
		t.Fatal(err)
	}
	info, _ := getOut["deploymentInfo"].(map[string]any)
	if info["status"] != "Succeeded" {
		t.Fatalf("get=%v", getOut)
	}

	listRaw, err := cdsvc.ListDeploymentsJSON([]store.CodeDeployDeployment{dep})
	if err != nil {
		t.Fatal(err)
	}
	var listOut map[string]any
	if err := json.Unmarshal(listRaw, &listOut); err != nil {
		t.Fatal(err)
	}
	ids, _ := listOut["deployments"].([]any)
	if len(ids) != 1 {
		t.Fatalf("list=%v", listOut)
	}
}
