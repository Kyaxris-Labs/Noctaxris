package store_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestSecretsFourStepRotationStages(t *testing.T) {
	st := openSecretsStore(t)
	account := "000000000001"

	created, err := st.CreateSecret(account, "us-east-1", "four-step", "v1", nil, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetSecretRotationConfig(account, created.Name, "arn:aws:lambda:us-east-1:"+account+":function:rotator", ""); err != nil {
		t.Fatal(err)
	}

	var steps []string
	sec, err := st.RotateSecretFourStep(account, created.Name, "tok-rotate-1", func(eventJSON string) error {
		var evt map[string]string
		if err := json.Unmarshal([]byte(eventJSON), &evt); err != nil {
			return err
		}
		steps = append(steps, evt["Step"])
		if evt["ClientRequestToken"] != "tok-rotate-1" {
			t.Fatalf("token=%q", evt["ClientRequestToken"])
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	wantSteps := []string{
		store.SecretRotationStepCreateSecret,
		store.SecretRotationStepSetSecret,
		store.SecretRotationStepTestSecret,
		store.SecretRotationStepFinishSecret,
	}
	if strings.Join(steps, ",") != strings.Join(wantSteps, ",") {
		t.Fatalf("steps=%v want %v", steps, wantSteps)
	}
	if sec.SecretString == "" || sec.SecretString == "v1" {
		t.Fatalf("current value not rotated: %+v", sec)
	}
	if sec.VersionID != "tok-rotate-1" {
		t.Fatalf("VersionID=%q want tok-rotate-1", sec.VersionID)
	}

	got, err := st.GetSecretValue(account, created.Name)
	if err != nil {
		t.Fatal(err)
	}
	if got.SecretString != sec.SecretString {
		t.Fatalf("GetSecretValue=%q want %q", got.SecretString, sec.SecretString)
	}

	desc, err := st.DescribeSecret(account, created.Name)
	if err != nil {
		t.Fatal(err)
	}
	stages := desc.VersionIdsToStages
	if stages == nil {
		t.Fatal("VersionIdsToStages nil")
	}
	cur := stages["tok-rotate-1"]
	if !containsStage(cur, store.SecretVersionStageCurrent) {
		t.Fatalf("pending version stages=%v want AWSCURRENT", cur)
	}
	if containsStage(cur, store.SecretVersionStagePending) {
		t.Fatalf("AWSPENDING should be cleared on finish: %v", cur)
	}
	prev := stages[created.VersionID]
	if !containsStage(prev, store.SecretVersionStagePrevious) {
		t.Fatalf("v1 stages=%v want AWSPREVIOUS (map=%v)", prev, stages)
	}
}

func TestSecretsFourStepBlocksOrphanPending(t *testing.T) {
	st := openSecretsStore(t)
	account := "000000000001"
	if _, err := st.CreateSecret(account, "us-east-1", "orphan-rot", "v1", nil, "", "", ""); err != nil {
		t.Fatal(err)
	}

	_, err := st.RotateSecretFourStep(account, "orphan-rot", "tok-a", func(eventJSON string) error {
		var evt map[string]string
		_ = json.Unmarshal([]byte(eventJSON), &evt)
		if evt["Step"] == store.SecretRotationStepTestSecret {
			return errors.New("boom at testSecret")
		}
		return nil
	})
	if err == nil {
		t.Fatal("want mid-rotation error")
	}

	_, err = st.RotateSecretFourStep(account, "orphan-rot", "tok-b", func(string) error { return nil })
	if !errors.Is(err, store.ErrSecretRotationInProgress) {
		t.Fatalf("want ErrSecretRotationInProgress, got %v", err)
	}

	// Clear AWSPENDING via UpdateSecretVersionStage so a later rotate can proceed.
	if err := st.UpdateSecretVersionStage(account, "orphan-rot", "", "tok-a", store.SecretVersionStagePending); err != nil {
		t.Fatal(err)
	}
	sec, err := st.RotateSecretFourStep(account, "orphan-rot", "tok-c", func(string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if sec.VersionID != "tok-c" || sec.SecretString == "v1" {
		t.Fatalf("recover rotate failed: %+v", sec)
	}
}

func TestUpdateSecretVersionStageAndGetByStage(t *testing.T) {
	st := openSecretsStore(t)
	account := "000000000001"
	created, err := st.CreateSecret(account, "us-east-1", "stage-get", "current", nil, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	pending, err := st.PutSecretValueWithStages(account, created.Name, "pending-val", nil, "pending-1", []string{store.SecretVersionStagePending})
	if err != nil {
		t.Fatal(err)
	}
	if pending.SecretString != "pending-val" {
		t.Fatalf("pending=%+v", pending)
	}
	cur, err := st.GetSecretValueByStage(account, created.Name, store.SecretVersionStageCurrent, "")
	if err != nil {
		t.Fatal(err)
	}
	if cur.SecretString != "current" {
		t.Fatalf("current=%q", cur.SecretString)
	}
	pend, err := st.GetSecretValueByStage(account, created.Name, store.SecretVersionStagePending, "")
	if err != nil {
		t.Fatal(err)
	}
	if pend.SecretString != "pending-val" {
		t.Fatalf("pending get=%q", pend.SecretString)
	}
	if err := st.UpdateSecretVersionStage(account, created.Name, "pending-1", created.VersionID, store.SecretVersionStageCurrent); err != nil {
		t.Fatal(err)
	}
	after, err := st.GetSecretValue(account, created.Name)
	if err != nil {
		t.Fatal(err)
	}
	if after.SecretString != "pending-val" || after.VersionID != "pending-1" {
		t.Fatalf("after promote=%+v", after)
	}
}

func containsStage(stages []string, want string) bool {
	for _, s := range stages {
		if s == want {
			return true
		}
	}
	return false
}
