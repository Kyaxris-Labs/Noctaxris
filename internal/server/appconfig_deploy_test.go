package server_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func setupAppConfigDeployLab(t *testing.T, handler http.Handler, now time.Time) (appID, envID, profileID string, version int) {
	t.Helper()
	createApp := mustAppConfigJSON(t, handler, "AmazonAppConfig.CreateApplication", "appconfig", map[string]any{
		"Name": "deploy-lab",
	}, now)
	if createApp.Code != http.StatusOK {
		t.Fatalf("CreateApplication status=%d body=%q", createApp.Code, createApp.Body.String())
	}
	var appOut map[string]any
	_ = json.Unmarshal(createApp.Body.Bytes(), &appOut)
	appID, _ = appOut["Id"].(string)

	createEnv := mustAppConfigJSON(t, handler, "AmazonAppConfig.CreateEnvironment", "appconfig", map[string]any{
		"ApplicationId": appID,
		"Name":          "dev",
	}, now)
	if createEnv.Code != http.StatusOK {
		t.Fatalf("CreateEnvironment status=%d body=%q", createEnv.Code, createEnv.Body.String())
	}
	var envOut map[string]any
	_ = json.Unmarshal(createEnv.Body.Bytes(), &envOut)
	envID, _ = envOut["Id"].(string)

	createProfile := mustAppConfigJSON(t, handler, "AmazonAppConfig.CreateConfigurationProfile", "appconfig", map[string]any{
		"ApplicationId": appID,
		"Name":          "flags",
		"LocationUri":   "hosted",
	}, now)
	if createProfile.Code != http.StatusOK {
		t.Fatalf("CreateConfigurationProfile status=%d body=%q", createProfile.Code, createProfile.Body.String())
	}
	var profileOut map[string]any
	_ = json.Unmarshal(createProfile.Body.Bytes(), &profileOut)
	profileID, _ = profileOut["Id"].(string)

	hosted := mustAppConfigJSON(t, handler, "AmazonAppConfig.CreateHostedConfigurationVersion", "appconfig", map[string]any{
		"ApplicationId":          appID,
		"ConfigurationProfileId": profileID,
		"ContentType":            "application/json",
		"Content":                base64.StdEncoding.EncodeToString([]byte(`{"flag":true}`)),
	}, now)
	if hosted.Code != http.StatusOK {
		t.Fatalf("CreateHostedConfigurationVersion status=%d body=%q", hosted.Code, hosted.Body.String())
	}
	var hostedOut map[string]any
	_ = json.Unmarshal(hosted.Body.Bytes(), &hostedOut)
	version = int(hostedOut["VersionNumber"].(float64))
	return appID, envID, profileID, version
}

func TestAppConfigStartGetListDeployment(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	appID, envID, profileID, version := setupAppConfigDeployLab(t, handler, now)

	emptyList := mustAppConfigJSON(t, handler, "AmazonAppConfig.ListDeployments", "appconfig", map[string]any{
		"ApplicationId": appID,
	}, now)
	if emptyList.Code != http.StatusOK {
		t.Fatalf("ListDeployments empty status=%d body=%q", emptyList.Code, emptyList.Body.String())
	}
	if !strings.Contains(emptyList.Body.String(), `"Items"`) {
		t.Fatalf("ListDeployments empty want Items body=%q", emptyList.Body.String())
	}

	start := mustAppConfigJSON(t, handler, "AmazonAppConfig.StartDeployment", "appconfig", map[string]any{
		"ApplicationId":          appID,
		"EnvironmentId":          envID,
		"ConfigurationProfileId": profileID,
		"ConfigurationVersion":   version,
	}, now)
	if start.Code != http.StatusOK {
		t.Fatalf("StartDeployment status=%d body=%q", start.Code, start.Body.String())
	}
	var startOut map[string]any
	_ = json.Unmarshal(start.Body.Bytes(), &startOut)
	depID, _ := startOut["Id"].(string)
	if depID == "" {
		t.Fatalf("missing deployment Id: %s", start.Body.String())
	}

	get := mustAppConfigJSON(t, handler, "AmazonAppConfig.GetDeployment", "appconfig", map[string]any{
		"DeploymentId": depID,
	}, now)
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), depID) {
		t.Fatalf("GetDeployment status=%d body=%q", get.Code, get.Body.String())
	}

	list := mustAppConfigJSON(t, handler, "AmazonAppConfig.ListDeployments", "appconfig", map[string]any{
		"ApplicationId":          appID,
		"EnvironmentId":          envID,
		"ConfigurationProfileId": profileID,
	}, now)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), depID) {
		t.Fatalf("ListDeployments status=%d body=%q", list.Code, list.Body.String())
	}
}

func TestAppConfigStartDeploymentVersionString(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	appID, envID, profileID, _ := setupAppConfigDeployLab(t, handler, now)
	start := mustAppConfigJSON(t, handler, "AmazonAppConfig.StartDeployment", "appconfig", map[string]any{
		"ApplicationId":          appID,
		"EnvironmentId":          envID,
		"ConfigurationProfileId": profileID,
		"ConfigurationVersion":   "1",
	}, now)
	if start.Code != http.StatusOK {
		t.Fatalf("StartDeployment string version status=%d body=%q", start.Code, start.Body.String())
	}
}

func TestAppConfigDeploymentNegatives(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	appID, envID, profileID, _ := setupAppConfigDeployLab(t, handler, now)

	missingParams := mustAppConfigJSON(t, handler, "AmazonAppConfig.StartDeployment", "appconfig", map[string]any{
		"ApplicationId": appID,
	}, now)
	if missingParams.Code != http.StatusBadRequest || !strings.Contains(missingParams.Body.String(), "BadRequestException") {
		t.Fatalf("StartDeployment missing params want BadRequest status=%d body=%q", missingParams.Code, missingParams.Body.String())
	}

	badVersion := mustAppConfigJSON(t, handler, "AmazonAppConfig.StartDeployment", "appconfig", map[string]any{
		"ApplicationId":          appID,
		"EnvironmentId":          envID,
		"ConfigurationProfileId": profileID,
		"ConfigurationVersion":   0,
	}, now)
	if badVersion.Code != http.StatusBadRequest || !strings.Contains(badVersion.Body.String(), "BadRequestException") {
		t.Fatalf("StartDeployment version 0 want BadRequest status=%d body=%q", badVersion.Code, badVersion.Body.String())
	}

	badVersionStr := mustAppConfigJSON(t, handler, "AmazonAppConfig.StartDeployment", "appconfig", map[string]any{
		"ApplicationId":          appID,
		"EnvironmentId":          envID,
		"ConfigurationProfileId": profileID,
		"ConfigurationVersion":   "nope",
	}, now)
	if badVersionStr.Code != http.StatusBadRequest || !strings.Contains(badVersionStr.Body.String(), "BadRequestException") {
		t.Fatalf("StartDeployment bad string version want BadRequest status=%d body=%q", badVersionStr.Code, badVersionStr.Body.String())
	}

	notFound := mustAppConfigJSON(t, handler, "AmazonAppConfig.StartDeployment", "appconfig", map[string]any{
		"ApplicationId":          "missing-app",
		"EnvironmentId":          envID,
		"ConfigurationProfileId": profileID,
		"ConfigurationVersion":   1,
	}, now)
	if notFound.Code != http.StatusNotFound || !strings.Contains(notFound.Body.String(), "ResourceNotFoundException") {
		t.Fatalf("StartDeployment missing app want NotFound status=%d body=%q", notFound.Code, notFound.Body.String())
	}

	getMissing := mustAppConfigJSON(t, handler, "AmazonAppConfig.GetDeployment", "appconfig", map[string]any{
		"DeploymentId": "dep-missing",
	}, now)
	if getMissing.Code != http.StatusNotFound || !strings.Contains(getMissing.Body.String(), "ResourceNotFoundException") {
		t.Fatalf("GetDeployment missing want NotFound status=%d body=%q", getMissing.Code, getMissing.Body.String())
	}

	getEmpty := mustAppConfigJSON(t, handler, "AmazonAppConfig.GetDeployment", "appconfig", map[string]any{}, now)
	if getEmpty.Code != http.StatusBadRequest || !strings.Contains(getEmpty.Body.String(), "BadRequestException") {
		t.Fatalf("GetDeployment empty want BadRequest status=%d body=%q", getEmpty.Code, getEmpty.Body.String())
	}

	listEmptyApp := mustAppConfigJSON(t, handler, "AmazonAppConfig.ListDeployments", "appconfig", map[string]any{}, now)
	if listEmptyApp.Code != http.StatusBadRequest || !strings.Contains(listEmptyApp.Body.String(), "BadRequestException") {
		t.Fatalf("ListDeployments empty want BadRequest status=%d body=%q", listEmptyApp.Code, listEmptyApp.Body.String())
	}

	listMissingApp := mustAppConfigJSON(t, handler, "AmazonAppConfig.ListDeployments", "appconfig", map[string]any{
		"ApplicationId": "no-such-app",
	}, now)
	if listMissingApp.Code != http.StatusNotFound || !strings.Contains(listMissingApp.Body.String(), "ResourceNotFoundException") {
		t.Fatalf("ListDeployments missing app want NotFound status=%d body=%q", listMissingApp.Code, listMissingApp.Body.String())
	}

	unknown := mustAppConfigJSON(t, handler, "AmazonAppConfig.DeleteApplication", "appconfig", map[string]any{}, now)
	if unknown.Code != http.StatusNotImplemented || !strings.Contains(unknown.Body.String(), "InternalFailure") {
		t.Fatalf("unknown AppConfig action want InternalFailure status=%d body=%q", unknown.Code, unknown.Body.String())
	}
}
