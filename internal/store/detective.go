package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrDetectiveNotFound   = errors.New("ResourceNotFoundException")
	ErrDetectiveBadRequest = errors.New("ValidationException")
)

const DefaultDetectiveRegion = "us-east-1"

const detectiveSchema = `
CREATE TABLE IF NOT EXISTS detective_graphs (
  account_id TEXT NOT NULL,
  region TEXT NOT NULL,
  graph_id TEXT NOT NULL,
  graph_arn TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, region)
);
`

// DetectiveGraph is a lab behavior graph row.
type DetectiveGraph struct {
	GraphARN    string
	GraphID     string
	Region      string
	CreatedAt   time.Time
	AccountID   string
}

// DetectiveSearchFilter joins CloudTrail JSONL with GuardDuty findings.
type DetectiveSearchFilter struct {
	ResourceArn string
	IpAddress   string
	MaxResults  int
}

// DetectiveSearchResult is the lab SearchGraph payload (PascalCase envelope in services layer).
type DetectiveSearchResult struct {
	GraphARN            string
	CloudTrailEvents    []json.RawMessage
	GuardDutyFindings   []GuardDutyFinding
	MatchedResourceArn  string
	MatchedIpAddress    string
}

// EnsureDetectiveSchema creates Detective tables if missing.
func EnsureDetectiveSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure detective schema: db is nil")
	}
	if _, err := db.Exec(detectiveSchema); err != nil {
		return fmt.Errorf("ensure detective schema: %w", err)
	}
	return nil
}

func (s *Store) EnsureDetectiveSchema() error {
	return EnsureDetectiveSchema(s.db)
}

func detectiveGraphARN(region, accountID, graphID string) string {
	if region == "" {
		region = DefaultDetectiveRegion
	}
	return fmt.Sprintf("arn:aws:detective:%s:%s:graph:%s", region, accountID, graphID)
}

// CreateDetectiveGraph enables Detective for the account in region (one graph per account per region).
func (s *Store) CreateDetectiveGraph(accountID, region string) (DetectiveGraph, error) {
	if err := s.EnsureDetectiveSchema(); err != nil {
		return DetectiveGraph{}, err
	}
	if region == "" {
		region = DefaultDetectiveRegion
	}
	var existing DetectiveGraph
	var existingCreatedMS int64
	err := s.db.QueryRow(
		`SELECT graph_id, graph_arn, created_at FROM detective_graphs WHERE account_id = ? AND region = ?`,
		accountID, region,
	).Scan(&existing.GraphID, &existing.GraphARN, &existingCreatedMS)
	if err == nil {
		existing.Region = region
		existing.AccountID = accountID
		existing.CreatedAt = time.UnixMilli(existingCreatedMS).UTC()
		return existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return DetectiveGraph{}, fmt.Errorf("create detective graph: %w", err)
	}

	graphID := strings.ReplaceAll(uuid.NewString(), "-", "")
	arn := detectiveGraphARN(region, accountID, graphID)
	now := time.Now().UTC()
	nowMS := now.UnixMilli()
	_, err = s.db.Exec(
		`INSERT INTO detective_graphs (account_id, region, graph_id, graph_arn, created_at) VALUES (?, ?, ?, ?, ?)`,
		accountID, region, graphID, arn, nowMS,
	)
	if err != nil {
		return DetectiveGraph{}, fmt.Errorf("create detective graph insert: %w", err)
	}
	return DetectiveGraph{
		GraphARN:  arn,
		GraphID:   graphID,
		Region:    region,
		AccountID: accountID,
		CreatedAt: now,
	}, nil
}

// ListDetectiveGraphs returns graphs for an account (optionally filtered by region).
func (s *Store) ListDetectiveGraphs(accountID, region string, maxResults int) ([]DetectiveGraph, error) {
	if err := s.EnsureDetectiveSchema(); err != nil {
		return nil, err
	}
	if maxResults <= 0 || maxResults > 50 {
		maxResults = 50
	}
	var rows *sql.Rows
	var err error
	if strings.TrimSpace(region) != "" {
		rows, err = s.db.Query(
			`SELECT graph_id, graph_arn, region, created_at FROM detective_graphs
			 WHERE account_id = ? AND region = ? ORDER BY created_at LIMIT ?`,
			accountID, region, maxResults,
		)
	} else {
		rows, err = s.db.Query(
			`SELECT graph_id, graph_arn, region, created_at FROM detective_graphs
			 WHERE account_id = ? ORDER BY created_at LIMIT ?`,
			accountID, maxResults,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("list detective graphs: %w", err)
	}
	defer rows.Close()
	var out []DetectiveGraph
	for rows.Next() {
		var g DetectiveGraph
		var createdMS int64
		if err := rows.Scan(&g.GraphID, &g.GraphARN, &g.Region, &createdMS); err != nil {
			return nil, fmt.Errorf("list detective graphs scan: %w", err)
		}
		g.AccountID = accountID
		g.CreatedAt = time.UnixMilli(createdMS).UTC()
		out = append(out, g)
	}
	return out, rows.Err()
}

// GetDetectiveGraphByARN returns a graph owned by the account or ErrDetectiveNotFound.
func (s *Store) GetDetectiveGraphByARN(accountID, graphARN string) (DetectiveGraph, error) {
	if err := s.EnsureDetectiveSchema(); err != nil {
		return DetectiveGraph{}, err
	}
	graphARN = strings.TrimSpace(graphARN)
	if graphARN == "" {
		return DetectiveGraph{}, fmt.Errorf("%w: GraphArn required", ErrDetectiveBadRequest)
	}
	var g DetectiveGraph
	var createdMS int64
	err := s.db.QueryRow(
		`SELECT graph_id, graph_arn, region, created_at FROM detective_graphs
		 WHERE account_id = ? AND graph_arn = ?`,
		accountID, graphARN,
	).Scan(&g.GraphID, &g.GraphARN, &g.Region, &createdMS)
	if errors.Is(err, sql.ErrNoRows) {
		return DetectiveGraph{}, fmt.Errorf("%w: graph not found", ErrDetectiveNotFound)
	}
	if err != nil {
		return DetectiveGraph{}, fmt.Errorf("get detective graph: %w", err)
	}
	g.AccountID = accountID
	g.CreatedAt = time.UnixMilli(createdMS).UTC()
	return g, nil
}

// ListGuardDutyFindingsForAccount returns findings across all detectors (newest first).
func (s *Store) ListGuardDutyFindingsForAccount(accountID string, maxResults int) ([]GuardDutyFinding, error) {
	if err := s.EnsureGuardDutySchema(); err != nil {
		return nil, err
	}
	if maxResults <= 0 || maxResults > 50 {
		maxResults = 50
	}
	rows, err := s.db.Query(
		`SELECT finding_json FROM guardduty_findings
		 WHERE account_id = ? ORDER BY updated_at DESC LIMIT ?`,
		accountID, maxResults,
	)
	if err != nil {
		return nil, fmt.Errorf("list guardduty findings for account: %w", err)
	}
	defer rows.Close()
	var out []GuardDutyFinding
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("list guardduty findings scan: %w", err)
		}
		var f GuardDutyFinding
		if err := json.Unmarshal([]byte(raw), &f); err != nil {
			return nil, fmt.Errorf("unmarshal guardduty finding: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// SearchDetectiveGraph joins CloudTrail JSONL events with GuardDuty findings by resource ARN and/or IP.
func (s *Store) SearchDetectiveGraph(accountID, dataRoot string, graphARN string, filter DetectiveSearchFilter) (DetectiveSearchResult, error) {
	if _, err := s.GetDetectiveGraphByARN(accountID, graphARN); err != nil {
		return DetectiveSearchResult{}, err
	}
	resourceArn := strings.TrimSpace(filter.ResourceArn)
	ip := strings.TrimSpace(filter.IpAddress)
	if resourceArn == "" && ip == "" {
		return DetectiveSearchResult{}, fmt.Errorf("%w: ResourceArn or IpAddress required", ErrDetectiveBadRequest)
	}
	max := filter.MaxResults
	if max <= 0 || max > 50 {
		max = 25
	}

	ctEvents, err := searchDetectiveCloudTrailEvents(dataRoot, resourceArn, ip, max)
	if err != nil {
		return DetectiveSearchResult{}, err
	}

	gdAll, err := s.ListGuardDutyFindingsForAccount(accountID, 50)
	if err != nil {
		return DetectiveSearchResult{}, err
	}
	var gdMatched []GuardDutyFinding
	for _, f := range gdAll {
		if detectiveFindingMatches(f, resourceArn, ip) {
			gdMatched = append(gdMatched, f)
			if len(gdMatched) >= max {
				break
			}
		}
	}

	return DetectiveSearchResult{
		GraphARN:           graphARN,
		CloudTrailEvents:   ctEvents,
		GuardDutyFindings:  gdMatched,
		MatchedResourceArn: resourceArn,
		MatchedIpAddress:   ip,
	}, nil
}

func searchDetectiveCloudTrailEvents(dataRoot, resourceArn, ip string, max int) ([]json.RawMessage, error) {
	path := filepath.Join(dataRoot, "cloudtrail", cloudtrailEventsFile)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("search detective cloudtrail: %w", err)
	}
	lines := strings.Split(string(raw), "\n")
	var out []json.RawMessage
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		var doc map[string]any
		if err := json.Unmarshal([]byte(line), &doc); err != nil {
			continue
		}
		if !detectiveCloudTrailEventMatches(doc, resourceArn, ip) {
			continue
		}
		out = append(out, json.RawMessage(line))
		if len(out) >= max {
			break
		}
	}
	return out, nil
}

func detectiveCloudTrailEventMatches(doc map[string]any, resourceArn, ip string) bool {
	if ip != "" {
		if sip, _ := doc["sourceIPAddress"].(string); strings.EqualFold(strings.TrimSpace(sip), ip) {
			return true
		}
	}
	if resourceArn != "" {
		if detectiveMapContainsARN(doc, resourceArn) {
			return true
		}
	}
	if ip != "" && detectiveMapContainsIP(doc, ip) {
		return true
	}
	return false
}

func detectiveFindingMatches(f GuardDutyFinding, resourceArn, ip string) bool {
	raw, err := json.Marshal(f)
	if err != nil {
		return false
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return false
	}
	if resourceArn != "" && detectiveMapContainsARN(doc, resourceArn) {
		return true
	}
	if ip != "" && detectiveMapContainsIP(doc, ip) {
		return true
	}
	return false
}

func detectiveMapContainsARN(v any, arn string) bool {
	arn = strings.TrimSpace(arn)
	if arn == "" {
		return false
	}
	switch t := v.(type) {
	case string:
		return strings.EqualFold(t, arn) || strings.Contains(strings.ToLower(t), strings.ToLower(arn))
	case map[string]any:
		for _, child := range t {
			if detectiveMapContainsARN(child, arn) {
				return true
			}
		}
	case []any:
		for _, child := range t {
			if detectiveMapContainsARN(child, arn) {
				return true
			}
		}
	}
	return false
}

func detectiveMapContainsIP(v any, ip string) bool {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return false
	}
	switch t := v.(type) {
	case string:
		return strings.EqualFold(t, ip)
	case map[string]any:
		for _, child := range t {
			if detectiveMapContainsIP(child, ip) {
				return true
			}
		}
	case []any:
		for _, child := range t {
			if detectiveMapContainsIP(child, ip) {
				return true
			}
		}
	}
	return false
}
