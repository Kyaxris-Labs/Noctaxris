package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrCloudFrontNotFound   = errors.New("NoSuchDistribution")
	ErrCloudFrontBadRequest = errors.New("InvalidArgument")
	ErrCloudFrontExists     = errors.New("DistributionAlreadyExists")
)

const DefaultCloudFrontRegion = "us-east-1"

// CloudFrontStatusInProgress is the control-plane stub status until a fake-edge path exists.
const CloudFrontStatusInProgress = "InProgress"

const cloudfrontSchema = `
CREATE TABLE IF NOT EXISTS cloudfront_distributions (
  account_id TEXT NOT NULL,
  id TEXT NOT NULL,
  arn TEXT NOT NULL,
  domain_name TEXT NOT NULL,
  comment TEXT NOT NULL DEFAULT '',
  enabled INTEGER NOT NULL DEFAULT 1,
  origins_json TEXT NOT NULL,
  caller_reference TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'InProgress',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, id)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_cf_caller ON cloudfront_distributions(account_id, caller_reference);
`

// CloudFrontOrigin is a lite origin (S3 bucket name or Gateway API id string).
type CloudFrontOrigin struct {
	ID         string `json:"Id"`
	DomainName string `json:"DomainName"`
	OriginType string `json:"OriginType"` // s3 | apigateway
}

// CloudFrontDistribution is a lab CloudFront distribution row.
type CloudFrontDistribution struct {
	ID              string
	ARN             string
	DomainName      string
	Comment         string
	Enabled         bool
	OriginsJSON     string
	CallerReference string
	Status          string
	CreatedAt       int64
}

// EnsureCloudFrontSchema creates CloudFront tables if missing.
func EnsureCloudFrontSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure cloudfront schema: db is nil")
	}
	if _, err := db.Exec(cloudfrontSchema); err != nil {
		return fmt.Errorf("ensure cloudfront schema: %w", err)
	}
	return nil
}

// EnsureCloudFrontSchema ensures CloudFront tables on an open store.
func (s *Store) EnsureCloudFrontSchema() error {
	return EnsureCloudFrontSchema(s.db)
}

// CloudFrontDistributionARN builds arn:aws:cloudfront::ACCOUNT:distribution/ID
func CloudFrontDistributionARN(accountID, id string) string {
	return fmt.Sprintf("arn:aws:cloudfront::%s:distribution/%s", accountID, id)
}

// CreateCloudFrontDistribution creates a lite distribution.
func (s *Store) CreateCloudFrontDistribution(accountID, comment, callerReference string, enabled bool, origins []CloudFrontOrigin) (CloudFrontDistribution, error) {
	callerReference = strings.TrimSpace(callerReference)
	if callerReference == "" {
		return CloudFrontDistribution{}, fmt.Errorf("%w: CallerReference required", ErrCloudFrontBadRequest)
	}
	if len(origins) == 0 {
		return CloudFrontDistribution{}, fmt.Errorf("%w: at least one Origin required", ErrCloudFrontBadRequest)
	}
	for i := range origins {
		origins[i].ID = strings.TrimSpace(origins[i].ID)
		origins[i].DomainName = strings.TrimSpace(origins[i].DomainName)
		origins[i].OriginType = strings.ToLower(strings.TrimSpace(origins[i].OriginType))
		if origins[i].ID == "" || origins[i].DomainName == "" {
			return CloudFrontDistribution{}, fmt.Errorf("%w: Origin Id and DomainName required", ErrCloudFrontBadRequest)
		}
		switch origins[i].OriginType {
		case "", "s3":
			origins[i].OriginType = "s3"
		case "apigateway", "gateway":
			origins[i].OriginType = "apigateway"
		default:
			return CloudFrontDistribution{}, fmt.Errorf("%w: OriginType must be s3 or apigateway", ErrCloudFrontBadRequest)
		}
	}
	for _, o := range origins {
		switch o.OriginType {
		case "s3":
			if _, err := s.GetBucket(accountID, o.DomainName); err != nil {
				return CloudFrontDistribution{}, fmt.Errorf("%w: S3 origin DomainName must be an existing lab bucket name", ErrCloudFrontBadRequest)
			}
		case "apigateway":
			if _, err := s.GetAPIGatewayAPI(accountID, o.DomainName); err != nil {
				return CloudFrontDistribution{}, fmt.Errorf("%w: apigateway origin DomainName must be an existing lab HTTP API id", ErrCloudFrontBadRequest)
			}
		}
	}
	originsJSON, err := json.Marshal(origins)
	if err != nil {
		return CloudFrontDistribution{}, fmt.Errorf("marshal origins: %w", err)
	}
	id := "E" + strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", "")[:13])
	arn := CloudFrontDistributionARN(accountID, id)
	// No fake-edge listener yet: omit DomainName and keep Status InProgress (not Deployed).
	domain := ""
	now := time.Now().UTC().UnixMilli()
	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	_, err = s.db.Exec(
		`INSERT INTO cloudfront_distributions
		 (account_id, id, arn, domain_name, comment, enabled, origins_json, caller_reference, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		accountID, id, arn, domain, comment, enabledInt, string(originsJSON), callerReference, CloudFrontStatusInProgress, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "constraint") {
			return CloudFrontDistribution{}, ErrCloudFrontExists
		}
		return CloudFrontDistribution{}, fmt.Errorf("create distribution: %w", err)
	}
	return CloudFrontDistribution{
		ID: id, ARN: arn, DomainName: domain, Comment: comment, Enabled: enabled,
		OriginsJSON: string(originsJSON), CallerReference: callerReference, Status: CloudFrontStatusInProgress, CreatedAt: now,
	}, nil
}

// GetCloudFrontDistribution returns a distribution by ID.
func (s *Store) GetCloudFrontDistribution(accountID, id string) (CloudFrontDistribution, error) {
	id = strings.TrimSpace(id)
	var d CloudFrontDistribution
	var enabledInt int
	err := s.db.QueryRow(
		`SELECT id, arn, domain_name, comment, enabled, origins_json, caller_reference, status, created_at
		 FROM cloudfront_distributions WHERE account_id = ? AND id = ?`,
		accountID, id,
	).Scan(&d.ID, &d.ARN, &d.DomainName, &d.Comment, &enabledInt, &d.OriginsJSON, &d.CallerReference, &d.Status, &d.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return CloudFrontDistribution{}, ErrCloudFrontNotFound
	}
	if err != nil {
		return CloudFrontDistribution{}, fmt.Errorf("get distribution: %w", err)
	}
	d.Enabled = enabledInt != 0
	return d, nil
}

// ListCloudFrontDistributions lists distributions for an account.
func (s *Store) ListCloudFrontDistributions(accountID string) ([]CloudFrontDistribution, error) {
	rows, err := s.db.Query(
		`SELECT id, arn, domain_name, comment, enabled, origins_json, caller_reference, status, created_at
		 FROM cloudfront_distributions WHERE account_id = ? ORDER BY id`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list distributions: %w", err)
	}
	defer rows.Close()
	var out []CloudFrontDistribution
	for rows.Next() {
		var d CloudFrontDistribution
		var enabledInt int
		if err := rows.Scan(&d.ID, &d.ARN, &d.DomainName, &d.Comment, &enabledInt, &d.OriginsJSON, &d.CallerReference, &d.Status, &d.CreatedAt); err != nil {
			return nil, fmt.Errorf("list distributions scan: %w", err)
		}
		d.Enabled = enabledInt != 0
		out = append(out, d)
	}
	return out, rows.Err()
}

// DeleteCloudFrontDistribution deletes a distribution by ID.
func (s *Store) DeleteCloudFrontDistribution(accountID, id string) error {
	res, err := s.db.Exec(`DELETE FROM cloudfront_distributions WHERE account_id = ? AND id = ?`, accountID, strings.TrimSpace(id))
	if err != nil {
		return fmt.Errorf("delete distribution: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrCloudFrontNotFound
	}
	return nil
}
