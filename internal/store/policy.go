package store

import "fmt"

// PutPolicy stores or replaces an identity policy document by policy ID.
func (s *Store) PutPolicy(policyID, document string) error {
	_, err := s.db.Exec(
		`INSERT INTO policies (policy_id, document) VALUES (?, ?)
		 ON CONFLICT(policy_id) DO UPDATE SET document = excluded.document`,
		policyID, document,
	)
	if err != nil {
		return fmt.Errorf("put policy %s: %w", policyID, err)
	}
	return nil
}

// AttachPolicy attaches a stored policy to a principal ARN.
func (s *Store) AttachPolicy(principalARN, policyID string) error {
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO policy_attachments (principal_arn, policy_id) VALUES (?, ?)`,
		principalARN, policyID,
	)
	if err != nil {
		return fmt.Errorf("attach policy %s to %s: %w", policyID, principalARN, err)
	}
	return nil
}

// ListAttachedPolicyDocuments returns policy documents attached to principalARN.
func (s *Store) ListAttachedPolicyDocuments(principalARN string) ([]string, error) {
	rows, err := s.db.Query(
		`SELECT p.document
		 FROM policy_attachments a
		 JOIN policies p ON p.policy_id = a.policy_id
		 WHERE a.principal_arn = ?
		 ORDER BY a.policy_id`,
		principalARN,
	)
	if err != nil {
		return nil, fmt.Errorf("list attached policies for %s: %w", principalARN, err)
	}
	defer rows.Close()

	var docs []string
	for rows.Next() {
		var doc string
		if err := rows.Scan(&doc); err != nil {
			return nil, fmt.Errorf("list attached policies for %s: %w", principalARN, err)
		}
		docs = append(docs, doc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list attached policies for %s: %w", principalARN, err)
	}
	if docs == nil {
		docs = []string{}
	}
	return docs, nil
}

// AttachedPolicyRef is a managed policy attachment listing entry.
type AttachedPolicyRef struct {
	PolicyARN  string
	PolicyName string
}

// ListAttachedPolicyRefs returns attached managed policy ARNs for principalARN.
func (s *Store) ListAttachedPolicyRefs(principalARN string) ([]AttachedPolicyRef, error) {
	rows, err := s.db.Query(
		`SELECT a.policy_id, COALESCE(p.policy_name, '')
		 FROM policy_attachments a
		 LEFT JOIN policies p ON p.policy_id = a.policy_id
		 WHERE a.principal_arn = ?
		 ORDER BY a.policy_id`,
		principalARN,
	)
	if err != nil {
		return nil, fmt.Errorf("list attached policy refs for %s: %w", principalARN, err)
	}
	defer rows.Close()

	var out []AttachedPolicyRef
	for rows.Next() {
		var ref AttachedPolicyRef
		if err := rows.Scan(&ref.PolicyARN, &ref.PolicyName); err != nil {
			return nil, fmt.Errorf("list attached policy refs for %s: %w", principalARN, err)
		}
		if ref.PolicyName == "" {
			ref.PolicyName = policyNameFromARN(ref.PolicyARN)
		}
		out = append(out, ref)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list attached policy refs for %s: %w", principalARN, err)
	}
	if out == nil {
		out = []AttachedPolicyRef{}
	}
	return out, nil
}

func policyNameFromARN(arn string) string {
	for i := len(arn) - 1; i >= 0; i-- {
		if arn[i] == '/' {
			return arn[i+1:]
		}
	}
	return arn
}
