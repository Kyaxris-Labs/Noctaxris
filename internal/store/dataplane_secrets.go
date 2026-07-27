package store

import (
	"errors"
	"fmt"
	"strings"
)

// DataPlaneSecretKind selects the Secrets Manager name prefix for nested engines.
type DataPlaneSecretKind string

const (
	DataPlaneSecretRDS         DataPlaneSecretKind = "rds"
	DataPlaneSecretElastiCache DataPlaneSecretKind = "elasticache"
	DataPlaneSecretMemoryDB    DataPlaneSecretKind = "memorydb"
	DataPlaneSecretDocDB       DataPlaneSecretKind = "docdb"
)

// DataPlaneMasterSecretName builds noctaxris/<kind>/<resourceID>.
// Used by ElastiCache/DocumentDB create paths.
func DataPlaneMasterSecretName(kind DataPlaneSecretKind, resourceID string) string {
	return fmt.Sprintf("noctaxris/%s/%s", kind, strings.ToLower(strings.TrimSpace(resourceID)))
}

// EnsureDataPlaneMasterSecret creates (or updates) a username/password JSON secret.
// Returns the secret ARN. Fail-closed when secretString cannot be stored.
func (s *Store) EnsureDataPlaneMasterSecret(
	accountID, region string,
	kind DataPlaneSecretKind,
	resourceID, username, password string,
) (string, error) {
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)
	if username == "" || password == "" {
		return "", fmt.Errorf("%w: username and password are required for data-plane secrets", ErrRDSBadRequest)
	}
	name := DataPlaneMasterSecretName(kind, resourceID)
	secretString, err := RDSMasterSecretJSON(username, password)
	if err != nil {
		return "", err
	}
	sec, err := s.CreateSecret(accountID, region, name, secretString, nil, "", "Noctaxris nested data-plane master user", "")
	if err == nil {
		return sec.ARN, nil
	}
	if !errors.Is(err, ErrSecretAlreadyExists) {
		return "", fmt.Errorf("ensure data-plane secret: %w", err)
	}
	if _, err := s.PutSecretValue(accountID, name, secretString, nil); err != nil {
		return "", fmt.Errorf("ensure data-plane secret: put: %w", err)
	}
	desc, err := s.DescribeSecret(accountID, name)
	if err != nil {
		return "", fmt.Errorf("ensure data-plane secret: describe: %w", err)
	}
	return desc.ARN, nil
}

// ResolveDataPlaneSecretARN validates a secret exists and is readable for the account.
func (s *Store) ResolveDataPlaneSecretARN(accountID, secretARN string) (Secret, error) {
	secretARN = strings.TrimSpace(secretARN)
	if secretARN == "" {
		return Secret{}, fmt.Errorf("secret ARN is required")
	}
	sec, err := s.GetSecretValue(accountID, secretARN)
	if err != nil {
		return Secret{}, err
	}
	return sec, nil
}
