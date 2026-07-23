package cognito

import (
	"encoding/json"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// CreateUserPoolJSON builds CreateUserPool response.
func CreateUserPoolJSON(p store.CognitoUserPool) ([]byte, error) {
	return json.Marshal(map[string]any{
		"UserPool": map[string]any{
			"Id":       p.PoolID,
			"Name":     p.Name,
			"Arn":      p.ARN,
			"CreationDate": float64(p.CreatedAt) / 1000.0,
		},
	})
}

// DescribeUserPoolJSON builds DescribeUserPool response.
func DescribeUserPoolJSON(p store.CognitoUserPool) ([]byte, error) {
	return CreateUserPoolJSON(p)
}

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
			"Attributes": []map[string]string{
				{"Name": "sub", "Value": u.Sub},
			},
		},
	})
}

// SignUpJSON builds SignUp response.
func SignUpJSON(u store.CognitoUser) ([]byte, error) {
	return json.Marshal(map[string]any{
		"UserConfirmed": false,
		"UserSub":       u.Sub,
	})
}

// ConfirmSignUpJSON is an empty OK body.
func ConfirmSignUpJSON() ([]byte, error) { return []byte(`{}`), nil }

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

// RevokeTokenJSON is an empty OK body.
func RevokeTokenJSON() ([]byte, error) { return []byte(`{}`), nil }
