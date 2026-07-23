package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

// OIDCProvider is an IAM OIDC identity provider config.
type OIDCProvider struct {
	ProviderARN string
	AccountID   string
	URL         string
	ClientID    string   // primary (first) client ID for backward-compatible callers
	ClientIDs   []string // full ClientIDList
	Thumbprints []string // stored for honesty; JWKS path does not validate them
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

// PutOIDCProvider stores or replaces an OIDC provider with a single client ID.
func (s *Store) PutOIDCProvider(accountID, url, clientID string) (providerARN string, err error) {
	return s.PutOIDCProviderOpts(accountID, url, []string{clientID}, nil)
}

// PutOIDCProviderOpts stores an OIDC provider with ClientIDList and optional thumbprints.
func (s *Store) PutOIDCProviderOpts(accountID, url string, clientIDs, thumbprints []string) (providerARN string, err error) {
	clientIDs = normalizeStringList(clientIDs)
	if accountID == "" || url == "" || len(clientIDs) == 0 {
		return "", fmt.Errorf("put oidc provider: account_id, url, and client_id required")
	}
	providerARN = OIDCProviderARN(accountID, url)
	clientIDsJSON, err := json.Marshal(clientIDs)
	if err != nil {
		return "", fmt.Errorf("put oidc provider: marshal client ids: %w", err)
	}
	thumbJSON, err := json.Marshal(normalizeStringList(thumbprints))
	if err != nil {
		return "", fmt.Errorf("put oidc provider: marshal thumbprints: %w", err)
	}
	_, err = s.db.Exec(
		`INSERT INTO oidc_providers (provider_arn, account_id, url, client_id, client_ids, thumbprints)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(provider_arn) DO UPDATE SET
		   account_id = excluded.account_id,
		   url = excluded.url,
		   client_id = excluded.client_id,
		   client_ids = excluded.client_ids,
		   thumbprints = excluded.thumbprints`,
		providerARN, accountID, url, clientIDs[0], string(clientIDsJSON), string(thumbJSON),
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

func normalizeStringList(in []string) []string {
	var out []string
	seen := map[string]struct{}{}
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func scanOIDCProvider(providerARN, accountID, url, clientID, clientIDsJSON, thumbJSON string) OIDCProvider {
	p := OIDCProvider{
		ProviderARN: providerARN,
		AccountID:   accountID,
		URL:         url,
		ClientID:    clientID,
	}
	if strings.TrimSpace(clientIDsJSON) != "" {
		_ = json.Unmarshal([]byte(clientIDsJSON), &p.ClientIDs)
	}
	if len(p.ClientIDs) == 0 && clientID != "" {
		p.ClientIDs = []string{clientID}
	}
	if p.ClientID == "" && len(p.ClientIDs) > 0 {
		p.ClientID = p.ClientIDs[0]
	}
	if strings.TrimSpace(thumbJSON) != "" {
		_ = json.Unmarshal([]byte(thumbJSON), &p.Thumbprints)
	}
	if p.Thumbprints == nil {
		p.Thumbprints = []string{}
	}
	return p
}

// ListOIDCProviders returns OIDC providers for accountID.
func (s *Store) ListOIDCProviders(accountID string) ([]OIDCProvider, error) {
	rows, err := s.db.Query(
		`SELECT provider_arn, account_id, url, client_id,
		        COALESCE(client_ids, ''), COALESCE(thumbprints, '')
		 FROM oidc_providers
		 WHERE account_id = ? ORDER BY url`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list oidc providers %s: %w", accountID, err)
	}
	defer rows.Close()

	var out []OIDCProvider
	for rows.Next() {
		var providerARN, acct, url, clientID, clientIDsJSON, thumbJSON string
		if err := rows.Scan(&providerARN, &acct, &url, &clientID, &clientIDsJSON, &thumbJSON); err != nil {
			return nil, fmt.Errorf("list oidc providers %s: %w", accountID, err)
		}
		out = append(out, scanOIDCProvider(providerARN, acct, url, clientID, clientIDsJSON, thumbJSON))
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
	var providerARNOut, accountID, url, clientID, clientIDsJSON, thumbJSON string
	err := s.db.QueryRow(
		`SELECT provider_arn, account_id, url, client_id,
		        COALESCE(client_ids, ''), COALESCE(thumbprints, '')
		 FROM oidc_providers WHERE provider_arn = ?`,
		providerARN,
	).Scan(&providerARNOut, &accountID, &url, &clientID, &clientIDsJSON, &thumbJSON)
	if err != nil {
		return OIDCProvider{}, err
	}
	return scanOIDCProvider(providerARNOut, accountID, url, clientID, clientIDsJSON, thumbJSON), nil
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
