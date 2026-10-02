package iot_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	iotsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/iot"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestDescribeEndpointAndNamedShadowsJSON(t *testing.T) {
	raw, err := iotsvc.DescribeEndpointJSON("127.0.0.1:4566")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"endpointAddress":"127.0.0.1:4566"`) {
		t.Fatalf("body=%s", raw)
	}

	empty, err := iotsvc.ListNamedShadowsJSON(nil, 1700000000)
	if err != nil {
		t.Fatal(err)
	}
	var emptyBody map[string]any
	if err := json.Unmarshal(empty, &emptyBody); err != nil {
		t.Fatal(err)
	}
	results, _ := emptyBody["results"].([]any)
	if results == nil || len(results) != 0 || emptyBody["timestamp"] != float64(1700000000) {
		t.Fatalf("empty named shadows=%s", empty)
	}

	named, err := iotsvc.ListNamedShadowsJSON([]string{"cfg", "ops"}, 42)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(named), `"cfg"`) || !strings.Contains(string(named), `"ops"`) {
		t.Fatalf("named=%s", named)
	}
}

func TestRoleAliasJSONShapes(t *testing.T) {
	ra := store.IoTRoleAlias{
		RoleAlias:                 "lab-alias",
		RoleAliasARN:              "arn:aws:iot:us-east-1:1:rolealias/lab-alias",
		RoleARN:                   "arn:aws:iam::1:role/iot",
		CredentialDurationSeconds: 3600,
	}
	created, err := iotsvc.CreateRoleAliasJSON(ra)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"roleAlias":"lab-alias"`, `"roleArn":"arn:aws:iam::1:role/iot"`, `"credentialDurationSeconds":3600`} {
		if !strings.Contains(string(created), want) {
			t.Fatalf("missing %s in %s", want, created)
		}
	}

	listed, err := iotsvc.ListRoleAliasesJSON([]store.IoTRoleAlias{ra})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(listed), `"roleAliases"`) || !strings.Contains(string(listed), "lab-alias") {
		t.Fatalf("list=%s", listed)
	}
	emptyList, err := iotsvc.ListRoleAliasesJSON(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(emptyList), `"roleAliases":[]`) {
		t.Fatalf("empty list=%s", emptyList)
	}
}

func TestJobJSONShapes(t *testing.T) {
	job := store.IoTJob{
		JobID: "job-1", JobARN: "arn:aws:iot:us-east-1:1:job/job-1",
		Status: "IN_PROGRESS", Targets: []string{"thing-a"}, Document: `{"op":"x"}`, CreatedAt: 99,
	}
	created, err := iotsvc.CreateJobJSON(job)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(created), `"jobId":"job-1"`) || !strings.Contains(string(created), `"jobArn"`) {
		t.Fatalf("create=%s", created)
	}
	desc, err := iotsvc.DescribeJobJSON(job)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(desc), `"targets"`) || !strings.Contains(string(desc), `"document"`) {
		t.Fatalf("describe=%s", desc)
	}

	queued := store.IoTJobExecution{
		JobID: "job-1", ThingName: "thing-a", Status: store.IoTJobExecQueued,
		ExecutionNumber: 1, VersionNumber: 1, QueuedAt: 10, LastUpdatedAt: 10,
	}
	inProg := store.IoTJobExecution{
		JobID: "job-2", ThingName: "thing-a", Status: store.IoTJobExecInProgress,
		ExecutionNumber: 1, VersionNumber: 2, QueuedAt: 5, StartedAt: 8, LastUpdatedAt: 9,
	}
	pending, err := iotsvc.GetPendingJobExecutionsJSON([]store.IoTJobExecution{inProg}, []store.IoTJobExecution{queued})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(pending), `"inProgressJobs"`) || !strings.Contains(string(pending), `"queuedJobs"`) {
		t.Fatalf("pending=%s", pending)
	}
	if !strings.Contains(string(pending), `"startedAt":8`) {
		t.Fatalf("startedAt missing in summary: %s", pending)
	}

	ex := store.IoTJobExecution{
		JobID: "job-1", ThingName: "thing-a", Status: store.IoTJobExecInProgress,
		ExecutionNumber: 1, VersionNumber: 2, QueuedAt: 10, StartedAt: 11, LastUpdatedAt: 12,
		StatusDetails: map[string]string{"step": "1"}, JobDocument: `{"op":"x"}`,
	}
	withDoc, err := iotsvc.DescribeJobExecutionJSON(ex, true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(withDoc), `"jobDocument"`) || !strings.Contains(string(withDoc), `"statusDetails"`) {
		t.Fatalf("withDoc=%s", withDoc)
	}
	noDoc, err := iotsvc.DescribeJobExecutionJSON(ex, false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(noDoc), `"jobDocument"`) {
		t.Fatalf("jobDocument should be omitted: %s", noDoc)
	}
}

func TestCredentialsProviderJSON(t *testing.T) {
	exp := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	raw, err := iotsvc.CredentialsProviderJSON("AKIAEXAMPLE", "secret", "token", exp)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	creds, _ := body["credentials"].(map[string]any)
	if creds["accessKeyId"] != "AKIAEXAMPLE" || creds["secretAccessKey"] != "secret" || creds["sessionToken"] != "token" {
		t.Fatalf("creds=%v", creds)
	}
	if creds["expiration"] != "2026-01-02T03:04:05Z" {
		t.Fatalf("expiration=%v", creds["expiration"])
	}
}

func TestListThingPrincipalsNilAndBadActionsJSON(t *testing.T) {
	raw, err := iotsvc.ListThingPrincipalsJSON(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"principals":[]`) {
		t.Fatalf("nil principals=%s", raw)
	}
	rule := store.IoTTopicRule{RuleName: "r", SQL: "SELECT * FROM 't'", ActionsJSON: `not-json`}
	got, err := iotsvc.GetTopicRuleJSON(rule)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(got, &body); err != nil {
		t.Fatal(err)
	}
	ruleObj, _ := body["rule"].(map[string]any)
	actions, _ := ruleObj["actions"].([]any)
	if actions == nil || len(actions) != 0 {
		t.Fatalf("bad actions should become []: %s", got)
	}
}
