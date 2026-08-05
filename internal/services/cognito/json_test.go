package cognito_test

import (
	"encoding/json"
	"testing"

	cognitosvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/cognito"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestCognitoJSON(t *testing.T) {
	pool := store.CognitoUserPool{PoolID: "pool-1", Name: "lab-pool", ARN: "arn:aws:cognito-idp:us-east-1:1:userpool/pool-1"}
	if _, err := cognitosvc.CreateUserPoolJSON(pool); err != nil {
		t.Fatal(err)
	}
	if _, err := cognitosvc.DescribeUserPoolJSON(pool); err != nil {
		t.Fatal(err)
	}
	if _, err := cognitosvc.UpdateUserPoolJSON(); err != nil {
		t.Fatal(err)
	}
	if _, err := cognitosvc.ListUserPoolsJSON([]store.CognitoUserPool{pool}); err != nil {
		t.Fatal(err)
	}
	if _, err := cognitosvc.DeleteUserPoolJSON(); err != nil {
		t.Fatal(err)
	}

	client := store.CognitoUserPoolClient{
		ClientID: "client-1", ClientName: "lab-client", PoolID: pool.PoolID,
	}
	if _, err := cognitosvc.CreateUserPoolClientJSON(client); err != nil {
		t.Fatal(err)
	}
	if _, err := cognitosvc.DescribeUserPoolClientJSON(client); err != nil {
		t.Fatal(err)
	}
	if _, err := cognitosvc.ListUserPoolClientsJSON([]store.CognitoUserPoolClient{client}); err != nil {
		t.Fatal(err)
	}
	if _, err := cognitosvc.DeleteUserPoolClientJSON(); err != nil {
		t.Fatal(err)
	}

	user := store.CognitoUser{Username: "alice", PoolID: pool.PoolID, UserStatus: "FORCE_CHANGE_PASSWORD"}
	if _, err := cognitosvc.AdminCreateUserJSON(user); err != nil {
		t.Fatal(err)
	}
	if _, err := cognitosvc.SignUpJSON(user); err != nil {
		t.Fatal(err)
	}
	if _, err := cognitosvc.ConfirmSignUpJSON(); err != nil {
		t.Fatal(err)
	}
	if _, err := cognitosvc.ConfirmForgotPasswordJSON(); err != nil {
		t.Fatal(err)
	}

	details := store.CognitoCodeDeliveryDetails{Destination: "a@example.com", DeliveryMedium: "EMAIL", AttributeName: "email"}
	if _, err := cognitosvc.CodeDeliveryDetailsJSON(details); err != nil {
		t.Fatal(err)
	}
	if _, err := cognitosvc.UpdateUserAttributesJSON([]store.CognitoCodeDeliveryDetails{details}); err != nil {
		t.Fatal(err)
	}
	if _, err := cognitosvc.VerifyUserAttributeJSON(); err != nil {
		t.Fatal(err)
	}

	auth := store.CognitoAuthResult{AccessToken: "at", IDToken: "it", RefreshToken: "rt", ExpiresIn: 3600}
	raw, err := cognitosvc.AuthResultJSON(auth)
	if err != nil {
		t.Fatal(err)
	}
	var ar map[string]any
	if err := json.Unmarshal(raw, &ar); err != nil {
		t.Fatal(err)
	}
	outcome := store.CognitoAuthOutcome{ChallengeName: "SOFTWARE_TOKEN_MFA", Session: "sess", CognitoAuthResult: auth}
	if _, err := cognitosvc.AuthOutcomeJSON(outcome); err != nil {
		t.Fatal(err)
	}
	outcome2 := store.CognitoAuthOutcome{CognitoAuthResult: auth}
	if _, err := cognitosvc.AuthOutcomeJSON(outcome2); err != nil {
		t.Fatal(err)
	}
	if _, err := cognitosvc.AssociateSoftwareTokenJSON("secret", "sess"); err != nil {
		t.Fatal(err)
	}
	if _, err := cognitosvc.VerifySoftwareTokenJSON("SUCCESS"); err != nil {
		t.Fatal(err)
	}
	if _, err := cognitosvc.RevokeTokenJSON(); err != nil {
		t.Fatal(err)
	}
}
