package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/compute"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/sts"
	ecssvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/ecs"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	ecsJSONContentType = "application/x-amz-json-1.1"
	ecsEventSource     = "ecs.amazonaws.com"
	ecsInvokeSession   = "noctaxris-ecs"
	defaultECSEndpoint = "http://host.docker.internal:4566"
)

func (s *Server) handleECS(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = ecsAction(action)

	switch action {
	case catalog.ActionECSRegisterTaskDefinition:
		s.ecsRegisterTaskDefinition(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionECSDescribeTaskDefinition:
		s.ecsDescribeTaskDefinition(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionECSListTaskDefinitions:
		s.ecsListTaskDefinitions(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionECSDeregisterTaskDefinition:
		s.ecsDeregisterTaskDefinition(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionECSRunTask:
		s.ecsRunTask(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionECSDescribeTasks:
		s.ecsDescribeTasks(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionECSListTasks:
		s.ecsListTasks(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionECSStopTask:
		s.ecsStopTask(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionECSDescribeClusters:
		s.ecsDescribeClusters(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionECSListClusters:
		s.ecsListClusters(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionECSCreateService:
		s.ecsCreateService(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionECSUpdateService:
		s.ecsUpdateService(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionECSDeleteService:
		s.ecsDeleteService(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionECSDescribeServices:
		s.ecsDescribeServices(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionECSListServices:
		s.ecsListServices(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeECSError(w, r, body, requestID, http.StatusNotImplemented, "InternalFailure",
			"This ECS action is not implemented.", readOnly, eventID, verified)
	}
}

func ecsAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "RegisterTaskDefinition":
		return catalog.ActionECSRegisterTaskDefinition
	case "DescribeTaskDefinition":
		return catalog.ActionECSDescribeTaskDefinition
	case "ListTaskDefinitions":
		return catalog.ActionECSListTaskDefinitions
	case "DeregisterTaskDefinition":
		return catalog.ActionECSDeregisterTaskDefinition
	case "RunTask":
		return catalog.ActionECSRunTask
	case "DescribeTasks":
		return catalog.ActionECSDescribeTasks
	case "ListTasks":
		return catalog.ActionECSListTasks
	case "StopTask":
		return catalog.ActionECSStopTask
	case "DescribeClusters":
		return catalog.ActionECSDescribeClusters
	case "ListClusters":
		return catalog.ActionECSListClusters
	case "CreateService":
		return catalog.ActionECSCreateService
	case "UpdateService":
		return catalog.ActionECSUpdateService
	case "DeleteService":
		return catalog.ActionECSDeleteService
	case "DescribeServices":
		return catalog.ActionECSDescribeServices
	case "ListServices":
		return catalog.ActionECSListServices
	default:
		return action
	}
}

func (s *Server) ecsRegion(verified *authn.Verified) string {
	if verified.Region != "" {
		return verified.Region
	}
	return store.DefaultECSRegion
}

func (s *Server) ecsClusterARN(verified *authn.Verified, clusterName string) string {
	return store.ClusterARN(s.ecsRegion(verified), verified.AccountID, clusterName)
}

func (s *Server) ecsTaskDefinitionFamilyResource(verified *authn.Verified, family string) string {
	return fmt.Sprintf("arn:aws:ecs:%s:%s:task-definition/%s",
		s.ecsRegion(verified), verified.AccountID, family)
}

func (s *Server) authorizeECS(verified *authn.Verified, action, resource string) bool {
	return s.authorize(verified, action, resource)
}

func (s *Server) checkECSPassRole(verified *authn.Verified, roleARN string) error {
	accountID, roleName, ok := sts.ParseRoleARN(roleARN)
	if !ok {
		return errors.New("RoleArn must be a valid IAM role ARN")
	}
	if accountID != verified.AccountID {
		return errors.New("RoleArn must be in the same account")
	}
	storedARN, trust, err := s.store.GetRole(accountID, roleName)
	if err != nil {
		return errors.New("RoleArn not found")
	}
	if storedARN != "" {
		roleARN = storedARN
	}
	in, ok := s.evalInputs(verified)
	if !ok {
		return errors.New("not authorized to pass role to ECS")
	}
	decision := authz.CheckPassRole(authz.PassRoleRequest{
		Caller: authz.RequestContext{
			Principal:     verified.Principal,
			Resource:      roleARN,
			Region:        verified.Region,
			ConditionKeys: s.conditionKeys(verified),
		},
		EvalInputs:       in,
		RoleARN:          roleARN,
		TrustPolicyDoc:   trust,
		ServicePrincipal: authz.ServicePrincipalECSTasks,
	})
	if decision != authz.Allow {
		return errors.New("not authorized to pass role to ECS")
	}
	return nil
}

func containerDefinitionsFromParams(params map[string]any) []map[string]any {
	raw, ok := params["containerDefinitions"].([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func ecsPrimaryContainer(td store.ECSTaskDefinition) (map[string]any, error) {
	if len(td.ContainerDefs) == 0 {
		return nil, errors.New("task definition has no container definitions")
	}
	for _, def := range td.ContainerDefs {
		essential, ok := def["essential"].(bool)
		if !ok || essential {
			return def, nil
		}
	}
	return td.ContainerDefs[0], nil
}

func ecsContainerCommand(def map[string]any) []string {
	raw, ok := def["command"].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func ecsContainerEnv(def map[string]any) map[string]string {
	out := map[string]string{}
	envObj, _ := def["environment"].([]any)
	for _, item := range envObj {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name, _ := m["name"].(string)
		value, _ := m["value"].(string)
		if name != "" {
			out[name] = value
		}
	}
	return out
}

func (s *Server) ecsRegisterTaskDefinition(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	family := stringParam(params["family"])
	taskRoleARN := stringParam(params["taskRoleArn"])
	executionRoleARN := stringParam(params["executionRoleArn"])
	containerDefs := containerDefinitionsFromParams(params)

	if family == "" {
		s.writeECSError(w, r, body, requestID, http.StatusBadRequest, "ClientException",
			"Family is required.", readOnly, eventID, verified)
		return
	}
	resource := s.ecsTaskDefinitionFamilyResource(verified, family)
	if !s.authorizeECS(verified, catalog.ActionECSRegisterTaskDefinition, resource) {
		s.writeECSError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform ecs:RegisterTaskDefinition.", readOnly, eventID, verified)
		return
	}
	if err := s.checkECSPassRole(verified, taskRoleARN); err != nil {
		s.writeECSError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err := s.checkECSPassRole(verified, executionRoleARN); err != nil {
		s.writeECSError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			err.Error(), readOnly, eventID, verified)
		return
	}

	region := s.ecsRegion(verified)
	td, err := s.store.RegisterTaskDefinition(verified.AccountID, region, store.RegisterTaskDefinitionInput{
		Family:           family,
		ContainerDefs:    containerDefs,
		TaskRoleARN:      taskRoleARN,
		ExecutionRoleARN: executionRoleARN,
	})
	if err != nil {
		code := "ClientException"
		if errors.Is(err, store.ErrECSMissingTaskRoleARN) || errors.Is(err, store.ErrECSMissingExecutionRoleARN) {
			code = "ClientException"
		}
		s.writeECSError(w, r, body, requestID, http.StatusBadRequest, code,
			err.Error(), readOnly, eventID, verified)
		return
	}

	payload, err := ecssvc.RegisterTaskDefinitionJSON(td)
	if err != nil {
		s.writeECSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeECSOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ecsEventSource, "RegisterTaskDefinition", readOnly)
}

func (s *Server) ecsDescribeTaskDefinition(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	family := stringParam(params["taskDefinition"])
	if family == "" {
		s.writeECSError(w, r, body, requestID, http.StatusBadRequest, "ClientException",
			"taskDefinition is required.", readOnly, eventID, verified)
		return
	}
	revision := intParam(params["revision"], 0)

	var td store.ECSTaskDefinition
	var err error
	if revision > 0 {
		td, err = s.store.DescribeTaskDefinition(verified.AccountID, family, revision)
	} else if strings.Contains(family, "task-definition/") {
		td, err = s.store.DescribeTaskDefinitionByARN(verified.AccountID, family)
	} else {
		arns, listErr := s.store.ListTaskDefinitions(verified.AccountID, family)
		if listErr != nil {
			err = listErr
		} else if len(arns) == 0 {
			err = store.ErrECSTaskDefinitionNotFound
		} else {
			td, err = s.store.DescribeTaskDefinitionByARN(verified.AccountID, arns[len(arns)-1])
		}
	}
	if err != nil {
		if errors.Is(err, store.ErrECSTaskDefinitionNotFound) {
			s.writeECSError(w, r, body, requestID, http.StatusBadRequest, "ClientException",
				"Task definition not found.", readOnly, eventID, verified)
			return
		}
		s.writeECSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe task definition.", readOnly, eventID, verified)
		return
	}

	resource := td.ARN
	if !s.authorizeECS(verified, catalog.ActionECSDescribeTaskDefinition, resource) {
		s.writeECSError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform ecs:DescribeTaskDefinition.", readOnly, eventID, verified)
		return
	}

	payload, err := ecssvc.DescribeTaskDefinitionJSON(td)
	if err != nil {
		s.writeECSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeECSOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ecsEventSource, "DescribeTaskDefinition", readOnly)
}

func (s *Server) ecsListTaskDefinitions(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	family := stringParam(params["family"])
	resource := "*"
	if family != "" {
		resource = s.ecsTaskDefinitionFamilyResource(verified, family)
	}
	if !s.authorizeECS(verified, catalog.ActionECSListTaskDefinitions, resource) {
		s.writeECSError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform ecs:ListTaskDefinitions.", readOnly, eventID, verified)
		return
	}

	arns, err := s.store.ListTaskDefinitions(verified.AccountID, family)
	if err != nil {
		s.writeECSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list task definitions.", readOnly, eventID, verified)
		return
	}

	payload, err := ecssvc.ListTaskDefinitionsJSON(arns)
	if err != nil {
		s.writeECSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeECSOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ecsEventSource, "ListTaskDefinitions", readOnly)
}

func (s *Server) ecsDeregisterTaskDefinition(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	taskDefARN := stringParam(params["taskDefinition"])
	if taskDefARN == "" {
		s.writeECSError(w, r, body, requestID, http.StatusBadRequest, "ClientException",
			"taskDefinition is required.", readOnly, eventID, verified)
		return
	}
	if !s.authorizeECS(verified, catalog.ActionECSDeregisterTaskDefinition, taskDefARN) {
		s.writeECSError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform ecs:DeregisterTaskDefinition.", readOnly, eventID, verified)
		return
	}

	td, err := s.store.DescribeTaskDefinitionByARN(verified.AccountID, taskDefARN)
	if err != nil {
		if errors.Is(err, store.ErrECSTaskDefinitionNotFound) {
			s.writeECSError(w, r, body, requestID, http.StatusBadRequest, "ClientException",
				"Task definition not found.", readOnly, eventID, verified)
			return
		}
		s.writeECSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe task definition.", readOnly, eventID, verified)
		return
	}
	if err := s.store.DeregisterTaskDefinition(verified.AccountID, taskDefARN); err != nil {
		if errors.Is(err, store.ErrECSTaskDefinitionNotFound) {
			s.writeECSError(w, r, body, requestID, http.StatusBadRequest, "ClientException",
				"Task definition not found.", readOnly, eventID, verified)
			return
		}
		s.writeECSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to deregister task definition.", readOnly, eventID, verified)
		return
	}
	td.Status = store.ECSTaskDefinitionInactive

	payload, err := ecssvc.DeregisterTaskDefinitionJSON(td)
	if err != nil {
		s.writeECSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeECSOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ecsEventSource, "DeregisterTaskDefinition", readOnly)
}

func (s *Server) ecsRunTask(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	cluster := stringParam(params["cluster"])
	taskDef := stringParam(params["taskDefinition"])
	if taskDef == "" {
		s.writeECSError(w, r, body, requestID, http.StatusBadRequest, "ClientException",
			"taskDefinition is required.", readOnly, eventID, verified)
		return
	}

	clusterResource := s.ecsClusterARN(verified, cluster)
	if !s.authorizeECS(verified, catalog.ActionECSRunTask, clusterResource) {
		s.writeECSError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform ecs:RunTask.", readOnly, eventID, verified)
		return
	}

	region := s.ecsRegion(verified)
	td, err := s.resolveTaskDefinition(verified.AccountID, taskDef)
	if err != nil {
		if errors.Is(err, store.ErrECSTaskDefinitionNotFound) {
			s.writeECSError(w, r, body, requestID, http.StatusBadRequest, "ClientException",
				"Task definition not found.", readOnly, eventID, verified)
			return
		}
		s.writeECSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to resolve task definition.", readOnly, eventID, verified)
		return
	}
	if td.Status != store.ECSTaskDefinitionActive {
		s.writeECSError(w, r, body, requestID, http.StatusBadRequest, "ClientException",
			"Task definition not found.", readOnly, eventID, verified)
		return
	}

	taskRoleARN := td.TaskRoleARN
	executionRoleARN := td.ExecutionRoleARN
	if overrides, ok := params["overrides"].(map[string]any); ok {
		if role := stringParam(overrides["taskRoleArn"]); role != "" {
			taskRoleARN = role
		}
		if role := stringParam(overrides["executionRoleArn"]); role != "" {
			executionRoleARN = role
		}
	}
	if err := s.checkECSPassRole(verified, taskRoleARN); err != nil {
		s.writeECSError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err := s.checkECSPassRole(verified, executionRoleARN); err != nil {
		s.writeECSError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			err.Error(), readOnly, eventID, verified)
		return
	}

	if strings.TrimSpace(s.cfg.DockerHost) == "" {
		s.writeECSError(w, r, body, requestID, http.StatusServiceUnavailable, "ServiceException",
			"compute unavailable", readOnly, eventID, verified)
		return
	}

	task, err := s.store.RunTask(verified.AccountID, region, store.RunTaskInput{
		Cluster:        cluster,
		TaskDefinition: td.ARN,
	})
	if err != nil {
		s.writeECSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to run task.", readOnly, eventID, verified)
		return
	}

	if err := s.executeECSTask(r.Context(), verified.AccountID, region, cluster, td, taskRoleARN, task.TaskARN); err != nil {
		_ = s.store.DeleteTask(verified.AccountID, task.TaskARN)
		if strings.Contains(err.Error(), "compute unavailable") {
			s.writeECSError(w, r, body, requestID, http.StatusServiceUnavailable, "ServiceException",
				"compute unavailable", readOnly, eventID, verified)
			return
		}
		s.writeECSError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			err.Error(), readOnly, eventID, verified)
		return
	}

	payload, err := ecssvc.RunTaskJSON([]store.ECSTask{task})
	if err != nil {
		s.writeECSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeECSOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ecsEventSource, "RunTask", readOnly)
}

func (s *Server) resolveTaskDefinition(accountID, taskDef string) (store.ECSTaskDefinition, error) {
	taskDef = strings.TrimSpace(taskDef)
	if strings.Contains(taskDef, "task-definition/") {
		return s.store.DescribeTaskDefinitionByARN(accountID, taskDef)
	}
	arns, err := s.store.ListTaskDefinitions(accountID, taskDef)
	if err != nil {
		return store.ECSTaskDefinition{}, err
	}
	if len(arns) == 0 {
		return store.ECSTaskDefinition{}, store.ErrECSTaskDefinitionNotFound
	}
	return s.store.DescribeTaskDefinitionByARN(accountID, arns[len(arns)-1])
}

func (s *Server) executeECSTask(
	ctx context.Context,
	accountID, region, cluster string,
	td store.ECSTaskDefinition,
	taskRoleARN, taskARN string,
) error {
	containerDef, err := ecsPrimaryContainer(td)
	if err != nil {
		return err
	}
	imageURI, _ := containerDef["image"].(string)
	imageURI = strings.TrimSpace(imageURI)
	if imageURI == "" {
		return errors.New("container image is required")
	}

	cli, err := s.computeClient()
	if err != nil || cli == nil {
		return errors.New("compute unavailable")
	}

	roleAccountID, _, ok := sts.ParseRoleARN(taskRoleARN)
	if !ok {
		return errors.New("task role ARN is invalid")
	}
	secret, err := randomSecret()
	if err != nil {
		return fmt.Errorf("mint credentials: %w", err)
	}
	sessionToken, err := randomSecret()
	if err != nil {
		return fmt.Errorf("mint credentials: %w", err)
	}
	expires := s.now().UTC().Add(defaultSessionDuration)
	accessKeyID, err := s.store.MintTempCredentialsOpts(store.MintTempOpts{
		AccountID:    roleAccountID,
		RoleARN:      taskRoleARN,
		SessionName:  ecsInvokeSession,
		Secret:       secret,
		SessionToken: sessionToken,
		Expires:      expires,
	})
	if err != nil {
		return fmt.Errorf("mint task role credentials: %w", err)
	}

	endpoint := strings.TrimSpace(s.cfg.LambdaEndpointURL)
	if endpoint == "" {
		endpoint = defaultECSEndpoint
	}
	env := ecsContainerEnv(containerDef)
	env["AWS_ACCESS_KEY_ID"] = accessKeyID
	env["AWS_SECRET_ACCESS_KEY"] = secret
	env["AWS_SESSION_TOKEN"] = sessionToken
	env["AWS_DEFAULT_REGION"] = store.DefaultECSRegion
	env["AWS_REGION"] = store.DefaultECSRegion
	env["AWS_ENDPOINT_URL"] = endpoint
	env["AWS_ENDPOINT_URL_STS"] = endpoint
	env["AWS_ENDPOINT_URL_IAM"] = endpoint
	env["AWS_ENDPOINT_URL_S3"] = endpoint
	env["AWS_ENDPOINT_URL_DYNAMODB"] = endpoint
	env["AWS_ENDPOINT_URL_SQS"] = endpoint
	env["AWS_ENDPOINT_URL_LAMBDA"] = endpoint
	env["AWS_ENDPOINT_URL_KMS"] = endpoint
	env["AWS_ENDPOINT_URL_ECR"] = endpoint
	env["AWS_ENDPOINT_URL_ECS"] = endpoint

	pullRef, useAuth, username, password, err := compute.IssueLabRegistryPull(
		s.store, s.cfg.ListenAddr, accountID, imageURI, "ecs-tasks.amazonaws.com",
	)
	if err != nil {
		return err
	}
	if useAuth {
		if err := cli.PullLabRegistryImage(ctx, pullRef, username, password); err != nil {
			return fmt.Errorf("pull lab registry image: %w", err)
		}
	}

	containerID, err := cli.RunECSTask(ctx, compute.ECSRunOpts{
		ImageURI:    pullRef,
		Command:     ecsContainerCommand(containerDef),
		Env:         env,
		EndpointURL: endpoint,
	})
	if err != nil {
		return err
	}
	if err := s.store.SetTaskRuntimeID(accountID, taskARN, containerID); err != nil {
		_ = cli.StopECSTask(ctx, containerID)
		return err
	}
	go s.reapECSTask(accountID, region, cluster, taskARN, containerID)
	return nil
}

// reapECSTask waits for the DinD container to exit then marks the store task STOPPED.
func (s *Server) reapECSTask(accountID, region, cluster, taskARN, containerID string) {
	cli, err := s.computeClient()
	if err != nil || cli == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	if _, err := cli.WaitECSTaskExit(ctx, containerID); err != nil {
		return
	}
	_ = cli.StopECSTask(context.Background(), containerID)
	_, _ = s.store.StopTask(accountID, region, store.StopTaskInput{
		Cluster: cluster,
		Task:    taskARN,
	})
}

// syncECSTaskStatuses marks RUNNING tasks STOPPED when their DinD container has exited.
func (s *Server) syncECSTaskStatuses(ctx context.Context, accountID, region, cluster string, tasks []store.ECSTask) []store.ECSTask {
	if strings.TrimSpace(s.cfg.DockerHost) == "" || len(tasks) == 0 {
		return tasks
	}
	cli, err := s.computeClient()
	if err != nil || cli == nil {
		return tasks
	}
	out := make([]store.ECSTask, len(tasks))
	copy(out, tasks)
	for i, task := range out {
		if task.LastStatus != store.ECSTaskStatusRunning {
			continue
		}
		runtimeID, err := s.store.TaskRuntimeID(accountID, task.TaskARN)
		if err != nil || runtimeID == "" {
			continue
		}
		running, err := cli.ContainerRunning(ctx, runtimeID)
		if err != nil || running {
			continue
		}
		_ = cli.StopECSTask(ctx, runtimeID)
		stopped, err := s.store.StopTask(accountID, region, store.StopTaskInput{
			Cluster: cluster,
			Task:    task.TaskARN,
		})
		if err == nil {
			out[i] = stopped
		}
	}
	return out
}

func (s *Server) ecsDescribeTasks(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	cluster := stringParam(params["cluster"])
	taskARNs := stringSliceParam(params["tasks"])
	clusterResource := s.ecsClusterARN(verified, cluster)
	if !s.authorizeECS(verified, catalog.ActionECSDescribeTasks, clusterResource) {
		s.writeECSError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform ecs:DescribeTasks.", readOnly, eventID, verified)
		return
	}

	tasks, err := s.store.DescribeTasks(verified.AccountID, cluster, taskARNs)
	if err != nil {
		s.writeECSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe tasks.", readOnly, eventID, verified)
		return
	}
	tasks = s.syncECSTaskStatuses(r.Context(), verified.AccountID, s.ecsRegion(verified), cluster, tasks)

	payload, err := ecssvc.DescribeTasksJSON(tasks)
	if err != nil {
		s.writeECSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeECSOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ecsEventSource, "DescribeTasks", readOnly)
}

func (s *Server) ecsListTasks(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	cluster := stringParam(params["cluster"])
	clusterResource := s.ecsClusterARN(verified, cluster)
	if !s.authorizeECS(verified, catalog.ActionECSListTasks, clusterResource) {
		s.writeECSError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform ecs:ListTasks.", readOnly, eventID, verified)
		return
	}

	tasks, err := s.store.ListTasks(verified.AccountID, cluster)
	if err != nil {
		s.writeECSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list tasks.", readOnly, eventID, verified)
		return
	}
	tasks = s.syncECSTaskStatuses(r.Context(), verified.AccountID, s.ecsRegion(verified), cluster, tasks)
	arns := make([]string, 0, len(tasks))
	for _, task := range tasks {
		arns = append(arns, task.TaskARN)
	}

	payload, err := ecssvc.ListTasksJSON(arns)
	if err != nil {
		s.writeECSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeECSOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ecsEventSource, "ListTasks", readOnly)
}

func (s *Server) ecsStopTask(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	cluster := stringParam(params["cluster"])
	taskARN := stringParam(params["task"])
	if taskARN == "" {
		s.writeECSError(w, r, body, requestID, http.StatusBadRequest, "ClientException",
			"task is required.", readOnly, eventID, verified)
		return
	}
	clusterResource := s.ecsClusterARN(verified, cluster)
	if !s.authorizeECS(verified, catalog.ActionECSStopTask, clusterResource) {
		s.writeECSError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform ecs:StopTask.", readOnly, eventID, verified)
		return
	}

	region := s.ecsRegion(verified)
	if runtimeID, err := s.store.TaskRuntimeID(verified.AccountID, taskARN); err == nil && runtimeID != "" {
		if cli, cliErr := s.computeClient(); cliErr == nil && cli != nil {
			_ = cli.StopECSTask(r.Context(), runtimeID)
		}
	}

	task, err := s.store.StopTask(verified.AccountID, region, store.StopTaskInput{
		Cluster: cluster,
		Task:    taskARN,
	})
	if err != nil {
		if errors.Is(err, store.ErrECSTaskNotFound) {
			s.writeECSError(w, r, body, requestID, http.StatusBadRequest, "ClientException",
				"Task not found.", readOnly, eventID, verified)
			return
		}
		s.writeECSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to stop task.", readOnly, eventID, verified)
		return
	}

	payload, err := ecssvc.StopTaskJSON(task)
	if err != nil {
		s.writeECSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeECSOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ecsEventSource, "StopTask", readOnly)
}

func (s *Server) ecsDescribeClusters(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	names := stringSliceParam(params["clusters"])
	resource := s.ecsClusterARN(verified, store.DefaultECSClusterName)
	if !s.authorizeECS(verified, catalog.ActionECSDescribeClusters, resource) {
		s.writeECSError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform ecs:DescribeClusters.", readOnly, eventID, verified)
		return
	}

	region := s.ecsRegion(verified)
	clusters, err := s.store.DescribeClusters(verified.AccountID, region, names)
	if err != nil {
		s.writeECSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe clusters.", readOnly, eventID, verified)
		return
	}

	payload, err := ecssvc.DescribeClustersJSON(clusters)
	if err != nil {
		s.writeECSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeECSOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ecsEventSource, "DescribeClusters", readOnly)
}

func (s *Server) ecsListClusters(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	_ = params
	resource := s.ecsClusterARN(verified, store.DefaultECSClusterName)
	if !s.authorizeECS(verified, catalog.ActionECSListClusters, resource) {
		s.writeECSError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform ecs:ListClusters.", readOnly, eventID, verified)
		return
	}

	region := s.ecsRegion(verified)
	clusters, err := s.store.ListClusters(verified.AccountID, region)
	if err != nil {
		s.writeECSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list clusters.", readOnly, eventID, verified)
		return
	}
	arns := make([]string, 0, len(clusters))
	for _, c := range clusters {
		arns = append(arns, c.ClusterARN)
	}

	payload, err := ecssvc.ListClustersJSON(arns)
	if err != nil {
		s.writeECSError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeECSOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ecsEventSource, "ListClusters", readOnly)
}

func (s *Server) writeECSOK(w http.ResponseWriter, requestID string, payload []byte) {
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", ecsJSONContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeECSError(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID string,
	status int,
	code, message string,
	readOnly bool,
	eventID string,
	verified *authn.Verified,
) {
	_ = body
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", ecsJSONContentType)
	w.WriteHeader(status)
	payload, _ := json.Marshal(map[string]string{
		"__type":  code,
		"message": message,
	})
	_, _ = w.Write(payload)

	accessKeyID := ""
	accountID := ""
	if verified != nil {
		accessKeyID = verified.AccessKeyID
		accountID = verified.AccountID
	}
	s.auditAPIError(r, requestID, eventID, code, message, readOnly, accessKeyID, accountID, verified != nil)
}
