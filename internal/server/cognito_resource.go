package server

import (
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func cognitoRegion(verified *authn.Verified) string {
	if verified != nil && verified.Region != "" {
		return verified.Region
	}
	return store.DefaultCognitoRegion
}

// cognitoPoolResource returns the user-pool ARN when poolID is known; otherwise "*".
func cognitoPoolResource(verified *authn.Verified, poolID string) string {
	poolID = strings.TrimSpace(poolID)
	if verified == nil || poolID == "" {
		return "*"
	}
	return store.CognitoPoolARN(cognitoRegion(verified), verified.AccountID, poolID)
}

// cognitoUserResource returns the user ARN when username is known; otherwise the pool ARN.
func cognitoUserResource(verified *authn.Verified, poolID, username string) string {
	pool := cognitoPoolResource(verified, poolID)
	username = strings.TrimSpace(username)
	if pool == "*" || username == "" {
		return pool
	}
	return pool + "/user/" + username
}
