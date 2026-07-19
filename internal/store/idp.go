package store

import (
	"database/sql"
	"fmt"
	"strings"
)

// OIDCProvider is an IAM OIDC identity provider config.
type OIDCProvider struct {
	ProviderARN string
	AccountID   string
	URL         string
	ClientID    string
}

// SAMLProvider is an IAM SAML identity provider config.
type SAMLProvider struct {
	ProviderARN string
	AccountID   string
	MetadataXML string
}

// OIDCProviderARN builds an OIDC provider ARN from account and issuer URL.
func OIDCProviderARN(accountID, url string) string {
	trimmed := strings.TrimPrefix(url, "https://")
	trimmed = strings.TrimPrefix(trimmed, "http://")
	trimmed = strings.TrimSuffix(trimmed, "/")
	return fmt.Sprintf("arn:aws:iam::%s:oidc-provider/%s", accountID, trimmed)
}

// SAMLProviderARN builds a SAML provider ARN.
func SAMLProviderARN(accountID, name string) string {
	return fmt.Sprintf("arn:aws:iam::%s:saml-provider/%s", accountID, name)
}

// PutOIDCProvider stores or replaces an OIDC provider.
func (s *Store) PutOIDCProvider(accountID, url, clientID string) (providerARN string, err error) {
	if accountID == "" || url == "" || clientID == "" {
		return "", fmt.Errorf("put oidc provider: account_id, url, and client_id required")
	}
	providerARN = OIDCProviderARN(accountID, url)
	_, err = s.db.Exec(
		`INSERT INTO oidc_providers (provider_arn, account_id, url, client_id) VALUES (?, ?, ?, ?)
		 ON CONFLICT(provider_arn) DO UPDATE SET
		   account_id = excluded.account_id,
		   url = excluded.url,
		   client_id = excluded.client_id`,
		providerARN, accountID, url, clientID,
	)
	if err != nil {
		return "", fmt.Errorf("put oidc provider %s: %w", providerARN, err)
	}
	return providerARN, nil
}

// GetOIDCProviderByURL returns an OIDC provider matching accountID and URL.
func (s *Store) GetOIDCProviderByURL(accountID, url string) (OIDCProvider, error) {
	normalized := normalizeOIDCURL(url)
	providers, err := s.ListOIDCProviders(accountID)
	if err != nil {
		return OIDCProvider{}, err
	}
	for _, p := range providers {
		if normalizeOIDCURL(p.URL) == normalized {
			return p, nil
		}
	}
	return OIDCProvider{}, sql.ErrNoRows
}

func normalizeOIDCURL(url string) string {
	u := strings.TrimSpace(url)
	u = strings.TrimSuffix(u, "/")
	u = strings.TrimPrefix(u, "https://")
	u = strings.TrimPrefix(u, "http://")
	return strings.ToLower(u)
}

// ListOIDCProviders returns OIDC providers for accountID.
func (s *Store) ListOIDCProviders(accountID string) ([]OIDCProvider, error) {
	rows, err := s.db.Query(
		`SELECT provider_arn, account_id, url, client_id FROM oidc_providers
		 WHERE account_id = ? ORDER BY url`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list oidc providers %s: %w", accountID, err)
	}
	defer rows.Close()

	var out []OIDCProvider
	for rows.Next() {
		var p OIDCProvider
		if err := rows.Scan(&p.ProviderARN, &p.AccountID, &p.URL, &p.ClientID); err != nil {
			return nil, fmt.Errorf("list oidc providers %s: %w", accountID, err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list oidc providers %s: %w", accountID, err)
	}
	if out == nil {
		out = []OIDCProvider{}
	}
	return out, nil
}

// GetOIDCProvider returns an OIDC provider by ARN.
func (s *Store) GetOIDCProvider(providerARN string) (OIDCProvider, error) {
	var p OIDCProvider
	err := s.db.QueryRow(
		`SELECT provider_arn, account_id, url, client_id FROM oidc_providers WHERE provider_arn = ?`,
		providerARN,
	).Scan(&p.ProviderARN, &p.AccountID, &p.URL, &p.ClientID)
	if err != nil {
		return OIDCProvider{}, err
	}
	return p, nil
}

// DeleteOIDCProvider deletes an OIDC provider by ARN.
func (s *Store) DeleteOIDCProvider(providerARN string) error {
	res, err := s.db.Exec(`DELETE FROM oidc_providers WHERE provider_arn = ?`, providerARN)
	if err != nil {
		return fmt.Errorf("delete oidc provider %s: %w", providerARN, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete oidc provider %s: %w", providerARN, err)
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// PutSAMLProvider stores or replaces a SAML provider by name.
func (s *Store) PutSAMLProvider(accountID, name, metadataXML string) (providerARN string, err error) {
	if accountID == "" || name == "" || metadataXML == "" {
		return "", fmt.Errorf("put saml provider: account_id, name, and metadata_xml required")
	}
	providerARN = SAMLProviderARN(accountID, name)
	_, err = s.db.Exec(
		`INSERT INTO saml_providers (provider_arn, account_id, metadata_xml) VALUES (?, ?, ?)
		 ON CONFLICT(provider_arn) DO UPDATE SET
		   account_id = excluded.account_id,
		   metadata_xml = excluded.metadata_xml`,
		providerARN, accountID, metadataXML,
	)
	if err != nil {
		return "", fmt.Errorf("put saml provider %s: %w", providerARN, err)
	}
	return providerARN, nil
}

// GetSAMLProvider returns a SAML provider by ARN.
func (s *Store) GetSAMLProvider(providerARN string) (SAMLProvider, error) {
	var p SAMLProvider
	err := s.db.QueryRow(
		`SELECT provider_arn, account_id, metadata_xml FROM saml_providers WHERE provider_arn = ?`,
		providerARN,
	).Scan(&p.ProviderARN, &p.AccountID, &p.MetadataXML)
	if err != nil {
		return SAMLProvider{}, err
	}
	return p, nil
}

// ListSAMLProviders returns SAML providers for accountID.
func (s *Store) ListSAMLProviders(accountID string) ([]SAMLProvider, error) {
	rows, err := s.db.Query(
		`SELECT provider_arn, account_id, metadata_xml FROM saml_providers
		 WHERE account_id = ? ORDER BY provider_arn`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list saml providers %s: %w", accountID, err)
	}
	defer rows.Close()

	var out []SAMLProvider
	for rows.Next() {
		var p SAMLProvider
		if err := rows.Scan(&p.ProviderARN, &p.AccountID, &p.MetadataXML); err != nil {
			return nil, fmt.Errorf("list saml providers %s: %w", accountID, err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list saml providers %s: %w", accountID, err)
	}
	if out == nil {
		out = []SAMLProvider{}
	}
	return out, nil
}

// DeleteSAMLProvider deletes a SAML provider by ARN.
func (s *Store) DeleteSAMLProvider(providerARN string) error {
	res, err := s.db.Exec(`DELETE FROM saml_providers WHERE provider_arn = ?`, providerARN)
	if err != nil {
		return fmt.Errorf("delete saml provider %s: %w", providerARN, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete saml provider %s: %w", providerARN, err)
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}
