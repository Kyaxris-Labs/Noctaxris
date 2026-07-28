package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/compute"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestSSMSendCommandGetInvocationWithHook(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	list, err := st.RunInstances(testAccountID, testRegion, store.RunInstancesInput{
		ImageID: "ami-alpine", MinCount: 1, MaxCount: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	instanceID := list[0].InstanceID
	if err := st.SetEC2ContainerID(testAccountID, testRegion, instanceID, "cid-lab-1", store.EC2StateRunning); err != nil {
		t.Fatal(err)
	}

	srv.SetSSMExecHookForTest(func(_ context.Context, containerID string, commands []string, _ int) (compute.ExecResult, error) {
		if containerID != "cid-lab-1" {
			t.Fatalf("containerID=%q", containerID)
		}
		if len(commands) != 1 || commands[0] != "echo hello-ssm" {
			t.Fatalf("commands=%v", commands)
		}
		return compute.ExecResult{Stdout: "hello-ssm\n", ExitCode: 0}, nil
	})

	sendRec := mustSSMJSON(t, handler, "SendCommand", map[string]any{
		"DocumentName":   "AWS-RunShellScript",
		"InstanceIds":    []string{instanceID},
		"TimeoutSeconds": 60,
		"Parameters": map[string]any{
			"commands": []string{"echo hello-ssm"},
		},
	}, now)
	if sendRec.Code != http.StatusOK {
		t.Fatalf("SendCommand status=%d body=%q", sendRec.Code, sendRec.Body.String())
	}
	var sendOut map[string]any
	if err := json.Unmarshal(sendRec.Body.Bytes(), &sendOut); err != nil {
		t.Fatal(err)
	}
	cmdObj, _ := sendOut["Command"].(map[string]any)
	commandID, _ := cmdObj["CommandId"].(string)
	if commandID == "" {
		t.Fatalf("missing CommandId in %q", sendRec.Body.String())
	}

	var invOut map[string]any
	deadline := time.Now().Add(2 * time.Second)
	for {
		getRec := mustSSMJSON(t, handler, "GetCommandInvocation", map[string]any{
			"CommandId":  commandID,
			"InstanceId": instanceID,
		}, now)
		if getRec.Code != http.StatusOK {
			t.Fatalf("GetCommandInvocation status=%d body=%q", getRec.Code, getRec.Body.String())
		}
		if err := json.Unmarshal(getRec.Body.Bytes(), &invOut); err != nil {
			t.Fatal(err)
		}
		status, _ := invOut["Status"].(string)
		if status == store.SSMCommandStatusSuccess || status == store.SSMCommandStatusFailed || status == store.SSMCommandStatusTimedOut {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("invocation still %q body=%v", status, invOut)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if invOut["Status"] != store.SSMCommandStatusSuccess {
		t.Fatalf("Status=%v body=%v", invOut["Status"], invOut)
	}
	stdout, _ := invOut["StandardOutputContent"].(string)
	if !strings.Contains(stdout, "hello-ssm") {
		t.Fatalf("stdout=%q", stdout)
	}
	if code, _ := invOut["ResponseCode"].(float64); int(code) != 0 {
		t.Fatalf("ResponseCode=%v", invOut["ResponseCode"])
	}

	listRec := mustSSMJSON(t, handler, "ListCommandInvocations", map[string]any{
		"CommandId": commandID,
	}, now)
	if listRec.Code != http.StatusOK {
		t.Fatalf("ListCommandInvocations status=%d body=%q", listRec.Code, listRec.Body.String())
	}
	var listOut map[string]any
	if err := json.Unmarshal(listRec.Body.Bytes(), &listOut); err != nil {
		t.Fatal(err)
	}
	arr, _ := listOut["CommandInvocations"].([]any)
	if len(arr) != 1 {
		t.Fatalf("CommandInvocations=%v", listOut["CommandInvocations"])
	}
}

func TestSSMSendCommandRejectsUnsupportedDocument(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	list, err := st.RunInstances(testAccountID, testRegion, store.RunInstancesInput{
		ImageID: "ami-alpine", MinCount: 1, MaxCount: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	rec := mustSSMJSON(t, handler, "SendCommand", map[string]any{
		"DocumentName":   "AWS-RunPowerShellScript",
		"InstanceIds":    []string{list[0].InstanceID},
		"TimeoutSeconds": 60,
		"Parameters":     map[string]any{"commands": []string{"echo x"}},
	}, now)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "InvalidDocument") {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestSSMSendCommandInvalidInstance(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	rec := mustSSMJSON(t, handler, "SendCommand", map[string]any{
		"DocumentName":   "AWS-RunShellScript",
		"InstanceIds":    []string{"i-missing"},
		"TimeoutSeconds": 60,
		"Parameters":     map[string]any{"commands": []string{"echo x"}},
	}, now)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "InvalidInstanceId") {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestSSMSendCommandFailsWithoutContainer(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	list, err := st.RunInstances(testAccountID, testRegion, store.RunInstancesInput{
		ImageID: "ami-alpine", MinCount: 1, MaxCount: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	instanceID := list[0].InstanceID

	sendRec := mustSSMJSON(t, handler, "SendCommand", map[string]any{
		"DocumentName":   "AWS-RunShellScript",
		"InstanceIds":    []string{instanceID},
		"TimeoutSeconds": 60,
		"Parameters":     map[string]any{"commands": []string{"echo x"}},
	}, now)
	if sendRec.Code != http.StatusOK {
		t.Fatalf("SendCommand status=%d body=%q", sendRec.Code, sendRec.Body.String())
	}
	var sendOut map[string]any
	_ = json.Unmarshal(sendRec.Body.Bytes(), &sendOut)
	cmdObj, _ := sendOut["Command"].(map[string]any)
	commandID, _ := cmdObj["CommandId"].(string)

	var status string
	deadline := time.Now().Add(2 * time.Second)
	for {
		getRec := mustSSMJSON(t, handler, "GetCommandInvocation", map[string]any{
			"CommandId":  commandID,
			"InstanceId": instanceID,
		}, now)
		var invOut map[string]any
		_ = json.Unmarshal(getRec.Body.Bytes(), &invOut)
		status, _ = invOut["Status"].(string)
		if status == store.SSMCommandStatusFailed {
			stderr, _ := invOut["StandardErrorContent"].(string)
			if !strings.Contains(stderr, "no nested container") {
				t.Fatalf("stderr=%q", stderr)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("want Failed, got %q", status)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestSSMSendCommandAccessDenied(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	_, userARN, err := st.CreateUser(testAccountID, "ssm-cmd-denied")
	if err != nil {
		t.Fatal(err)
	}
	userAKID, userSecret, err := st.CreateUserAccessKey(testAccountID, "ssm-cmd-denied")
	if err != nil {
		t.Fatal(err)
	}
	denyOnly := `{"Version":"2012-10-17","Statement":[{"Effect":"Deny","Action":"ssm:*","Resource":"*"}]}`
	if err := st.PutInlinePolicy(userARN, "nossm", denyOnly); err != nil {
		t.Fatal(err)
	}

	rec := mustSSMJSONWithCreds(t, handler, "SendCommand", map[string]any{
		"DocumentName":   "AWS-RunShellScript",
		"InstanceIds":    []string{"i-any"},
		"TimeoutSeconds": 60,
		"Parameters":     map[string]any{"commands": []string{"echo x"}},
	}, userAKID, userSecret, now)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "AccessDeniedException") {
		t.Fatalf("body=%q", rec.Body.String())
	}
}
