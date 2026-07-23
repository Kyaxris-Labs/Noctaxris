package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

// EnsureCognitoTriggerSchema adds RoleArn + LambdaConfig columns on user pools.
func EnsureCognitoTriggerSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("ensure cognito trigger schema: db is nil")
	}
	for _, stmt := range []string{
		`ALTER TABLE cognito_user_pools ADD COLUMN role_arn TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE cognito_user_pools ADD COLUMN lambda_config_json TEXT NOT NULL DEFAULT '{}'`,
	} {
		if _, err := db.Exec(stmt); err != nil && !isDuplicateColumnErr(err) {
			return fmt.Errorf("ensure cognito trigger schema: migrate: %w", err)
		}
	}
	return nil
}

func (s *Store) EnsureCognitoTriggerSchema() error {
	return EnsureCognitoTriggerSchema(s.db)
}

func parseCognitoLambdaConfig(raw string) CognitoLambdaConfig {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return CognitoLambdaConfig{}
	}
	var cfg CognitoLambdaConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return CognitoLambdaConfig{}
	}
	return cfg
}

func marshalCognitoLambdaConfig(cfg CognitoLambdaConfig) (string, error) {
	b, err := json.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("marshal lambda config: %w", err)
	}
	return string(b), nil
}

// CognitoLambdaConfigFromAPI maps Create/UpdateUserPool LambdaConfig object fields.
func CognitoLambdaConfigFromAPI(raw map[string]any) CognitoLambdaConfig {
	if raw == nil {
		return CognitoLambdaConfig{}
	}
	get := func(k string) string {
		v, _ := raw[k].(string)
		return strings.TrimSpace(v)
	}
	cfg := CognitoLambdaConfig{
		PreSignUp:                   get("PreSignUp"),
		PostConfirmation:            get("PostConfirmation"),
		PreAuthentication:           get("PreAuthentication"),
		PostAuthentication:          get("PostAuthentication"),
		PreTokenGeneration:          get("PreTokenGeneration"),
		UserMigration:               get("UserMigration"),
		DefineAuthChallenge:         get("DefineAuthChallenge"),
		CreateAuthChallenge:         get("CreateAuthChallenge"),
		VerifyAuthChallengeResponse: get("VerifyAuthChallengeResponse"),
		CustomMessage:               get("CustomMessage"),
	}
	if ptg, ok := raw["PreTokenGenerationConfig"].(map[string]any); ok {
		if arn, _ := ptg["LambdaArn"].(string); strings.TrimSpace(arn) != "" {
			cfg.PreTokenGeneration = strings.TrimSpace(arn)
		}
	}
	return cfg
}

// CognitoLambdaConfigToAPI builds DescribeUserPool LambdaConfig (omit empty).
func CognitoLambdaConfigToAPI(cfg CognitoLambdaConfig) map[string]any {
	if !cfg.HasTriggers() {
		return nil
	}
	out := map[string]any{}
	put := func(k, v string) {
		if strings.TrimSpace(v) != "" {
			out[k] = v
		}
	}
	put("PreSignUp", cfg.PreSignUp)
	put("PostConfirmation", cfg.PostConfirmation)
	put("PreAuthentication", cfg.PreAuthentication)
	put("PostAuthentication", cfg.PostAuthentication)
	put("PreTokenGeneration", cfg.PreTokenGeneration)
	put("UserMigration", cfg.UserMigration)
	put("DefineAuthChallenge", cfg.DefineAuthChallenge)
	put("CreateAuthChallenge", cfg.CreateAuthChallenge)
	put("VerifyAuthChallengeResponse", cfg.VerifyAuthChallengeResponse)
	put("CustomMessage", cfg.CustomMessage)
	return out
}

// SetCognitoUserPoolTriggers stores RoleArn + LambdaConfig (UpdateUserPool replace semantics).
func (s *Store) SetCognitoUserPoolTriggers(accountID, poolID, roleARN string, cfg CognitoLambdaConfig) (CognitoUserPool, error) {
	poolID = strings.TrimSpace(poolID)
	roleARN = strings.TrimSpace(roleARN)
	if poolID == "" {
		return CognitoUserPool{}, fmt.Errorf("%w: UserPoolId required", ErrCognitoBadRequest)
	}
	if _, err := s.DescribeCognitoUserPool(accountID, poolID); err != nil {
		return CognitoUserPool{}, err
	}
	raw, err := marshalCognitoLambdaConfig(cfg)
	if err != nil {
		return CognitoUserPool{}, err
	}
	_, err = s.db.Exec(
		`UPDATE cognito_user_pools SET role_arn = ?, lambda_config_json = ?
		 WHERE account_id = ? AND pool_id = ?`,
		roleARN, raw, accountID, poolID,
	)
	if err != nil {
		return CognitoUserPool{}, fmt.Errorf("update user pool triggers: %w", err)
	}
	return s.DescribeCognitoUserPool(accountID, poolID)
}
