package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestIoTThingShadowPolicyAndTopicRuleOps(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	thing := mustJSONTarget(t, handler, "AWSIotService.CreateThing", "iot", map[string]any{
		"thingName": "shadow-ops",
		"attributePayload": map[string]any{
			"attributes": map[string]string{"env": "lab"},
		},
	}, now)
	if thing.Code != http.StatusOK {
		t.Fatalf("CreateThing %d %s", thing.Code, thing.Body.String())
	}
	dup := mustJSONTarget(t, handler, "AWSIotService.CreateThing", "iot", map[string]any{"thingName": "shadow-ops"}, now)
	if dup.Code == http.StatusOK {
		t.Fatalf("dup thing should fail")
	}
	desc := mustJSONTarget(t, handler, "AWSIotService.DescribeThing", "iot", map[string]any{"thingName": "shadow-ops"}, now)
	if desc.Code != http.StatusOK {
		t.Fatalf("DescribeThing %d %s", desc.Code, desc.Body.String())
	}
	list := mustJSONTarget(t, handler, "AWSIotService.ListThings", "iot", map[string]any{}, now)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "shadow-ops") {
		t.Fatalf("ListThings %d %s", list.Code, list.Body.String())
	}
	upd := mustJSONTarget(t, handler, "AWSIotService.UpdateThing", "iot", map[string]any{
		"thingName": "shadow-ops",
		"attributePayload": map[string]any{
			"attributes": map[string]string{"env": "prod"},
		},
	}, now)
	if upd.Code != http.StatusOK {
		t.Fatalf("UpdateThing %d %s", upd.Code, upd.Body.String())
	}

	pol := mustJSONTarget(t, handler, "AWSIotService.CreatePolicy", "iot", map[string]any{
		"policyName":     "shadow-pol",
		"policyDocument": `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"iot:*","Resource":"*"}]}`,
	}, now)
	if pol.Code != http.StatusOK {
		t.Fatalf("CreatePolicy %d %s", pol.Code, pol.Body.String())
	}
	dupPol := mustJSONTarget(t, handler, "AWSIotService.CreatePolicy", "iot", map[string]any{
		"policyName": "shadow-pol", "policyDocument": `{}`,
	}, now)
	if dupPol.Code == http.StatusOK {
		t.Fatalf("dup policy should fail")
	}

	rule := mustJSONTarget(t, handler, "AWSIotService.CreateTopicRule", "iot", map[string]any{
		"ruleName": "shadow_rule",
		"topicRulePayload": map[string]any{
			"sql": "SELECT * FROM 'topic/ops'",
			"actions": []map[string]any{
				{"republish": map[string]any{"topic": "topic/out", "qos": 0}},
			},
		},
	}, now)
	if rule.Code != http.StatusOK {
		t.Fatalf("CreateTopicRule %d %s", rule.Code, rule.Body.String())
	}
	getRule := mustJSONTarget(t, handler, "AWSIotService.GetTopicRule", "iot", map[string]any{"ruleName": "shadow_rule"}, now)
	if getRule.Code != http.StatusOK {
		t.Fatalf("GetTopicRule %d %s", getRule.Code, getRule.Body.String())
	}
	listRules := mustJSONTarget(t, handler, "AWSIotService.ListTopicRules", "iot", map[string]any{}, now)
	if listRules.Code != http.StatusOK {
		t.Fatalf("ListTopicRules %d %s", listRules.Code, listRules.Body.String())
	}
	replace := mustJSONTarget(t, handler, "AWSIotService.ReplaceTopicRule", "iot", map[string]any{
		"ruleName": "shadow_rule",
		"topicRulePayload": map[string]any{
			"sql":     "SELECT * FROM 'topic/ops2'",
			"actions": []map[string]any{{"republish": map[string]any{"topic": "topic/out2"}}},
		},
	}, now)
	if replace.Code != http.StatusOK {
		t.Fatalf("ReplaceTopicRule %d %s", replace.Code, replace.Body.String())
	}
	delRule := mustJSONTarget(t, handler, "AWSIotService.DeleteTopicRule", "iot", map[string]any{"ruleName": "shadow_rule"}, now)
	if delRule.Code != http.StatusOK {
		t.Fatalf("DeleteTopicRule %d %s", delRule.Code, delRule.Body.String())
	}

	delPol := mustJSONTarget(t, handler, "AWSIotService.DeletePolicy", "iot", map[string]any{"policyName": "shadow-pol"}, now)
	if delPol.Code != http.StatusOK {
		t.Fatalf("DeletePolicy %d %s", delPol.Code, delPol.Body.String())
	}
	delThing := mustJSONTarget(t, handler, "AWSIotService.DeleteThing", "iot", map[string]any{"thingName": "shadow-ops"}, now)
	if delThing.Code != http.StatusOK {
		t.Fatalf("DeleteThing %d %s", delThing.Code, delThing.Body.String())
	}
}

func TestEventBridgeBusRuleTargetPutEventsOps(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	bus := mustEventsJSON(t, handler, "CreateEventBus", map[string]any{"Name": "ops-bus2"}, now)
	if bus.Code != http.StatusOK {
		t.Fatalf("CreateEventBus %d %s", bus.Code, bus.Body.String())
	}
	q := mustSQSJSON(t, handler, "CreateQueue", map[string]any{"QueueName": "ops-bus2-q"}, now)
	if q.Code != http.StatusOK {
		t.Fatalf("CreateQueue %d %s", q.Code, q.Body.String())
	}
	qARN := "arn:aws:sqs:" + testRegion + ":" + testAccountID + ":ops-bus2-q"

	rule := mustEventsJSON(t, handler, "PutRule", map[string]any{
		"Name":         "ops-rule2",
		"EventBusName": "ops-bus2",
		"EventPattern": `{"source":["noctaxris.ops"]}`,
		"State":        "ENABLED",
	}, now)
	if rule.Code != http.StatusOK {
		t.Fatalf("PutRule %d %s", rule.Code, rule.Body.String())
	}
	targets := mustEventsJSON(t, handler, "PutTargets", map[string]any{
		"Rule": "ops-rule2", "EventBusName": "ops-bus2",
		"Targets": []map[string]any{{"Id": "1", "Arn": qARN}},
	}, now)
	if targets.Code != http.StatusOK {
		t.Fatalf("PutTargets %d %s", targets.Code, targets.Body.String())
	}
	listTargets := mustEventsJSON(t, handler, "ListTargetsByRule", map[string]any{
		"Rule": "ops-rule2", "EventBusName": "ops-bus2",
	}, now)
	if listTargets.Code != http.StatusOK {
		t.Fatalf("ListTargetsByRule %d %s", listTargets.Code, listTargets.Body.String())
	}
	disable := mustEventsJSON(t, handler, "DisableRule", map[string]any{"Name": "ops-rule2", "EventBusName": "ops-bus2"}, now)
	if disable.Code != http.StatusOK {
		t.Fatalf("DisableRule %d %s", disable.Code, disable.Body.String())
	}
	enable := mustEventsJSON(t, handler, "EnableRule", map[string]any{"Name": "ops-rule2", "EventBusName": "ops-bus2"}, now)
	if enable.Code != http.StatusOK {
		t.Fatalf("EnableRule %d %s", enable.Code, enable.Body.String())
	}
	putEvents := mustEventsJSON(t, handler, "PutEvents", map[string]any{
		"Entries": []map[string]any{{
			"Source": "noctaxris.ops", "DetailType": "ops", "Detail": `{"ok":true}`, "EventBusName": "ops-bus2",
		}},
	}, now)
	if putEvents.Code != http.StatusOK {
		t.Fatalf("PutEvents %d %s", putEvents.Code, putEvents.Body.String())
	}
	rmTargets := mustEventsJSON(t, handler, "RemoveTargets", map[string]any{
		"Rule": "ops-rule2", "EventBusName": "ops-bus2", "Ids": []string{"1"},
	}, now)
	if rmTargets.Code != http.StatusOK {
		t.Fatalf("RemoveTargets %d %s", rmTargets.Code, rmTargets.Body.String())
	}
	delRule := mustEventsJSON(t, handler, "DeleteRule", map[string]any{"Name": "ops-rule2", "EventBusName": "ops-bus2"}, now)
	if delRule.Code != http.StatusOK {
		t.Fatalf("DeleteRule %d %s", delRule.Code, delRule.Body.String())
	}
	delBus := mustEventsJSON(t, handler, "DeleteEventBus", map[string]any{"Name": "ops-bus2"}, now)
	if delBus.Code != http.StatusOK {
		t.Fatalf("DeleteEventBus %d %s", delBus.Code, delBus.Body.String())
	}
}

func TestOrganizationsJSONListAndPolicyNegatives(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	listJSON := mustJSONTarget(t, handler, "AWSOrganizationsV20161128.ListAccounts", "organizations", map[string]any{}, now)
	if listJSON.Code != http.StatusOK {
		t.Fatalf("ListAccounts JSON %d %s", listJSON.Code, listJSON.Body.String())
	}

	post := func(body string) *httptest.ResponseRecorder {
		t.Helper()
		req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(body))
		signHeader(t, req, []byte(body), testAccessKey, testSecret, testRegion, "organizations", now)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	_ = post("Action=EnablePolicyType&Version=2016-11-28&RootId=r-root&PolicyType=SERVICE_CONTROL_POLICY")
	badPol := post("Action=CreatePolicy&Version=2016-11-28&Name=&Type=SERVICE_CONTROL_POLICY&Content=" + url.QueryEscape(`{}`))
	if badPol.Code == http.StatusOK {
		t.Fatalf("empty policy name should fail")
	}
	badAttach := post("Action=AttachPolicy&Version=2016-11-28&PolicyId=p-missing&TargetId=r-root")
	if badAttach.Code == http.StatusOK {
		t.Fatalf("AttachPolicy missing should fail")
	}
	badDesc := post("Action=DescribePolicy&Version=2016-11-28&PolicyId=p-missing")
	if badDesc.Code == http.StatusOK {
		t.Fatalf("DescribePolicy missing should fail")
	}
	badOU := post("Action=CreateOrganizationalUnit&Version=2016-11-28&ParentId=r-root&Name=")
	if badOU.Code == http.StatusOK {
		t.Fatalf("empty OU name should fail")
	}
	badMove := post("Action=MoveAccount&Version=2016-11-28&AccountId=000000000099&SourceParentId=r-root&DestinationParentId=r-root")
	if badMove.Code == http.StatusOK {
		t.Fatalf("MoveAccount missing account should fail")
	}
}

func TestBatchDescribeAndJobDefNegatives(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "batch-ops-svc", batchServiceTrustOK, now)
	mustCreateIAMRole(t, handler, "batch-ops-job", batchJobTrustOK, now)
	svcRole := "arn:aws:iam::" + testAccountID + ":role/batch-ops-svc"
	jobRole := "arn:aws:iam::" + testAccountID + ":role/batch-ops-job"

	ce := mustBatchREST(t, handler, "/v1/createcomputeenvironment", map[string]any{
		"computeEnvironmentName": "ops-ce", "type": "MANAGED", "serviceRole": svcRole,
	}, now)
	if ce.Code != http.StatusOK {
		t.Fatalf("CreateComputeEnvironment %d %s", ce.Code, ce.Body.String())
	}
	jq := mustBatchREST(t, handler, "/v1/createjobqueue", map[string]any{
		"jobQueueName": "ops-jq", "priority": 1,
		"computeEnvironmentOrder": []map[string]any{{"order": 1, "computeEnvironment": "ops-ce"}},
	}, now)
	if jq.Code != http.StatusOK {
		t.Fatalf("CreateJobQueue %d %s", jq.Code, jq.Body.String())
	}
	jd := mustBatchREST(t, handler, "/v1/registerjobdefinition", map[string]any{
		"jobDefinitionName": "ops-jd", "type": "container",
		"containerProperties": map[string]any{
			"image": "alpine:3.20", "command": []string{"echo", "hi"}, "jobRoleArn": jobRole,
		},
	}, now)
	if jd.Code != http.StatusOK {
		t.Fatalf("RegisterJobDefinition %d %s", jd.Code, jd.Body.String())
	}
	descCE := mustBatchREST(t, handler, "/v1/describecomputeenvironments", map[string]any{
		"computeEnvironments": []string{"ops-ce"},
	}, now)
	if descCE.Code != http.StatusOK {
		t.Fatalf("DescribeComputeEnvironments %d %s", descCE.Code, descCE.Body.String())
	}
	descJQ := mustBatchREST(t, handler, "/v1/describejobqueues", map[string]any{"jobQueues": []string{"ops-jq"}}, now)
	if descJQ.Code != http.StatusOK {
		t.Fatalf("DescribeJobQueues %d %s", descJQ.Code, descJQ.Body.String())
	}
	descJD := mustBatchREST(t, handler, "/v1/describejobdefinitions", map[string]any{
		"jobDefinitionName": "ops-jd",
	}, now)
	if descJD.Code != http.StatusOK {
		t.Fatalf("DescribeJobDefinitions %d %s", descJD.Code, descJD.Body.String())
	}
	submit := mustBatchREST(t, handler, "/v1/submitjob", map[string]any{
		"jobName": "ops-job", "jobQueue": "ops-jq", "jobDefinition": "ops-jd",
	}, now)
	if submit.Code == http.StatusOK {
		var out map[string]any
		_ = json.Unmarshal(submit.Body.Bytes(), &out)
		jobID, _ := out["jobId"].(string)
		_ = mustBatchREST(t, handler, "/v1/describejobs", map[string]any{"jobs": []string{jobID}}, now)
		_ = mustBatchREST(t, handler, "/v1/canceljob", map[string]any{"jobId": jobID, "reason": "lab"}, now)
		_ = mustBatchREST(t, handler, "/v1/terminatejob", map[string]any{"jobId": jobID, "reason": "lab"}, now)
	}

	fargate := mustBatchREST(t, handler, "/v1/registerjobdefinition", map[string]any{
		"jobDefinitionName": "ops-fargate", "type": "container",
		"platformCapabilities": []string{"FARGATE"},
		"containerProperties": map[string]any{
			"image": "alpine:3.20", "networkMode": "awsvpc",
		},
	}, now)
	if fargate.Code == http.StatusOK {
		t.Fatalf("Fargate shape should fail closed")
	}
	_ = mustBatchREST(t, handler, "/v1/deregisterjobdefinition", map[string]any{"jobDefinition": "ops-jd"}, now)
}
