package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
)

// OrganizationAccountAccessRoleName is the IAM role created in member accounts.
const OrganizationAccountAccessRoleName = "OrganizationAccountAccessRole"

// OrganizationAccountAccessRoleTrustPolicy returns the trust policy JSON that
// allows the management account root to assume OrganizationAccountAccessRole.
func OrganizationAccountAccessRoleTrustPolicy(mgmtAccountID string) string {
	return fmt.Sprintf(
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::%s:root"},"Action":"sts:AssumeRole","Resource":"*"}]}`,
		mgmtAccountID,
	)
}

// RoleARN builds an IAM role ARN for the given account and role name.
func RoleARN(accountID, roleName string) string {
	return fmt.Sprintf("arn:aws:iam::%s:role/%s", accountID, roleName)
}

// CreateMemberAccount creates a member account under mgmtAccountID with
// OrganizationAccountAccessRole and a SUCCEEDED create-account request.
func (s *Store) CreateMemberAccount(mgmtAccountID, email, accountName string) (requestID, accountID string, err error) {
	accountID, err = s.allocateAccountID(mgmtAccountID)
	if err != nil {
		return "", "", err
	}
	requestID, err = newCreateAccountRequestID()
	if err != nil {
		return "", "", err
	}
	trust := OrganizationAccountAccessRoleTrustPolicy(mgmtAccountID)
	roleARN := RoleARN(accountID, OrganizationAccountAccessRoleName)

	tx, err := s.db.Begin()
	if err != nil {
		return "", "", err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`INSERT INTO accounts (account_id) VALUES (?)`, accountID); err != nil {
		return "", "", fmt.Errorf("insert account %s: %w", accountID, err)
	}
	if _, err := tx.Exec(
		`INSERT INTO create_account_requests
		 (request_id, status, account_name, email, account_id, requested_by_account_id, failure_reason)
		 VALUES (?, 'SUCCEEDED', ?, ?, ?, ?, NULL)`,
		requestID, accountName, email, accountID, mgmtAccountID,
	); err != nil {
		return "", "", fmt.Errorf("insert create_account_request: %w", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO roles (account_id, role_name, role_arn, trust_policy) VALUES (?, ?, ?, ?)`,
		accountID, OrganizationAccountAccessRoleName, roleARN, trust,
	); err != nil {
		return "", "", fmt.Errorf("insert OrganizationAccountAccessRole: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", "", err
	}
	return requestID, accountID, nil
}

// DescribeCreateAccountStatus returns the status of a CreateAccount request.
func (s *Store) DescribeCreateAccountStatus(requestID string) (status, accountID, email, failure string, err error) {
	var (
		acc  sql.NullString
		em   sql.NullString
		fail sql.NullString
		st   string
	)
	err = s.db.QueryRow(
		`SELECT status, account_id, email, failure_reason FROM create_account_requests WHERE request_id = ?`,
		requestID,
	).Scan(&st, &acc, &em, &fail)
	if err != nil {
		return "", "", "", "", err
	}
	return st, acc.String, em.String, fail.String, nil
}

func (s *Store) allocateAccountID(mgmtAccountID string) (string, error) {
	const start int64 = 2
	for n := start; n < 1_000_000_000_000; n++ {
		id := fmt.Sprintf("%012d", n)
		if id == mgmtAccountID {
			continue
		}
		var existing string
		err := s.db.QueryRow(`SELECT account_id FROM accounts WHERE account_id = ?`, id).Scan(&existing)
		if errors.Is(err, sql.ErrNoRows) {
			return id, nil
		}
		if err != nil {
			return "", fmt.Errorf("allocate account id: %w", err)
		}
	}
	return "", fmt.Errorf("exhausted account id space")
}

func newCreateAccountRequestID() (string, error) {
	var b [10]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "car-" + hex.EncodeToString(b[:]), nil
}
