package store

import (
	"encoding/json"
	"fmt"
	"strings"
)

// sendLabDLQMessage delivers a failure payload to an SQS dead-letter queue ARN.
func (s *Store) sendLabDLQMessage(ownerAccount, dlqARN string, body []byte) error {
	dlqARN = strings.TrimSpace(dlqARN)
	if dlqARN == "" {
		return fmt.Errorf("dlq arn is required")
	}
	queueAccount, queueName, err := s.resolveSQSEndpoint(ownerAccount, dlqARN)
	if err != nil {
		return err
	}
	_, err = s.SendMessage(queueAccount, queueName, body, false, nil, "", nil)
	return err
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
