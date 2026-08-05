package cognito

import (
	"encoding/json"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func userPoolObject(p store.CognitoUserPool) map[string]any {
	up := map[string]any{
		"Id":           p.PoolID,
		"Name":         p.Name,
		"Arn":          p.ARN,
		"CreationDate": float64(p.CreatedAt) / 1000.0,
	}
	if p.RoleArn != "" {
		up["RoleArn"] = p.RoleArn
	}
	if lc := store.CognitoLambdaConfigToAPI(p.LambdaConfig); lc != nil {
		up["LambdaConfig"] = lc
	}
	return up
}

// CreateUserPoolJSON builds CreateUserPool response.
func CreateUserPoolJSON(p store.CognitoUserPool) ([]byte, error) {
	return json.Marshal(map[string]any{"UserPool": userPoolObject(p)})
}

// DescribeUserPoolJSON builds DescribeUserPool response.
func DescribeUserPoolJSON(p store.CognitoUserPool) ([]byte, error) {
	return CreateUserPoolJSON(p)
}

// UpdateUserPoolJSON is an empty OK body (AWS UpdateUserPool returns {}).
func UpdateUserPoolJSON() ([]byte, error) { return []byte(`{}`), nil }

// ListUserPoolsJSON builds ListUserPools response.
func ListUserPoolsJSON(pools []store.CognitoUserPool) ([]byte, error) {
	items := make([]map[string]any, 0, len(pools))
	for _, p := range pools {
		items = append(items, map[string]any{
			"Id":   p.PoolID,
			"Name": p.Name,
		})
	}
	return json.Marshal(map[string]any{"UserPools": items})
}

// DeleteUserPoolJSON is an empty OK body.
func DeleteUserPoolJSON() ([]byte, error) { return []byte(`{}`), nil }

// CreateUserPoolClientJSON builds CreateUserPoolClient response.
func CreateUserPoolClientJSON(c store.CognitoUserPoolClient) ([]byte, error) {
	return json.Marshal(map[string]any{
		"UserPoolClient": map[string]any{
			"ClientId":   c.ClientID,
			"ClientName": c.ClientName,
			"UserPoolId": c.PoolID,
		},
	})
}

// DescribeUserPoolClientJSON builds DescribeUserPoolClient response.
func DescribeUserPoolClientJSON(c store.CognitoUserPoolClient) ([]byte, error) {
	return CreateUserPoolClientJSON(c)
}

// ListUserPoolClientsJSON builds ListUserPoolClients response.
func ListUserPoolClientsJSON(clients []store.CognitoUserPoolClient) ([]byte, error) {
	items := make([]map[string]any, 0, len(clients))
	for _, c := range clients {
		items = append(items, map[string]any{
			"ClientId":   c.ClientID,
			"ClientName": c.ClientName,
			"UserPoolId": c.PoolID,
		})
	}
	return json.Marshal(map[string]any{"UserPoolClients": items})
}

// DeleteUserPoolClientJSON is an empty OK body.
func DeleteUserPoolClientJSON() ([]byte, error) { return []byte(`{}`), nil }

// AdminCreateUserJSON builds AdminCreateUser response.
func AdminCreateUserJSON(u store.CognitoUser) ([]byte, error) {
	return json.Marshal(map[string]any{
		"User": map[string]any{
			"Username":   u.Username,
			"UserStatus": u.UserStatus,
			"Enabled":    u.Enabled,
			"Attributes": []map[string]string{
				{"Name": "sub", "Value": u.Sub},
			},
		},
	})
}

// AdminGetUserJSON builds AdminGetUser response.
func AdminGetUserJSON(u store.CognitoUser) ([]byte, error) {
	return json.Marshal(map[string]any{
		"Username":       u.Username,
		"UserStatus":     u.UserStatus,
		"Enabled":        u.Enabled,
		"UserCreateDate": float64(u.CreatedAt) / 1000.0,
		"UserAttributes": []map[string]string{
			{"Name": "sub", "Value": u.Sub},
		},
	})
}

// ListUsersJSON builds ListUsers response.
func ListUsersJSON(users []store.CognitoUser) ([]byte, error) {
	items := make([]map[string]any, 0, len(users))
	for _, u := range users {
		items = append(items, map[string]any{
			"Username":   u.Username,
			"UserStatus": u.UserStatus,
			"Enabled":    u.Enabled,
			"Attributes": []map[string]string{
				{"Name": "sub", "Value": u.Sub},
			},
		})
	}
	return json.Marshal(map[string]any{"Users": items})
}

// AdminSetUserPasswordJSON is an empty OK body.
func AdminSetUserPasswordJSON() ([]byte, error) { return []byte(`{}`), nil }

// AdminDeleteUserJSON is an empty OK body.
func AdminDeleteUserJSON() ([]byte, error) { return []byte(`{}`), nil }

// AdminDisableUserJSON is an empty OK body.
func AdminDisableUserJSON() ([]byte, error) { return []byte(`{}`), nil }

// SignUpJSON builds SignUp response.
func SignUpJSON(u store.CognitoUser) ([]byte, error) {
	return json.Marshal(map[string]any{
		"UserConfirmed": false,
		"UserSub":       u.Sub,
	})
}

// ConfirmSignUpJSON is an empty OK body.
func ConfirmSignUpJSON() ([]byte, error) { return []byte(`{}`), nil }

// ConfirmForgotPasswordJSON is an empty OK body.
func ConfirmForgotPasswordJSON() ([]byte, error) { return []byte(`{}`), nil }

// CodeDeliveryDetailsJSON builds ForgotPassword / ResendConfirmationCode /
// GetUserAttributeVerificationCode response.
func CodeDeliveryDetailsJSON(d store.CognitoCodeDeliveryDetails) ([]byte, error) {
	return json.Marshal(map[string]any{
		"CodeDeliveryDetails": map[string]any{
			"Destination":    d.Destination,
			"DeliveryMedium": d.DeliveryMedium,
			"AttributeName":  d.AttributeName,
		},
	})
}

// UpdateUserAttributesJSON builds UpdateUserAttributes response.
func UpdateUserAttributesJSON(details []store.CognitoCodeDeliveryDetails) ([]byte, error) {
	list := make([]map[string]any, 0, len(details))
	for _, d := range details {
		list = append(list, map[string]any{
			"Destination":    d.Destination,
			"DeliveryMedium": d.DeliveryMedium,
			"AttributeName":  d.AttributeName,
		})
	}
	return json.Marshal(map[string]any{"CodeDeliveryDetailsList": list})
}

// VerifyUserAttributeJSON is an empty OK body.
func VerifyUserAttributeJSON() ([]byte, error) { return []byte(`{}`), nil }

// AuthResultJSON builds InitiateAuth / AdminInitiateAuth AuthenticationResult.
// Omits RefreshToken when empty (AWS refresh without a rotated refresh token).
func AuthResultJSON(a store.CognitoAuthResult) ([]byte, error) {
	result := map[string]any{
		"AccessToken": a.AccessToken,
		"IdToken":     a.IDToken,
		"ExpiresIn":   a.ExpiresIn,
		"TokenType":   a.TokenType,
	}
	if a.RefreshToken != "" {
		result["RefreshToken"] = a.RefreshToken
	}
	return json.Marshal(map[string]any{"AuthenticationResult": result})
}

// AuthOutcomeJSON builds InitiateAuth / AdminInitiateAuth / RespondToAuthChallenge body.
// Challenge responses include ChallengeName + Session; success includes AuthenticationResult.
func AuthOutcomeJSON(o store.CognitoAuthOutcome) ([]byte, error) {
	if o.ChallengeName != "" {
		out := map[string]any{
			"ChallengeName": o.ChallengeName,
			"Session":       o.Session,
		}
		if len(o.ChallengeParameters) > 0 {
			out["ChallengeParameters"] = o.ChallengeParameters
		}
		return json.Marshal(out)
	}
	return AuthResultJSON(o.CognitoAuthResult)
}

// AssociateSoftwareTokenJSON builds AssociateSoftwareToken response.
func AssociateSoftwareTokenJSON(secretCode, session string) ([]byte, error) {
	return json.Marshal(map[string]any{
		"SecretCode": secretCode,
		"Session":    session,
	})
}

// VerifySoftwareTokenJSON builds VerifySoftwareToken response.
func VerifySoftwareTokenJSON(status string) ([]byte, error) {
	if status == "" {
		status = "SUCCESS"
	}
	return json.Marshal(map[string]any{"Status": status})
}

// RevokeTokenJSON is an empty OK body.
func RevokeTokenJSON() ([]byte, error) { return []byte(`{}`), nil }
