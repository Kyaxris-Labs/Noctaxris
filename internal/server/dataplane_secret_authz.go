package server

import (
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// authorizeSecretsMint requires secretsmanager:CreateSecret (new) or PutSecretValue (existing)
// before control-plane paths that mint master secrets into Secrets Manager.
func (s *Server) authorizeSecretsMint(verified *authn.Verified, region, secretName string) bool {
	if s == nil || verified == nil || s.store == nil {
		return false
	}
	secretName = strings.TrimSpace(secretName)
	if secretName == "" {
		return false
	}
	if region == "" {
		region = store.DefaultSecretsRegion
	}
	if desc, err := s.store.DescribeSecret(verified.AccountID, secretName); err == nil {
		return s.authorize(verified, catalog.ActionSecretsPutSecretValue, desc.ARN)
	}
	suffix, err := store.NewSecretARNSuffix()
	if err != nil {
		return false
	}
	arn := store.SecretARN(region, verified.AccountID, secretName, suffix)
	return s.authorize(verified, catalog.ActionSecretsCreateSecret, arn)
}
