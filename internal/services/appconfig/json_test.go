package appconfig_test

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	appconfigsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/appconfig"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestAppConfigJSON(t *testing.T) {
	app := store.AppConfigApplication{ID: "a1", Name: "lab", Description: "d"}
	raw, err := appconfigsvc.CreateApplicationJSON(app)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out["Id"] != "a1" {
		t.Fatalf("app=%v", out)
	}

	env := store.AppConfigEnvironment{ApplicationID: "a1", ID: "e1", Name: "prod", Description: "env"}
	raw, _ = appconfigsvc.CreateEnvironmentJSON(env)
	_ = json.Unmarshal(raw, &out)
	if out["State"] != "ReadyForDeployment" {
		t.Fatalf("env=%v", out)
	}

	prof := store.AppConfigProfile{ApplicationID: "a1", ID: "p1", Name: "cfg", LocationURI: "hosted", Description: "p"}
	raw, _ = appconfigsvc.CreateConfigurationProfileJSON(prof)
	_ = json.Unmarshal(raw, &out)
	if out["Type"] != "AWS.Freeform" {
		t.Fatalf("profile=%v", out)
	}

	ver := store.AppConfigHostedVersion{
		ApplicationID: "a1", ProfileID: "p1", VersionNumber: 3,
		ContentType: "application/json", Content: []byte(`{"k":1}`),
	}
	raw, _ = appconfigsvc.CreateHostedConfigurationVersionJSON(ver)
	_ = json.Unmarshal(raw, &out)
	if out["VersionNumber"] != float64(3) {
		t.Fatalf("version=%v", out)
	}

	raw, _ = appconfigsvc.GetConfigurationJSON(ver)
	_ = json.Unmarshal(raw, &out)
	wantB64 := base64.StdEncoding.EncodeToString(ver.Content)
	if out["Content"] != wantB64 || out["ConfigurationVersion"] != "3" {
		t.Fatalf("get cfg=%v", out)
	}

	raw, _ = appconfigsvc.StartConfigurationSessionJSON("tok-abc")
	_ = json.Unmarshal(raw, &out)
	if out["InitialConfigurationToken"] != "tok-abc" {
		t.Fatalf("session=%v", out)
	}
}
