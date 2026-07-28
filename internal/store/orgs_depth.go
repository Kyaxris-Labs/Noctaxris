package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/validate"
)

const orgAccountParentsSchema = `
CREATE TABLE IF NOT EXISTS org_account_parents (
  account_id TEXT PRIMARY KEY,
  parent_id TEXT NOT NULL
);
`

// EnsureOrgAccountPlacementSchema creates the account→parent OU/root table.
func EnsureOrgAccountPlacementSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure org account placement schema: db is nil")
	}
	if _, err := db.Exec(orgAccountParentsSchema); err != nil {
		return fmt.Errorf("ensure org account placement schema: %w", err)
	}
	return nil
}

// EnsureOrgAccountPlacementSchema ensures placement tables on an open store.
func (s *Store) EnsureOrgAccountPlacementSchema() error {
	return EnsureOrgAccountPlacementSchema(s.db)
}

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

// OrgParent is a ListParents entry (root or OU).
type OrgParent struct {
	ID   string
	Type string // ROOT or ORGANIZATIONAL_UNIT
}

// ListOrgPolicies returns all organization policies, optionally filtered by store type (SCP or RCP).
func (s *Store) ListOrgPolicies(policyType string) ([]OrgPolicy, error) {
	var rows *sql.Rows
	var err error
	policyType = strings.TrimSpace(policyType)
	if policyType != "" {
		rows, err = s.db.Query(
			`SELECT id, type, name, document FROM org_policies WHERE type = ? ORDER BY name`,
			policyType,
		)
	} else {
		rows, err = s.db.Query(
			`SELECT id, type, name, document FROM org_policies ORDER BY name`,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("list org policies: %w", err)
	}
	defer rows.Close()

	var out []OrgPolicy
	for rows.Next() {
		var p OrgPolicy
		if err := rows.Scan(&p.ID, &p.Type, &p.Name, &p.Document); err != nil {
			return nil, fmt.Errorf("list org policies: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list org policies: %w", err)
	}
	if out == nil {
		out = []OrgPolicy{}
	}
	return out, nil
}

// ListAccountsForParent returns member accounts placed directly under parentID (root or OU).
func (s *Store) ListAccountsForParent(parentID string) ([]Account, error) {
	if parentID == "" {
		return nil, fmt.Errorf("list accounts for parent: parent_id required")
	}
	if err := s.validateOrgParentID(parentID); err != nil {
		return nil, fmt.Errorf("list accounts for parent: %w", err)
	}
	accounts, err := s.ListAccounts()
	if err != nil {
		return nil, err
	}
	var out []Account
	for _, a := range accounts {
		pid, err := s.AccountParentID(a.AccountID)
		if err != nil {
			return nil, err
		}
		if pid == parentID {
			out = append(out, a)
		}
	}
	if out == nil {
		out = []Account{}
	}
	return out, nil
}

// ListParents returns root and OU ancestors for an account or organizational unit id.
func (s *Store) ListParents(childID string) ([]OrgParent, error) {
	childID = strings.TrimSpace(childID)
	if childID == "" {
		return nil, fmt.Errorf("list parents: child_id required")
	}
	if childID == OrgRootID || strings.HasPrefix(childID, "r-") {
		return []OrgParent{}, nil
	}

	var current string
	var err error
	switch {
	case strings.HasPrefix(childID, "ou-"):
		current, err = s.ouParentID(childID)
	case len(childID) == 12:
		if err := validate.AccountID(childID); err != nil {
			return nil, fmt.Errorf("list parents: child_id must be a root, OU, or account id")
		}
		current, err = s.AccountParentID(childID)
	default:
		return nil, fmt.Errorf("list parents: child_id must be a root, OU, or account id")
	}
	if err != nil {
		return nil, fmt.Errorf("list parents: %w", err)
	}

	var parents []OrgParent
	for current != "" {
		if current == OrgRootID || strings.HasPrefix(current, "r-") {
			parents = append(parents, OrgParent{ID: current, Type: "ROOT"})
			break
		}
		if !strings.HasPrefix(current, "ou-") {
			return nil, fmt.Errorf("list parents: invalid parent %s", current)
		}
		parents = append(parents, OrgParent{ID: current, Type: "ORGANIZATIONAL_UNIT"})
		current, err = s.ouParentID(current)
		if err != nil {
			return nil, fmt.Errorf("list parents: %w", err)
		}
	}
	if parents == nil {
		parents = []OrgParent{}
	}
	return parents, nil
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

// AccountParentID returns the Organizations parent (root or OU) for accountID.
// Missing placement rows default to OrgRootID (CreateAccount members start under root).
func (s *Store) AccountParentID(accountID string) (string, error) {
	if accountID == "" {
		return "", fmt.Errorf("account parent: account_id required")
	}
	var parentID string
	err := s.db.QueryRow(
		`SELECT parent_id FROM org_account_parents WHERE account_id = ?`,
		accountID,
	).Scan(&parentID)
	if errors.Is(err, sql.ErrNoRows) {
		return OrgRootID, nil
	}
	if err != nil {
		return "", fmt.Errorf("account parent for %s: %w", accountID, err)
	}
	if parentID == "" {
		return OrgRootID, nil
	}
	return parentID, nil
}

// SetAccountParent places accountID directly under parentID (root or OU).
// Used by CreateMemberAccount and MoveAccount.
func (s *Store) SetAccountParent(accountID, parentID string) error {
	if accountID == "" {
		return fmt.Errorf("set account parent: account_id required")
	}
	if parentID == "" {
		return fmt.Errorf("set account parent: parent_id required")
	}
	if err := s.validateOrgParentID(parentID); err != nil {
		return err
	}
	_, err := s.db.Exec(
		`INSERT INTO org_account_parents (account_id, parent_id) VALUES (?, ?)
		 ON CONFLICT(account_id) DO UPDATE SET parent_id = excluded.parent_id`,
		accountID, parentID,
	)
	if err != nil {
		return fmt.Errorf("set account parent %s under %s: %w", accountID, parentID, err)
	}
	return nil
}

// MoveAccount moves accountID from sourceParentID to destinationParentID.
// SourceParentID must match the account's current parent (default OrgRootID).
func (s *Store) MoveAccount(accountID, sourceParentID, destinationParentID string) error {
	if accountID == "" || sourceParentID == "" || destinationParentID == "" {
		return fmt.Errorf("move account: AccountId, SourceParentId, and DestinationParentId are required")
	}
	if !s.AccountExists(accountID) {
		return fmt.Errorf("move account: account not found")
	}
	if err := s.validateOrgParentID(sourceParentID); err != nil {
		return fmt.Errorf("move account source: %w", err)
	}
	if err := s.validateOrgParentID(destinationParentID); err != nil {
		return fmt.Errorf("move account destination: %w", err)
	}
	current, err := s.AccountParentID(accountID)
	if err != nil {
		return err
	}
	if current != sourceParentID {
		return fmt.Errorf("move account: source parent mismatch (current %s)", current)
	}
	if current == destinationParentID {
		return fmt.Errorf("move account: account already under destination")
	}
	return s.SetAccountParent(accountID, destinationParentID)
}

// OUPathToRoot returns OU ids from the account's parent up to (but not including) root.
// Order is nearest parent first. Empty when the account sits directly under root.
func (s *Store) OUPathToRoot(accountID string) ([]string, error) {
	parent, err := s.AccountParentID(accountID)
	if err != nil {
		return nil, err
	}
	var path []string
	seen := map[string]bool{}
	for parent != "" && parent != OrgRootID && !strings.HasPrefix(parent, "r-") {
		if seen[parent] {
			return nil, fmt.Errorf("ou path for %s: cycle at %s", accountID, parent)
		}
		seen[parent] = true
		path = append(path, parent)
		next, err := s.ouParentID(parent)
		if err != nil {
			return nil, err
		}
		parent = next
	}
	if path == nil {
		path = []string{}
	}
	return path, nil
}

func (s *Store) ouParentID(ouID string) (string, error) {
	var parentID string
	err := s.db.QueryRow(`SELECT parent_id FROM org_ous WHERE id = ?`, ouID).Scan(&parentID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("ou parent: organizational unit %s not found", ouID)
	}
	if err != nil {
		return "", fmt.Errorf("ou parent for %s: %w", ouID, err)
	}
	return parentID, nil
}

func (s *Store) validateOrgParentID(parentID string) error {
	if parentID == OrgRootID || strings.HasPrefix(parentID, "r-") {
		return nil
	}
	if !strings.HasPrefix(parentID, "ou-") {
		return fmt.Errorf("parent id must be a root or OU id")
	}
	var id string
	err := s.db.QueryRow(`SELECT id FROM org_ous WHERE id = ?`, parentID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("organizational unit %s not found", parentID)
	}
	if err != nil {
		return fmt.Errorf("validate parent %s: %w", parentID, err)
	}
	return nil
}

// SCPDocsForAccount returns SCP documents that apply to accountID.
// Includes policies attached to the account, each OU on the path to root, and the organization root.
func (s *Store) SCPDocsForAccount(accountID string) ([]string, error) {
	return s.orgPolicyDocsForAccount(accountID, "SCP")
}

// RCPDocsForAccount returns RCP documents that apply to accountID.
func (s *Store) RCPDocsForAccount(accountID string) ([]string, error) {
	return s.orgPolicyDocsForAccount(accountID, "RCP")
}

// OrgFilterDocsForAccount returns SCP and RCP documents for accountID using a
// single OUPathToRoot walk. Path or query errors propagate (callers Deny).
func (s *Store) OrgFilterDocsForAccount(accountID string) (scp, rcp []string, err error) {
	ouPath, err := s.OUPathToRoot(accountID)
	if err != nil {
		return nil, nil, fmt.Errorf("org filter docs for account %s: %w", accountID, err)
	}
	scp, err = s.orgPolicyDocsForAccountWithPath(accountID, "SCP", ouPath)
	if err != nil {
		return nil, nil, err
	}
	rcp, err = s.orgPolicyDocsForAccountWithPath(accountID, "RCP", ouPath)
	if err != nil {
		return nil, nil, err
	}
	return scp, rcp, nil
}

func (s *Store) orgPolicyDocsForAccount(accountID, policyType string) ([]string, error) {
	ouPath, err := s.OUPathToRoot(accountID)
	if err != nil {
		return nil, fmt.Errorf("%s docs for account %s: %w", policyType, accountID, err)
	}
	return s.orgPolicyDocsForAccountWithPath(accountID, policyType, ouPath)
}

func (s *Store) orgPolicyDocsForAccountWithPath(accountID, policyType string, ouPath []string) ([]string, error) {
	args := []any{policyType, accountID, OrgRootID}
	ouClause := ""
	if len(ouPath) > 0 {
		ph := make([]string, len(ouPath))
		for i, ouID := range ouPath {
			ph[i] = "?"
			args = append(args, ouID)
		}
		ouClause = " OR (a.target_type = 'ou' AND a.target_id IN (" + strings.Join(ph, ",") + "))"
	}

	q := `SELECT DISTINCT p.id, p.document
		 FROM org_policies p
		 JOIN org_policy_attachments a ON a.policy_id = p.id
		 WHERE p.type = ?
		   AND (
		     (a.target_type = 'account' AND a.target_id = ?)
		     OR (a.target_type = 'root' AND a.target_id = ?)` + ouClause + `
		   )
		 ORDER BY p.id`
	rows, err := s.db.Query(q, args...)
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
