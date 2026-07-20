package store

import (
	"database/sql"
	"fmt"
	"strings"
)

const taggingSchema = `
CREATE TABLE IF NOT EXISTS resource_tags (
  account_id TEXT NOT NULL,
  resource_arn TEXT NOT NULL,
  tag_key TEXT NOT NULL,
  tag_value TEXT NOT NULL,
  PRIMARY KEY (account_id, resource_arn, tag_key)
);
CREATE INDEX IF NOT EXISTS idx_resource_tags_arn ON resource_tags(account_id, resource_arn);
`

// ResourceTag is one key/value on a resource ARN.
type ResourceTag struct {
	Key   string
	Value string
}

// TaggedResource is a resource ARN with its tags.
type TaggedResource struct {
	ResourceARN string
	Tags        []ResourceTag
}

// TagFilter matches resources that have the key (and optional value set).
type TagFilter struct {
	Key    string
	Values []string
}

// EnsureTaggingSchema creates the resource_tags table if missing.
func EnsureTaggingSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure tagging schema: db is nil")
	}
	if _, err := db.Exec(taggingSchema); err != nil {
		return fmt.Errorf("ensure tagging schema: %w", err)
	}
	return nil
}

// EnsureTaggingSchema ensures tagging tables on an open store.
func (s *Store) EnsureTaggingSchema() error {
	return EnsureTaggingSchema(s.db)
}

// TagResources sets tags on the given ARNs for the account. Returns failed ARNs with reason.
func (s *Store) TagResources(accountID string, resourceARNs []string, tags map[string]string) (failed map[string]string, err error) {
	failed = map[string]string{}
	if len(tags) == 0 {
		return failed, fmt.Errorf("tag resources: tags required")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("tag resources: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, arn := range resourceARNs {
		arn = strings.TrimSpace(arn)
		if arn == "" {
			continue
		}
		if !resourceARNBelongsToAccount(arn, accountID) {
			failed[arn] = "InvalidParameterException: resource ARN account does not match caller"
			continue
		}
		for k, v := range tags {
			k = strings.TrimSpace(k)
			if k == "" {
				failed[arn] = "InvalidParameterException: tag key required"
				continue
			}
			_, err := tx.Exec(
				`INSERT INTO resource_tags (account_id, resource_arn, tag_key, tag_value)
				 VALUES (?, ?, ?, ?)
				 ON CONFLICT(account_id, resource_arn, tag_key) DO UPDATE SET tag_value = excluded.tag_value`,
				accountID, arn, k, v,
			)
			if err != nil {
				return nil, fmt.Errorf("tag resources: upsert: %w", err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("tag resources: commit: %w", err)
	}
	return failed, nil
}

// UntagResources removes tag keys from the given ARNs.
func (s *Store) UntagResources(accountID string, resourceARNs, tagKeys []string) (failed map[string]string, err error) {
	failed = map[string]string{}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("untag resources: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, arn := range resourceARNs {
		arn = strings.TrimSpace(arn)
		if arn == "" {
			continue
		}
		if !resourceARNBelongsToAccount(arn, accountID) {
			failed[arn] = "InvalidParameterException: resource ARN account does not match caller"
			continue
		}
		for _, k := range tagKeys {
			k = strings.TrimSpace(k)
			if k == "" {
				continue
			}
			_, err := tx.Exec(
				`DELETE FROM resource_tags WHERE account_id = ? AND resource_arn = ? AND tag_key = ?`,
				accountID, arn, k,
			)
			if err != nil {
				return nil, fmt.Errorf("untag resources: delete: %w", err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("untag resources: commit: %w", err)
	}
	return failed, nil
}

// ListResourceTags returns tags for one resource ARN in the account.
func (s *Store) ListResourceTags(accountID, resourceARN string) ([]ResourceTag, error) {
	resourceARN = strings.TrimSpace(resourceARN)
	if accountID == "" || resourceARN == "" {
		return nil, nil
	}
	rows, err := s.db.Query(
		`SELECT tag_key, tag_value FROM resource_tags
		 WHERE account_id = ? AND resource_arn = ?
		 ORDER BY tag_key`,
		accountID, resourceARN,
	)
	if err != nil {
		return nil, fmt.Errorf("list resource tags: %w", err)
	}
	defer rows.Close()

	var out []ResourceTag
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("list resource tags: scan: %w", err)
		}
		out = append(out, ResourceTag{Key: k, Value: v})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// GetResources returns tagged resources filtered by tag filters and resource type filters.
func (s *Store) GetResources(
	accountID string, tagFilters []TagFilter, resourceTypeFilters []string,
) ([]TaggedResource, error) {
	rows, err := s.db.Query(
		`SELECT resource_arn, tag_key, tag_value FROM resource_tags WHERE account_id = ? ORDER BY resource_arn, tag_key`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("get resources: %w", err)
	}
	defer rows.Close()

	byARN := map[string][]ResourceTag{}
	var order []string
	for rows.Next() {
		var arn, k, v string
		if err := rows.Scan(&arn, &k, &v); err != nil {
			return nil, fmt.Errorf("get resources: scan: %w", err)
		}
		if _, ok := byARN[arn]; !ok {
			order = append(order, arn)
		}
		byARN[arn] = append(byARN[arn], ResourceTag{Key: k, Value: v})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var out []TaggedResource
	for _, arn := range order {
		tags := byARN[arn]
		if !resourceTypeMatches(arn, resourceTypeFilters) {
			continue
		}
		if !tagFiltersMatch(tags, tagFilters) {
			continue
		}
		out = append(out, TaggedResource{ResourceARN: arn, Tags: tags})
	}
	return out, nil
}

func resourceARNBelongsToAccount(arn, accountID string) bool {
	// arn:aws:service:region:account:resource...
	parts := strings.SplitN(arn, ":", 6)
	if len(parts) < 5 {
		return false
	}
	acct := parts[4]
	if acct == "" {
		// S3 bucket ARNs omit account: arn:aws:s3:::bucket — accept for lab caller account.
		return strings.HasPrefix(arn, "arn:aws:s3:::")
	}
	return acct == accountID
}

func resourceTypeMatches(arn string, filters []string) bool {
	if len(filters) == 0 {
		return true
	}
	for _, f := range filters {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		// Accept either "s3" / "sqs" service or "s3:bucket" style.
		if strings.Contains(strings.ToLower(arn), strings.ToLower(f)) {
			return true
		}
		parts := strings.SplitN(arn, ":", 6)
		if len(parts) >= 3 && strings.EqualFold(parts[2], f) {
			return true
		}
		if len(parts) >= 6 {
			res := parts[5]
			if strings.EqualFold(parts[2]+":"+strings.SplitN(res, "/", 2)[0], f) {
				return true
			}
		}
	}
	return false
}

func tagFiltersMatch(tags []ResourceTag, filters []TagFilter) bool {
	if len(filters) == 0 {
		return true
	}
	byKey := map[string]string{}
	for _, t := range tags {
		byKey[t.Key] = t.Value
	}
	for _, f := range filters {
		val, ok := byKey[f.Key]
		if !ok {
			return false
		}
		if len(f.Values) == 0 {
			continue
		}
		matched := false
		for _, want := range f.Values {
			if val == want {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}
