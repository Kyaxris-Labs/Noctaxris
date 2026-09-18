package store

import (
	"crypto/x509"
	"database/sql"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"time"
)

const defaultIoTCredentialDurationSeconds = 3600

// IoTRoleAlias maps a device credential request to an IAM role.
type IoTRoleAlias struct {
	RoleAlias                 string
	RoleAliasARN              string
	RoleARN                   string
	CredentialDurationSeconds int
	CreatedAt                 int64
}

// CreateIoTRoleAlias stores a role alias.
func (s *Store) CreateIoTRoleAlias(accountID, region, alias, roleARN string, durationSeconds int) (IoTRoleAlias, error) {
	if err := s.EnsureIoTSchema(); err != nil {
		return IoTRoleAlias{}, err
	}
	region = iotRegion(region)
	alias = strings.TrimSpace(alias)
	roleARN = strings.TrimSpace(roleARN)
	if alias == "" || roleARN == "" {
		return IoTRoleAlias{}, fmt.Errorf("%w: roleAlias and roleArn required", ErrIoTBadRequest)
	}
	if durationSeconds <= 0 {
		durationSeconds = defaultIoTCredentialDurationSeconds
	}
	now := time.Now().UTC().UnixMilli()
	arn := IoTRoleAliasARN(region, accountID, alias)
	_, err := s.db.Exec(
		`INSERT INTO iot_role_aliases
		 (account_id, region, role_alias, role_alias_arn, role_arn, credential_duration_seconds, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		accountID, region, alias, arn, roleARN, durationSeconds, now,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return IoTRoleAlias{}, ErrIoTConflict
		}
		return IoTRoleAlias{}, fmt.Errorf("create role alias: %w", err)
	}
	return IoTRoleAlias{
		RoleAlias: alias, RoleAliasARN: arn, RoleARN: roleARN,
		CredentialDurationSeconds: durationSeconds, CreatedAt: now,
	}, nil
}

// DescribeIoTRoleAlias returns a role alias.
func (s *Store) DescribeIoTRoleAlias(accountID, region, alias string) (IoTRoleAlias, error) {
	if err := s.EnsureIoTSchema(); err != nil {
		return IoTRoleAlias{}, err
	}
	var ra IoTRoleAlias
	err := s.db.QueryRow(
		`SELECT role_alias, role_alias_arn, role_arn, credential_duration_seconds, created_at
		 FROM iot_role_aliases WHERE account_id = ? AND region = ? AND role_alias = ?`,
		accountID, iotRegion(region), strings.TrimSpace(alias),
	).Scan(&ra.RoleAlias, &ra.RoleAliasARN, &ra.RoleARN, &ra.CredentialDurationSeconds, &ra.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return IoTRoleAlias{}, ErrIoTNotFound
	}
	if err != nil {
		return IoTRoleAlias{}, fmt.Errorf("describe role alias: %w", err)
	}
	return ra, nil
}

// ListIoTRoleAliases lists role aliases for an account/region.
func (s *Store) ListIoTRoleAliases(accountID, region string) ([]IoTRoleAlias, error) {
	if err := s.EnsureIoTSchema(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT role_alias, role_alias_arn, role_arn, credential_duration_seconds, created_at
		 FROM iot_role_aliases WHERE account_id = ? AND region = ? ORDER BY role_alias`,
		accountID, iotRegion(region),
	)
	if err != nil {
		return nil, fmt.Errorf("list role aliases: %w", err)
	}
	defer rows.Close()
	var out []IoTRoleAlias
	for rows.Next() {
		var ra IoTRoleAlias
		if err := rows.Scan(&ra.RoleAlias, &ra.RoleAliasARN, &ra.RoleARN, &ra.CredentialDurationSeconds, &ra.CreatedAt); err != nil {
			return nil, fmt.Errorf("list role aliases scan: %w", err)
		}
		out = append(out, ra)
	}
	if out == nil {
		out = []IoTRoleAlias{}
	}
	return out, rows.Err()
}

// DeleteIoTRoleAlias removes a role alias.
func (s *Store) DeleteIoTRoleAlias(accountID, region, alias string) error {
	if err := s.EnsureIoTSchema(); err != nil {
		return err
	}
	res, err := s.db.Exec(
		`DELETE FROM iot_role_aliases WHERE account_id = ? AND region = ? AND role_alias = ?`,
		accountID, iotRegion(region), strings.TrimSpace(alias),
	)
	if err != nil {
		return fmt.Errorf("delete role alias: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrIoTNotFound
	}
	return nil
}

// ResolveIoTDeviceFromClientCert maps a TLS client certificate to an ACTIVE attached thing.
func (s *Store) ResolveIoTDeviceFromClientCert(cert *x509.Certificate) (IoTMQTTDeviceContext, error) {
	if cert == nil {
		return IoTMQTTDeviceContext{}, ErrIoTNotFound
	}
	return s.ResolveIoTMQTTDeviceByCertificate(IoTCertificateIDFromDER(cert.Raw))
}

// ParseIoTCertificatePEM parses the first CERTIFICATE PEM block.
func ParseIoTCertificatePEM(pemBytes string) (*x509.Certificate, error) {
	block, _ := pem.Decode([]byte(pemBytes))
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("%w: certificate pem", ErrIoTBadRequest)
	}
	return x509.ParseCertificate(block.Bytes)
}
