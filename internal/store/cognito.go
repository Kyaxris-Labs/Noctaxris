package store

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/jwtutil"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrCognitoNotFound     = errors.New("ResourceNotFoundException")
	ErrCognitoBadRequest   = errors.New("InvalidParameterException")
	ErrCognitoUnauthorized = errors.New("NotAuthorizedException")
	ErrCognitoUserExists   = errors.New("UsernameExistsException")
	ErrCognitoUserNotFound = errors.New("UserNotFoundException")
	ErrCognitoInvalidPass  = errors.New("InvalidPasswordException")
)

const (
	DefaultCognitoRegion   = "us-east-1"
	CognitoLabIssuerHost   = "http://127.0.0.1:4566"
	cognitoTokenTTLSeconds = 3600
	cognitoBcryptCost      = 10
)

const cognitoSchema = `
CREATE TABLE IF NOT EXISTS cognito_user_pools (
  account_id TEXT NOT NULL,
  region TEXT NOT NULL,
  pool_id TEXT NOT NULL,
  name TEXT NOT NULL,
  arn TEXT NOT NULL,
  kid TEXT NOT NULL,
  sealed_private_key BLOB NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, pool_id)
);
CREATE INDEX IF NOT EXISTS idx_cognito_pools_name ON cognito_user_pools(account_id, name);
CREATE TABLE IF NOT EXISTS cognito_user_pool_clients (
  account_id TEXT NOT NULL,
  pool_id TEXT NOT NULL,
  client_id TEXT NOT NULL,
  client_name TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, pool_id, client_id)
);
CREATE TABLE IF NOT EXISTS cognito_users (
  account_id TEXT NOT NULL,
  pool_id TEXT NOT NULL,
  username TEXT NOT NULL,
  sub TEXT NOT NULL,
  password_hash TEXT NOT NULL,
  user_status TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, pool_id, username)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_cognito_users_sub ON cognito_users(account_id, pool_id, sub);
CREATE TABLE IF NOT EXISTS cognito_refresh_tokens (
  token_hash TEXT PRIMARY KEY,
  account_id TEXT NOT NULL,
  pool_id TEXT NOT NULL,
  client_id TEXT NOT NULL,
  username TEXT NOT NULL,
  expires_at INTEGER NOT NULL
);
`

// CognitoLambdaConfig is the lab subset of LambdaConfigType trigger ARNs.
type CognitoLambdaConfig struct {
	PreSignUp                   string `json:"PreSignUp,omitempty"`
	PostConfirmation            string `json:"PostConfirmation,omitempty"`
	PreAuthentication           string `json:"PreAuthentication,omitempty"`
	PostAuthentication          string `json:"PostAuthentication,omitempty"`
	PreTokenGeneration          string `json:"PreTokenGeneration,omitempty"`
	UserMigration               string `json:"UserMigration,omitempty"`
	DefineAuthChallenge         string `json:"DefineAuthChallenge,omitempty"`
	CreateAuthChallenge         string `json:"CreateAuthChallenge,omitempty"`
	VerifyAuthChallengeResponse string `json:"VerifyAuthChallengeResponse,omitempty"`
	CustomMessage               string `json:"CustomMessage,omitempty"`
}

// HasTriggers reports whether any trigger ARN is set.
func (c CognitoLambdaConfig) HasTriggers() bool {
	return strings.TrimSpace(c.PreSignUp) != "" ||
		strings.TrimSpace(c.PostConfirmation) != "" ||
		strings.TrimSpace(c.PreAuthentication) != "" ||
		strings.TrimSpace(c.PostAuthentication) != "" ||
		strings.TrimSpace(c.PreTokenGeneration) != "" ||
		strings.TrimSpace(c.UserMigration) != "" ||
		strings.TrimSpace(c.DefineAuthChallenge) != "" ||
		strings.TrimSpace(c.CreateAuthChallenge) != "" ||
		strings.TrimSpace(c.VerifyAuthChallengeResponse) != "" ||
		strings.TrimSpace(c.CustomMessage) != ""
}

// CognitoUserPool is a lab User Pool row.
type CognitoUserPool struct {
	PoolID       string
	Region       string
	Name         string
	ARN          string
	Kid          string
	CreatedAt    int64
	RoleArn      string
	LambdaConfig CognitoLambdaConfig
}

// CognitoUserPoolClient is an app client.
type CognitoUserPoolClient struct {
	ClientID   string
	ClientName string
	PoolID     string
	CreatedAt  int64
}

// CognitoUser is a pool user (password never returned).
type CognitoUser struct {
	Username   string
	Sub        string
	UserStatus string
	PoolID     string
	CreatedAt  int64
}

// CognitoAuthResult mirrors AuthenticationResultType lite.
type CognitoAuthResult struct {
	AccessToken  string
	IDToken      string
	RefreshToken string
	ExpiresIn    int
	TokenType    string
}

// EnsureCognitoSchema creates Cognito tables if missing.
func EnsureCognitoSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure cognito schema: db is nil")
	}
	if _, err := db.Exec(cognitoSchema); err != nil {
		return fmt.Errorf("ensure cognito schema: %w", err)
	}
	if err := EnsureCognitoMFASchema(db); err != nil {
		return err
	}
	if err := EnsureCognitoSRPSchema(db); err != nil {
		return err
	}
	if err := EnsureCognitoTriggerSchema(db); err != nil {
		return err
	}
	if err := EnsureCognitoCustomMessageSchema(db); err != nil {
		return err
	}
	if err := EnsureCognitoCustomAuthSchema(db); err != nil {
		return err
	}
	if err := EnsureCognitoForgotAttrsSchema(db); err != nil {
		return err
	}
	return nil
}

// EnsureCognitoSchema ensures Cognito tables on an open store.
func (s *Store) EnsureCognitoSchema() error {
	return EnsureCognitoSchema(s.db)
}

// CognitoIssuerURL returns the stable lab issuer for a pool.
func CognitoIssuerURL(region, poolID string) string {
	if region == "" {
		region = DefaultCognitoRegion
	}
	return fmt.Sprintf("%s/cognito-idp/%s/%s", CognitoLabIssuerHost, region, poolID)
}

// CognitoJWKSPath returns the HTTP path for JWKS (no host).
func CognitoJWKSPath(region, poolID string) string {
	if region == "" {
		region = DefaultCognitoRegion
	}
	return fmt.Sprintf("/cognito-idp/%s/%s/.well-known/jwks.json", region, poolID)
}

// CognitoPoolARN builds arn:aws:cognito-idp:REGION:ACCOUNT:userpool/POOL_ID
func CognitoPoolARN(region, accountID, poolID string) string {
	if region == "" {
		region = DefaultCognitoRegion
	}
	return fmt.Sprintf("arn:aws:cognito-idp:%s:%s:userpool/%s", region, accountID, poolID)
}

func newCognitoPoolID(region string) string {
	if region == "" {
		region = DefaultCognitoRegion
	}
	raw := strings.ReplaceAll(uuid.NewString(), "-", "")
	return region + "_" + raw[:9]
}

func newCognitoClientID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func newCognitoKid() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// CreateCognitoUserPool creates a pool and generates an RSA signing key (sealed at rest).
func (s *Store) CreateCognitoUserPool(accountID, region, name string) (CognitoUserPool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return CognitoUserPool{}, fmt.Errorf("%w: PoolName required", ErrCognitoBadRequest)
	}
	if region == "" {
		region = DefaultCognitoRegion
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return CognitoUserPool{}, fmt.Errorf("generate pool key: %w", err)
	}
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return CognitoUserPool{}, fmt.Errorf("marshal pool key: %w", err)
	}
	sealed, err := s.SealWithMaster(pkcs8)
	if err != nil {
		return CognitoUserPool{}, fmt.Errorf("seal pool key: %w", err)
	}
	poolID := newCognitoPoolID(region)
	kid := newCognitoKid()
	arn := CognitoPoolARN(region, accountID, poolID)
	now := time.Now().UTC().UnixMilli()
	_, err = s.db.Exec(
		`INSERT INTO cognito_user_pools (account_id, region, pool_id, name, arn, kid, sealed_private_key, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		accountID, region, poolID, name, arn, kid, sealed, now,
	)
	if err != nil {
		return CognitoUserPool{}, fmt.Errorf("create user pool: %w", err)
	}
	return CognitoUserPool{PoolID: poolID, Region: region, Name: name, ARN: arn, Kid: kid, CreatedAt: now}, nil
}

// DescribeCognitoUserPool returns a pool by ID.
func (s *Store) DescribeCognitoUserPool(accountID, poolID string) (CognitoUserPool, error) {
	poolID = strings.TrimSpace(poolID)
	var p CognitoUserPool
	var roleARN, lambdaJSON string
	err := s.db.QueryRow(
		`SELECT pool_id, region, name, arn, kid, created_at,
		        COALESCE(role_arn, ''), COALESCE(lambda_config_json, '{}')
		 FROM cognito_user_pools
		 WHERE account_id = ? AND pool_id = ?`,
		accountID, poolID,
	).Scan(&p.PoolID, &p.Region, &p.Name, &p.ARN, &p.Kid, &p.CreatedAt, &roleARN, &lambdaJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return CognitoUserPool{}, ErrCognitoNotFound
	}
	if err != nil {
		return CognitoUserPool{}, fmt.Errorf("describe user pool: %w", err)
	}
	p.RoleArn = roleARN
	p.LambdaConfig = parseCognitoLambdaConfig(lambdaJSON)
	return p, nil
}

// GetCognitoUserPoolByID returns a pool by global pool_id (lab JWKS lookup).
func (s *Store) GetCognitoUserPoolByID(poolID string) (accountID string, pool CognitoUserPool, err error) {
	poolID = strings.TrimSpace(poolID)
	var roleARN, lambdaJSON string
	err = s.db.QueryRow(
		`SELECT account_id, pool_id, region, name, arn, kid, created_at,
		        COALESCE(role_arn, ''), COALESCE(lambda_config_json, '{}')
		 FROM cognito_user_pools WHERE pool_id = ?`,
		poolID,
	).Scan(&accountID, &pool.PoolID, &pool.Region, &pool.Name, &pool.ARN, &pool.Kid, &pool.CreatedAt, &roleARN, &lambdaJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return "", CognitoUserPool{}, ErrCognitoNotFound
	}
	if err != nil {
		return "", CognitoUserPool{}, fmt.Errorf("get user pool by id: %w", err)
	}
	pool.RoleArn = roleARN
	pool.LambdaConfig = parseCognitoLambdaConfig(lambdaJSON)
	return accountID, pool, nil
}

// ListCognitoUserPools lists pools for an account.
func (s *Store) ListCognitoUserPools(accountID string) ([]CognitoUserPool, error) {
	rows, err := s.db.Query(
		`SELECT pool_id, region, name, arn, kid, created_at FROM cognito_user_pools
		 WHERE account_id = ? ORDER BY created_at`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list user pools: %w", err)
	}
	defer rows.Close()
	var out []CognitoUserPool
	for rows.Next() {
		var p CognitoUserPool
		if err := rows.Scan(&p.PoolID, &p.Region, &p.Name, &p.ARN, &p.Kid, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("list user pools scan: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// DeleteCognitoUserPool deletes a pool and related rows.
func (s *Store) DeleteCognitoUserPool(accountID, poolID string) error {
	poolID = strings.TrimSpace(poolID)
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("delete user pool begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.Exec(`DELETE FROM cognito_user_pools WHERE account_id = ? AND pool_id = ?`, accountID, poolID)
	if err != nil {
		return fmt.Errorf("delete user pool: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrCognitoNotFound
	}
	_, _ = tx.Exec(`DELETE FROM cognito_user_pool_clients WHERE account_id = ? AND pool_id = ?`, accountID, poolID)
	_, _ = tx.Exec(`DELETE FROM cognito_users WHERE account_id = ? AND pool_id = ?`, accountID, poolID)
	_, _ = tx.Exec(`DELETE FROM cognito_refresh_tokens WHERE account_id = ? AND pool_id = ?`, accountID, poolID)
	_, _ = tx.Exec(`DELETE FROM cognito_mfa_sessions WHERE account_id = ? AND pool_id = ?`, accountID, poolID)
	_, _ = tx.Exec(`DELETE FROM cognito_srp_sessions WHERE account_id = ? AND pool_id = ?`, accountID, poolID)
	_, _ = tx.Exec(`DELETE FROM cognito_custom_auth_sessions WHERE account_id = ? AND pool_id = ?`, accountID, poolID)
	_, _ = tx.Exec(`DELETE FROM cognito_custom_messages WHERE account_id = ? AND pool_id = ?`, accountID, poolID)
	_, _ = tx.Exec(`DELETE FROM cognito_confirmation_codes WHERE account_id = ? AND pool_id = ?`, accountID, poolID)
	_, _ = tx.Exec(`DELETE FROM cognito_user_attributes WHERE account_id = ? AND pool_id = ?`, accountID, poolID)
	return tx.Commit()
}

// CreateCognitoUserPoolClient creates an app client.
func (s *Store) CreateCognitoUserPoolClient(accountID, poolID, clientName string) (CognitoUserPoolClient, error) {
	poolID = strings.TrimSpace(poolID)
	clientName = strings.TrimSpace(clientName)
	if poolID == "" {
		return CognitoUserPoolClient{}, fmt.Errorf("%w: UserPoolId required", ErrCognitoBadRequest)
	}
	if clientName == "" {
		clientName = "lab-client"
	}
	if _, err := s.DescribeCognitoUserPool(accountID, poolID); err != nil {
		return CognitoUserPoolClient{}, err
	}
	clientID := newCognitoClientID()
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO cognito_user_pool_clients (account_id, pool_id, client_id, client_name, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		accountID, poolID, clientID, clientName, now,
	)
	if err != nil {
		return CognitoUserPoolClient{}, fmt.Errorf("create user pool client: %w", err)
	}
	return CognitoUserPoolClient{ClientID: clientID, ClientName: clientName, PoolID: poolID, CreatedAt: now}, nil
}

// DescribeCognitoUserPoolClient returns a client.
func (s *Store) DescribeCognitoUserPoolClient(accountID, poolID, clientID string) (CognitoUserPoolClient, error) {
	var c CognitoUserPoolClient
	err := s.db.QueryRow(
		`SELECT client_id, client_name, pool_id, created_at FROM cognito_user_pool_clients
		 WHERE account_id = ? AND pool_id = ? AND client_id = ?`,
		accountID, strings.TrimSpace(poolID), strings.TrimSpace(clientID),
	).Scan(&c.ClientID, &c.ClientName, &c.PoolID, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return CognitoUserPoolClient{}, ErrCognitoNotFound
	}
	if err != nil {
		return CognitoUserPoolClient{}, fmt.Errorf("describe user pool client: %w", err)
	}
	return c, nil
}

// ListCognitoUserPoolClients lists clients for a pool.
func (s *Store) ListCognitoUserPoolClients(accountID, poolID string) ([]CognitoUserPoolClient, error) {
	if _, err := s.DescribeCognitoUserPool(accountID, poolID); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT client_id, client_name, pool_id, created_at FROM cognito_user_pool_clients
		 WHERE account_id = ? AND pool_id = ? ORDER BY created_at`,
		accountID, strings.TrimSpace(poolID),
	)
	if err != nil {
		return nil, fmt.Errorf("list user pool clients: %w", err)
	}
	defer rows.Close()
	var out []CognitoUserPoolClient
	for rows.Next() {
		var c CognitoUserPoolClient
		if err := rows.Scan(&c.ClientID, &c.ClientName, &c.PoolID, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("list user pool clients scan: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// DeleteCognitoUserPoolClient deletes a client.
func (s *Store) DeleteCognitoUserPoolClient(accountID, poolID, clientID string) error {
	res, err := s.db.Exec(
		`DELETE FROM cognito_user_pool_clients WHERE account_id = ? AND pool_id = ? AND client_id = ?`,
		accountID, strings.TrimSpace(poolID), strings.TrimSpace(clientID),
	)
	if err != nil {
		return fmt.Errorf("delete user pool client: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrCognitoNotFound
	}
	return nil
}

func (s *Store) loadPoolPrivateKey(accountID, poolID string) (*rsa.PrivateKey, string, string, error) {
	var sealed []byte
	var kid, region string
	err := s.db.QueryRow(
		`SELECT sealed_private_key, kid, region FROM cognito_user_pools WHERE account_id = ? AND pool_id = ?`,
		accountID, poolID,
	).Scan(&sealed, &kid, &region)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", "", ErrCognitoNotFound
	}
	if err != nil {
		return nil, "", "", fmt.Errorf("load pool key: %w", err)
	}
	pkcs8, err := s.UnsealWithMaster(sealed)
	if err != nil {
		return nil, "", "", fmt.Errorf("unseal pool key: %w", err)
	}
	parsed, err := x509.ParsePKCS8PrivateKey(pkcs8)
	if err != nil {
		return nil, "", "", fmt.Errorf("parse pool key: %w", err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, "", "", fmt.Errorf("pool key is not RSA")
	}
	return key, kid, region, nil
}

// CognitoJWKSJSON returns JWKS for a pool (by pool id, any account).
func (s *Store) CognitoJWKSJSON(poolID string) ([]byte, error) {
	accountID, pool, err := s.GetCognitoUserPoolByID(poolID)
	if err != nil {
		return nil, err
	}
	key, kid, _, err := s.loadPoolPrivateKey(accountID, pool.PoolID)
	if err != nil {
		return nil, err
	}
	if kid == "" {
		kid = pool.Kid
	}
	return jwtutil.MarshalJWKS(&key.PublicKey, kid)
}

func hashPassword(password string) (string, error) {
	if len(password) < 6 {
		return "", fmt.Errorf("%w: password must be at least 6 characters", ErrCognitoInvalidPass)
	}
	sum, err := bcrypt.GenerateFromPassword([]byte(password), cognitoBcryptCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(sum), nil
}

func checkPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// AdminCreateCognitoUser creates a CONFIRMED user with a permanent password (lab lite).
func (s *Store) AdminCreateCognitoUser(accountID, poolID, username, password string) (CognitoUser, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return CognitoUser{}, fmt.Errorf("%w: Username required", ErrCognitoBadRequest)
	}
	if _, err := s.DescribeCognitoUserPool(accountID, poolID); err != nil {
		return CognitoUser{}, err
	}
	hash, err := hashPassword(password)
	if err != nil {
		return CognitoUser{}, err
	}
	sub := uuid.NewString()
	now := time.Now().UTC().UnixMilli()
	_, err = s.db.Exec(
		`INSERT INTO cognito_users (account_id, pool_id, username, sub, password_hash, user_status, created_at)
		 VALUES (?, ?, ?, ?, ?, 'CONFIRMED', ?)`,
		accountID, strings.TrimSpace(poolID), username, sub, hash, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "constraint") {
			return CognitoUser{}, ErrCognitoUserExists
		}
		return CognitoUser{}, fmt.Errorf("admin create user: %w", err)
	}
	if err := s.storeUserSRPVerifier(accountID, strings.TrimSpace(poolID), username, password); err != nil {
		return CognitoUser{}, err
	}
	// CustomMessage_AdminCreateUser: lab has no SES; Invoke, render/store body fields for tests.
	cmPayload, err := s.FireCognitoTriggerEvent(accountID, strings.TrimSpace(poolID), CognitoTriggerCustomMessage, CognitoTriggerEventInput{
		TriggerSource: "CustomMessage_AdminCreateUser",
		UserPoolID:    strings.TrimSpace(poolID),
		Username:      username,
		UserSub:       sub,
		UserStatus:    "CONFIRMED",
		CodeParameter: "{####}",
	})
	if err != nil {
		return CognitoUser{}, err
	}
	if err := s.applyCustomMessagePayload(accountID, strings.TrimSpace(poolID), username, "CustomMessage_AdminCreateUser", "{####}", cmPayload); err != nil {
		return CognitoUser{}, err
	}
	return CognitoUser{Username: username, Sub: sub, UserStatus: "CONFIRMED", PoolID: poolID, CreatedAt: now}, nil
}

// SignUpCognitoUser creates an UNCONFIRMED user (ConfirmSignUp sets CONFIRMED).
func (s *Store) SignUpCognitoUser(accountID, clientID, username, password string) (CognitoUser, string, error) {
	username = strings.TrimSpace(username)
	clientID = strings.TrimSpace(clientID)
	if username == "" || clientID == "" {
		return CognitoUser{}, "", fmt.Errorf("%w: Username and ClientId required", ErrCognitoBadRequest)
	}
	acct, poolID, err := s.lookupClient(clientID)
	if err != nil {
		return CognitoUser{}, "", err
	}
	if accountID != "" && acct != accountID {
		return CognitoUser{}, "", ErrCognitoNotFound
	}
	if _, err := s.FireCognitoTriggerIfConfigured(
		acct, poolID, clientID, username, "", "UNCONFIRMED",
		CognitoTriggerPreSignUp, "PreSignUp_SignUp",
	); err != nil {
		return CognitoUser{}, "", err
	}
	hash, err := hashPassword(password)
	if err != nil {
		return CognitoUser{}, "", err
	}
	sub := uuid.NewString()
	now := time.Now().UTC().UnixMilli()
	_, err = s.db.Exec(
		`INSERT INTO cognito_users (account_id, pool_id, username, sub, password_hash, user_status, created_at)
		 VALUES (?, ?, ?, ?, ?, 'UNCONFIRMED', ?)`,
		acct, poolID, username, sub, hash, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "constraint") {
			return CognitoUser{}, "", ErrCognitoUserExists
		}
		return CognitoUser{}, "", fmt.Errorf("sign up: %w", err)
	}
	if err := s.storeUserSRPVerifier(acct, poolID, username, password); err != nil {
		return CognitoUser{}, "", err
	}
	// CustomMessage_SignUp: lab has no SES; Invoke, render/store body fields for tests.
	cmPayload, err := s.FireCognitoTriggerEvent(acct, poolID, CognitoTriggerCustomMessage, CognitoTriggerEventInput{
		TriggerSource: "CustomMessage_SignUp",
		UserPoolID:    poolID,
		Username:      username,
		ClientID:      clientID,
		UserSub:       sub,
		UserStatus:    "UNCONFIRMED",
		CodeParameter: "{####}",
	})
	if err != nil {
		return CognitoUser{}, "", err
	}
	if err := s.applyCustomMessagePayload(acct, poolID, username, "CustomMessage_SignUp", "{####}", cmPayload); err != nil {
		return CognitoUser{}, "", err
	}
	return CognitoUser{Username: username, Sub: sub, UserStatus: "UNCONFIRMED", PoolID: poolID, CreatedAt: now}, poolID, nil
}

// ConfirmSignUpCognitoUser marks a user CONFIRMED (lab: any confirmation code accepted if non-empty).
func (s *Store) ConfirmSignUpCognitoUser(clientID, username, confirmationCode string) error {
	clientID = strings.TrimSpace(clientID)
	username = strings.TrimSpace(username)
	if clientID == "" || username == "" || strings.TrimSpace(confirmationCode) == "" {
		return fmt.Errorf("%w: ClientId, Username, and ConfirmationCode required", ErrCognitoBadRequest)
	}
	var acct, poolID, sub string
	err := s.db.QueryRow(
		`SELECT c.account_id, c.pool_id, u.sub FROM cognito_user_pool_clients c
		 JOIN cognito_users u ON u.account_id = c.account_id AND u.pool_id = c.pool_id
		 WHERE c.client_id = ? AND u.username = ?`,
		clientID, username,
	).Scan(&acct, &poolID, &sub)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrCognitoUserNotFound
	}
	if err != nil {
		return fmt.Errorf("confirm signup: %w", err)
	}
	_, err = s.db.Exec(
		`UPDATE cognito_users SET user_status = 'CONFIRMED' WHERE account_id = ? AND pool_id = ? AND username = ?`,
		acct, poolID, username,
	)
	if err != nil {
		return fmt.Errorf("confirm signup update: %w", err)
	}
	if _, err := s.FireCognitoTriggerIfConfigured(
		acct, poolID, clientID, username, sub, "CONFIRMED",
		CognitoTriggerPostConfirmation, "PostConfirmation_ConfirmSignUp",
	); err != nil {
		return err
	}
	return nil
}

// CognitoCodeDeliveryDetails is the lite ForgotPassword / ResendConfirmationCode delivery stub (no SES).
type CognitoCodeDeliveryDetails struct {
	Destination    string
	DeliveryMedium string
	AttributeName  string
}

// ForgotPasswordCognitoUser fires CustomMessage_ForgotPassword (stub code {####} / 123456; no SES).
func (s *Store) ForgotPasswordCognitoUser(clientID, username string) (CognitoCodeDeliveryDetails, error) {
	clientID = strings.TrimSpace(clientID)
	username = strings.TrimSpace(username)
	if clientID == "" || username == "" {
		return CognitoCodeDeliveryDetails{}, fmt.Errorf("%w: ClientId and Username required", ErrCognitoBadRequest)
	}
	acct, poolID, err := s.lookupClient(clientID)
	if err != nil {
		return CognitoCodeDeliveryDetails{}, err
	}
	sub, _, status, err := s.getUser(acct, poolID, username)
	if err != nil {
		return CognitoCodeDeliveryDetails{}, err
	}
	cmPayload, err := s.FireCognitoTriggerEvent(acct, poolID, CognitoTriggerCustomMessage, CognitoTriggerEventInput{
		TriggerSource: "CustomMessage_ForgotPassword",
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
	if err := s.applyCustomMessagePayload(acct, poolID, username, "CustomMessage_ForgotPassword", "{####}", cmPayload); err != nil {
		return CognitoCodeDeliveryDetails{}, err
	}
	if err := s.storeCognitoConfirmationCode(acct, poolID, username, cognitoConfirmPurposeForgotPassword, CognitoLabConfirmationCode, "email"); err != nil {
		return CognitoCodeDeliveryDetails{}, err
	}
	return CognitoCodeDeliveryDetails{
		Destination:    "t***@example.com",
		DeliveryMedium: "EMAIL",
		AttributeName:  "email",
	}, nil
}

// ResendConfirmationCodeCognitoUser fires CustomMessage_ResendCode for an UNCONFIRMED user (stub; no SES).
func (s *Store) ResendConfirmationCodeCognitoUser(clientID, username string) (CognitoCodeDeliveryDetails, error) {
	clientID = strings.TrimSpace(clientID)
	username = strings.TrimSpace(username)
	if clientID == "" || username == "" {
		return CognitoCodeDeliveryDetails{}, fmt.Errorf("%w: ClientId and Username required", ErrCognitoBadRequest)
	}
	acct, poolID, err := s.lookupClient(clientID)
	if err != nil {
		return CognitoCodeDeliveryDetails{}, err
	}
	sub, _, status, err := s.getUser(acct, poolID, username)
	if err != nil {
		return CognitoCodeDeliveryDetails{}, err
	}
	if status != "UNCONFIRMED" {
		return CognitoCodeDeliveryDetails{}, fmt.Errorf("%w: User cannot be confirmed. Current status is %s", ErrCognitoBadRequest, status)
	}
	cmPayload, err := s.FireCognitoTriggerEvent(acct, poolID, CognitoTriggerCustomMessage, CognitoTriggerEventInput{
		TriggerSource: "CustomMessage_ResendCode",
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
	if err := s.applyCustomMessagePayload(acct, poolID, username, "CustomMessage_ResendCode", "{####}", cmPayload); err != nil {
		return CognitoCodeDeliveryDetails{}, err
	}
	return CognitoCodeDeliveryDetails{
		Destination:    "t***@example.com",
		DeliveryMedium: "EMAIL",
		AttributeName:  "email",
	}, nil
}

func (s *Store) lookupClient(clientID string) (accountID, poolID string, err error) {
	err = s.db.QueryRow(
		`SELECT account_id, pool_id FROM cognito_user_pool_clients WHERE client_id = ?`,
		strings.TrimSpace(clientID),
	).Scan(&accountID, &poolID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", ErrCognitoNotFound
	}
	if err != nil {
		return "", "", fmt.Errorf("lookup client: %w", err)
	}
	return accountID, poolID, nil
}

func (s *Store) getUser(accountID, poolID, username string) (sub, hash, status string, err error) {
	err = s.db.QueryRow(
		`SELECT sub, password_hash, user_status FROM cognito_users
		 WHERE account_id = ? AND pool_id = ? AND username = ?`,
		accountID, poolID, username,
	).Scan(&sub, &hash, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", "", ErrCognitoUserNotFound
	}
	if err != nil {
		return "", "", "", fmt.Errorf("get user: %w", err)
	}
	return sub, hash, status, nil
}

// InitiateCognitoAuth USER_PASSWORD_AUTH (client + username + password).
// When the user has software-token MFA enabled, returns ChallengeName SOFTWARE_TOKEN_MFA
// and a Session (no tokens) instead of AuthenticationResult.
func (s *Store) InitiateCognitoAuth(clientID, username, password string) (CognitoAuthOutcome, error) {
	accountID, poolID, err := s.lookupClient(clientID)
	if err != nil {
		return CognitoAuthOutcome{}, err
	}
	return s.authenticateAndIssue(accountID, poolID, clientID, username, password)
}

// AdminInitiateCognitoAuth USER_PASSWORD_AUTH with explicit pool.
func (s *Store) AdminInitiateCognitoAuth(accountID, poolID, clientID, username, password string) (CognitoAuthOutcome, error) {
	if _, err := s.DescribeCognitoUserPoolClient(accountID, poolID, clientID); err != nil {
		return CognitoAuthOutcome{}, err
	}
	return s.authenticateAndIssue(accountID, poolID, clientID, username, password)
}

func (s *Store) authenticateAndIssue(accountID, poolID, clientID, username, password string) (CognitoAuthOutcome, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return CognitoAuthOutcome{}, fmt.Errorf("%w: USERNAME and PASSWORD required", ErrCognitoBadRequest)
	}
	sub, hash, status, err := s.getUser(accountID, poolID, username)
	if err != nil {
		if errors.Is(err, ErrCognitoUserNotFound) {
			migrated, mErr := s.tryUserMigrationAuthentication(accountID, poolID, clientID, username, password)
			if mErr != nil {
				return CognitoAuthOutcome{}, mErr
			}
			if !migrated {
				return CognitoAuthOutcome{}, ErrCognitoUnauthorized
			}
			sub, hash, status, err = s.getUser(accountID, poolID, username)
			if err != nil {
				return CognitoAuthOutcome{}, err
			}
		} else {
			return CognitoAuthOutcome{}, err
		}
	}
	if status != "CONFIRMED" {
		return CognitoAuthOutcome{}, fmt.Errorf("%w: User is not confirmed", ErrCognitoUnauthorized)
	}
	if _, err := s.FireCognitoTriggerIfConfigured(
		accountID, poolID, clientID, username, sub, status,
		CognitoTriggerPreAuthentication, "PreAuthentication_Authentication",
	); err != nil {
		return CognitoAuthOutcome{}, err
	}
	if !checkPassword(hash, password) {
		return CognitoAuthOutcome{}, ErrCognitoUnauthorized
	}
	mfaOn, err := s.userMFAEnabled(accountID, poolID, username)
	if err != nil {
		return CognitoAuthOutcome{}, err
	}
	if mfaOn {
		return s.createSOFTWARETokenMFAChallenge(accountID, poolID, clientID, username)
	}
	result, err := s.issueTokensAfterAuth(accountID, poolID, clientID, username, sub, status, true)
	if err != nil {
		return CognitoAuthOutcome{}, err
	}
	return CognitoAuthOutcome{CognitoAuthResult: result}, nil
}

// tryUserMigrationAuthentication Invokes UserMigration when configured for a missing user.
// Returns migrated=true when the Lambda response includes userAttributes and the user was created.
func (s *Store) tryUserMigrationAuthentication(accountID, poolID, clientID, username, password string) (migrated bool, err error) {
	pool, err := s.DescribeCognitoUserPool(accountID, poolID)
	if err != nil {
		return false, err
	}
	if pool.LambdaConfig.ARNFor(CognitoTriggerUserMigration) == "" {
		return false, nil
	}
	payload, err := s.FireCognitoTriggerEvent(accountID, poolID, CognitoTriggerUserMigration, CognitoTriggerEventInput{
		TriggerSource: "UserMigration_Authentication",
		UserPoolID:    poolID,
		Username:      username,
		ClientID:      clientID,
		Password:      password,
	})
	if err != nil {
		return false, err
	}
	attrs, ok, err := ParseCognitoUserMigrationAttributes(payload)
	if err != nil {
		return false, fmt.Errorf("%w: %v", ErrCognitoTriggerFailed, err)
	}
	if !ok {
		return false, ErrCognitoUserNotFound
	}
	_ = attrs // lab stores password only; attributes acknowledge successful migration response
	if _, err := s.AdminCreateCognitoUser(accountID, poolID, username, password); err != nil {
		if errors.Is(err, ErrCognitoUserExists) {
			return true, nil
		}
		return false, err
	}
	return true, nil
}

// issueTokensAfterAuth runs PreTokenGeneration (and optional PostAuthentication) around token minting.
func (s *Store) issueTokensAfterAuth(
	accountID, poolID, clientID, username, sub, status string,
	firePostAuth bool,
) (CognitoAuthResult, error) {
	if status == "" {
		status = "CONFIRMED"
	}
	payload, err := s.FireCognitoTriggerIfConfigured(
		accountID, poolID, clientID, username, sub, status,
		CognitoTriggerPreTokenGeneration, "TokenGeneration_Authentication",
	)
	if err != nil {
		return CognitoAuthResult{}, err
	}
	override, err := ParseCognitoPreTokenClaimsOverride(payload)
	if err != nil {
		return CognitoAuthResult{}, fmt.Errorf("%w: %v", ErrCognitoTriggerFailed, err)
	}
	result, err := s.issueTokens(accountID, poolID, clientID, username, sub, override)
	if err != nil {
		return CognitoAuthResult{}, err
	}
	if firePostAuth {
		if _, err := s.FireCognitoTriggerIfConfigured(
			accountID, poolID, clientID, username, sub, status,
			CognitoTriggerPostAuthentication, "PostAuthentication_Authentication",
		); err != nil {
			return CognitoAuthResult{}, err
		}
	}
	return result, nil
}

func hashRefreshToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func (s *Store) issueTokens(accountID, poolID, clientID, username, sub string, override CognitoClaimsOverride) (CognitoAuthResult, error) {
	key, kid, region, err := s.loadPoolPrivateKey(accountID, poolID)
	if err != nil {
		return CognitoAuthResult{}, err
	}
	iss := CognitoIssuerURL(region, poolID)
	now := time.Now().UTC()
	exp := now.Add(time.Duration(cognitoTokenTTLSeconds) * time.Second).Unix()
	iat := now.Unix()

	idClaimMap := map[string]any{
		"sub":              sub,
		"iss":              iss,
		"aud":              clientID,
		"token_use":        "id",
		"auth_time":        iat,
		"iat":              iat,
		"exp":              exp,
		"cognito:username": username,
		"email_verified":   false,
	}
	ApplyCognitoClaimsOverrideToIDClaims(idClaimMap, override)
	idClaims, _ := json.Marshal(idClaimMap)
	accessClaims, _ := json.Marshal(map[string]any{
		"sub":       sub,
		"iss":       iss,
		"client_id": clientID,
		"token_use": "access",
		"scope":     "openid",
		"auth_time": iat,
		"iat":       iat,
		"exp":       exp,
		"username":  username,
	})
	idToken, err := jwtutil.SignRS256(idClaims, key, kid)
	if err != nil {
		return CognitoAuthResult{}, fmt.Errorf("sign id token: %w", err)
	}
	accessToken, err := jwtutil.SignRS256(accessClaims, key, kid)
	if err != nil {
		return CognitoAuthResult{}, fmt.Errorf("sign access token: %w", err)
	}
	refreshRaw := uuid.NewString() + uuid.NewString()
	tokenHash := hashRefreshToken(refreshRaw)
	_, err = s.db.Exec(
		`INSERT INTO cognito_refresh_tokens (token_hash, account_id, pool_id, client_id, username, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		tokenHash, accountID, poolID, clientID, username, now.Add(30*24*time.Hour).Unix(),
	)
	if err != nil {
		return CognitoAuthResult{}, fmt.Errorf("store refresh token: %w", err)
	}
	return CognitoAuthResult{
		AccessToken:  accessToken,
		IDToken:      idToken,
		RefreshToken: refreshRaw,
		ExpiresIn:    cognitoTokenTTLSeconds,
		TokenType:    "Bearer",
	}, nil
}

// RefreshCognitoTokens validates a stored refresh token hash + clientId, rotates the refresh
// token, and issues new access/id tokens (lab: rotation always on).
func (s *Store) RefreshCognitoTokens(accountID, clientID, refreshToken string) (CognitoAuthResult, error) {
	clientID = strings.TrimSpace(clientID)
	refreshToken = strings.TrimSpace(refreshToken)
	if clientID == "" || refreshToken == "" {
		return CognitoAuthResult{}, fmt.Errorf("%w: ClientId and REFRESH_TOKEN required", ErrCognitoBadRequest)
	}
	tokenHash := hashRefreshToken(refreshToken)
	var acct, poolID, username string
	var expiresAt int64
	err := s.db.QueryRow(
		`SELECT account_id, pool_id, username, expires_at FROM cognito_refresh_tokens
		 WHERE token_hash = ? AND client_id = ?`,
		tokenHash, clientID,
	).Scan(&acct, &poolID, &username, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return CognitoAuthResult{}, ErrCognitoUnauthorized
	}
	if err != nil {
		return CognitoAuthResult{}, fmt.Errorf("lookup refresh token: %w", err)
	}
	if accountID != "" && accountID != acct {
		return CognitoAuthResult{}, ErrCognitoUnauthorized
	}
	if time.Now().UTC().Unix() > expiresAt {
		_, _ = s.db.Exec(`DELETE FROM cognito_refresh_tokens WHERE token_hash = ?`, tokenHash)
		return CognitoAuthResult{}, ErrCognitoUnauthorized
	}
	sub, _, status, err := s.getUser(acct, poolID, username)
	if err != nil {
		if errors.Is(err, ErrCognitoUserNotFound) {
			return CognitoAuthResult{}, ErrCognitoUnauthorized
		}
		return CognitoAuthResult{}, err
	}
	if status != "CONFIRMED" {
		return CognitoAuthResult{}, fmt.Errorf("%w: User is not confirmed", ErrCognitoUnauthorized)
	}
	// Rotate: invalidate presented refresh token before issuing a new one.
	if _, err := s.db.Exec(`DELETE FROM cognito_refresh_tokens WHERE token_hash = ?`, tokenHash); err != nil {
		return CognitoAuthResult{}, fmt.Errorf("rotate refresh token: %w", err)
	}
	payload, err := s.FireCognitoTriggerIfConfigured(
		acct, poolID, clientID, username, sub, status,
		CognitoTriggerPreTokenGeneration, "TokenGeneration_RefreshTokens",
	)
	if err != nil {
		return CognitoAuthResult{}, err
	}
	override, err := ParseCognitoPreTokenClaimsOverride(payload)
	if err != nil {
		return CognitoAuthResult{}, fmt.Errorf("%w: %v", ErrCognitoTriggerFailed, err)
	}
	return s.issueTokens(acct, poolID, clientID, username, sub, override)
}

// RevokeCognitoToken deletes the refresh token hash so subsequent refresh fails.
// Public IdP shape: ClientId + Token (refresh token). Does not log the raw token.
func (s *Store) RevokeCognitoToken(clientID, refreshToken string) error {
	clientID = strings.TrimSpace(clientID)
	refreshToken = strings.TrimSpace(refreshToken)
	if clientID == "" || refreshToken == "" {
		return fmt.Errorf("%w: ClientId and Token required", ErrCognitoBadRequest)
	}
	tokenHash := hashRefreshToken(refreshToken)
	res, err := s.db.Exec(
		`DELETE FROM cognito_refresh_tokens WHERE token_hash = ? AND client_id = ?`,
		tokenHash, clientID,
	)
	if err != nil {
		return fmt.Errorf("revoke refresh token: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrCognitoUnauthorized
	}
	return nil
}
