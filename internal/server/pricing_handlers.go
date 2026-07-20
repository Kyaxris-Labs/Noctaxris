package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	pricingsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/pricing"
)

const (
	pricingJSONContentType = "application/x-amz-json-1.1"
	pricingEventSource     = "pricing.amazonaws.com"
)

func (s *Server) handlePricing(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = pricingAction(action)

	switch action {
	case catalog.ActionPricingDescribeServices:
		s.pricingDescribeServices(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionPricingGetAttributeValues:
		s.pricingGetAttributeValues(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionPricingGetProducts:
		s.pricingGetProducts(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writePricingError(w, r, body, requestID, http.StatusNotImplemented, "InvalidAction",
			"This Pricing action is not implemented.", readOnly, eventID, verified)
	}
}

func pricingAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "DescribeServices":
		return catalog.ActionPricingDescribeServices
	case "GetAttributeValues":
		return catalog.ActionPricingGetAttributeValues
	case "GetProducts":
		return catalog.ActionPricingGetProducts
	default:
		return action
	}
}

func (s *Server) pricingDescribeServices(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	code, _ := params["ServiceCode"].(string)
	if !s.authorize(verified, catalog.ActionPricingDescribeServices, "*") {
		s.writePricingError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform pricing:DescribeServices.", readOnly, eventID, verified)
		return
	}
	services, err := s.store.DescribePricingServices(code)
	if err != nil {
		s.writePricingError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe services.", readOnly, eventID, verified)
		return
	}
	payload, _ := pricingsvc.DescribeServicesJSON(services)
	s.writePricingOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, pricingEventSource, "DescribeServices", readOnly)
}

func (s *Server) pricingGetAttributeValues(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	code, _ := params["ServiceCode"].(string)
	attr, _ := params["AttributeName"].(string)
	if !s.authorize(verified, catalog.ActionPricingGetAttributeValues, "*") {
		s.writePricingError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform pricing:GetAttributeValues.", readOnly, eventID, verified)
		return
	}
	values, err := s.store.GetPricingAttributeValues(code, attr)
	if err != nil {
		s.writePricingError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	payload, _ := pricingsvc.GetAttributeValuesJSON(values)
	s.writePricingOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, pricingEventSource, "GetAttributeValues", readOnly)
}

func (s *Server) pricingGetProducts(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	code, _ := params["ServiceCode"].(string)
	filters := map[string]string{}
	if raw, ok := params["Filters"].([]any); ok {
		for _, item := range raw {
			m, _ := item.(map[string]any)
			field, _ := m["Field"].(string)
			value, _ := m["Value"].(string)
			if field != "" && value != "" {
				filters[field] = value
			}
		}
	}
	if !s.authorize(verified, catalog.ActionPricingGetProducts, "*") {
		s.writePricingError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform pricing:GetProducts.", readOnly, eventID, verified)
		return
	}
	products, err := s.store.GetPricingProducts(code, filters)
	if err != nil {
		s.writePricingError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	payload, _ := pricingsvc.GetProductsJSON(products)
	s.writePricingOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, pricingEventSource, "GetProducts", readOnly)
}

func (s *Server) writePricingOK(w http.ResponseWriter, payload []byte) {
	w.Header().Set("Content-Type", pricingJSONContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writePricingError(
	w http.ResponseWriter, r *http.Request, body []byte, requestID string, status int, code, message string,
	readOnly bool, eventID string, verified *authn.Verified,
) {
	w.Header().Set("Content-Type", pricingJSONContentType)
	w.Header().Set("x-amzn-ErrorType", code)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"__type": code, "message": message})
	_ = body
	s.writeSuccessAudit(r, requestID, eventID, verified, pricingEventSource, code, readOnly)
}
