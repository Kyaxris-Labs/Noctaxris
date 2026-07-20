package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	appconfigsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/appconfig"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	appconfigJSONContentType = "application/x-amz-json-1.1"
	appconfigEventSource     = "appconfig.amazonaws.com"
)

func (s *Server) handleAppConfig(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = appconfigAction(action)

	switch action {
	case catalog.ActionAppConfigCreateApplication:
		s.appconfigCreateApplication(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAppConfigCreateEnvironment:
		s.appconfigCreateEnvironment(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAppConfigCreateConfigurationProfile:
		s.appconfigCreateProfile(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAppConfigCreateHostedConfigurationVersion:
		s.appconfigCreateHostedVersion(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAppConfigGetConfiguration:
		s.appconfigGetConfiguration(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAppConfigDataStartConfigurationSession:
		s.appconfigStartSession(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionAppConfigDataGetLatestConfiguration:
		s.appconfigGetLatest(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeAppConfigError(w, r, body, requestID, http.StatusNotImplemented, "InternalFailure",
			"This AppConfig action is not implemented.", readOnly, eventID, verified)
	}
}

func appconfigAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "CreateApplication":
		return catalog.ActionAppConfigCreateApplication
	case "CreateEnvironment":
		return catalog.ActionAppConfigCreateEnvironment
	case "CreateConfigurationProfile":
		return catalog.ActionAppConfigCreateConfigurationProfile
	case "CreateHostedConfigurationVersion":
		return catalog.ActionAppConfigCreateHostedConfigurationVersion
	case "GetConfiguration":
		return catalog.ActionAppConfigGetConfiguration
	case "StartConfigurationSession":
		return catalog.ActionAppConfigDataStartConfigurationSession
	case "GetLatestConfiguration":
		return catalog.ActionAppConfigDataGetLatestConfiguration
	default:
		return action
	}
}

func (s *Server) appconfigCreateApplication(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name, _ := params["Name"].(string)
	desc, _ := params["Description"].(string)
	if strings.TrimSpace(name) == "" {
		s.writeAppConfigError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			"Name is required.", readOnly, eventID, verified)
		return
	}
	if !s.authorize(verified, catalog.ActionAppConfigCreateApplication, "*") {
		s.writeAppConfigError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform appconfig:CreateApplication.", readOnly, eventID, verified)
		return
	}
	app, err := s.store.CreateAppConfigApplication(verified.AccountID, name, desc)
	if errors.Is(err, store.ErrAppConfigAlreadyExists) {
		s.writeAppConfigError(w, r, body, requestID, http.StatusConflict, "ConflictException",
			"Application already exists.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAppConfigError(w, r, body, requestID, http.StatusInternalServerError, "InternalServerException",
			"Unable to create application.", readOnly, eventID, verified)
		return
	}
	payload, _ := appconfigsvc.CreateApplicationJSON(app)
	s.writeAppConfigOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, appconfigEventSource, "CreateApplication", readOnly)
}

func (s *Server) appconfigCreateEnvironment(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	appID, _ := params["ApplicationId"].(string)
	name, _ := params["Name"].(string)
	desc, _ := params["Description"].(string)
	if strings.TrimSpace(appID) == "" || strings.TrimSpace(name) == "" {
		s.writeAppConfigError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			"ApplicationId and Name are required.", readOnly, eventID, verified)
		return
	}
	if !s.authorize(verified, catalog.ActionAppConfigCreateEnvironment, "*") {
		s.writeAppConfigError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform appconfig:CreateEnvironment.", readOnly, eventID, verified)
		return
	}
	env, err := s.store.CreateAppConfigEnvironment(verified.AccountID, appID, name, desc)
	if errors.Is(err, store.ErrAppConfigNotFound) {
		s.writeAppConfigError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Application not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAppConfigError(w, r, body, requestID, http.StatusInternalServerError, "InternalServerException",
			"Unable to create environment.", readOnly, eventID, verified)
		return
	}
	payload, _ := appconfigsvc.CreateEnvironmentJSON(env)
	s.writeAppConfigOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, appconfigEventSource, "CreateEnvironment", readOnly)
}

func (s *Server) appconfigCreateProfile(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	appID, _ := params["ApplicationId"].(string)
	name, _ := params["Name"].(string)
	desc, _ := params["Description"].(string)
	if strings.TrimSpace(appID) == "" || strings.TrimSpace(name) == "" {
		s.writeAppConfigError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			"ApplicationId and Name are required.", readOnly, eventID, verified)
		return
	}
	if !s.authorize(verified, catalog.ActionAppConfigCreateConfigurationProfile, "*") {
		s.writeAppConfigError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform appconfig:CreateConfigurationProfile.", readOnly, eventID, verified)
		return
	}
	p, err := s.store.CreateAppConfigProfile(verified.AccountID, appID, name, desc)
	if errors.Is(err, store.ErrAppConfigNotFound) {
		s.writeAppConfigError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Application not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAppConfigError(w, r, body, requestID, http.StatusInternalServerError, "InternalServerException",
			"Unable to create configuration profile.", readOnly, eventID, verified)
		return
	}
	payload, _ := appconfigsvc.CreateConfigurationProfileJSON(p)
	s.writeAppConfigOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, appconfigEventSource, "CreateConfigurationProfile", readOnly)
}

func (s *Server) appconfigCreateHostedVersion(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	appID, _ := params["ApplicationId"].(string)
	profileID, _ := params["ConfigurationProfileId"].(string)
	contentType, _ := params["ContentType"].(string)
	if strings.TrimSpace(appID) == "" || strings.TrimSpace(profileID) == "" {
		s.writeAppConfigError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			"ApplicationId and ConfigurationProfileId are required.", readOnly, eventID, verified)
		return
	}
	if !s.authorize(verified, catalog.ActionAppConfigCreateHostedConfigurationVersion, "*") {
		s.writeAppConfigError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform appconfig:CreateHostedConfigurationVersion.", readOnly, eventID, verified)
		return
	}
	content, err := store.DecodeKinesisData(params["Content"])
	if err != nil {
		s.writeAppConfigError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			"Content must be base64.", readOnly, eventID, verified)
		return
	}
	v, err := s.store.CreateAppConfigHostedVersion(verified.AccountID, appID, profileID, contentType, content)
	if errors.Is(err, store.ErrAppConfigNotFound) {
		s.writeAppConfigError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Profile not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAppConfigError(w, r, body, requestID, http.StatusInternalServerError, "InternalServerException",
			"Unable to create hosted configuration version.", readOnly, eventID, verified)
		return
	}
	payload, _ := appconfigsvc.CreateHostedConfigurationVersionJSON(v)
	s.writeAppConfigOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, appconfigEventSource, "CreateHostedConfigurationVersion", readOnly)
}

func (s *Server) appconfigGetConfiguration(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	appID, _ := params["Application"].(string)
	if appID == "" {
		appID, _ = params["ApplicationId"].(string)
	}
	envID, _ := params["Environment"].(string)
	if envID == "" {
		envID, _ = params["EnvironmentId"].(string)
	}
	profileID, _ := params["Configuration"].(string)
	if profileID == "" {
		profileID, _ = params["ConfigurationProfileId"].(string)
	}
	if appID == "" || envID == "" || profileID == "" {
		s.writeAppConfigError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			"Application, Environment, and Configuration are required.", readOnly, eventID, verified)
		return
	}
	if !s.authorize(verified, catalog.ActionAppConfigGetConfiguration, "*") {
		s.writeAppConfigError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform appconfig:GetConfiguration.", readOnly, eventID, verified)
		return
	}
	v, err := s.store.GetAppConfigConfiguration(verified.AccountID, appID, envID, profileID)
	if errors.Is(err, store.ErrAppConfigNotFound) {
		s.writeAppConfigError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Configuration not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAppConfigError(w, r, body, requestID, http.StatusInternalServerError, "InternalServerException",
			"Unable to get configuration.", readOnly, eventID, verified)
		return
	}
	payload, _ := appconfigsvc.GetConfigurationJSON(v)
	s.writeAppConfigOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, appconfigEventSource, "GetConfiguration", readOnly)
}

func (s *Server) appconfigStartSession(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	appID, _ := params["ApplicationIdentifier"].(string)
	envID, _ := params["EnvironmentIdentifier"].(string)
	profileID, _ := params["ConfigurationProfileIdentifier"].(string)
	if appID == "" || envID == "" || profileID == "" {
		s.writeAppConfigError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			"ApplicationIdentifier, EnvironmentIdentifier, and ConfigurationProfileIdentifier are required.", readOnly, eventID, verified)
		return
	}
	if !s.authorize(verified, catalog.ActionAppConfigDataStartConfigurationSession, "*") {
		s.writeAppConfigError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform appconfigdata:StartConfigurationSession.", readOnly, eventID, verified)
		return
	}
	token, err := s.store.StartAppConfigSession(verified.AccountID, appID, envID, profileID)
	if errors.Is(err, store.ErrAppConfigNotFound) {
		s.writeAppConfigError(w, r, body, requestID, http.StatusNotFound, "ResourceNotFoundException",
			"Resource not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAppConfigError(w, r, body, requestID, http.StatusInternalServerError, "InternalServerException",
			"Unable to start configuration session.", readOnly, eventID, verified)
		return
	}
	payload, _ := appconfigsvc.StartConfigurationSessionJSON(token)
	s.writeAppConfigOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, "appconfigdata.amazonaws.com", "StartConfigurationSession", readOnly)
}

func (s *Server) appconfigGetLatest(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	token, _ := params["ConfigurationToken"].(string)
	if token == "" {
		s.writeAppConfigError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			"ConfigurationToken is required.", readOnly, eventID, verified)
		return
	}
	if !s.authorize(verified, catalog.ActionAppConfigDataGetLatestConfiguration, "*") {
		s.writeAppConfigError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform appconfigdata:GetLatestConfiguration.", readOnly, eventID, verified)
		return
	}
	content, contentType, nextToken, version, err := s.store.GetLatestAppConfigConfiguration(token)
	if errors.Is(err, store.ErrAppConfigNotFound) {
		s.writeAppConfigError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			"Invalid configuration token.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeAppConfigError(w, r, body, requestID, http.StatusInternalServerError, "InternalServerException",
			"Unable to get latest configuration.", readOnly, eventID, verified)
		return
	}
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", contentType)
	if contentType == "" {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	w.Header().Set("Next-Token", nextToken)
	w.Header().Set("Version-Label", version)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
	s.writeSuccessAudit(r, requestID, eventID, verified, "appconfigdata.amazonaws.com", "GetLatestConfiguration", readOnly)
}

func (s *Server) writeAppConfigOK(w http.ResponseWriter, requestID string, payload []byte) {
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", appconfigJSONContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeAppConfigError(
	w http.ResponseWriter, r *http.Request, body []byte, requestID string,
	status int, code, message string, readOnly bool, eventID string, verified *authn.Verified,
) {
	_ = body
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", appconfigJSONContentType)
	w.WriteHeader(status)
	payload, _ := json.Marshal(map[string]string{"__type": code, "Message": message})
	_, _ = w.Write(payload)
	accessKeyID := ""
	accountID := ""
	if verified != nil {
		accessKeyID = verified.AccessKeyID
		accountID = verified.AccountID
	}
	s.auditAPIError(r, requestID, eventID, code, message, readOnly, accessKeyID, accountID, verified != nil)
}
