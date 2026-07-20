package authn

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/identity"
)

const (
	algorithmAWS4 = "AWS4-HMAC-SHA256"
	emptyPayload  = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
)

// Verified is the result of a successful SigV4 verification.
type Verified struct {
	Principal       identity.Principal
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	Region          string
	Service         string
	AccountID       string
	// SourceIP is the caller address for aws:SourceIp (set by the HTTP server).
	SourceIP string
}

// ResolvedKey is the credential material returned by KeyLookup.
type ResolvedKey struct {
	AccountID     string
	Secret        string
	IsRoot        bool
	UserName      string
	Status        string
	SessionToken  string
	RoleARN       string
	SessionName   string
	FederatedUser string
	SessionPolicy string
	ExpiresAt     time.Time
}

// KeyLookup resolves an access key id to credential material.
type KeyLookup func(accessKeyID string) (ResolvedKey, error)

type authMaterial struct {
	accessKeyID   string
	date          string
	region        string
	service       string
	signedHeaders []string
	signature     string
	amzDate       string
	queryAuth     bool
	expires       string
}

// Verify validates AWS SigV4 Authorization header or query authentication.
func Verify(r *http.Request, body []byte, now time.Time, skew time.Duration, lookup KeyLookup) (*Verified, error) {
	if r == nil {
		return nil, newError(CodeMissingAuthenticationToken, "request is required")
	}
	if lookup == nil {
		return nil, newError(CodeInvalidClientTokenId, "key lookup is required")
	}

	mat, err := extractAuth(r)
	if err != nil {
		return nil, err
	}

	reqTime, err := parseAmzDate(mat.amzDate)
	if err != nil {
		return nil, newError(CodeSignatureDoesNotMatch, "invalid X-Amz-Date")
	}
	if skew < 0 {
		skew = 0
	}
	delta := now.UTC().Sub(reqTime)
	if delta < 0 {
		delta = -delta
	}
	if delta > skew {
		return nil, newError(CodeRequestTimeTooSkewed, "request time differs too much from server time")
	}

	if mat.queryAuth && mat.expires != "" {
		secs, err := strconv.ParseInt(mat.expires, 10, 64)
		if err != nil || secs < 0 || secs > 604800 {
			return nil, newError(CodeSignatureDoesNotMatch, "invalid X-Amz-Expires")
		}
		if now.UTC().After(reqTime.Add(time.Duration(secs) * time.Second)) {
			return nil, newError(CodeRequestTimeTooSkewed, "presigned request has expired")
		}
	}

	credDate := mat.date
	if len(mat.amzDate) >= 8 && mat.amzDate[:8] != credDate {
		return nil, newError(CodeSignatureDoesNotMatch, "credential scope date mismatch")
	}

	cred, err := lookup(mat.accessKeyID)
	if err != nil || cred.AccountID == "" || cred.Secret == "" {
		return nil, newError(CodeInvalidClientTokenId, "the access key id does not exist")
	}
	if cred.Status != "" && cred.Status != "Active" {
		return nil, newError(CodeInvalidClientTokenId, "the access key is inactive")
	}
	if !cred.ExpiresAt.IsZero() && now.UTC().After(cred.ExpiresAt.UTC()) {
		return nil, newError(CodeInvalidClientTokenId, "the security token included in the request is expired")
	}
	if cred.SessionToken != "" {
		reqToken := strings.TrimSpace(r.Header.Get("X-Amz-Security-Token"))
		if reqToken == "" {
			reqToken = strings.TrimSpace(r.URL.Query().Get("X-Amz-Security-Token"))
		}
		if reqToken == "" || reqToken != cred.SessionToken {
			return nil, newError(CodeInvalidClientTokenId, "the security token included in the request is invalid")
		}
	}

	payloadHash, err := resolvePayloadHash(r, body, mat)
	if err != nil {
		return nil, err
	}
	canonicalReq := buildCanonicalRequest(r, mat, payloadHash)
	stringToSign := buildStringToSign(mat.amzDate, credDate, mat.region, mat.service, canonicalReq)
	signingKey := deriveSigningKey(cred.Secret, credDate, mat.region, mat.service)
	expected := hex.EncodeToString(hmacSHA256(signingKey, stringToSign))

	if !hmac.Equal([]byte(strings.ToLower(mat.signature)), []byte(strings.ToLower(expected))) {
		return nil, newError(CodeSignatureDoesNotMatch, "the request signature we calculated does not match the signature you provided")
	}

	var principal identity.Principal
	if cred.IsRoot {
		principal = identity.RootPrincipal(cred.AccountID, mat.accessKeyID)
	} else if cred.FederatedUser != "" {
		principal = identity.FederatedUserPrincipal(cred.AccountID, cred.FederatedUser, mat.accessKeyID)
	} else if cred.RoleARN != "" {
		_, roleName, ok := parseRoleNameFromARN(cred.RoleARN)
		if !ok {
			roleName = cred.RoleARN
		}
		principal = identity.RoleSessionPrincipal(cred.AccountID, roleName, cred.SessionName, mat.accessKeyID)
	} else if cred.UserName != "" {
		principal = identity.UserPrincipal(cred.AccountID, cred.UserName, mat.accessKeyID)
	} else {
		principal = identity.Principal{
			Kind:        identity.KindUser,
			AccountID:   cred.AccountID,
			AccessKeyID: mat.accessKeyID,
			IsRoot:      false,
		}
	}

	return &Verified{
		Principal:       principal,
		AccessKeyID:     mat.accessKeyID,
		SecretAccessKey: cred.Secret,
		SessionToken:    cred.SessionToken,
		Region:          mat.region,
		Service:         mat.service,
		AccountID:       cred.AccountID,
	}, nil
}

func parseRoleNameFromARN(roleARN string) (accountID, roleName string, ok bool) {
	const marker = ":role/"
	i := strings.Index(roleARN, marker)
	if i < 0 {
		return "", "", false
	}
	prefix := roleARN[:i]
	const iamPrefix = "arn:aws:iam::"
	if !strings.HasPrefix(prefix, iamPrefix) {
		return "", "", false
	}
	return prefix[len(iamPrefix):], roleARN[i+len(marker):], true
}

func extractAuth(r *http.Request) (*authMaterial, error) {
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if authHeader != "" {
		return parseAuthorizationHeader(r, authHeader)
	}

	q := r.URL.Query()
	if strings.EqualFold(q.Get("X-Amz-Algorithm"), algorithmAWS4) {
		return parseQueryAuth(r, q)
	}

	return nil, newError(CodeMissingAuthenticationToken, "request is missing Authentication Token")
}

func parseAuthorizationHeader(r *http.Request, header string) (*authMaterial, error) {
	if !strings.HasPrefix(header, algorithmAWS4) {
		return nil, newError(CodeMissingAuthenticationToken, "unsupported authorization algorithm")
	}
	rest := strings.TrimSpace(strings.TrimPrefix(header, algorithmAWS4))
	parts := splitAuthParts(rest)

	cred := parts["Credential"]
	signedHeaders := parts["SignedHeaders"]
	sig := parts["Signature"]
	if cred == "" || signedHeaders == "" || sig == "" {
		return nil, newError(CodeSignatureDoesNotMatch, "malformed Authorization header")
	}

	akid, date, region, service, err := parseCredential(cred)
	if err != nil {
		return nil, err
	}

	amzDate := r.Header.Get("X-Amz-Date")
	if amzDate == "" {
		amzDate = r.Header.Get("x-amz-date")
	}
	if amzDate == "" {
		return nil, newError(CodeSignatureDoesNotMatch, "missing X-Amz-Date")
	}

	return &authMaterial{
		accessKeyID:   akid,
		date:          date,
		region:        region,
		service:       service,
		signedHeaders: strings.Split(strings.ToLower(signedHeaders), ";"),
		signature:     sig,
		amzDate:       amzDate,
		queryAuth:     false,
	}, nil
}

func parseQueryAuth(r *http.Request, q url.Values) (*authMaterial, error) {
	cred := q.Get("X-Amz-Credential")
	signedHeaders := q.Get("X-Amz-SignedHeaders")
	sig := q.Get("X-Amz-Signature")
	amzDate := q.Get("X-Amz-Date")
	if amzDate == "" {
		amzDate = r.Header.Get("X-Amz-Date")
	}
	if cred == "" || signedHeaders == "" || sig == "" || amzDate == "" {
		return nil, newError(CodeSignatureDoesNotMatch, "malformed query authentication")
	}

	akid, date, region, service, err := parseCredential(cred)
	if err != nil {
		return nil, err
	}

	return &authMaterial{
		accessKeyID:   akid,
		date:          date,
		region:        region,
		service:       service,
		signedHeaders: strings.Split(strings.ToLower(signedHeaders), ";"),
		signature:     sig,
		amzDate:       amzDate,
		queryAuth:     true,
		expires:       q.Get("X-Amz-Expires"),
	}, nil
}

func parseCredential(cred string) (akid, date, region, service string, err error) {
	// Credential may be URL-decoded already from query; header is raw.
	parts := strings.Split(cred, "/")
	if len(parts) != 5 || parts[4] != "aws4_request" {
		return "", "", "", "", newError(CodeSignatureDoesNotMatch, "malformed credential scope")
	}
	if parts[0] == "" || parts[1] == "" || parts[2] == "" || parts[3] == "" {
		return "", "", "", "", newError(CodeSignatureDoesNotMatch, "malformed credential scope")
	}
	return parts[0], parts[1], parts[2], parts[3], nil
}

func splitAuthParts(rest string) map[string]string {
	out := make(map[string]string)
	for _, part := range strings.Split(rest, ",") {
		part = strings.TrimSpace(part)
		key, val, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		out[strings.TrimSpace(key)] = strings.TrimSpace(val)
	}
	return out
}

func parseAmzDate(s string) (time.Time, error) {
	return time.Parse("20060102T150405Z", s)
}

const unsignedPayload = "UNSIGNED-PAYLOAD"

func bodySHA256Hex(body []byte) string {
	if len(body) == 0 {
		return emptyPayload
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func claimedContentSHA256(r *http.Request) string {
	h := r.Header.Get("X-Amz-Content-Sha256")
	if h == "" {
		h = r.Header.Get("x-amz-content-sha256")
	}
	return strings.TrimSpace(h)
}

func allowUnsignedPayload(mat *authMaterial) bool {
	return mat != nil && mat.queryAuth && strings.EqualFold(mat.service, "s3")
}

// resolvePayloadHash binds X-Amz-Content-Sha256 to the request body except for
// S3 query-auth UNSIGNED-PAYLOAD (presigned URLs). Mismatch fails closed.
func resolvePayloadHash(r *http.Request, body []byte, mat *authMaterial) (string, error) {
	actual := bodySHA256Hex(body)
	claimed := claimedContentSHA256(r)
	if claimed == "" {
		// AWS S3 presigned URLs sign with UNSIGNED-PAYLOAD when the header is omitted.
		if allowUnsignedPayload(mat) {
			return unsignedPayload, nil
		}
		return actual, nil
	}
	if strings.EqualFold(claimed, unsignedPayload) {
		if !allowUnsignedPayload(mat) {
			return "", newError(CodeSignatureDoesNotMatch, "UNSIGNED-PAYLOAD is only allowed for S3 query authentication")
		}
		return unsignedPayload, nil
	}
	if !strings.EqualFold(claimed, actual) {
		return "", newError(CodeSignatureDoesNotMatch, "X-Amz-Content-Sha256 does not match the request body")
	}
	return claimed, nil
}

func buildCanonicalRequest(r *http.Request, mat *authMaterial, payloadHash string) string {
	method := r.Method
	if method == "" {
		method = http.MethodGet
	}

	canonicalURI := canonicalURI(r)
	canonicalQuery := canonicalQueryString(r, mat.queryAuth)
	canonicalHeaders, signedHeaders := canonicalHeaders(r, mat.signedHeaders)

	return strings.Join([]string{
		method,
		canonicalURI,
		canonicalQuery,
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")
}

func canonicalURI(r *http.Request) string {
	path := r.URL.EscapedPath()
	if path == "" {
		path = r.URL.Path
	}
	if path == "" {
		return "/"
	}
	// EscapedPath is already percent-encoded; ensure leading slash.
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}

func canonicalQueryString(r *http.Request, queryAuth bool) string {
	q := r.URL.Query()
	type kv struct{ k, v string }
	var pairs []kv
	for key, values := range q {
		if queryAuth && strings.EqualFold(key, "X-Amz-Signature") {
			continue
		}
		encKey := uriEncode(key, true)
		if len(values) == 0 {
			pairs = append(pairs, kv{encKey, ""})
			continue
		}
		for _, v := range values {
			pairs = append(pairs, kv{encKey, uriEncode(v, true)})
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].k == pairs[j].k {
			return pairs[i].v < pairs[j].v
		}
		return pairs[i].k < pairs[j].k
	})
	parts := make([]string, len(pairs))
	for i, p := range pairs {
		parts[i] = p.k + "=" + p.v
	}
	return strings.Join(parts, "&")
}

func canonicalHeaders(r *http.Request, signed []string) (canonical, signedHeaderList string) {
	var b strings.Builder
	names := make([]string, 0, len(signed))
	for _, name := range signed {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" {
			continue
		}
		names = append(names, name)
		val := headerValue(r, name)
		b.WriteString(name)
		b.WriteByte(':')
		b.WriteString(val)
		b.WriteByte('\n')
	}
	return b.String(), strings.Join(names, ";")
}

func headerValue(r *http.Request, name string) string {
	if name == "host" {
		host := r.Host
		if host == "" {
			host = r.Header.Get("Host")
		}
		return trimHeaderValue(host)
	}
	vals := r.Header.Values(name)
	if len(vals) == 0 {
		// http.Header is canonical; try Get for case variants.
		v := r.Header.Get(name)
		return trimHeaderValue(v)
	}
	parts := make([]string, len(vals))
	for i, v := range vals {
		parts[i] = trimHeaderValue(v)
	}
	return strings.Join(parts, ",")
}

func trimHeaderValue(v string) string {
	v = strings.TrimSpace(v)
	// Collapse sequential spaces to a single space.
	var b strings.Builder
	prevSpace := false
	for _, r := range v {
		if r == ' ' || r == '\t' {
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
			continue
		}
		prevSpace = false
		b.WriteRune(r)
	}
	return b.String()
}

func buildStringToSign(amzDate, credDate, region, service, canonicalRequest string) string {
	scope := fmt.Sprintf("%s/%s/%s/aws4_request", credDate, region, service)
	sum := sha256.Sum256([]byte(canonicalRequest))
	return strings.Join([]string{
		algorithmAWS4,
		amzDate,
		scope,
		hex.EncodeToString(sum[:]),
	}, "\n")
}

func deriveSigningKey(secret, date, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), date)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, service)
	return hmacSHA256(kService, "aws4_request")
}

func hmacSHA256(key []byte, data string) []byte {
	m := hmac.New(sha256.New, key)
	_, _ = m.Write([]byte(data))
	return m.Sum(nil)
}

// uriEncode encodes per AWS SigV4 rules.
func uriEncode(s string, encodeSlash bool) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.' || c == '~' || (c == '/' && !encodeSlash) {
			b.WriteByte(c)
			continue
		}
		fmt.Fprintf(&b, "%%%02X", c)
	}
	return b.String()
}
