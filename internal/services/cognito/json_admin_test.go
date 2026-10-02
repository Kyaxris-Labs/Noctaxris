package cognito_test

import (
	"encoding/json"
	"strings"
	"testing"

	cognitosvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/cognito"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestCognitoAdminJSONHelpers(t *testing.T) {
	u := store.CognitoUser{
		Username: "alice", Sub: "sub-1", UserStatus: "CONFIRMED",
		PoolID: "us-east-1_lab", CreatedAt: 1_700_000_000_000, Enabled: true,
	}
	getRaw, err := cognitosvc.AdminGetUserJSON(u)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(getRaw), `"Username":"alice"`) || !strings.Contains(string(getRaw), `"Enabled":true`) {
		t.Fatalf("AdminGetUserJSON=%s", getRaw)
	}

	listRaw, err := cognitosvc.ListUsersJSON([]store.CognitoUser{u})
	if err != nil {
		t.Fatal(err)
	}
	var listBody map[string]any
	if err := json.Unmarshal(listRaw, &listBody); err != nil {
		t.Fatal(err)
	}
	users, _ := listBody["Users"].([]any)
	if len(users) != 1 {
		t.Fatalf("ListUsersJSON=%s", listRaw)
	}

	emptyList, err := cognitosvc.ListUsersJSON(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(emptyList), `"Users":[]`) && !strings.Contains(string(emptyList), `"Users": null`) {
		// empty slice marshals to []
		if !strings.Contains(string(emptyList), `"Users"`) {
			t.Fatalf("empty list=%s", emptyList)
		}
	}

	for name, fn := range map[string]func() ([]byte, error){
		"AdminSetUserPasswordJSON": cognitosvc.AdminSetUserPasswordJSON,
		"AdminDeleteUserJSON":      cognitosvc.AdminDeleteUserJSON,
		"AdminDisableUserJSON":     cognitosvc.AdminDisableUserJSON,
	} {
		raw, err := fn()
		if err != nil || string(raw) != "{}" {
			t.Fatalf("%s raw=%q err=%v", name, raw, err)
		}
	}
}
