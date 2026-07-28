package store

import (
	"fmt"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/identity"
)

// EvaluateIoTDevicePolicy evaluates the union of IoT policies attached to the certificate ARN.
// Actions are iot:Connect, iot:Publish, iot:Subscribe, and iot:Receive. Deny overrides Allow;
// no matching Allow yields false (fail closed). MQTT clientId is not used.
func (s *Store) EvaluateIoTDevicePolicy(accountID, region, certificateID, action, resource string) bool {
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
	ctx := authz.RequestContext{
		Principal: identity.Principal{AccountID: accountID},
		Action:    action,
		Resource:  resource,
		Region:    iotRegion(region),
	}
	return authz.Evaluate(ctx, docs) == authz.Allow
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
