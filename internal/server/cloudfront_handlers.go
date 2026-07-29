package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	cfsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/cloudfront"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	cloudfrontJSONContentType = "application/x-amz-json-1.1"
	cloudfrontEventSource     = "cloudfront.amazonaws.com"
)

func (s *Server) handleCloudFront(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = cloudfrontAction(action)

	switch action {
	case catalog.ActionCloudFrontCreateDistribution:
		s.cfCreateDistribution(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCloudFrontGetDistribution:
		s.cfGetDistribution(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCloudFrontGetDistributionConfig:
		s.cfGetDistributionConfig(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCloudFrontUpdateDistribution:
		s.cfUpdateDistribution(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCloudFrontListDistributions:
		s.cfListDistributions(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCloudFrontDeleteDistribution:
		s.cfDeleteDistribution(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCloudFrontCreateInvalidation:
		s.cfCreateInvalidation(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCloudFrontGetInvalidation:
		s.cfGetInvalidation(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCloudFrontListInvalidations:
		s.cfListInvalidations(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeCloudFrontError(w, r, body, requestID, http.StatusNotImplemented, "InvalidAction",
			"This CloudFront action is not implemented.", readOnly, eventID, verified)
	}
}

func cloudfrontAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "CreateDistribution":
		return catalog.ActionCloudFrontCreateDistribution
	case "GetDistribution":
		return catalog.ActionCloudFrontGetDistribution
	case "GetDistributionConfig":
		return catalog.ActionCloudFrontGetDistributionConfig
	case "UpdateDistribution":
		return catalog.ActionCloudFrontUpdateDistribution
	case "ListDistributions":
		return catalog.ActionCloudFrontListDistributions
	case "DeleteDistribution":
		return catalog.ActionCloudFrontDeleteDistribution
	case "CreateInvalidation":
		return catalog.ActionCloudFrontCreateInvalidation
	case "GetInvalidation":
		return catalog.ActionCloudFrontGetInvalidation
	case "ListInvalidations":
		return catalog.ActionCloudFrontListInvalidations
	default:
		return action
	}
}

func cloudFrontHasOrigins(params map[string]any) bool {
	cfg, _ := params["DistributionConfig"].(map[string]any)
	if cfg == nil {
		cfg = params
	}
	if originsBlock, ok := cfg["Origins"].(map[string]any); ok {
		if _, ok := originsBlock["Items"]; ok {
			return true
		}
	}
	if _, ok := params["Origins"].([]any); ok {
		return true
	}
	return false
}

func cloudFrontHasBehaviors(params map[string]any) bool {
	cfg, _ := params["DistributionConfig"].(map[string]any)
	if cfg == nil {
		cfg = params
	}
	if _, ok := cfg["DefaultCacheBehavior"].(map[string]any); ok {
		return true
	}
	if behaviorsBlock, ok := cfg["CacheBehaviors"].(map[string]any); ok {
		if _, ok := behaviorsBlock["Items"]; ok {
			return true
		}
	}
	if _, ok := params["CacheBehaviors"].([]any); ok {
		return true
	}
	return false
}

func parseCloudFrontInvalidationPaths(params map[string]any) (caller string, paths []string) {
	batch, _ := params["InvalidationBatch"].(map[string]any)
	if batch == nil {
		batch = params
	}
	caller, _ = batch["CallerReference"].(string)
	pathsBlock, _ := batch["Paths"].(map[string]any)
	rawItems, _ := pathsBlock["Items"].([]any)
	if rawItems == nil {
		if flat, ok := batch["Paths"].([]any); ok {
			rawItems = flat
		}
	}
	for _, item := range rawItems {
		switch v := item.(type) {
		case string:
			paths = append(paths, v)
		case map[string]any:
			if p, ok := v["Path"].(string); ok {
				paths = append(paths, p)
			}
		}
	}
	return caller, paths
}

func cloudFrontIfMatch(r *http.Request, params map[string]any) string {
	if m, ok := params["IfMatch"].(string); ok && strings.TrimSpace(m) != "" {
		return m
	}
	return r.Header.Get("If-Match")
}

func parseCloudFrontOrigins(params map[string]any) []store.CloudFrontOrigin {
	cfg, _ := params["DistributionConfig"].(map[string]any)
	if cfg == nil {
		cfg = params
	}
	originsBlock, _ := cfg["Origins"].(map[string]any)
	rawItems, _ := originsBlock["Items"].([]any)
	if rawItems == nil {
		if flat, ok := params["Origins"].([]any); ok {
			rawItems = flat
		}
	}
	var out []store.CloudFrontOrigin
	for _, item := range rawItems {
		m, _ := item.(map[string]any)
		id, _ := m["Id"].(string)
		domain, _ := m["DomainName"].(string)
		otype, _ := m["OriginType"].(string)
		if otype == "" {
			if _, ok := m["S3OriginConfig"]; ok {
				otype = "s3"
			} else if _, ok := m["CustomOriginConfig"]; ok {
				otype = "apigateway"
			}
		}
		out = append(out, store.CloudFrontOrigin{ID: id, DomainName: domain, OriginType: otype})
	}
	return out
}

func parseCloudFrontBehaviors(params map[string]any) []store.CloudFrontCacheBehavior {
	cfg, _ := params["DistributionConfig"].(map[string]any)
	if cfg == nil {
		cfg = params
	}
	var out []store.CloudFrontCacheBehavior
	if def, ok := cfg["DefaultCacheBehavior"].(map[string]any); ok {
		target, _ := def["TargetOriginId"].(string)
		if strings.TrimSpace(target) != "" {
			out = append(out, store.CloudFrontCacheBehavior{
				PathPattern: "*", TargetOriginId: target,
			})
		}
	}
	behaviorsBlock, _ := cfg["CacheBehaviors"].(map[string]any)
	rawItems, _ := behaviorsBlock["Items"].([]any)
	if rawItems == nil {
		if flat, ok := params["CacheBehaviors"].([]any); ok {
			rawItems = flat
		}
	}
	for _, item := range rawItems {
		m, _ := item.(map[string]any)
		pat, _ := m["PathPattern"].(string)
		target, _ := m["TargetOriginId"].(string)
		if strings.TrimSpace(pat) == "" || strings.TrimSpace(target) == "" {
			continue
		}
		out = append(out, store.CloudFrontCacheBehavior{
			PathPattern: pat, TargetOriginId: target,
		})
	}
	return out
}

func parseCloudFrontLogging(cfg map[string]any) store.CloudFrontLoggingConfig {
	if cfg == nil {
		return store.CloudFrontLoggingConfig{}
	}
	block, _ := cfg["Logging"].(map[string]any)
	if block == nil {
		return store.CloudFrontLoggingConfig{}
	}
	enabled, _ := block["Enabled"].(bool)
	bucket, _ := block["Bucket"].(string)
	prefix, _ := block["Prefix"].(string)
	return store.CloudFrontLoggingConfig{
		Enabled: enabled,
		Bucket:  bucket,
		Prefix:  prefix,
	}
}

func (s *Server) cfCreateDistribution(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionCloudFrontCreateDistribution, "*") {
		s.writeCloudFrontError(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform cloudfront:CreateDistribution.", readOnly, eventID, verified)
		return
	}
	cfg, _ := params["DistributionConfig"].(map[string]any)
	if cfg == nil {
		cfg = params
	}
	caller, _ := cfg["CallerReference"].(string)
	comment, _ := cfg["Comment"].(string)
	enabled := true
	if e, ok := cfg["Enabled"].(bool); ok {
		enabled = e
	}
	d, err := s.store.CreateCloudFrontDistributionWithBehaviors(
		verified.AccountID, comment, caller, enabled,
		parseCloudFrontOrigins(params), parseCloudFrontBehaviors(params),
	)
	if errors.Is(err, store.ErrCloudFrontExists) {
		s.writeCloudFrontError(w, r, body, requestID, http.StatusConflict, "DistributionAlreadyExists",
			"Distribution already exists.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCloudFrontBadRequest) {
		s.writeCloudFrontError(w, r, body, requestID, http.StatusBadRequest, "InvalidArgument",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCloudFrontError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create distribution.", readOnly, eventID, verified)
		return
	}
	if logging := parseCloudFrontLogging(cfg); logging.Enabled || logging.Bucket != "" {
		if err := s.store.SetCloudFrontDistributionLogging(verified.AccountID, d.ID, logging); err != nil {
			if errors.Is(err, store.ErrCloudFrontBadRequest) {
				s.writeCloudFrontError(w, r, body, requestID, http.StatusBadRequest, "InvalidArgument",
					err.Error(), readOnly, eventID, verified)
				return
			}
			s.writeCloudFrontError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to configure distribution logging.", readOnly, eventID, verified)
			return
		}
		d, err = s.store.GetCloudFrontDistribution(verified.AccountID, d.ID)
		if err != nil {
			s.writeCloudFrontError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to load distribution.", readOnly, eventID, verified)
			return
		}
	}
	payload, _ := cfsvc.CreateDistributionJSON(d)
	s.writeCloudFrontOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cloudfrontEventSource, "CreateDistribution", readOnly)
}

func (s *Server) cfGetDistribution(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	id, _ := params["Id"].(string)
	if !s.authorize(verified, catalog.ActionCloudFrontGetDistribution, "*") {
		s.writeCloudFrontError(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform cloudfront:GetDistribution.", readOnly, eventID, verified)
		return
	}
	d, err := s.store.GetCloudFrontDistribution(verified.AccountID, id)
	if errors.Is(err, store.ErrCloudFrontNotFound) {
		s.writeCloudFrontError(w, r, body, requestID, http.StatusNotFound, "NoSuchDistribution",
			"Distribution not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCloudFrontError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to get distribution.", readOnly, eventID, verified)
		return
	}
	payload, _ := cfsvc.GetDistributionJSON(d)
	s.writeCloudFrontOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cloudfrontEventSource, "GetDistribution", readOnly)
}

func (s *Server) cfGetDistributionConfig(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	id, _ := params["Id"].(string)
	if !s.authorize(verified, catalog.ActionCloudFrontGetDistributionConfig, "*") {
		s.writeCloudFrontError(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform cloudfront:GetDistributionConfig.", readOnly, eventID, verified)
		return
	}
	d, err := s.store.GetCloudFrontDistribution(verified.AccountID, id)
	if errors.Is(err, store.ErrCloudFrontNotFound) {
		s.writeCloudFrontError(w, r, body, requestID, http.StatusNotFound, "NoSuchDistribution",
			"Distribution not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCloudFrontError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to get distribution config.", readOnly, eventID, verified)
		return
	}
	payload, _ := cfsvc.GetDistributionConfigJSON(d)
	w.Header().Set("ETag", d.ETag)
	s.writeCloudFrontOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cloudfrontEventSource, "GetDistributionConfig", readOnly)
}

func (s *Server) cfUpdateDistribution(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	id, _ := params["Id"].(string)
	if !s.authorize(verified, catalog.ActionCloudFrontUpdateDistribution, "*") {
		s.writeCloudFrontError(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform cloudfront:UpdateDistribution.", readOnly, eventID, verified)
		return
	}
	cfg, _ := params["DistributionConfig"].(map[string]any)
	if cfg == nil {
		cfg = params
	}
	in := store.UpdateCloudFrontDistributionInput{
		IfMatch:      cloudFrontIfMatch(r, params),
		HasOrigins:   cloudFrontHasOrigins(params),
		HasBehaviors: cloudFrontHasBehaviors(params),
	}
	if in.HasOrigins {
		in.Origins = parseCloudFrontOrigins(params)
	}
	if in.HasBehaviors {
		in.Behaviors = parseCloudFrontBehaviors(params)
	}
	if comment, ok := cfg["Comment"].(string); ok {
		in.Comment = &comment
	}
	if enabled, ok := cfg["Enabled"].(bool); ok {
		in.Enabled = &enabled
	}
	d, err := s.store.UpdateCloudFrontDistribution(verified.AccountID, id, in)
	if errors.Is(err, store.ErrCloudFrontNotFound) {
		s.writeCloudFrontError(w, r, body, requestID, http.StatusNotFound, "NoSuchDistribution",
			"Distribution not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCloudFrontPrecondition) {
		s.writeCloudFrontError(w, r, body, requestID, http.StatusBadRequest, "InvalidIfMatchVersion",
			"The If-Match version is missing or not valid for the resource.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCloudFrontBadRequest) {
		s.writeCloudFrontError(w, r, body, requestID, http.StatusBadRequest, "InvalidArgument",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCloudFrontError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to update distribution.", readOnly, eventID, verified)
		return
	}
	if logging := parseCloudFrontLogging(cfg); logging.Enabled || logging.Bucket != "" {
		if err := s.store.SetCloudFrontDistributionLogging(verified.AccountID, d.ID, logging); err != nil {
			if errors.Is(err, store.ErrCloudFrontBadRequest) {
				s.writeCloudFrontError(w, r, body, requestID, http.StatusBadRequest, "InvalidArgument",
					err.Error(), readOnly, eventID, verified)
				return
			}
			s.writeCloudFrontError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to configure distribution logging.", readOnly, eventID, verified)
			return
		}
		d, err = s.store.GetCloudFrontDistribution(verified.AccountID, d.ID)
		if err != nil {
			s.writeCloudFrontError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to load distribution.", readOnly, eventID, verified)
			return
		}
	}
	payload, _ := cfsvc.UpdateDistributionJSON(d)
	w.Header().Set("ETag", d.ETag)
	s.writeCloudFrontOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cloudfrontEventSource, "UpdateDistribution", readOnly)
}

func (s *Server) cfCreateInvalidation(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	distID, _ := params["DistributionId"].(string)
	if distID == "" {
		distID, _ = params["Id"].(string)
	}
	if !s.authorize(verified, catalog.ActionCloudFrontCreateInvalidation, "*") {
		s.writeCloudFrontError(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform cloudfront:CreateInvalidation.", readOnly, eventID, verified)
		return
	}
	caller, paths := parseCloudFrontInvalidationPaths(params)
	inv, err := s.store.CreateCloudFrontInvalidation(verified.AccountID, distID, caller, paths)
	if errors.Is(err, store.ErrCloudFrontNotFound) {
		s.writeCloudFrontError(w, r, body, requestID, http.StatusNotFound, "NoSuchDistribution",
			"Distribution not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCloudFrontBadRequest) {
		s.writeCloudFrontError(w, r, body, requestID, http.StatusBadRequest, "InvalidArgument",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCloudFrontError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create invalidation.", readOnly, eventID, verified)
		return
	}
	payload, _ := cfsvc.CreateInvalidationJSON(inv)
	s.writeCloudFrontOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cloudfrontEventSource, "CreateInvalidation", readOnly)
}

func (s *Server) cfGetInvalidation(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	distID, _ := params["DistributionId"].(string)
	id, _ := params["Id"].(string)
	if !s.authorize(verified, catalog.ActionCloudFrontGetInvalidation, "*") {
		s.writeCloudFrontError(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform cloudfront:GetInvalidation.", readOnly, eventID, verified)
		return
	}
	inv, err := s.store.GetCloudFrontInvalidation(verified.AccountID, distID, id)
	if errors.Is(err, store.ErrCloudFrontNotFound) {
		s.writeCloudFrontError(w, r, body, requestID, http.StatusNotFound, "NoSuchDistribution",
			"Distribution not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCloudFrontInvalidationNotFound) {
		s.writeCloudFrontError(w, r, body, requestID, http.StatusNotFound, "NoSuchInvalidation",
			"Invalidation not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCloudFrontError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to get invalidation.", readOnly, eventID, verified)
		return
	}
	payload, _ := cfsvc.GetInvalidationJSON(inv)
	s.writeCloudFrontOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cloudfrontEventSource, "GetInvalidation", readOnly)
}

func (s *Server) cfListInvalidations(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	distID, _ := params["DistributionId"].(string)
	if distID == "" {
		distID, _ = params["Id"].(string)
	}
	if !s.authorize(verified, catalog.ActionCloudFrontListInvalidations, "*") {
		s.writeCloudFrontError(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform cloudfront:ListInvalidations.", readOnly, eventID, verified)
		return
	}
	invs, err := s.store.ListCloudFrontInvalidations(verified.AccountID, distID)
	if errors.Is(err, store.ErrCloudFrontNotFound) {
		s.writeCloudFrontError(w, r, body, requestID, http.StatusNotFound, "NoSuchDistribution",
			"Distribution not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCloudFrontError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list invalidations.", readOnly, eventID, verified)
		return
	}
	payload, _ := cfsvc.ListInvalidationsJSON(invs)
	s.writeCloudFrontOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cloudfrontEventSource, "ListInvalidations", readOnly)
}

func (s *Server) cfListDistributions(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	_ = params
	if !s.authorize(verified, catalog.ActionCloudFrontListDistributions, "*") {
		s.writeCloudFrontError(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform cloudfront:ListDistributions.", readOnly, eventID, verified)
		return
	}
	dists, err := s.store.ListCloudFrontDistributions(verified.AccountID)
	if err != nil {
		s.writeCloudFrontError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list distributions.", readOnly, eventID, verified)
		return
	}
	payload, _ := cfsvc.ListDistributionsJSON(dists)
	s.writeCloudFrontOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cloudfrontEventSource, "ListDistributions", readOnly)
}

func (s *Server) cfDeleteDistribution(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	id, _ := params["Id"].(string)
	if !s.authorize(verified, catalog.ActionCloudFrontDeleteDistribution, "*") {
		s.writeCloudFrontError(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform cloudfront:DeleteDistribution.", readOnly, eventID, verified)
		return
	}
	err := s.store.DeleteCloudFrontDistribution(verified.AccountID, id)
	if errors.Is(err, store.ErrCloudFrontNotFound) {
		s.writeCloudFrontError(w, r, body, requestID, http.StatusNotFound, "NoSuchDistribution",
			"Distribution not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCloudFrontError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete distribution.", readOnly, eventID, verified)
		return
	}
	payload, _ := cfsvc.DeleteDistributionJSON()
	s.writeCloudFrontOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, cloudfrontEventSource, "DeleteDistribution", readOnly)
}

func (s *Server) writeCloudFrontOK(w http.ResponseWriter, payload []byte) {
	w.Header().Set("Content-Type", cloudfrontJSONContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeCloudFrontError(
	w http.ResponseWriter, r *http.Request, body []byte, requestID string, status int, code, message string,
	readOnly bool, eventID string, verified *authn.Verified,
) {
	w.Header().Set("Content-Type", cloudfrontJSONContentType)
	w.Header().Set("x-amzn-ErrorType", code)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"__type": code, "message": message})
	_ = body
	s.writeSuccessAudit(r, requestID, eventID, verified, cloudfrontEventSource, code, readOnly)
}
