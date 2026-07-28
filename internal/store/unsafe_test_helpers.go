package store

import "fmt"

// UnsafeDeleteUserRowForTest deletes the users row without cleaning access keys.
// Used only by IdentityLoadFailClosed* tests to force identity load errors
// while SigV4 still verifies via the access_keys row.
func (s *Store) UnsafeDeleteUserRowForTest(accountID, userName string) error {
	_, err := s.db.Exec(`DELETE FROM users WHERE account_id = ? AND user_name = ?`, accountID, userName)
	if err != nil {
		return fmt.Errorf("unsafe delete user row %s/%s: %w", accountID, userName, err)
	}
	return nil
}

// UnsafeSetOUParentForTest sets org_ous.parent_id (can create cycles for OUPathToRoot tests).
func (s *Store) UnsafeSetOUParentForTest(ouID, parentID string) error {
	_, err := s.db.Exec(`UPDATE org_ous SET parent_id = ? WHERE id = ?`, parentID, ouID)
	if err != nil {
		return fmt.Errorf("unsafe set ou parent %s: %w", ouID, err)
	}
	return nil
}
