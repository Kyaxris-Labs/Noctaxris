package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	mqsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/mq"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	mqJSONContentType = "application/x-amz-json-1.0"
	mqEventSource     = "mq.amazonaws.com"
)

func (s *Server) handleMQ(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = mqAction(action)

	switch action {
	case catalog.ActionMQCreateBroker:
		s.mqCreate(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionMQDescribeBroker:
		s.mqDescribe(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionMQListBrokers:
		s.mqList(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionMQDeleteBroker:
		s.mqDelete(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeMQError(w, r, body, requestID, http.StatusNotImplemented, "InternalFailure",
			"This MQ action is not implemented.", readOnly, eventID, verified)
	}
}

func mqAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "CreateBroker":
		return catalog.ActionMQCreateBroker
	case "DescribeBroker":
		return catalog.ActionMQDescribeBroker
	case "ListBrokers":
		return catalog.ActionMQListBrokers
	case "DeleteBroker":
		return catalog.ActionMQDeleteBroker
	default:
		return action
	}
}

func (s *Server) mqRegion(verified *authn.Verified) string {
	if verified.Region != "" {
		return verified.Region
	}
	return store.DefaultMQRegion
}

func (s *Server) mqCreate(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name, _ := params["BrokerName"].(string)
	engine, _ := params["EngineType"].(string)
	version, _ := params["EngineVersion"].(string)
	deploy, _ := params["DeploymentMode"].(string)
	instance, _ := params["HostInstanceType"].(string)
	if publiclyAccessibleTrue(params["PubliclyAccessible"]) {
		s.writeMQError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			"PubliclyAccessible brokers are not supported (Internal nested network only).", readOnly, eventID, verified)
		return
	}
	if !s.authorize(verified, catalog.ActionMQCreateBroker, "*") {
		s.writeMQError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform mq:CreateBroker.", readOnly, eventID, verified)
		return
	}
	b, err := s.store.CreateMQBroker(verified.AccountID, s.mqRegion(verified), name, engine, version, deploy, instance)
	if errors.Is(err, store.ErrMQBrokerExists) {
		s.writeMQError(w, r, body, requestID, http.StatusConflict, "ConflictException",
			"Broker already exists.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrMQBadRequest) {
		s.writeMQError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeMQError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create broker.", readOnly, eventID, verified)
		return
	}
	if strings.EqualFold(b.EngineType, "RABBITMQ") {
		// Nested RabbitMQ on Internal noctaxris-data; no host port publish.
		_ = tryStartNestedDataEngine(s, verified.AccountID, "mq", b.BrokerID, map[string]string{
			"RABBITMQ_DEFAULT_USER": "noctaxris",
			"RABBITMQ_DEFAULT_PASS": "noctaxris-mq-lab",
		})
		if updated, err := s.store.DescribeMQBroker(verified.AccountID, b.BrokerID); err == nil {
			b = updated
		}
	}
	payload, _ := mqsvc.CreateBrokerJSON(b)
	s.writeMQOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, mqEventSource, "CreateBroker", readOnly)
}

func (s *Server) mqDescribe(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	id, _ := params["BrokerId"].(string)
	if !s.authorize(verified, catalog.ActionMQDescribeBroker, "*") {
		s.writeMQError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform mq:DescribeBroker.", readOnly, eventID, verified)
		return
	}
	b, err := s.store.DescribeMQBroker(verified.AccountID, id)
	if errors.Is(err, store.ErrMQBrokerNotFound) {
		s.writeMQError(w, r, body, requestID, http.StatusBadRequest, "NotFoundException",
			"Broker not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeMQError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe broker.", readOnly, eventID, verified)
		return
	}
	payload, _ := mqsvc.DescribeBrokerJSON(b)
	s.writeMQOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, mqEventSource, "DescribeBroker", readOnly)
}

func (s *Server) mqList(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	_ = params
	if !s.authorize(verified, catalog.ActionMQListBrokers, "*") {
		s.writeMQError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform mq:ListBrokers.", readOnly, eventID, verified)
		return
	}
	brokers, err := s.store.ListMQBrokers(verified.AccountID)
	if err != nil {
		s.writeMQError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list brokers.", readOnly, eventID, verified)
		return
	}
	payload, _ := mqsvc.ListBrokersJSON(brokers)
	s.writeMQOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, mqEventSource, "ListBrokers", readOnly)
}

func (s *Server) mqDelete(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	id, _ := params["BrokerId"].(string)
	if !s.authorize(verified, catalog.ActionMQDeleteBroker, "*") {
		s.writeMQError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform mq:DeleteBroker.", readOnly, eventID, verified)
		return
	}
	containerID, err := s.store.DeleteMQBroker(verified.AccountID, id)
	if errors.Is(err, store.ErrMQBrokerNotFound) {
		s.writeMQError(w, r, body, requestID, http.StatusBadRequest, "NotFoundException",
			"Broker not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeMQError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete broker.", readOnly, eventID, verified)
		return
	}
	_ = tryStopNestedDataEngine(s, containerID)
	w.Header().Set("Content-Type", mqJSONContentType)
	w.Header().Set("x-amzn-RequestId", requestID)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{}`))
	s.writeSuccessAudit(r, requestID, eventID, verified, mqEventSource, "DeleteBroker", readOnly)
}

func (s *Server) writeMQOK(w http.ResponseWriter, requestID string, payload []byte) {
	w.Header().Set("Content-Type", mqJSONContentType)
	w.Header().Set("x-amzn-RequestId", requestID)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeMQError(
	w http.ResponseWriter, r *http.Request, body []byte, requestID string,
	status int, code, message string, readOnly bool, eventID string, verified *authn.Verified,
) {
	w.Header().Set("Content-Type", mqJSONContentType)
	w.Header().Set("x-amzn-RequestId", requestID)
	w.Header().Set("x-amzn-ErrorType", code)
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"__type":"` + code + `","message":"` + message + `"}`))
	_ = body
	_ = r
	_ = readOnly
	_ = eventID
	_ = verified
}

func publiclyAccessibleTrue(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return strings.EqualFold(strings.TrimSpace(t), "true")
	default:
		return false
	}
}
