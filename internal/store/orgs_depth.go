package store

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/Kyaxris-Labs/Noctaxris/internal/validate"
)

// ManagementAccountID is the lab bootstrap account treated as the Organizations
// management account. No separate store flag is used; account 000000000001 is
// always management for SCP exemption and org control-plane semantics.
const ManagementAccountID = "000000000001"

// OrgRootID is the fixed Organizations root identifier used as CreateOU parent.
const OrgRootID = "r-root"

// Account is an Organizations / local account listing entry.
type Account struct {
	AccountID string
}

// OrgPolicy is an Organizations SCP or RCP.
type OrgPolicy struct {
	ID       string
	Type     string // SCP or RCP
	Name     string
	Document string
}

// OrganizationalUnit is an Organizations OU listing entry.
type OrganizationalUnit struct {
	ID       string
	ParentID string
	Name     string
}

// IsManagementAccount reports whether accountID is the lab management account.
func (s *Store) IsManagementAccount(accountID string) bool {
	return accountID == ManagementAccountID
}

// ListAccounts returns all accounts known to the store.
func (s *Store) ListAccounts() ([]Account, error) {
	rows, err := s.db.Query(`SELECT account_id FROM accounts ORDER BY account_id`)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	defer rows.Close()

	var out []Account
	for rows.Next() {
		var a Account
		if err := rows.Scan(&a.AccountID); err != nil {
			return nil, fmt.Errorf("list accounts: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	if out == nil {
		out = []Account{}
	}
	return out, nil
}

// CreateOrganizationalUnit creates an OU under parentID (typically OrgRootID).
func (s *Store) CreateOrganizationalUnit(parentID, name string) (ouID string, err error) {
	if parentID == "" {
		return "", fmt.Errorf("create organizational unit: parent_id required")
	}
	if name == "" {
		return "", fmt.Errorf("create organizational unit: name required")
	}
	ouID, err = newOrgUnitID()
	if err != nil {
		return "", err
	}
	_, err = s.db.Exec(
		`INSERT INTO org_ous (id, parent_id, name) VALUES (?, ?, ?)`,
		ouID, parentID, name,
	)
	if err != nil {
		return "", fmt.Errorf("create organizational unit %s: %w", name, err)
	}
	return ouID, nil
}

// CreateOrgPolicy creates an Organizations SCP or RCP.
func (s *Store) CreateOrgPolicy(policyType, name, document string) (policyID string, err error) {
	switch policyType {
	case "SCP", "RCP":
	default:
		return "", fmt.Errorf("create org policy: type must be SCP or RCP")
	}
	if name == "" {
		return "", fmt.Errorf("create org policy: name required")
	}
	if err := validate.PolicyDocument(document); err != nil {
		return "", fmt.Errorf("create org policy: %w", err)
	}
	policyID, err = newOrgPolicyID()
	if err != nil {
		return "", err
	}
	_, err = s.db.Exec(
		`INSERT INTO org_policies (id, type, name, document) VALUES (?, ?, ?, ?)`,
		policyID, policyType, name, document,
	)
	if err != nil {
		return "", fmt.Errorf("create org policy %s: %w", name, err)
	}
	return policyID, nil
}

// AttachOrgPolicy attaches an org policy to a root, OU, or account target.
func (s *Store) AttachOrgPolicy(policyID, targetType, targetID string) error {
	switch targetType {
	case "root", "ou", "account":
	default:
		return fmt.Errorf("attach org policy: target_type must be root, ou, or account")
	}
	if targetID == "" {
		return fmt.Errorf("attach org policy: target_id required")
	}
	var existing string
	err := s.db.QueryRow(`SELECT id FROM org_policies WHERE id = ?`, policyID).Scan(&existing)
	if err != nil {
		return fmt.Errorf("attach org policy: %w", err)
	}
	_, err = s.db.Exec(
		`INSERT OR IGNORE INTO org_policy_attachments (policy_id, target_type, target_id) VALUES (?, ?, ?)`,
		policyID, targetType, targetID,
	)
	if err != nil {
		return fmt.Errorf("attach org policy %s to %s/%s: %w", policyID, targetType, targetID, err)
	}
	return nil
}

// DetachOrgPolicy removes an org policy attachment from a target.
func (s *Store) DetachOrgPolicy(policyID, targetType, targetID string) error {
	switch targetType {
	case "root", "ou", "account":
	default:
		return fmt.Errorf("detach org policy: target_type must be root, ou, or account")
	}
	if targetID == "" {
		return fmt.Errorf("detach org policy: target_id required")
	}
	res, err := s.db.Exec(
		`DELETE FROM org_policy_attachments WHERE policy_id = ? AND target_type = ? AND target_id = ?`,
		policyID, targetType, targetID,
	)
	if err != nil {
		return fmt.Errorf("detach org policy %s from %s/%s: %w", policyID, targetType, targetID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("detach org policy %s from %s/%s: %w", policyID, targetType, targetID, err)
	}
	if n == 0 {
		return fmt.Errorf("detach org policy: attachment not found")
	}
	return nil
}

// GetOrgPolicy returns an org policy by id.
func (s *Store) GetOrgPolicy(policyID string) (OrgPolicy, error) {
	var p OrgPolicy
	err := s.db.QueryRow(
		`SELECT id, type, name, document FROM org_policies WHERE id = ?`,
		policyID,
	).Scan(&p.ID, &p.Type, &p.Name, &p.Document)
	if err != nil {
		return OrgPolicy{}, fmt.Errorf("get org policy %s: %w", policyID, err)
	}
	return p, nil
}

// ListOrganizationalUnitsForParent returns OUs whose parent_id matches parentID.
func (s *Store) ListOrganizationalUnitsForParent(parentID string) ([]OrganizationalUnit, error) {
	if parentID == "" {
		return nil, fmt.Errorf("list organizational units: parent_id required")
	}
	rows, err := s.db.Query(
		`SELECT id, parent_id, name FROM org_ous WHERE parent_id = ? ORDER BY name`,
		parentID,
	)
	if err != nil {
		return nil, fmt.Errorf("list organizational units for parent %s: %w", parentID, err)
	}
	defer rows.Close()

	var out []OrganizationalUnit
	for rows.Next() {
		var ou OrganizationalUnit
		if err := rows.Scan(&ou.ID, &ou.ParentID, &ou.Name); err != nil {
			return nil, fmt.Errorf("list organizational units for parent %s: %w", parentID, err)
		}
		out = append(out, ou)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list organizational units for parent %s: %w", parentID, err)
	}
	if out == nil {
		out = []OrganizationalUnit{}
	}
	return out, nil
}

// EnableOrgPolicyType records that policyType (SCP or RCP) is enabled on rootID.
func (s *Store) EnableOrgPolicyType(rootID, policyType string) error {
	switch policyType {
	case "SCP", "RCP":
	default:
		return fmt.Errorf("enable org policy type: type must be SCP or RCP")
	}
	if rootID == "" {
		return fmt.Errorf("enable org policy type: root_id required")
	}
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO org_enabled_policy_types (root_id, policy_type) VALUES (?, ?)`,
		rootID, policyType,
	)
	if err != nil {
		return fmt.Errorf("enable org policy type %s on %s: %w", policyType, rootID, err)
	}
	return nil
}

// IsOrgPolicyTypeEnabled reports whether policyType is enabled on rootID.
func (s *Store) IsOrgPolicyTypeEnabled(rootID, policyType string) bool {
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM org_enabled_policy_types WHERE root_id = ? AND policy_type = ?`,
		rootID, policyType,
	).Scan(&n)
	return err == nil && n > 0
}

// ListEnabledOrgPolicyTypes returns enabled policy types (SCP/RCP) for rootID.
func (s *Store) ListEnabledOrgPolicyTypes(rootID string) ([]string, error) {
	rows, err := s.db.Query(
		`SELECT policy_type FROM org_enabled_policy_types WHERE root_id = ? ORDER BY policy_type`,
		rootID,
	)
	if err != nil {
		return nil, fmt.Errorf("list enabled org policy types for %s: %w", rootID, err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, fmt.Errorf("list enabled org policy types for %s: %w", rootID, err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list enabled org policy types for %s: %w", rootID, err)
	}
	if out == nil {
		out = []string{}
	}
	return out, nil
}

// ListPoliciesForTarget returns org policies attached to the given target.
func (s *Store) ListPoliciesForTarget(targetType, targetID string) ([]OrgPolicy, error) {
	rows, err := s.db.Query(
		`SELECT p.id, p.type, p.name, p.document
		 FROM org_policy_attachments a
		 JOIN org_policies p ON p.id = a.policy_id
		 WHERE a.target_type = ? AND a.target_id = ?
		 ORDER BY p.name`,
		targetType, targetID,
	)
	if err != nil {
		return nil, fmt.Errorf("list policies for target %s/%s: %w", targetType, targetID, err)
	}
	defer rows.Close()

	var out []OrgPolicy
	for rows.Next() {
		var p OrgPolicy
		if err := rows.Scan(&p.ID, &p.Type, &p.Name, &p.Document); err != nil {
			return nil, fmt.Errorf("list policies for target %s/%s: %w", targetType, targetID, err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list policies for target %s/%s: %w", targetType, targetID, err)
	}
	if out == nil {
		out = []OrgPolicy{}
	}
	return out, nil
}

// SCPDocsForAccount returns SCP documents that apply to accountID.
// Includes policies attached directly to the account and to the organization root.
// OU-path inheritance requires account placement (not stored in this tranche);
// OU-attached policies are not included unless also attached to account or root.
func (s *Store) SCPDocsForAccount(accountID string) ([]string, error) {
	return s.orgPolicyDocsForAccount(accountID, "SCP")
}

// RCPDocsForAccount returns RCP documents that apply to accountID.
func (s *Store) RCPDocsForAccount(accountID string) ([]string, error) {
	return s.orgPolicyDocsForAccount(accountID, "RCP")
}

func (s *Store) orgPolicyDocsForAccount(accountID, policyType string) ([]string, error) {
	rows, err := s.db.Query(
		`SELECT DISTINCT p.id, p.document
		 FROM org_policies p
		 JOIN org_policy_attachments a ON a.policy_id = p.id
		 WHERE p.type = ?
		   AND (
		     (a.target_type = 'account' AND a.target_id = ?)
		     OR (a.target_type = 'root' AND a.target_id = ?)
		   )
		 ORDER BY p.id`,
		policyType, accountID, OrgRootID,
	)
	if err != nil {
		return nil, fmt.Errorf("%s docs for account %s: %w", policyType, accountID, err)
	}
	defer rows.Close()

	var docs []string
	for rows.Next() {
		var id, doc string
		if err := rows.Scan(&id, &doc); err != nil {
			return nil, fmt.Errorf("%s docs for account %s: %w", policyType, accountID, err)
		}
		docs = append(docs, doc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s docs for account %s: %w", policyType, accountID, err)
	}
	if docs == nil {
		docs = []string{}
	}
	return docs, nil
}

func newOrgUnitID() (string, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "ou-" + hex.EncodeToString(b[:]), nil
}

func newOrgPolicyID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "p-" + hex.EncodeToString(b[:]), nil
}
