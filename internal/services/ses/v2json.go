package ses

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func labDkimTokens(identity string) []string {
	sum := sha256.Sum256([]byte(identity))
	return []string{
		hex.EncodeToString(sum[0:8]),
		hex.EncodeToString(sum[8:16]),
		hex.EncodeToString(sum[16:24]),
	}
}

func v2IdentityType(t string) string {
	if strings.EqualFold(t, "EmailAddress") {
		return "EMAIL_ADDRESS"
	}
	return "DOMAIN"
}

func v2VerificationStatus(id store.SESIdentity) string {
	if id.Verified {
		return "SUCCESS"
	}
	if strings.EqualFold(id.Type, "Domain") {
		return "PENDING"
	}
	return "PENDING"
}

func v2DkimStatus(id store.SESIdentity) string {
	if id.Verified {
		return "SUCCESS"
	}
	return "PENDING"
}

func CreateEmailIdentityJSON(id store.SESIdentity) ([]byte, error) {
	out := map[string]any{
		"IdentityType":            v2IdentityType(id.Type),
		"VerifiedForSendingStatus": id.Verified,
		"DkimAttributes": map[string]any{
			"SigningEnabled": true,
			"Status":         v2DkimStatus(id),
			"Tokens":         labDkimTokens(id.Identity),
		},
	}
	return json.Marshal(out)
}

func ListEmailIdentitiesJSON(ids []store.SESIdentity) ([]byte, error) {
	items := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		status := v2VerificationStatus(id)
		items = append(items, map[string]any{
			"IdentityType":       v2IdentityType(id.Type),
			"IdentityName":       id.Identity,
			"SendingEnabled":     status == "SUCCESS",
			"VerificationStatus": status,
		})
	}
	return json.Marshal(map[string]any{"EmailIdentities": items})
}

func GetEmailIdentityJSON(id store.SESIdentity) ([]byte, error) {
	status := v2VerificationStatus(id)
	out := map[string]any{
		"IdentityType":             v2IdentityType(id.Type),
		"VerifiedForSendingStatus": id.Verified,
		"VerificationStatus":       status,
		"FeedbackForwardingStatus": true,
		"DkimAttributes": map[string]any{
			"SigningEnabled": true,
			"Status":         v2DkimStatus(id),
			"Tokens":         labDkimTokens(id.Identity),
		},
		"MailFromAttributes": map[string]any{
			"BehaviorOnMxFailure": "USE_DEFAULT_VALUE",
		},
		"Policies": map[string]any{},
		"Tags":     []any{},
	}
	return json.Marshal(out)
}

func SendEmailV2JSON(messageID string) ([]byte, error) {
	return json.Marshal(map[string]string{"MessageId": messageID})
}

func GetAccountJSON(sentCount int64) ([]byte, error) {
	out := map[string]any{
		"DedicatedIpAutoWarmupEnabled": false,
		"EnforcementStatus":          "HEALTHY",
		"ProductionAccessEnabled":      true,
		"SendingEnabled":             true,
		"SendQuota": map[string]float64{
			"Max24HourSend":    200,
			"MaxSendRate":      1,
			"SentLast24Hours":  float64(sentCount),
		},
		"SuppressionAttributes": map[string]any{
			"SuppressedReasons": []string{"BOUNCE", "COMPLAINT"},
		},
	}
	return json.Marshal(out)
}

func EmptyJSON() []byte {
	return []byte("{}")
}
