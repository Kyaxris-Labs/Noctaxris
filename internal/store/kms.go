package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	// DefaultKMSRegion is the lab region embedded in KMS ARNs.
	DefaultKMSRegion = "us-east-1"

	KeyStateEnabled         = "Enabled"
	KeyStateDisabled        = "Disabled"
	KeyStatePendingDeletion = "PendingDeletion"

	KeyUsageEncryptDecrypt = "ENCRYPT_DECRYPT"

	cmkMaterialSize = 32

	defaultPendingWindowDays = 30
	minPendingWindowDays     = 7
	maxPendingWindowDays     = 30

	// Lab convenience aliases approximating AWS-managed service keys.
	AliasAWSS3       = "alias/aws/s3"
	AliasAWSDynamoDB = "alias/aws/dynamodb"
	AliasAWSSQS      = "alias/aws/sqs"
)

// awsManagedConvenienceAliases are lab stand-ins for AWS-managed KMS key aliases.
var awsManagedConvenienceAliases = []string{AliasAWSS3, AliasAWSDynamoDB, AliasAWSSQS}

// ErrInvalidKeyState is returned when a KMS key is not in a compatible state.
var ErrInvalidKeyState = errors.New("invalid key state")

// ErrUnsupportedKeyRotation is returned when rotation is not allowed for the key.
var ErrUnsupportedKeyRotation = errors.New("key rotation not supported")

// Key is a customer-managed KMS key record (material remains sealed).
type Key struct {
	KeyID               string
	AccountID           string
	ARN                 string
	KeyState            string
	KeyUsage            string
	KeyPolicy           string
	CreationDate        string
	DeletionDate        string
	PendingWindowInDays int
	KeyRotationEnabled  bool
	LastRotationDate    string
	RotationPeriodDays  int
}

// Alias is a KMS alias pointing at a CMK.
type Alias struct {
	AliasName   string
	AccountID   string
	TargetKeyID string
}

// Grant is a KMS grant on a CMK.
type Grant struct {
	GrantID           string
	KeyID             string
	AccountID         string
	GranteePrincipal  string
	RetiringPrincipal string
	Operations        []string
	Name              string
}

// DefaultKeyPolicy returns the lab default key policy JSON allowing account root
// (and the creator IAM user, when creatorPrincipalARN is a user ARN) kms:* on the key.
func DefaultKeyPolicy(accountID, creatorPrincipalARN string) string {
	statements := []map[string]any{
		{
			"Sid":    "Enable IAM User Permissions",
			"Effect": "Allow",
			"Principal": map[string]any{
				"AWS": fmt.Sprintf("arn:aws:iam::%s:root", accountID),
			},
			"Action":   "kms:*",
			"Resource": "*",
		},
	}
	if isIAMUserARN(creatorPrincipalARN) {
		statements = append(statements, map[string]any{
			"Sid":    "Allow Key Creator",
			"Effect": "Allow",
			"Principal": map[string]any{
				"AWS": creatorPrincipalARN,
			},
			"Action":   "kms:*",
			"Resource": "*",
		})
	}
	doc := map[string]any{
		"Version":   "2012-10-17",
		"Id":        "key-default-1",
		"Statement": statements,
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		// Static structure; marshal failure is a programming error.
		return `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"*"},"Action":"kms:*","Resource":"*"}]}`
	}
	return string(raw)
}

func isIAMUserARN(arn string) bool {
	return strings.Contains(arn, ":user/")
}

// KeyARN builds arn:aws:kms:REGION:ACCOUNT:key/KEYID.
func KeyARN(region, accountID, keyID string) string {
	if region == "" {
		region = DefaultKMSRegion
	}
	return fmt.Sprintf("arn:aws:kms:%s:%s:key/%s", region, accountID, keyID)
}

// AliasARN builds arn:aws:kms:REGION:ACCOUNT:alias/NAME (aliasName includes alias/ prefix).
func AliasARN(region, accountID, aliasName string) string {
	if region == "" {
		region = DefaultKMSRegion
	}
	name := strings.TrimPrefix(aliasName, "alias/")
	return fmt.Sprintf("arn:aws:kms:%s:%s:alias/%s", region, accountID, name)
}

// CreateKey mints a CMK, seals 32-byte material under the store master key, and stores it.
func (s *Store) CreateKey(accountID, creatorARN, policyOverride string) (Key, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return Key{}, fmt.Errorf("create key: begin: %w", err)
	}
	defer tx.Rollback()
	k, err := s.createKeyTx(tx, accountID, creatorARN, policyOverride)
	if err != nil {
		return Key{}, err
	}
	if err := tx.Commit(); err != nil {
		return Key{}, fmt.Errorf("create key: commit: %w", err)
	}
	return k, nil
}

// GetKey returns a CMK by key ID. Runs the pending-deletion sweeper first.
func (s *Store) GetKey(keyID string) (Key, error) {
	return s.sweepThenGetKey(keyID)
}

// ListKeys returns CMKs for an account ordered by creation_date.
func (s *Store) ListKeys(accountID string) ([]Key, error) {
	if _, err := s.SweepExpiredPendingKeys(time.Now().UTC()); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT key_id, account_id, arn, key_state, key_usage, key_policy, creation_date,
		        COALESCE(deletion_date, ''), COALESCE(key_rotation_enabled, 0),
		        COALESCE(last_rotation_date, ''), COALESCE(rotation_period_days, 0)
		 FROM kms_keys WHERE account_id = ? ORDER BY creation_date, key_id`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list keys %s: %w", accountID, err)
	}
	defer rows.Close()

	var out []Key
	for rows.Next() {
		var k Key
		var rotation int
		var period int
		if err := rows.Scan(&k.KeyID, &k.AccountID, &k.ARN, &k.KeyState, &k.KeyUsage, &k.KeyPolicy, &k.CreationDate,
			&k.DeletionDate, &rotation, &k.LastRotationDate, &period); err != nil {
			return nil, fmt.Errorf("list keys %s: %w", accountID, err)
		}
		k.KeyRotationEnabled = rotation != 0
		k.RotationPeriodDays = period
		out = append(out, k)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list keys %s: %w", accountID, err)
	}
	if out == nil {
		out = []Key{}
	}
	return out, nil
}

// SetKeyState sets Enabled or Disabled on a CMK.
func (s *Store) SetKeyState(keyID, state string) error {
	switch state {
	case KeyStateEnabled, KeyStateDisabled:
	default:
		return fmt.Errorf("set key state %s: invalid state %q", keyID, state)
	}
	k, err := s.GetKey(keyID)
	if err != nil {
		return err
	}
	if k.KeyState == KeyStatePendingDeletion {
		return fmt.Errorf("set key state %s: %w", keyID, ErrInvalidKeyState)
	}
	res, err := s.db.Exec(`UPDATE kms_keys SET key_state = ? WHERE key_id = ?`, state, keyID)
	if err != nil {
		return fmt.Errorf("set key state %s: %w", keyID, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("set key state %s: %w", keyID, err)
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ScheduleKeyDeletion moves a CMK to PendingDeletion and stores DeletionDate.
// pendingWindowInDays defaults to 30 and is clamped to 7–30.
func (s *Store) ScheduleKeyDeletion(keyID string, pendingWindowInDays int) (Key, error) {
	k, err := s.GetKey(keyID)
	if err != nil {
		return Key{}, err
	}
	switch k.KeyState {
	case KeyStateEnabled, KeyStateDisabled:
	default:
		return Key{}, fmt.Errorf("schedule key deletion %s: %w", keyID, ErrInvalidKeyState)
	}
	days := pendingWindowInDays
	if days <= 0 {
		days = defaultPendingWindowDays
	}
	if days < minPendingWindowDays {
		days = minPendingWindowDays
	}
	if days > maxPendingWindowDays {
		days = maxPendingWindowDays
	}
	deletionDate := time.Now().UTC().Add(time.Duration(days) * 24 * time.Hour).Format(time.RFC3339)
	res, err := s.db.Exec(
		`UPDATE kms_keys SET key_state = ?, deletion_date = ? WHERE key_id = ?`,
		KeyStatePendingDeletion, deletionDate, keyID,
	)
	if err != nil {
		return Key{}, fmt.Errorf("schedule key deletion %s: %w", keyID, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return Key{}, fmt.Errorf("schedule key deletion %s: %w", keyID, err)
	}
	if affected == 0 {
		return Key{}, sql.ErrNoRows
	}
	k.KeyState = KeyStatePendingDeletion
	k.DeletionDate = deletionDate
	k.PendingWindowInDays = days
	return k, nil
}

// CancelKeyDeletion cancels PendingDeletion and sets the key to Disabled.
// Matches AWS KMS: cancel leaves the key Disabled; callers must EnableKey to use it again.
func (s *Store) CancelKeyDeletion(keyID string) error {
	k, err := s.GetKey(keyID)
	if err != nil {
		return err
	}
	if k.KeyState != KeyStatePendingDeletion {
		return fmt.Errorf("cancel key deletion %s: %w", keyID, ErrInvalidKeyState)
	}
	res, err := s.db.Exec(
		`UPDATE kms_keys SET key_state = ?, deletion_date = '' WHERE key_id = ?`,
		KeyStateDisabled, keyID,
	)
	if err != nil {
		return fmt.Errorf("cancel key deletion %s: %w", keyID, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("cancel key deletion %s: %w", keyID, err)
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// SetKeyRotationEnabled persists automatic rotation status for a symmetric ENCRYPT_DECRYPT CMK.
// When enabling, lab rotates sealed key material once immediately so rotation is more than a flag.
func (s *Store) SetKeyRotationEnabled(keyID string, enabled bool) error {
	k, err := s.GetKey(keyID)
	if err != nil {
		return err
	}
	if k.KeyUsage != KeyUsageEncryptDecrypt {
		return fmt.Errorf("set key rotation %s: %w", keyID, ErrUnsupportedKeyRotation)
	}
	if k.KeyState == KeyStatePendingDeletion {
		return fmt.Errorf("set key rotation %s: %w", keyID, ErrInvalidKeyState)
	}
	flag := 0
	if enabled {
		flag = 1
	}
	res, err := s.db.Exec(`UPDATE kms_keys SET key_rotation_enabled = ? WHERE key_id = ?`, flag, keyID)
	if err != nil {
		return fmt.Errorf("set key rotation %s: %w", keyID, err)
	}
	if _, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("set key rotation %s: %w", keyID, err)
	}
	if enabled {
		if err := s.RotateKeyMaterial(keyID); err != nil {
			return err
		}
	}
	return nil
}

// GetKeyRotationEnabled returns whether automatic key rotation is enabled.
func (s *Store) GetKeyRotationEnabled(keyID string) (bool, error) {
	k, err := s.GetKey(keyID)
	if err != nil {
		return false, err
	}
	return k.KeyRotationEnabled, nil
}

// KeyUsableForCrypto reports whether the key may be used for cryptographic operations.
func KeyUsableForCrypto(state string) bool {
	return state == KeyStateEnabled
}

// GetKeyPolicy returns the key policy document for keyID.
func (s *Store) GetKeyPolicy(keyID string) (string, error) {
	var policy string
	err := s.db.QueryRow(`SELECT key_policy FROM kms_keys WHERE key_id = ?`, keyID).Scan(&policy)
	if err != nil {
		return "", err
	}
	return policy, nil
}

// PutKeyPolicy replaces the key policy document.
func (s *Store) PutKeyPolicy(keyID, policy string) error {
	if strings.TrimSpace(policy) == "" {
		return fmt.Errorf("put key policy %s: policy is required", keyID)
	}
	if !json.Valid([]byte(policy)) {
		return fmt.Errorf("put key policy %s: policy is not valid JSON", keyID)
	}
	res, err := s.db.Exec(`UPDATE kms_keys SET key_policy = ? WHERE key_id = ?`, policy, keyID)
	if err != nil {
		return fmt.Errorf("put key policy %s: %w", keyID, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("put key policy %s: %w", keyID, err)
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// UnsealKeyMaterial returns the plaintext CMK bytes for keyID.
func (s *Store) UnsealKeyMaterial(keyID string) ([]byte, error) {
	var sealed []byte
	err := s.db.QueryRow(`SELECT sealed_material FROM kms_keys WHERE key_id = ?`, keyID).Scan(&sealed)
	if err != nil {
		return nil, err
	}
	plain, err := Unseal(s.master, sealed)
	if err != nil {
		return nil, fmt.Errorf("unseal key material %s: %w", keyID, err)
	}
	return plain, nil
}

// CreateAlias creates aliasName (must be alias/...) pointing at targetKeyID.
func (s *Store) CreateAlias(accountID, aliasName, targetKeyID string) error {
	aliasName = normalizeAliasName(aliasName)
	if !strings.HasPrefix(aliasName, "alias/") || aliasName == "alias/" {
		return fmt.Errorf("create alias: invalid alias name %q", aliasName)
	}
	if _, err := s.GetKey(targetKeyID); err != nil {
		return fmt.Errorf("create alias: target key: %w", err)
	}
	_, err := s.db.Exec(
		`INSERT INTO kms_aliases (alias_name, account_id, target_key_id) VALUES (?, ?, ?)`,
		aliasName, accountID, targetKeyID,
	)
	if err != nil {
		return fmt.Errorf("create alias %s: %w", aliasName, err)
	}
	return nil
}

// ListAliases returns aliases for an account.
func (s *Store) ListAliases(accountID string) ([]Alias, error) {
	rows, err := s.db.Query(
		`SELECT alias_name, account_id, target_key_id FROM kms_aliases
		 WHERE account_id = ? ORDER BY alias_name`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list aliases %s: %w", accountID, err)
	}
	defer rows.Close()

	var out []Alias
	for rows.Next() {
		var a Alias
		if err := rows.Scan(&a.AliasName, &a.AccountID, &a.TargetKeyID); err != nil {
			return nil, fmt.Errorf("list aliases %s: %w", accountID, err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list aliases %s: %w", accountID, err)
	}
	if out == nil {
		out = []Alias{}
	}
	return out, nil
}

// DeleteAlias removes an alias.
func (s *Store) DeleteAlias(accountID, aliasName string) error {
	aliasName = normalizeAliasName(aliasName)
	res, err := s.db.Exec(
		`DELETE FROM kms_aliases WHERE account_id = ? AND alias_name = ?`,
		accountID, aliasName,
	)
	if err != nil {
		return fmt.Errorf("delete alias %s: %w", aliasName, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete alias %s: %w", aliasName, err)
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// UpdateAlias retargets an existing alias.
func (s *Store) UpdateAlias(accountID, aliasName, targetKeyID string) error {
	aliasName = normalizeAliasName(aliasName)
	if _, err := s.GetKey(targetKeyID); err != nil {
		return fmt.Errorf("update alias: target key: %w", err)
	}
	res, err := s.db.Exec(
		`UPDATE kms_aliases SET target_key_id = ? WHERE account_id = ? AND alias_name = ?`,
		targetKeyID, accountID, aliasName,
	)
	if err != nil {
		return fmt.Errorf("update alias %s: %w", aliasName, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update alias %s: %w", aliasName, err)
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ResolveKeyID resolves a key id, key ARN, alias name, or alias ARN to a key UUID.
func (s *Store) ResolveKeyID(accountID, keyIdOrAliasOrArn string) (string, error) {
	id := strings.TrimSpace(keyIdOrAliasOrArn)
	if id == "" {
		return "", fmt.Errorf("resolve key id: empty identifier")
	}

	if strings.HasPrefix(id, "arn:aws:kms:") {
		parts := strings.Split(id, ":")
		if len(parts) < 6 {
			return "", fmt.Errorf("resolve key id: invalid ARN %q", id)
		}
		arnAccount := parts[4]
		resource := parts[5]
		if strings.HasPrefix(resource, "key/") {
			keyID := strings.TrimPrefix(resource, "key/")
			key, err := s.GetKey(keyID)
			if err != nil {
				return "", err
			}
			if arnAccount != "" && key.AccountID != arnAccount {
				return "", sql.ErrNoRows
			}
			return keyID, nil
		}
		if strings.HasPrefix(resource, "alias/") {
			aliasAccount := arnAccount
			if aliasAccount == "" {
				aliasAccount = accountID
			}
			return s.resolveAlias(aliasAccount, "alias/"+strings.TrimPrefix(resource, "alias/"))
		}
		return "", fmt.Errorf("resolve key id: unsupported ARN resource %q", resource)
	}

	if strings.HasPrefix(id, "alias/") {
		return s.resolveAlias(accountID, id)
	}

	if _, err := s.GetKey(id); err != nil {
		return "", err
	}
	return id, nil
}

func (s *Store) resolveAlias(accountID, aliasName string) (string, error) {
	aliasName = normalizeAliasName(aliasName)
	keyID, err := s.lookupAliasTarget(accountID, aliasName)
	if err == nil {
		return keyID, nil
	}
	if errors.Is(err, sql.ErrNoRows) && isAWSManagedConvenienceAlias(aliasName) {
		if _, ensErr := s.EnsureAWSManagedConvenienceAliases(accountID); ensErr != nil {
			return "", ensErr
		}
		return s.lookupAliasTarget(accountID, aliasName)
	}
	return "", err
}

func (s *Store) lookupAliasTarget(accountID, aliasName string) (string, error) {
	var keyID string
	err := s.db.QueryRow(
		`SELECT target_key_id FROM kms_aliases WHERE account_id = ? AND alias_name = ?`,
		accountID, aliasName,
	).Scan(&keyID)
	if err != nil {
		return "", err
	}
	return keyID, nil
}

func isAWSManagedConvenienceAlias(aliasName string) bool {
	aliasName = normalizeAliasName(aliasName)
	for _, a := range awsManagedConvenienceAliases {
		if aliasName == a {
			return true
		}
	}
	return false
}

// EnsureAWSManagedConvenienceAliases creates a per-account lab CMK and seeds
// alias/aws/s3, alias/aws/dynamodb, and alias/aws/sqs pointing at it.
// These are lab approximations of AWS-managed keys, not AWS-owned key parity.
// Idempotent: returns the existing target key id when any alias is already present.
func (s *Store) EnsureAWSManagedConvenienceAliases(accountID string) (string, error) {
	if strings.TrimSpace(accountID) == "" {
		return "", fmt.Errorf("ensure aws managed aliases: account id required")
	}

	tx, err := s.db.Begin()
	if err != nil {
		return "", fmt.Errorf("ensure aws managed aliases: begin: %w", err)
	}
	defer tx.Rollback()

	var existingKeyID string
	for _, alias := range awsManagedConvenienceAliases {
		var keyID string
		err := tx.QueryRow(
			`SELECT target_key_id FROM kms_aliases WHERE account_id = ? AND alias_name = ?`,
			accountID, alias,
		).Scan(&keyID)
		if err == nil {
			existingKeyID = keyID
			break
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("ensure aws managed aliases: lookup %s: %w", alias, err)
		}
	}

	keyID := existingKeyID
	if keyID == "" {
		k, createErr := s.createKeyTx(tx, accountID, fmt.Sprintf("arn:aws:iam::%s:root", accountID), "")
		if createErr != nil {
			return "", fmt.Errorf("ensure aws managed aliases: create key: %w", createErr)
		}
		keyID = k.KeyID
	}

	for _, alias := range awsManagedConvenienceAliases {
		var current string
		err := tx.QueryRow(
			`SELECT target_key_id FROM kms_aliases WHERE account_id = ? AND alias_name = ?`,
			accountID, alias,
		).Scan(&current)
		switch {
		case err == nil:
			if current != keyID {
				if _, uerr := tx.Exec(
					`UPDATE kms_aliases SET target_key_id = ? WHERE account_id = ? AND alias_name = ?`,
					keyID, accountID, alias,
				); uerr != nil {
					return "", fmt.Errorf("ensure aws managed aliases: retarget %s: %w", alias, uerr)
				}
			}
		case errors.Is(err, sql.ErrNoRows):
			if _, ierr := tx.Exec(
				`INSERT INTO kms_aliases (alias_name, account_id, target_key_id) VALUES (?, ?, ?)`,
				alias, accountID, keyID,
			); ierr != nil {
				return "", fmt.Errorf("ensure aws managed aliases: insert %s: %w", alias, ierr)
			}
		default:
			return "", fmt.Errorf("ensure aws managed aliases: check %s: %w", alias, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("ensure aws managed aliases: commit: %w", err)
	}
	return keyID, nil
}

// createKeyTx inserts a CMK within an existing transaction.
func (s *Store) createKeyTx(tx *sql.Tx, accountID, creatorARN, policyOverride string) (Key, error) {
	keyID := uuid.NewString()
	material := make([]byte, cmkMaterialSize)
	if _, err := io.ReadFull(rand.Reader, material); err != nil {
		return Key{}, fmt.Errorf("create key: generate material: %w", err)
	}
	sealed, err := Seal(s.master, material)
	if err != nil {
		return Key{}, fmt.Errorf("create key: seal material: %w", err)
	}
	policy := strings.TrimSpace(policyOverride)
	if policy == "" {
		policy = DefaultKeyPolicy(accountID, creatorARN)
	}
	arn := KeyARN(DefaultKMSRegion, accountID, keyID)
	created := nowRFC3339()
	_, err = tx.Exec(
		`INSERT INTO kms_keys
		 (key_id, account_id, arn, key_state, key_usage, sealed_material, key_policy, creation_date, deletion_date, key_rotation_enabled,
		  last_rotation_date, rotation_period_days)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, '', 0, '', ?)`,
		keyID, accountID, arn, KeyStateEnabled, KeyUsageEncryptDecrypt, sealed, policy, created,
		DefaultLabRotationPeriodDays,
	)
	if err != nil {
		return Key{}, fmt.Errorf("create key: %w", err)
	}
	return Key{
		KeyID:        keyID,
		AccountID:    accountID,
		ARN:          arn,
		KeyState:     KeyStateEnabled,
		KeyUsage:     KeyUsageEncryptDecrypt,
		KeyPolicy:    policy,
		CreationDate: created,
	}, nil
}

func normalizeAliasName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return name
	}
	if strings.HasPrefix(name, "arn:aws:kms:") {
		parts := strings.Split(name, ":")
		if len(parts) >= 6 && strings.HasPrefix(parts[5], "alias/") {
			return "alias/" + strings.TrimPrefix(parts[5], "alias/")
		}
	}
	if !strings.HasPrefix(name, "alias/") {
		return "alias/" + name
	}
	return name
}

// CreateGrant stores a grant on a CMK.
func (s *Store) CreateGrant(accountID, keyID, granteePrincipal, retiringPrincipal string, operations []string, name string) (Grant, error) {
	if _, err := s.GetKey(keyID); err != nil {
		return Grant{}, fmt.Errorf("create grant: %w", err)
	}
	if len(operations) == 0 {
		return Grant{}, fmt.Errorf("create grant: operations required")
	}
	if strings.TrimSpace(granteePrincipal) == "" {
		return Grant{}, fmt.Errorf("create grant: grantee principal required")
	}
	opsJSON, err := json.Marshal(operations)
	if err != nil {
		return Grant{}, fmt.Errorf("create grant: marshal operations: %w", err)
	}
	grantID := uuid.NewString()
	retiring := sql.NullString{}
	if strings.TrimSpace(retiringPrincipal) != "" {
		retiring = sql.NullString{String: retiringPrincipal, Valid: true}
	}
	nameVal := sql.NullString{}
	if strings.TrimSpace(name) != "" {
		nameVal = sql.NullString{String: name, Valid: true}
	}
	_, err = s.db.Exec(
		`INSERT INTO kms_grants
		 (grant_id, key_id, account_id, grantee_principal, retiring_principal, operations, name)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		grantID, keyID, accountID, granteePrincipal, retiring, string(opsJSON), nameVal,
	)
	if err != nil {
		return Grant{}, fmt.Errorf("create grant: %w", err)
	}
	return Grant{
		GrantID:           grantID,
		KeyID:             keyID,
		AccountID:         accountID,
		GranteePrincipal:  granteePrincipal,
		RetiringPrincipal: retiringPrincipal,
		Operations:        append([]string(nil), operations...),
		Name:              name,
	}, nil
}

// ListGrants returns grants for a key.
func (s *Store) ListGrants(keyID string) ([]Grant, error) {
	rows, err := s.db.Query(
		`SELECT grant_id, key_id, account_id, grantee_principal, retiring_principal, operations, name
		 FROM kms_grants WHERE key_id = ? ORDER BY grant_id`,
		keyID,
	)
	if err != nil {
		return nil, fmt.Errorf("list grants %s: %w", keyID, err)
	}
	defer rows.Close()

	var out []Grant
	for rows.Next() {
		g, err := scanGrant(rows)
		if err != nil {
			return nil, fmt.Errorf("list grants %s: %w", keyID, err)
		}
		out = append(out, g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list grants %s: %w", keyID, err)
	}
	if out == nil {
		out = []Grant{}
	}
	return out, nil
}

type grantScanner interface {
	Scan(dest ...any) error
}

func scanGrant(row grantScanner) (Grant, error) {
	var (
		g        Grant
		retiring sql.NullString
		name     sql.NullString
		opsRaw   string
	)
	if err := row.Scan(&g.GrantID, &g.KeyID, &g.AccountID, &g.GranteePrincipal, &retiring, &opsRaw, &name); err != nil {
		return Grant{}, err
	}
	if retiring.Valid {
		g.RetiringPrincipal = retiring.String
	}
	if name.Valid {
		g.Name = name.String
	}
	if err := json.Unmarshal([]byte(opsRaw), &g.Operations); err != nil {
		return Grant{}, fmt.Errorf("parse operations: %w", err)
	}
	if g.Operations == nil {
		g.Operations = []string{}
	}
	return g, nil
}

// RetireGrant removes a grant when the caller is the retiring or grantee principal.
func (s *Store) RetireGrant(grantID, callerARN string) error {
	var grantee, retiring sql.NullString
	err := s.db.QueryRow(
		`SELECT grantee_principal, retiring_principal FROM kms_grants WHERE grant_id = ?`,
		grantID,
	).Scan(&grantee, &retiring)
	if err != nil {
		return err
	}
	ok := grantee.Valid && grantee.String == callerARN
	if retiring.Valid && retiring.String == callerARN {
		ok = true
	}
	if !ok {
		return fmt.Errorf("retire grant %s: caller not authorized to retire", grantID)
	}
	return s.deleteGrant(grantID)
}

// RevokeGrant removes a grant by id (key owner path).
func (s *Store) RevokeGrant(grantID string) error {
	return s.deleteGrant(grantID)
}

func (s *Store) deleteGrant(grantID string) error {
	res, err := s.db.Exec(`DELETE FROM kms_grants WHERE grant_id = ?`, grantID)
	if err != nil {
		return fmt.Errorf("delete grant %s: %w", grantID, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete grant %s: %w", grantID, err)
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// FindMatchingGrant reports whether a grant on keyID allows principalARN the given operation.
// operation may be "Encrypt" or "kms:Encrypt".
func (s *Store) FindMatchingGrant(keyID, principalARN, operation string) (bool, error) {
	grants, err := s.ListGrants(keyID)
	if err != nil {
		return false, err
	}
	want := normalizeGrantOp(operation)
	for _, g := range grants {
		if g.GranteePrincipal != principalARN {
			continue
		}
		for _, op := range g.Operations {
			if normalizeGrantOp(op) == want {
				return true, nil
			}
		}
	}
	return false, nil
}

func normalizeGrantOp(op string) string {
	op = strings.TrimSpace(op)
	if i := strings.Index(op, ":"); i >= 0 {
		op = op[i+1:]
	}
	return op
}
