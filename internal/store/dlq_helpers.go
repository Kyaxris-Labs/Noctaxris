package store

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
)

// labDLQServicePrincipals are the lab services that deliver failure payloads via sendLabDLQMessage.
// Callers do not pass a principal; foreign DLQ Policy must Allow at least one of these (or fail closed).
var labDLQServicePrincipals = []string{
	authz.ServicePrincipalEvents,
	authz.ServicePrincipalSNS,
	authz.ServicePrincipalPipes,
}

// sendLabDLQMessage delivers a failure payload to an SQS dead-letter queue ARN.
// Same-account delivery mirrors SQS redrive (RedriveAllowPolicy when set). Foreign queues require
// a destination Policy that Allows a lab DLQ service principal (fail closed; no open proxy).
func (s *Store) sendLabDLQMessage(ownerAccount, dlqARN string, body []byte) error {
	dlqARN = strings.TrimSpace(dlqARN)
	if dlqARN == "" {
		return fmt.Errorf("dlq arn is required")
	}
	queueAccount, queueName, err := s.resolveSQSEndpoint(ownerAccount, dlqARN)
	if err != nil {
		return err
	}
	q, err := s.GetQueue(queueAccount, queueName)
	if err != nil {
		return err
	}
	if err := s.labDLQDeliveryAllowed(ownerAccount, q); err != nil {
		return err
	}
	_, err = s.SendMessage(queueAccount, queueName, body, false, nil, "", nil)
	return err
}

func (s *Store) labDLQDeliveryAllowed(ownerAccount string, q Queue) error {
	allow, hasAllow := parseRedriveAllowPolicy(q.Attributes)
	// Lab DLQ is not an SQS source queue ARN; byQueue / denyAll therefore deny (allowsSource("")).
	if hasAllow && !allow.allowsSource("") {
		return fmt.Errorf("redrive not allowed by dead-letter queue RedriveAllowPolicy")
	}
	if q.AccountID == ownerAccount {
		return nil
	}
	for _, sp := range labDLQServicePrincipals {
		if s.deliveryTargetResourcePolicyAllows(ownerAccount, q.QueueARN, actionSQSSendMessage, sp, "") {
			return nil
		}
	}
	return fmt.Errorf("lab dlq: queue policy does not Allow delivery")
}

func parseSNSRedrivePolicy(attrs map[string]string) (string, bool) {
	if attrs == nil {
		return "", false
	}
	raw := strings.TrimSpace(attrs["RedrivePolicy"])
	if raw == "" {
		return "", false
	}
	var parsed struct {
		DeadLetterTargetArn string `json:"deadLetterTargetArn"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return "", false
	}
	arn := strings.TrimSpace(parsed.DeadLetterTargetArn)
	if arn == "" {
		return "", false
	}
	return arn, true
}
