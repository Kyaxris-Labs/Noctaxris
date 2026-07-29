package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	lightsailsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/lightsail"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	lightsailJSONContentType = "application/x-amz-json-1.1"
	lightsailEventSource     = "lightsail.amazonaws.com"
)

func (s *Server) handleLightsail(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = lightsailAction(action)

	switch action {
	case catalog.ActionLightsailGetBlueprints:
		s.lightsailGetBlueprints(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionLightsailGetBundles:
		s.lightsailGetBundles(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionLightsailCreateInstances:
		s.lightsailCreateInstances(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLightsailGetInstance:
		s.lightsailGetInstance(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLightsailGetInstances:
		s.lightsailGetInstances(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionLightsailStartInstance:
		s.lightsailMutateInstance(w, r, body, requestID, eventID, verified, readOnly, params, "start")
	case catalog.ActionLightsailStopInstance:
		s.lightsailMutateInstance(w, r, body, requestID, eventID, verified, readOnly, params, "stop")
	case catalog.ActionLightsailRebootInstance:
		s.lightsailMutateInstance(w, r, body, requestID, eventID, verified, readOnly, params, "reboot")
	case catalog.ActionLightsailDeleteInstance:
		s.lightsailMutateInstance(w, r, body, requestID, eventID, verified, readOnly, params, "delete")
	case catalog.ActionLightsailCreateDisk:
		s.lightsailCreateDisk(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLightsailGetDisk:
		s.lightsailGetDisk(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLightsailGetDisks:
		s.lightsailGetDisks(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionLightsailDeleteDisk:
		s.lightsailDeleteDisk(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLightsailAttachDisk:
		s.lightsailAttachDisk(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLightsailDetachDisk:
		s.lightsailDetachDisk(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLightsailAllocateStaticIp:
		s.lightsailAllocateStaticIP(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLightsailGetStaticIp:
		s.lightsailGetStaticIP(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLightsailGetStaticIps:
		s.lightsailGetStaticIPs(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionLightsailReleaseStaticIp:
		s.lightsailReleaseStaticIP(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLightsailAttachStaticIp:
		s.lightsailAttachStaticIP(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLightsailDetachStaticIp:
		s.lightsailDetachStaticIP(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLightsailCreateKeyPair:
		s.lightsailCreateKeyPair(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLightsailGetKeyPair:
		s.lightsailGetKeyPair(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLightsailGetKeyPairs:
		s.lightsailGetKeyPairs(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionLightsailDeleteKeyPair:
		s.lightsailDeleteKeyPair(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLightsailOpenInstancePublicPorts:
		s.lightsailOpenInstancePublicPorts(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLightsailCloseInstancePublicPorts:
		s.lightsailCloseInstancePublicPorts(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionLightsailGetInstancePortStates:
		s.lightsailGetInstancePortStates(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeLightsailError(w, r, body, requestID, http.StatusNotImplemented, "UnsupportedOperationException",
			"This Lightsail action is not implemented.", readOnly, eventID, verified)
	}
}

func lightsailAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "GetBlueprints":
		return catalog.ActionLightsailGetBlueprints
	case "GetBundles":
		return catalog.ActionLightsailGetBundles
	case "CreateInstances":
		return catalog.ActionLightsailCreateInstances
	case "GetInstance":
		return catalog.ActionLightsailGetInstance
	case "GetInstances":
		return catalog.ActionLightsailGetInstances
	case "StartInstance":
		return catalog.ActionLightsailStartInstance
	case "StopInstance":
		return catalog.ActionLightsailStopInstance
	case "RebootInstance":
		return catalog.ActionLightsailRebootInstance
	case "DeleteInstance":
		return catalog.ActionLightsailDeleteInstance
	case "CreateDisk":
		return catalog.ActionLightsailCreateDisk
	case "GetDisk":
		return catalog.ActionLightsailGetDisk
	case "GetDisks":
		return catalog.ActionLightsailGetDisks
	case "DeleteDisk":
		return catalog.ActionLightsailDeleteDisk
	case "AttachDisk":
		return catalog.ActionLightsailAttachDisk
	case "DetachDisk":
		return catalog.ActionLightsailDetachDisk
	case "AllocateStaticIp":
		return catalog.ActionLightsailAllocateStaticIp
	case "GetStaticIp":
		return catalog.ActionLightsailGetStaticIp
	case "GetStaticIps":
		return catalog.ActionLightsailGetStaticIps
	case "ReleaseStaticIp":
		return catalog.ActionLightsailReleaseStaticIp
	case "AttachStaticIp":
		return catalog.ActionLightsailAttachStaticIp
	case "DetachStaticIp":
		return catalog.ActionLightsailDetachStaticIp
	case "CreateKeyPair":
		return catalog.ActionLightsailCreateKeyPair
	case "GetKeyPair":
		return catalog.ActionLightsailGetKeyPair
	case "GetKeyPairs":
		return catalog.ActionLightsailGetKeyPairs
	case "DeleteKeyPair":
		return catalog.ActionLightsailDeleteKeyPair
	case "OpenInstancePublicPorts":
		return catalog.ActionLightsailOpenInstancePublicPorts
	case "CloseInstancePublicPorts":
		return catalog.ActionLightsailCloseInstancePublicPorts
	case "GetInstancePortStates":
		return catalog.ActionLightsailGetInstancePortStates
	default:
		return action
	}
}

func (s *Server) lightsailRegion(verified *authn.Verified) string {
	if verified.Region != "" {
		return verified.Region
	}
	return store.DefaultLightsailRegion
}

func lightsailStringList(params map[string]any, keys ...string) []string {
	for _, key := range keys {
		if raw, ok := params[key]; ok {
			switch v := raw.(type) {
			case []any:
				out := make([]string, 0, len(v))
				for _, item := range v {
					if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
						out = append(out, s)
					}
				}
				return out
			case []string:
				return v
			}
		}
	}
	return nil
}

func lightsailString(params map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := params[key].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func lightsailInt(params map[string]any, keys ...string) (int, bool) {
	for _, key := range keys {
		switch n := params[key].(type) {
		case float64:
			return int(n), true
		case int:
			return n, true
		case int64:
			return int(n), true
		case json.Number:
			i, err := n.Int64()
			if err == nil {
				return int(i), true
			}
		}
	}
	return 0, false
}

func lightsailPortInfo(params map[string]any) map[string]any {
	for _, key := range []string{"portInfo", "PortInfo"} {
		if m, ok := params[key].(map[string]any); ok {
			return m
		}
	}
	return nil
}

func (s *Server) writeLightsailStoreErr(
	w http.ResponseWriter, r *http.Request, body []byte, requestID string,
	readOnly bool, eventID string, verified *authn.Verified, err error, notFoundMsg, failMsg string,
) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, store.ErrLightsailNotFound) {
		s.writeLightsailError(w, r, body, requestID, http.StatusBadRequest, "NotFoundException",
			notFoundMsg, readOnly, eventID, verified)
		return true
	}
	if errors.Is(err, store.ErrLightsailExists) || errors.Is(err, store.ErrLightsailBadRequest) {
		code := "InvalidInputException"
		if errors.Is(err, store.ErrLightsailExists) {
			code = "InvalidResourceNameException"
		}
		s.writeLightsailError(w, r, body, requestID, http.StatusBadRequest, code,
			err.Error(), readOnly, eventID, verified)
		return true
	}
	s.writeLightsailError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
		failMsg, readOnly, eventID, verified)
	return true
}

func (s *Server) lightsailGetBlueprints(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool,
) {
	if !s.authorize(verified, catalog.ActionLightsailGetBlueprints, "*") {
		s.writeLightsailError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lightsail:GetBlueprints.", readOnly, eventID, verified)
		return
	}
	payload, _ := lightsailsvc.GetBlueprintsJSON()
	s.writeLightsailOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lightsailEventSource, "GetBlueprints", readOnly)
}

func (s *Server) lightsailGetBundles(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool,
) {
	if !s.authorize(verified, catalog.ActionLightsailGetBundles, "*") {
		s.writeLightsailError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lightsail:GetBundles.", readOnly, eventID, verified)
		return
	}
	payload, _ := lightsailsvc.GetBundlesJSON()
	s.writeLightsailOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lightsailEventSource, "GetBundles", readOnly)
}

func (s *Server) lightsailCreateInstances(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionLightsailCreateInstances, "*") {
		s.writeLightsailError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lightsail:CreateInstances.", readOnly, eventID, verified)
		return
	}
	names := lightsailStringList(params, "instanceNames", "InstanceNames")
	az := lightsailString(params, "availabilityZone", "AvailabilityZone")
	blueprintID := lightsailString(params, "blueprintId", "BlueprintId")
	bundleID := lightsailString(params, "bundleId", "BundleId")
	instances, err := s.store.CreateLightsailInstances(verified.AccountID, s.lightsailRegion(verified), names, az, blueprintID, bundleID)
	if errors.Is(err, store.ErrLightsailExists) {
		s.writeLightsailError(w, r, body, requestID, http.StatusBadRequest, "InvalidResourceNameException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrLightsailBadRequest) {
		s.writeLightsailError(w, r, body, requestID, http.StatusBadRequest, "InvalidInputException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeLightsailError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to create instances.", readOnly, eventID, verified)
		return
	}
	payload, _ := lightsailsvc.CreateInstancesJSON(instances)
	s.writeLightsailOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lightsailEventSource, "CreateInstances", readOnly)
}

func (s *Server) lightsailGetInstance(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name := lightsailString(params, "instanceName", "InstanceName")
	if !s.authorize(verified, catalog.ActionLightsailGetInstance, "*") {
		s.writeLightsailError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lightsail:GetInstance.", readOnly, eventID, verified)
		return
	}
	inst, err := s.store.GetLightsailInstance(verified.AccountID, s.lightsailRegion(verified), name)
	if errors.Is(err, store.ErrLightsailNotFound) {
		s.writeLightsailError(w, r, body, requestID, http.StatusBadRequest, "NotFoundException",
			"Instance not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeLightsailError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to get instance.", readOnly, eventID, verified)
		return
	}
	payload, _ := lightsailsvc.GetInstanceJSON(inst)
	s.writeLightsailOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lightsailEventSource, "GetInstance", readOnly)
}

func (s *Server) lightsailGetInstances(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool,
) {
	if !s.authorize(verified, catalog.ActionLightsailGetInstances, "*") {
		s.writeLightsailError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lightsail:GetInstances.", readOnly, eventID, verified)
		return
	}
	list, err := s.store.GetLightsailInstances(verified.AccountID, s.lightsailRegion(verified))
	if err != nil {
		s.writeLightsailError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to list instances.", readOnly, eventID, verified)
		return
	}
	payload, _ := lightsailsvc.GetInstancesJSON(list)
	s.writeLightsailOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lightsailEventSource, "GetInstances", readOnly)
}

func (s *Server) lightsailMutateInstance(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any, op string,
) {
	name := lightsailString(params, "instanceName", "InstanceName")
	var (
		action string
		opType string
		err    error
		inst   store.LightsailInstance
	)
	switch op {
	case "start":
		action, opType = catalog.ActionLightsailStartInstance, "StartInstance"
		if !s.authorize(verified, action, "*") {
			s.writeLightsailError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
				"User is not authorized to perform lightsail:StartInstance.", readOnly, eventID, verified)
			return
		}
		inst, err = s.store.StartLightsailInstance(verified.AccountID, s.lightsailRegion(verified), name)
	case "stop":
		action, opType = catalog.ActionLightsailStopInstance, "StopInstance"
		if !s.authorize(verified, action, "*") {
			s.writeLightsailError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
				"User is not authorized to perform lightsail:StopInstance.", readOnly, eventID, verified)
			return
		}
		inst, err = s.store.StopLightsailInstance(verified.AccountID, s.lightsailRegion(verified), name)
	case "reboot":
		action, opType = catalog.ActionLightsailRebootInstance, "RebootInstance"
		if !s.authorize(verified, action, "*") {
			s.writeLightsailError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
				"User is not authorized to perform lightsail:RebootInstance.", readOnly, eventID, verified)
			return
		}
		inst, err = s.store.RebootLightsailInstance(verified.AccountID, s.lightsailRegion(verified), name)
	case "delete":
		action, opType = catalog.ActionLightsailDeleteInstance, "DeleteInstance"
		if !s.authorize(verified, action, "*") {
			s.writeLightsailError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
				"User is not authorized to perform lightsail:DeleteInstance.", readOnly, eventID, verified)
			return
		}
		inst, err = s.store.GetLightsailInstance(verified.AccountID, s.lightsailRegion(verified), name)
		if err == nil {
			err = s.store.DeleteLightsailInstance(verified.AccountID, s.lightsailRegion(verified), name)
		}
	default:
		s.writeLightsailError(w, r, body, requestID, http.StatusBadRequest, "InvalidInputException",
			"Unknown instance operation.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrLightsailNotFound) {
		s.writeLightsailError(w, r, body, requestID, http.StatusBadRequest, "NotFoundException",
			"Instance not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeLightsailError(w, r, body, requestID, http.StatusInternalServerError, "ServiceException",
			"Unable to update instance.", readOnly, eventID, verified)
		return
	}
	payload, _ := lightsailsvc.InstanceOperationJSON(inst, opType)
	s.writeLightsailOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lightsailEventSource, opType, readOnly)
}

func (s *Server) lightsailCreateDisk(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionLightsailCreateDisk, "*") {
		s.writeLightsailError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lightsail:CreateDisk.", readOnly, eventID, verified)
		return
	}
	size, ok := lightsailInt(params, "sizeInGb", "SizeInGb")
	if !ok {
		s.writeLightsailError(w, r, body, requestID, http.StatusBadRequest, "InvalidInputException",
			"sizeInGb required", readOnly, eventID, verified)
		return
	}
	disk, err := s.store.CreateLightsailDisk(
		verified.AccountID, s.lightsailRegion(verified),
		lightsailString(params, "diskName", "DiskName"),
		lightsailString(params, "availabilityZone", "AvailabilityZone"),
		size,
	)
	if s.writeLightsailStoreErr(w, r, body, requestID, readOnly, eventID, verified, err, "Disk not found.", "Unable to create disk.") {
		return
	}
	payload, _ := lightsailsvc.DiskOperationJSON(disk, "CreateDisk")
	s.writeLightsailOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lightsailEventSource, "CreateDisk", readOnly)
}

func (s *Server) lightsailGetDisk(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionLightsailGetDisk, "*") {
		s.writeLightsailError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lightsail:GetDisk.", readOnly, eventID, verified)
		return
	}
	disk, err := s.store.GetLightsailDisk(verified.AccountID, s.lightsailRegion(verified), lightsailString(params, "diskName", "DiskName"))
	if s.writeLightsailStoreErr(w, r, body, requestID, readOnly, eventID, verified, err, "Disk not found.", "Unable to get disk.") {
		return
	}
	payload, _ := lightsailsvc.GetDiskJSON(disk)
	s.writeLightsailOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lightsailEventSource, "GetDisk", readOnly)
}

func (s *Server) lightsailGetDisks(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool,
) {
	if !s.authorize(verified, catalog.ActionLightsailGetDisks, "*") {
		s.writeLightsailError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lightsail:GetDisks.", readOnly, eventID, verified)
		return
	}
	list, err := s.store.GetLightsailDisks(verified.AccountID, s.lightsailRegion(verified))
	if s.writeLightsailStoreErr(w, r, body, requestID, readOnly, eventID, verified, err, "Disk not found.", "Unable to list disks.") {
		return
	}
	payload, _ := lightsailsvc.GetDisksJSON(list)
	s.writeLightsailOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lightsailEventSource, "GetDisks", readOnly)
}

func (s *Server) lightsailDeleteDisk(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionLightsailDeleteDisk, "*") {
		s.writeLightsailError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lightsail:DeleteDisk.", readOnly, eventID, verified)
		return
	}
	disk, err := s.store.DeleteLightsailDisk(verified.AccountID, s.lightsailRegion(verified), lightsailString(params, "diskName", "DiskName"))
	if s.writeLightsailStoreErr(w, r, body, requestID, readOnly, eventID, verified, err, "Disk not found.", "Unable to delete disk.") {
		return
	}
	payload, _ := lightsailsvc.DiskOperationJSON(disk, "DeleteDisk")
	s.writeLightsailOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lightsailEventSource, "DeleteDisk", readOnly)
}

func (s *Server) lightsailAttachDisk(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionLightsailAttachDisk, "*") {
		s.writeLightsailError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lightsail:AttachDisk.", readOnly, eventID, verified)
		return
	}
	disk, err := s.store.AttachLightsailDisk(
		verified.AccountID, s.lightsailRegion(verified),
		lightsailString(params, "diskName", "DiskName"),
		lightsailString(params, "instanceName", "InstanceName"),
		lightsailString(params, "diskPath", "DiskPath"),
	)
	if s.writeLightsailStoreErr(w, r, body, requestID, readOnly, eventID, verified, err, "Disk or instance not found.", "Unable to attach disk.") {
		return
	}
	payload, _ := lightsailsvc.DiskOperationJSON(disk, "AttachDisk")
	s.writeLightsailOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lightsailEventSource, "AttachDisk", readOnly)
}

func (s *Server) lightsailDetachDisk(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionLightsailDetachDisk, "*") {
		s.writeLightsailError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lightsail:DetachDisk.", readOnly, eventID, verified)
		return
	}
	disk, err := s.store.DetachLightsailDisk(verified.AccountID, s.lightsailRegion(verified), lightsailString(params, "diskName", "DiskName"))
	if s.writeLightsailStoreErr(w, r, body, requestID, readOnly, eventID, verified, err, "Disk not found.", "Unable to detach disk.") {
		return
	}
	payload, _ := lightsailsvc.DiskOperationJSON(disk, "DetachDisk")
	s.writeLightsailOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lightsailEventSource, "DetachDisk", readOnly)
}

func (s *Server) lightsailAllocateStaticIP(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionLightsailAllocateStaticIp, "*") {
		s.writeLightsailError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lightsail:AllocateStaticIp.", readOnly, eventID, verified)
		return
	}
	ip, err := s.store.AllocateLightsailStaticIP(
		verified.AccountID, s.lightsailRegion(verified), lightsailString(params, "staticIpName", "StaticIpName"),
	)
	if s.writeLightsailStoreErr(w, r, body, requestID, readOnly, eventID, verified, err, "Static IP not found.", "Unable to allocate static IP.") {
		return
	}
	payload, _ := lightsailsvc.StaticIPOperationJSON(ip, "AllocateStaticIp")
	s.writeLightsailOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lightsailEventSource, "AllocateStaticIp", readOnly)
}

func (s *Server) lightsailGetStaticIP(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionLightsailGetStaticIp, "*") {
		s.writeLightsailError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lightsail:GetStaticIp.", readOnly, eventID, verified)
		return
	}
	ip, err := s.store.GetLightsailStaticIP(
		verified.AccountID, s.lightsailRegion(verified), lightsailString(params, "staticIpName", "StaticIpName"),
	)
	if s.writeLightsailStoreErr(w, r, body, requestID, readOnly, eventID, verified, err, "Static IP not found.", "Unable to get static IP.") {
		return
	}
	payload, _ := lightsailsvc.GetStaticIPJSON(ip)
	s.writeLightsailOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lightsailEventSource, "GetStaticIp", readOnly)
}

func (s *Server) lightsailGetStaticIPs(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool,
) {
	if !s.authorize(verified, catalog.ActionLightsailGetStaticIps, "*") {
		s.writeLightsailError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lightsail:GetStaticIps.", readOnly, eventID, verified)
		return
	}
	list, err := s.store.GetLightsailStaticIPs(verified.AccountID, s.lightsailRegion(verified))
	if s.writeLightsailStoreErr(w, r, body, requestID, readOnly, eventID, verified, err, "Static IP not found.", "Unable to list static IPs.") {
		return
	}
	payload, _ := lightsailsvc.GetStaticIPsJSON(list)
	s.writeLightsailOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lightsailEventSource, "GetStaticIps", readOnly)
}

func (s *Server) lightsailReleaseStaticIP(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionLightsailReleaseStaticIp, "*") {
		s.writeLightsailError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lightsail:ReleaseStaticIp.", readOnly, eventID, verified)
		return
	}
	ip, err := s.store.ReleaseLightsailStaticIP(
		verified.AccountID, s.lightsailRegion(verified), lightsailString(params, "staticIpName", "StaticIpName"),
	)
	if s.writeLightsailStoreErr(w, r, body, requestID, readOnly, eventID, verified, err, "Static IP not found.", "Unable to release static IP.") {
		return
	}
	payload, _ := lightsailsvc.StaticIPOperationJSON(ip, "ReleaseStaticIp")
	s.writeLightsailOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lightsailEventSource, "ReleaseStaticIp", readOnly)
}

func (s *Server) lightsailAttachStaticIP(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionLightsailAttachStaticIp, "*") {
		s.writeLightsailError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lightsail:AttachStaticIp.", readOnly, eventID, verified)
		return
	}
	ip, err := s.store.AttachLightsailStaticIP(
		verified.AccountID, s.lightsailRegion(verified),
		lightsailString(params, "staticIpName", "StaticIpName"),
		lightsailString(params, "instanceName", "InstanceName"),
	)
	if s.writeLightsailStoreErr(w, r, body, requestID, readOnly, eventID, verified, err, "Static IP or instance not found.", "Unable to attach static IP.") {
		return
	}
	payload, _ := lightsailsvc.StaticIPOperationJSON(ip, "AttachStaticIp")
	s.writeLightsailOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lightsailEventSource, "AttachStaticIp", readOnly)
}

func (s *Server) lightsailDetachStaticIP(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionLightsailDetachStaticIp, "*") {
		s.writeLightsailError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lightsail:DetachStaticIp.", readOnly, eventID, verified)
		return
	}
	ip, err := s.store.DetachLightsailStaticIP(
		verified.AccountID, s.lightsailRegion(verified), lightsailString(params, "staticIpName", "StaticIpName"),
	)
	if s.writeLightsailStoreErr(w, r, body, requestID, readOnly, eventID, verified, err, "Static IP not found.", "Unable to detach static IP.") {
		return
	}
	payload, _ := lightsailsvc.StaticIPOperationJSON(ip, "DetachStaticIp")
	s.writeLightsailOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lightsailEventSource, "DetachStaticIp", readOnly)
}

func (s *Server) lightsailCreateKeyPair(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionLightsailCreateKeyPair, "*") {
		s.writeLightsailError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lightsail:CreateKeyPair.", readOnly, eventID, verified)
		return
	}
	kp, priv, err := s.store.CreateLightsailKeyPair(
		verified.AccountID, s.lightsailRegion(verified), lightsailString(params, "keyPairName", "KeyPairName"),
	)
	if s.writeLightsailStoreErr(w, r, body, requestID, readOnly, eventID, verified, err, "Key pair not found.", "Unable to create key pair.") {
		return
	}
	payload, _ := lightsailsvc.CreateKeyPairJSON(kp, priv)
	s.writeLightsailOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lightsailEventSource, "CreateKeyPair", readOnly)
}

func (s *Server) lightsailGetKeyPair(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionLightsailGetKeyPair, "*") {
		s.writeLightsailError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lightsail:GetKeyPair.", readOnly, eventID, verified)
		return
	}
	kp, err := s.store.GetLightsailKeyPair(
		verified.AccountID, s.lightsailRegion(verified), lightsailString(params, "keyPairName", "KeyPairName"),
	)
	if s.writeLightsailStoreErr(w, r, body, requestID, readOnly, eventID, verified, err, "Key pair not found.", "Unable to get key pair.") {
		return
	}
	payload, _ := lightsailsvc.GetKeyPairJSON(kp)
	s.writeLightsailOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lightsailEventSource, "GetKeyPair", readOnly)
}

func (s *Server) lightsailGetKeyPairs(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool,
) {
	if !s.authorize(verified, catalog.ActionLightsailGetKeyPairs, "*") {
		s.writeLightsailError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lightsail:GetKeyPairs.", readOnly, eventID, verified)
		return
	}
	list, err := s.store.GetLightsailKeyPairs(verified.AccountID, s.lightsailRegion(verified))
	if s.writeLightsailStoreErr(w, r, body, requestID, readOnly, eventID, verified, err, "Key pair not found.", "Unable to list key pairs.") {
		return
	}
	payload, _ := lightsailsvc.GetKeyPairsJSON(list)
	s.writeLightsailOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lightsailEventSource, "GetKeyPairs", readOnly)
}

func (s *Server) lightsailDeleteKeyPair(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionLightsailDeleteKeyPair, "*") {
		s.writeLightsailError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lightsail:DeleteKeyPair.", readOnly, eventID, verified)
		return
	}
	kp, err := s.store.DeleteLightsailKeyPair(
		verified.AccountID, s.lightsailRegion(verified), lightsailString(params, "keyPairName", "KeyPairName"),
	)
	if s.writeLightsailStoreErr(w, r, body, requestID, readOnly, eventID, verified, err, "Key pair not found.", "Unable to delete key pair.") {
		return
	}
	payload, _ := lightsailsvc.KeyPairOperationJSON(kp, "DeleteKeyPair")
	s.writeLightsailOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lightsailEventSource, "DeleteKeyPair", readOnly)
}

func (s *Server) lightsailOpenInstancePublicPorts(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionLightsailOpenInstancePublicPorts, "*") {
		s.writeLightsailError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lightsail:OpenInstancePublicPorts.", readOnly, eventID, verified)
		return
	}
	inst, _, err := s.store.OpenLightsailInstancePublicPorts(
		verified.AccountID, s.lightsailRegion(verified),
		lightsailString(params, "instanceName", "InstanceName"),
		lightsailPortInfo(params),
	)
	if s.writeLightsailStoreErr(w, r, body, requestID, readOnly, eventID, verified, err, "Instance not found.", "Unable to open ports.") {
		return
	}
	payload, _ := lightsailsvc.PortOperationJSON(inst, "OpenInstancePublicPorts")
	s.writeLightsailOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lightsailEventSource, "OpenInstancePublicPorts", readOnly)
}

func (s *Server) lightsailCloseInstancePublicPorts(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionLightsailCloseInstancePublicPorts, "*") {
		s.writeLightsailError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lightsail:CloseInstancePublicPorts.", readOnly, eventID, verified)
		return
	}
	inst, err := s.store.CloseLightsailInstancePublicPorts(
		verified.AccountID, s.lightsailRegion(verified),
		lightsailString(params, "instanceName", "InstanceName"),
		lightsailPortInfo(params),
	)
	if s.writeLightsailStoreErr(w, r, body, requestID, readOnly, eventID, verified, err, "Instance not found.", "Unable to close ports.") {
		return
	}
	payload, _ := lightsailsvc.PortOperationJSON(inst, "CloseInstancePublicPorts")
	s.writeLightsailOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lightsailEventSource, "CloseInstancePublicPorts", readOnly)
}

func (s *Server) lightsailGetInstancePortStates(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionLightsailGetInstancePortStates, "*") {
		s.writeLightsailError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform lightsail:GetInstancePortStates.", readOnly, eventID, verified)
		return
	}
	ports, err := s.store.GetLightsailInstancePortStates(
		verified.AccountID, s.lightsailRegion(verified), lightsailString(params, "instanceName", "InstanceName"),
	)
	if s.writeLightsailStoreErr(w, r, body, requestID, readOnly, eventID, verified, err, "Instance not found.", "Unable to get port states.") {
		return
	}
	payload, _ := lightsailsvc.GetInstancePortStatesJSON(ports)
	s.writeLightsailOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, lightsailEventSource, "GetInstancePortStates", readOnly)
}

func (s *Server) writeLightsailOK(w http.ResponseWriter, payload []byte) {
	w.Header().Set("Content-Type", lightsailJSONContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeLightsailError(
	w http.ResponseWriter, r *http.Request, body []byte, requestID string, status int, code, message string,
	readOnly bool, eventID string, verified *authn.Verified,
) {
	w.Header().Set("Content-Type", lightsailJSONContentType)
	w.Header().Set("x-amzn-ErrorType", code)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"__type": code, "message": message})
	_ = body
	s.writeSuccessAudit(r, requestID, eventID, verified, lightsailEventSource, code, readOnly)
}
