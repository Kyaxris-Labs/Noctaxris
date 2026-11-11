package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/validate"
)

const standaloneSeedPath = "/_noctaxris/lab/accounts:seedStandalone"

const standaloneSeedDefaultName = "default"

func (s *Server) handleSeedStandalone(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	if verified == nil || !verified.Principal.IsRoot || !s.store.IsManagementAccount(verified.AccountID) {
		accountID := ""
		accessKeyID := ""
		if verified != nil {
			accountID = verified.AccountID
			accessKeyID = verified.AccessKeyID
		}
		s.writeAWSError(w, requestID, http.StatusForbidden, "AccessDenied",
			"Not authorized to seed a standalone account.", readOnly, r, eventID,
			accessKeyID, accountID, verified != nil)
		return
	}
	name := standaloneSeedDefaultName
	if len(strings.TrimSpace(string(body))) > 0 {
		var req struct {
			Name *string `json:"name"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			s.writeAWSError(w, requestID, http.StatusBadRequest, "InvalidRequest",
				"Request body must be a JSON object.", readOnly, r, eventID,
				verified.AccessKeyID, verified.AccountID, true)
			return
		}
		if req.Name != nil {
			raw := *req.Name
			trimmed := strings.TrimSpace(raw)
			if raw != "" && trimmed == "" {
				s.writeAWSError(w, requestID, http.StatusBadRequest, "ValidationError",
					"Seed name must match an IAM name.", readOnly, r, eventID,
					verified.AccessKeyID, verified.AccountID, true)
				return
			}
			if trimmed != "" {
				name = trimmed
			}
		}
	}
	if err := validate.IAMName(name); err != nil {
		s.writeAWSError(w, requestID, http.StatusBadRequest, "ValidationError",
			"Seed name must match an IAM name.", readOnly, r, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}
	seeded, err := s.store.SeedStandaloneAccount(name, s.cfg.RootSecretAccessKey)
	if err != nil {
		s.writeAWSError(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to seed the standalone account.", readOnly, r, eventID,
			verified.AccessKeyID, verified.AccountID, true)
		return
	}
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"accountId":       seeded.AccountID,
		"userName":        seeded.UserName,
		"userArn":         seeded.UserARN,
		"accessKeyId":     seeded.AccessKeyID,
		"secretAccessKey": seeded.SecretAccessKey,
	})
}

func iamPolicyArnAccount(policyArn string) (string, bool) {
	parts := strings.Split(strings.TrimSpace(policyArn), ":")
	if len(parts) < 6 || parts[0] != "arn" || parts[2] != "iam" {
		return "", false
	}
	account := strings.TrimSpace(parts[4])
	if account == "" {
		return "", false
	}
	return account, true
}

// policyDocumentInCallerAccount reports whether a policy document read may
// proceed. Malformed or empty ARNs proceed so the existing not-found path
// stays. A real stored account other than the caller is denied before the
// document is loaded. The account aws is the AWS-managed policy account and
// is still attempted.
func (s *Server) denyIAMOutsideCaller(
	w http.ResponseWriter,
	requestID string,
	readOnly bool,
	r *http.Request,
	eventID, accessKeyID, callerAccount, resourceArn, message string,
) bool {
	if s.policyDocumentInCallerAccount(callerAccount, resourceArn) {
		return false
	}
	s.writeAWSError(w, requestID, http.StatusForbidden, "AccessDenied",
		message, readOnly, r, eventID, accessKeyID, callerAccount, accessKeyID != "")
	return true
}

func (s *Server) policyDocumentInCallerAccount(callerAccount, policyArn string) bool {
	account, ok := iamPolicyArnAccount(policyArn)
	if !ok || account == callerAccount || account == "aws" {
		return true
	}
	if s.store == nil {
		return false
	}
	return !s.store.AccountExists(account)
}
