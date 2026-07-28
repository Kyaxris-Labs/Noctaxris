package store

import (
	"database/sql"
	"fmt"

	"github.com/Kyaxris-Labs/Noctaxris/internal/validate"
)

// Group is an IAM group record.
type Group struct {
	AccountID string
	GroupName string
	GroupID   string
	ARN       string
}

// CreateGroup creates an IAM group in accountID.
func (s *Store) CreateGroup(accountID, groupName string) (groupID, arn string, err error) {
	if err := validate.AccountID(accountID); err != nil {
		return "", "", fmt.Errorf("create group: %w", err)
	}
	if err := validate.IAMName(groupName); err != nil {
		return "", "", fmt.Errorf("create group: %w", err)
	}
	groupID, err = newIAMResourceID("AGPA")
	if err != nil {
		return "", "", err
	}
	arn = GroupARN(accountID, "/", groupName)
	_, err = s.db.Exec(
		`INSERT INTO iam_groups (account_id, group_name, group_id, arn) VALUES (?, ?, ?, ?)`,
		accountID, groupName, groupID, arn,
	)
	if err != nil {
		return "", "", fmt.Errorf("create group %s/%s: %w", accountID, groupName, err)
	}
	return groupID, arn, nil
}

// GetGroup returns an IAM group.
func (s *Store) GetGroup(accountID, groupName string) (Group, error) {
	var g Group
	err := s.db.QueryRow(
		`SELECT account_id, group_name, group_id, arn FROM iam_groups
		 WHERE account_id = ? AND group_name = ?`,
		accountID, groupName,
	).Scan(&g.AccountID, &g.GroupName, &g.GroupID, &g.ARN)
	if err != nil {
		return Group{}, err
	}
	return g, nil
}

// DeleteGroup deletes an IAM group and its memberships.
func (s *Store) DeleteGroup(accountID, groupName string) error {
	g, err := s.GetGroup(accountID, groupName)
	if err != nil {
		return fmt.Errorf("delete group: %w", err)
	}
	if _, err := s.db.Exec(
		`DELETE FROM iam_group_memberships WHERE account_id = ? AND group_name = ?`,
		accountID, groupName,
	); err != nil {
		return fmt.Errorf("delete group memberships %s/%s: %w", accountID, groupName, err)
	}
	if _, err := s.db.Exec(`DELETE FROM policy_attachments WHERE principal_arn = ?`, g.ARN); err != nil {
		return fmt.Errorf("delete group attachments %s: %w", g.ARN, err)
	}
	if _, err := s.db.Exec(`DELETE FROM inline_policies WHERE principal_arn = ?`, g.ARN); err != nil {
		return fmt.Errorf("delete group inline policies %s: %w", g.ARN, err)
	}
	res, err := s.db.Exec(
		`DELETE FROM iam_groups WHERE account_id = ? AND group_name = ?`,
		accountID, groupName,
	)
	if err != nil {
		return fmt.Errorf("delete group %s/%s: %w", accountID, groupName, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete group %s/%s: %w", accountID, groupName, err)
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ListGroups returns all IAM groups in accountID.
func (s *Store) ListGroups(accountID string) ([]Group, error) {
	rows, err := s.db.Query(
		`SELECT account_id, group_name, group_id, arn FROM iam_groups
		 WHERE account_id = ? ORDER BY group_name`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list groups %s: %w", accountID, err)
	}
	defer rows.Close()

	var out []Group
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.AccountID, &g.GroupName, &g.GroupID, &g.ARN); err != nil {
			return nil, fmt.Errorf("list groups %s: %w", accountID, err)
		}
		out = append(out, g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list groups %s: %w", accountID, err)
	}
	if out == nil {
		out = []Group{}
	}
	return out, nil
}

// AddUserToGroup adds userName to groupName in accountID.
func (s *Store) AddUserToGroup(accountID, groupName, userName string) error {
	if _, err := s.GetGroup(accountID, groupName); err != nil {
		return fmt.Errorf("add user to group: %w", err)
	}
	if _, err := s.GetUser(accountID, userName); err != nil {
		return fmt.Errorf("add user to group: %w", err)
	}
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO iam_group_memberships (account_id, group_name, user_name) VALUES (?, ?, ?)`,
		accountID, groupName, userName,
	)
	if err != nil {
		return fmt.Errorf("add user %s to group %s/%s: %w", userName, accountID, groupName, err)
	}
	return nil
}

// RemoveUserFromGroup removes userName from groupName in accountID.
func (s *Store) RemoveUserFromGroup(accountID, groupName, userName string) error {
	if _, err := s.GetGroup(accountID, groupName); err != nil {
		return fmt.Errorf("remove user from group: %w", err)
	}
	res, err := s.db.Exec(
		`DELETE FROM iam_group_memberships WHERE account_id = ? AND group_name = ? AND user_name = ?`,
		accountID, groupName, userName,
	)
	if err != nil {
		return fmt.Errorf("remove user %s from group %s/%s: %w", userName, accountID, groupName, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("remove user %s from group %s/%s: %w", userName, accountID, groupName, err)
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ListGroupMembers returns users that belong to groupName.
func (s *Store) ListGroupMembers(accountID, groupName string) ([]User, error) {
	if _, err := s.GetGroup(accountID, groupName); err != nil {
		return nil, fmt.Errorf("list group members: %w", err)
	}
	rows, err := s.db.Query(
		`SELECT u.account_id, u.user_name, u.user_id, u.arn
		 FROM iam_group_memberships m
		 JOIN users u ON u.account_id = m.account_id AND u.user_name = m.user_name
		 WHERE m.account_id = ? AND m.group_name = ?
		 ORDER BY u.user_name`,
		accountID, groupName,
	)
	if err != nil {
		return nil, fmt.Errorf("list group members %s/%s: %w", accountID, groupName, err)
	}
	defer rows.Close()

	var out []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.AccountID, &u.UserName, &u.UserID, &u.ARN); err != nil {
			return nil, fmt.Errorf("list group members %s/%s: %w", accountID, groupName, err)
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list group members %s/%s: %w", accountID, groupName, err)
	}
	if out == nil {
		out = []User{}
	}
	return out, nil
}

// ListGroupsForUser returns groups that include userName.
func (s *Store) ListGroupsForUser(accountID, userName string) ([]Group, error) {
	rows, err := s.db.Query(
		`SELECT g.account_id, g.group_name, g.group_id, g.arn
		 FROM iam_group_memberships m
		 JOIN iam_groups g ON g.account_id = m.account_id AND g.group_name = m.group_name
		 WHERE m.account_id = ? AND m.user_name = ?
		 ORDER BY g.group_name`,
		accountID, userName,
	)
	if err != nil {
		return nil, fmt.Errorf("list groups for user %s/%s: %w", accountID, userName, err)
	}
	defer rows.Close()

	var out []Group
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.AccountID, &g.GroupName, &g.GroupID, &g.ARN); err != nil {
			return nil, fmt.Errorf("list groups for user %s/%s: %w", accountID, userName, err)
		}
		out = append(out, g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list groups for user %s/%s: %w", accountID, userName, err)
	}
	if out == nil {
		out = []Group{}
	}
	return out, nil
}

// AttachGroupPolicy attaches a managed policy ARN to a group via policy_attachments.
func (s *Store) AttachGroupPolicy(accountID, groupName, policyARN string) error {
	g, err := s.GetGroup(accountID, groupName)
	if err != nil {
		return fmt.Errorf("attach group policy: %w", err)
	}
	return s.AttachPolicy(g.ARN, policyARN)
}

// DetachGroupPolicy detaches a managed policy ARN from a group.
func (s *Store) DetachGroupPolicy(accountID, groupName, policyARN string) error {
	g, err := s.GetGroup(accountID, groupName)
	if err != nil {
		return fmt.Errorf("detach group policy: %w", err)
	}
	return s.detachPolicy(g.ARN, policyARN)
}

// ListGroupPolicies returns managed policy ARNs attached to the group.
func (s *Store) ListGroupPolicies(accountID, groupName string) ([]string, error) {
	g, err := s.GetGroup(accountID, groupName)
	if err != nil {
		return nil, fmt.Errorf("list group policies: %w", err)
	}
	refs, err := s.ListAttachedPolicyRefs(g.ARN)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		out = append(out, ref.PolicyARN)
	}
	return out, nil
}

// PutGroupInlinePolicy stores an inline policy on a group (reuses inline_policies by group ARN).
func (s *Store) PutGroupInlinePolicy(accountID, groupName, policyName, document string) error {
	g, err := s.GetGroup(accountID, groupName)
	if err != nil {
		return fmt.Errorf("put group inline policy: %w", err)
	}
	return s.PutInlinePolicy(g.ARN, policyName, document)
}

// IdentityPolicyDocsForUser returns identity policy documents for the user plus
// attached and inline policies from every group the user belongs to.
// Reads run under a single deferred transaction (not BEGIN IMMEDIATE).
// Attached managed policies use INNER JOIN to policies (omit missing synced rows).
func (s *Store) IdentityPolicyDocsForUser(accountID, userName string) ([]string, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("identity policy docs for user: begin: %w", err)
	}
	defer tx.Rollback()

	var userARN string
	err = tx.QueryRow(
		`SELECT arn FROM users WHERE account_id = ? AND user_name = ?`,
		accountID, userName,
	).Scan(&userARN)
	if err != nil {
		return nil, fmt.Errorf("identity policy docs for user: %w", err)
	}

	rows, err := tx.Query(
		`SELECT p.document
		 FROM policy_attachments a
		 JOIN policies p ON p.policy_id = a.policy_id
		 WHERE a.principal_arn = ?
		 UNION ALL
		 SELECT document
		 FROM inline_policies
		 WHERE principal_arn = ?
		 UNION ALL
		 SELECT p.document
		 FROM iam_group_memberships m
		 JOIN iam_groups g ON g.account_id = m.account_id AND g.group_name = m.group_name
		 JOIN policy_attachments a ON a.principal_arn = g.arn
		 JOIN policies p ON p.policy_id = a.policy_id
		 WHERE m.account_id = ? AND m.user_name = ?
		 UNION ALL
		 SELECT i.document
		 FROM iam_group_memberships m
		 JOIN iam_groups g ON g.account_id = m.account_id AND g.group_name = m.group_name
		 JOIN inline_policies i ON i.principal_arn = g.arn
		 WHERE m.account_id = ? AND m.user_name = ?`,
		userARN, userARN, accountID, userName, accountID, userName,
	)
	if err != nil {
		return nil, fmt.Errorf("identity policy docs for user: %w", err)
	}

	var docs []string
	for rows.Next() {
		var doc string
		if err := rows.Scan(&doc); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("identity policy docs for user: %w", err)
		}
		docs = append(docs, doc)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("identity policy docs for user: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("identity policy docs for user: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("identity policy docs for user: commit: %w", err)
	}
	if docs == nil {
		docs = []string{}
	}
	return docs, nil
}

// IdentityPolicyDocsForUserSequentialForTest exposes the pre-batch sequential
// reader for equivalence tests.
func (s *Store) IdentityPolicyDocsForUserSequentialForTest(accountID, userName string) ([]string, error) {
	return s.identityPolicyDocsForUserSequential(accountID, userName)
}

// identityPolicyDocsForUserSequential is the pre-batch reader retained for
// equivalence checks against the batched path.
func (s *Store) identityPolicyDocsForUserSequential(accountID, userName string) ([]string, error) {
	u, err := s.GetUser(accountID, userName)
	if err != nil {
		return nil, fmt.Errorf("identity policy docs for user: %w", err)
	}
	var docs []string

	attached, err := s.ListAttachedPolicyDocuments(u.ARN)
	if err != nil {
		return nil, err
	}
	docs = append(docs, attached...)

	inlines, err := s.ListInlinePolicies(u.ARN)
	if err != nil {
		return nil, err
	}
	for _, p := range inlines {
		docs = append(docs, p.Document)
	}

	groups, err := s.ListGroupsForUser(accountID, userName)
	if err != nil {
		return nil, err
	}
	for _, g := range groups {
		gAttached, err := s.ListAttachedPolicyDocuments(g.ARN)
		if err != nil {
			return nil, err
		}
		docs = append(docs, gAttached...)
		gInlines, err := s.ListInlinePolicies(g.ARN)
		if err != nil {
			return nil, err
		}
		for _, p := range gInlines {
			docs = append(docs, p.Document)
		}
	}

	if docs == nil {
		docs = []string{}
	}
	return docs, nil
}
