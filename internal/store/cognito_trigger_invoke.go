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
	CognitoTriggerPreSignUp          CognitoTriggerName = "PreSignUp"
	CognitoTriggerPostConfirmation   CognitoTriggerName = "PostConfirmation"
	CognitoTriggerPreAuthentication  CognitoTriggerName = "PreAuthentication"
	CognitoTriggerPostAuthentication CognitoTriggerName = "PostAuthentication"
	CognitoTriggerPreTokenGeneration CognitoTriggerName = "PreTokenGeneration"
)

// CognitoTriggerInvoker synchronously Invokes a Cognito Lambda trigger by ARN.
// A non-nil error fails the Cognito API closed.
type CognitoTriggerInvoker func(functionARN, eventJSON string) error

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
	event := map[string]any{
		"version":       "1",
		"triggerSource": strings.TrimSpace(in.TriggerSource),
		"region":        region,
		"userPoolId":    strings.TrimSpace(in.UserPoolID),
		"userName":      strings.TrimSpace(in.Username),
		"callerContext": map[string]any{
			"awsSdkVersion": "aws-sdk-noctaxris-lab",
			"clientId":      strings.TrimSpace(in.ClientID),
		},
		"request": map[string]any{
			"userAttributes": attrs,
		},
		"response": map[string]any{},
	}
	if strings.HasPrefix(strings.TrimSpace(in.TriggerSource), "TokenGeneration_") {
		req := event["request"].(map[string]any)
		req["groupConfiguration"] = map[string]any{
			"groupsToOverride":   []string{},
			"iamRolesToOverride": []string{},
			"preferredRole":      nil,
		}
		event["response"] = map[string]any{
			"claimsOverrideDetails": map[string]any{},
		}
	}
	b, err := json.Marshal(event)
	if err != nil {
		return "", fmt.Errorf("marshal cognito trigger event: %w", err)
	}
	return string(b), nil
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
// No-op when the slot is empty. Fail closed when the invoker is set and returns an error,
// or when an ARN is configured but no invoker is registered (server must wire it).
func (s *Store) FireCognitoTriggerIfConfigured(
	accountID, poolID, clientID, username, userSub, userStatus string,
	name CognitoTriggerName,
	triggerSource string,
) error {
	pool, err := s.DescribeCognitoUserPool(accountID, poolID)
	if err != nil {
		return err
	}
	arn := pool.LambdaConfig.ARNFor(name)
	if arn == "" {
		return nil
	}
	invoker := s.cognitoTriggerInvokerOrNil()
	if invoker == nil {
		// Store-only tests omit the hook; the server always wires a sync Invoke.
		return nil
	}
	eventJSON, err := BuildCognitoTriggerEventJSON(CognitoTriggerEventInput{
		TriggerSource: triggerSource,
		Region:        pool.Region,
		UserPoolID:    pool.PoolID,
		Username:      username,
		ClientID:      clientID,
		UserSub:       userSub,
		UserStatus:    userStatus,
	})
	if err != nil {
		return err
	}
	if err := invoker(arn, eventJSON); err != nil {
		return fmt.Errorf("%w: %v", ErrCognitoTriggerFailed, err)
	}
	return nil
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
