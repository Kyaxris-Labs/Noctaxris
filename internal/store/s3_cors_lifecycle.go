package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrNoSuchCORSConfiguration       = errors.New("NoSuchCORSConfiguration")
	ErrNoSuchLifecycleConfiguration  = errors.New("NoSuchLifecycleConfiguration")
	ErrInvalidCORSConfiguration      = errors.New("InvalidRequest")
	ErrInvalidLifecycleConfiguration = errors.New("InvalidRequest")
)

// S3CORSRule is one CORS rule persisted on a bucket.
type S3CORSRule struct {
	ID             string   `json:"id,omitempty"`
	AllowedOrigins []string `json:"allowedOrigins"`
	AllowedMethods []string `json:"allowedMethods"`
	AllowedHeaders []string `json:"allowedHeaders,omitempty"`
	ExposeHeaders  []string `json:"exposeHeaders,omitempty"`
	MaxAgeSeconds  *int     `json:"maxAgeSeconds,omitempty"`
}

// S3CORSConfiguration is the CORS configuration for a bucket.
type S3CORSConfiguration struct {
	Rules []S3CORSRule `json:"rules"`
}

// S3LifecycleExpiration is an Expiration action on a lifecycle rule.
type S3LifecycleExpiration struct {
	Days                      *int   `json:"days,omitempty"`
	Date                      string `json:"date,omitempty"`
	ExpiredObjectDeleteMarker *bool  `json:"expiredObjectDeleteMarker,omitempty"`
}

// S3LifecycleTransition is a Transition action on a lifecycle rule.
type S3LifecycleTransition struct {
	Days         *int   `json:"days,omitempty"`
	Date         string `json:"date,omitempty"`
	StorageClass string `json:"storageClass,omitempty"`
}

// S3LifecycleAbortIncompleteMultipartUpload cancels incomplete multipart uploads.
type S3LifecycleAbortIncompleteMultipartUpload struct {
	DaysAfterInitiation *int `json:"daysAfterInitiation,omitempty"`
}

// S3LifecycleNoncurrentVersionExpiration expires noncurrent versions.
type S3LifecycleNoncurrentVersionExpiration struct {
	NoncurrentDays          *int `json:"noncurrentDays,omitempty"`
	NewerNoncurrentVersions *int `json:"newerNoncurrentVersions,omitempty"`
}

// S3LifecycleFilter is a lifecycle rule Filter (prefix and/or tag lite).
type S3LifecycleFilter struct {
	Prefix string `json:"prefix,omitempty"`
	TagKey string `json:"tagKey,omitempty"`
	TagVal string `json:"tagValue,omitempty"`
}

// S3LifecycleRule is one lifecycle rule persisted on a bucket (config only; no sweeper).
type S3LifecycleRule struct {
	ID                             string                                    `json:"id,omitempty"`
	Status                         string                                    `json:"status"`
	Prefix                         string                                    `json:"prefix,omitempty"`
	Filter                         *S3LifecycleFilter                        `json:"filter,omitempty"`
	Expiration                     *S3LifecycleExpiration                    `json:"expiration,omitempty"`
	Transitions                    []S3LifecycleTransition                   `json:"transitions,omitempty"`
	AbortIncompleteMultipartUpload *S3LifecycleAbortIncompleteMultipartUpload `json:"abortIncompleteMultipartUpload,omitempty"`
	NoncurrentVersionExpiration    *S3LifecycleNoncurrentVersionExpiration   `json:"noncurrentVersionExpiration,omitempty"`
}

// S3LifecycleConfiguration is the lifecycle configuration for a bucket.
type S3LifecycleConfiguration struct {
	Rules []S3LifecycleRule `json:"rules"`
}

// EnsureS3CORSLifecycleSchema adds CORS and lifecycle JSON columns on s3_buckets.
func EnsureS3CORSLifecycleSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure s3 cors lifecycle schema: db is nil")
	}
	stmts := []string{
		`ALTER TABLE s3_buckets ADD COLUMN cors_json TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE s3_buckets ADD COLUMN lifecycle_json TEXT NOT NULL DEFAULT ''`,
	}
	for _, stmt := range stmts {
		if err := execMigrateStmt(db, stmt, nil); err != nil {
			return fmt.Errorf("ensure s3 cors lifecycle schema: %w", err)
		}
	}
	return nil
}

// EnsureS3CORSLifecycleSchema ensures CORS/lifecycle columns on an open store.
func (s *Store) EnsureS3CORSLifecycleSchema() error {
	return EnsureS3CORSLifecycleSchema(s.db)
}

func normalizeS3CORSStringList(in []string) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		out = append(out, v)
	}
	return out
}

func validateCORSConfiguration(cfg S3CORSConfiguration) (S3CORSConfiguration, error) {
	if len(cfg.Rules) == 0 {
		return S3CORSConfiguration{}, fmt.Errorf("%w: CORSConfiguration must contain at least one CORSRule", ErrInvalidCORSConfiguration)
	}
	if len(cfg.Rules) > 100 {
		return S3CORSConfiguration{}, fmt.Errorf("%w: CORSConfiguration may contain at most 100 CORSRule entries", ErrInvalidCORSConfiguration)
	}
	allowedMethods := map[string]struct{}{
		"GET": {}, "PUT": {}, "POST": {}, "DELETE": {}, "HEAD": {},
	}
	out := S3CORSConfiguration{Rules: make([]S3CORSRule, 0, len(cfg.Rules))}
	for i, rule := range cfg.Rules {
		origins := normalizeS3CORSStringList(rule.AllowedOrigins)
		methods := normalizeS3CORSStringList(rule.AllowedMethods)
		if len(origins) == 0 {
			return S3CORSConfiguration{}, fmt.Errorf("%w: CORSRule %d requires AllowedOrigin", ErrInvalidCORSConfiguration, i)
		}
		if len(methods) == 0 {
			return S3CORSConfiguration{}, fmt.Errorf("%w: CORSRule %d requires AllowedMethod", ErrInvalidCORSConfiguration, i)
		}
		for _, m := range methods {
			if _, ok := allowedMethods[strings.ToUpper(m)]; !ok {
				return S3CORSConfiguration{}, fmt.Errorf("%w: unsupported AllowedMethod %q", ErrInvalidCORSConfiguration, m)
			}
		}
		normalizedMethods := make([]string, len(methods))
		for j, m := range methods {
			normalizedMethods[j] = strings.ToUpper(m)
		}
		out.Rules = append(out.Rules, S3CORSRule{
			ID:             strings.TrimSpace(rule.ID),
			AllowedOrigins: origins,
			AllowedMethods: normalizedMethods,
			AllowedHeaders: normalizeS3CORSStringList(rule.AllowedHeaders),
			ExposeHeaders:  normalizeS3CORSStringList(rule.ExposeHeaders),
			MaxAgeSeconds:  rule.MaxAgeSeconds,
		})
	}
	return out, nil
}

// PutBucketCors replaces the bucket CORS configuration.
func (s *Store) PutBucketCors(accountID, bucket string, cfg S3CORSConfiguration) error {
	if err := s.EnsureS3CORSLifecycleSchema(); err != nil {
		return err
	}
	if _, err := s.GetBucket(accountID, bucket); err != nil {
		return err
	}
	normalized, err := validateCORSConfiguration(cfg)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(normalized)
	if err != nil {
		return fmt.Errorf("put bucket cors: marshal: %w", err)
	}
	_, err = s.db.Exec(
		`UPDATE s3_buckets SET cors_json = ? WHERE account_id = ? AND name = ?`,
		string(raw), accountID, bucket,
	)
	if err != nil {
		return fmt.Errorf("put bucket cors: %w", err)
	}
	return nil
}

// GetBucketCors returns the CORS configuration or ErrNoSuchCORSConfiguration.
func (s *Store) GetBucketCors(accountID, bucket string) (S3CORSConfiguration, error) {
	if err := s.EnsureS3CORSLifecycleSchema(); err != nil {
		return S3CORSConfiguration{}, err
	}
	if _, err := s.GetBucket(accountID, bucket); err != nil {
		return S3CORSConfiguration{}, err
	}
	var raw string
	err := s.db.QueryRow(
		`SELECT COALESCE(cors_json, '') FROM s3_buckets WHERE account_id = ? AND name = ?`,
		accountID, bucket,
	).Scan(&raw)
	if err != nil {
		return S3CORSConfiguration{}, fmt.Errorf("get bucket cors: %w", err)
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return S3CORSConfiguration{}, ErrNoSuchCORSConfiguration
	}
	var cfg S3CORSConfiguration
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return S3CORSConfiguration{}, fmt.Errorf("get bucket cors: %w", err)
	}
	return cfg, nil
}

// DeleteBucketCors clears the bucket CORS configuration.
func (s *Store) DeleteBucketCors(accountID, bucket string) error {
	if err := s.EnsureS3CORSLifecycleSchema(); err != nil {
		return err
	}
	if _, err := s.GetBucket(accountID, bucket); err != nil {
		return err
	}
	_, err := s.db.Exec(
		`UPDATE s3_buckets SET cors_json = '' WHERE account_id = ? AND name = ?`,
		accountID, bucket,
	)
	if err != nil {
		return fmt.Errorf("delete bucket cors: %w", err)
	}
	return nil
}

func validateLifecycleConfiguration(cfg S3LifecycleConfiguration) (S3LifecycleConfiguration, error) {
	if len(cfg.Rules) == 0 {
		return S3LifecycleConfiguration{}, fmt.Errorf("%w: LifecycleConfiguration must contain at least one Rule", ErrInvalidLifecycleConfiguration)
	}
	if len(cfg.Rules) > 1000 {
		return S3LifecycleConfiguration{}, fmt.Errorf("%w: LifecycleConfiguration may contain at most 1000 Rule entries", ErrInvalidLifecycleConfiguration)
	}
	out := S3LifecycleConfiguration{Rules: make([]S3LifecycleRule, 0, len(cfg.Rules))}
	for i, rule := range cfg.Rules {
		status := strings.TrimSpace(rule.Status)
		if status != "Enabled" && status != "Disabled" {
			return S3LifecycleConfiguration{}, fmt.Errorf("%w: Rule %d Status must be Enabled or Disabled", ErrInvalidLifecycleConfiguration, i)
		}
		hasAction := rule.Expiration != nil ||
			len(rule.Transitions) > 0 ||
			rule.AbortIncompleteMultipartUpload != nil ||
			rule.NoncurrentVersionExpiration != nil
		if !hasAction {
			return S3LifecycleConfiguration{}, fmt.Errorf("%w: Rule %d requires at least one lifecycle action", ErrInvalidLifecycleConfiguration, i)
		}
		out.Rules = append(out.Rules, S3LifecycleRule{
			ID:                             strings.TrimSpace(rule.ID),
			Status:                         status,
			Prefix:                         strings.TrimSpace(rule.Prefix),
			Filter:                         rule.Filter,
			Expiration:                     rule.Expiration,
			Transitions:                    rule.Transitions,
			AbortIncompleteMultipartUpload: rule.AbortIncompleteMultipartUpload,
			NoncurrentVersionExpiration:    rule.NoncurrentVersionExpiration,
		})
	}
	return out, nil
}

// PutLifecycleConfiguration replaces the bucket lifecycle configuration (stored only; no expiry sweeper).
func (s *Store) PutLifecycleConfiguration(accountID, bucket string, cfg S3LifecycleConfiguration) error {
	if err := s.EnsureS3CORSLifecycleSchema(); err != nil {
		return err
	}
	if _, err := s.GetBucket(accountID, bucket); err != nil {
		return err
	}
	normalized, err := validateLifecycleConfiguration(cfg)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(normalized)
	if err != nil {
		return fmt.Errorf("put lifecycle configuration: marshal: %w", err)
	}
	_, err = s.db.Exec(
		`UPDATE s3_buckets SET lifecycle_json = ? WHERE account_id = ? AND name = ?`,
		string(raw), accountID, bucket,
	)
	if err != nil {
		return fmt.Errorf("put lifecycle configuration: %w", err)
	}
	return nil
}

// GetLifecycleConfiguration returns the lifecycle configuration or ErrNoSuchLifecycleConfiguration.
func (s *Store) GetLifecycleConfiguration(accountID, bucket string) (S3LifecycleConfiguration, error) {
	if err := s.EnsureS3CORSLifecycleSchema(); err != nil {
		return S3LifecycleConfiguration{}, err
	}
	if _, err := s.GetBucket(accountID, bucket); err != nil {
		return S3LifecycleConfiguration{}, err
	}
	var raw string
	err := s.db.QueryRow(
		`SELECT COALESCE(lifecycle_json, '') FROM s3_buckets WHERE account_id = ? AND name = ?`,
		accountID, bucket,
	).Scan(&raw)
	if err != nil {
		return S3LifecycleConfiguration{}, fmt.Errorf("get lifecycle configuration: %w", err)
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return S3LifecycleConfiguration{}, ErrNoSuchLifecycleConfiguration
	}
	var cfg S3LifecycleConfiguration
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return S3LifecycleConfiguration{}, fmt.Errorf("get lifecycle configuration: %w", err)
	}
	return cfg, nil
}

// DeleteLifecycleConfiguration clears the bucket lifecycle configuration.
func (s *Store) DeleteLifecycleConfiguration(accountID, bucket string) error {
	if err := s.EnsureS3CORSLifecycleSchema(); err != nil {
		return err
	}
	if _, err := s.GetBucket(accountID, bucket); err != nil {
		return err
	}
	_, err := s.db.Exec(
		`UPDATE s3_buckets SET lifecycle_json = '' WHERE account_id = ? AND name = ?`,
		accountID, bucket,
	)
	if err != nil {
		return fmt.Errorf("delete lifecycle configuration: %w", err)
	}
	return nil
}
