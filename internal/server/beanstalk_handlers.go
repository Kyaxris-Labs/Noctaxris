package server

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	ebsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/elasticbeanstalk"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const beanstalkEventSource = "elasticbeanstalk.amazonaws.com"

func (s *Server) handleElasticBeanstalk(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := formParams(r, body)
	if action == "" {
		action = params.Get("Action")
		if action == "" {
			action = params.Get("Operation")
		}
	}
	action = beanstalkAction(action)

	switch action {
	case catalog.ActionBeanstalkCreateApplication:
		s.beanstalkCreateApplication(w, r, requestID, eventID, verified, readOnly, params)
	case catalog.ActionBeanstalkDescribeApplications:
		s.beanstalkDescribeApplications(w, r, requestID, eventID, verified, readOnly, params)
	case catalog.ActionBeanstalkDeleteApplication:
		s.beanstalkDeleteApplication(w, r, requestID, eventID, verified, readOnly, params)
	case catalog.ActionBeanstalkCreateApplicationVersion:
		s.beanstalkCreateVersion(w, r, requestID, eventID, verified, readOnly, params)
	case catalog.ActionBeanstalkCreateEnvironment:
		s.beanstalkCreateEnvironment(w, r, requestID, eventID, verified, readOnly, params)
	case catalog.ActionBeanstalkDescribeEnvironments:
		s.beanstalkDescribeEnvironments(w, r, requestID, eventID, verified, readOnly, params)
	case catalog.ActionBeanstalkTerminateEnvironment:
		s.beanstalkTerminateEnvironment(w, r, requestID, eventID, verified, readOnly, params)
	case catalog.ActionBeanstalkListAvailableSolutionStacks:
		s.beanstalkListStacks(w, r, requestID, eventID, verified, readOnly)
	default:
		s.writeBeanstalkError(w, r, requestID, http.StatusBadRequest, "InvalidAction",
			"This Elastic Beanstalk action is not implemented.", readOnly, eventID, verified)
	}
}

func beanstalkAction(action string) string {
	if i := strings.LastIndex(action, ":"); i >= 0 {
		action = action[i+1:]
	}
	switch action {
	case "CreateApplication":
		return catalog.ActionBeanstalkCreateApplication
	case "DescribeApplications":
		return catalog.ActionBeanstalkDescribeApplications
	case "DeleteApplication":
		return catalog.ActionBeanstalkDeleteApplication
	case "CreateApplicationVersion":
		return catalog.ActionBeanstalkCreateApplicationVersion
	case "CreateEnvironment":
		return catalog.ActionBeanstalkCreateEnvironment
	case "DescribeEnvironments":
		return catalog.ActionBeanstalkDescribeEnvironments
	case "TerminateEnvironment":
		return catalog.ActionBeanstalkTerminateEnvironment
	case "ListAvailableSolutionStacks":
		return catalog.ActionBeanstalkListAvailableSolutionStacks
	default:
		return action
	}
}

func (s *Server) beanstalkRegion(verified *authn.Verified) string {
	if verified.Region != "" {
		return verified.Region
	}
	return store.DefaultBeanstalkRegion
}

func (s *Server) beanstalkCreateApplication(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	if !s.authorize(verified, catalog.ActionBeanstalkCreateApplication, "*") {
		s.writeBeanstalkError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform elasticbeanstalk:CreateApplication.", readOnly, eventID, verified)
		return
	}
	app, err := s.store.CreateBeanstalkApplication(verified.AccountID, s.beanstalkRegion(verified),
		params.Get("ApplicationName"), params.Get("Description"))
	if errors.Is(err, store.ErrBeanstalkExists) {
		s.writeBeanstalkError(w, r, requestID, http.StatusBadRequest, "InvalidParameterValue",
			"Application already exists.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrBeanstalkBadRequest) {
		s.writeBeanstalkError(w, r, requestID, http.StatusBadRequest, "ValidationError", err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeBeanstalkError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create application.", readOnly, eventID, verified)
		return
	}
	payload, _ := ebsvc.CreateApplicationXML(app, requestID)
	s.writeBeanstalkOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, beanstalkEventSource, "CreateApplication", readOnly)
}

func (s *Server) beanstalkDescribeApplications(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	if !s.authorize(verified, catalog.ActionBeanstalkDescribeApplications, "*") {
		s.writeBeanstalkError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform elasticbeanstalk:DescribeApplications.", readOnly, eventID, verified)
		return
	}
	apps, err := s.store.DescribeBeanstalkApplications(verified.AccountID, s.beanstalkRegion(verified),
		formMemberList(params, "ApplicationNames"))
	if err != nil {
		s.writeBeanstalkError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe applications.", readOnly, eventID, verified)
		return
	}
	payload, _ := ebsvc.DescribeApplicationsXML(apps, requestID)
	s.writeBeanstalkOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, beanstalkEventSource, "DescribeApplications", readOnly)
}

func (s *Server) beanstalkDeleteApplication(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	if !s.authorize(verified, catalog.ActionBeanstalkDeleteApplication, "*") {
		s.writeBeanstalkError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform elasticbeanstalk:DeleteApplication.", readOnly, eventID, verified)
		return
	}
	force := strings.EqualFold(params.Get("TerminateEnvByForce"), "true")
	err := s.store.DeleteBeanstalkApplication(verified.AccountID, s.beanstalkRegion(verified),
		params.Get("ApplicationName"), force)
	if errors.Is(err, store.ErrBeanstalkNotFound) {
		s.writeBeanstalkError(w, r, requestID, http.StatusBadRequest, "InvalidParameterValue",
			"Application not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrBeanstalkBadRequest) {
		s.writeBeanstalkError(w, r, requestID, http.StatusBadRequest, "ValidationError", err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeBeanstalkError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete application.", readOnly, eventID, verified)
		return
	}
	payload, _ := ebsvc.EmptyOKXML("DeleteApplication", requestID)
	s.writeBeanstalkOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, beanstalkEventSource, "DeleteApplication", readOnly)
}

func (s *Server) beanstalkCreateVersion(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	if !s.authorize(verified, catalog.ActionBeanstalkCreateApplicationVersion, "*") {
		s.writeBeanstalkError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform elasticbeanstalk:CreateApplicationVersion.", readOnly, eventID, verified)
		return
	}
	v, err := s.store.CreateBeanstalkApplicationVersion(
		verified.AccountID, s.beanstalkRegion(verified),
		params.Get("ApplicationName"), params.Get("VersionLabel"), params.Get("Description"),
		params.Get("SourceBundle.S3Bucket"), params.Get("SourceBundle.S3Key"),
	)
	if errors.Is(err, store.ErrBeanstalkNotFound) || errors.Is(err, store.ErrBeanstalkExists) || errors.Is(err, store.ErrBeanstalkBadRequest) {
		s.writeBeanstalkError(w, r, requestID, http.StatusBadRequest, "InvalidParameterValue", err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeBeanstalkError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create application version.", readOnly, eventID, verified)
		return
	}
	payload, _ := ebsvc.CreateApplicationVersionXML(v, requestID)
	s.writeBeanstalkOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, beanstalkEventSource, "CreateApplicationVersion", readOnly)
}

func (s *Server) beanstalkCreateEnvironment(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	if !s.authorize(verified, catalog.ActionBeanstalkCreateEnvironment, "*") {
		s.writeBeanstalkError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform elasticbeanstalk:CreateEnvironment.", readOnly, eventID, verified)
		return
	}
	env, err := s.store.CreateBeanstalkEnvironment(
		verified.AccountID, s.beanstalkRegion(verified),
		params.Get("ApplicationName"), params.Get("EnvironmentName"), params.Get("VersionLabel"),
		params.Get("SolutionStackName"), params.Get("Description"), params.Get("CNAMEPrefix"),
	)
	if errors.Is(err, store.ErrBeanstalkNotFound) || errors.Is(err, store.ErrBeanstalkExists) || errors.Is(err, store.ErrBeanstalkBadRequest) {
		s.writeBeanstalkError(w, r, requestID, http.StatusBadRequest, "InvalidParameterValue", err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeBeanstalkError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create environment.", readOnly, eventID, verified)
		return
	}
	payload, _ := ebsvc.CreateEnvironmentXML(env, requestID)
	s.writeBeanstalkOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, beanstalkEventSource, "CreateEnvironment", readOnly)
}

func (s *Server) beanstalkDescribeEnvironments(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	if !s.authorize(verified, catalog.ActionBeanstalkDescribeEnvironments, "*") {
		s.writeBeanstalkError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform elasticbeanstalk:DescribeEnvironments.", readOnly, eventID, verified)
		return
	}
	includeDeleted := strings.EqualFold(params.Get("IncludeDeleted"), "true")
	envs, err := s.store.DescribeBeanstalkEnvironments(
		verified.AccountID, s.beanstalkRegion(verified),
		params.Get("ApplicationName"), formMemberList(params, "EnvironmentNames"), includeDeleted,
	)
	if err != nil {
		s.writeBeanstalkError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe environments.", readOnly, eventID, verified)
		return
	}
	payload, _ := ebsvc.DescribeEnvironmentsXML(envs, requestID)
	s.writeBeanstalkOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, beanstalkEventSource, "DescribeEnvironments", readOnly)
}

func (s *Server) beanstalkTerminateEnvironment(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	if !s.authorize(verified, catalog.ActionBeanstalkTerminateEnvironment, "*") {
		s.writeBeanstalkError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform elasticbeanstalk:TerminateEnvironment.", readOnly, eventID, verified)
		return
	}
	env, err := s.store.TerminateBeanstalkEnvironment(
		verified.AccountID, s.beanstalkRegion(verified),
		params.Get("EnvironmentName"), params.Get("EnvironmentId"),
	)
	if errors.Is(err, store.ErrBeanstalkNotFound) {
		s.writeBeanstalkError(w, r, requestID, http.StatusBadRequest, "InvalidParameterValue",
			"Environment not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeBeanstalkError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to terminate environment.", readOnly, eventID, verified)
		return
	}
	payload, _ := ebsvc.TerminateEnvironmentResponseXML(env, requestID)
	s.writeBeanstalkOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, beanstalkEventSource, "TerminateEnvironment", readOnly)
}

func (s *Server) beanstalkListStacks(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool,
) {
	if !s.authorize(verified, catalog.ActionBeanstalkListAvailableSolutionStacks, "*") {
		s.writeBeanstalkError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform elasticbeanstalk:ListAvailableSolutionStacks.", readOnly, eventID, verified)
		return
	}
	payload, _ := ebsvc.ListAvailableSolutionStacksXML(store.BeanstalkSolutionStacks(), requestID)
	s.writeBeanstalkOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, beanstalkEventSource, "ListAvailableSolutionStacks", readOnly)
}

func (s *Server) writeBeanstalkOK(w http.ResponseWriter, payload []byte) {
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeBeanstalkError(
	w http.ResponseWriter, r *http.Request, requestID string, status int, code, message string,
	readOnly bool, eventID string, verified *authn.Verified,
) {
	payload := []byte(`<?xml version="1.0" encoding="UTF-8"?>` +
		`<ErrorResponse xmlns="http://elasticbeanstalk.amazonaws.com/docs/2010-12-01/"><Error><Type>Sender</Type><Code>` +
		xmlEscape(code) + `</Code><Message>` + xmlEscape(message) +
		`</Message></Error><RequestId>` + xmlEscape(requestID) + `</RequestId></ErrorResponse>`)
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, beanstalkEventSource, code, readOnly)
}
