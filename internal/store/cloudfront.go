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
	ErrCloudFrontNotFound             = errors.New("NoSuchDistribution")
	ErrCloudFrontBadRequest           = errors.New("InvalidArgument")
	ErrCloudFrontExists               = errors.New("DistributionAlreadyExists")
	ErrCloudFrontPrecondition         = errors.New("InvalidIfMatchVersion")
	ErrCloudFrontInvalidationNotFound = errors.New("NoSuchInvalidation")
)

// CloudFrontInvalidationStatusCompleted is the lab invalidation status (theatre completes immediately).
const CloudFrontInvalidationStatusCompleted = "Completed"

const DefaultCloudFrontRegion = "us-east-1"

// CloudFrontStatusInProgress is retained for older rows; new creates use Deployed.
const CloudFrontStatusInProgress = "InProgress"

// CloudFrontStatusDeployed is the lab status once a fake-edge DomainName is assigned.
const CloudFrontStatusDeployed = "Deployed"

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

CREATE TABLE IF NOT EXISTS cloudfront_invalidations (
  account_id TEXT NOT NULL,
  distribution_id TEXT NOT NULL,
  id TEXT NOT NULL,
  caller_reference TEXT NOT NULL,
  paths_json TEXT NOT NULL,
  status TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, distribution_id, id)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_cf_inv_caller
  ON cloudfront_invalidations(account_id, distribution_id, caller_reference);
`

// CloudFrontOrigin is a lite origin (S3 bucket name or Gateway API id string).
type CloudFrontOrigin struct {
	ID         string `json:"Id"`
	DomainName string `json:"DomainName"`
	OriginType string `json:"OriginType"` // s3 | apigateway
}

// CloudFrontCacheBehavior is a lite PathPattern → TargetOriginId mapping.
// PathPattern "*" is the default behavior (applied when no other pattern matches).
type CloudFrontCacheBehavior struct {
	PathPattern    string `json:"PathPattern"`
	TargetOriginId string `json:"TargetOriginId"`
}

// CloudFrontDistribution is a lab CloudFront distribution row.
type CloudFrontDistribution struct {
	ID              string
	ARN             string
	DomainName      string
	Comment         string
	Enabled         bool
	OriginsJSON     string
	BehaviorsJSON   string
	CallerReference string
	Status          string
	CreatedAt       int64
	ETag            string
	LoggingEnabled  bool
	LoggingBucket   string
	LoggingPrefix   string
}

// CloudFrontInvalidation is a lab invalidation batch (Completed immediately).
type CloudFrontInvalidation struct {
	ID              string
	DistributionID  string
	CallerReference string
	PathsJSON       string
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
	if err := execMigrateStmts(db, []string{
		`ALTER TABLE cloudfront_distributions ADD COLUMN logging_enabled INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE cloudfront_distributions ADD COLUMN logging_bucket TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE cloudfront_distributions ADD COLUMN logging_prefix TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE cloudfront_distributions ADD COLUMN behaviors_json TEXT NOT NULL DEFAULT '[]'`,
		`ALTER TABLE cloudfront_distributions ADD COLUMN etag TEXT NOT NULL DEFAULT ''`,
	}); err != nil {
		return fmt.Errorf("ensure cloudfront schema: migrate: %w", err)
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
	return s.CreateCloudFrontDistributionWithBehaviors(accountID, comment, callerReference, enabled, origins, nil)
}

// CreateCloudFrontDistributionWithBehaviors creates a distribution with optional cache behaviors.
// Behaviors are matched in list order; PathPattern "*" is the default fallback origin.
func (s *Store) CreateCloudFrontDistributionWithBehaviors(
	accountID, comment, callerReference string, enabled bool,
	origins []CloudFrontOrigin, behaviors []CloudFrontCacheBehavior,
) (CloudFrontDistribution, error) {
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
	originIDs := make(map[string]struct{}, len(origins))
	for _, o := range origins {
		originIDs[o.ID] = struct{}{}
	}
	cleanedBehaviors := make([]CloudFrontCacheBehavior, 0, len(behaviors))
	for _, b := range behaviors {
		pat := strings.TrimSpace(b.PathPattern)
		target := strings.TrimSpace(b.TargetOriginId)
		if pat == "" || target == "" {
			return CloudFrontDistribution{}, fmt.Errorf("%w: CacheBehavior PathPattern and TargetOriginId required", ErrCloudFrontBadRequest)
		}
		if _, ok := originIDs[target]; !ok {
			return CloudFrontDistribution{}, fmt.Errorf("%w: CacheBehavior TargetOriginId %q not found in Origins", ErrCloudFrontBadRequest, target)
		}
		cleanedBehaviors = append(cleanedBehaviors, CloudFrontCacheBehavior{
			PathPattern: pat, TargetOriginId: target,
		})
	}
	originsJSON, err := json.Marshal(origins)
	if err != nil {
		return CloudFrontDistribution{}, fmt.Errorf("marshal origins: %w", err)
	}
	behaviorsJSON, err := json.Marshal(cleanedBehaviors)
	if err != nil {
		return CloudFrontDistribution{}, fmt.Errorf("marshal behaviors: %w", err)
	}
	id := "E" + strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", "")[:13])
	arn := CloudFrontDistributionARN(accountID, id)
	etag := cloudFrontNewETag()
	// Lab fake-edge host string only (no DNS / WAN PoP). Path fetch is /cloudfront/{id}/...
	domain := fmt.Sprintf("d%s.cloudfront.noctaxris.local", strings.ToLower(id))
	status := CloudFrontStatusDeployed
	now := time.Now().UTC().UnixMilli()
	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	_, err = s.db.Exec(
		`INSERT INTO cloudfront_distributions
		 (account_id, id, arn, domain_name, comment, enabled, origins_json, behaviors_json, caller_reference, status, created_at, etag)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		accountID, id, arn, domain, comment, enabledInt, string(originsJSON), string(behaviorsJSON), callerReference, status, now, etag,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "constraint") {
			return CloudFrontDistribution{}, ErrCloudFrontExists
		}
		return CloudFrontDistribution{}, fmt.Errorf("create distribution: %w", err)
	}
	return CloudFrontDistribution{
		ID: id, ARN: arn, DomainName: domain, Comment: comment, Enabled: enabled,
		OriginsJSON: string(originsJSON), BehaviorsJSON: string(behaviorsJSON),
		CallerReference: callerReference, Status: status, CreatedAt: now, ETag: etag,
	}, nil
}

func cloudFrontNewETag() string {
	return strings.ReplaceAll(uuid.NewString(), "-", "")
}

// MatchCloudFrontPathPattern reports whether requestPath matches a lite PathPattern.
// Bare "*" matches all. A single trailing "*" is a prefix match. Otherwise equality.
// Leading "/" on pattern or path is optional (normalized). Case sensitive.
func MatchCloudFrontPathPattern(requestPath, pattern string) bool {
	pattern = strings.TrimSpace(pattern)
	requestPath = strings.TrimSpace(requestPath)
	if pattern == "" {
		return false
	}
	if pattern == "*" {
		return true
	}
	normPath := cloudFrontNormalizePath(requestPath)
	normPat := cloudFrontNormalizePath(pattern)
	if strings.Count(normPat, "*") > 1 {
		return false
	}
	if strings.HasSuffix(normPat, "*") {
		return strings.HasPrefix(normPath, strings.TrimSuffix(normPat, "*"))
	}
	if strings.Contains(normPat, "*") {
		return false
	}
	return normPath == normPat
}

func cloudFrontNormalizePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" || p == "*" {
		return p
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return p
}

// SelectCloudFrontOrigin picks an origin for objectKey using cache behaviors in list order.
// First non-"*" matching PathPattern wins; otherwise the first "*" (default) TargetOriginId;
// with no behaviors, the first origin is used.
func SelectCloudFrontOrigin(origins []CloudFrontOrigin, behaviors []CloudFrontCacheBehavior, objectKey string) (CloudFrontOrigin, error) {
	if len(origins) == 0 {
		return CloudFrontOrigin{}, fmt.Errorf("%w: no origins", ErrCloudFrontBadRequest)
	}
	byID := make(map[string]CloudFrontOrigin, len(origins))
	for _, o := range origins {
		byID[o.ID] = o
	}
	path := cloudFrontNormalizePath(objectKey)
	if objectKey == "" {
		path = "/"
	}
	var defaultTarget string
	for _, b := range behaviors {
		pat := strings.TrimSpace(b.PathPattern)
		target := strings.TrimSpace(b.TargetOriginId)
		if pat == "*" {
			if defaultTarget == "" {
				defaultTarget = target
			}
			continue
		}
		if MatchCloudFrontPathPattern(path, pat) {
			o, ok := byID[target]
			if !ok {
				return CloudFrontOrigin{}, fmt.Errorf("%w: TargetOriginId %q not found", ErrCloudFrontBadRequest, target)
			}
			return o, nil
		}
	}
	if defaultTarget != "" {
		o, ok := byID[defaultTarget]
		if !ok {
			return CloudFrontOrigin{}, fmt.Errorf("%w: default TargetOriginId %q not found", ErrCloudFrontBadRequest, defaultTarget)
		}
		return o, nil
	}
	return origins[0], nil
}

func scanCloudFrontDistribution(
	scan func(dest ...any) error,
) (CloudFrontDistribution, error) {
	var d CloudFrontDistribution
	var enabledInt, loggingInt int
	if err := scan(
		&d.ID, &d.ARN, &d.DomainName, &d.Comment, &enabledInt, &d.OriginsJSON, &d.BehaviorsJSON,
		&d.CallerReference, &d.Status, &d.CreatedAt, &d.ETag,
		&loggingInt, &d.LoggingBucket, &d.LoggingPrefix,
	); err != nil {
		return CloudFrontDistribution{}, err
	}
	d.Enabled = enabledInt != 0
	d.LoggingEnabled = loggingInt != 0
	if strings.TrimSpace(d.BehaviorsJSON) == "" {
		d.BehaviorsJSON = "[]"
	}
	return d, nil
}

func (s *Store) ensureCloudFrontETag(accountID string, d *CloudFrontDistribution) error {
	if d == nil || strings.TrimSpace(d.ETag) != "" {
		return nil
	}
	etag := cloudFrontNewETag()
	_, err := s.db.Exec(
		`UPDATE cloudfront_distributions SET etag = ? WHERE account_id = ? AND id = ? AND (etag IS NULL OR etag = '')`,
		etag, accountID, d.ID,
	)
	if err != nil {
		return fmt.Errorf("ensure distribution etag: %w", err)
	}
	d.ETag = etag
	return nil
}

// GetCloudFrontDistribution returns a distribution by ID.
func (s *Store) GetCloudFrontDistribution(accountID, id string) (CloudFrontDistribution, error) {
	id = strings.TrimSpace(id)
	d, err := scanCloudFrontDistribution(
		s.db.QueryRow(
			`SELECT id, arn, domain_name, comment, enabled, origins_json, COALESCE(behaviors_json, '[]'),
			        caller_reference, status, created_at, COALESCE(etag, ''),
			        logging_enabled, logging_bucket, logging_prefix
			 FROM cloudfront_distributions WHERE account_id = ? AND id = ?`,
			accountID, id,
		).Scan,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return CloudFrontDistribution{}, ErrCloudFrontNotFound
	}
	if err != nil {
		return CloudFrontDistribution{}, fmt.Errorf("get distribution: %w", err)
	}
	if err := s.ensureCloudFrontETag(accountID, &d); err != nil {
		return CloudFrontDistribution{}, err
	}
	return d, nil
}

// ListCloudFrontDistributions lists distributions for an account.
func (s *Store) ListCloudFrontDistributions(accountID string) ([]CloudFrontDistribution, error) {
	rows, err := s.db.Query(
		`SELECT id, arn, domain_name, comment, enabled, origins_json, COALESCE(behaviors_json, '[]'),
		        caller_reference, status, created_at, COALESCE(etag, ''),
		        logging_enabled, logging_bucket, logging_prefix
		 FROM cloudfront_distributions WHERE account_id = ? ORDER BY id`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list distributions: %w", err)
	}
	defer rows.Close()
	var out []CloudFrontDistribution
	for rows.Next() {
		d, err := scanCloudFrontDistribution(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("list distributions scan: %w", err)
		}
		if err := s.ensureCloudFrontETag(accountID, &d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// UpdateCloudFrontDistributionInput is the mutable lab subset for UpdateDistribution.
type UpdateCloudFrontDistributionInput struct {
	IfMatch      string
	Comment      *string
	Enabled      *bool
	Origins      []CloudFrontOrigin
	Behaviors    []CloudFrontCacheBehavior
	HasOrigins   bool
	HasBehaviors bool
}

// UpdateCloudFrontDistribution mutates Enabled/origins/cache behaviors when IfMatch matches ETag.
func (s *Store) UpdateCloudFrontDistribution(accountID, id string, in UpdateCloudFrontDistributionInput) (CloudFrontDistribution, error) {
	id = strings.TrimSpace(id)
	ifMatch := strings.Trim(strings.TrimSpace(in.IfMatch), `"`)
	if ifMatch == "" {
		return CloudFrontDistribution{}, fmt.Errorf("%w: IfMatch required", ErrCloudFrontPrecondition)
	}
	existing, err := s.GetCloudFrontDistribution(accountID, id)
	if err != nil {
		return CloudFrontDistribution{}, err
	}
	if strings.Trim(strings.TrimSpace(existing.ETag), `"`) != ifMatch {
		return CloudFrontDistribution{}, ErrCloudFrontPrecondition
	}

	comment := existing.Comment
	if in.Comment != nil {
		comment = *in.Comment
	}
	enabled := existing.Enabled
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	originsJSON := existing.OriginsJSON
	behaviorsJSON := existing.BehaviorsJSON
	var origins []CloudFrontOrigin
	if in.HasOrigins {
		if len(in.Origins) == 0 {
			return CloudFrontDistribution{}, fmt.Errorf("%w: at least one Origin required", ErrCloudFrontBadRequest)
		}
		cleaned, err := s.validateCloudFrontOrigins(accountID, in.Origins)
		if err != nil {
			return CloudFrontDistribution{}, err
		}
		origins = cleaned
		b, err := json.Marshal(cleaned)
		if err != nil {
			return CloudFrontDistribution{}, fmt.Errorf("marshal origins: %w", err)
		}
		originsJSON = string(b)
	} else {
		_ = json.Unmarshal([]byte(existing.OriginsJSON), &origins)
	}
	if in.HasBehaviors {
		originIDs := make(map[string]struct{}, len(origins))
		for _, o := range origins {
			originIDs[o.ID] = struct{}{}
		}
		cleaned, err := validateCloudFrontBehaviors(in.Behaviors, originIDs)
		if err != nil {
			return CloudFrontDistribution{}, err
		}
		b, err := json.Marshal(cleaned)
		if err != nil {
			return CloudFrontDistribution{}, fmt.Errorf("marshal behaviors: %w", err)
		}
		behaviorsJSON = string(b)
	}

	newETag := cloudFrontNewETag()
	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	res, err := s.db.Exec(
		`UPDATE cloudfront_distributions
		 SET comment = ?, enabled = ?, origins_json = ?, behaviors_json = ?, etag = ?, status = ?
		 WHERE account_id = ? AND id = ? AND etag = ?`,
		comment, enabledInt, originsJSON, behaviorsJSON, newETag, CloudFrontStatusDeployed,
		accountID, id, existing.ETag,
	)
	if err != nil {
		return CloudFrontDistribution{}, fmt.Errorf("update distribution: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return CloudFrontDistribution{}, ErrCloudFrontPrecondition
	}
	return s.GetCloudFrontDistribution(accountID, id)
}

func (s *Store) validateCloudFrontOrigins(accountID string, origins []CloudFrontOrigin) ([]CloudFrontOrigin, error) {
	out := make([]CloudFrontOrigin, len(origins))
	copy(out, origins)
	for i := range out {
		out[i].ID = strings.TrimSpace(out[i].ID)
		out[i].DomainName = strings.TrimSpace(out[i].DomainName)
		out[i].OriginType = strings.ToLower(strings.TrimSpace(out[i].OriginType))
		if out[i].ID == "" || out[i].DomainName == "" {
			return nil, fmt.Errorf("%w: Origin Id and DomainName required", ErrCloudFrontBadRequest)
		}
		switch out[i].OriginType {
		case "", "s3":
			out[i].OriginType = "s3"
		case "apigateway", "gateway":
			out[i].OriginType = "apigateway"
		default:
			return nil, fmt.Errorf("%w: OriginType must be s3 or apigateway", ErrCloudFrontBadRequest)
		}
	}
	for _, o := range out {
		switch o.OriginType {
		case "s3":
			if _, err := s.GetBucket(accountID, o.DomainName); err != nil {
				return nil, fmt.Errorf("%w: S3 origin DomainName must be an existing lab bucket name", ErrCloudFrontBadRequest)
			}
		case "apigateway":
			if _, err := s.GetAPIGatewayAPI(accountID, o.DomainName); err != nil {
				return nil, fmt.Errorf("%w: apigateway origin DomainName must be an existing lab HTTP API id", ErrCloudFrontBadRequest)
			}
		}
	}
	return out, nil
}

func validateCloudFrontBehaviors(behaviors []CloudFrontCacheBehavior, originIDs map[string]struct{}) ([]CloudFrontCacheBehavior, error) {
	cleaned := make([]CloudFrontCacheBehavior, 0, len(behaviors))
	for _, b := range behaviors {
		pat := strings.TrimSpace(b.PathPattern)
		target := strings.TrimSpace(b.TargetOriginId)
		if pat == "" || target == "" {
			return nil, fmt.Errorf("%w: CacheBehavior PathPattern and TargetOriginId required", ErrCloudFrontBadRequest)
		}
		if _, ok := originIDs[target]; !ok {
			return nil, fmt.Errorf("%w: CacheBehavior TargetOriginId %q not found in Origins", ErrCloudFrontBadRequest, target)
		}
		cleaned = append(cleaned, CloudFrontCacheBehavior{
			PathPattern: pat, TargetOriginId: target,
		})
	}
	return cleaned, nil
}

// CreateCloudFrontInvalidation stores paths and marks Status Completed immediately.
func (s *Store) CreateCloudFrontInvalidation(accountID, distributionID, callerReference string, paths []string) (CloudFrontInvalidation, error) {
	distributionID = strings.TrimSpace(distributionID)
	callerReference = strings.TrimSpace(callerReference)
	if callerReference == "" {
		return CloudFrontInvalidation{}, fmt.Errorf("%w: CallerReference required", ErrCloudFrontBadRequest)
	}
	if _, err := s.GetCloudFrontDistribution(accountID, distributionID); err != nil {
		return CloudFrontInvalidation{}, err
	}
	cleaned := make([]string, 0, len(paths))
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		cleaned = append(cleaned, p)
	}
	if len(cleaned) == 0 {
		return CloudFrontInvalidation{}, fmt.Errorf("%w: at least one Path required", ErrCloudFrontBadRequest)
	}
	pathsJSON, err := json.Marshal(cleaned)
	if err != nil {
		return CloudFrontInvalidation{}, fmt.Errorf("marshal paths: %w", err)
	}
	id := "I" + strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", "")[:13])
	now := time.Now().UTC().UnixMilli()
	status := CloudFrontInvalidationStatusCompleted
	_, err = s.db.Exec(
		`INSERT INTO cloudfront_invalidations
		 (account_id, distribution_id, id, caller_reference, paths_json, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		accountID, distributionID, id, callerReference, string(pathsJSON), status, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "constraint") {
			existing, getErr := s.getCloudFrontInvalidationByCaller(accountID, distributionID, callerReference)
			if getErr == nil {
				var existingPaths []string
				_ = json.Unmarshal([]byte(existing.PathsJSON), &existingPaths)
				if cloudFrontPathsEqual(existingPaths, cleaned) {
					return existing, nil
				}
				return CloudFrontInvalidation{}, fmt.Errorf("%w: InvalidationBatchAlreadyExists", ErrCloudFrontBadRequest)
			}
			return CloudFrontInvalidation{}, ErrCloudFrontExists
		}
		return CloudFrontInvalidation{}, fmt.Errorf("create invalidation: %w", err)
	}
	return CloudFrontInvalidation{
		ID: id, DistributionID: distributionID, CallerReference: callerReference,
		PathsJSON: string(pathsJSON), Status: status, CreatedAt: now,
	}, nil
}

func cloudFrontPathsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (s *Store) getCloudFrontInvalidationByCaller(accountID, distributionID, callerReference string) (CloudFrontInvalidation, error) {
	var inv CloudFrontInvalidation
	err := s.db.QueryRow(
		`SELECT id, distribution_id, caller_reference, paths_json, status, created_at
		 FROM cloudfront_invalidations
		 WHERE account_id = ? AND distribution_id = ? AND caller_reference = ?`,
		accountID, distributionID, callerReference,
	).Scan(&inv.ID, &inv.DistributionID, &inv.CallerReference, &inv.PathsJSON, &inv.Status, &inv.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return CloudFrontInvalidation{}, ErrCloudFrontInvalidationNotFound
	}
	if err != nil {
		return CloudFrontInvalidation{}, fmt.Errorf("get invalidation by caller: %w", err)
	}
	return inv, nil
}

// GetCloudFrontInvalidation returns an invalidation by distribution and id.
func (s *Store) GetCloudFrontInvalidation(accountID, distributionID, id string) (CloudFrontInvalidation, error) {
	if _, err := s.GetCloudFrontDistribution(accountID, distributionID); err != nil {
		return CloudFrontInvalidation{}, err
	}
	var inv CloudFrontInvalidation
	err := s.db.QueryRow(
		`SELECT id, distribution_id, caller_reference, paths_json, status, created_at
		 FROM cloudfront_invalidations
		 WHERE account_id = ? AND distribution_id = ? AND id = ?`,
		accountID, strings.TrimSpace(distributionID), strings.TrimSpace(id),
	).Scan(&inv.ID, &inv.DistributionID, &inv.CallerReference, &inv.PathsJSON, &inv.Status, &inv.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return CloudFrontInvalidation{}, ErrCloudFrontInvalidationNotFound
	}
	if err != nil {
		return CloudFrontInvalidation{}, fmt.Errorf("get invalidation: %w", err)
	}
	return inv, nil
}

// ListCloudFrontInvalidations lists invalidations for a distribution (newest first).
func (s *Store) ListCloudFrontInvalidations(accountID, distributionID string) ([]CloudFrontInvalidation, error) {
	if _, err := s.GetCloudFrontDistribution(accountID, distributionID); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT id, distribution_id, caller_reference, paths_json, status, created_at
		 FROM cloudfront_invalidations
		 WHERE account_id = ? AND distribution_id = ?
		 ORDER BY created_at DESC, id DESC`,
		accountID, strings.TrimSpace(distributionID),
	)
	if err != nil {
		return nil, fmt.Errorf("list invalidations: %w", err)
	}
	defer rows.Close()
	var out []CloudFrontInvalidation
	for rows.Next() {
		var inv CloudFrontInvalidation
		if err := rows.Scan(&inv.ID, &inv.DistributionID, &inv.CallerReference, &inv.PathsJSON, &inv.Status, &inv.CreatedAt); err != nil {
			return nil, fmt.Errorf("list invalidations scan: %w", err)
		}
		out = append(out, inv)
	}
	return out, rows.Err()
}

// DeleteCloudFrontDistribution deletes a distribution by ID.
func (s *Store) DeleteCloudFrontDistribution(accountID, id string) error {
	id = strings.TrimSpace(id)
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("delete distribution begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM cloudfront_invalidations WHERE account_id = ? AND distribution_id = ?`, accountID, id); err != nil {
		return fmt.Errorf("delete distribution invalidations: %w", err)
	}
	res, err := tx.Exec(`DELETE FROM cloudfront_distributions WHERE account_id = ? AND id = ?`, accountID, id)
	if err != nil {
		return fmt.Errorf("delete distribution: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrCloudFrontNotFound
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("delete distribution commit: %w", err)
	}
	return nil
}
