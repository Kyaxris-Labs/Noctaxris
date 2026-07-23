package server

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	ecssvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/ecs"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

var (
	ecsServiceReconcilerOnce     sync.Once
	ecsServiceReconcilerStop     chan struct{}
	ecsServiceReconcilerStopOnce sync.Once
)

func (s *Server) ensureECSServiceReconciler() {
	ecsServiceReconcilerOnce.Do(func() {
		ecsServiceReconcilerStop = make(chan struct{})
		go s.runECSServiceReconciler(ecsServiceReconcilerStop)
	})
}

// StopECSServiceReconciler stops the ECS DesiredCount reconciler if running.
func (s *Server) StopECSServiceReconciler() {
	ecsServiceReconcilerStopOnce.Do(func() {
		if ecsServiceReconcilerStop != nil {
			close(ecsServiceReconcilerStop)
		}
	})
}

func (s *Server) runECSServiceReconciler(stop <-chan struct{}) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			s.reconcileAllECSServices()
		}
	}
}

func (s *Server) reconcileAllECSServices() {
	items, err := s.store.ListActiveServicesWithAccount()
	if err != nil {
		return
	}
	for _, item := range items {
		s.reconcileECSService(item.AccountID, store.DefaultECSRegion, item.Service)
	}
}

func (s *Server) reconcileECSService(accountID, region string, svc store.ECSService) {
	runningARNs, err := s.store.ListServiceTaskARNs(accountID, svc.ClusterName, svc.ServiceName, store.ECSTaskStatusRunning)
	if err != nil {
		return
	}
	running := len(runningARNs)
	for running > svc.DesiredCount {
		taskARN := runningARNs[0]
		runningARNs = runningARNs[1:]
		_, _ = s.store.StopTask(accountID, region, store.StopTaskInput{
			Cluster: svc.ClusterName,
			Task:    taskARN,
		})
		running--
	}
	for running < svc.DesiredCount {
		td, err := s.store.DescribeTaskDefinitionByARN(accountID, svc.TaskDefinition)
		if err != nil {
			return
		}
		// Fail closed when task-def roles diverge from roles PassRole'd at Create/Update.
		if !ecsServiceRolesPassRoleMatch(svc, td) {
			return
		}
		task, err := s.store.RunServiceTask(accountID, region, svc.ClusterName, svc.ServiceName, td.ARN)
		if err != nil {
			return
		}
		if err := s.executeECSTask(context.Background(), accountID, region, svc.ClusterName, td, td.TaskRoleARN, td.ExecutionRoleARN, task.TaskARN); err != nil {
			_ = s.store.DeleteTask(accountID, task.TaskARN)
			// Without DinD, leave runningCount below desired (lab-safe).
			return
		}
		running++
	}
}

func ecsServiceRolesPassRoleMatch(svc store.ECSService, td store.ECSTaskDefinition) bool {
	if strings.TrimSpace(svc.PassedTaskRoleARN) == "" || strings.TrimSpace(svc.PassedExecutionRoleARN) == "" {
		return false
	}
	return strings.TrimSpace(svc.PassedTaskRoleARN) == strings.TrimSpace(td.TaskRoleARN) &&
		strings.TrimSpace(svc.PassedExecutionRoleARN) == strings.TrimSpace(td.ExecutionRoleARN)
}

func (s *Server) ecsCreateService(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	s.ensureECSServiceReconciler()
	region := s.ecsRegion(verified)
	cluster, _ := params["cluster"].(string)
	serviceName, _ := params["serviceName"].(string)
	taskDef, _ := params["taskDefinition"].(string)
	desired := 0
	switch v := params["desiredCount"].(type) {
	case float64:
		desired = int(v)
	case int:
		desired = v
	}
	resource := s.ecsClusterARN(verified, store.NormalizeECSClusterNamePublic(cluster))
	if !s.authorizeECS(verified, catalog.ActionECSCreateService, resource) {
		s.writeECSError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform ecs:CreateService.", readOnly, eventID, verified)
		return
	}
	td, err := s.resolveTaskDefinition(verified.AccountID, taskDef)
	if errors.Is(err, store.ErrECSTaskDefinitionNotFound) {
		s.writeECSError(w, r, body, requestID, http.StatusBadRequest, "ClientException",
			"task definition not found", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeECSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to resolve task definition.", readOnly, eventID, verified)
		return
	}
	if err := s.checkECSPassRole(verified, td.TaskRoleARN, ""); err != nil {
		s.writeECSError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err := s.checkECSPassRole(verified, td.ExecutionRoleARN, ""); err != nil {
		s.writeECSError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	svc, err := s.store.CreateService(verified.AccountID, region, store.CreateServiceInput{
		Cluster:               cluster,
		ServiceName:           serviceName,
		TaskDefinition:        taskDef,
		DesiredCount:          desired,
		PassedTaskRoleARN:     td.TaskRoleARN,
		PassedExecutionRoleARN: td.ExecutionRoleARN,
	})
	if errors.Is(err, store.ErrECSServiceAlreadyExists) {
		s.writeECSError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrECSTaskDefinitionNotFound) {
		s.writeECSError(w, r, body, requestID, http.StatusBadRequest, "ClientException",
			"task definition not found", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeECSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create service.", readOnly, eventID, verified)
		return
	}
	// Inline reconcile for tests without waiting for ticker (scale-down and store-only bookkeeping).
	s.reconcileECSService(verified.AccountID, region, svc)
	svc, _ = s.store.GetService(verified.AccountID, svc.ClusterName, svc.ServiceName)
	payload, err := ecssvc.CreateServiceJSON(svc)
	if err != nil {
		s.writeECSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeECSOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ecsEventSource, "CreateService", readOnly)
}

func (s *Server) ecsUpdateService(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	s.ensureECSServiceReconciler()
	region := s.ecsRegion(verified)
	cluster, _ := params["cluster"].(string)
	serviceName, _ := params["service"].(string)
	if serviceName == "" {
		serviceName, _ = params["serviceName"].(string)
	}
	resource := s.ecsClusterARN(verified, store.NormalizeECSClusterNamePublic(cluster))
	if !s.authorizeECS(verified, catalog.ActionECSUpdateService, resource) {
		s.writeECSError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform ecs:UpdateService.", readOnly, eventID, verified)
		return
	}
	in := store.UpdateServiceInput{
		Cluster: cluster,
		Service: serviceName,
	}
	if tdRef, ok := params["taskDefinition"].(string); ok {
		in.TaskDefinition = tdRef
	}
	if raw, ok := params["desiredCount"]; ok {
		switch v := raw.(type) {
		case float64:
			n := int(v)
			in.DesiredCount = &n
		case int:
			in.DesiredCount = &v
		}
	}
	cur, err := s.store.GetService(verified.AccountID, cluster, serviceName)
	if errors.Is(err, store.ErrECSServiceNotFound) {
		s.writeECSError(w, r, body, requestID, http.StatusBadRequest, "ServiceNotFoundException",
			"Service not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeECSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to load service.", readOnly, eventID, verified)
		return
	}
	targetTDRef := cur.TaskDefinition
	if strings.TrimSpace(in.TaskDefinition) != "" {
		targetTDRef = in.TaskDefinition
	}
	needPassRole := strings.TrimSpace(in.TaskDefinition) != "" ||
		(in.DesiredCount != nil && *in.DesiredCount > 0)
	if needPassRole {
		td, resolveErr := s.resolveTaskDefinition(verified.AccountID, targetTDRef)
		if errors.Is(resolveErr, store.ErrECSTaskDefinitionNotFound) {
			s.writeECSError(w, r, body, requestID, http.StatusBadRequest, "ClientException",
				"task definition not found", readOnly, eventID, verified)
			return
		}
		if resolveErr != nil {
			s.writeECSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to resolve task definition.", readOnly, eventID, verified)
			return
		}
		if err := s.checkECSPassRole(verified, td.TaskRoleARN, ""); err != nil {
			s.writeECSError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
				err.Error(), readOnly, eventID, verified)
			return
		}
		if err := s.checkECSPassRole(verified, td.ExecutionRoleARN, ""); err != nil {
			s.writeECSError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
				err.Error(), readOnly, eventID, verified)
			return
		}
		in.PassedTaskRoleARN = td.TaskRoleARN
		in.PassedExecutionRoleARN = td.ExecutionRoleARN
	}
	svc, err := s.store.UpdateService(verified.AccountID, region, in)
	if errors.Is(err, store.ErrECSServiceNotFound) {
		s.writeECSError(w, r, body, requestID, http.StatusBadRequest, "ServiceNotFoundException",
			"Service not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeECSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to update service.", readOnly, eventID, verified)
		return
	}
	s.reconcileECSService(verified.AccountID, region, svc)
	svc, _ = s.store.GetService(verified.AccountID, svc.ClusterName, svc.ServiceName)
	payload, err := ecssvc.CreateServiceJSON(svc)
	if err != nil {
		s.writeECSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeECSOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ecsEventSource, "UpdateService", readOnly)
}

func (s *Server) ecsDeleteService(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	s.ensureECSServiceReconciler()
	region := s.ecsRegion(verified)
	cluster, _ := params["cluster"].(string)
	serviceName, _ := params["service"].(string)
	if serviceName == "" {
		serviceName, _ = params["serviceName"].(string)
	}
	resource := s.ecsClusterARN(verified, store.NormalizeECSClusterNamePublic(cluster))
	if !s.authorizeECS(verified, catalog.ActionECSDeleteService, resource) {
		s.writeECSError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform ecs:DeleteService.", readOnly, eventID, verified)
		return
	}
	svc, err := s.store.DeleteService(verified.AccountID, cluster, serviceName)
	if errors.Is(err, store.ErrECSServiceNotFound) {
		s.writeECSError(w, r, body, requestID, http.StatusBadRequest, "ServiceNotFoundException",
			"Service not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeECSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete service.", readOnly, eventID, verified)
		return
	}
	s.reconcileECSService(verified.AccountID, region, svc)
	svc, _ = s.store.GetService(verified.AccountID, svc.ClusterName, svc.ServiceName)
	payload, err := ecssvc.CreateServiceJSON(svc)
	if err != nil {
		s.writeECSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeECSOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ecsEventSource, "DeleteService", readOnly)
}

func (s *Server) ecsDescribeServices(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	cluster, _ := params["cluster"].(string)
	resource := s.ecsClusterARN(verified, store.NormalizeECSClusterNamePublic(cluster))
	if !s.authorizeECS(verified, catalog.ActionECSDescribeServices, resource) {
		s.writeECSError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform ecs:DescribeServices.", readOnly, eventID, verified)
		return
	}
	names := stringSliceParam(params["services"])
	services, err := s.store.DescribeServices(verified.AccountID, cluster, names)
	if err != nil {
		s.writeECSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe services.", readOnly, eventID, verified)
		return
	}
	payload, err := ecssvc.DescribeServicesJSON(services)
	if err != nil {
		s.writeECSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeECSOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ecsEventSource, "DescribeServices", readOnly)
}

func (s *Server) ecsListServices(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	cluster, _ := params["cluster"].(string)
	resource := s.ecsClusterARN(verified, store.NormalizeECSClusterNamePublic(cluster))
	if !s.authorizeECS(verified, catalog.ActionECSListServices, resource) {
		s.writeECSError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform ecs:ListServices.", readOnly, eventID, verified)
		return
	}
	arns, err := s.store.ListServices(verified.AccountID, cluster)
	if err != nil {
		s.writeECSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list services.", readOnly, eventID, verified)
		return
	}
	payload, err := ecssvc.ListServicesJSON(arns)
	if err != nil {
		s.writeECSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeECSOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ecsEventSource, "ListServices", readOnly)
}
