package acm

import (
	"encoding/json"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// RequestCertificateJSON builds RequestCertificate response.
func RequestCertificateJSON(c store.ACMCertificate) ([]byte, error) {
	return json.Marshal(map[string]any{"CertificateArn": c.CertificateARN})
}

// DescribeCertificateJSON builds DescribeCertificate response.
func DescribeCertificateJSON(c store.ACMCertificate) ([]byte, error) {
	return json.Marshal(map[string]any{
		"Certificate": map[string]any{
			"CertificateArn": c.CertificateARN,
			"DomainName":     c.DomainName,
			"Status":         c.Status,
			"Type":           "AMAZON_ISSUED",
		},
	})
}

// ListCertificatesJSON builds ListCertificates response.
func ListCertificatesJSON(certs []store.ACMCertificate) ([]byte, error) {
	items := make([]map[string]any, 0, len(certs))
	for _, c := range certs {
		items = append(items, map[string]any{
			"CertificateArn": c.CertificateARN,
			"DomainName":     c.DomainName,
			"Status":         c.Status,
		})
	}
	return json.Marshal(map[string]any{"CertificateSummaryList": items})
}

// DeleteCertificateJSON is an empty OK body.
func DeleteCertificateJSON() ([]byte, error) { return []byte(`{}`), nil }
