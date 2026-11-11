package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/validate"
)

const standaloneAccountSchema = `
CREATE TABLE IF NOT EXISTS standalone_accounts (
  seed_name TEXT PRIMARY KEY,
  account_id TEXT NOT NULL,
  user_name TEXT NOT NULL,
  access_key_id TEXT NOT NULL
);
`

const standaloneAdminUser = "admin"
const standaloneAdminPolicy = "AdministratorAccess"

const standaloneAdminDocument = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"*","Resource":"*"}]}`

// StandaloneAccount is an account outside the lab organization with one
// IAM user access key. IsRoot on that key is false.
type StandaloneAccount struct {
	SeedName        string
	AccountID       string
	UserName        string
	UserARN         string
	AccessKeyID     string
	SecretAccessKey string
}

// EnsureStandaloneAccountSchema creates the seed-name table.
func EnsureStandaloneAccountSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure standalone account schema: db is nil")
	}
	if _, err := db.Exec(standaloneAccountSchema); err != nil {
		return fmt.Errorf("ensure standalone account schema: %w", err)
	}
	return nil
}

// SeedStandaloneAccount creates or returns the named account. The account is
// not an organization member and has no OrganizationAccountAccessRole.
// avoidSecret, when set, is never stored as the user secret.
func (s *Store) SeedStandaloneAccount(seedName, avoidSecret string) (StandaloneAccount, error) {
	if err := validate.IAMName(seedName); err != nil {
		return StandaloneAccount{}, fmt.Errorf("seed standalone account: %w", err)
	}
	if err := EnsureStandaloneAccountSchema(s.db); err != nil {
		return StandaloneAccount{}, err
	}
	existing, ok, err := s.lookupStandaloneAccount(seedName)
	if err != nil {
		return StandaloneAccount{}, err
	}
	if ok {
		return existing, nil
	}

	accountID, err := s.allocateStandaloneAccountID()
	if err != nil {
		return StandaloneAccount{}, err
	}
	if err := s.InsertAccount(accountID, OrgOutsideID); err != nil {
		return StandaloneAccount{}, fmt.Errorf("seed standalone account: %w", err)
	}
	_, userARN, err := s.CreateUser(accountID, standaloneAdminUser)
	if err != nil {
		return StandaloneAccount{}, fmt.Errorf("seed standalone account: %w", err)
	}
	if err := s.PutInlinePolicy(userARN, standaloneAdminPolicy, standaloneAdminDocument); err != nil {
		return StandaloneAccount{}, fmt.Errorf("seed standalone account: %w", err)
	}
	accessKeyID, secret, err := s.mintUserKeyAvoiding(accountID, standaloneAdminUser, avoidSecret)
	if err != nil {
		return StandaloneAccount{}, err
	}
	res, err := s.db.Exec(
		`INSERT INTO standalone_accounts (seed_name, account_id, user_name, access_key_id)
		 VALUES (?, ?, ?, ?)`,
		seedName, accountID, standaloneAdminUser, accessKeyID,
	)
	if err != nil {
		return StandaloneAccount{}, fmt.Errorf("seed standalone account: record seed: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return StandaloneAccount{}, err
	}
	if n == 0 {
		return s.mustLookupStandalone(seedName)
	}
	return StandaloneAccount{
		SeedName:        seedName,
		AccountID:       accountID,
		UserName:        standaloneAdminUser,
		UserARN:         userARN,
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secret,
	}, nil
}

func (s *Store) mustLookupStandalone(seedName string) (StandaloneAccount, error) {
	got, ok, err := s.lookupStandaloneAccount(seedName)
	if err != nil {
		return StandaloneAccount{}, err
	}
	if !ok {
		return StandaloneAccount{}, fmt.Errorf("seed standalone account: %s missing after insert", seedName)
	}
	return got, nil
}

func (s *Store) lookupStandaloneAccount(seedName string) (StandaloneAccount, bool, error) {
	var accountID, userName, accessKeyID string
	err := s.db.QueryRow(
		`SELECT account_id, user_name, access_key_id FROM standalone_accounts WHERE seed_name = ?`,
		seedName,
	).Scan(&accountID, &userName, &accessKeyID)
	if errors.Is(err, sql.ErrNoRows) {
		return StandaloneAccount{}, false, nil
	}
	if err != nil {
		return StandaloneAccount{}, false, fmt.Errorf("seed standalone account: lookup %s: %w", seedName, err)
	}
	ak, err := s.LookupAccessKeyRecord(accessKeyID)
	if err != nil {
		return StandaloneAccount{}, false, fmt.Errorf("seed standalone account: load key: %w", err)
	}
	if ak.IsRoot || ak.AccountID != accountID {
		return StandaloneAccount{}, false, fmt.Errorf("seed standalone account: stored key is not a user key for %s", accountID)
	}
	return StandaloneAccount{
		SeedName:        seedName,
		AccountID:       accountID,
		UserName:        userName,
		UserARN:         UserARN(accountID, "/", userName),
		AccessKeyID:     accessKeyID,
		SecretAccessKey: ak.Secret,
	}, true, nil
}

func (s *Store) mintUserKeyAvoiding(accountID, userName, avoidSecret string) (string, string, error) {
	for i := 0; i < 5; i++ {
		accessKeyID, secret, err := s.CreateUserAccessKey(accountID, userName)
		if err != nil {
			return "", "", fmt.Errorf("seed standalone account: %w", err)
		}
		if avoidSecret == "" || secret != avoidSecret {
			return accessKeyID, secret, nil
		}
		if err := s.DeleteAccessKey(accessKeyID); err != nil {
			return "", "", fmt.Errorf("seed standalone account: discard colliding secret: %w", err)
		}
	}
	return "", "", fmt.Errorf("seed standalone account: access key secret collided with the rejected secret")
}

// InsertAccount records an account. parentID empty leaves the placement row
// absent, which AccountParentID treats as the organization root. A non-empty
// parent is stored as given, including OrgOutsideID. SetAccountParent still
// rejects parents that are not a root or an existing organizational unit.
func (s *Store) InsertAccount(accountID, parentID string) error {
	if err := validate.AccountID(accountID); err != nil {
		return fmt.Errorf("insert account: %w", err)
	}
	if _, err := s.db.Exec(`INSERT INTO accounts (account_id) VALUES (?)`, accountID); err != nil {
		return fmt.Errorf("insert account %s: %w", accountID, err)
	}
	parentID = strings.TrimSpace(parentID)
	if parentID == "" {
		return nil
	}
	if _, err := s.db.Exec(
		`INSERT INTO org_account_parents (account_id, parent_id) VALUES (?, ?)`,
		accountID, parentID,
	); err != nil {
		return fmt.Errorf("insert account parent %s: %w", accountID, err)
	}
	return nil
}

func (s *Store) allocateStandaloneAccountID() (string, error) {
	for i := 0; i < 64; i++ {
		var b [8]byte
		if _, err := rand.Read(b[:]); err != nil {
			return "", fmt.Errorf("seed standalone account: %w", err)
		}
		n := binary.BigEndian.Uint64(b[:]) % 1_000_000_000_000
		if n == 0 {
			continue
		}
		id := fmt.Sprintf("%012d", n)
		if id == ManagementAccountID {
			continue
		}
		if s.AccountExists(id) {
			continue
		}
		return id, nil
	}
	return "", fmt.Errorf("seed standalone account: exhausted account id attempts")
}
