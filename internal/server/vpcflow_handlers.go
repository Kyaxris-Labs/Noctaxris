package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	ec2svc "github.com/Kyaxris-Labs/Noctaxris/internal/services/ec2"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const ec2EventSource = "ec2.amazonaws.com"

func (s *Server) handleVPCFlow(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = vpcFlowAction(action)

	switch action {
	case catalog.ActionEC2CreateFlowLogs:
		s.vpcCreateFlowLogs(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionEC2InjectFlowLogs:
		s.vpcInjectFlowLogs(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeVPCFlowError(w, r, body, requestID, http.StatusNotImplemented, "InvalidAction",
			"This EC2 action is not implemented.", readOnly, eventID, verified)
	}
}

func vpcFlowAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "CreateFlowLogs":
		return catalog.ActionEC2CreateFlowLogs
	case "InjectFlowLogs":
		return catalog.ActionEC2InjectFlowLogs
	default:
		return action
	}
}

func (s *Server) vpcCreateFlowLogs(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionEC2CreateFlowLogs, "*") {
		s.writeVPCFlowError(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform ec2:CreateFlowLogs.", readOnly, eventID, verified)
		return
	}
	var resourceIDs []string
	if raw, ok := params["ResourceIds"].([]any); ok {
		for _, v := range raw {
			if id, ok := v.(string); ok && id != "" {
				resourceIDs = append(resourceIDs, id)
			}
		}
	}
	resourceType, _ := params["ResourceType"].(string)
	if resourceType == "" {
		resourceType, _ = params["resourceType"].(string)
	}
	trafficType, _ := params["TrafficType"].(string)
	if trafficType == "" {
		trafficType, _ = params["trafficType"].(string)
	}
	logDestType, _ := params["LogDestinationType"].(string)
	if logDestType == "" {
		logDestType, _ = params["logDestinationType"].(string)
	}
	logDest, _ := params["LogDestination"].(string)
	if logDest == "" {
		logDest, _ = params["logDestination"].(string)
	}
	logFormat, _ := params["LogFormat"].(string)
	if logFormat == "" {
		logFormat, _ = params["logFormat"].(string)
	}
	deliverARN, _ := params["DeliverLogsPermissionArn"].(string)
	if deliverARN == "" {
		deliverARN, _ = params["deliverLogsPermissionArn"].(string)
	}
	region := verified.Region
	if region == "" {
		region = store.DefaultVPCFlowRegion
	}

	fl, err := s.store.CreateVPCFlowLog(verified.AccountID, store.CreateVPCFlowLogInput{
		ResourceIDs:              resourceIDs,
		ResourceType:             resourceType,
		TrafficType:              trafficType,
		LogDestinationType:       logDestType,
		LogDestination:           logDest,
		LogFormat:                logFormat,
		DeliverLogsPermissionArn: deliverARN,
		Region:                   region,
	})
	if errors.Is(err, store.ErrVPCFlowBadRequest) {
		s.writeVPCFlowError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterValue",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeVPCFlowError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create flow log.", readOnly, eventID, verified)
		return
	}
	payload, _ := ec2svc.CreateFlowLogsJSON([]string{fl.FlowLogID}, nil)
	s.writeVPCFlowOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ec2EventSource, "CreateFlowLogs", false,
		WithAuditRequestParameters(map[string]any{
			"flowLogId":          fl.FlowLogID,
			"logDestinationType": fl.LogDestinationType,
		}))
}

func (s *Server) vpcInjectFlowLogs(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.cfg.VPCFlowInject {
		s.writeVPCFlowError(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"ec2:InjectFlowLogs is disabled. Set NOCTAXRIS_VPCFLOW_INJECT=1.", readOnly, eventID, verified)
		return
	}
	if !s.authorize(verified, catalog.ActionEC2InjectFlowLogs, "*") {
		s.writeVPCFlowError(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform ec2:InjectFlowLogs.", readOnly, eventID, verified)
		return
	}
	flowLogID, _ := params["FlowLogId"].(string)
	if flowLogID == "" {
		flowLogID, _ = params["flowLogId"].(string)
	}
	if flowLogID == "" {
		s.writeVPCFlowError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterValue",
			"FlowLogId is required.", readOnly, eventID, verified)
		return
	}

	var records []store.VPCFlowRecord
	if rawLines, ok := params["Lines"].([]any); ok {
		for _, item := range rawLines {
			line, ok := item.(string)
			if !ok || strings.TrimSpace(line) == "" {
				continue
			}
			if rec, ok := parseVPCFlowV2Line(line, verified.AccountID); ok {
				records = append(records, rec)
			}
		}
	}
	if rawRecs, ok := params["Records"].([]any); ok {
		for _, item := range rawRecs {
			b, _ := json.Marshal(item)
			var rec store.VPCFlowRecord
			if err := json.Unmarshal(b, &rec); err != nil {
				s.writeVPCFlowError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterValue",
					"Invalid Records entry.", readOnly, eventID, verified)
				return
			}
			records = append(records, rec)
		}
	}

	delivered, err := s.store.InjectVPCFlowLogs(verified.AccountID, flowLogID, records, time.Now().UTC())
	if errors.Is(err, store.ErrVPCFlowNotFound) {
		s.writeVPCFlowError(w, r, body, requestID, http.StatusBadRequest, "InvalidFlowLogId.NotFound",
			"The flow log ID does not exist.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrVPCFlowBadRequest) {
		s.writeVPCFlowError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterValue",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeVPCFlowError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to inject flow logs.", readOnly, eventID, verified)
		return
	}
	payload, _ := ec2svc.InjectFlowLogsJSON(delivered, flowLogID)
	s.writeVPCFlowOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ec2EventSource, "InjectFlowLogs", false,
		WithAuditRequestParameters(map[string]any{"flowLogId": flowLogID, "delivered": delivered}))
}

func parseVPCFlowV2Line(line, defaultAccount string) (store.VPCFlowRecord, bool) {
	parts := strings.Fields(strings.TrimSpace(line))
	if len(parts) < 14 {
		return store.VPCFlowRecord{}, false
	}
	version, _ := strconv.Atoi(parts[0])
	srcPort, _ := strconv.Atoi(parts[5])
	dstPort, _ := strconv.Atoi(parts[6])
	protocol, _ := strconv.Atoi(parts[7])
	packets, _ := strconv.ParseInt(parts[8], 10, 64)
	bytesVal, _ := strconv.ParseInt(parts[9], 10, 64)
	start, _ := strconv.ParseInt(parts[10], 10, 64)
	end, _ := strconv.ParseInt(parts[11], 10, 64)
	accountID := parts[1]
	if accountID == "" {
		accountID = defaultAccount
	}
	return store.VPCFlowRecord{
		Version: version, AccountID: accountID, InterfaceID: parts[2],
		SrcAddr: parts[3], DstAddr: parts[4], SrcPort: srcPort, DstPort: dstPort,
		Protocol: protocol, Packets: packets, Bytes: bytesVal, Start: start, End: end,
		Action: parts[12], LogStatus: parts[13],
	}, true
}

func (s *Server) writeVPCFlowOK(w http.ResponseWriter, payload []byte) {
	w.Header().Set("Content-Type", "application/x-amz-json-1.1")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeVPCFlowError(
	w http.ResponseWriter, r *http.Request, body []byte, requestID string,
	status int, code, message string, readOnly bool, eventID string, verified *authn.Verified,
) {
	ak, acct := "", ""
	known := false
	if verified != nil {
		ak, acct, known = verified.AccessKeyID, verified.AccountID, true
	}
	s.writeAPIError(w, r, body, requestID, status, code, message, readOnly, eventID, ak, acct, known)
}
