package server

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	asgsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/autoscaling"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

var (
	asgReconcilerOnce     sync.Once
	asgReconcilerStop     chan struct{}
	asgReconcilerStopOnce sync.Once
)

const asgEventSource = "autoscaling.amazonaws.com"

func (s *Server) handleAutoScaling(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := formParams(r, body)
	action = asgAction(action)

	switch action {
	case catalog.ActionASGCreateLaunchConfiguration:
		s.asgCreateLaunchConfiguration(w, r, requestID, eventID, verified, readOnly, params)
	case catalog.ActionASGDescribeLaunchConfigurations:
		s.asgDescribeLaunchConfigurations(w, r, requestID, eventID, verified, readOnly, params)
	case catalog.ActionASGDeleteLaunchConfiguration:
		s.asgDeleteLaunchConfiguration(w, r, requestID, eventID, verified, readOnly, params)
	case catalog.ActionASGCreateAutoScalingGroup:
		s.asgCreateGroup(w, r, requestID, eventID, verified, readOnly, params)
	case catalog.ActionASGDescribeAutoScalingGroups:
		s.asgDescribeGroups(w, r, requestID, eventID, verified, readOnly, params)
	case catalog.ActionASGUpdateAutoScalingGroup:
		s.asgUpdateGroup(w, r, requestID, eventID, verified, readOnly, params)
	case catalog.ActionASGDeleteAutoScalingGroup:
		s.asgDeleteGroup(w, r, requestID, eventID, verified, readOnly, params)
	case catalog.ActionASGSetDesiredCapacity:
		s.asgSetDesired(w, r, requestID, eventID, verified, readOnly, params)
	case catalog.ActionASGPutScalingPolicy:
		s.asgPutScalingPolicy(w, r, requestID, eventID, verified, readOnly, params)
	case catalog.ActionASGDescribePolicies:
		s.asgDescribePolicies(w, r, requestID, eventID, verified, readOnly, params)
	case catalog.ActionASGDeletePolicy:
		s.asgDeletePolicy(w, r, requestID, eventID, verified, readOnly, params)
	case catalog.ActionASGPutLifecycleHook:
		s.asgPutLifecycleHook(w, r, requestID, eventID, verified, readOnly, params)
	case catalog.ActionASGDescribeLifecycleHooks:
		s.asgDescribeLifecycleHooks(w, r, requestID, eventID, verified, readOnly, params)
	case catalog.ActionASGDeleteLifecycleHook:
		s.asgDeleteLifecycleHook(w, r, requestID, eventID, verified, readOnly, params)
	case catalog.ActionASGAttachInstances:
		s.asgAttachInstances(w, r, requestID, eventID, verified, readOnly, params)
	case catalog.ActionASGDetachInstances:
		s.asgDetachInstances(w, r, requestID, eventID, verified, readOnly, params)
	case catalog.ActionASGDescribeAutoScalingInstances:
		s.asgDescribeAutoScalingInstances(w, r, requestID, eventID, verified, readOnly, params)
	case catalog.ActionASGAttachLoadBalancerTargetGroups:
		s.asgAttachLoadBalancerTargetGroups(w, r, requestID, eventID, verified, readOnly, params)
	case catalog.ActionASGDetachLoadBalancerTargetGroups:
		s.asgDetachLoadBalancerTargetGroups(w, r, requestID, eventID, verified, readOnly, params)
	case catalog.ActionASGDescribeLoadBalancerTargetGroups:
		s.asgDescribeLoadBalancerTargetGroups(w, r, requestID, eventID, verified, readOnly, params)
	default:
		s.writeASGError(w, r, requestID, http.StatusBadRequest, "InvalidAction",
			"This Auto Scaling action is not implemented.", readOnly, eventID, verified)
	}
}

func asgAction(action string) string {
	if strings.HasPrefix(action, "autoscaling:") {
		return action
	}
	// Shared short names (e.g. DeletePolicy) may be normalized to another service.
	if i := strings.Index(action, ":"); i >= 0 {
		action = action[i+1:]
	}
	switch action {
	case "CreateLaunchConfiguration":
		return catalog.ActionASGCreateLaunchConfiguration
	case "DescribeLaunchConfigurations":
		return catalog.ActionASGDescribeLaunchConfigurations
	case "DeleteLaunchConfiguration":
		return catalog.ActionASGDeleteLaunchConfiguration
	case "CreateAutoScalingGroup":
		return catalog.ActionASGCreateAutoScalingGroup
	case "DescribeAutoScalingGroups":
		return catalog.ActionASGDescribeAutoScalingGroups
	case "UpdateAutoScalingGroup":
		return catalog.ActionASGUpdateAutoScalingGroup
	case "DeleteAutoScalingGroup":
		return catalog.ActionASGDeleteAutoScalingGroup
	case "SetDesiredCapacity":
		return catalog.ActionASGSetDesiredCapacity
	case "PutScalingPolicy":
		return catalog.ActionASGPutScalingPolicy
	case "DescribePolicies":
		return catalog.ActionASGDescribePolicies
	case "DeletePolicy":
		return catalog.ActionASGDeletePolicy
	case "PutLifecycleHook":
		return catalog.ActionASGPutLifecycleHook
	case "DescribeLifecycleHooks":
		return catalog.ActionASGDescribeLifecycleHooks
	case "DeleteLifecycleHook":
		return catalog.ActionASGDeleteLifecycleHook
	case "AttachInstances":
		return catalog.ActionASGAttachInstances
	case "DetachInstances":
		return catalog.ActionASGDetachInstances
	case "DescribeAutoScalingInstances":
		return catalog.ActionASGDescribeAutoScalingInstances
	case "AttachLoadBalancerTargetGroups":
		return catalog.ActionASGAttachLoadBalancerTargetGroups
	case "DetachLoadBalancerTargetGroups":
		return catalog.ActionASGDetachLoadBalancerTargetGroups
	case "DescribeLoadBalancerTargetGroups":
		return catalog.ActionASGDescribeLoadBalancerTargetGroups
	default:
		return action
	}
}

func (s *Server) asgRegion(verified *authn.Verified) string {
	if verified.Region != "" {
		return verified.Region
	}
	return store.DefaultASGRegion
}

func (s *Server) ensureASGReconciler() {
	asgReconcilerOnce.Do(func() {
		asgReconcilerStop = make(chan struct{})
		go s.runASGReconciler(asgReconcilerStop)
	})
}

// StopASGReconciler stops the ASG capacity reconciler if running.
func (s *Server) StopASGReconciler() {
	asgReconcilerStopOnce.Do(func() {
		if asgReconcilerStop != nil {
			close(asgReconcilerStop)
		}
	})
}

func (s *Server) runASGReconciler(stop <-chan struct{}) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			s.reconcileAllASGs(context.Background())
		}
	}
}

func (s *Server) reconcileAllASGs(ctx context.Context) {
	items, err := s.store.ListASGGroupsWithAccount()
	if err != nil {
		return
	}
	for _, item := range items {
		s.reconcileASGCapacity(ctx, item.AccountID, item.Region, item.Group.AutoScalingGroupName)
	}
}

// reconcileASGCapacity matches DesiredCapacity via store.RunInstances / terminate,
// then starts nested EC2 containers when the engine is configured.
func (s *Server) reconcileASGCapacity(ctx context.Context, accountID, region, name string) {
	beforeCIDs := map[string]string{}
	if groups, err := s.store.DescribeAutoScalingGroups(accountID, region, []string{name}); err == nil && len(groups) == 1 {
		for _, m := range groups[0].Instances {
			inst, err := s.store.GetEC2Instance(accountID, region, m.InstanceID)
			if err == nil && inst.ContainerID != "" {
				beforeCIDs[m.InstanceID] = inst.ContainerID
			}
		}
	}
	g, err := s.store.ReconcileASGCapacity(accountID, region, name)
	if err != nil {
		return
	}
	after := make(map[string]struct{}, len(g.Instances))
	for _, m := range g.Instances {
		after[m.InstanceID] = struct{}{}
	}
	if strings.TrimSpace(s.cfg.DockerHost) != "" {
		if cli, cerr := s.computeClient(); cerr == nil && cli != nil {
			for id, cid := range beforeCIDs {
				if _, ok := after[id]; !ok {
					_ = cli.TerminateEC2Instance(ctx, cid)
				}
			}
		}
		for _, m := range g.Instances {
			if m.LifecycleState != "Pending" {
				continue
			}
			inst, err := s.store.GetEC2Instance(accountID, region, m.InstanceID)
			if err != nil || inst.ContainerID != "" {
				continue
			}
			if err := s.startEC2Container(ctx, accountID, region, &inst); err != nil {
				log.Printf("asg: start ec2 container %s: %v (left pending)", m.InstanceID, err)
			}
		}
	}
}

func formMemberList(params url.Values, prefix string) []string {
	var out []string
	for i := 1; ; i++ {
		v := strings.TrimSpace(params.Get(prefix + ".member." + strconv.Itoa(i)))
		if v == "" {
			v = strings.TrimSpace(params.Get(prefix + "." + strconv.Itoa(i)))
		}
		if v == "" {
			break
		}
		out = append(out, v)
	}
	return out
}

func (s *Server) asgCreateLaunchConfiguration(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	if !s.authorize(verified, catalog.ActionASGCreateLaunchConfiguration, "*") {
		s.writeASGError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform autoscaling:CreateLaunchConfiguration.", readOnly, eventID, verified)
		return
	}
	_, err := s.store.CreateLaunchConfiguration(
		verified.AccountID, s.asgRegion(verified),
		params.Get("LaunchConfigurationName"), params.Get("ImageId"), params.Get("InstanceType"),
		params.Get("KeyName"), params.Get("UserData"), params.Get("IamInstanceProfile"),
		formMemberList(params, "SecurityGroups"),
	)
	if errors.Is(err, store.ErrASGExists) {
		s.writeASGError(w, r, requestID, http.StatusBadRequest, "AlreadyExists",
			"Launch configuration already exists.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrASGBadRequest) {
		s.writeASGError(w, r, requestID, http.StatusBadRequest, "ValidationError", err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeASGError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create launch configuration.", readOnly, eventID, verified)
		return
	}
	payload, _ := asgsvc.EmptyOKXML("CreateLaunchConfiguration", requestID)
	s.writeASGOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, asgEventSource, "CreateLaunchConfiguration", readOnly)
}

func (s *Server) asgDescribeLaunchConfigurations(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	if !s.authorize(verified, catalog.ActionASGDescribeLaunchConfigurations, "*") {
		s.writeASGError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform autoscaling:DescribeLaunchConfigurations.", readOnly, eventID, verified)
		return
	}
	names := formMemberList(params, "LaunchConfigurationNames")
	if n := strings.TrimSpace(params.Get("LaunchConfigurationName")); n != "" {
		names = append(names, n)
	}
	list, err := s.store.DescribeLaunchConfigurations(verified.AccountID, s.asgRegion(verified), names)
	if err != nil {
		s.writeASGError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe launch configurations.", readOnly, eventID, verified)
		return
	}
	payload, _ := asgsvc.DescribeLaunchConfigurationsXML(list, requestID)
	s.writeASGOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, asgEventSource, "DescribeLaunchConfigurations", readOnly)
}

func (s *Server) asgDeleteLaunchConfiguration(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	if !s.authorize(verified, catalog.ActionASGDeleteLaunchConfiguration, "*") {
		s.writeASGError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform autoscaling:DeleteLaunchConfiguration.", readOnly, eventID, verified)
		return
	}
	err := s.store.DeleteLaunchConfiguration(verified.AccountID, s.asgRegion(verified), params.Get("LaunchConfigurationName"))
	if errors.Is(err, store.ErrASGNotFound) {
		s.writeASGError(w, r, requestID, http.StatusBadRequest, "ValidationError",
			"Launch configuration not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeASGError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete launch configuration.", readOnly, eventID, verified)
		return
	}
	payload, _ := asgsvc.EmptyOKXML("DeleteLaunchConfiguration", requestID)
	s.writeASGOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, asgEventSource, "DeleteLaunchConfiguration", readOnly)
}

func (s *Server) asgCreateGroup(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	s.ensureASGReconciler()
	if !s.authorize(verified, catalog.ActionASGCreateAutoScalingGroup, "*") {
		s.writeASGError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform autoscaling:CreateAutoScalingGroup.", readOnly, eventID, verified)
		return
	}
	minSize, _ := strconv.Atoi(params.Get("MinSize"))
	maxSize, _ := strconv.Atoi(params.Get("MaxSize"))
	desired := minSize
	if v := params.Get("DesiredCapacity"); v != "" {
		desired, _ = strconv.Atoi(v)
	}
	cooldown, _ := strconv.Atoi(params.Get("DefaultCooldown"))
	region := s.asgRegion(verified)
	name := params.Get("AutoScalingGroupName")
	_, err := s.store.CreateAutoScalingGroup(
		verified.AccountID, region,
		name, params.Get("LaunchConfigurationName"),
		minSize, maxSize, desired, formMemberList(params, "AvailabilityZones"),
		params.Get("VPCZoneIdentifier"), params.Get("HealthCheckType"), cooldown,
	)
	if errors.Is(err, store.ErrASGExists) {
		s.writeASGError(w, r, requestID, http.StatusBadRequest, "AlreadyExists",
			"Auto scaling group already exists.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrASGBadRequest) {
		s.writeASGError(w, r, requestID, http.StatusBadRequest, "ValidationError", err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeASGError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create auto scaling group.", readOnly, eventID, verified)
		return
	}
	s.reconcileASGCapacity(r.Context(), verified.AccountID, region, name)
	payload, _ := asgsvc.EmptyOKXML("CreateAutoScalingGroup", requestID)
	s.writeASGOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, asgEventSource, "CreateAutoScalingGroup", readOnly)
}

func (s *Server) asgDescribeGroups(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	if !s.authorize(verified, catalog.ActionASGDescribeAutoScalingGroups, "*") {
		s.writeASGError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform autoscaling:DescribeAutoScalingGroups.", readOnly, eventID, verified)
		return
	}
	names := formMemberList(params, "AutoScalingGroupNames")
	list, err := s.store.DescribeAutoScalingGroups(verified.AccountID, s.asgRegion(verified), names)
	if err != nil {
		s.writeASGError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe auto scaling groups.", readOnly, eventID, verified)
		return
	}
	payload, _ := asgsvc.DescribeAutoScalingGroupsXML(list, requestID)
	s.writeASGOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, asgEventSource, "DescribeAutoScalingGroups", readOnly)
}

func (s *Server) asgUpdateGroup(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	s.ensureASGReconciler()
	if !s.authorize(verified, catalog.ActionASGUpdateAutoScalingGroup, "*") {
		s.writeASGError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform autoscaling:UpdateAutoScalingGroup.", readOnly, eventID, verified)
		return
	}
	var minSize, maxSize, desired, cooldown *int
	if v := params.Get("MinSize"); v != "" {
		n, _ := strconv.Atoi(v)
		minSize = &n
	}
	if v := params.Get("MaxSize"); v != "" {
		n, _ := strconv.Atoi(v)
		maxSize = &n
	}
	if v := params.Get("DesiredCapacity"); v != "" {
		n, _ := strconv.Atoi(v)
		desired = &n
	}
	if v := params.Get("DefaultCooldown"); v != "" {
		n, _ := strconv.Atoi(v)
		cooldown = &n
	}
	var lcName *string
	if v := params.Get("LaunchConfigurationName"); v != "" {
		lcName = &v
	}
	region := s.asgRegion(verified)
	name := params.Get("AutoScalingGroupName")
	_, err := s.store.UpdateAutoScalingGroup(
		verified.AccountID, region, name,
		minSize, maxSize, desired, lcName, formMemberList(params, "AvailabilityZones"), cooldown,
	)
	if errors.Is(err, store.ErrASGNotFound) {
		s.writeASGError(w, r, requestID, http.StatusBadRequest, "ValidationError",
			"Auto scaling group not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrASGBadRequest) {
		s.writeASGError(w, r, requestID, http.StatusBadRequest, "ValidationError", err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeASGError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to update auto scaling group.", readOnly, eventID, verified)
		return
	}
	s.reconcileASGCapacity(r.Context(), verified.AccountID, region, name)
	payload, _ := asgsvc.EmptyOKXML("UpdateAutoScalingGroup", requestID)
	s.writeASGOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, asgEventSource, "UpdateAutoScalingGroup", readOnly)
}

func (s *Server) asgDeleteGroup(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	if !s.authorize(verified, catalog.ActionASGDeleteAutoScalingGroup, "*") {
		s.writeASGError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform autoscaling:DeleteAutoScalingGroup.", readOnly, eventID, verified)
		return
	}
	region := s.asgRegion(verified)
	name := params.Get("AutoScalingGroupName")
	force := strings.EqualFold(strings.TrimSpace(params.Get("ForceDelete")), "true")

	beforeCIDs := map[string]string{}
	if groups, err := s.store.DescribeAutoScalingGroups(verified.AccountID, region, []string{name}); err == nil && len(groups) == 1 {
		for _, m := range groups[0].Instances {
			inst, err := s.store.GetEC2Instance(verified.AccountID, region, m.InstanceID)
			if err == nil && inst.ContainerID != "" {
				beforeCIDs[m.InstanceID] = inst.ContainerID
			}
		}
	}

	err := s.store.DeleteAutoScalingGroup(verified.AccountID, region, name, force)
	if errors.Is(err, store.ErrASGNotFound) {
		s.writeASGError(w, r, requestID, http.StatusBadRequest, "ValidationError",
			"Auto scaling group not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrASGBadRequest) {
		s.writeASGError(w, r, requestID, http.StatusBadRequest, "ValidationError", err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeASGError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete auto scaling group.", readOnly, eventID, verified)
		return
	}
	if force && strings.TrimSpace(s.cfg.DockerHost) != "" && len(beforeCIDs) > 0 {
		if cli, cerr := s.computeClient(); cerr == nil && cli != nil {
			for _, cid := range beforeCIDs {
				_ = cli.TerminateEC2Instance(r.Context(), cid)
			}
		}
	}
	payload, _ := asgsvc.EmptyOKXML("DeleteAutoScalingGroup", requestID)
	s.writeASGOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, asgEventSource, "DeleteAutoScalingGroup", readOnly)
}

func (s *Server) asgSetDesired(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	s.ensureASGReconciler()
	if !s.authorize(verified, catalog.ActionASGSetDesiredCapacity, "*") {
		s.writeASGError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform autoscaling:SetDesiredCapacity.", readOnly, eventID, verified)
		return
	}
	desired, _ := strconv.Atoi(params.Get("DesiredCapacity"))
	region := s.asgRegion(verified)
	name := params.Get("AutoScalingGroupName")
	err := s.store.SetDesiredCapacity(verified.AccountID, region, name, desired)
	if errors.Is(err, store.ErrASGNotFound) {
		s.writeASGError(w, r, requestID, http.StatusBadRequest, "ValidationError",
			"Auto scaling group not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrASGBadRequest) {
		s.writeASGError(w, r, requestID, http.StatusBadRequest, "ValidationError", err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeASGError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to set desired capacity.", readOnly, eventID, verified)
		return
	}
	s.reconcileASGCapacity(r.Context(), verified.AccountID, region, name)
	payload, _ := asgsvc.EmptyOKXML("SetDesiredCapacity", requestID)
	s.writeASGOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, asgEventSource, "SetDesiredCapacity", readOnly)
}

func (s *Server) asgPutScalingPolicy(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	if !s.authorize(verified, catalog.ActionASGPutScalingPolicy, "*") {
		s.writeASGError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform autoscaling:PutScalingPolicy.", readOnly, eventID, verified)
		return
	}
	var warmup *int
	if v := params.Get("EstimatedInstanceWarmup"); v != "" {
		n, _ := strconv.Atoi(v)
		warmup = &n
	}
	cooldown, _ := strconv.Atoi(params.Get("Cooldown"))
	adjustment, _ := strconv.Atoi(params.Get("ScalingAdjustment"))
	var tt *store.ASGTargetTrackingConfiguration
	metricType := params.Get("TargetTrackingConfiguration.PredefinedMetricSpecification.PredefinedMetricType")
	targetVal := params.Get("TargetTrackingConfiguration.TargetValue")
	if metricType != "" || targetVal != "" {
		tt = &store.ASGTargetTrackingConfiguration{
			PredefinedMetricType: metricType,
			ResourceLabel:        params.Get("TargetTrackingConfiguration.PredefinedMetricSpecification.ResourceLabel"),
		}
		if targetVal != "" {
			f, _ := strconv.ParseFloat(targetVal, 64)
			tt.TargetValue = f
		}
	}
	policy, err := s.store.PutScalingPolicy(
		verified.AccountID, s.asgRegion(verified),
		params.Get("AutoScalingGroupName"), params.Get("PolicyName"),
		params.Get("PolicyType"), params.Get("AdjustmentType"),
		adjustment, cooldown, warmup, tt,
	)
	if errors.Is(err, store.ErrASGNotFound) {
		s.writeASGError(w, r, requestID, http.StatusBadRequest, "ValidationError",
			"Auto scaling group not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrASGBadRequest) {
		s.writeASGError(w, r, requestID, http.StatusBadRequest, "ValidationError", err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeASGError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to put scaling policy.", readOnly, eventID, verified)
		return
	}
	payload, _ := asgsvc.PutScalingPolicyXML(policy.PolicyARN, requestID)
	s.writeASGOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, asgEventSource, "PutScalingPolicy", readOnly)
}

func (s *Server) asgDescribePolicies(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	if !s.authorize(verified, catalog.ActionASGDescribePolicies, "*") {
		s.writeASGError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform autoscaling:DescribePolicies.", readOnly, eventID, verified)
		return
	}
	list, err := s.store.DescribePolicies(
		verified.AccountID, s.asgRegion(verified),
		params.Get("AutoScalingGroupName"), formMemberList(params, "PolicyNames"),
	)
	if err != nil {
		s.writeASGError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe policies.", readOnly, eventID, verified)
		return
	}
	payload, _ := asgsvc.DescribePoliciesXML(list, requestID)
	s.writeASGOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, asgEventSource, "DescribePolicies", readOnly)
}

func (s *Server) asgDeletePolicy(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	if !s.authorize(verified, catalog.ActionASGDeletePolicy, "*") {
		s.writeASGError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform autoscaling:DeletePolicy.", readOnly, eventID, verified)
		return
	}
	nameOrARN := params.Get("PolicyName")
	if nameOrARN == "" {
		nameOrARN = params.Get("PolicyARN")
	}
	err := s.store.DeletePolicy(
		verified.AccountID, s.asgRegion(verified),
		params.Get("AutoScalingGroupName"), nameOrARN,
	)
	if errors.Is(err, store.ErrASGNotFound) {
		s.writeASGError(w, r, requestID, http.StatusBadRequest, "ValidationError",
			"Scaling policy not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrASGBadRequest) {
		s.writeASGError(w, r, requestID, http.StatusBadRequest, "ValidationError", err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeASGError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete policy.", readOnly, eventID, verified)
		return
	}
	payload, _ := asgsvc.EmptyOKXML("DeletePolicy", requestID)
	s.writeASGOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, asgEventSource, "DeletePolicy", readOnly)
}

func (s *Server) asgPutLifecycleHook(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	if !s.authorize(verified, catalog.ActionASGPutLifecycleHook, "*") {
		s.writeASGError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform autoscaling:PutLifecycleHook.", readOnly, eventID, verified)
		return
	}
	var timeout *int
	if v := params.Get("HeartbeatTimeout"); v != "" {
		n, _ := strconv.Atoi(v)
		timeout = &n
	}
	_, err := s.store.PutLifecycleHook(
		verified.AccountID, s.asgRegion(verified),
		params.Get("AutoScalingGroupName"), params.Get("LifecycleHookName"),
		params.Get("LifecycleTransition"), params.Get("NotificationTargetARN"),
		params.Get("RoleARN"), params.Get("NotificationMetadata"),
		timeout, params.Get("DefaultResult"),
	)
	if errors.Is(err, store.ErrASGNotFound) {
		s.writeASGError(w, r, requestID, http.StatusBadRequest, "ValidationError",
			"Auto scaling group not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrASGBadRequest) {
		s.writeASGError(w, r, requestID, http.StatusBadRequest, "ValidationError", err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeASGError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to put lifecycle hook.", readOnly, eventID, verified)
		return
	}
	payload, _ := asgsvc.EmptyOKXML("PutLifecycleHook", requestID)
	s.writeASGOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, asgEventSource, "PutLifecycleHook", readOnly)
}

func (s *Server) asgDescribeLifecycleHooks(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	if !s.authorize(verified, catalog.ActionASGDescribeLifecycleHooks, "*") {
		s.writeASGError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform autoscaling:DescribeLifecycleHooks.", readOnly, eventID, verified)
		return
	}
	list, err := s.store.DescribeLifecycleHooks(
		verified.AccountID, s.asgRegion(verified),
		params.Get("AutoScalingGroupName"), formMemberList(params, "LifecycleHookNames"),
	)
	if errors.Is(err, store.ErrASGNotFound) {
		s.writeASGError(w, r, requestID, http.StatusBadRequest, "ValidationError",
			"Auto scaling group not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrASGBadRequest) {
		s.writeASGError(w, r, requestID, http.StatusBadRequest, "ValidationError", err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeASGError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe lifecycle hooks.", readOnly, eventID, verified)
		return
	}
	payload, _ := asgsvc.DescribeLifecycleHooksXML(list, requestID)
	s.writeASGOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, asgEventSource, "DescribeLifecycleHooks", readOnly)
}

func (s *Server) asgDeleteLifecycleHook(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	if !s.authorize(verified, catalog.ActionASGDeleteLifecycleHook, "*") {
		s.writeASGError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform autoscaling:DeleteLifecycleHook.", readOnly, eventID, verified)
		return
	}
	err := s.store.DeleteLifecycleHook(
		verified.AccountID, s.asgRegion(verified),
		params.Get("AutoScalingGroupName"), params.Get("LifecycleHookName"),
	)
	if errors.Is(err, store.ErrASGNotFound) {
		s.writeASGError(w, r, requestID, http.StatusBadRequest, "ValidationError",
			"Lifecycle hook not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrASGBadRequest) {
		s.writeASGError(w, r, requestID, http.StatusBadRequest, "ValidationError", err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeASGError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete lifecycle hook.", readOnly, eventID, verified)
		return
	}
	payload, _ := asgsvc.EmptyOKXML("DeleteLifecycleHook", requestID)
	s.writeASGOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, asgEventSource, "DeleteLifecycleHook", readOnly)
}

func (s *Server) asgAttachInstances(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	if !s.authorize(verified, catalog.ActionASGAttachInstances, "*") {
		s.writeASGError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform autoscaling:AttachInstances.", readOnly, eventID, verified)
		return
	}
	err := s.store.AttachASGInstances(
		verified.AccountID, s.asgRegion(verified),
		params.Get("AutoScalingGroupName"), formMemberList(params, "InstanceIds"),
	)
	if errors.Is(err, store.ErrASGNotFound) {
		s.writeASGError(w, r, requestID, http.StatusBadRequest, "ValidationError",
			"Auto scaling group not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrASGBadRequest) {
		s.writeASGError(w, r, requestID, http.StatusBadRequest, "ValidationError", err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeASGError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to attach instances.", readOnly, eventID, verified)
		return
	}
	payload, _ := asgsvc.EmptyOKXML("AttachInstances", requestID)
	s.writeASGOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, asgEventSource, "AttachInstances", readOnly)
}

func (s *Server) asgDetachInstances(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	if !s.authorize(verified, catalog.ActionASGDetachInstances, "*") {
		s.writeASGError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform autoscaling:DetachInstances.", readOnly, eventID, verified)
		return
	}
	decrement := strings.EqualFold(strings.TrimSpace(params.Get("ShouldDecrementDesiredCapacity")), "true")
	err := s.store.DetachASGInstances(
		verified.AccountID, s.asgRegion(verified),
		params.Get("AutoScalingGroupName"), formMemberList(params, "InstanceIds"), decrement,
	)
	if errors.Is(err, store.ErrASGNotFound) {
		s.writeASGError(w, r, requestID, http.StatusBadRequest, "ValidationError",
			"Auto scaling group not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrASGBadRequest) {
		s.writeASGError(w, r, requestID, http.StatusBadRequest, "ValidationError", err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeASGError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to detach instances.", readOnly, eventID, verified)
		return
	}
	payload, _ := asgsvc.DetachInstancesXML(requestID)
	s.writeASGOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, asgEventSource, "DetachInstances", readOnly)
}

func (s *Server) asgDescribeAutoScalingInstances(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	if !s.authorize(verified, catalog.ActionASGDescribeAutoScalingInstances, "*") {
		s.writeASGError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform autoscaling:DescribeAutoScalingInstances.", readOnly, eventID, verified)
		return
	}
	list, err := s.store.DescribeAutoScalingInstances(
		verified.AccountID, s.asgRegion(verified), formMemberList(params, "InstanceIds"),
	)
	if err != nil {
		s.writeASGError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe auto scaling instances.", readOnly, eventID, verified)
		return
	}
	payload, _ := asgsvc.DescribeAutoScalingInstancesXML(list, requestID)
	s.writeASGOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, asgEventSource, "DescribeAutoScalingInstances", readOnly)
}

func (s *Server) asgAttachLoadBalancerTargetGroups(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	if !s.authorize(verified, catalog.ActionASGAttachLoadBalancerTargetGroups, "*") {
		s.writeASGError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform autoscaling:AttachLoadBalancerTargetGroups.", readOnly, eventID, verified)
		return
	}
	err := s.store.AttachLoadBalancerTargetGroups(
		verified.AccountID, s.asgRegion(verified),
		params.Get("AutoScalingGroupName"), formMemberList(params, "TargetGroupARNs"),
	)
	if errors.Is(err, store.ErrASGNotFound) {
		s.writeASGError(w, r, requestID, http.StatusBadRequest, "ValidationError",
			"Auto scaling group not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrASGBadRequest) {
		s.writeASGError(w, r, requestID, http.StatusBadRequest, "ValidationError", err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeASGError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to attach load balancer target groups.", readOnly, eventID, verified)
		return
	}
	payload, _ := asgsvc.EmptyOKXML("AttachLoadBalancerTargetGroups", requestID)
	s.writeASGOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, asgEventSource, "AttachLoadBalancerTargetGroups", readOnly)
}

func (s *Server) asgDetachLoadBalancerTargetGroups(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	if !s.authorize(verified, catalog.ActionASGDetachLoadBalancerTargetGroups, "*") {
		s.writeASGError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform autoscaling:DetachLoadBalancerTargetGroups.", readOnly, eventID, verified)
		return
	}
	err := s.store.DetachLoadBalancerTargetGroups(
		verified.AccountID, s.asgRegion(verified),
		params.Get("AutoScalingGroupName"), formMemberList(params, "TargetGroupARNs"),
	)
	if errors.Is(err, store.ErrASGNotFound) {
		s.writeASGError(w, r, requestID, http.StatusBadRequest, "ValidationError",
			"Auto scaling group not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrASGBadRequest) {
		s.writeASGError(w, r, requestID, http.StatusBadRequest, "ValidationError", err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeASGError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to detach load balancer target groups.", readOnly, eventID, verified)
		return
	}
	payload, _ := asgsvc.EmptyOKXML("DetachLoadBalancerTargetGroups", requestID)
	s.writeASGOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, asgEventSource, "DetachLoadBalancerTargetGroups", readOnly)
}

func (s *Server) asgDescribeLoadBalancerTargetGroups(
	w http.ResponseWriter, r *http.Request, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params url.Values,
) {
	if !s.authorize(verified, catalog.ActionASGDescribeLoadBalancerTargetGroups, "*") {
		s.writeASGError(w, r, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform autoscaling:DescribeLoadBalancerTargetGroups.", readOnly, eventID, verified)
		return
	}
	arns, err := s.store.DescribeLoadBalancerTargetGroups(
		verified.AccountID, s.asgRegion(verified), params.Get("AutoScalingGroupName"),
	)
	if errors.Is(err, store.ErrASGNotFound) {
		s.writeASGError(w, r, requestID, http.StatusBadRequest, "ValidationError",
			"Auto scaling group not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrASGBadRequest) {
		s.writeASGError(w, r, requestID, http.StatusBadRequest, "ValidationError", err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeASGError(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe load balancer target groups.", readOnly, eventID, verified)
		return
	}
	payload, _ := asgsvc.DescribeLoadBalancerTargetGroupsXML(arns, requestID)
	s.writeASGOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, asgEventSource, "DescribeLoadBalancerTargetGroups", readOnly)
}

func (s *Server) writeASGOK(w http.ResponseWriter, payload []byte) {
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeASGError(
	w http.ResponseWriter, r *http.Request, requestID string, status int, code, message string,
	readOnly bool, eventID string, verified *authn.Verified,
) {
	payload := []byte(`<?xml version="1.0" encoding="UTF-8"?>` +
		`<ErrorResponse xmlns="http://autoscaling.amazonaws.com/doc/2011-01-01/"><Error><Type>Sender</Type><Code>` +
		xmlEscape(code) + `</Code><Message>` + xmlEscape(message) +
		`</Message></Error><RequestId>` + xmlEscape(requestID) + `</RequestId></ErrorResponse>`)
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, asgEventSource, code, readOnly)
}

func xmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}
