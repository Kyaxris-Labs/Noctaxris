package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// CognitoLabConfirmationCode is the stub code rendered into CustomMessage bodies and
// accepted by ConfirmForgotPassword / VerifyUserAttribute (no SES).
const CognitoLabConfirmationCode = "123456"

const (
	cognitoConfirmPurposeForgotPassword = "FORGOT_PASSWORD"
	cognitoConfirmPurposeAttrVerify     = "ATTR_VERIFY"
)

const cognitoForgotAttrsSchema = `
CREATE TABLE IF NOT EXISTS cognito_confirmation_codes (
  account_id TEXT NOT NULL,
  pool_id TEXT NOT NULL,
  username TEXT NOT NULL,
  purpose TEXT NOT NULL,
  code TEXT NOT NULL,
  attribute_name TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, pool_id, username, purpose)
);
CREATE TABLE IF NOT EXISTS cognito_user_attributes (
  account_id TEXT NOT NULL,
  pool_id TEXT NOT NULL,
  username TEXT NOT NULL,
  attr_name TEXT NOT NULL,
  attr_value TEXT NOT NULL,
  verified INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (account_id, pool_id, username, attr_name)
);
`

// CognitoUserAttribute is a lab-stored user attribute row.
type CognitoUserAttribute struct {
	Name     string
	Value    string
	Verified bool
}

// EnsureCognitoForgotAttrsSchema creates confirmation-code and user-attribute tables.
func EnsureCognitoForgotAttrsSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure cognito forgot attrs schema: db is nil")
	}
	if _, err := db.Exec(cognitoForgotAttrsSchema); err != nil {
		return fmt.Errorf("ensure cognito forgot attrs schema: %w", err)
	}
	return nil
}

func (s *Store) EnsureCognitoForgotAttrsSchema() error {
	return EnsureCognitoForgotAttrsSchema(s.db)
}

func (s *Store) storeCognitoConfirmationCode(accountID, poolID, username, purpose, code, attributeName string) error {
	if err := s.EnsureCognitoForgotAttrsSchema(); err != nil {
		return err
	}
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO cognito_confirmation_codes
		 (account_id, pool_id, username, purpose, code, attribute_name, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(account_id, pool_id, username, purpose) DO UPDATE SET
		   code = excluded.code,
		   attribute_name = excluded.attribute_name,
		   created_at = excluded.created_at`,
		accountID, poolID, username, purpose, code, attributeName, now,
	)
	if err != nil {
		return fmt.Errorf("store confirmation code: %w", err)
	}
	return nil
}

func (s *Store) clearCognitoConfirmationCode(accountID, poolID, username, purpose string) error {
	if err := s.EnsureCognitoForgotAttrsSchema(); err != nil {
		return err
	}
	_, err := s.db.Exec(
		`DELETE FROM cognito_confirmation_codes
		 WHERE account_id = ? AND pool_id = ? AND username = ? AND purpose = ?`,
		accountID, poolID, username, purpose,
	)
	if err != nil {
		return fmt.Errorf("clear confirmation code: %w", err)
	}
	return nil
}

func (s *Store) getCognitoConfirmationCode(accountID, poolID, username, purpose string) (code, attributeName string, err error) {
	if err := s.EnsureCognitoForgotAttrsSchema(); err != nil {
		return "", "", err
	}
	err = s.db.QueryRow(
		`SELECT code, attribute_name FROM cognito_confirmation_codes
		 WHERE account_id = ? AND pool_id = ? AND username = ? AND purpose = ?`,
		accountID, poolID, username, purpose,
	).Scan(&code, &attributeName)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", ErrCognitoCodeMismatch
	}
	if err != nil {
		return "", "", fmt.Errorf("get confirmation code: %w", err)
	}
	return code, attributeName, nil
}

// ConfirmForgotPasswordCognitoUser sets a new password when the lab confirmation code matches.
func (s *Store) ConfirmForgotPasswordCognitoUser(clientID, username, confirmationCode, password string) error {
	clientID = strings.TrimSpace(clientID)
	username = strings.TrimSpace(username)
	confirmationCode = strings.TrimSpace(confirmationCode)
	if clientID == "" || username == "" || confirmationCode == "" || password == "" {
		return fmt.Errorf("%w: ClientId, Username, ConfirmationCode, and Password required", ErrCognitoBadRequest)
	}
	acct, poolID, err := s.lookupClient(clientID)
	if err != nil {
		return err
	}
	if _, _, _, err := s.getUser(acct, poolID, username); err != nil {
		return err
	}
	stored, _, err := s.getCognitoConfirmationCode(acct, poolID, username, cognitoConfirmPurposeForgotPassword)
	if err != nil {
		return err
	}
	if stored != confirmationCode {
		return ErrCognitoCodeMismatch
	}
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`UPDATE cognito_users SET password_hash = ? WHERE account_id = ? AND pool_id = ? AND username = ?`,
		hash, acct, poolID, username,
	)
	if err != nil {
		return fmt.Errorf("confirm forgot password update: %w", err)
	}
	if err := s.storeUserSRPVerifier(acct, poolID, username, password); err != nil {
		return err
	}
	return s.clearCognitoConfirmationCode(acct, poolID, username, cognitoConfirmPurposeForgotPassword)
}

// UpdateUserAttributesCognitoUser stores lab attributes for the access-token identity.
// When email or phone_number is updated and CustomMessage is configured, fires
// CustomMessage_UpdateUserAttribute (stub code {####} / 123456; no SES).
func (s *Store) UpdateUserAttributesCognitoUser(accessToken string, attrs map[string]string) ([]CognitoCodeDeliveryDetails, error) {
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return nil, fmt.Errorf("%w: AccessToken required", ErrCognitoBadRequest)
	}
	if len(attrs) == 0 {
		return nil, fmt.Errorf("%w: UserAttributes required", ErrCognitoBadRequest)
	}
	acct, poolID, clientID, username, err := s.resolveAccessTokenIdentity(accessToken)
	if err != nil {
		return nil, err
	}
	if err := s.EnsureCognitoForgotAttrsSchema(); err != nil {
		return nil, err
	}
	sub, _, status, err := s.getUser(acct, poolID, username)
	if err != nil {
		return nil, err
	}

	var delivery []CognitoCodeDeliveryDetails
	for name, value := range attrs {
		name = strings.TrimSpace(name)
		if name == "" {
			return nil, fmt.Errorf("%w: attribute Name required", ErrCognitoBadRequest)
		}
		needsVerify := name == "email" || name == "phone_number"
		verified := 0
		if !needsVerify {
			verified = 1
		}
		_, err := s.db.Exec(
			`INSERT INTO cognito_user_attributes
			 (account_id, pool_id, username, attr_name, attr_value, verified)
			 VALUES (?, ?, ?, ?, ?, ?)
			 ON CONFLICT(account_id, pool_id, username, attr_name) DO UPDATE SET
			   attr_value = excluded.attr_value,
			   verified = excluded.verified`,
			acct, poolID, username, name, value, verified,
		)
		if err != nil {
			return nil, fmt.Errorf("store user attribute: %w", err)
		}
		if !needsVerify {
			continue
		}
		cmPayload, err := s.FireCognitoTriggerEvent(acct, poolID, CognitoTriggerCustomMessage, CognitoTriggerEventInput{
			TriggerSource: "CustomMessage_UpdateUserAttribute",
			UserPoolID:    poolID,
			Username:      username,
			ClientID:      clientID,
			UserSub:       sub,
			UserStatus:    status,
			CodeParameter: "{####}",
		})
		if err != nil {
			return nil, err
		}
		if err := s.applyCustomMessagePayload(acct, poolID, username, "CustomMessage_UpdateUserAttribute", "{####}", cmPayload); err != nil {
			return nil, err
		}
		if err := s.storeCognitoConfirmationCode(acct, poolID, username, cognitoConfirmPurposeAttrVerify, CognitoLabConfirmationCode, name); err != nil {
			return nil, err
		}
		medium := "EMAIL"
		dest := "t***@example.com"
		if name == "phone_number" {
			medium = "SMS"
			dest = "+*******9999"
		}
		delivery = append(delivery, CognitoCodeDeliveryDetails{
			Destination:    dest,
			DeliveryMedium: medium,
			AttributeName:  name,
		})
	}
	return delivery, nil
}

// GetUserAttributeVerificationCodeCognitoUser fires CustomMessage_VerifyUserAttribute
// for the named attribute (stub code {####} / 123456; no SES).
func (s *Store) GetUserAttributeVerificationCodeCognitoUser(accessToken, attributeName string) (CognitoCodeDeliveryDetails, error) {
	accessToken = strings.TrimSpace(accessToken)
	attributeName = strings.TrimSpace(attributeName)
	if accessToken == "" || attributeName == "" {
		return CognitoCodeDeliveryDetails{}, fmt.Errorf("%w: AccessToken and AttributeName required", ErrCognitoBadRequest)
	}
	acct, poolID, clientID, username, err := s.resolveAccessTokenIdentity(accessToken)
	if err != nil {
		return CognitoCodeDeliveryDetails{}, err
	}
	if err := s.EnsureCognitoForgotAttrsSchema(); err != nil {
		return CognitoCodeDeliveryDetails{}, err
	}
	var value string
	err = s.db.QueryRow(
		`SELECT attr_value FROM cognito_user_attributes
		 WHERE account_id = ? AND pool_id = ? AND username = ? AND attr_name = ?`,
		acct, poolID, username, attributeName,
	).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return CognitoCodeDeliveryDetails{}, fmt.Errorf("%w: attribute %s not found", ErrCognitoBadRequest, attributeName)
	}
	if err != nil {
		return CognitoCodeDeliveryDetails{}, fmt.Errorf("get user attribute: %w", err)
	}
	_ = value
	sub, _, status, err := s.getUser(acct, poolID, username)
	if err != nil {
		return CognitoCodeDeliveryDetails{}, err
	}
	cmPayload, err := s.FireCognitoTriggerEvent(acct, poolID, CognitoTriggerCustomMessage, CognitoTriggerEventInput{
		TriggerSource: "CustomMessage_VerifyUserAttribute",
		UserPoolID:    poolID,
		Username:      username,
		ClientID:      clientID,
		UserSub:       sub,
		UserStatus:    status,
		CodeParameter: "{####}",
	})
	if err != nil {
		return CognitoCodeDeliveryDetails{}, err
	}
	if err := s.applyCustomMessagePayload(acct, poolID, username, "CustomMessage_VerifyUserAttribute", "{####}", cmPayload); err != nil {
		return CognitoCodeDeliveryDetails{}, err
	}
	if err := s.storeCognitoConfirmationCode(acct, poolID, username, cognitoConfirmPurposeAttrVerify, CognitoLabConfirmationCode, attributeName); err != nil {
		return CognitoCodeDeliveryDetails{}, err
	}
	medium := "EMAIL"
	dest := "t***@example.com"
	if attributeName == "phone_number" {
		medium = "SMS"
		dest = "+*******9999"
	}
	return CognitoCodeDeliveryDetails{
		Destination:    dest,
		DeliveryMedium: medium,
		AttributeName:  attributeName,
	}, nil
}

// VerifyUserAttributeCognitoUser marks an attribute verified when the lab code matches.
func (s *Store) VerifyUserAttributeCognitoUser(accessToken, attributeName, code string) error {
	accessToken = strings.TrimSpace(accessToken)
	attributeName = strings.TrimSpace(attributeName)
	code = strings.TrimSpace(code)
	if accessToken == "" || attributeName == "" || code == "" {
		return fmt.Errorf("%w: AccessToken, AttributeName, and Code required", ErrCognitoBadRequest)
	}
	acct, poolID, _, username, err := s.resolveAccessTokenIdentity(accessToken)
	if err != nil {
		return err
	}
	if err := s.EnsureCognitoForgotAttrsSchema(); err != nil {
		return err
	}
	stored, attrName, err := s.getCognitoConfirmationCode(acct, poolID, username, cognitoConfirmPurposeAttrVerify)
	if err != nil {
		return err
	}
	if stored != code || (attrName != "" && attrName != attributeName) {
		return ErrCognitoCodeMismatch
	}
	res, err := s.db.Exec(
		`UPDATE cognito_user_attributes SET verified = 1
		 WHERE account_id = ? AND pool_id = ? AND username = ? AND attr_name = ?`,
		acct, poolID, username, attributeName,
	)
	if err != nil {
		return fmt.Errorf("verify user attribute: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: attribute %s not found", ErrCognitoBadRequest, attributeName)
	}
	return s.clearCognitoConfirmationCode(acct, poolID, username, cognitoConfirmPurposeAttrVerify)
}

// GetCognitoUserAttribute returns a lab-stored attribute (test helper).
func (s *Store) GetCognitoUserAttribute(accountID, poolID, username, attrName string) (CognitoUserAttribute, error) {
	if err := s.EnsureCognitoForgotAttrsSchema(); err != nil {
		return CognitoUserAttribute{}, err
	}
	var row CognitoUserAttribute
	var verified int
	err := s.db.QueryRow(
		`SELECT attr_name, attr_value, verified FROM cognito_user_attributes
		 WHERE account_id = ? AND pool_id = ? AND username = ? AND attr_name = ?`,
		accountID, poolID, username, attrName,
	).Scan(&row.Name, &row.Value, &verified)
	if errors.Is(err, sql.ErrNoRows) {
		return CognitoUserAttribute{}, ErrCognitoNotFound
	}
	if err != nil {
		return CognitoUserAttribute{}, fmt.Errorf("get user attribute: %w", err)
	}
	row.Verified = verified == 1
	return row, nil
}
