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
	case catalog.ActionCloudFrontListDistributions:
		s.cfListDistributions(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCloudFrontDeleteDistribution:
		s.cfDeleteDistribution(w, r, body, requestID, eventID, verified, readOnly, params)
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
	case "ListDistributions":
		return catalog.ActionCloudFrontListDistributions
	case "DeleteDistribution":
		return catalog.ActionCloudFrontDeleteDistribution
	default:
		return action
	}
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
