package server_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func mustAppConfigJSON(t *testing.T, handler http.Handler, target, service string, payload map[string]any, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", target)
	signHeader(t, req, raw, testAccessKey, testSecret, testRegion, service, now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestAppConfigRoundTrip(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createApp := mustAppConfigJSON(t, handler, "AmazonAppConfig.CreateApplication", "appconfig", map[string]any{
		"Name": "lab-app",
	}, now)
	if createApp.Code != http.StatusOK {
		t.Fatalf("CreateApplication status=%d body=%q", createApp.Code, createApp.Body.String())
	}
	var appOut map[string]any
	_ = json.Unmarshal(createApp.Body.Bytes(), &appOut)
	appID, _ := appOut["Id"].(string)

	createEnv := mustAppConfigJSON(t, handler, "AmazonAppConfig.CreateEnvironment", "appconfig", map[string]any{
		"ApplicationId": appID,
		"Name":          "dev",
	}, now)
	if createEnv.Code != http.StatusOK {
		t.Fatalf("CreateEnvironment status=%d body=%q", createEnv.Code, createEnv.Body.String())
	}
	var envOut map[string]any
	_ = json.Unmarshal(createEnv.Body.Bytes(), &envOut)
	envID, _ := envOut["Id"].(string)

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
	profileID, _ := profileOut["Id"].(string)

	hosted := mustAppConfigJSON(t, handler, "AmazonAppConfig.CreateHostedConfigurationVersion", "appconfig", map[string]any{
		"ApplicationId":          appID,
		"ConfigurationProfileId": profileID,
		"ContentType":            "application/json",
		"Content":                base64.StdEncoding.EncodeToString([]byte(`{"x":1}`)),
	}, now)
	if hosted.Code != http.StatusOK {
		t.Fatalf("CreateHostedConfigurationVersion status=%d body=%q", hosted.Code, hosted.Body.String())
	}

	get := mustAppConfigJSON(t, handler, "AmazonAppConfig.GetConfiguration", "appconfig", map[string]any{
		"Application":   appID,
		"Environment":   envID,
		"Configuration": profileID,
		"ClientId":      "lab",
	}, now)
	if get.Code != http.StatusOK {
		t.Fatalf("GetConfiguration status=%d body=%q", get.Code, get.Body.String())
	}

	start := mustAppConfigJSON(t, handler, "AmazonAppConfigData.StartConfigurationSession", "appconfigdata", map[string]any{
		"ApplicationIdentifier":          appID,
		"EnvironmentIdentifier":          envID,
		"ConfigurationProfileIdentifier": profileID,
	}, now)
	if start.Code != http.StatusOK {
		t.Fatalf("StartConfigurationSession status=%d body=%q", start.Code, start.Body.String())
	}
	var startOut map[string]any
	_ = json.Unmarshal(start.Body.Bytes(), &startOut)
	token, _ := startOut["InitialConfigurationToken"].(string)

	latest := mustAppConfigJSON(t, handler, "AmazonAppConfigData.GetLatestConfiguration", "appconfigdata", map[string]any{
		"ConfigurationToken": token,
	}, now)
	if latest.Code != http.StatusOK {
		t.Fatalf("GetLatestConfiguration status=%d body=%q", latest.Code, latest.Body.String())
	}
	if latest.Body.String() != `{"x":1}` {
		t.Fatalf("latest body=%q", latest.Body.String())
	}
}
