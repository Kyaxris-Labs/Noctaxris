package server

import (
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// enforceAssociatedWAF blocks the request with 403 when a Web ACL associated to
// any candidate ARN evaluates to Block. Missing associations are a no-op.
func (s *Server) enforceAssociatedWAF(w http.ResponseWriter, accountID string, candidateARNs []string) bool {
	action, associated, err := s.store.EvaluateAssociatedWAF(accountID, candidateARNs, "")
	if err != nil || !associated {
		return true
	}
	if strings.EqualFold(action, "Block") {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return false
	}
	return true
}

func httpAPIWAFCandidateARNs(region, accountID, apiID, stage string) []string {
	if region == "" {
		region = store.DefaultAPIGatewayRegion
	}
	return []string{
		"arn:aws:apigateway:" + region + "::/apis/" + apiID + "/stages/" + stage,
		"arn:aws:apigateway:" + region + "::/apis/" + apiID,
		"arn:aws:execute-api:" + region + ":" + accountID + ":" + apiID + "/" + stage,
		"arn:aws:execute-api:" + region + ":" + accountID + ":" + apiID,
	}
}

func appSyncWAFCandidateARNs(region, accountID, apiID string) []string {
	if region == "" {
		region = store.DefaultAppSyncRegion
	}
	return []string{
		"arn:aws:appsync:" + region + ":" + accountID + ":apis/" + apiID,
	}
}
