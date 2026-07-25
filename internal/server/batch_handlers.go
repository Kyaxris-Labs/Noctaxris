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
	batchsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/batch"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	batchJSONContentType = "application/json"
	batchEventSource     = "batch.amazonaws.com"
	defaultBatchEndpoint = "http://host.docker.internal:4566"
	batchJobSession      = "noctaxris-batch"
)

func (s *Server) handleBatch(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = batchAction(action)

	switch action {
	case catalog.ActionBatchCreateComputeEnvironment:
		s.batchCreateComputeEnvironment(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionBatchCreateJobQueue:
		s.batchCreateJobQueue(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionBatchRegisterJobDefinition:
		s.batchRegisterJobDefinition(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionBatchSubmitJob:
		s.batchSubmitJob(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionBatchDescribeComputeEnvironments:
		s.batchDescribeComputeEnvironments(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionBatchDescribeJobQueues:
		s.batchDescribeJobQueues(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionBatchDescribeJobDefinitions:
		s.batchDescribeJobDefinitions(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionBatchDescribeJobs:
		s.batchDescribeJobs(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeBatchError(w, r, body, requestID, http.StatusNotImplemented, "ClientException",
			"This Batch action is not implemented.", readOnly, eventID, verified)
	}
}

func batchAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "CreateComputeEnvironment":
		return catalog.ActionBatchCreateComputeEnvironment
	case "CreateJobQueue":
		return catalog.ActionBatchCreateJobQueue
	case "RegisterJobDefinition":
		return catalog.ActionBatchRegisterJobDefinition
	case "SubmitJob":
		return catalog.ActionBatchSubmitJob
	case "DescribeComputeEnvironments":
		return catalog.ActionBatchDescribeComputeEnvironments
	case "DescribeJobQueues":
		return catalog.ActionBatchDescribeJobQueues
	case "DescribeJobDefinitions":
		return catalog.ActionBatchDescribeJobDefinitions
	case "DescribeJobs":
		return catalog.ActionBatchDescribeJobs
	default:
		return action
	}
}

func isBatchRESTPath(path string) bool {
	return strings.HasPrefix(strings.ToLower(path), "/v1/")
}

func resolveBatchREST(r *http.Request) string {
	p := strings.ToLower(strings.TrimSuffix(r.URL.Path, "/"))
	switch p {
	case "/v1/createcomputeenvironment":
		return catalog.ActionBatchCreateComputeEnvironment
	case "/v1/createjobqueue":
		return catalog.ActionBatchCreateJobQueue
	case "/v1/registerjobdefinition":
		return catalog.ActionBatchRegisterJobDefinition
	case "/v1/submitjob":
		return catalog.ActionBatchSubmitJob
	case "/v1/describecomputeenvironments":
		return catalog.ActionBatchDescribeComputeEnvironments
	case "/v1/describejobqueues":
		return catalog.ActionBatchDescribeJobQueues
	case "/v1/describejobdefinitions":
		return catalog.ActionBatchDescribeJobDefinitions
	case "/v1/describejobs":
		return catalog.ActionBatchDescribeJobs
	default:
		return ""
	}
}

func (s *Server) batchRegion(verified *authn.Verified) string {
	if verified.Region != "" {
		return verified.Region
	}
	return store.DefaultBatchRegion
}

func (s *Server) checkBatchServicePassRole(verified *authn.Verified, roleARN string) error {
	if strings.TrimSpace(roleARN) == "" {
		return nil
	}
	accountID, roleName, ok := sts.ParseRoleARN(roleARN)
	if !ok {
		return errors.New("serviceRole must be a valid IAM role ARN")
	}
	if accountID != verified.AccountID {
		return errors.New("serviceRole must be in the same account")
	}
	storedARN, trust, err := s.store.GetRole(accountID, roleName)
	if err != nil {
		return errors.New("serviceRole not found")
	}
	if storedARN != "" {
		roleARN = storedARN
	}
	in, ok := s.evalInputs(verified)
	if !ok {
		return errors.New("not authorized to pass role to Batch")
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
		ServicePrincipal: authz.ServicePrincipalBatch,
	})
	if decision != authz.Allow {
		return errors.New("not authorized to pass role to Batch")
	}
	return nil
}

func (s *Server) checkBatchJobPassRole(verified *authn.Verified, roleARN string) error {
	if strings.TrimSpace(roleARN) == "" {
		return nil
	}
	accountID, roleName, ok := sts.ParseRoleARN(roleARN)
	if !ok {
		return errors.New("jobRoleArn must be a valid IAM role ARN")
	}
	if accountID != verified.AccountID {
		return errors.New("jobRoleArn must be in the same account")
	}
	storedARN, trust, err := s.store.GetRole(accountID, roleName)
	if err != nil {
		return errors.New("jobRoleArn not found")
	}
	if storedARN != "" {
		roleARN = storedARN
	}
	in, ok := s.evalInputs(verified)
	if !ok {
		return errors.New("not authorized to pass role to Batch")
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
		return errors.New("not authorized to pass role to Batch")
	}
	return nil
}

func (s *Server) batchCreateComputeEnvironment(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name := stringParam(params["computeEnvironmentName"])
	serviceRole := stringParam(params["serviceRole"])
	resource := store.BatchComputeEnvironmentARN(s.batchRegion(verified), verified.AccountID, name)
	if name == "" {
		resource = "*"
	}
	if !s.authorize(verified, catalog.ActionBatchCreateComputeEnvironment, resource) {
		s.writeBatchError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform batch:CreateComputeEnvironment.", readOnly, eventID, verified)
		return
	}
	if err := s.checkBatchServicePassRole(verified, serviceRole); err != nil {
		s.writeBatchError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	ce, err := s.store.CreateBatchComputeEnvironment(verified.AccountID, s.batchRegion(verified), store.CreateBatchComputeEnvironmentInput{
		Name:        name,
		Type:        stringParam(params["type"]),
		State:       stringParam(params["state"]),
		ServiceRole: serviceRole,
	})
	if errors.Is(err, store.ErrBatchAlreadyExists) {
		s.writeBatchError(w, r, body, requestID, http.StatusBadRequest, "ClientException",
			"Compute environment already exists.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrBatchInvalidInput) {
		s.writeBatchError(w, r, body, requestID, http.StatusBadRequest, "ClientException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeBatchError(w, r, body, requestID, http.StatusInternalServerError, "ServerException",
			"Unable to create compute environment.", readOnly, eventID, verified)
		return
	}
	payload, err := batchsvc.CreateComputeEnvironmentJSON(ce)
	if err != nil {
		s.writeBatchError(w, r, body, requestID, http.StatusInternalServerError, "ServerException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeBatchOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, batchEventSource, "CreateComputeEnvironment", readOnly)
}

func (s *Server) batchCreateJobQueue(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name := stringParam(params["jobQueueName"])
	resource := store.BatchJobQueueARN(s.batchRegion(verified), verified.AccountID, name)
	if name == "" {
		resource = "*"
	}
	if !s.authorize(verified, catalog.ActionBatchCreateJobQueue, resource) {
		s.writeBatchError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform batch:CreateJobQueue.", readOnly, eventID, verified)
		return
	}
	var order []map[string]any
	if raw, ok := params["computeEnvironmentOrder"].([]any); ok {
		for _, item := range raw {
			if m, ok := item.(map[string]any); ok {
				order = append(order, m)
			}
		}
	}
	priority := 1
	switch v := params["priority"].(type) {
	case float64:
		priority = int(v)
	case int:
		priority = v
	}
	jq, err := s.store.CreateBatchJobQueue(verified.AccountID, s.batchRegion(verified), store.CreateBatchJobQueueInput{
		Name:                    name,
		State:                   stringParam(params["state"]),
		Priority:                priority,
		ComputeEnvironmentOrder: order,
	})
	if errors.Is(err, store.ErrBatchAlreadyExists) {
		s.writeBatchError(w, r, body, requestID, http.StatusBadRequest, "ClientException",
			"Job queue already exists.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrBatchInvalidInput) {
		s.writeBatchError(w, r, body, requestID, http.StatusBadRequest, "ClientException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeBatchError(w, r, body, requestID, http.StatusInternalServerError, "ServerException",
			"Unable to create job queue.", readOnly, eventID, verified)
		return
	}
	payload, err := batchsvc.CreateJobQueueJSON(jq)
	if err != nil {
		s.writeBatchError(w, r, body, requestID, http.StatusInternalServerError, "ServerException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeBatchOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, batchEventSource, "CreateJobQueue", readOnly)
}

func (s *Server) batchRegisterJobDefinition(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name := stringParam(params["jobDefinitionName"])
	resource := fmt.Sprintf("arn:aws:batch:%s:%s:job-definition/%s", s.batchRegion(verified), verified.AccountID, name)
	if name == "" {
		resource = "*"
	}
	if !s.authorize(verified, catalog.ActionBatchRegisterJobDefinition, resource) {
		s.writeBatchError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform batch:RegisterJobDefinition.", readOnly, eventID, verified)
		return
	}
	container, _ := params["containerProperties"].(map[string]any)
	image := stringParam(container["image"])
	jobRole := stringParam(container["jobRoleArn"])
	execRole := stringParam(container["executionRoleArn"])
	if err := s.checkBatchJobPassRole(verified, jobRole); err != nil {
		s.writeBatchError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err := s.checkBatchJobPassRole(verified, execRole); err != nil {
		s.writeBatchError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err := batchRejectUnsupportedFargateShape(params, container); err != nil {
		s.writeBatchError(w, r, body, requestID, http.StatusBadRequest, "ClientException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	var cmd []string
	if raw, ok := container["command"].([]any); ok {
		for _, c := range raw {
			if s, ok := c.(string); ok {
				cmd = append(cmd, s)
			}
		}
	}
	env := map[string]string{}
	if raw, ok := container["environment"].([]any); ok {
		for _, item := range raw {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			n := stringParam(m["name"])
			if n != "" {
				env[n] = stringParam(m["value"])
			}
		}
	}
	jd, err := s.store.RegisterBatchJobDefinition(verified.AccountID, s.batchRegion(verified), store.RegisterBatchJobDefinitionInput{
		Name:             name,
		Type:             stringParam(params["type"]),
		Image:            image,
		Command:          cmd,
		JobRoleARN:       jobRole,
		ExecutionRoleARN: execRole,
		Env:              env,
	})
	if errors.Is(err, store.ErrBatchInvalidInput) {
		s.writeBatchError(w, r, body, requestID, http.StatusBadRequest, "ClientException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeBatchError(w, r, body, requestID, http.StatusInternalServerError, "ServerException",
			"Unable to register job definition.", readOnly, eventID, verified)
		return
	}
	payload, err := batchsvc.RegisterJobDefinitionJSON(jd)
	if err != nil {
		s.writeBatchError(w, r, body, requestID, http.StatusInternalServerError, "ServerException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeBatchOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, batchEventSource, "RegisterJobDefinition", readOnly)
}

func (s *Server) batchSubmitJob(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	jobName := stringParam(params["jobName"])
	queue := stringParam(params["jobQueue"])
	jobDef := stringParam(params["jobDefinition"])
	if !s.authorize(verified, catalog.ActionBatchSubmitJob, "*") {
		s.writeBatchError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform batch:SubmitJob.", readOnly, eventID, verified)
		return
	}
	if strings.TrimSpace(s.cfg.DockerHost) == "" {
		s.writeBatchError(w, r, body, requestID, http.StatusServiceUnavailable, "ServerException",
			"compute unavailable", readOnly, eventID, verified)
		return
	}
	jdPreview, err := s.store.GetBatchJobDefinition(verified.AccountID, jobDef)
	if err == nil {
		if err := s.checkBatchJobPassRole(verified, jdPreview.JobRoleARN); err != nil {
			s.writeBatchError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
				err.Error(), readOnly, eventID, verified)
			return
		}
		if err := s.checkBatchJobPassRole(verified, jdPreview.ExecutionRoleARN); err != nil {
			s.writeBatchError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
				err.Error(), readOnly, eventID, verified)
			return
		}
	}
	job, jd, err := s.store.SubmitBatchJob(verified.AccountID, s.batchRegion(verified), jobName, queue, jobDef)
	if errors.Is(err, store.ErrBatchInvalidInput) {
		s.writeBatchError(w, r, body, requestID, http.StatusBadRequest, "ClientException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeBatchError(w, r, body, requestID, http.StatusInternalServerError, "ServerException",
			"Unable to submit job.", readOnly, eventID, verified)
		return
	}
	// Start nested container; return SUBMITTED; background reap (ECS pattern).
	if err := s.startBatchJobContainer(r.Context(), verified.AccountID, job, jd, params); err != nil {
		_ = s.store.SetBatchJobRuntime(verified.AccountID, job.JobID, "", store.BatchJobStatusFailed, time.Now().UTC().Format(time.RFC3339))
		if strings.Contains(err.Error(), "compute unavailable") {
			s.writeBatchError(w, r, body, requestID, http.StatusServiceUnavailable, "ServerException",
				"compute unavailable", readOnly, eventID, verified)
			return
		}
		s.writeBatchError(w, r, body, requestID, http.StatusInternalServerError, "ServerException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	job.Status = store.BatchJobStatusSubmitted
	payload, err := batchsvc.SubmitJobJSON(job)
	if err != nil {
		s.writeBatchError(w, r, body, requestID, http.StatusInternalServerError, "ServerException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeBatchOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, batchEventSource, "SubmitJob", readOnly)
}

func batchRejectUnsupportedFargateShape(params, container map[string]any) error {
	if caps, ok := params["platformCapabilities"].([]any); ok {
		for _, c := range caps {
			if strings.EqualFold(stringParam(c), "FARGATE") {
				return fmt.Errorf("platformCapabilities FARGATE is not supported (lab nested Docker only)")
			}
		}
	}
	if nm := stringParam(container["networkMode"]); strings.EqualFold(nm, "awsvpc") {
		return fmt.Errorf("networkMode awsvpc is not supported (lab nested Docker only)")
	}
	return nil
}

func batchPullRoleARN(jd store.BatchJobDefinition) string {
	if role := strings.TrimSpace(jd.ExecutionRoleARN); role != "" {
		return role
	}
	return strings.TrimSpace(jd.JobRoleARN)
}

// startBatchJobContainer starts the nested container and returns once RUNNING is recorded.
// Exit reaping runs in the background.
func (s *Server) startBatchJobContainer(ctx context.Context, accountID string, job store.BatchJob, jd store.BatchJobDefinition, params map[string]any) error {
	cli, err := s.computeClient()
	if err != nil || cli == nil {
		return errors.New("compute unavailable")
	}
	cmd := jd.Command
	env := map[string]string{}
	_ = json.Unmarshal([]byte(jd.EnvJSON), &env)
	if overrides, ok := params["containerOverrides"].(map[string]any); ok {
		if raw, ok := overrides["command"].([]any); ok && len(raw) > 0 {
			cmd = nil
			for _, c := range raw {
				if s, ok := c.(string); ok {
					cmd = append(cmd, s)
				}
			}
		}
		if raw, ok := overrides["environment"].([]any); ok {
			for _, item := range raw {
				m, ok := item.(map[string]any)
				if !ok {
					continue
				}
				n := stringParam(m["name"])
				if n != "" {
					env[n] = stringParam(m["value"])
				}
			}
		}
	}
	endpoint := strings.TrimSpace(s.cfg.LambdaEndpointURL)
	if endpoint == "" {
		endpoint = defaultBatchEndpoint
	}
	env["AWS_BATCH_JOB_ID"] = job.JobID
	env["AWS_BATCH_JQ_NAME"] = job.JobQueue
	env["AWS_ENDPOINT_URL_BATCH"] = endpoint
	if roleARN := strings.TrimSpace(jd.JobRoleARN); roleARN != "" {
		minted, mintErr := s.mintRoleSessionEnv(roleARN, batchJobSession, endpoint, store.DefaultBatchRegion)
		if mintErr != nil {
			return mintErr
		}
		for k, v := range minted {
			env[k] = v
		}
	}
	pullRole := batchPullRoleARN(jd)
	pullRef, useAuth, username, password, err := s.labRegistryPullOpts(accountID, jd.Image, pullRole)
	if err != nil {
		return err
	}
	cid, err := cli.RunECSTask(ctx, compute.ECSRunOpts{
		ImageURI:         pullRef,
		Command:          cmd,
		Env:              env,
		EndpointURL:      endpoint,
		ListenAddr:       s.cfg.ListenAddr,
		LabRegistryPull:  useAuth,
		RegistryUsername: username,
		RegistryPassword: password,
	})
	if err != nil {
		return err
	}
	if err := s.store.SetBatchJobRuntime(accountID, job.JobID, cid, store.BatchJobStatusRunning, ""); err != nil {
		_ = cli.StopECSTask(ctx, cid)
		return err
	}
	go s.reapBatchJob(accountID, job.JobID, cid)
	return nil
}

func (s *Server) reapBatchJob(accountID, jobID, containerID string) {
	cli, err := s.computeClient()
	if err != nil || cli == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	exitCode, waitErr := cli.WaitECSTaskExit(ctx, containerID)
	status := store.BatchJobStatusSucceeded
	if waitErr != nil || exitCode != 0 {
		status = store.BatchJobStatusFailed
	}
	_ = cli.StopECSTask(context.Background(), containerID)
	_ = s.store.SetBatchJobRuntime(accountID, jobID, containerID, status, time.Now().UTC().Format(time.RFC3339))
}

func (s *Server) batchDescribeComputeEnvironments(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionBatchDescribeComputeEnvironments, "*") {
		s.writeBatchError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform batch:DescribeComputeEnvironments.", readOnly, eventID, verified)
		return
	}
	ces, err := s.store.DescribeBatchComputeEnvironments(verified.AccountID, stringSliceParam(params["computeEnvironments"]))
	if err != nil {
		s.writeBatchError(w, r, body, requestID, http.StatusInternalServerError, "ServerException",
			"Unable to describe compute environments.", readOnly, eventID, verified)
		return
	}
	payload, err := batchsvc.DescribeComputeEnvironmentsJSON(ces)
	if err != nil {
		s.writeBatchError(w, r, body, requestID, http.StatusInternalServerError, "ServerException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeBatchOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, batchEventSource, "DescribeComputeEnvironments", readOnly)
}

func (s *Server) batchDescribeJobQueues(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionBatchDescribeJobQueues, "*") {
		s.writeBatchError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform batch:DescribeJobQueues.", readOnly, eventID, verified)
		return
	}
	queues, err := s.store.DescribeBatchJobQueues(verified.AccountID, stringSliceParam(params["jobQueues"]))
	if err != nil {
		s.writeBatchError(w, r, body, requestID, http.StatusInternalServerError, "ServerException",
			"Unable to describe job queues.", readOnly, eventID, verified)
		return
	}
	payload, err := batchsvc.DescribeJobQueuesJSON(queues)
	if err != nil {
		s.writeBatchError(w, r, body, requestID, http.StatusInternalServerError, "ServerException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeBatchOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, batchEventSource, "DescribeJobQueues", readOnly)
}

func (s *Server) batchDescribeJobDefinitions(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionBatchDescribeJobDefinitions, "*") {
		s.writeBatchError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform batch:DescribeJobDefinitions.", readOnly, eventID, verified)
		return
	}
	defs, err := s.store.DescribeBatchJobDefinitions(verified.AccountID, stringParam(params["jobDefinitionName"]))
	if err != nil {
		s.writeBatchError(w, r, body, requestID, http.StatusInternalServerError, "ServerException",
			"Unable to describe job definitions.", readOnly, eventID, verified)
		return
	}
	payload, err := batchsvc.DescribeJobDefinitionsJSON(defs)
	if err != nil {
		s.writeBatchError(w, r, body, requestID, http.StatusInternalServerError, "ServerException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeBatchOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, batchEventSource, "DescribeJobDefinitions", readOnly)
}

func (s *Server) batchDescribeJobs(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionBatchDescribeJobs, "*") {
		s.writeBatchError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform batch:DescribeJobs.", readOnly, eventID, verified)
		return
	}
	jobs, err := s.store.DescribeBatchJobs(verified.AccountID, stringSliceParam(params["jobs"]))
	if err != nil {
		s.writeBatchError(w, r, body, requestID, http.StatusInternalServerError, "ServerException",
			"Unable to describe jobs.", readOnly, eventID, verified)
		return
	}
	payload, err := batchsvc.DescribeJobsJSON(jobs)
	if err != nil {
		s.writeBatchError(w, r, body, requestID, http.StatusInternalServerError, "ServerException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeBatchOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, batchEventSource, "DescribeJobs", readOnly)
}

func (s *Server) writeBatchOK(w http.ResponseWriter, requestID string, payload []byte) {
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", batchJSONContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeBatchError(
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
	w.Header().Set("Content-Type", batchJSONContentType)
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
