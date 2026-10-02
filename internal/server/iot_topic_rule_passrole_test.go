package server_test

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

const iotRuleTrustOK = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"iot.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
const iotRuleTrustBad = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::000000000001:root"},"Action":"sts:AssumeRole"}]}`

func TestIoTCreateTopicRulePassRoleDenyWrongTrust(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "iot-rule-bad", iotRuleTrustBad, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/iot-rule-bad"

	rec := mustJSONTarget(t, handler, "AWSIotService.CreateTopicRule", "iot", map[string]any{
		"ruleName": "passrole-deny-rule",
		"topicRulePayload": map[string]any{
			"sql": "SELECT * FROM 'lab/passrole'",
			"actions": []any{
				map[string]any{"sqs": map[string]any{
					"queueUrl": "http://127.0.0.1:4566/" + testAccountID + "/q",
					"roleArn":  roleARN,
				}},
			},
		},
	}, now)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("CreateTopicRule status=%d want 403 body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "not authorized to pass role to IoT") {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestIoTCreateTopicRulePassRoleAllow(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "iot-rule-ok", iotRuleTrustOK, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/iot-rule-ok"

	rec := mustJSONTarget(t, handler, "AWSIotService.CreateTopicRule", "iot", map[string]any{
		"ruleName": "passrole-ok-rule",
		"topicRulePayload": map[string]any{
			"sql": "SELECT * FROM 'lab/passrole-ok'",
			"actions": []any{
				map[string]any{"sqs": map[string]any{
					"queueUrl": "http://127.0.0.1:4566/" + testAccountID + "/q",
					"roleArn":  roleARN,
				}},
			},
		},
	}, now)
	if rec.Code != http.StatusOK {
		t.Fatalf("CreateTopicRule status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestIoTReplaceTopicRulePassRoleDenyWrongTrust(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustCreateIAMRole(t, handler, "iot-rule-replace-ok", iotRuleTrustOK, now)
	mustCreateIAMRole(t, handler, "iot-rule-replace-bad", iotRuleTrustBad, now)
	okARN := "arn:aws:iam::" + testAccountID + ":role/iot-rule-replace-ok"
	badARN := "arn:aws:iam::" + testAccountID + ":role/iot-rule-replace-bad"

	create := mustJSONTarget(t, handler, "AWSIotService.CreateTopicRule", "iot", map[string]any{
		"ruleName": "passrole-replace",
		"topicRulePayload": map[string]any{
			"sql": "SELECT * FROM 'lab/replace'",
			"actions": []any{
				map[string]any{"sqs": map[string]any{
					"queueUrl": "http://127.0.0.1:4566/" + testAccountID + "/q",
					"roleArn":  okARN,
				}},
			},
		},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateTopicRule status=%d body=%q", create.Code, create.Body.String())
	}

	replace := mustJSONTarget(t, handler, "AWSIotService.ReplaceTopicRule", "iot", map[string]any{
		"ruleName": "passrole-replace",
		"topicRulePayload": map[string]any{
			"sql": "SELECT * FROM 'lab/replace2'",
			"actions": []any{
				map[string]any{"sqs": map[string]any{
					"queueUrl": "http://127.0.0.1:4566/" + testAccountID + "/q2",
					"roleArn":  badARN,
				}},
			},
		},
	}, now)
	if replace.Code != http.StatusForbidden {
		t.Fatalf("ReplaceTopicRule status=%d want 403 body=%q", replace.Code, replace.Body.String())
	}
	if !strings.Contains(replace.Body.String(), "not authorized to pass role to IoT") {
		t.Fatalf("body=%q", replace.Body.String())
	}
}
