package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestIoTCertPolicyPrincipalsAndTopicRuleMgmt(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	thing := mustJSONTarget(t, handler, "AWSIotService.CreateThing", "iot", map[string]any{
		"thingName": "cov-thing",
	}, now)
	if thing.Code != http.StatusOK {
		t.Fatalf("CreateThing status=%d body=%q", thing.Code, thing.Body.String())
	}

	cert := mustJSONTarget(t, handler, "AWSIotService.CreateKeysAndCertificate", "iot", map[string]any{
		"setAsActive": true,
	}, now)
	if cert.Code != http.StatusOK {
		t.Fatalf("CreateKeysAndCertificate status=%d body=%q", cert.Code, cert.Body.String())
	}
	var certOut map[string]any
	_ = json.Unmarshal(cert.Body.Bytes(), &certOut)
	certID, _ := certOut["certificateId"].(string)
	certARN, _ := certOut["certificateArn"].(string)
	if certID == "" {
		t.Fatalf("missing certificateId: %s", cert.Body.String())
	}

	listCerts := mustJSONTarget(t, handler, "AWSIotService.ListCertificates", "iot", map[string]any{}, now)
	if listCerts.Code != http.StatusOK || !strings.Contains(listCerts.Body.String(), certID) {
		t.Fatalf("ListCertificates status=%d body=%q", listCerts.Code, listCerts.Body.String())
	}

	pol := mustJSONTarget(t, handler, "AWSIotService.CreatePolicy", "iot", map[string]any{
		"policyName":     "cov-pol",
		"policyDocument": `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"iot:*","Resource":"*"}]}`,
	}, now)
	if pol.Code != http.StatusOK {
		t.Fatalf("CreatePolicy status=%d body=%q", pol.Code, pol.Body.String())
	}
	getPol := mustJSONTarget(t, handler, "AWSIotService.GetPolicy", "iot", map[string]any{
		"policyName": "cov-pol",
	}, now)
	if getPol.Code != http.StatusOK || !strings.Contains(getPol.Body.String(), "cov-pol") {
		t.Fatalf("GetPolicy status=%d body=%q", getPol.Code, getPol.Body.String())
	}
	listPol := mustJSONTarget(t, handler, "AWSIotService.ListPolicies", "iot", map[string]any{}, now)
	if listPol.Code != http.StatusOK || !strings.Contains(listPol.Body.String(), "cov-pol") {
		t.Fatalf("ListPolicies status=%d body=%q", listPol.Code, listPol.Body.String())
	}
	missingPol := mustJSONTarget(t, handler, "AWSIotService.GetPolicy", "iot", map[string]any{
		"policyName": "nope",
	}, now)
	if missingPol.Code == http.StatusOK {
		t.Fatalf("GetPolicy missing should fail: %q", missingPol.Body.String())
	}

	attachPol := mustJSONTarget(t, handler, "AWSIotService.AttachPolicy", "iot", map[string]any{
		"policyName": "cov-pol",
		"target":     certARN,
	}, now)
	if attachPol.Code != http.StatusOK {
		t.Fatalf("AttachPolicy status=%d body=%q", attachPol.Code, attachPol.Body.String())
	}
	attachThing := mustJSONTarget(t, handler, "AWSIotService.AttachThingPrincipal", "iot", map[string]any{
		"thingName": "cov-thing",
		"principal": certARN,
	}, now)
	if attachThing.Code != http.StatusOK {
		t.Fatalf("AttachThingPrincipal status=%d body=%q", attachThing.Code, attachThing.Body.String())
	}
	listPrinc := mustJSONTarget(t, handler, "AWSIotService.ListThingPrincipals", "iot", map[string]any{
		"thingName": "cov-thing",
	}, now)
	if listPrinc.Code != http.StatusOK || !strings.Contains(listPrinc.Body.String(), certARN) {
		t.Fatalf("ListThingPrincipals status=%d body=%q", listPrinc.Code, listPrinc.Body.String())
	}

	createRule := mustJSONTarget(t, handler, "AWSIotService.CreateTopicRule", "iot", map[string]any{
		"ruleName": "cov-rule",
		"topicRulePayload": map[string]any{
			"sql":         "SELECT * FROM 'cov/+'",
			"description": "coverage",
			"actions": []any{
				map[string]any{"repost": map[string]any{"topic": "out/cov"}},
			},
		},
	}, now)
	if createRule.Code != http.StatusOK {
		t.Fatalf("CreateTopicRule status=%d body=%q", createRule.Code, createRule.Body.String())
	}
	listRules := mustJSONTarget(t, handler, "AWSIotService.ListTopicRules", "iot", map[string]any{}, now)
	if listRules.Code != http.StatusOK || !strings.Contains(listRules.Body.String(), "cov-rule") {
		t.Fatalf("ListTopicRules status=%d body=%q", listRules.Code, listRules.Body.String())
	}
	replace := mustJSONTarget(t, handler, "AWSIotService.ReplaceTopicRule", "iot", map[string]any{
		"ruleName": "cov-rule",
		"topicRulePayload": map[string]any{
			"sql":           "SELECT * FROM 'cov/#'",
			"description":   "replaced",
			"ruleDisabled":  false,
			"actions": []any{
				map[string]any{"repost": map[string]any{"topic": "out/cov2"}},
			},
		},
	}, now)
	if replace.Code != http.StatusOK {
		t.Fatalf("ReplaceTopicRule status=%d body=%q", replace.Code, replace.Body.String())
	}
	disable := mustJSONTarget(t, handler, "AWSIotService.DisableTopicRule", "iot", map[string]any{
		"ruleName": "cov-rule",
	}, now)
	if disable.Code != http.StatusOK {
		t.Fatalf("DisableTopicRule status=%d body=%q", disable.Code, disable.Body.String())
	}
	enable := mustJSONTarget(t, handler, "AWSIotService.EnableTopicRule", "iot", map[string]any{
		"ruleName": "cov-rule",
	}, now)
	if enable.Code != http.StatusOK {
		t.Fatalf("EnableTopicRule status=%d body=%q", enable.Code, enable.Body.String())
	}
	delRule := mustJSONTarget(t, handler, "AWSIotService.DeleteTopicRule", "iot", map[string]any{
		"ruleName": "cov-rule",
	}, now)
	if delRule.Code != http.StatusOK {
		t.Fatalf("DeleteTopicRule status=%d body=%q", delRule.Code, delRule.Body.String())
	}
	delRuleGone := mustJSONTarget(t, handler, "AWSIotService.DeleteTopicRule", "iot", map[string]any{
		"ruleName": "cov-rule",
	}, now)
	if delRuleGone.Code == http.StatusOK {
		t.Fatalf("DeleteTopicRule missing should fail: %q", delRuleGone.Body.String())
	}
	replaceGone := mustJSONTarget(t, handler, "AWSIotService.ReplaceTopicRule", "iot", map[string]any{
		"ruleName": "cov-rule",
		"topicRulePayload": map[string]any{
			"sql":     "SELECT * FROM 'x'",
			"actions": []any{},
		},
	}, now)
	if replaceGone.Code == http.StatusOK {
		t.Fatalf("ReplaceTopicRule missing should fail: %q", replaceGone.Body.String())
	}

	detachPol := mustJSONTarget(t, handler, "AWSIotService.DetachPolicy", "iot", map[string]any{
		"policyName": "cov-pol", "target": certARN,
	}, now)
	if detachPol.Code != http.StatusOK {
		t.Fatalf("DetachPolicy status=%d body=%q", detachPol.Code, detachPol.Body.String())
	}
	delThing := mustJSONTarget(t, handler, "AWSIotService.DeleteThing", "iot", map[string]any{
		"thingName": "cov-thing",
	}, now)
	if delThing.Code != http.StatusOK {
		t.Fatalf("DeleteThing status=%d body=%q", delThing.Code, delThing.Body.String())
	}
	delPol := mustJSONTarget(t, handler, "AWSIotService.DeletePolicy", "iot", map[string]any{
		"policyName": "cov-pol",
	}, now)
	if delPol.Code != http.StatusOK {
		t.Fatalf("DeletePolicy status=%d body=%q", delPol.Code, delPol.Body.String())
	}
	delPolGone := mustJSONTarget(t, handler, "AWSIotService.DeletePolicy", "iot", map[string]any{
		"policyName": "cov-pol",
	}, now)
	if delPolGone.Code == http.StatusOK {
		t.Fatalf("DeletePolicy missing should fail: %q", delPolGone.Body.String())
	}

	inactivate := mustJSONTarget(t, handler, "AWSIotService.UpdateCertificate", "iot", map[string]any{
		"certificateId": certID, "newStatus": "INACTIVE",
	}, now)
	if inactivate.Code != http.StatusOK {
		t.Fatalf("UpdateCertificate status=%d body=%q", inactivate.Code, inactivate.Body.String())
	}
	delCert := mustJSONTarget(t, handler, "AWSIotService.DeleteCertificate", "iot", map[string]any{
		"certificateId": certID,
	}, now)
	if delCert.Code != http.StatusOK {
		t.Fatalf("DeleteCertificate status=%d body=%q", delCert.Code, delCert.Body.String())
	}
	delCertGone := mustJSONTarget(t, handler, "AWSIotService.DeleteCertificate", "iot", map[string]any{
		"certificateId": certID,
	}, now)
	if delCertGone.Code == http.StatusOK {
		t.Fatalf("DeleteCertificate missing should fail: %q", delCertGone.Body.String())
	}
}
