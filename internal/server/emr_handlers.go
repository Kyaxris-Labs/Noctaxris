package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	emrsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/emr"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	emrJSONContentType = "application/x-amz-json-1.1"
	emrEventSource     = "elasticmapreduce.amazonaws.com"
)

func (s *Server) handleEMR(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = emrAction(action)

	switch action {
	case catalog.ActionEMRRunJobFlow:
		s.emrRunJobFlow(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionEMRDescribeCluster:
		s.emrDescribe(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionEMRListClusters:
		s.emrList(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionEMRTerminateJobFlows:
		s.emrTerminate(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionEMRAddJobFlowSteps:
		s.emrAddJobFlowSteps(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionEMRDescribeStep:
		s.emrDescribeStep(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionEMRListSteps:
		s.emrListSteps(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionEMRCancelSteps:
		s.emrCancelSteps(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionEMRListInstanceGroups:
		s.emrListInstanceGroups(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionEMRListInstanceFleets:
		s.emrListInstanceFleets(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionEMRAddTags:
		s.emrAddTags(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionEMRRemoveTags:
		s.emrRemoveTags(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionEMRCreateSecurityConfiguration:
		s.emrCreateSecurityConfiguration(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionEMRDescribeSecurityConfiguration:
		s.emrDescribeSecurityConfiguration(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionEMRDeleteSecurityConfiguration:
		s.emrDeleteSecurityConfiguration(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionEMRListSecurityConfigurations:
		s.emrListSecurityConfigurations(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeEMRError(w, requestID, http.StatusNotImplemented, "InternalFailure",
			"This EMR action is not implemented.")
	}
}

func emrAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "RunJobFlow":
		return catalog.ActionEMRRunJobFlow
	case "DescribeCluster":
		return catalog.ActionEMRDescribeCluster
	case "ListClusters":
		return catalog.ActionEMRListClusters
	case "TerminateJobFlows":
		return catalog.ActionEMRTerminateJobFlows
	case "AddJobFlowSteps":
		return catalog.ActionEMRAddJobFlowSteps
	case "DescribeStep":
		return catalog.ActionEMRDescribeStep
	case "ListSteps":
		return catalog.ActionEMRListSteps
	case "CancelSteps":
		return catalog.ActionEMRCancelSteps
	case "ListInstanceGroups":
		return catalog.ActionEMRListInstanceGroups
	case "ListInstanceFleets":
		return catalog.ActionEMRListInstanceFleets
	case "AddTags":
		return catalog.ActionEMRAddTags
	case "RemoveTags":
		return catalog.ActionEMRRemoveTags
	case "CreateSecurityConfiguration":
		return catalog.ActionEMRCreateSecurityConfiguration
	case "DescribeSecurityConfiguration":
		return catalog.ActionEMRDescribeSecurityConfiguration
	case "DeleteSecurityConfiguration":
		return catalog.ActionEMRDeleteSecurityConfiguration
	case "ListSecurityConfigurations":
		return catalog.ActionEMRListSecurityConfigurations
	default:
		return action
	}
}

func (s *Server) emrRegion(verified *authn.Verified) string {
	if verified.Region != "" {
		return verified.Region
	}
	return store.DefaultEMRRegion
}

func (s *Server) emrRunJobFlow(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionEMRRunJobFlow, "*") {
		s.writeEMRError(w, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform elasticmapreduce:RunJobFlow.")
		return
	}
	in := store.EMRRunJobFlowInput{
		Name:         stringField(params, "Name"),
		ReleaseLabel: stringField(params, "ReleaseLabel"),
		LogURI:       stringField(params, "LogUri"),
		Tags:         parseEMRTags(params["Tags"]),
	}
	if instances, ok := params["Instances"].(map[string]any); ok {
		in.Groups = parseEMRInstanceGroups(instances["InstanceGroups"])
		in.Fleets = parseEMRInstanceFleets(instances["InstanceFleets"])
	}
	c, err := s.store.RunEMRJobFlow(verified.AccountID, s.emrRegion(verified), in)
	if errors.Is(err, store.ErrEMRValidation) {
		s.writeEMRError(w, requestID, http.StatusBadRequest, "ValidationException", err.Error())
		return
	}
	if err != nil {
		s.writeEMRError(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to run job flow.")
		return
	}
	payload, _ := emrsvc.RunJobFlowJSON(c)
	s.writeEMROK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, emrEventSource, "RunJobFlow", readOnly)
}

func (s *Server) emrDescribe(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionEMRDescribeCluster, "*") {
		s.writeEMRError(w, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform elasticmapreduce:DescribeCluster.")
		return
	}
	id, _ := params["ClusterId"].(string)
	c, err := s.store.DescribeEMRCluster(verified.AccountID, id)
	if errors.Is(err, store.ErrEMRNotFound) {
		s.writeEMRError(w, requestID, http.StatusBadRequest, "InvalidRequestException",
			"Cluster not found.")
		return
	}
	if err != nil {
		s.writeEMRError(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe cluster.")
		return
	}
	payload, _ := emrsvc.DescribeClusterJSON(c)
	s.writeEMROK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, emrEventSource, "DescribeCluster", readOnly)
}

func (s *Server) emrList(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	_ = params
	if !s.authorize(verified, catalog.ActionEMRListClusters, "*") {
		s.writeEMRError(w, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform elasticmapreduce:ListClusters.")
		return
	}
	clusters, err := s.store.ListEMRClusters(verified.AccountID)
	if err != nil {
		s.writeEMRError(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list clusters.")
		return
	}
	payload, _ := emrsvc.ListClustersJSON(clusters)
	s.writeEMROK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, emrEventSource, "ListClusters", readOnly)
}

func (s *Server) emrTerminate(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionEMRTerminateJobFlows, "*") {
		s.writeEMRError(w, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform elasticmapreduce:TerminateJobFlows.")
		return
	}
	var ids []string
	if raw, ok := params["JobFlowIds"].([]any); ok {
		for _, v := range raw {
			if s, ok := v.(string); ok {
				ids = append(ids, s)
			}
		}
	}
	err := s.store.TerminateEMRJobFlows(verified.AccountID, ids)
	if errors.Is(err, store.ErrEMRValidation) {
		s.writeEMRError(w, requestID, http.StatusBadRequest, "ValidationException", err.Error())
		return
	}
	if errors.Is(err, store.ErrEMRNotFound) {
		s.writeEMRError(w, requestID, http.StatusBadRequest, "InvalidRequestException", err.Error())
		return
	}
	if err != nil {
		s.writeEMRError(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to terminate job flows.")
		return
	}
	w.Header().Set("Content-Type", emrJSONContentType)
	w.Header().Set("x-amzn-RequestId", requestID)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{}`))
	s.writeSuccessAudit(r, requestID, eventID, verified, emrEventSource, "TerminateJobFlows", readOnly)
}

func (s *Server) emrAddJobFlowSteps(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionEMRAddJobFlowSteps, "*") {
		s.writeEMRError(w, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform elasticmapreduce:AddJobFlowSteps.")
		return
	}
	jobFlowID, _ := params["JobFlowId"].(string)
	inputs := parseEMRStepInputs(params["Steps"])
	ids, err := s.store.AddEMRJobFlowSteps(verified.AccountID, jobFlowID, inputs)
	if errors.Is(err, store.ErrEMRValidation) {
		s.writeEMRError(w, requestID, http.StatusBadRequest, "ValidationException", err.Error())
		return
	}
	if errors.Is(err, store.ErrEMRNotFound) {
		s.writeEMRError(w, requestID, http.StatusBadRequest, "InvalidRequestException", err.Error())
		return
	}
	if err != nil {
		s.writeEMRError(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to add job flow steps.")
		return
	}
	payload, _ := emrsvc.AddJobFlowStepsJSON(ids)
	s.writeEMROK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, emrEventSource, "AddJobFlowSteps", readOnly)
}

func (s *Server) emrDescribeStep(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionEMRDescribeStep, "*") {
		s.writeEMRError(w, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform elasticmapreduce:DescribeStep.")
		return
	}
	clusterID, _ := params["ClusterId"].(string)
	stepID, _ := params["StepId"].(string)
	st, err := s.store.DescribeEMRStep(verified.AccountID, clusterID, stepID)
	if errors.Is(err, store.ErrEMRValidation) {
		s.writeEMRError(w, requestID, http.StatusBadRequest, "ValidationException", err.Error())
		return
	}
	if errors.Is(err, store.ErrEMRNotFound) {
		s.writeEMRError(w, requestID, http.StatusBadRequest, "InvalidRequestException", err.Error())
		return
	}
	if err != nil {
		s.writeEMRError(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe step.")
		return
	}
	payload, _ := emrsvc.DescribeStepJSON(st)
	s.writeEMROK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, emrEventSource, "DescribeStep", readOnly)
}

func (s *Server) emrListSteps(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionEMRListSteps, "*") {
		s.writeEMRError(w, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform elasticmapreduce:ListSteps.")
		return
	}
	clusterID, _ := params["ClusterId"].(string)
	steps, err := s.store.ListEMRSteps(verified.AccountID, clusterID, emrStringList(params["StepStates"]), emrStringList(params["StepIds"]))
	if errors.Is(err, store.ErrEMRValidation) {
		s.writeEMRError(w, requestID, http.StatusBadRequest, "ValidationException", err.Error())
		return
	}
	if errors.Is(err, store.ErrEMRNotFound) {
		s.writeEMRError(w, requestID, http.StatusBadRequest, "InvalidRequestException", err.Error())
		return
	}
	if err != nil {
		s.writeEMRError(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list steps.")
		return
	}
	payload, _ := emrsvc.ListStepsJSON(steps)
	s.writeEMROK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, emrEventSource, "ListSteps", readOnly)
}

func (s *Server) emrCancelSteps(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionEMRCancelSteps, "*") {
		s.writeEMRError(w, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform elasticmapreduce:CancelSteps.")
		return
	}
	clusterID, _ := params["ClusterId"].(string)
	infos, err := s.store.CancelEMRSteps(verified.AccountID, clusterID, emrStringList(params["StepIds"]))
	if errors.Is(err, store.ErrEMRValidation) {
		s.writeEMRError(w, requestID, http.StatusBadRequest, "ValidationException", err.Error())
		return
	}
	if errors.Is(err, store.ErrEMRNotFound) {
		s.writeEMRError(w, requestID, http.StatusBadRequest, "InvalidRequestException", err.Error())
		return
	}
	if err != nil {
		s.writeEMRError(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to cancel steps.")
		return
	}
	payload, _ := emrsvc.CancelStepsJSON(infos)
	s.writeEMROK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, emrEventSource, "CancelSteps", readOnly)
}

func (s *Server) emrListInstanceGroups(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionEMRListInstanceGroups, "*") {
		s.writeEMRError(w, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform elasticmapreduce:ListInstanceGroups.")
		return
	}
	clusterID, _ := params["ClusterId"].(string)
	groups, err := s.store.ListEMRInstanceGroups(verified.AccountID, clusterID)
	if errors.Is(err, store.ErrEMRValidation) {
		s.writeEMRError(w, requestID, http.StatusBadRequest, "ValidationException", err.Error())
		return
	}
	if errors.Is(err, store.ErrEMRNotFound) {
		s.writeEMRError(w, requestID, http.StatusBadRequest, "InvalidRequestException", err.Error())
		return
	}
	if err != nil {
		s.writeEMRError(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list instance groups.")
		return
	}
	payload, _ := emrsvc.ListInstanceGroupsJSON(groups)
	s.writeEMROK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, emrEventSource, "ListInstanceGroups", readOnly)
}

func (s *Server) emrListInstanceFleets(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionEMRListInstanceFleets, "*") {
		s.writeEMRError(w, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform elasticmapreduce:ListInstanceFleets.")
		return
	}
	clusterID, _ := params["ClusterId"].(string)
	fleets, err := s.store.ListEMRInstanceFleets(verified.AccountID, clusterID)
	if errors.Is(err, store.ErrEMRValidation) {
		s.writeEMRError(w, requestID, http.StatusBadRequest, "ValidationException", err.Error())
		return
	}
	if errors.Is(err, store.ErrEMRNotFound) {
		s.writeEMRError(w, requestID, http.StatusBadRequest, "InvalidRequestException", err.Error())
		return
	}
	if err != nil {
		s.writeEMRError(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list instance fleets.")
		return
	}
	payload, _ := emrsvc.ListInstanceFleetsJSON(fleets)
	s.writeEMROK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, emrEventSource, "ListInstanceFleets", readOnly)
}

func (s *Server) emrAddTags(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionEMRAddTags, "*") {
		s.writeEMRError(w, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform elasticmapreduce:AddTags.")
		return
	}
	resourceID, _ := params["ResourceId"].(string)
	err := s.store.AddEMRTags(verified.AccountID, resourceID, parseEMRTags(params["Tags"]))
	if errors.Is(err, store.ErrEMRValidation) {
		s.writeEMRError(w, requestID, http.StatusBadRequest, "ValidationException", err.Error())
		return
	}
	if errors.Is(err, store.ErrEMRNotFound) {
		s.writeEMRError(w, requestID, http.StatusBadRequest, "InvalidRequestException", err.Error())
		return
	}
	if err != nil {
		s.writeEMRError(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to add tags.")
		return
	}
	w.Header().Set("Content-Type", emrJSONContentType)
	w.Header().Set("x-amzn-RequestId", requestID)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{}`))
	s.writeSuccessAudit(r, requestID, eventID, verified, emrEventSource, "AddTags", readOnly)
}

func (s *Server) emrRemoveTags(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionEMRRemoveTags, "*") {
		s.writeEMRError(w, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform elasticmapreduce:RemoveTags.")
		return
	}
	resourceID, _ := params["ResourceId"].(string)
	err := s.store.RemoveEMRTags(verified.AccountID, resourceID, emrStringList(params["TagKeys"]))
	if errors.Is(err, store.ErrEMRValidation) {
		s.writeEMRError(w, requestID, http.StatusBadRequest, "ValidationException", err.Error())
		return
	}
	if errors.Is(err, store.ErrEMRNotFound) {
		s.writeEMRError(w, requestID, http.StatusBadRequest, "InvalidRequestException", err.Error())
		return
	}
	if err != nil {
		s.writeEMRError(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to remove tags.")
		return
	}
	w.Header().Set("Content-Type", emrJSONContentType)
	w.Header().Set("x-amzn-RequestId", requestID)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{}`))
	s.writeSuccessAudit(r, requestID, eventID, verified, emrEventSource, "RemoveTags", readOnly)
}

func (s *Server) emrCreateSecurityConfiguration(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionEMRCreateSecurityConfiguration, "*") {
		s.writeEMRError(w, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform elasticmapreduce:CreateSecurityConfiguration.")
		return
	}
	name, _ := params["Name"].(string)
	secCfg, _ := params["SecurityConfiguration"].(string)
	sc, err := s.store.CreateEMRSecurityConfiguration(verified.AccountID, name, secCfg)
	if errors.Is(err, store.ErrEMRValidation) {
		s.writeEMRError(w, requestID, http.StatusBadRequest, "ValidationException", err.Error())
		return
	}
	if errors.Is(err, store.ErrEMRNotFound) {
		s.writeEMRError(w, requestID, http.StatusBadRequest, "InvalidRequestException", err.Error())
		return
	}
	if err != nil {
		s.writeEMRError(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create security configuration.")
		return
	}
	payload, _ := emrsvc.CreateSecurityConfigurationJSON(sc)
	s.writeEMROK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, emrEventSource, "CreateSecurityConfiguration", readOnly)
}

func (s *Server) emrDescribeSecurityConfiguration(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionEMRDescribeSecurityConfiguration, "*") {
		s.writeEMRError(w, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform elasticmapreduce:DescribeSecurityConfiguration.")
		return
	}
	name, _ := params["Name"].(string)
	sc, err := s.store.DescribeEMRSecurityConfiguration(verified.AccountID, name)
	if errors.Is(err, store.ErrEMRValidation) {
		s.writeEMRError(w, requestID, http.StatusBadRequest, "ValidationException", err.Error())
		return
	}
	if errors.Is(err, store.ErrEMRNotFound) {
		s.writeEMRError(w, requestID, http.StatusBadRequest, "InvalidRequestException", err.Error())
		return
	}
	if err != nil {
		s.writeEMRError(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe security configuration.")
		return
	}
	payload, _ := emrsvc.DescribeSecurityConfigurationJSON(sc)
	s.writeEMROK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, emrEventSource, "DescribeSecurityConfiguration", readOnly)
}

func (s *Server) emrDeleteSecurityConfiguration(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionEMRDeleteSecurityConfiguration, "*") {
		s.writeEMRError(w, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform elasticmapreduce:DeleteSecurityConfiguration.")
		return
	}
	name, _ := params["Name"].(string)
	err := s.store.DeleteEMRSecurityConfiguration(verified.AccountID, name)
	if errors.Is(err, store.ErrEMRValidation) {
		s.writeEMRError(w, requestID, http.StatusBadRequest, "ValidationException", err.Error())
		return
	}
	if errors.Is(err, store.ErrEMRNotFound) {
		s.writeEMRError(w, requestID, http.StatusBadRequest, "InvalidRequestException", err.Error())
		return
	}
	if err != nil {
		s.writeEMRError(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete security configuration.")
		return
	}
	w.Header().Set("Content-Type", emrJSONContentType)
	w.Header().Set("x-amzn-RequestId", requestID)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{}`))
	s.writeSuccessAudit(r, requestID, eventID, verified, emrEventSource, "DeleteSecurityConfiguration", readOnly)
}

func (s *Server) emrListSecurityConfigurations(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	_ = params
	if !s.authorize(verified, catalog.ActionEMRListSecurityConfigurations, "*") {
		s.writeEMRError(w, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform elasticmapreduce:ListSecurityConfigurations.")
		return
	}
	configs, err := s.store.ListEMRSecurityConfigurations(verified.AccountID)
	if err != nil {
		s.writeEMRError(w, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list security configurations.")
		return
	}
	payload, _ := emrsvc.ListSecurityConfigurationsJSON(configs)
	s.writeEMROK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, emrEventSource, "ListSecurityConfigurations", readOnly)
}

func parseEMRStepInputs(raw any) []store.EMRStepInput {
	arr, ok := raw.([]any)
	if !ok || len(arr) == 0 {
		return nil
	}
	out := make([]store.EMRStepInput, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		in := store.EMRStepInput{
			Name:             stringField(m, "Name"),
			ActionOnFailure:  stringField(m, "ActionOnFailure"),
			ExecutionRoleArn: stringField(m, "ExecutionRoleArn"),
		}
		if jarStep, ok := m["HadoopJarStep"].(map[string]any); ok {
			in.Jar = stringField(jarStep, "Jar")
			in.MainClass = stringField(jarStep, "MainClass")
			in.Args = emrStringList(jarStep["Args"])
			in.Properties = emrProperties(jarStep["Properties"])
		}
		out = append(out, in)
	}
	return out
}

func parseEMRInstanceGroups(raw any) []store.EMRInstanceGroupInput {
	arr, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]store.EMRInstanceGroupInput, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		groupType := stringField(m, "InstanceRole")
		if groupType == "" {
			groupType = stringField(m, "InstanceGroupType")
		}
		out = append(out, store.EMRInstanceGroupInput{
			Name:              stringField(m, "Name"),
			InstanceGroupType: groupType,
			InstanceType:      stringField(m, "InstanceType"),
			Market:            stringField(m, "Market"),
			BidPrice:          stringField(m, "BidPrice"),
			InstanceCount:     intFromJSONNumber(m["InstanceCount"]),
		})
	}
	return out
}

func parseEMRInstanceFleets(raw any) []store.EMRInstanceFleetInput {
	arr, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]store.EMRInstanceFleetInput, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, store.EMRInstanceFleetInput{
			Name:                   stringField(m, "Name"),
			InstanceFleetType:      stringField(m, "InstanceFleetType"),
			TargetOnDemandCapacity: intFromJSONNumber(m["TargetOnDemandCapacity"]),
			TargetSpotCapacity:     intFromJSONNumber(m["TargetSpotCapacity"]),
		})
	}
	return out
}

func parseEMRTags(raw any) map[string]string {
	arr, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := map[string]string{}
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		k := stringField(m, "Key")
		if k != "" {
			out[k] = stringField(m, "Value")
		}
	}
	return out
}

func emrStringList(raw any) []string {
	arr, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, v := range arr {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func emrProperties(raw any) map[string]string {
	arr, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := map[string]string{}
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		k := stringField(m, "Key")
		if k != "" {
			out[k] = stringField(m, "Value")
		}
	}
	return out
}

func stringField(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func (s *Server) writeEMROK(w http.ResponseWriter, requestID string, payload []byte) {
	w.Header().Set("Content-Type", emrJSONContentType)
	w.Header().Set("x-amzn-RequestId", requestID)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeEMRError(w http.ResponseWriter, requestID string, status int, code, message string) {
	w.Header().Set("Content-Type", emrJSONContentType)
	w.Header().Set("x-amzn-RequestId", requestID)
	w.Header().Set("x-amzn-ErrorType", code)
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"__type":"` + code + `","message":"` + message + `"}`))
}
