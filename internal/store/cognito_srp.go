package store

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Cognito USER_SRP_AUTH uses the RFC 5054 3072-bit group and Cognito-specific
// password hashing / HKDF ("Caldera Derived Key"), matching amazon-cognito-identity-js.

const (
	cognitoSRPNHex = "FFFFFFFFFFFFFFFFC90FDAA22168C234C4C6628B80DC1CD1" +
		"29024E088A67CC74020BBEA63B139B22514A08798E3404DD" +
		"EF9519B3CD3A431B302B0A6DF25F14374FE1356D6D51C245" +
		"E485B576625E7EC6F44C42E9A637ED6B0BFF5CB6F406B7ED" +
		"EE386BFB5A899FA5AE9F24117C4B1FE649286651ECE45B3D" +
		"C2007CB8A163BF0598DA48361C55D39A69163FA8FD24CF5F" +
		"83655D23DCA3AD961C62F356208552BB9ED529077096966D" +
		"670C354E4ABC9804F1746C08CA18217C32905E462E36CE3B" +
		"E39E772C180E86039B2783A2EC07A28FB5C55DF06F4C52C9" +
		"DE2BCBF6955817183995497CEA956AE515D2261898FA0510" +
		"15728E5A8AAAC42DAD33170D04507A33A85521ABDF1CBA64" +
		"ECFB850458DBEF0A8AEA71575D060C7DB3970F85A6E1E4C7" +
		"ABF5AE8CDB0933D71E8C94E04A25619DCEE3D2261AD2EE6B" +
		"F12FFA06D98A0864D87602733EC86A64521F2B18177B200C" +
		"BBE117577A615D6C770988C0BAD946E208E24FA074E5AB31" +
		"43DB5BFCE0FD108E4B82D120A93AD2CAFFFFFFFFFFFFFFFF"
	cognitoSRPGHex        = "2"
	cognitoSRPSessionTTL  = 5 * time.Minute
	cognitoSRPSessionKind = "PASSWORD_VERIFIER"
	cognitoSRPInfoBits    = "Caldera Derived Key"
	cognitoSRPSaltBytes   = 16
	cognitoSRPSecretBytes = 64
)

var (
	cognitoSRPN *big.Int
	cognitoSRPG *big.Int
	cognitoSRPK *big.Int
)

func init() {
	cognitoSRPN = mustHexBig(cognitoSRPNHex)
	cognitoSRPG = mustHexBig(cognitoSRPGHex)
	// k = H(pad(N) || pad(g)) with Cognito's fixed pad form ("00"+N+"0"+g).
	cognitoSRPK = mustHexBig(cognitoHexHash("00" + cognitoSRPNHex + "0" + cognitoSRPGHex))
}

const cognitoSRPSchema = `
CREATE TABLE IF NOT EXISTS cognito_srp_sessions (
  session_id TEXT PRIMARY KEY,
  account_id TEXT NOT NULL,
  pool_id TEXT NOT NULL,
  client_id TEXT NOT NULL,
  username TEXT NOT NULL,
  secret_block TEXT NOT NULL,
  srp_a TEXT NOT NULL,
  srp_b TEXT NOT NULL,
  sealed_b BLOB NOT NULL,
  expires_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_cognito_srp_sessions_client
  ON cognito_srp_sessions(account_id, pool_id, client_id);
`

// EnsureCognitoSRPSchema adds SRP verifier columns and challenge sessions.
func EnsureCognitoSRPSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure cognito srp schema: db is nil")
	}
	if _, err := db.Exec(cognitoSRPSchema); err != nil {
		return fmt.Errorf("ensure cognito srp schema: %w", err)
	}
	for _, stmt := range []string{
		`ALTER TABLE cognito_users ADD COLUMN srp_salt TEXT`,
		`ALTER TABLE cognito_users ADD COLUMN srp_verifier TEXT`,
	} {
		if _, err := db.Exec(stmt); err != nil && !isDuplicateColumnErr(err) {
			return fmt.Errorf("ensure cognito srp schema: migrate: %w", err)
		}
	}
	return nil
}

func (s *Store) EnsureCognitoSRPSchema() error {
	return EnsureCognitoSRPSchema(s.db)
}

func mustHexBig(h string) *big.Int {
	n, ok := new(big.Int).SetString(h, 16)
	if !ok {
		panic("invalid cognito SRP constant")
	}
	return n
}

func cognitoPoolName(poolID string) string {
	poolID = strings.TrimSpace(poolID)
	if i := strings.Index(poolID, "_"); i >= 0 && i+1 < len(poolID) {
		return poolID[i+1:]
	}
	return poolID
}

func cognitoHashSHA256(buf []byte) string {
	sum := sha256.Sum256(buf)
	h := hex.EncodeToString(sum[:])
	if len(h) >= 64 {
		return h
	}
	return strings.Repeat("0", 64-len(h)) + h
}

func cognitoHexHash(hexString string) string {
	b, err := hex.DecodeString(hexString)
	if err != nil {
		return cognitoHashSHA256(nil)
	}
	return cognitoHashSHA256(b)
}

func cognitoPadHex(v any) string {
	var h string
	switch t := v.(type) {
	case string:
		h = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(t)), "0x")
	case *big.Int:
		h = strings.ToLower(t.Text(16))
	default:
		return ""
	}
	if len(h)%2 == 1 {
		h = "0" + h
	} else if len(h) > 0 && strings.ContainsRune("89abcdef", rune(h[0])) {
		h = "00" + h
	}
	return h
}

func cognitoComputeHKDF(ikm, salt []byte) []byte {
	mac := hmac.New(sha256.New, salt)
	_, _ = mac.Write(ikm)
	prk := mac.Sum(nil)
	info := append([]byte(cognitoSRPInfoBits), 1)
	mac2 := hmac.New(sha256.New, prk)
	_, _ = mac2.Write(info)
	return mac2.Sum(nil)[:16]
}

func cognitoCalculateU(a, b *big.Int) *big.Int {
	uHex := cognitoHexHash(cognitoPadHex(a) + cognitoPadHex(b))
	return mustHexBig(uHex)
}

func cognitoSRPPrivateX(poolID, username, password, saltHex string) *big.Int {
	userPass := cognitoPoolName(poolID) + username + ":" + password
	inner := cognitoHashSHA256([]byte(userPass))
	return mustHexBig(cognitoHexHash(cognitoPadHex(saltHex) + inner))
}

func cognitoGenerateSaltHex() (string, error) {
	b := make([]byte, cognitoSRPSaltBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("srp salt: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func cognitoComputeVerifier(poolID, username, password, saltHex string) string {
	x := cognitoSRPPrivateX(poolID, username, password, saltHex)
	v := new(big.Int).Exp(cognitoSRPG, x, cognitoSRPN)
	return strings.ToLower(v.Text(16))
}

func (s *Store) storeUserSRPVerifier(accountID, poolID, username, password string) error {
	salt, err := cognitoGenerateSaltHex()
	if err != nil {
		return err
	}
	verifier := cognitoComputeVerifier(poolID, username, password, salt)
	_, err = s.db.Exec(
		`UPDATE cognito_users SET srp_salt = ?, srp_verifier = ?
		 WHERE account_id = ? AND pool_id = ? AND username = ?`,
		salt, verifier, accountID, poolID, username,
	)
	if err != nil {
		return fmt.Errorf("store srp verifier: %w", err)
	}
	return nil
}

func (s *Store) getUserSRP(accountID, poolID, username string) (sub, status, saltHex, verifierHex string, err error) {
	err = s.db.QueryRow(
		`SELECT sub, user_status, COALESCE(srp_salt, ''), COALESCE(srp_verifier, '')
		 FROM cognito_users WHERE account_id = ? AND pool_id = ? AND username = ?`,
		accountID, poolID, username,
	).Scan(&sub, &status, &saltHex, &verifierHex)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", "", "", ErrCognitoUserNotFound
	}
	if err != nil {
		return "", "", "", "", fmt.Errorf("get user srp: %w", err)
	}
	return sub, status, saltHex, verifierHex, nil
}

// CognitoSRPClient is a lab SRP client helper for tests and SDK round-trips.
type CognitoSRPClient struct {
	PoolID   string
	Username string
	Password string
	smallA   *big.Int
	LargeA   *big.Int
}

// NewCognitoSRPClient builds a client ephemeral A for USER_SRP_AUTH.
func NewCognitoSRPClient(poolID, username, password string) (*CognitoSRPClient, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" || strings.TrimSpace(poolID) == "" {
		return nil, fmt.Errorf("%w: pool, username, and password required", ErrCognitoBadRequest)
	}
	a, err := rand.Int(rand.Reader, cognitoSRPN)
	if err != nil {
		return nil, fmt.Errorf("srp client a: %w", err)
	}
	if a.Sign() == 0 {
		a = big.NewInt(1)
	}
	A := new(big.Int).Exp(cognitoSRPG, a, cognitoSRPN)
	if new(big.Int).Mod(A, cognitoSRPN).Sign() == 0 {
		return nil, fmt.Errorf("%w: invalid SRP_A", ErrCognitoBadRequest)
	}
	return &CognitoSRPClient{
		PoolID:   strings.TrimSpace(poolID),
		Username: username,
		Password: password,
		smallA:   a,
		LargeA:   A,
	}, nil
}

// SRPAHex returns the hex SRP_A auth parameter.
func (c *CognitoSRPClient) SRPAHex() string {
	return strings.ToLower(c.LargeA.Text(16))
}

// CognitoSRPTimestamp formats time like Cognito clients (no leading zero on day).
func CognitoSRPTimestamp(t time.Time) string {
	t = t.UTC()
	// Mon Jan 2 15:04:05 UTC 2006 — strip leading zero on day via %e-style.
	s := t.Format("Mon Jan 02 15:04:05 UTC 2006")
	// "Jan 02" -> "Jan 2" when day < 10
	if len(s) >= 10 && s[8] == '0' {
		s = s[:8] + s[9:]
	}
	return s
}

// PasswordVerifierChallengeResponses builds RespondToAuthChallenge inputs.
func (c *CognitoSRPClient) PasswordVerifierChallengeResponses(params map[string]string, at time.Time) (map[string]string, error) {
	userID := strings.TrimSpace(params["USER_ID_FOR_SRP"])
	if userID == "" {
		userID = c.Username
	}
	saltHex := strings.TrimSpace(params["SALT"])
	srpBHex := strings.TrimSpace(params["SRP_B"])
	secretBlock := strings.TrimSpace(params["SECRET_BLOCK"])
	if saltHex == "" || srpBHex == "" || secretBlock == "" {
		return nil, fmt.Errorf("%w: incomplete PASSWORD_VERIFIER challenge", ErrCognitoBadRequest)
	}
	B, ok := new(big.Int).SetString(srpBHex, 16)
	if !ok || B.Sign() == 0 {
		return nil, fmt.Errorf("%w: invalid SRP_B", ErrCognitoBadRequest)
	}
	hkdf, err := c.passwordAuthenticationKey(userID, B, saltHex)
	if err != nil {
		return nil, err
	}
	secretBytes, err := base64.StdEncoding.DecodeString(secretBlock)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid SECRET_BLOCK", ErrCognitoBadRequest)
	}
	ts := CognitoSRPTimestamp(at)
	msg := append([]byte(cognitoPoolName(c.PoolID)), []byte(userID)...)
	msg = append(msg, secretBytes...)
	msg = append(msg, []byte(ts)...)
	mac := hmac.New(sha256.New, hkdf)
	_, _ = mac.Write(msg)
	sig := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return map[string]string{
		"USERNAME":                    userID,
		"PASSWORD_CLAIM_SECRET_BLOCK": secretBlock,
		"PASSWORD_CLAIM_SIGNATURE":    sig,
		"TIMESTAMP":                   ts,
	}, nil
}

func (c *CognitoSRPClient) passwordAuthenticationKey(username string, B *big.Int, saltHex string) ([]byte, error) {
	u := cognitoCalculateU(c.LargeA, B)
	if u.Sign() == 0 {
		return nil, fmt.Errorf("%w: U cannot be zero", ErrCognitoUnauthorized)
	}
	x := cognitoSRPPrivateX(c.PoolID, username, c.Password, saltHex)
	gx := new(big.Int).Exp(cognitoSRPG, x, cognitoSRPN)
	kv := new(big.Int).Mul(cognitoSRPK, gx)
	kv.Mod(kv, cognitoSRPN)
	base := new(big.Int).Sub(B, kv)
	base.Mod(base, cognitoSRPN)
	exp := new(big.Int).Add(c.smallA, new(big.Int).Mul(u, x))
	S := new(big.Int).Exp(base, exp, cognitoSRPN)
	ikm, err := hex.DecodeString(cognitoPadHex(S))
	if err != nil {
		return nil, fmt.Errorf("srp ikm: %w", err)
	}
	salt, err := hex.DecodeString(cognitoPadHex(u))
	if err != nil {
		return nil, fmt.Errorf("srp salt: %w", err)
	}
	return cognitoComputeHKDF(ikm, salt), nil
}

// InitiateCognitoSRPAuth starts USER_SRP_AUTH and returns PASSWORD_VERIFIER challenge.
func (s *Store) InitiateCognitoSRPAuth(clientID, username, srpAHex string) (CognitoAuthOutcome, error) {
	accountID, poolID, err := s.lookupClient(clientID)
	if err != nil {
		return CognitoAuthOutcome{}, err
	}
	return s.beginCognitoSRP(accountID, poolID, clientID, username, srpAHex)
}

func (s *Store) beginCognitoSRP(accountID, poolID, clientID, username, srpAHex string) (CognitoAuthOutcome, error) {
	username = strings.TrimSpace(username)
	srpAHex = strings.TrimSpace(srpAHex)
	if username == "" || srpAHex == "" {
		return CognitoAuthOutcome{}, fmt.Errorf("%w: USERNAME and SRP_A required", ErrCognitoBadRequest)
	}
	A, ok := new(big.Int).SetString(srpAHex, 16)
	if !ok || A.Sign() == 0 || new(big.Int).Mod(A, cognitoSRPN).Sign() == 0 {
		return CognitoAuthOutcome{}, fmt.Errorf("%w: invalid SRP_A", ErrCognitoBadRequest)
	}
	sub, status, saltHex, verifierHex, err := s.getUserSRP(accountID, poolID, username)
	if err != nil {
		if errors.Is(err, ErrCognitoUserNotFound) {
			return CognitoAuthOutcome{}, ErrCognitoUnauthorized
		}
		return CognitoAuthOutcome{}, err
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
	if saltHex == "" || verifierHex == "" {
		return CognitoAuthOutcome{}, fmt.Errorf("%w: USER_SRP_AUTH unavailable for user", ErrCognitoUnauthorized)
	}
	v, ok := new(big.Int).SetString(verifierHex, 16)
	if !ok || v.Sign() == 0 {
		return CognitoAuthOutcome{}, ErrCognitoUnauthorized
	}
	b, err := rand.Int(rand.Reader, cognitoSRPN)
	if err != nil {
		return CognitoAuthOutcome{}, fmt.Errorf("srp b: %w", err)
	}
	if b.Sign() == 0 {
		b = big.NewInt(1)
	}
	gb := new(big.Int).Exp(cognitoSRPG, b, cognitoSRPN)
	kv := new(big.Int).Mul(cognitoSRPK, v)
	kv.Mod(kv, cognitoSRPN)
	B := new(big.Int).Add(kv, gb)
	B.Mod(B, cognitoSRPN)
	if B.Sign() == 0 {
		return CognitoAuthOutcome{}, fmt.Errorf("srp B generation failed")
	}
	secretRaw := make([]byte, cognitoSRPSecretBytes)
	if _, err := rand.Read(secretRaw); err != nil {
		return CognitoAuthOutcome{}, fmt.Errorf("srp secret block: %w", err)
	}
	secretBlock := base64.StdEncoding.EncodeToString(secretRaw)
	sealedB, err := s.SealWithMaster([]byte(b.Text(16)))
	if err != nil {
		return CognitoAuthOutcome{}, fmt.Errorf("seal srp b: %w", err)
	}
	sessionID := uuid.NewString() + uuid.NewString()
	expires := time.Now().UTC().Add(cognitoSRPSessionTTL).Unix()
	_, err = s.db.Exec(
		`INSERT INTO cognito_srp_sessions
		 (session_id, account_id, pool_id, client_id, username, secret_block, srp_a, srp_b, sealed_b, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sessionID, accountID, poolID, clientID, username, secretBlock,
		strings.ToLower(A.Text(16)), strings.ToLower(B.Text(16)), sealedB, expires,
	)
	if err != nil {
		return CognitoAuthOutcome{}, fmt.Errorf("store srp session: %w", err)
	}
	return CognitoAuthOutcome{
		ChallengeName: cognitoSRPSessionKind,
		Session:       sessionID,
		ChallengeParameters: map[string]string{
			"SALT":            saltHex,
			"SECRET_BLOCK":    secretBlock,
			"SRP_B":           strings.ToLower(B.Text(16)),
			"USERNAME":        username,
			"USER_ID_FOR_SRP": username,
		},
	}, nil
}

// RespondToCognitoPASSWORDVerifierChallenge completes USER_SRP_AUTH.
func (s *Store) RespondToCognitoPASSWORDVerifierChallenge(
	clientID, session string, responses map[string]string,
) (CognitoAuthOutcome, error) {
	clientID = strings.TrimSpace(clientID)
	session = strings.TrimSpace(session)
	if clientID == "" || session == "" {
		return CognitoAuthOutcome{}, fmt.Errorf("%w: ClientId and Session required", ErrCognitoBadRequest)
	}
	username := strings.TrimSpace(responses["USERNAME"])
	secretBlock := strings.TrimSpace(responses["PASSWORD_CLAIM_SECRET_BLOCK"])
	signature := strings.TrimSpace(responses["PASSWORD_CLAIM_SIGNATURE"])
	timestamp := strings.TrimSpace(responses["TIMESTAMP"])
	if username == "" || secretBlock == "" || signature == "" || timestamp == "" {
		return CognitoAuthOutcome{}, fmt.Errorf("%w: PASSWORD_VERIFIER responses incomplete", ErrCognitoBadRequest)
	}

	var acct, poolID, sessUser, sessSecret, srpAHex, srpBHex string
	var sealedB []byte
	var expiresAt int64
	err := s.db.QueryRow(
		`SELECT account_id, pool_id, username, secret_block, srp_a, srp_b, sealed_b, expires_at
		 FROM cognito_srp_sessions WHERE session_id = ? AND client_id = ?`,
		session, clientID,
	).Scan(&acct, &poolID, &sessUser, &sessSecret, &srpAHex, &srpBHex, &sealedB, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return CognitoAuthOutcome{}, ErrCognitoUnauthorized
	}
	if err != nil {
		return CognitoAuthOutcome{}, fmt.Errorf("lookup srp session: %w", err)
	}
	if time.Now().UTC().Unix() > expiresAt {
		_, _ = s.db.Exec(`DELETE FROM cognito_srp_sessions WHERE session_id = ?`, session)
		return CognitoAuthOutcome{}, ErrCognitoUnauthorized
	}
	if !strings.EqualFold(sessUser, username) || sessSecret != secretBlock {
		return CognitoAuthOutcome{}, ErrCognitoUnauthorized
	}
	bHex, err := s.UnsealWithMaster(sealedB)
	if err != nil {
		return CognitoAuthOutcome{}, fmt.Errorf("unseal srp b: %w", err)
	}
	b, ok := new(big.Int).SetString(string(bHex), 16)
	if !ok {
		return CognitoAuthOutcome{}, ErrCognitoUnauthorized
	}
	A, ok := new(big.Int).SetString(srpAHex, 16)
	if !ok {
		return CognitoAuthOutcome{}, ErrCognitoUnauthorized
	}
	B, ok := new(big.Int).SetString(srpBHex, 16)
	if !ok {
		return CognitoAuthOutcome{}, ErrCognitoUnauthorized
	}
	sub, status, _, verifierHex, err := s.getUserSRP(acct, poolID, username)
	if err != nil {
		if errors.Is(err, ErrCognitoUserNotFound) {
			return CognitoAuthOutcome{}, ErrCognitoUnauthorized
		}
		return CognitoAuthOutcome{}, err
	}
	if status != "CONFIRMED" {
		return CognitoAuthOutcome{}, fmt.Errorf("%w: User is not confirmed", ErrCognitoUnauthorized)
	}
	v, ok := new(big.Int).SetString(verifierHex, 16)
	if !ok {
		return CognitoAuthOutcome{}, ErrCognitoUnauthorized
	}
	u := cognitoCalculateU(A, B)
	if u.Sign() == 0 {
		return CognitoAuthOutcome{}, ErrCognitoUnauthorized
	}
	// S = (A * v^u)^b mod N
	vu := new(big.Int).Exp(v, u, cognitoSRPN)
	base := new(big.Int).Mul(A, vu)
	base.Mod(base, cognitoSRPN)
	S := new(big.Int).Exp(base, b, cognitoSRPN)
	ikm, err := hex.DecodeString(cognitoPadHex(S))
	if err != nil {
		return CognitoAuthOutcome{}, ErrCognitoUnauthorized
	}
	salt, err := hex.DecodeString(cognitoPadHex(u))
	if err != nil {
		return CognitoAuthOutcome{}, ErrCognitoUnauthorized
	}
	hkdf := cognitoComputeHKDF(ikm, salt)
	secretBytes, err := base64.StdEncoding.DecodeString(secretBlock)
	if err != nil {
		return CognitoAuthOutcome{}, ErrCognitoUnauthorized
	}
	msg := append([]byte(cognitoPoolName(poolID)), []byte(username)...)
	msg = append(msg, secretBytes...)
	msg = append(msg, []byte(timestamp)...)
	mac := hmac.New(sha256.New, hkdf)
	_, _ = mac.Write(msg)
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return CognitoAuthOutcome{}, ErrCognitoUnauthorized
	}
	_, _ = s.db.Exec(`DELETE FROM cognito_srp_sessions WHERE session_id = ?`, session)

	mfaOn, err := s.userMFAEnabled(acct, poolID, username)
	if err != nil {
		return CognitoAuthOutcome{}, err
	}
	if mfaOn {
		return s.createSOFTWARETokenMFAChallenge(acct, poolID, clientID, username)
	}
	result, err := s.issueTokensAfterAuth(acct, poolID, clientID, username, sub, "CONFIRMED", true)
	if err != nil {
		return CognitoAuthOutcome{}, err
	}
	return CognitoAuthOutcome{CognitoAuthResult: result}, nil
}
