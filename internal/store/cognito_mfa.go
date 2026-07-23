package store

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"database/sql"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/jwtutil"
	"github.com/google/uuid"
)

const (
	cognitoTOTPDigits       = 6
	cognitoTOTPPeriod       = 30
	cognitoTOTPSkewSteps    = 1
	cognitoMFASessionTTL    = 5 * time.Minute
	cognitoSessionKindAssoc = "ASSOCIATE_SOFTWARE_TOKEN"
	cognitoSessionKindMFA   = "SOFTWARE_TOKEN_MFA"
)

var (
	// ErrCognitoCodeMismatch is CodeMismatchException (wrong TOTP).
	ErrCognitoCodeMismatch = errors.New("CodeMismatchException")
)

const cognitoMFASchema = `
CREATE TABLE IF NOT EXISTS cognito_mfa_sessions (
  session_id TEXT PRIMARY KEY,
  account_id TEXT NOT NULL,
  pool_id TEXT NOT NULL,
  client_id TEXT NOT NULL,
  username TEXT NOT NULL,
  kind TEXT NOT NULL,
  sealed_pending_totp BLOB,
  expires_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_cognito_mfa_sessions_user
  ON cognito_mfa_sessions(account_id, pool_id, username);
`

// EnsureCognitoMFASchema adds per-user TOTP columns and MFA session table.
func EnsureCognitoMFASchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure cognito mfa schema: db is nil")
	}
	if _, err := db.Exec(cognitoMFASchema); err != nil {
		return fmt.Errorf("ensure cognito mfa schema: %w", err)
	}
	for _, stmt := range []string{
		`ALTER TABLE cognito_users ADD COLUMN mfa_enabled INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE cognito_users ADD COLUMN sealed_totp_secret BLOB`,
	} {
		if _, err := db.Exec(stmt); err != nil && !isDuplicateColumnErr(err) {
			return fmt.Errorf("ensure cognito mfa schema: migrate: %w", err)
		}
	}
	return nil
}

func (s *Store) EnsureCognitoMFASchema() error {
	return EnsureCognitoMFASchema(s.db)
}

// CognitoAuthOutcome is InitiateAuth / AdminInitiateAuth result: tokens or an MFA challenge.
type CognitoAuthOutcome struct {
	CognitoAuthResult
	ChallengeName       string
	Session             string
	ChallengeParameters map[string]string
}

// GenerateCognitoTOTP returns the RFC 6238 TOTP for secret at t (SHA1, 30s, 6 digits).
// Exported for tests; secrets must not be logged.
func GenerateCognitoTOTP(secretCode string, t time.Time) (string, error) {
	key, err := decodeTOTPSecret(secretCode)
	if err != nil {
		return "", err
	}
	counter := uint64(t.UTC().Unix()) / cognitoTOTPPeriod
	return fmt.Sprintf("%0*d", cognitoTOTPDigits, hotp(key, counter)), nil
}

func decodeTOTPSecret(secretCode string) ([]byte, error) {
	secretCode = strings.TrimSpace(strings.ToUpper(secretCode))
	if secretCode == "" {
		return nil, fmt.Errorf("%w: empty TOTP secret", ErrCognitoBadRequest)
	}
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secretCode)
	if err != nil {
		key, err = base32.StdEncoding.DecodeString(secretCode)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: invalid TOTP secret encoding", ErrCognitoBadRequest)
	}
	if len(key) == 0 {
		return nil, fmt.Errorf("%w: empty TOTP secret key", ErrCognitoBadRequest)
	}
	return key, nil
}

func hotp(key []byte, counter uint64) int {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)
	mac := hmac.New(sha1.New, key)
	_, _ = mac.Write(buf[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	truncated := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	mod := 1
	for i := 0; i < cognitoTOTPDigits; i++ {
		mod *= 10
	}
	return int(truncated % uint32(mod))
}

func validateCognitoTOTP(secretCode, userCode string, now time.Time) bool {
	userCode = strings.TrimSpace(userCode)
	if len(userCode) != cognitoTOTPDigits {
		return false
	}
	key, err := decodeTOTPSecret(secretCode)
	if err != nil {
		return false
	}
	counter := int64(now.UTC().Unix()) / cognitoTOTPPeriod
	for d := -cognitoTOTPSkewSteps; d <= cognitoTOTPSkewSteps; d++ {
		c := uint64(counter + int64(d))
		if fmt.Sprintf("%0*d", cognitoTOTPDigits, hotp(key, c)) == userCode {
			return true
		}
	}
	return false
}

func newCognitoMFASessionID() string {
	return uuid.NewString() + uuid.NewString()
}

func newTOTPSecretCode() (string, error) {
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate totp secret: %w", err)
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw), nil
}

func (s *Store) userMFAEnabled(accountID, poolID, username string) (bool, error) {
	var flag int
	err := s.db.QueryRow(
		`SELECT mfa_enabled FROM cognito_users WHERE account_id = ? AND pool_id = ? AND username = ?`,
		accountID, poolID, username,
	).Scan(&flag)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrCognitoUserNotFound
	}
	if err != nil {
		return false, fmt.Errorf("user mfa enabled: %w", err)
	}
	return flag == 1, nil
}

func (s *Store) loadUserTOTPSecret(accountID, poolID, username string) (string, error) {
	var sealed []byte
	var enabled int
	err := s.db.QueryRow(
		`SELECT mfa_enabled, sealed_totp_secret FROM cognito_users
		 WHERE account_id = ? AND pool_id = ? AND username = ?`,
		accountID, poolID, username,
	).Scan(&enabled, &sealed)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrCognitoUserNotFound
	}
	if err != nil {
		return "", fmt.Errorf("load totp secret: %w", err)
	}
	if enabled != 1 || len(sealed) == 0 {
		return "", ErrCognitoUnauthorized
	}
	plain, err := s.UnsealWithMaster(sealed)
	if err != nil {
		return "", fmt.Errorf("unseal totp secret: %w", err)
	}
	return string(plain), nil
}

func (s *Store) resolveAccessTokenIdentity(accessToken string) (accountID, poolID, clientID, username string, err error) {
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return "", "", "", "", fmt.Errorf("%w: AccessToken or Session required", ErrCognitoBadRequest)
	}
	// Peek issuer from unverified payload only after JWKS verify succeeds for that pool.
	parts := strings.Split(accessToken, ".")
	if len(parts) != 3 {
		return "", "", "", "", ErrCognitoUnauthorized
	}
	// Brute-force pool lookup via claim iss after verify: decode mid-payload is avoided;
	// verify against each known pool is expensive. Parse iss from payload after base64 decode
	// of the middle segment without trusting signature until JWKS verify.
	payloadJSON, decErr := jwtPayloadBytes(accessToken)
	if decErr != nil {
		return "", "", "", "", ErrCognitoUnauthorized
	}
	var peek struct {
		Iss string `json:"iss"`
	}
	if err := json.Unmarshal(payloadJSON, &peek); err != nil || peek.Iss == "" {
		return "", "", "", "", ErrCognitoUnauthorized
	}
	poolID = poolIDFromIssuer(peek.Iss)
	if poolID == "" {
		return "", "", "", "", ErrCognitoUnauthorized
	}
	jwks, err := s.CognitoJWKSJSON(poolID)
	if err != nil {
		return "", "", "", "", ErrCognitoUnauthorized
	}
	claims, err := jwtutil.VerifyCompactRS256(accessToken, jwks)
	if err != nil {
		return "", "", "", "", ErrCognitoUnauthorized
	}
	if jwtutil.ClaimString(claims, "token_use") != "access" {
		return "", "", "", "", ErrCognitoUnauthorized
	}
	if jwtutil.ClaimExpired(claims, time.Now().UTC()) {
		return "", "", "", "", ErrCognitoUnauthorized
	}
	username = jwtutil.ClaimString(claims, "username")
	clientID = jwtutil.ClaimString(claims, "client_id")
	if username == "" || clientID == "" {
		return "", "", "", "", ErrCognitoUnauthorized
	}
	accountID, _, err = s.GetCognitoUserPoolByID(poolID)
	if err != nil {
		return "", "", "", "", ErrCognitoUnauthorized
	}
	return accountID, poolID, clientID, username, nil
}

func poolIDFromIssuer(iss string) string {
	// http://127.0.0.1:4566/cognito-idp/<region>/<poolId>
	const marker = "/cognito-idp/"
	i := strings.Index(iss, marker)
	if i < 0 {
		return ""
	}
	rest := iss[i+len(marker):]
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
}

// AssociateSoftwareTokenMFA begins TOTP setup. Pass access token in sessionOrAccess
// (or a prior associate session). accountID/poolID/username may be empty when using an access token.
func (s *Store) AssociateSoftwareTokenMFA(accountID, poolID, username, sessionOrAccess string) (secretCode, session string, err error) {
	sessionOrAccess = strings.TrimSpace(sessionOrAccess)
	if sessionOrAccess == "" {
		return "", "", fmt.Errorf("%w: AccessToken or Session required", ErrCognitoBadRequest)
	}

	var clientID string
	if strings.Count(sessionOrAccess, ".") == 2 {
		accountID, poolID, clientID, username, err = s.resolveAccessTokenIdentity(sessionOrAccess)
		if err != nil {
			return "", "", err
		}
	} else {
		row, loadErr := s.loadMFASession(sessionOrAccess)
		if loadErr != nil {
			return "", "", loadErr
		}
		if row.Kind != cognitoSessionKindAssoc && row.Kind != cognitoSessionKindMFA {
			return "", "", ErrCognitoUnauthorized
		}
		accountID, poolID, clientID, username = row.AccountID, row.PoolID, row.ClientID, row.Username
		_, _ = s.db.Exec(`DELETE FROM cognito_mfa_sessions WHERE session_id = ?`, sessionOrAccess)
	}

	if accountID == "" || poolID == "" || username == "" {
		return "", "", fmt.Errorf("%w: user identity required", ErrCognitoBadRequest)
	}
	if _, _, status, getErr := s.getUser(accountID, poolID, username); getErr != nil {
		if errors.Is(getErr, ErrCognitoUserNotFound) {
			return "", "", ErrCognitoUnauthorized
		}
		return "", "", getErr
	} else if status != "CONFIRMED" {
		return "", "", fmt.Errorf("%w: User is not confirmed", ErrCognitoUnauthorized)
	}
	if clientID == "" {
		// Lab associate via MFA session may lack client; pick any client for the pool.
		clients, listErr := s.ListCognitoUserPoolClients(accountID, poolID)
		if listErr != nil || len(clients) == 0 {
			return "", "", fmt.Errorf("%w: ClientId required", ErrCognitoBadRequest)
		}
		clientID = clients[0].ClientID
	}

	secretCode, err = newTOTPSecretCode()
	if err != nil {
		return "", "", err
	}
	sealed, err := s.SealWithMaster([]byte(secretCode))
	if err != nil {
		return "", "", fmt.Errorf("seal pending totp: %w", err)
	}
	session = newCognitoMFASessionID()
	expires := time.Now().UTC().Add(cognitoMFASessionTTL).Unix()
	_, err = s.db.Exec(
		`INSERT INTO cognito_mfa_sessions
		 (session_id, account_id, pool_id, client_id, username, kind, sealed_pending_totp, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		session, accountID, poolID, clientID, username, cognitoSessionKindAssoc, sealed, expires,
	)
	if err != nil {
		return "", "", fmt.Errorf("store associate session: %w", err)
	}
	return secretCode, session, nil
}

// VerifySoftwareTokenMFA enables TOTP after a valid code; clears the associate session.
// Pass session in the session parameter (preferred). accountID/poolID/username may be empty.
func (s *Store) VerifySoftwareTokenMFA(accountID, poolID, username, session, userCode string) error {
	session = strings.TrimSpace(session)
	userCode = strings.TrimSpace(userCode)
	if session == "" {
		return fmt.Errorf("%w: Session required", ErrCognitoBadRequest)
	}
	if len(userCode) != cognitoTOTPDigits {
		return fmt.Errorf("%w: UserCode must be 6 digits", ErrCognitoBadRequest)
	}
	row, err := s.loadMFASession(session)
	if err != nil {
		return err
	}
	if row.Kind != cognitoSessionKindAssoc {
		return ErrCognitoUnauthorized
	}
	if accountID != "" && accountID != row.AccountID {
		return ErrCognitoUnauthorized
	}
	if poolID != "" && poolID != row.PoolID {
		return ErrCognitoUnauthorized
	}
	if username != "" && username != row.Username {
		return ErrCognitoUnauthorized
	}
	if len(row.SealedPendingTOTP) == 0 {
		return ErrCognitoUnauthorized
	}
	plain, err := s.UnsealWithMaster(row.SealedPendingTOTP)
	if err != nil {
		return fmt.Errorf("unseal pending totp: %w", err)
	}
	secretCode := string(plain)
	if !validateCognitoTOTP(secretCode, userCode, time.Now().UTC()) {
		return ErrCognitoCodeMismatch
	}
	sealed, err := s.SealWithMaster([]byte(secretCode))
	if err != nil {
		return fmt.Errorf("seal totp secret: %w", err)
	}
	res, err := s.db.Exec(
		`UPDATE cognito_users SET mfa_enabled = 1, sealed_totp_secret = ?
		 WHERE account_id = ? AND pool_id = ? AND username = ?`,
		sealed, row.AccountID, row.PoolID, row.Username,
	)
	if err != nil {
		return fmt.Errorf("enable software token mfa: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrCognitoUserNotFound
	}
	_, _ = s.db.Exec(`DELETE FROM cognito_mfa_sessions WHERE session_id = ?`, session)
	return nil
}

// RespondToSOFTWARETokenMFAChallenge validates TOTP and returns CognitoAuthResult (tokens).
func (s *Store) RespondToSOFTWARETokenMFAChallenge(clientID, username, session, totpCode string) (CognitoAuthResult, error) {
	clientID = strings.TrimSpace(clientID)
	username = strings.TrimSpace(username)
	session = strings.TrimSpace(session)
	totpCode = strings.TrimSpace(totpCode)
	if clientID == "" || username == "" || session == "" {
		return CognitoAuthResult{}, fmt.Errorf("%w: ClientId, USERNAME, and Session required", ErrCognitoBadRequest)
	}
	row, err := s.loadMFASession(session)
	if err != nil {
		return CognitoAuthResult{}, err
	}
	if row.Kind != cognitoSessionKindMFA {
		return CognitoAuthResult{}, ErrCognitoUnauthorized
	}
	if row.ClientID != clientID || row.Username != username {
		return CognitoAuthResult{}, ErrCognitoUnauthorized
	}
	secretCode, err := s.loadUserTOTPSecret(row.AccountID, row.PoolID, row.Username)
	if err != nil {
		return CognitoAuthResult{}, err
	}
	if !validateCognitoTOTP(secretCode, totpCode, time.Now().UTC()) {
		return CognitoAuthResult{}, ErrCognitoCodeMismatch
	}
	sub, _, status, err := s.getUser(row.AccountID, row.PoolID, row.Username)
	if err != nil {
		if errors.Is(err, ErrCognitoUserNotFound) {
			return CognitoAuthResult{}, ErrCognitoUnauthorized
		}
		return CognitoAuthResult{}, err
	}
	if status != "CONFIRMED" {
		return CognitoAuthResult{}, fmt.Errorf("%w: User is not confirmed", ErrCognitoUnauthorized)
	}
	_, _ = s.db.Exec(`DELETE FROM cognito_mfa_sessions WHERE session_id = ?`, session)
	return s.issueTokensAfterAuth(row.AccountID, row.PoolID, clientID, username, sub, status, true)
}

type cognitoMFASessionRow struct {
	SessionID         string
	AccountID         string
	PoolID            string
	ClientID          string
	Username          string
	Kind              string
	SealedPendingTOTP []byte
	ExpiresAt         int64
}

func (s *Store) loadMFASession(sessionID string) (cognitoMFASessionRow, error) {
	var row cognitoMFASessionRow
	err := s.db.QueryRow(
		`SELECT session_id, account_id, pool_id, client_id, username, kind, sealed_pending_totp, expires_at
		 FROM cognito_mfa_sessions WHERE session_id = ?`,
		strings.TrimSpace(sessionID),
	).Scan(
		&row.SessionID, &row.AccountID, &row.PoolID, &row.ClientID, &row.Username,
		&row.Kind, &row.SealedPendingTOTP, &row.ExpiresAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return cognitoMFASessionRow{}, ErrCognitoUnauthorized
	}
	if err != nil {
		return cognitoMFASessionRow{}, fmt.Errorf("load mfa session: %w", err)
	}
	if time.Now().UTC().Unix() > row.ExpiresAt {
		_, _ = s.db.Exec(`DELETE FROM cognito_mfa_sessions WHERE session_id = ?`, row.SessionID)
		return cognitoMFASessionRow{}, ErrCognitoUnauthorized
	}
	return row, nil
}

func (s *Store) createSOFTWARETokenMFAChallenge(accountID, poolID, clientID, username string) (CognitoAuthOutcome, error) {
	session := newCognitoMFASessionID()
	expires := time.Now().UTC().Add(cognitoMFASessionTTL).Unix()
	_, err := s.db.Exec(
		`INSERT INTO cognito_mfa_sessions
		 (session_id, account_id, pool_id, client_id, username, kind, sealed_pending_totp, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, NULL, ?)`,
		session, accountID, poolID, clientID, username, cognitoSessionKindMFA, expires,
	)
	if err != nil {
		return CognitoAuthOutcome{}, fmt.Errorf("store mfa challenge session: %w", err)
	}
	return CognitoAuthOutcome{
		ChallengeName: cognitoSessionKindMFA,
		Session:       session,
		ChallengeParameters: map[string]string{
			"USER_ID_FOR_SRP":      username,
			"FRIENDLY_DEVICE_NAME": "SoftwareToken",
		},
	}, nil
}

// jwtPayloadBytes returns the JWT payload segment decoded (no signature check).
func jwtPayloadBytes(token string) ([]byte, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("jwt: segments")
	}
	return base64.RawURLEncoding.DecodeString(parts[1])
}
