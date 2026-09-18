package store

import (
	"fmt"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/identity"
)

// EvaluateIoTDevicePolicy evaluates the union of IoT policies attached to the certificate ARN.
// Actions are iot:Connect, iot:Publish, iot:Subscribe, iot:Receive, and data-plane IoT actions.
// Deny overrides Allow; no matching Allow yields false (fail closed).
// Policy variables ${iot:Connection.Thing.ThingName} substitute the attached thing name.
func (s *Store) EvaluateIoTDevicePolicy(accountID, region, certificateID, action, resource string) bool {
	return s.EvaluateIoTDevicePolicyWithClientID(accountID, region, certificateID, "", action, resource)
}

// EvaluateIoTDevicePolicyWithClientID is EvaluateIoTDevicePolicy plus ${iot:ClientId}.
func (s *Store) EvaluateIoTDevicePolicyWithClientID(accountID, region, certificateID, clientID, action, resource string) bool {
	action = strings.TrimSpace(action)
	if action == "" {
		return false
	}
	c, err := s.DescribeIoTCertificate(accountID, region, certificateID)
	if err != nil {
		return false
	}
	docs, err := s.listIoTPolicyDocumentsForTarget(accountID, region, c.CertificateARN)
	if err != nil || len(docs) == 0 {
		return false
	}
	thingName := ""
	if dev, err := s.ResolveIoTMQTTDeviceByCertificate(certificateID); err == nil {
		thingName = dev.ThingName
	}
	interpolated := interpolateIoTPolicyDocs(docs, thingName, clientID)
	ctx := authz.RequestContext{
		Principal: identity.Principal{AccountID: accountID},
		Action:    action,
		Resource:  resource,
		Region:    iotRegion(region),
	}
	return authz.Evaluate(ctx, interpolated) == authz.Allow
}

// AllowMQTTConnect reports whether an MQTT CONNECT is allowed for the device certificate.
// clientId must equal the attached thing name. Filename-shaped clientIds are denied.
func (s *Store) AllowMQTTConnect(certificateID, clientID string) bool {
	clientID = strings.TrimSpace(clientID)
	if clientID == "" || mqttClientIDLooksLikeFilename(clientID) {
		return false
	}
	dev, err := s.ResolveIoTMQTTDeviceByCertificate(certificateID)
	if err != nil {
		return false
	}
	if clientID != dev.ThingName {
		return false
	}
	resource := IoTClientARN(dev.Region, dev.AccountID, clientID)
	return s.EvaluateIoTDevicePolicyWithClientID(
		dev.AccountID, dev.Region, dev.CertificateID, clientID, "iot:Connect", resource,
	)
}

func mqttClientIDLooksLikeFilename(clientID string) bool {
	lower := strings.ToLower(clientID)
	if strings.ContainsAny(clientID, `/\`) {
		return true
	}
	for _, suf := range []string{".pem", ".crt", ".cer", ".key", ".pub", ".p12", ".pfx"} {
		if strings.HasSuffix(lower, suf) {
			return true
		}
	}
	return false
}

func interpolateIoTPolicyDocs(docs []string, thingName, clientID string) []string {
	out := make([]string, len(docs))
	for i, doc := range docs {
		s := doc
		if thingName != "" {
			s = strings.ReplaceAll(s, "${iot:Connection.Thing.ThingName}", thingName)
		}
		if clientID != "" {
			s = strings.ReplaceAll(s, "${iot:ClientId}", clientID)
		}
		out[i] = s
	}
	return out
}

func (s *Store) listIoTPolicyDocumentsForTarget(accountID, region, targetARN string) ([]string, error) {
	if err := s.EnsureIoTSchema(); err != nil {
		return nil, err
	}
	region = iotRegion(region)
	targetARN = strings.TrimSpace(targetARN)
	if targetARN == "" {
		return nil, fmt.Errorf("%w: target required", ErrIoTBadRequest)
	}
	rows, err := s.db.Query(
		`SELECT p.policy_document
		 FROM iot_policy_attachments a
		 JOIN iot_policies p ON p.account_id = a.account_id AND p.region = a.region AND p.policy_name = a.policy_name
		 WHERE a.account_id = ? AND a.region = ? AND a.target = ?`,
		accountID, region, targetARN,
	)
	if err != nil {
		return nil, fmt.Errorf("list iot policy documents: %w", err)
	}
	defer rows.Close()
	var docs []string
	for rows.Next() {
		var doc string
		if err := rows.Scan(&doc); err != nil {
			return nil, fmt.Errorf("list iot policy documents scan: %w", err)
		}
		docs = append(docs, doc)
	}
	return docs, rows.Err()
}
