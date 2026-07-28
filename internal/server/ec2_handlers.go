package server

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/compute"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	ec2svc "github.com/Kyaxris-Labs/Noctaxris/internal/services/ec2"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const defaultEC2Endpoint = "http://host.docker.internal:4566"

func (s *Server) handleEC2(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	action = ec2Action(action)
	switch action {
	case catalog.ActionEC2CreateFlowLogs, catalog.ActionEC2InjectFlowLogs:
		s.handleVPCFlow(w, r, body, requestID, eventID, action, verified, readOnly)
		return
	case catalog.ActionEC2RunInstances:
		s.ec2RunInstances(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionEC2DescribeInstances:
		s.ec2DescribeInstances(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionEC2DescribeImages:
		s.ec2DescribeImages(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionEC2TerminateInstances:
		s.ec2TerminateInstances(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionEC2StopInstances:
		s.ec2StopInstances(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionEC2StartInstances:
		s.ec2StartInstances(w, r, body, requestID, eventID, verified, readOnly)
	default:
		if s.handleEC2Network(w, r, body, requestID, eventID, action, verified, readOnly) {
			return
		}
		s.writeEC2Error(w, r, requestID, http.StatusBadRequest, "InvalidAction",
			"This EC2 action is not implemented.", readOnly, eventID, verified)
	}
}

func ec2Action(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "RunInstances":
		return catalog.ActionEC2RunInstances
	case "DescribeInstances":
		return catalog.ActionEC2DescribeInstances
	case "DescribeImages":
		return catalog.ActionEC2DescribeImages
	case "TerminateInstances":
		return catalog.ActionEC2TerminateInstances
	case "StopInstances":
		return catalog.ActionEC2StopInstances
	case "StartInstances":
		return catalog.ActionEC2StartInstances
	case "CreateVpc":
		return catalog.ActionEC2CreateVpc
	case "DeleteVpc":
		return catalog.ActionEC2DeleteVpc
	case "DescribeVpcs":
		return catalog.ActionEC2DescribeVpcs
	case "CreateSubnet":
		return catalog.ActionEC2CreateSubnet
	case "DeleteSubnet":
		return catalog.ActionEC2DeleteSubnet
	case "DescribeSubnets":
		return catalog.ActionEC2DescribeSubnets
	case "CreateSecurityGroup":
		return catalog.ActionEC2CreateSecurityGroup
	case "DeleteSecurityGroup":
		return catalog.ActionEC2DeleteSecurityGroup
	case "DescribeSecurityGroups":
		return catalog.ActionEC2DescribeSecurityGroups
	case "AuthorizeSecurityGroupIngress":
		return catalog.ActionEC2AuthorizeSecurityGroupIngress
	case "AuthorizeSecurityGroupEgress":
		return catalog.ActionEC2AuthorizeSecurityGroupEgress
	case "RevokeSecurityGroupIngress":
		return catalog.ActionEC2RevokeSecurityGroupIngress
	case "RevokeSecurityGroupEgress":
		return catalog.ActionEC2RevokeSecurityGroupEgress
	case "DescribeNetworkInterfaces":
		return catalog.ActionEC2DescribeNetworkInterfaces
	case "CreateNetworkInterface":
		return catalog.ActionEC2CreateNetworkInterface
	case "CreateFlowLogs":
		return catalog.ActionEC2CreateFlowLogs
	case "InjectFlowLogs":
		return catalog.ActionEC2InjectFlowLogs
	default:
		return action
	}
}

func (s *Server) ec2Region(verified *authn.Verified) string {
	if verified.Region != "" {
		return verified.Region
	}
	return store.DefaultEC2Region
}

func (s *Server) ec2RunInstances(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool,
) {
	if !s.authorize(verified, catalog.ActionEC2RunInstances, "*") {
		s.writeEC2Error(w, r, requestID, http.StatusForbidden, "UnauthorizedOperation",
			"User is not authorized to perform ec2:RunInstances.", readOnly, eventID, verified)
		return
	}
	params := formParams(r, body)
	minCount, _ := strconv.Atoi(strings.TrimSpace(params.Get("MinCount")))
	maxCount, _ := strconv.Atoi(strings.TrimSpace(params.Get("MaxCount")))
	region := s.ec2Region(verified)
	iamProfile := ec2IamInstanceProfile(params)
	instances, err := s.store.RunInstances(verified.AccountID, region, store.RunInstancesInput{
		ImageID:            params.Get("ImageId"),
		InstanceType:       params.Get("InstanceType"),
		MinCount:           minCount,
		MaxCount:           maxCount,
		KeyName:            params.Get("KeyName"),
		UserData:           params.Get("UserData"),
		IamInstanceProfile: iamProfile,
		AvailabilityZone:   params.Get("Placement.AvailabilityZone"),
	})
	if errors.Is(err, store.ErrEC2BadRequest) {
		s.writeEC2Error(w, r, requestID, http.StatusBadRequest, "InvalidParameterValue",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeEC2Error(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to run instances.", readOnly, eventID, verified)
		return
	}

	// Nested DinD: promote pending → running. Without engine, leave pending (documented).
	// UserData/IMDS failures inside start are logged and do not roll back the instance.
	if strings.TrimSpace(s.cfg.DockerHost) != "" {
		for i := range instances {
			if err := s.startEC2ContainerWithProfile(r.Context(), verified.AccountID, region, &instances[i], iamProfile); err != nil {
				log.Printf("ec2: start container %s: %v (left pending)", instances[i].InstanceID, err)
				continue
			}
		}
	}

	payload, err := ec2svc.RunInstancesXML(instances, requestID)
	if err != nil {
		s.writeEC2Error(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeEC2OK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ec2EventSource, "RunInstances", readOnly)
}

func (s *Server) startEC2Container(ctx context.Context, accountID, region string, inst *store.EC2Instance) error {
	return s.startEC2ContainerWithProfile(ctx, accountID, region, inst, inst.IamInstanceProfile)
}

func (s *Server) startEC2ContainerWithProfile(
	ctx context.Context, accountID, region string, inst *store.EC2Instance, iamProfile string,
) error {
	cli, err := s.computeClient()
	if err != nil || cli == nil {
		return errors.New("compute unavailable")
	}
	endpoint := strings.TrimSpace(s.cfg.LambdaEndpointURL)
	if endpoint == "" {
		endpoint = defaultEC2Endpoint
	}
	env := map[string]string{
		"NOCTAXRIS_INSTANCE_ID": inst.InstanceID,
		"NOCTAXRIS_AMI_ID":      inst.ImageID,
	}
	pullRef, useAuth, username, password, err := s.labRegistryPullOpts(accountID, inst.DockerImage, "")
	if err != nil {
		return err
	}
	profile := strings.TrimSpace(iamProfile)
	if profile == "" {
		profile = strings.TrimSpace(inst.IamInstanceProfile)
	}
	started, err := cli.StartEC2Instance(ctx, compute.EC2RunOpts{
		ImageURI:           pullRef,
		Env:                env,
		EndpointURL:        endpoint,
		ListenAddr:         s.cfg.ListenAddr,
		LabRegistryPull:    useAuth,
		RegistryUsername:   username,
		RegistryPassword:   password,
		InstanceID:         inst.InstanceID,
		AMIID:              inst.ImageID,
		IamInstanceProfile: profile,
		UserData:           inst.UserData,
	})
	if err != nil {
		return err
	}
	if started.UserDataErr != nil {
		log.Printf("ec2: UserData for %s: %v (instance still running)", inst.InstanceID, started.UserDataErr)
	}
	if started.IMDSErr != nil {
		log.Printf("ec2: IMDS for %s: %v (instance still running)", inst.InstanceID, started.IMDSErr)
	}
	cid := started.ContainerID
	if err := s.store.SetEC2ContainerID(accountID, region, inst.InstanceID, cid, store.EC2StateRunning); err != nil {
		_ = cli.TerminateEC2Instance(ctx, cid)
		return err
	}
	if ip := strings.TrimSpace(started.PrivateIP); ip != "" {
		if err := s.store.SetEC2PrivateIP(accountID, region, inst.InstanceID, ip); err != nil {
			log.Printf("ec2: set private IP for %s: %v", inst.InstanceID, err)
		} else {
			inst.PrivateIP = ip
		}
	}
	inst.ContainerID = cid
	inst.StateName = store.EC2StateRunning
	inst.StateCode = store.EC2StateCodeRunning
	return nil
}

func ec2IamInstanceProfile(params interface{ Get(string) string }) string {
	if name := strings.TrimSpace(params.Get("IamInstanceProfile.Name")); name != "" {
		return name
	}
	if arn := strings.TrimSpace(params.Get("IamInstanceProfile.Arn")); arn != "" {
		if i := strings.LastIndex(arn, "/"); i >= 0 && i+1 < len(arn) {
			return arn[i+1:]
		}
		return arn
	}
	return strings.TrimSpace(params.Get("IamInstanceProfile"))
}

func (s *Server) ec2DescribeInstances(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool,
) {
	if !s.authorize(verified, catalog.ActionEC2DescribeInstances, "*") {
		s.writeEC2Error(w, r, requestID, http.StatusForbidden, "UnauthorizedOperation",
			"User is not authorized to perform ec2:DescribeInstances.", readOnly, eventID, verified)
		return
	}
	params := formParams(r, body)
	ids := formMemberList(params, "InstanceId")
	list, err := s.store.DescribeInstances(verified.AccountID, s.ec2Region(verified), ids)
	if err != nil {
		s.writeEC2Error(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe instances.", readOnly, eventID, verified)
		return
	}
	payload, err := ec2svc.DescribeInstancesXML(list, requestID)
	if err != nil {
		s.writeEC2Error(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeEC2OK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ec2EventSource, "DescribeInstances", true)
}

func (s *Server) ec2DescribeImages(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool,
) {
	if !s.authorize(verified, catalog.ActionEC2DescribeImages, "*") {
		s.writeEC2Error(w, r, requestID, http.StatusForbidden, "UnauthorizedOperation",
			"User is not authorized to perform ec2:DescribeImages.", readOnly, eventID, verified)
		return
	}
	params := formParams(r, body)
	ids := formMemberList(params, "ImageId")
	list := store.DescribeEC2Images(ids)
	payload, err := ec2svc.DescribeImagesXML(list, requestID)
	if err != nil {
		s.writeEC2Error(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeEC2OK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ec2EventSource, "DescribeImages", true)
}

func (s *Server) ec2TerminateInstances(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool,
) {
	if !s.authorize(verified, catalog.ActionEC2TerminateInstances, "*") {
		s.writeEC2Error(w, r, requestID, http.StatusForbidden, "UnauthorizedOperation",
			"User is not authorized to perform ec2:TerminateInstances.", readOnly, eventID, verified)
		return
	}
	params := formParams(r, body)
	ids := formMemberList(params, "InstanceId")
	if len(ids) == 0 {
		s.writeEC2Error(w, r, requestID, http.StatusBadRequest, "MissingParameter",
			"InstanceId is required.", readOnly, eventID, verified)
		return
	}
	region := s.ec2Region(verified)
	var changes []ec2svc.StateChange
	for _, id := range ids {
		inst, err := s.store.GetEC2Instance(verified.AccountID, region, id)
		if errors.Is(err, store.ErrEC2NotFound) {
			s.writeEC2Error(w, r, requestID, http.StatusBadRequest, "InvalidInstanceID.NotFound",
				"The instance ID '"+id+"' does not exist", readOnly, eventID, verified)
			return
		}
		if err != nil {
			s.writeEC2Error(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to terminate instances.", readOnly, eventID, verified)
			return
		}
		prevName, prevCode := inst.StateName, inst.StateCode
		_ = s.store.SetEC2InstanceState(verified.AccountID, region, id, store.EC2StateShuttingDown)
		if inst.ContainerID != "" && strings.TrimSpace(s.cfg.DockerHost) != "" {
			if cli, cerr := s.computeClient(); cerr == nil && cli != nil {
				_ = cli.TerminateEC2Instance(r.Context(), inst.ContainerID)
			}
		}
		_ = s.store.ClearEC2ContainerID(verified.AccountID, region, id)
		_ = s.store.SetEC2InstanceState(verified.AccountID, region, id, store.EC2StateTerminated)
		changes = append(changes, ec2svc.StateChange{
			InstanceID: id, PreviousName: prevName, PreviousCode: prevCode,
			CurrentName: store.EC2StateTerminated, CurrentCode: store.EC2StateCodeTerminated,
		})
	}
	payload, _ := ec2svc.TerminateInstancesXML(changes, requestID)
	s.writeEC2OK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ec2EventSource, "TerminateInstances", readOnly)
}

func (s *Server) ec2StopInstances(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool,
) {
	if !s.authorize(verified, catalog.ActionEC2StopInstances, "*") {
		s.writeEC2Error(w, r, requestID, http.StatusForbidden, "UnauthorizedOperation",
			"User is not authorized to perform ec2:StopInstances.", readOnly, eventID, verified)
		return
	}
	params := formParams(r, body)
	ids := formMemberList(params, "InstanceId")
	if len(ids) == 0 {
		s.writeEC2Error(w, r, requestID, http.StatusBadRequest, "MissingParameter",
			"InstanceId is required.", readOnly, eventID, verified)
		return
	}
	region := s.ec2Region(verified)
	var changes []ec2svc.StateChange
	for _, id := range ids {
		inst, err := s.store.GetEC2Instance(verified.AccountID, region, id)
		if errors.Is(err, store.ErrEC2NotFound) {
			s.writeEC2Error(w, r, requestID, http.StatusBadRequest, "InvalidInstanceID.NotFound",
				"The instance ID '"+id+"' does not exist", readOnly, eventID, verified)
			return
		}
		if err != nil {
			s.writeEC2Error(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to stop instances.", readOnly, eventID, verified)
			return
		}
		prevName, prevCode := inst.StateName, inst.StateCode
		_ = s.store.SetEC2InstanceState(verified.AccountID, region, id, store.EC2StateStopping)
		if inst.ContainerID != "" && strings.TrimSpace(s.cfg.DockerHost) != "" {
			if cli, cerr := s.computeClient(); cerr == nil && cli != nil {
				_ = cli.StopEC2Instance(r.Context(), inst.ContainerID)
			}
		}
		_ = s.store.SetEC2InstanceState(verified.AccountID, region, id, store.EC2StateStopped)
		changes = append(changes, ec2svc.StateChange{
			InstanceID: id, PreviousName: prevName, PreviousCode: prevCode,
			CurrentName: store.EC2StateStopped, CurrentCode: store.EC2StateCodeStopped,
		})
	}
	payload, _ := ec2svc.StopInstancesXML(changes, requestID)
	s.writeEC2OK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ec2EventSource, "StopInstances", readOnly)
}

func (s *Server) ec2StartInstances(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool,
) {
	if !s.authorize(verified, catalog.ActionEC2StartInstances, "*") {
		s.writeEC2Error(w, r, requestID, http.StatusForbidden, "UnauthorizedOperation",
			"User is not authorized to perform ec2:StartInstances.", readOnly, eventID, verified)
		return
	}
	params := formParams(r, body)
	ids := formMemberList(params, "InstanceId")
	if len(ids) == 0 {
		s.writeEC2Error(w, r, requestID, http.StatusBadRequest, "MissingParameter",
			"InstanceId is required.", readOnly, eventID, verified)
		return
	}
	region := s.ec2Region(verified)
	var changes []ec2svc.StateChange
	for _, id := range ids {
		inst, err := s.store.GetEC2Instance(verified.AccountID, region, id)
		if errors.Is(err, store.ErrEC2NotFound) {
			s.writeEC2Error(w, r, requestID, http.StatusBadRequest, "InvalidInstanceID.NotFound",
				"The instance ID '"+id+"' does not exist", readOnly, eventID, verified)
			return
		}
		if err != nil {
			s.writeEC2Error(w, r, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to start instances.", readOnly, eventID, verified)
			return
		}
		prevName, prevCode := inst.StateName, inst.StateCode
		_ = s.store.SetEC2InstanceState(verified.AccountID, region, id, store.EC2StatePending)
		currentName, currentCode := store.EC2StatePending, store.EC2StateCodePending
		if inst.ContainerID != "" && strings.TrimSpace(s.cfg.DockerHost) != "" {
			if cli, cerr := s.computeClient(); cerr == nil && cli != nil {
				if err := cli.StartStoppedEC2Instance(r.Context(), inst.ContainerID); err == nil {
					_ = s.store.SetEC2InstanceState(verified.AccountID, region, id, store.EC2StateRunning)
					currentName, currentCode = store.EC2StateRunning, store.EC2StateCodeRunning
				}
			}
		} else if inst.ContainerID == "" && strings.TrimSpace(s.cfg.DockerHost) != "" {
			// Never had a container (launched without engine); try create now.
			updated := inst
			updated.StateName = store.EC2StatePending
			if err := s.startEC2Container(r.Context(), verified.AccountID, region, &updated); err == nil {
				currentName, currentCode = store.EC2StateRunning, store.EC2StateCodeRunning
			}
		}
		changes = append(changes, ec2svc.StateChange{
			InstanceID: id, PreviousName: prevName, PreviousCode: prevCode,
			CurrentName: currentName, CurrentCode: currentCode,
		})
	}
	payload, _ := ec2svc.StartInstancesXML(changes, requestID)
	s.writeEC2OK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ec2EventSource, "StartInstances", readOnly)
}

func (s *Server) writeEC2OK(w http.ResponseWriter, payload []byte) {
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeEC2Error(
	w http.ResponseWriter, r *http.Request, requestID string, status int, code, message string,
	readOnly bool, eventID string, verified *authn.Verified,
) {
	payload := ec2svc.ErrorXML(code, message, requestID)
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ec2EventSource, code, readOnly)
}
