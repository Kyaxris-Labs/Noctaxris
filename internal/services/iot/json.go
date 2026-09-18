package iot

import (
	"encoding/json"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func thingMap(t store.IoTThing) map[string]any {
	return map[string]any{
		"thingName":  t.ThingName,
		"thingArn":   t.ThingARN,
		"attributes": t.Attributes,
		"version":    t.Version,
	}
}

// CreateThingJSON builds CreateThing response.
func CreateThingJSON(t store.IoTThing) ([]byte, error) {
	return json.Marshal(map[string]any{
		"thingName": t.ThingName,
		"thingArn":  t.ThingARN,
	})
}

// DescribeThingJSON builds DescribeThing response.
func DescribeThingJSON(t store.IoTThing) ([]byte, error) {
	return json.Marshal(thingMap(t))
}

// ListThingsJSON builds ListThings response.
func ListThingsJSON(things []store.IoTThing) ([]byte, error) {
	items := make([]map[string]any, 0, len(things))
	for _, t := range things {
		items = append(items, map[string]any{
			"thingName": t.ThingName,
			"thingArn":  t.ThingARN,
		})
	}
	return json.Marshal(map[string]any{"things": items})
}

// UpdateThingJSON builds UpdateThing response.
func UpdateThingJSON(t store.IoTThing) ([]byte, error) {
	return json.Marshal(map[string]any{
		"thingName": t.ThingName,
		"thingArn":  t.ThingARN,
		"version":   t.Version,
	})
}

// EmptyJSON is {}.
func EmptyJSON() ([]byte, error) { return []byte(`{}`), nil }

// CreateKeysAndCertificateJSON builds CreateKeysAndCertificate response.
func CreateKeysAndCertificateJSON(c store.IoTCertificate) ([]byte, error) {
	return json.Marshal(map[string]any{
		"certificateArn": c.CertificateARN,
		"certificateId":  c.CertificateID,
		"certificatePem": c.CertificatePEM,
		"keyPair": map[string]string{
			"PublicKey":  c.PublicKey,
			"PrivateKey": c.PrivateKey,
		},
	})
}

// DescribeCertificateJSON builds DescribeCertificate response.
func DescribeCertificateJSON(c store.IoTCertificate) ([]byte, error) {
	return json.Marshal(map[string]any{
		"certificateDescription": map[string]any{
			"certificateArn": c.CertificateARN,
			"certificateId":  c.CertificateID,
			"status":         c.Status,
			"certificatePem": c.CertificatePEM,
			"creationDate":   float64(c.CreatedAt) / 1000.0,
		},
	})
}

// ListCertificatesJSON builds ListCertificates response.
func ListCertificatesJSON(certs []store.IoTCertificate) ([]byte, error) {
	items := make([]map[string]any, 0, len(certs))
	for _, c := range certs {
		items = append(items, map[string]any{
			"certificateArn": c.CertificateARN,
			"certificateId":  c.CertificateID,
			"status":         c.Status,
			"creationDate":   float64(c.CreatedAt) / 1000.0,
		})
	}
	return json.Marshal(map[string]any{"certificates": items})
}

// CreatePolicyJSON builds CreatePolicy response.
func CreatePolicyJSON(p store.IoTPolicy) ([]byte, error) {
	return json.Marshal(map[string]any{
		"policyName":      p.PolicyName,
		"policyArn":       p.PolicyARN,
		"policyDocument":  p.PolicyDocument,
		"policyVersionId": "1",
	})
}

// GetPolicyJSON builds GetPolicy response.
func GetPolicyJSON(p store.IoTPolicy) ([]byte, error) {
	return CreatePolicyJSON(p)
}

// ListPoliciesJSON builds ListPolicies response.
func ListPoliciesJSON(policies []store.IoTPolicy) ([]byte, error) {
	items := make([]map[string]any, 0, len(policies))
	for _, p := range policies {
		items = append(items, map[string]any{
			"policyName": p.PolicyName,
			"policyArn":  p.PolicyARN,
		})
	}
	return json.Marshal(map[string]any{"policies": items})
}

// ListThingPrincipalsJSON builds ListThingPrincipals response.
func ListThingPrincipalsJSON(principals []string) ([]byte, error) {
	if principals == nil {
		principals = []string{}
	}
	return json.Marshal(map[string]any{"principals": principals})
}

func topicRuleActionsAny(actionsJSON string) any {
	var actions any
	if err := json.Unmarshal([]byte(actionsJSON), &actions); err != nil || actions == nil {
		return []any{}
	}
	return actions
}

// CreateTopicRuleJSON builds CreateTopicRule / ReplaceTopicRule response.
func CreateTopicRuleJSON(r store.IoTTopicRule) ([]byte, error) {
	return json.Marshal(map[string]any{
		"ruleArn":  r.RuleARN,
		"ruleName": r.RuleName,
	})
}

// GetTopicRuleJSON builds GetTopicRule response.
func GetTopicRuleJSON(r store.IoTTopicRule) ([]byte, error) {
	return json.Marshal(map[string]any{
		"ruleArn": r.RuleARN,
		"rule": map[string]any{
			"ruleName":     r.RuleName,
			"sql":          r.SQL,
			"description":  r.Description,
			"ruleDisabled": r.RuleDisabled,
			"actions":      topicRuleActionsAny(r.ActionsJSON),
			"createdAt":    float64(r.CreatedAt) / 1000.0,
		},
	})
}

// ListTopicRulesJSON builds ListTopicRules response.
func ListTopicRulesJSON(rules []store.IoTTopicRule) ([]byte, error) {
	items := make([]map[string]any, 0, len(rules))
	for _, r := range rules {
		items = append(items, map[string]any{
			"ruleArn":      r.RuleARN,
			"ruleName":     r.RuleName,
			"topicPattern": store.ExtractIoTTopicFilter(r.SQL),
			"ruleDisabled": r.RuleDisabled,
			"createdAt":    float64(r.CreatedAt) / 1000.0,
		})
	}
	return json.Marshal(map[string]any{"rules": items})
}

// DescribeEndpointJSON builds DescribeEndpoint response.
func DescribeEndpointJSON(address string) ([]byte, error) {
	return json.Marshal(map[string]any{"endpointAddress": address})
}

// ListNamedShadowsJSON builds ListNamedShadowsForThing response.
func ListNamedShadowsJSON(names []string, ts int64) ([]byte, error) {
	if names == nil {
		names = []string{}
	}
	return json.Marshal(map[string]any{
		"results":   names,
		"timestamp": ts,
	})
}

// CreateRoleAliasJSON builds CreateRoleAlias / DescribeRoleAlias response.
func CreateRoleAliasJSON(ra store.IoTRoleAlias) ([]byte, error) {
	return json.Marshal(map[string]any{
		"roleAlias":                 ra.RoleAlias,
		"roleAliasArn":              ra.RoleAliasARN,
		"roleArn":                   ra.RoleARN,
		"credentialDurationSeconds": ra.CredentialDurationSeconds,
	})
}

// ListRoleAliasesJSON builds ListRoleAliases response.
func ListRoleAliasesJSON(aliases []store.IoTRoleAlias) ([]byte, error) {
	items := make([]map[string]any, 0, len(aliases))
	for _, ra := range aliases {
		items = append(items, map[string]any{
			"roleAlias":    ra.RoleAlias,
			"roleAliasArn": ra.RoleAliasARN,
			"roleArn":      ra.RoleARN,
		})
	}
	return json.Marshal(map[string]any{"roleAliases": items})
}

// CreateJobJSON builds CreateJob response.
func CreateJobJSON(j store.IoTJob) ([]byte, error) {
	return json.Marshal(map[string]any{
		"jobId":  j.JobID,
		"jobArn": j.JobARN,
	})
}

// DescribeJobJSON builds DescribeJob response.
func DescribeJobJSON(j store.IoTJob) ([]byte, error) {
	return json.Marshal(map[string]any{
		"job": map[string]any{
			"jobId":     j.JobID,
			"jobArn":    j.JobARN,
			"status":    j.Status,
			"targets":   j.Targets,
			"document":  j.Document,
			"createdAt": float64(j.CreatedAt),
		},
	})
}

func jobExecutionSummary(ex store.IoTJobExecution) map[string]any {
	m := map[string]any{
		"jobId":           ex.JobID,
		"executionNumber": ex.ExecutionNumber,
		"versionNumber":   ex.VersionNumber,
		"queuedAt":        ex.QueuedAt,
		"lastUpdatedAt":   ex.LastUpdatedAt,
	}
	if ex.StartedAt > 0 {
		m["startedAt"] = ex.StartedAt
	}
	return m
}

// GetPendingJobExecutionsJSON builds GetPendingJobExecutions response.
func GetPendingJobExecutionsJSON(inProgress, queued []store.IoTJobExecution) ([]byte, error) {
	inItems := make([]map[string]any, 0, len(inProgress))
	for _, ex := range inProgress {
		inItems = append(inItems, jobExecutionSummary(ex))
	}
	qItems := make([]map[string]any, 0, len(queued))
	for _, ex := range queued {
		qItems = append(qItems, jobExecutionSummary(ex))
	}
	return json.Marshal(map[string]any{
		"inProgressJobs": inItems,
		"queuedJobs":     qItems,
	})
}

// DescribeJobExecutionJSON builds DescribeJobExecution / StartNextPendingJobExecution response.
func DescribeJobExecutionJSON(ex store.IoTJobExecution, includeDocument bool) ([]byte, error) {
	exec := map[string]any{
		"jobId":           ex.JobID,
		"thingName":       ex.ThingName,
		"status":          ex.Status,
		"executionNumber": ex.ExecutionNumber,
		"versionNumber":   ex.VersionNumber,
		"queuedAt":        ex.QueuedAt,
		"lastUpdatedAt":   ex.LastUpdatedAt,
		"statusDetails":   ex.StatusDetails,
	}
	if ex.StartedAt > 0 {
		exec["startedAt"] = ex.StartedAt
	}
	if includeDocument {
		exec["jobDocument"] = ex.JobDocument
	}
	return json.Marshal(map[string]any{"execution": exec})
}

// CredentialsProviderJSON builds GET /role-aliases/{alias}/credentials response.
func CredentialsProviderJSON(accessKeyID, secret, sessionToken string, expiration time.Time) ([]byte, error) {
	return json.Marshal(map[string]any{
		"credentials": map[string]any{
			"accessKeyId":     accessKeyID,
			"secretAccessKey": secret,
			"sessionToken":    sessionToken,
			"expiration":      expiration.UTC().Format(time.RFC3339),
		},
	})
}

// ListRetainedMessagesJSON builds an empty retained-message list.
func ListRetainedMessagesJSON(topics []string) ([]byte, error) {
	if topics == nil {
		topics = []string{}
	}
	return json.Marshal(map[string]any{"retainedTopics": topics})
}
