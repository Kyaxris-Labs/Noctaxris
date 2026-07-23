package server

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func applyHTTPAPICORSHeaders(w http.ResponseWriter, r *http.Request, cors store.APIGatewayCORS) {
	if !cors.HasCORS() {
		return
	}
	origin := store.MatchCORSOrigin(cors.AllowOrigins, r.Header.Get("Origin"))
	if origin == "" {
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	if cors.AllowCredentials {
		w.Header().Set("Access-Control-Allow-Credentials", "true")
	}
	if len(cors.ExposeHeaders) > 0 {
		w.Header().Set("Access-Control-Expose-Headers", strings.Join(cors.ExposeHeaders, ","))
	}
	methods := cors.AllowMethods
	if len(methods) == 0 {
		methods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}
	}
	w.Header().Set("Access-Control-Allow-Methods", strings.Join(methods, ","))
	headers := cors.AllowHeaders
	if len(headers) == 0 {
		if reqH := strings.TrimSpace(r.Header.Get("Access-Control-Request-Headers")); reqH != "" {
			headers = []string{reqH}
		} else {
			headers = []string{"*"}
		}
	}
	w.Header().Set("Access-Control-Allow-Headers", strings.Join(headers, ","))
	if cors.MaxAge > 0 {
		w.Header().Set("Access-Control-Max-Age", strconv.Itoa(cors.MaxAge))
	}
}

func writeHTTPAPICORSPreflight(w http.ResponseWriter, r *http.Request, cors store.APIGatewayCORS) bool {
	if r.Method != http.MethodOptions || !cors.HasCORS() {
		return false
	}
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	reqMethod := strings.TrimSpace(r.Header.Get("Access-Control-Request-Method"))
	if origin == "" || reqMethod == "" {
		return false
	}
	if store.MatchCORSOrigin(cors.AllowOrigins, origin) == "" {
		w.WriteHeader(http.StatusForbidden)
		return true
	}
	if len(cors.AllowMethods) > 0 && !corsMethodAllowed(cors.AllowMethods, reqMethod) {
		w.WriteHeader(http.StatusForbidden)
		return true
	}
	applyHTTPAPICORSHeaders(w, r, cors)
	w.WriteHeader(http.StatusNoContent)
	return true
}

func corsMethodAllowed(allow []string, method string) bool {
	method = strings.ToUpper(strings.TrimSpace(method))
	for _, m := range allow {
		if m == "*" || strings.EqualFold(m, method) {
			return true
		}
	}
	return false
}
