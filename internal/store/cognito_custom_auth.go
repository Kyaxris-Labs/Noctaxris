package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	cognitoCustomAuthSessionKind = "CUSTOM_CHALLENGE"
	cognitoCustomAuthSessionTTL  = 3 * time.Minute
)

const cognitoCustomAuthSchema = `
CREATE TABLE IF NOT EXISTS cognito_custom_auth_sessions (
  session_id TEXT PRIMARY KEY,
  account_id TEXT NOT NULL,
  pool_id TEXT NOT NULL,
  client_id TEXT NOT NULL,
  username TEXT NOT NULL,
  session_json TEXT NOT NULL,
  private_params_json TEXT NOT NULL DEFAULT '{}',
  challenge_metadata TEXT NOT NULL DEFAULT '',
  expires_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_cognito_custom_auth_user
  ON cognito_custom_auth_sessions(account_id, pool_id, username);
`

// CognitoChallengeResult is one entry in the custom-auth session array.
type CognitoChallengeResult struct {
	ChallengeName     string `json:"challengeName"`
	ChallengeResult   bool   `json:"challengeResult"`
	ChallengeMetadata string `json:"challengeMetadata,omitempty"`
}

// EnsureCognitoCustomAuthSchema creates custom-auth challenge sessions.
func EnsureCognitoCustomAuthSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure cognito custom auth schema: db is nil")
	}
	if _, err := db.Exec(cognitoCustomAuthSchema); err != nil {
		return fmt.Errorf("ensure cognito custom auth schema: %w", err)
	}
	return nil
}

func (s *Store) EnsureCognitoCustomAuthSchema() error {
	return EnsureCognitoCustomAuthSchema(s.db)
}

type cognitoDefineAuthResponse struct {
	ChallengeName      string
	IssueTokens        bool
	FailAuthentication bool
}

type cognitoCreateAuthResponse struct {
	PublicChallengeParameters  map[string]string
	PrivateChallengeParameters map[string]string
	ChallengeMetadata          string
}

func parseDefineAuthResponse(payload []byte) (cognitoDefineAuthResponse, error) {
	out := cognitoDefineAuthResponse{}
	if len(strings.TrimSpace(string(payload))) == 0 {
		return out, fmt.Errorf("empty define auth response")
	}
	var root map[string]any
	if err := json.Unmarshal(payload, &root); err != nil {
		return out, err
	}
	resp := root
	if nested, ok := root["response"].(map[string]any); ok {
		resp = nested
	}
	if v, ok := resp["challengeName"].(string); ok {
		out.ChallengeName = strings.TrimSpace(v)
	}
	out.IssueTokens = asBool(resp["issueTokens"])
	out.FailAuthentication = asBool(resp["failAuthentication"])
	return out, nil
}

func parseCreateAuthResponse(payload []byte) (cognitoCreateAuthResponse, error) {
	out := cognitoCreateAuthResponse{
		PublicChallengeParameters:  map[string]string{},
		PrivateChallengeParameters: map[string]string{},
	}
	if len(strings.TrimSpace(string(payload))) == 0 {
		return out, nil
	}
	var root map[string]any
	if err := json.Unmarshal(payload, &root); err != nil {
		return out, err
	}
	resp := root
	if nested, ok := root["response"].(map[string]any); ok {
		resp = nested
	}
	out.PublicChallengeParameters = stringMapFromAny(resp["publicChallengeParameters"])
	out.PrivateChallengeParameters = stringMapFromAny(resp["privateChallengeParameters"])
	if v, ok := resp["challengeMetadata"].(string); ok {
		out.ChallengeMetadata = v
	}
	return out, nil
}

func parseVerifyAuthResponse(payload []byte) (bool, error) {
	if len(strings.TrimSpace(string(payload))) == 0 {
		return false, nil
	}
	var root map[string]any
	if err := json.Unmarshal(payload, &root); err != nil {
		return false, err
	}
	resp := root
	if nested, ok := root["response"].(map[string]any); ok {
		resp = nested
	}
	return asBool(resp["answerCorrect"]), nil
}

func asBool(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return strings.EqualFold(t, "true")
	default:
		return false
	}
}

func stringMapFromAny(v any) map[string]string {
	out := map[string]string{}
	m, ok := v.(map[string]any)
	if !ok {
		return out
	}
	for k, raw := range m {
		out[k] = fmt.Sprint(raw)
	}
	return out
}

// InitiateCognitoCustomAuth starts CUSTOM_AUTH (Define → optional Create → tokens or challenge).
func (s *Store) InitiateCognitoCustomAuth(clientID, username string) (CognitoAuthOutcome, error) {
	accountID, poolID, err := s.lookupClient(clientID)
	if err != nil {
		return CognitoAuthOutcome{}, err
	}
	return s.runCustomAuthDefine(accountID, poolID, clientID, username, nil)
}

func (s *Store) runCustomAuthDefine(
	accountID, poolID, clientID, username string,
	session []CognitoChallengeResult,
) (CognitoAuthOutcome, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return CognitoAuthOutcome{}, fmt.Errorf("%w: USERNAME required", ErrCognitoBadRequest)
	}
	if err := s.EnsureCognitoCustomAuthSchema(); err != nil {
		return CognitoAuthOutcome{}, err
	}
	pool, err := s.DescribeCognitoUserPool(accountID, poolID)
	if err != nil {
		return CognitoAuthOutcome{}, err
	}
	if pool.LambdaConfig.ARNFor(CognitoTriggerDefineAuthChallenge) == "" {
		return CognitoAuthOutcome{}, fmt.Errorf("%w: DefineAuthChallenge trigger is required for CUSTOM_AUTH", ErrCognitoBadRequest)
	}

	sub, status := "", ""
	userNotFound := false
	sub, _, status, err = s.getUser(accountID, poolID, username)
	if errors.Is(err, ErrCognitoUserNotFound) {
		userNotFound = true
		err = nil
	}
	if err != nil {
		return CognitoAuthOutcome{}, err
	}
	if !userNotFound && status != "CONFIRMED" {
		return CognitoAuthOutcome{}, fmt.Errorf("%w: User is not confirmed", ErrCognitoUnauthorized)
	}
	if session == nil {
		session = []CognitoChallengeResult{}
	}

	payload, err := s.FireCognitoTriggerEvent(accountID, poolID, CognitoTriggerDefineAuthChallenge, CognitoTriggerEventInput{
		TriggerSource:      "DefineAuthChallenge_Authentication",
		UserPoolID:         poolID,
		Username:           username,
		ClientID:           clientID,
		UserSub:            sub,
		UserStatus:         status,
		UserNotFound:       userNotFound,
		ChallengeSession:   session,
	})
	if err != nil {
		return CognitoAuthOutcome{}, err
	}
	defined, err := parseDefineAuthResponse(payload)
	if err != nil {
		return CognitoAuthOutcome{}, fmt.Errorf("%w: %v", ErrCognitoInvalidLambdaResponse, err)
	}
	if defined.FailAuthentication {
		return CognitoAuthOutcome{}, ErrCognitoUnauthorized
	}
	if defined.IssueTokens {
		if userNotFound {
			return CognitoAuthOutcome{}, ErrCognitoUnauthorized
		}
		result, err := s.issueTokensAfterAuth(accountID, poolID, clientID, username, sub, status, true)
		if err != nil {
			return CognitoAuthOutcome{}, err
		}
		return CognitoAuthOutcome{CognitoAuthResult: result}, nil
	}
	chalName := strings.ToUpper(strings.TrimSpace(defined.ChallengeName))
	if chalName == "" {
		return CognitoAuthOutcome{}, fmt.Errorf("%w: DefineAuthChallenge must set challengeName or issueTokens", ErrCognitoInvalidLambdaResponse)
	}
	if chalName != "CUSTOM_CHALLENGE" {
		return CognitoAuthOutcome{}, fmt.Errorf("%w: lab CUSTOM_AUTH supports CUSTOM_CHALLENGE only (got %s)", ErrCognitoBadRequest, chalName)
	}
	if pool.LambdaConfig.ARNFor(CognitoTriggerCreateAuthChallenge) == "" {
		return CognitoAuthOutcome{}, fmt.Errorf("%w: CreateAuthChallenge trigger is required", ErrCognitoBadRequest)
	}

	createPayload, err := s.FireCognitoTriggerEvent(accountID, poolID, CognitoTriggerCreateAuthChallenge, CognitoTriggerEventInput{
		TriggerSource:    "CreateAuthChallenge_Authentication",
		UserPoolID:       poolID,
		Username:         username,
		ClientID:         clientID,
		UserSub:          sub,
		UserStatus:       status,
		UserNotFound:     userNotFound,
		ChallengeName:    "CUSTOM_CHALLENGE",
		ChallengeSession: session,
	})
	if err != nil {
		return CognitoAuthOutcome{}, err
	}
	created, err := parseCreateAuthResponse(createPayload)
	if err != nil {
		return CognitoAuthOutcome{}, fmt.Errorf("%w: %v", ErrCognitoInvalidLambdaResponse, err)
	}

	sessionJSON, err := json.Marshal(session)
	if err != nil {
		return CognitoAuthOutcome{}, err
	}
	privateJSON, err := json.Marshal(created.PrivateChallengeParameters)
	if err != nil {
		return CognitoAuthOutcome{}, err
	}
	sessionID := uuid.NewString() + uuid.NewString()
	expires := time.Now().UTC().Add(cognitoCustomAuthSessionTTL).Unix()
	_, err = s.db.Exec(
		`INSERT INTO cognito_custom_auth_sessions
		 (session_id, account_id, pool_id, client_id, username, session_json, private_params_json, challenge_metadata, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sessionID, accountID, poolID, clientID, username, string(sessionJSON), string(privateJSON),
		created.ChallengeMetadata, expires,
	)
	if err != nil {
		return CognitoAuthOutcome{}, fmt.Errorf("store custom auth session: %w", err)
	}
	params := map[string]string{"USERNAME": username}
	for k, v := range created.PublicChallengeParameters {
		params[k] = v
	}
	return CognitoAuthOutcome{
		ChallengeName:       cognitoCustomAuthSessionKind,
		Session:             sessionID,
		ChallengeParameters: params,
	}, nil
}

// RespondToCognitoCUSTOMChallenge completes one CUSTOM_CHALLENGE step (Verify → Define).
func (s *Store) RespondToCognitoCUSTOMChallenge(
	clientID, session string, responses map[string]string,
) (CognitoAuthOutcome, error) {
	clientID = strings.TrimSpace(clientID)
	session = strings.TrimSpace(session)
	if clientID == "" || session == "" {
		return CognitoAuthOutcome{}, fmt.Errorf("%w: ClientId and Session required", ErrCognitoBadRequest)
	}
	answer := strings.TrimSpace(responses["ANSWER"])
	username := strings.TrimSpace(responses["USERNAME"])
	if answer == "" {
		return CognitoAuthOutcome{}, fmt.Errorf("%w: ANSWER required", ErrCognitoBadRequest)
	}

	var acct, poolID, sessUser, sessionJSON, privateJSON, metadata string
	var expiresAt int64
	err := s.db.QueryRow(
		`SELECT account_id, pool_id, username, session_json, private_params_json, challenge_metadata, expires_at
		 FROM cognito_custom_auth_sessions WHERE session_id = ? AND client_id = ?`,
		session, clientID,
	).Scan(&acct, &poolID, &sessUser, &sessionJSON, &privateJSON, &metadata, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return CognitoAuthOutcome{}, ErrCognitoUnauthorized
	}
	if err != nil {
		return CognitoAuthOutcome{}, fmt.Errorf("lookup custom auth session: %w", err)
	}
	if time.Now().UTC().Unix() > expiresAt {
		_, _ = s.db.Exec(`DELETE FROM cognito_custom_auth_sessions WHERE session_id = ?`, session)
		return CognitoAuthOutcome{}, ErrCognitoUnauthorized
	}
	if username == "" {
		username = sessUser
	}
	if !strings.EqualFold(sessUser, username) {
		return CognitoAuthOutcome{}, ErrCognitoUnauthorized
	}

	var prior []CognitoChallengeResult
	if err := json.Unmarshal([]byte(sessionJSON), &prior); err != nil {
		return CognitoAuthOutcome{}, fmt.Errorf("%w: corrupt session", ErrCognitoUnauthorized)
	}
	privateParams := map[string]string{}
	_ = json.Unmarshal([]byte(privateJSON), &privateParams)

	pool, err := s.DescribeCognitoUserPool(acct, poolID)
	if err != nil {
		return CognitoAuthOutcome{}, err
	}
	if pool.LambdaConfig.ARNFor(CognitoTriggerVerifyAuthChallengeResponse) == "" {
		return CognitoAuthOutcome{}, fmt.Errorf("%w: VerifyAuthChallengeResponse trigger is required", ErrCognitoBadRequest)
	}

	sub, status := "", ""
	userNotFound := false
	sub, _, status, err = s.getUser(acct, poolID, username)
	if errors.Is(err, ErrCognitoUserNotFound) {
		userNotFound = true
		err = nil
	}
	if err != nil {
		return CognitoAuthOutcome{}, err
	}

	verifyPayload, err := s.FireCognitoTriggerEvent(acct, poolID, CognitoTriggerVerifyAuthChallengeResponse, CognitoTriggerEventInput{
		TriggerSource:              "VerifyAuthChallengeResponse_Authentication",
		UserPoolID:                 poolID,
		Username:                   username,
		ClientID:                   clientID,
		UserSub:                    sub,
		UserStatus:                 status,
		UserNotFound:               userNotFound,
		PrivateChallengeParameters: privateParams,
		ChallengeAnswer:            answer,
	})
	if err != nil {
		return CognitoAuthOutcome{}, err
	}
	answerCorrect, err := parseVerifyAuthResponse(verifyPayload)
	if err != nil {
		return CognitoAuthOutcome{}, fmt.Errorf("%w: %v", ErrCognitoInvalidLambdaResponse, err)
	}

	prior = append(prior, CognitoChallengeResult{
		ChallengeName:     "CUSTOM_CHALLENGE",
		ChallengeResult:   answerCorrect,
		ChallengeMetadata: metadata,
	})
	_, _ = s.db.Exec(`DELETE FROM cognito_custom_auth_sessions WHERE session_id = ?`, session)
	return s.runCustomAuthDefine(acct, poolID, clientID, username, prior)
}
