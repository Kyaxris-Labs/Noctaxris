package iot

import (
	"encoding/json"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func thingMap(t store.IoTThing) map[string]any {
	return map[string]any{
		"thingName":  t.ThingName,
		"thingArn":   t.ThingARN,
		"attributes": t.Attributes,
		"version":    t.Version,
	}
}

// CreateThingJSON builds CreateThing response.
func CreateThingJSON(t store.IoTThing) ([]byte, error) {
	return json.Marshal(map[string]any{
		"thingName": t.ThingName,
		"thingArn":  t.ThingARN,
	})
}

// DescribeThingJSON builds DescribeThing response.
func DescribeThingJSON(t store.IoTThing) ([]byte, error) {
	return json.Marshal(thingMap(t))
}

// ListThingsJSON builds ListThings response.
func ListThingsJSON(things []store.IoTThing) ([]byte, error) {
	items := make([]map[string]any, 0, len(things))
	for _, t := range things {
		items = append(items, map[string]any{
			"thingName": t.ThingName,
			"thingArn":  t.ThingARN,
		})
	}
	return json.Marshal(map[string]any{"things": items})
}

// UpdateThingJSON builds UpdateThing response.
func UpdateThingJSON(t store.IoTThing) ([]byte, error) {
	return json.Marshal(map[string]any{
		"thingName": t.ThingName,
		"thingArn":  t.ThingARN,
		"version":   t.Version,
	})
}

// EmptyJSON is {}.
func EmptyJSON() ([]byte, error) { return []byte(`{}`), nil }

// CreateKeysAndCertificateJSON builds CreateKeysAndCertificate response.
func CreateKeysAndCertificateJSON(c store.IoTCertificate) ([]byte, error) {
	return json.Marshal(map[string]any{
		"certificateArn": c.CertificateARN,
		"certificateId":  c.CertificateID,
		"certificatePem": c.CertificatePEM,
		"keyPair": map[string]string{
			"PublicKey":  c.PublicKey,
			"PrivateKey": c.PrivateKey,
		},
	})
}

// DescribeCertificateJSON builds DescribeCertificate response.
func DescribeCertificateJSON(c store.IoTCertificate) ([]byte, error) {
	return json.Marshal(map[string]any{
		"certificateDescription": map[string]any{
			"certificateArn": c.CertificateARN,
			"certificateId":  c.CertificateID,
			"status":         c.Status,
			"certificatePem": c.CertificatePEM,
			"creationDate":   float64(c.CreatedAt) / 1000.0,
		},
	})
}

// ListCertificatesJSON builds ListCertificates response.
func ListCertificatesJSON(certs []store.IoTCertificate) ([]byte, error) {
	items := make([]map[string]any, 0, len(certs))
	for _, c := range certs {
		items = append(items, map[string]any{
			"certificateArn": c.CertificateARN,
			"certificateId":  c.CertificateID,
			"status":         c.Status,
			"creationDate":   float64(c.CreatedAt) / 1000.0,
		})
	}
	return json.Marshal(map[string]any{"certificates": items})
}

// CreatePolicyJSON builds CreatePolicy response.
func CreatePolicyJSON(p store.IoTPolicy) ([]byte, error) {
	return json.Marshal(map[string]any{
		"policyName":     p.PolicyName,
		"policyArn":      p.PolicyARN,
		"policyDocument": p.PolicyDocument,
		"policyVersionId": "1",
	})
}

// GetPolicyJSON builds GetPolicy response.
func GetPolicyJSON(p store.IoTPolicy) ([]byte, error) {
	return CreatePolicyJSON(p)
}

// ListPoliciesJSON builds ListPolicies response.
func ListPoliciesJSON(policies []store.IoTPolicy) ([]byte, error) {
	items := make([]map[string]any, 0, len(policies))
	for _, p := range policies {
		items = append(items, map[string]any{
			"policyName": p.PolicyName,
			"policyArn":  p.PolicyARN,
		})
	}
	return json.Marshal(map[string]any{"policies": items})
}

// ListThingPrincipalsJSON builds ListThingPrincipals response.
func ListThingPrincipalsJSON(principals []string) ([]byte, error) {
	if principals == nil {
		principals = []string{}
	}
	return json.Marshal(map[string]any{"principals": principals})
}
