package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/Kyaxris-Labs/Noctaxris/internal/validate"
)

// ErrMFADeviceAlreadyEnabled is returned when EnableMFADevice targets a device
// that is already enabled (including for another user).
var ErrMFADeviceAlreadyEnabled = errors.New("mfa device already enabled")

// MFADevice is a virtual MFA device with decrypted seed for TOTP validation.
type MFADevice struct {
	AccountID string
	Serial    string
	UserName  string
	Seed      []byte
	Enabled   bool
}

// CreateVirtualMFADevice registers a virtual MFA device with an encrypted seed.
// The device is not enabled until EnableMFADevice associates it with a user.
func (s *Store) CreateVirtualMFADevice(accountID string, seed []byte) (serial string, err error) {
	if err := validate.AccountID(accountID); err != nil {
		return "", fmt.Errorf("create virtual mfa device: %w", err)
	}
	if len(seed) == 0 {
		return "", fmt.Errorf("create virtual mfa device: seed required")
	}
	suffix, err := newMFASerialSuffix()
	if err != nil {
		return "", err
	}
	serial = MFADeviceARN(accountID, suffix)
	sealed, err := s.SealWithMaster(seed)
	if err != nil {
		return "", fmt.Errorf("create virtual mfa device: seal seed: %w", err)
	}
	_, err = s.db.Exec(
		`INSERT INTO iam_mfa_devices (account_id, serial, user_name, seed_ciphertext, enabled)
		 VALUES (?, ?, '', ?, 0)`,
		accountID, serial, sealed,
	)
	if err != nil {
		return "", fmt.Errorf("create virtual mfa device %s: %w", accountID, err)
	}
	return serial, nil
}

// EnableMFADevice associates a virtual MFA device with a user and marks it enabled.
// Refuses to reassign a device that is already enabled.
func (s *Store) EnableMFADevice(accountID, serial, userName string) error {
	if _, err := s.GetUser(accountID, userName); err != nil {
		return fmt.Errorf("enable mfa device: %w", err)
	}
	dev, err := s.GetMFADevice(accountID, serial)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("enable mfa device %s: not found", serial)
		}
		return fmt.Errorf("enable mfa device: %w", err)
	}
	if dev.Enabled {
		return fmt.Errorf("enable mfa device %s: %w", serial, ErrMFADeviceAlreadyEnabled)
	}
	res, err := s.db.Exec(
		`UPDATE iam_mfa_devices SET user_name = ?, enabled = 1
		 WHERE account_id = ? AND serial = ? AND enabled = 0`,
		userName, accountID, serial,
	)
	if err != nil {
		return fmt.Errorf("enable mfa device %s: %w", serial, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("enable mfa device %s: %w", serial, err)
	}
	if affected == 0 {
		return fmt.Errorf("enable mfa device %s: not found", serial)
	}
	return nil
}

// GetMFADevice returns a virtual MFA device with the seed unsealed under the master key.
func (s *Store) GetMFADevice(accountID, serial string) (MFADevice, error) {
	var (
		d          MFADevice
		ciphertext []byte
		enabled    int
		userName   string
	)
	err := s.db.QueryRow(
		`SELECT account_id, serial, user_name, seed_ciphertext, enabled
		 FROM iam_mfa_devices WHERE account_id = ? AND serial = ?`,
		accountID, serial,
	).Scan(&d.AccountID, &d.Serial, &userName, &ciphertext, &enabled)
	if err != nil {
		return MFADevice{}, err
	}
	d.UserName = userName
	d.Enabled = enabled == 1
	seed, err := s.UnsealWithMaster(ciphertext)
	if err != nil {
		return MFADevice{}, fmt.Errorf("get mfa device: unseal seed: %w", err)
	}
	d.Seed = seed
	return d, nil
}

// ListMFADevices returns enabled MFA devices for a user in an account.
// When userName is empty, returns all devices in the account.
func (s *Store) ListMFADevices(accountID, userName string) ([]MFADevice, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if userName == "" {
		rows, err = s.db.Query(
			`SELECT account_id, serial, user_name, seed_ciphertext, enabled
			 FROM iam_mfa_devices WHERE account_id = ? ORDER BY serial`,
			accountID,
		)
	} else {
		rows, err = s.db.Query(
			`SELECT account_id, serial, user_name, seed_ciphertext, enabled
			 FROM iam_mfa_devices WHERE account_id = ? AND user_name = ? ORDER BY serial`,
			accountID, userName,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("list mfa devices: %w", err)
	}
	defer rows.Close()

	var out []MFADevice
	for rows.Next() {
		var (
			d          MFADevice
			ciphertext []byte
			enabled    int
			uname      string
		)
		if err := rows.Scan(&d.AccountID, &d.Serial, &uname, &ciphertext, &enabled); err != nil {
			return nil, fmt.Errorf("list mfa devices: %w", err)
		}
		d.UserName = uname
		d.Enabled = enabled == 1
		seed, err := s.UnsealWithMaster(ciphertext)
		if err != nil {
			return nil, fmt.Errorf("list mfa devices: unseal seed: %w", err)
		}
		d.Seed = seed
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list mfa devices: %w", err)
	}
	return out, nil
}

// DeactivateMFADevice disables a device for the given user and clears the association.
func (s *Store) DeactivateMFADevice(accountID, serial, userName string) error {
	res, err := s.db.Exec(
		`UPDATE iam_mfa_devices SET user_name = '', enabled = 0
		 WHERE account_id = ? AND serial = ? AND user_name = ? AND enabled = 1`,
		accountID, serial, userName,
	)
	if err != nil {
		return fmt.Errorf("deactivate mfa device %s: %w", serial, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("deactivate mfa device %s: %w", serial, err)
	}
	if affected == 0 {
		return fmt.Errorf("deactivate mfa device %s: %w", serial, sql.ErrNoRows)
	}
	return nil
}

func newMFASerialSuffix() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "device-" + hex.EncodeToString(b[:]), nil
}
