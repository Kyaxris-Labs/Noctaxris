package server

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/sts"
	iotsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/iot"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	iotRESTContentType     = "application/json"
	iotCredentialsThingHdr = "x-amzn-iot-thingname"
	iotLabEndpointPort     = "4566"
)

func iotPathParts(path string) []string {
	p := strings.Trim(path, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

func isIoTCredentialsPath(r *http.Request) bool {
	if r == nil || r.Method != http.MethodGet {
		return false
	}
	parts := iotPathParts(r.URL.Path)
	return len(parts) == 3 && parts[0] == "role-aliases" && parts[2] == "credentials" && parts[1] != ""
}

func isIoTShadowRESTPath(path string) bool {
	parts := iotPathParts(path)
	return len(parts) == 3 && parts[0] == "things" && parts[2] == "shadow" && parts[1] != ""
}

func isIoTJobsDataPath(path string) bool {
	parts := iotPathParts(path)
	if len(parts) < 3 || parts[0] != "things" || parts[2] != "jobs" || parts[1] == "" {
		return false
	}
	return len(parts) == 3 || len(parts) == 4
}

func isIoTListNamedShadowsPath(path string) bool {
	parts := iotPathParts(path)
	return len(parts) == 5 &&
		parts[0] == "api" && parts[1] == "things" && parts[2] == "shadow" &&
		parts[3] == "ListNamedShadowsForThing" && parts[4] != ""
}

func isIoTDescribeEndpointPath(path string) bool {
	p := strings.TrimSuffix(path, "/")
	return p == "/endpoint"
}

func isIoTDataPlaneRESTPath(path string) bool {
	return isIoTShadowRESTPath(path) || isIoTJobsDataPath(path) || isIoTListNamedShadowsPath(path)
}

func isIoTMuxRESTPath(path string) bool {
	if isIoTDataPlaneRESTPath(path) || isIoTDescribeEndpointPath(path) {
		return true
	}
	parts := iotPathParts(path)
	return len(parts) == 3 && parts[0] == "role-aliases" && parts[2] == "credentials" && parts[1] != ""
}

func isIoTDeviceDeniedControlAction(action string) bool {
	switch action {
	case catalog.ActionIoTListThings, catalog.ActionIoTListRoleAliases,
		"ListThings", "ListRoleAliases":
		return true
	default:
		return false
	}
}

func isIoTDataPlaneSigV4Service(verified *authn.Verified) bool {
	if verified == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(verified.Service)) {
	case "iot", "iotdata", "iot-data", "data.iot", "iotdevicegateway",
		"iot-jobs-data", "iotjobsdata":
		return true
	default:
		return false
	}
}

func isIoTRESTAfterAuth(r *http.Request, verified *authn.Verified) bool {
	if r == nil {
		return false
	}
	if isIoTDataPlaneRESTPath(r.URL.Path) {
		return isIoTDataPlaneSigV4Service(verified)
	}
	if isIoTDescribeEndpointPath(r.URL.Path) && r.Method == http.MethodGet {
		svc := ""
		if verified != nil {
			svc = strings.ToLower(verified.Service)
		}
		return svc == "iot" || strings.HasPrefix(svc, "iot")
	}
	return false
}

func isIoTEndpointType(endpointType string) bool {
	switch endpointType {
	case "", "iot:Data", "iot:Data-ATS", "iot:Jobs", "iot:CredentialProvider":
		return true
	default:
		return false
	}
}

func (s *Server) iotTLSDevice(r *http.Request) *store.IoTMQTTDeviceContext {
	if r == nil || r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		return nil
	}
	dev, err := s.store.ResolveIoTDeviceFromClientCert(r.TLS.PeerCertificates[0])
	if err != nil {
		return nil
	}
	return &dev
}

func (s *Server) iotEndpointAddress(endpointType string) string {
	host := strings.TrimSpace(s.cfg.IoTEndpointHost)
	if host == "" {
		host = "127.0.0.1"
	}
	port := s.iotDeviceHTTPPort()
	if net.ParseIP(host) != nil || strings.EqualFold(host, "localhost") {
		return net.JoinHostPort(host, port)
	}
	switch endpointType {
	case "iot:Data":
		return net.JoinHostPort("data.iot."+host, port)
	case "iot:Data-ATS":
		return net.JoinHostPort("data-ats.iot."+host, port)
	case "iot:Jobs":
		return net.JoinHostPort("jobs.iot."+host, port)
	case "iot:CredentialProvider":
		return net.JoinHostPort("credentials.iot."+host, port)
	default:
		return net.JoinHostPort(host, port)
	}
}

func iotEndpointHost(address string) string {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return address
	}
	return host
}

func (s *Server) handleIoTREST(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool,
) {
	path := r.URL.Path
	switch {
	case isIoTDescribeEndpointPath(path) && r.Method == http.MethodGet:
		params := map[string]any{"endpointType": r.URL.Query().Get("endpointType")}
		s.iotDescribeEndpoint(w, r, body, requestID, eventID, verified, readOnly, params)
	case isIoTListNamedShadowsPath(path) && r.Method == http.MethodGet:
		parts := iotPathParts(path)
		s.iotRESTListNamedShadows(w, r, body, requestID, eventID, verified, readOnly, parts[4])
	case isIoTShadowRESTPath(path):
		parts := iotPathParts(path)
		s.iotRESTShadow(w, r, body, requestID, eventID, verified, readOnly, parts[1])
	case isIoTJobsDataPath(path):
		s.iotRESTJobs(w, r, body, requestID, eventID, verified, readOnly)
	default:
		s.writeIoTRESTError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Unknown IoT data-plane path.", readOnly, eventID, verified)
	}
}

func (s *Server) iotRESTAuthorize(
	r *http.Request, verified *authn.Verified, iamAction, deviceAction, resource string,
) (accountID, region string, ok bool) {
	if device := s.iotTLSDevice(r); device != nil && strings.TrimSpace(r.Header.Get("Authorization")) == "" {
		if !s.store.EvaluateIoTDevicePolicy(device.AccountID, device.Region, device.CertificateID, deviceAction, resource) {
			return "", "", false
		}
		return device.AccountID, device.Region, true
	}
	if verified == nil {
		return "", "", false
	}
	if !s.authorize(verified, iamAction, resource) {
		return "", "", false
	}
	return verified.AccountID, s.iotRegion(verified), true
}

func (s *Server) iotRESTShadow(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, thingName string,
) {
	shadowName := strings.TrimSpace(r.URL.Query().Get("name"))
	region := store.DefaultIoTRegion
	if verified != nil && verified.Region != "" {
		region = verified.Region
	}
	if device := s.iotTLSDevice(r); device != nil && device.Region != "" {
		region = device.Region
	}
	resource := store.IoTThingARN(region, "", thingName)
	if verified != nil {
		resource = store.IoTThingARN(s.iotRegion(verified), verified.AccountID, thingName)
	}
	var iamAction, deviceAction, eventName string
	switch r.Method {
	case http.MethodGet:
		iamAction, deviceAction, eventName = catalog.ActionIoTDataGetThingShadow, "iot:GetThingShadow", "GetThingShadow"
	case http.MethodPost:
		iamAction, deviceAction, eventName = catalog.ActionIoTDataUpdateThingShadow, "iot:UpdateThingShadow", "UpdateThingShadow"
		readOnly = false
	case http.MethodDelete:
		iamAction, deviceAction, eventName = catalog.ActionIoTDataDeleteThingShadow, "iot:DeleteThingShadow", "DeleteThingShadow"
		readOnly = false
	default:
		s.writeIoTRESTError(w, r, body, requestID, http.StatusMethodNotAllowed, "MethodNotAllowedException",
			"Unsupported method for thing shadow.", readOnly, eventID, verified)
		return
	}
	accountID, region, ok := s.iotRESTAuthorize(r, verified, iamAction, deviceAction, resource)
	if !ok {
		s.writeIoTRESTError(w, r, body, requestID, http.StatusForbidden, "UnauthorizedException",
			"Not authorized to access this shadow.", readOnly, eventID, verified)
		return
	}
	resource = store.IoTThingARN(region, accountID, thingName)
	switch r.Method {
	case http.MethodGet:
		sh, err := s.store.GetIoTThingShadow(accountID, region, thingName, shadowName)
		if errors.Is(err, store.ErrIoTNotFound) {
			s.writeIoTRESTError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
				"Shadow not found.", readOnly, eventID, verified)
			return
		}
		if err != nil {
			s.writeIoTRESTError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailureException",
				"Unable to get thing shadow.", readOnly, eventID, verified)
			return
		}
		s.writeIoTRESTOK(w, []byte(sh.PayloadJSON))
	case http.MethodPost:
		payloadMap := jsonBodyMap(body)
		sh, err := s.store.UpdateIoTThingShadow(accountID, region, thingName, shadowName, payloadMap)
		if errors.Is(err, store.ErrIoTNotFound) {
			s.writeIoTRESTError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
				"Thing not found.", readOnly, eventID, verified)
			return
		}
		if err != nil {
			s.writeIoTRESTError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailureException",
				"Unable to update thing shadow.", readOnly, eventID, verified)
			return
		}
		s.writeIoTRESTOK(w, []byte(sh.PayloadJSON))
	case http.MethodDelete:
		err := s.store.DeleteIoTThingShadow(accountID, region, thingName, shadowName)
		if errors.Is(err, store.ErrIoTNotFound) {
			s.writeIoTRESTError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
				"Shadow not found.", readOnly, eventID, verified)
			return
		}
		if err != nil {
			s.writeIoTRESTError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailureException",
				"Unable to delete thing shadow.", readOnly, eventID, verified)
			return
		}
		empty, _ := iotsvc.EmptyJSON()
		s.writeIoTRESTOK(w, empty)
	}
	s.iotMaybeAudit(r, requestID, eventID, verified, iotDataEventSource, eventName, readOnly)
}

func (s *Server) iotRESTListNamedShadows(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, thingName string,
) {
	region := store.DefaultIoTRegion
	accountHint := ""
	if verified != nil {
		region = s.iotRegion(verified)
		accountHint = verified.AccountID
	}
	resource := store.IoTThingARN(region, accountHint, thingName)
	accountID, region, ok := s.iotRESTAuthorize(r, verified,
		catalog.ActionIoTDataListNamedShadowsForThing, "iot:ListNamedShadowsForThing", resource)
	if !ok {
		s.writeIoTRESTError(w, r, body, requestID, http.StatusForbidden, "UnauthorizedException",
			"Not authorized to list named shadows.", readOnly, eventID, verified)
		return
	}
	names, err := s.store.ListIoTNamedShadows(accountID, region, thingName)
	if err != nil {
		s.writeIoTRESTError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailureException",
			"Unable to list named shadows.", readOnly, eventID, verified)
		return
	}
	out, _ := iotsvc.ListNamedShadowsJSON(names, time.Now().UTC().Unix())
	s.writeIoTRESTOK(w, out)
	s.iotMaybeAudit(r, requestID, eventID, verified, iotDataEventSource, "ListNamedShadowsForThing", readOnly)
}

func (s *Server) iotRESTJobs(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool,
) {
	parts := iotPathParts(r.URL.Path)
	thingName := parts[1]
	region := store.DefaultIoTRegion
	accountHint := ""
	if verified != nil {
		region = s.iotRegion(verified)
		accountHint = verified.AccountID
	}
	resource := store.IoTThingARN(region, accountHint, thingName)
	switch {
	case len(parts) == 3 && r.Method == http.MethodGet:
		accountID, region, ok := s.iotRESTAuthorize(r, verified,
			catalog.ActionIoTJobsGetPendingJobExecutions, "iot:GetPendingJobExecutions", resource)
		if !ok {
			s.writeIoTRESTError(w, r, body, requestID, http.StatusForbidden, "UnauthorizedException",
				"Not authorized to get pending job executions.", readOnly, eventID, verified)
			return
		}
		inProgress, queued, err := s.store.ListPendingIoTJobExecutions(accountID, region, thingName)
		if err != nil {
			s.writeIoTRESTError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailureException",
				"Unable to list pending job executions.", readOnly, eventID, verified)
			return
		}
		out, _ := iotsvc.GetPendingJobExecutionsJSON(inProgress, queued)
		s.writeIoTRESTOK(w, out)
		s.iotMaybeAudit(r, requestID, eventID, verified, "iot-jobs-data.amazonaws.com", "GetPendingJobExecutions", readOnly)
	case len(parts) == 4 && r.Method == http.MethodGet:
		jobID, _ := url.PathUnescape(parts[3])
		if jobID == "" {
			jobID = parts[3]
		}
		accountID, region, ok := s.iotRESTAuthorize(r, verified,
			catalog.ActionIoTJobsDescribeJobExecution, "iot:DescribeJobExecution", resource)
		if !ok {
			s.writeIoTRESTError(w, r, body, requestID, http.StatusForbidden, "UnauthorizedException",
				"Not authorized to describe job execution.", readOnly, eventID, verified)
			return
		}
		ex, err := s.store.GetIoTJobExecution(accountID, region, thingName, jobID)
		if errors.Is(err, store.ErrIoTNotFound) {
			s.writeIoTRESTError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
				"Job execution not found.", readOnly, eventID, verified)
			return
		}
		if err != nil {
			s.writeIoTRESTError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailureException",
				"Unable to describe job execution.", readOnly, eventID, verified)
			return
		}
		includeDoc := !strings.EqualFold(r.URL.Query().Get("includeJobDocument"), "false")
		out, _ := iotsvc.DescribeJobExecutionJSON(ex, includeDoc)
		s.writeIoTRESTOK(w, out)
		s.iotMaybeAudit(r, requestID, eventID, verified, "iot-jobs-data.amazonaws.com", "DescribeJobExecution", readOnly)
	case len(parts) == 4 && r.Method == http.MethodPut && iotJobIDIsNext(parts[3]):
		accountID, region, ok := s.iotRESTAuthorize(r, verified,
			catalog.ActionIoTJobsStartNextPendingJobExecution, "iot:StartNextPendingJobExecution", resource)
		if !ok {
			s.writeIoTRESTError(w, r, body, requestID, http.StatusForbidden, "UnauthorizedException",
				"Not authorized to start the next job execution.", false, eventID, verified)
			return
		}
		params := jsonBodyMap(body)
		details := iotStringMapParam(params, "statusDetails", "StatusDetails")
		ex, err := s.store.StartNextIoTJobExecution(accountID, region, thingName, details)
		if errors.Is(err, store.ErrIoTNotFound) {
			empty, _ := iotsvc.EmptyJSON()
			s.writeIoTRESTOK(w, empty)
			s.iotMaybeAudit(r, requestID, eventID, verified, "iot-jobs-data.amazonaws.com", "StartNextPendingJobExecution", false)
			return
		}
		if err != nil {
			s.writeIoTRESTError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailureException",
				"Unable to start next job execution.", false, eventID, verified)
			return
		}
		out, _ := iotsvc.DescribeJobExecutionJSON(ex, true)
		s.writeIoTRESTOK(w, out)
		s.iotMaybeAudit(r, requestID, eventID, verified, "iot-jobs-data.amazonaws.com", "StartNextPendingJobExecution", false)
	default:
		s.writeIoTRESTError(w, r, body, requestID, http.StatusMethodNotAllowed, "MethodNotAllowedException",
			"Unsupported Jobs data-plane method.", readOnly, eventID, verified)
	}
}

// iotCredentialsPresentedHost is TLS SNI when the client sent it. Go and curl omit
// SNI for literal IP URLs, so empty SNI may fall back to Host when wantHost is an
// IP or localhost (Compose 127.0.0.1:8443). Hostname CredentialProvider labs still
// require SNI.
func iotCredentialsPresentedHost(r *http.Request, wantHost string) string {
	if r == nil || r.TLS == nil {
		return ""
	}
	sni := strings.TrimSpace(r.TLS.ServerName)
	if sni != "" {
		return sni
	}
	if net.ParseIP(wantHost) == nil && !strings.EqualFold(wantHost, "localhost") {
		return ""
	}
	return iotEndpointHost(r.Host)
}

func (s *Server) handleIoTCredentials(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string, readOnly bool,
) {
	wantHost := iotEndpointHost(s.iotEndpointAddress("iot:CredentialProvider"))
	if r.TLS == nil || !strings.EqualFold(iotCredentialsPresentedHost(r, wantHost), wantHost) {
		s.writeIoTRESTError(w, r, body, requestID, http.StatusForbidden, "ForbiddenException",
			"TLS SNI must match the CredentialProvider endpointAddress.", readOnly, eventID, nil)
		return
	}
	device := s.iotTLSDevice(r)
	if device == nil {
		s.writeIoTRESTError(w, r, body, requestID, http.StatusForbidden, "ForbiddenException",
			"Client certificate is required.", readOnly, eventID, nil)
		return
	}
	thingHdr := strings.TrimSpace(r.Header.Get(iotCredentialsThingHdr))
	if thingHdr == "" || thingHdr != device.ThingName {
		s.writeIoTRESTError(w, r, body, requestID, http.StatusForbidden, "ForbiddenException",
			"x-amzn-iot-thingname must match the certificate thing.", readOnly, eventID, nil)
		return
	}
	alias := iotPathParts(r.URL.Path)[1]
	ra, err := s.store.DescribeIoTRoleAlias(device.AccountID, device.Region, alias)
	if errors.Is(err, store.ErrIoTNotFound) {
		s.writeIoTRESTError(w, r, body, requestID, http.StatusForbidden, "ForbiddenException",
			"Role alias not found.", readOnly, eventID, nil)
		return
	}
	if err != nil {
		s.writeIoTRESTError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailureException",
			"Unable to load role alias.", readOnly, eventID, nil)
		return
	}
	if !s.store.EvaluateIoTDevicePolicy(device.AccountID, device.Region, device.CertificateID,
		catalog.ActionIoTAssumeRoleWithCertificate, ra.RoleAliasARN) {
		s.writeIoTRESTError(w, r, body, requestID, http.StatusForbidden, "ForbiddenException",
			"Not authorized to assume this role alias.", readOnly, eventID, nil)
		return
	}
	accountID, roleName, ok := sts.ParseRoleARN(ra.RoleARN)
	if !ok || accountID != device.AccountID {
		s.writeIoTRESTError(w, r, body, requestID, http.StatusForbidden, "ForbiddenException",
			"Role alias target is invalid.", readOnly, eventID, nil)
		return
	}
	role, err := s.store.GetRoleRecord(accountID, roleName)
	if err != nil {
		s.writeIoTRESTError(w, r, body, requestID, http.StatusForbidden, "ForbiddenException",
			"IAM role not found.", readOnly, eventID, nil)
		return
	}
	trustKeys := map[string]string{
		"aws:SourceAccount": accountID,
		"aws:SourceArn":     ra.RoleAliasARN,
	}
	if !authz.TrustAllowsServiceWithKeys(role.TrustPolicy, authz.ServicePrincipalIoTCredentials, trustKeys) {
		s.writeIoTRESTError(w, r, body, requestID, http.StatusForbidden, "ForbiddenException",
			"Role trust does not allow credentials.iot.amazonaws.com.", readOnly, eventID, nil)
		return
	}
	secret, err := randomSecret()
	if err != nil {
		s.writeIoTRESTError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailureException",
			"Unable to mint credentials.", readOnly, eventID, nil)
		return
	}
	sessionToken, err := randomSecret()
	if err != nil {
		s.writeIoTRESTError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailureException",
			"Unable to mint credentials.", readOnly, eventID, nil)
		return
	}
	dur := ra.CredentialDurationSeconds
	if dur <= 0 {
		dur = 3600
	}
	if role.MaxSessionDuration > 0 && dur > role.MaxSessionDuration {
		dur = role.MaxSessionDuration
	}
	expires := s.tokenExpiresAt(time.Duration(dur) * time.Second)
	accessKeyID, err := s.store.MintTempCredentialsOpts(store.MintTempOpts{
		AccountID:    accountID,
		RoleARN:      ra.RoleARN,
		SessionName:  device.ThingName,
		Secret:       secret,
		SessionToken: sessionToken,
		Expires:      expires,
	})
	if err != nil {
		s.writeIoTRESTError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailureException",
			"Unable to store credentials.", readOnly, eventID, nil)
		return
	}
	out, _ := iotsvc.CredentialsProviderJSON(accessKeyID, secret, sessionToken, expires)
	s.writeIoTRESTOK(w, out)
}

func (s *Server) tryIoTDeviceUnsigned(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string, readOnly bool,
) bool {
	if s.iotTLSDevice(r) == nil {
		return false
	}
	if isIoTDataPlaneRESTPath(r.URL.Path) {
		s.handleIoTREST(w, r, body, requestID, eventID, nil, readOnly)
		return true
	}
	action := iotAction(resolveAction(r, body))
	if isIoTDeviceDeniedControlAction(action) ||
		strings.HasPrefix(action, "iot:") ||
		strings.HasPrefix(action, "iot-data:") ||
		strings.HasPrefix(action, "iot-jobs-data:") {
		s.writeIoTError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"Device certificates cannot call this IoT control-plane action.", readOnly, eventID, nil)
		return true
	}
	return false
}

func iotJobIDIsNext(jobID string) bool {
	decoded, err := url.PathUnescape(jobID)
	if err != nil {
		decoded = jobID
	}
	return decoded == "$next"
}

func (s *Server) writeIoTRESTOK(w http.ResponseWriter, payload []byte) {
	w.Header().Set("Content-Type", iotRESTContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeIoTRESTError(
	w http.ResponseWriter, r *http.Request, body []byte, requestID string, status int, code, message string,
	readOnly bool, eventID string, verified *authn.Verified,
) {
	w.Header().Set("Content-Type", iotRESTContentType)
	w.Header().Set("x-amzn-ErrorType", code)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"message": message})
	_ = body
	s.iotMaybeAudit(r, requestID, eventID, verified, iotEventSource, code, readOnly)
}

func (s *Server) iotMaybeAudit(
	r *http.Request, requestID, eventID string, verified *authn.Verified,
	eventSource, eventName string, readOnly bool,
) {
	if verified == nil {
		return
	}
	s.writeSuccessAudit(r, requestID, eventID, verified, eventSource, eventName, readOnly)
}

func iotStringMapParam(params map[string]any, camel, pascal string) map[string]string {
	raw, ok := params[camel]
	if !ok {
		raw, ok = params[pascal]
	}
	if !ok {
		return map[string]string{}
	}
	return stringMapFromAny(raw)
}

func iotStringSliceParam(params map[string]any, keys ...string) []string {
	var raw any
	ok := false
	for _, k := range keys {
		if raw, ok = params[k]; ok {
			break
		}
	}
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func iotIntParam(params map[string]any, keys ...string) int {
	for _, k := range keys {
		switch v := params[k].(type) {
		case float64:
			return int(v)
		case int:
			return v
		case json.Number:
			n, _ := v.Int64()
			return int(n)
		case string:
			n, _ := strconv.Atoi(strings.TrimSpace(v))
			return n
		}
	}
	return 0
}

func iotStringParam(params map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := params[k].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}
