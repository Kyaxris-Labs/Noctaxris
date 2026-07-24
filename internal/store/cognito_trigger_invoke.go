package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ErrCognitoTriggerFailed is returned when a configured Lambda trigger is missing or Invoke fails.
// Handlers map this to UnexpectedLambdaException (fail closed).
var ErrCognitoTriggerFailed = errors.New("UnexpectedLambdaException")

// CognitoTriggerName identifies a LambdaConfig trigger slot.
type CognitoTriggerName string

const (
	CognitoTriggerPreSignUp                   CognitoTriggerName = "PreSignUp"
	CognitoTriggerPostConfirmation            CognitoTriggerName = "PostConfirmation"
	CognitoTriggerPreAuthentication           CognitoTriggerName = "PreAuthentication"
	CognitoTriggerPostAuthentication          CognitoTriggerName = "PostAuthentication"
	CognitoTriggerPreTokenGeneration          CognitoTriggerName = "PreTokenGeneration"
	CognitoTriggerCustomMessage               CognitoTriggerName = "CustomMessage"
	CognitoTriggerUserMigration               CognitoTriggerName = "UserMigration"
	CognitoTriggerDefineAuthChallenge         CognitoTriggerName = "DefineAuthChallenge"
	CognitoTriggerCreateAuthChallenge         CognitoTriggerName = "CreateAuthChallenge"
	CognitoTriggerVerifyAuthChallengeResponse CognitoTriggerName = "VerifyAuthChallengeResponse"
)

// CognitoTriggerInvoker synchronously Invokes a Cognito Lambda trigger by ARN.
// Returns the Lambda payload bytes (typically the event or response object).
// A non-nil error fails the Cognito API closed.
type CognitoTriggerInvoker func(functionARN, eventJSON string) (payload []byte, err error)

// CognitoClaimsOverride is the V1 PreTokenGeneration response.claimsOverrideDetails subset
// applied to ID tokens (claimsToAddOrOverride / claimsToSuppress).
type CognitoClaimsOverride struct {
	AddOrOverride map[string]string
	Suppress      []string
}

// reservedIDTokenClaims cannot be overridden or suppressed by PreTokenGeneration.
var reservedIDTokenClaims = map[string]struct{}{
	"iss":       {},
	"aud":       {},
	"client_id": {},
	"exp":       {},
	"iat":       {},
	"auth_time": {},
	"token_use": {},
	"sub":       {},
}

// ARNFor returns the configured function ARN for a trigger slot (empty if unset).
func (c CognitoLambdaConfig) ARNFor(name CognitoTriggerName) string {
	switch name {
	case CognitoTriggerPreSignUp:
		return strings.TrimSpace(c.PreSignUp)
	case CognitoTriggerPostConfirmation:
		return strings.TrimSpace(c.PostConfirmation)
	case CognitoTriggerPreAuthentication:
		return strings.TrimSpace(c.PreAuthentication)
	case CognitoTriggerPostAuthentication:
		return strings.TrimSpace(c.PostAuthentication)
	case CognitoTriggerPreTokenGeneration:
		return strings.TrimSpace(c.PreTokenGeneration)
	case CognitoTriggerCustomMessage:
		return strings.TrimSpace(c.CustomMessage)
	case CognitoTriggerUserMigration:
		return strings.TrimSpace(c.UserMigration)
	case CognitoTriggerDefineAuthChallenge:
		return strings.TrimSpace(c.DefineAuthChallenge)
	case CognitoTriggerCreateAuthChallenge:
		return strings.TrimSpace(c.CreateAuthChallenge)
	case CognitoTriggerVerifyAuthChallengeResponse:
		return strings.TrimSpace(c.VerifyAuthChallengeResponse)
	default:
		return ""
	}
}

// CognitoTriggerEventInput builds an AWS-shaped Cognito User Pools trigger event.
type CognitoTriggerEventInput struct {
	TriggerSource string
	Region        string
	UserPoolID    string
	Username      string
	ClientID      string
	UserSub       string
	UserStatus    string
	// CodeParameter is set for CustomMessage triggers (lab stub "{####}").
	CodeParameter string
	// Password is set for UserMigration_Authentication request.password.
	Password string
	// Custom-auth challenge fields.
	UserNotFound               bool
	ChallengeName              string
	ChallengeSession           []CognitoChallengeResult
	PrivateChallengeParameters map[string]string
	ChallengeAnswer            string
}

// BuildCognitoTriggerEventJSON builds the Lambda event payload for a Cognito trigger.
func BuildCognitoTriggerEventJSON(in CognitoTriggerEventInput) (string, error) {
	region := strings.TrimSpace(in.Region)
	if region == "" {
		region = DefaultCognitoRegion
	}
	attrs := map[string]string{}
	if sub := strings.TrimSpace(in.UserSub); sub != "" {
		attrs["sub"] = sub
	}
	if status := strings.TrimSpace(in.UserStatus); status != "" {
		attrs["cognito:user_status"] = status
	}
	triggerSource := strings.TrimSpace(in.TriggerSource)
	req := map[string]any{
		"userAttributes": attrs,
	}
	resp := map[string]any{}
	if strings.HasPrefix(triggerSource, "TokenGeneration_") {
		req["groupConfiguration"] = map[string]any{
			"groupsToOverride":   []string{},
			"iamRolesToOverride": []string{},
			"preferredRole":      nil,
		}
		resp = map[string]any{
			"claimsOverrideDetails": map[string]any{},
		}
	}
	if strings.HasPrefix(triggerSource, "CustomMessage_") || strings.TrimSpace(in.CodeParameter) != "" {
		code := strings.TrimSpace(in.CodeParameter)
		if code == "" {
			code = "{####}"
		}
		req["codeParameter"] = code
		req["usernameParameter"] = "{username}"
		resp = map[string]any{
			"smsMessage":   nil,
			"emailMessage": nil,
			"emailSubject": nil,
		}
	}
	if strings.HasPrefix(triggerSource, "UserMigration_") {
		req["password"] = in.Password
		resp = map[string]any{
			"userAttributes": map[string]string{},
		}
	}
	if strings.HasPrefix(triggerSource, "DefineAuthChallenge_") {
		session := in.ChallengeSession
		if session == nil {
			session = []CognitoChallengeResult{}
		}
		req["session"] = session
		req["userNotFound"] = in.UserNotFound
		resp = map[string]any{
			"challengeName":      nil,
			"issueTokens":        false,
			"failAuthentication": false,
		}
	}
	if strings.HasPrefix(triggerSource, "CreateAuthChallenge_") {
		session := in.ChallengeSession
		if session == nil {
			session = []CognitoChallengeResult{}
		}
		req["session"] = session
		req["challengeName"] = strings.TrimSpace(in.ChallengeName)
		req["userNotFound"] = in.UserNotFound
		resp = map[string]any{
			"publicChallengeParameters":  map[string]string{},
			"privateChallengeParameters": map[string]string{},
			"challengeMetadata":          nil,
		}
	}
	if strings.HasPrefix(triggerSource, "VerifyAuthChallengeResponse_") {
		priv := in.PrivateChallengeParameters
		if priv == nil {
			priv = map[string]string{}
		}
		req["privateChallengeParameters"] = priv
		req["challengeAnswer"] = in.ChallengeAnswer
		req["userNotFound"] = in.UserNotFound
		resp = map[string]any{
			"answerCorrect": false,
		}
	}
	event := map[string]any{
		"version":       "1",
		"triggerSource": triggerSource,
		"region":        region,
		"userPoolId":    strings.TrimSpace(in.UserPoolID),
		"userName":      strings.TrimSpace(in.Username),
		"callerContext": map[string]any{
			"awsSdkVersion": "aws-sdk-noctaxris-lab",
			"clientId":      strings.TrimSpace(in.ClientID),
		},
		"request":  req,
		"response": resp,
	}
	b, err := json.Marshal(event)
	if err != nil {
		return "", fmt.Errorf("marshal cognito trigger event: %w", err)
	}
	return string(b), nil
}

// ParseCognitoPreTokenClaimsOverride reads V1 claimsOverrideDetails from a Lambda payload.
// Accepts a full trigger event, a response object, or a bare claimsOverrideDetails object.
func ParseCognitoPreTokenClaimsOverride(payload []byte) (CognitoClaimsOverride, error) {
	out := CognitoClaimsOverride{AddOrOverride: map[string]string{}}
	if len(strings.TrimSpace(string(payload))) == 0 {
		return out, nil
	}
	var root map[string]any
	if err := json.Unmarshal(payload, &root); err != nil {
		return CognitoClaimsOverride{}, fmt.Errorf("parse pre token payload: %w", err)
	}
	details := extractClaimsOverrideDetails(root)
	if details == nil {
		return out, nil
	}
	if add, ok := details["claimsToAddOrOverride"].(map[string]any); ok {
		for k, v := range add {
			out.AddOrOverride[k] = fmt.Sprint(v)
		}
	}
	switch suppress := details["claimsToSuppress"].(type) {
	case []any:
		for _, item := range suppress {
			if s, ok := item.(string); ok && s != "" {
				out.Suppress = append(out.Suppress, s)
			}
		}
	case []string:
		out.Suppress = append(out.Suppress, suppress...)
	}
	return out, nil
}

func extractClaimsOverrideDetails(root map[string]any) map[string]any {
	if details, ok := root["claimsOverrideDetails"].(map[string]any); ok {
		return details
	}
	if resp, ok := root["response"].(map[string]any); ok {
		if details, ok := resp["claimsOverrideDetails"].(map[string]any); ok {
			return details
		}
	}
	return nil
}

// ApplyCognitoClaimsOverrideToIDClaims mutates ID token claims for V1 PreTokenGeneration.
// Reserved claims (iss, aud, client_id, exp, iat, auth_time, token_use, sub) are ignored.
func ApplyCognitoClaimsOverrideToIDClaims(claims map[string]any, override CognitoClaimsOverride) {
	for _, key := range override.Suppress {
		if _, reserved := reservedIDTokenClaims[key]; reserved {
			continue
		}
		delete(claims, key)
	}
	for key, val := range override.AddOrOverride {
		if _, reserved := reservedIDTokenClaims[key]; reserved {
			continue
		}
		claims[key] = val
	}
}

// ParseCognitoUserMigrationAttributes reads response.userAttributes from a migration payload.
// Returns ok=false when the map is missing or empty.
func ParseCognitoUserMigrationAttributes(payload []byte) (attrs map[string]string, ok bool, err error) {
	if len(strings.TrimSpace(string(payload))) == 0 {
		return nil, false, nil
	}
	var root map[string]any
	if err := json.Unmarshal(payload, &root); err != nil {
		return nil, false, fmt.Errorf("parse user migration payload: %w", err)
	}
	raw := extractUserMigrationAttributes(root)
	if len(raw) == 0 {
		return nil, false, nil
	}
	attrs = make(map[string]string, len(raw))
	for k, v := range raw {
		attrs[k] = fmt.Sprint(v)
	}
	return attrs, true, nil
}

func extractUserMigrationAttributes(root map[string]any) map[string]any {
	if resp, ok := root["response"].(map[string]any); ok {
		if attrs, ok := resp["userAttributes"].(map[string]any); ok {
			return attrs
		}
	}
	if attrs, ok := root["userAttributes"].(map[string]any); ok {
		return attrs
	}
	return nil
}

// SetCognitoTriggerInvoker registers the sync Invoke hook used for configured triggers.
func (s *Store) SetCognitoTriggerInvoker(fn CognitoTriggerInvoker) {
	s.cognitoTriggerMu.Lock()
	defer s.cognitoTriggerMu.Unlock()
	s.cognitoTriggerInvoker = fn
}

func (s *Store) cognitoTriggerInvokerOrNil() CognitoTriggerInvoker {
	s.cognitoTriggerMu.Lock()
	defer s.cognitoTriggerMu.Unlock()
	return s.cognitoTriggerInvoker
}

// FireCognitoTriggerIfConfigured Invokes the pool LambdaConfig ARN for name when set.
// Returns the Lambda payload (may be nil/empty). No-op when the slot is empty.
// Fail closed when the invoker is set and returns an error, or when an ARN is configured
// but no invoker is registered only in production — store-only tests omit the hook.
func (s *Store) FireCognitoTriggerIfConfigured(
	accountID, poolID, clientID, username, userSub, userStatus string,
	name CognitoTriggerName,
	triggerSource string,
) ([]byte, error) {
	return s.FireCognitoTriggerEvent(accountID, poolID, name, CognitoTriggerEventInput{
		TriggerSource: triggerSource,
		UserPoolID:    poolID,
		Username:      username,
		ClientID:      clientID,
		UserSub:       userSub,
		UserStatus:    userStatus,
	})
}

// FireCognitoTriggerEvent Invokes the configured ARN for name using a full event input.
func (s *Store) FireCognitoTriggerEvent(
	accountID, poolID string,
	name CognitoTriggerName,
	in CognitoTriggerEventInput,
) ([]byte, error) {
	pool, err := s.DescribeCognitoUserPool(accountID, poolID)
	if err != nil {
		return nil, err
	}
	arn := pool.LambdaConfig.ARNFor(name)
	if arn == "" {
		return nil, nil
	}
	invoker := s.cognitoTriggerInvokerOrNil()
	if invoker == nil {
		// Store-only tests omit the hook; the server always wires a sync Invoke.
		return nil, nil
	}
	if strings.TrimSpace(in.Region) == "" {
		in.Region = pool.Region
	}
	if strings.TrimSpace(in.UserPoolID) == "" {
		in.UserPoolID = pool.PoolID
	}
	eventJSON, err := BuildCognitoTriggerEventJSON(in)
	if err != nil {
		return nil, err
	}
	payload, err := invoker(arn, eventJSON)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCognitoTriggerFailed, err)
	}
	return payload, nil
}

// ResolveCognitoClientContext resolves account/pool for a client id (public IdP paths).
func (s *Store) ResolveCognitoClientContext(clientID string) (accountID string, pool CognitoUserPool, err error) {
	accountID, poolID, err := s.lookupClient(clientID)
	if err != nil {
		return "", CognitoUserPool{}, err
	}
	pool, err = s.DescribeCognitoUserPool(accountID, poolID)
	if err != nil {
		return "", CognitoUserPool{}, err
	}
	return accountID, pool, nil
}

// LookupCognitoUserByClient returns user row fields for trigger events.
func (s *Store) LookupCognitoUserByClient(clientID, username string) (accountID, poolID, sub, status string, err error) {
	accountID, poolID, err = s.lookupClient(clientID)
	if err != nil {
		return "", "", "", "", err
	}
	sub, _, status, err = s.getUser(accountID, poolID, username)
	if err != nil {
		return "", "", "", "", err
	}
	return accountID, poolID, sub, status, nil
}
