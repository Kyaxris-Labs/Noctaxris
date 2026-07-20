package store

import (
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"time"
)

const (
	// DefaultLabRotationPeriodDays is the lab automatic rotation period when enabled.
	DefaultLabRotationPeriodDays = 365

	kmsKeyMaterialsSchema = `
CREATE TABLE IF NOT EXISTS kms_key_materials (
  key_id TEXT NOT NULL,
  generation INTEGER NOT NULL,
  sealed_material BLOB NOT NULL,
  created_at TEXT NOT NULL,
  PRIMARY KEY (key_id, generation)
);
`
)

// EnsureKMSKeyMaterialSchema creates the prior-generation CMK material table.
func EnsureKMSKeyMaterialSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure kms key material schema: db is nil")
	}
	if _, err := db.Exec(kmsKeyMaterialsSchema); err != nil {
		return fmt.Errorf("ensure kms key material schema: %w", err)
	}
	return nil
}

// EnsureKMSKeyMaterialSchema ensures prior-generation material storage on an open store.
func (s *Store) EnsureKMSKeyMaterialSchema() error {
	return EnsureKMSKeyMaterialSchema(s.db)
}

// SweepExpiredPendingKeys hard-deletes PendingDeletion keys whose DeletionDate is at or before now.
// Also removes aliases and grants for those keys. Returns the number of keys deleted.
func (s *Store) SweepExpiredPendingKeys(now time.Time) (int, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	nowStr := now.UTC().Format(time.RFC3339)
	rows, err := s.db.Query(
		`SELECT key_id FROM kms_keys
		 WHERE key_state = ? AND deletion_date != '' AND deletion_date <= ?`,
		KeyStatePendingDeletion, nowStr,
	)
	if err != nil {
		return 0, fmt.Errorf("sweep expired keys: %w", err)
	}
	defer rows.Close()

	var keyIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return 0, fmt.Errorf("sweep expired keys: %w", err)
		}
		keyIDs = append(keyIDs, id)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("sweep expired keys: %w", err)
	}

	deleted := 0
	for _, keyID := range keyIDs {
		if err := s.purgeKey(keyID); err != nil {
			return deleted, err
		}
		deleted++
	}
	return deleted, nil
}

func (s *Store) purgeKey(keyID string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("purge key %s: begin: %w", keyID, err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM kms_grants WHERE key_id = ?`, keyID); err != nil {
		return fmt.Errorf("purge key %s: grants: %w", keyID, err)
	}
	if _, err := tx.Exec(`DELETE FROM kms_aliases WHERE target_key_id = ?`, keyID); err != nil {
		return fmt.Errorf("purge key %s: aliases: %w", keyID, err)
	}
	if _, err := tx.Exec(`DELETE FROM kms_key_materials WHERE key_id = ?`, keyID); err != nil {
		return fmt.Errorf("purge key %s: materials: %w", keyID, err)
	}
	res, err := tx.Exec(`DELETE FROM kms_keys WHERE key_id = ?`, keyID)
	if err != nil {
		return fmt.Errorf("purge key %s: %w", keyID, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("purge key %s: %w", keyID, err)
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("purge key %s: commit: %w", keyID, err)
	}
	return nil
}

// SetPendingDeletionDate overwrites DeletionDate for a PendingDeletion key (lab sweeper tests).
func (s *Store) SetPendingDeletionDate(keyID string, deletionDate time.Time) error {
	k, err := s.GetKey(keyID)
	if err != nil {
		return err
	}
	if k.KeyState != KeyStatePendingDeletion {
		return fmt.Errorf("set pending deletion date %s: %w", keyID, ErrInvalidKeyState)
	}
	_, err = s.db.Exec(
		`UPDATE kms_keys SET deletion_date = ? WHERE key_id = ?`,
		deletionDate.UTC().Format(time.RFC3339), keyID,
	)
	if err != nil {
		return fmt.Errorf("set pending deletion date %s: %w", keyID, err)
	}
	return nil
}

// RotateKeyMaterial archives current sealed material and installs fresh CMK bytes.
// Prior generations remain available for Decrypt. Lab-simplified (no AWS schedule parity).
func (s *Store) RotateKeyMaterial(keyID string) error {
	k, err := s.GetKey(keyID)
	if err != nil {
		return err
	}
	if k.KeyUsage != KeyUsageEncryptDecrypt {
		return fmt.Errorf("rotate key material %s: %w", keyID, ErrUnsupportedKeyRotation)
	}
	if k.KeyState == KeyStatePendingDeletion {
		return fmt.Errorf("rotate key material %s: %w", keyID, ErrInvalidKeyState)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("rotate key material %s: begin: %w", keyID, err)
	}
	defer tx.Rollback()

	var currentSealed []byte
	if err := tx.QueryRow(`SELECT sealed_material FROM kms_keys WHERE key_id = ?`, keyID).Scan(&currentSealed); err != nil {
		return fmt.Errorf("rotate key material %s: load: %w", keyID, err)
	}

	var maxGen sql.NullInt64
	if err := tx.QueryRow(
		`SELECT MAX(generation) FROM kms_key_materials WHERE key_id = ?`, keyID,
	).Scan(&maxGen); err != nil {
		return fmt.Errorf("rotate key material %s: max gen: %w", keyID, err)
	}
	nextGen := 1
	if maxGen.Valid {
		nextGen = int(maxGen.Int64) + 1
	}

	created := nowRFC3339()
	if _, err := tx.Exec(
		`INSERT INTO kms_key_materials (key_id, generation, sealed_material, created_at) VALUES (?, ?, ?, ?)`,
		keyID, nextGen, currentSealed, created,
	); err != nil {
		return fmt.Errorf("rotate key material %s: archive: %w", keyID, err)
	}

	material := make([]byte, cmkMaterialSize)
	if _, err := io.ReadFull(rand.Reader, material); err != nil {
		return fmt.Errorf("rotate key material %s: generate: %w", keyID, err)
	}
	sealed, err := Seal(s.master, material)
	if err != nil {
		return fmt.Errorf("rotate key material %s: seal: %w", keyID, err)
	}
	if _, err := tx.Exec(
		`UPDATE kms_keys SET sealed_material = ?, last_rotation_date = ? WHERE key_id = ?`,
		sealed, created, keyID,
	); err != nil {
		return fmt.Errorf("rotate key material %s: update: %w", keyID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("rotate key material %s: commit: %w", keyID, err)
	}
	return nil
}

// UnsealAllKeyMaterials returns current material first, then archived generations newest-first.
func (s *Store) UnsealAllKeyMaterials(keyID string) ([][]byte, error) {
	current, err := s.UnsealKeyMaterial(keyID)
	if err != nil {
		return nil, err
	}
	out := [][]byte{current}

	rows, err := s.db.Query(
		`SELECT sealed_material FROM kms_key_materials WHERE key_id = ? ORDER BY generation DESC`,
		keyID,
	)
	if err != nil {
		return nil, fmt.Errorf("unseal all key materials %s: %w", keyID, err)
	}
	defer rows.Close()
	for rows.Next() {
		var sealed []byte
		if err := rows.Scan(&sealed); err != nil {
			return nil, fmt.Errorf("unseal all key materials %s: %w", keyID, err)
		}
		plain, err := Unseal(s.master, sealed)
		if err != nil {
			return nil, fmt.Errorf("unseal all key materials %s: %w", keyID, err)
		}
		out = append(out, plain)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("unseal all key materials %s: %w", keyID, err)
	}
	return out, nil
}

// DecryptBlobWithKey tries current then prior CMK materials until one opens the blob.
func (s *Store) DecryptBlobWithKey(keyID string, blob []byte) ([]byte, error) {
	materials, err := s.UnsealAllKeyMaterials(keyID)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for _, cmk := range materials {
		plain, openErr := DecryptUnderCMK(cmk, blob)
		if openErr == nil {
			return plain, nil
		}
		lastErr = openErr
	}
	if lastErr == nil {
		lastErr = errors.New("unable to decrypt ciphertext")
	}
	return nil, lastErr
}

// MaybeAutoRotate rotates key material when rotation is enabled and the lab period has elapsed.
func (s *Store) MaybeAutoRotate(keyID string, now time.Time) error {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	k, err := s.getKeyRaw(keyID)
	if err != nil {
		return err
	}
	if !k.KeyRotationEnabled {
		return nil
	}
	if k.KeyState == KeyStatePendingDeletion {
		return nil
	}
	periodDays := k.RotationPeriodDays
	if periodDays <= 0 {
		periodDays = DefaultLabRotationPeriodDays
	}
	anchor := k.LastRotationDate
	if anchor == "" {
		anchor = k.CreationDate
	}
	last, err := time.Parse(time.RFC3339, anchor)
	if err != nil {
		last = now
	}
	if now.Sub(last) < time.Duration(periodDays)*24*time.Hour {
		return nil
	}
	return s.RotateKeyMaterial(keyID)
}

// SetLastRotationDate sets last_rotation_date (lab auto-rotate tests).
func (s *Store) SetLastRotationDate(keyID string, at time.Time) error {
	_, err := s.db.Exec(
		`UPDATE kms_keys SET last_rotation_date = ? WHERE key_id = ?`,
		at.UTC().Format(time.RFC3339), keyID,
	)
	if err != nil {
		return fmt.Errorf("set last rotation date %s: %w", keyID, err)
	}
	return nil
}

// SetRotationPeriodDays sets the lab automatic rotation period for a key.
func (s *Store) SetRotationPeriodDays(keyID string, days int) error {
	if days <= 0 {
		days = DefaultLabRotationPeriodDays
	}
	_, err := s.db.Exec(
		`UPDATE kms_keys SET rotation_period_days = ? WHERE key_id = ?`,
		days, keyID,
	)
	if err != nil {
		return fmt.Errorf("set rotation period %s: %w", keyID, err)
	}
	return nil
}

func (s *Store) getKeyRaw(keyID string) (Key, error) {
	var k Key
	var rotation int
	var period sql.NullInt64
	err := s.db.QueryRow(
		`SELECT key_id, account_id, arn, key_state, key_usage, key_policy, creation_date,
		        COALESCE(deletion_date, ''), COALESCE(key_rotation_enabled, 0),
		        COALESCE(last_rotation_date, ''), COALESCE(rotation_period_days, 0)
		 FROM kms_keys WHERE key_id = ?`,
		keyID,
	).Scan(&k.KeyID, &k.AccountID, &k.ARN, &k.KeyState, &k.KeyUsage, &k.KeyPolicy, &k.CreationDate,
		&k.DeletionDate, &rotation, &k.LastRotationDate, &period)
	if err != nil {
		return Key{}, err
	}
	k.KeyRotationEnabled = rotation != 0
	if period.Valid {
		k.RotationPeriodDays = int(period.Int64)
	}
	return k, nil
}

// sweepThenGetKey runs the deletion sweeper then loads the key.
func (s *Store) sweepThenGetKey(keyID string) (Key, error) {
	if _, err := s.SweepExpiredPendingKeys(time.Now().UTC()); err != nil {
		return Key{}, err
	}
	return s.getKeyRaw(keyID)
}
