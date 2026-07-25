package server

import (
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
)

func (s *Server) recordAccessKeyLastUsed(verified *authn.Verified) {
	if verified == nil {
		return
	}
	akid := strings.TrimSpace(verified.AccessKeyID)
	if akid == "" || !strings.HasPrefix(akid, "AKIA") {
		return
	}
	_ = s.store.RecordAccessKeyLastUsed(akid, verified.Service, verified.Region, s.now().UTC())
}
