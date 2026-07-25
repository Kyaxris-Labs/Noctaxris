package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	r53svc "github.com/Kyaxris-Labs/Noctaxris/internal/services/route53"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
	"github.com/google/uuid"
)

const (
	route53JSONContentType = "application/x-amz-json-1.1"
	route53EventSource     = "route53.amazonaws.com"
)

func (s *Server) handleRoute53(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = route53Action(action)

	switch action {
	case catalog.ActionRoute53CreateHostedZone:
		s.r53CreateHostedZone(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionRoute53DeleteHostedZone:
		s.r53DeleteHostedZone(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionRoute53ListHostedZones:
		s.r53ListHostedZones(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionRoute53ChangeResourceRecordSets:
		s.r53ChangeRRSets(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionRoute53ListResourceRecordSets:
		s.r53ListRRSets(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeRoute53Error(w, r, body, requestID, http.StatusNotImplemented, "InvalidAction",
			"This Route 53 action is not implemented.", readOnly, eventID, verified)
	}
}

func route53Action(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "CreateHostedZone":
		return catalog.ActionRoute53CreateHostedZone
	case "DeleteHostedZone":
		return catalog.ActionRoute53DeleteHostedZone
	case "ListHostedZones":
		return catalog.ActionRoute53ListHostedZones
	case "ChangeResourceRecordSets":
		return catalog.ActionRoute53ChangeResourceRecordSets
	case "ListResourceRecordSets":
		return catalog.ActionRoute53ListResourceRecordSets
	default:
		return action
	}
}

func (s *Server) r53CreateHostedZone(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name, _ := params["Name"].(string)
	caller, _ := params["CallerReference"].(string)
	private := false
	if cfg, ok := params["HostedZoneConfig"].(map[string]any); ok {
		if p, ok := cfg["PrivateZone"].(bool); ok {
			private = p
		}
	}
	if !s.authorize(verified, catalog.ActionRoute53CreateHostedZone, "*") {
		s.writeRoute53Error(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform route53:CreateHostedZone.", readOnly, eventID, verified)
		return
	}
	zone, err := s.store.CreateRoute53HostedZone(verified.AccountID, name, caller, private)
	if errors.Is(err, store.ErrRoute53BadRequest) {
		s.writeRoute53Error(w, r, body, requestID, http.StatusBadRequest, "InvalidInput",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeRoute53Error(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create hosted zone.", readOnly, eventID, verified)
		return
	}
	payload, _ := r53svc.CreateHostedZoneJSON(zone)
	s.writeRoute53OK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, route53EventSource, "CreateHostedZone", readOnly)
}

func (s *Server) r53DeleteHostedZone(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	zoneID, _ := params["Id"].(string)
	if zoneID == "" {
		zoneID, _ = params["HostedZoneId"].(string)
	}
	if !s.authorize(verified, catalog.ActionRoute53DeleteHostedZone, "*") {
		s.writeRoute53Error(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform route53:DeleteHostedZone.", readOnly, eventID, verified)
		return
	}
	err := s.store.DeleteRoute53HostedZone(verified.AccountID, zoneID)
	if errors.Is(err, store.ErrRoute53NotFound) {
		s.writeRoute53Error(w, r, body, requestID, http.StatusBadRequest, "NoSuchHostedZone",
			"Hosted zone not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeRoute53Error(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete hosted zone.", readOnly, eventID, verified)
		return
	}
	payload, _ := r53svc.DeleteHostedZoneJSON(zoneID)
	s.writeRoute53OK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, route53EventSource, "DeleteHostedZone", readOnly)
}

func (s *Server) r53ListHostedZones(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	_ = params
	if !s.authorize(verified, catalog.ActionRoute53ListHostedZones, "*") {
		s.writeRoute53Error(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform route53:ListHostedZones.", readOnly, eventID, verified)
		return
	}
	zones, err := s.store.ListRoute53HostedZones(verified.AccountID)
	if err != nil {
		s.writeRoute53Error(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list hosted zones.", readOnly, eventID, verified)
		return
	}
	payload, _ := r53svc.ListHostedZonesJSON(zones)
	s.writeRoute53OK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, route53EventSource, "ListHostedZones", readOnly)
}

func (s *Server) r53ChangeRRSets(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	zoneID, _ := params["HostedZoneId"].(string)
	if !s.authorize(verified, catalog.ActionRoute53ChangeResourceRecordSets, "*") {
		s.writeRoute53Error(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform route53:ChangeResourceRecordSets.", readOnly, eventID, verified)
		return
	}
	changes := parseRoute53Changes(params)
	err := s.store.ChangeRoute53ResourceRecordSets(verified.AccountID, zoneID, changes)
	if errors.Is(err, store.ErrRoute53NotFound) {
		s.writeRoute53Error(w, r, body, requestID, http.StatusBadRequest, "NoSuchHostedZone",
			"Hosted zone not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrRoute53BadRequest) {
		s.writeRoute53Error(w, r, body, requestID, http.StatusBadRequest, "InvalidChangeBatch",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeRoute53Error(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to change resource record sets.", readOnly, eventID, verified)
		return
	}
	payload, _ := r53svc.ChangeResourceRecordSetsJSON("C" + uuid.NewString())
	s.writeRoute53OK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, route53EventSource, "ChangeResourceRecordSets", readOnly)
}

func (s *Server) r53ListRRSets(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	zoneID, _ := params["HostedZoneId"].(string)
	if !s.authorize(verified, catalog.ActionRoute53ListResourceRecordSets, "*") {
		s.writeRoute53Error(w, r, body, requestID, http.StatusForbidden, "AccessDenied",
			"User is not authorized to perform route53:ListResourceRecordSets.", readOnly, eventID, verified)
		return
	}
	sets, err := s.store.ListRoute53ResourceRecordSets(verified.AccountID, zoneID)
	if errors.Is(err, store.ErrRoute53NotFound) {
		s.writeRoute53Error(w, r, body, requestID, http.StatusBadRequest, "NoSuchHostedZone",
			"Hosted zone not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeRoute53Error(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list resource record sets.", readOnly, eventID, verified)
		return
	}
	payload, err := listRoute53ResourceRecordSetsJSON(sets)
	if err != nil {
		s.writeRoute53Error(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to encode resource record sets.", readOnly, eventID, verified)
		return
	}
	s.writeRoute53OK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, route53EventSource, "ListResourceRecordSets", readOnly)
}

func parseRoute53Changes(params map[string]any) []store.Route53Change {
	var raw []any
	if batch, ok := params["ChangeBatch"].(map[string]any); ok {
		raw, _ = batch["Changes"].([]any)
	} else {
		raw, _ = params["Changes"].([]any)
	}
	var out []store.Route53Change
	for _, item := range raw {
		m, _ := item.(map[string]any)
		action, _ := m["Action"].(string)
		rrset, _ := m["ResourceRecordSet"].(map[string]any)
		name, _ := rrset["Name"].(string)
		rtype, _ := rrset["Type"].(string)
		ttl := 300
		switch v := rrset["TTL"].(type) {
		case float64:
			ttl = int(v)
		}
		var records []string
		if recs, ok := rrset["ResourceRecords"].([]any); ok {
			for _, r := range recs {
				rm, _ := r.(map[string]any)
				if v, ok := rm["Value"].(string); ok {
					records = append(records, v)
				}
			}
		}
		var alias *store.Route53AliasTarget
		if at, ok := rrset["AliasTarget"].(map[string]any); ok {
			dns, _ := at["DNSName"].(string)
			hz, _ := at["HostedZoneId"].(string)
			eth, _ := at["EvaluateTargetHealth"].(bool)
			alias = &store.Route53AliasTarget{
				DNSName:              dns,
				HostedZoneId:         hz,
				EvaluateTargetHealth: eth,
			}
		}
		out = append(out, store.Route53Change{
			Action: action, Name: name, Type: rtype, TTL: ttl, Records: records, AliasTarget: alias,
		})
	}
	return out
}

// listRoute53ResourceRecordSetsJSON encodes ListResourceRecordSets with AliasTarget when set.
func listRoute53ResourceRecordSetsJSON(sets []store.Route53ResourceRecordSet) ([]byte, error) {
	items := make([]map[string]any, 0, len(sets))
	for _, rs := range sets {
		item := map[string]any{
			"Name": rs.Name,
			"Type": rs.Type,
		}
		if rs.AliasTarget != nil {
			item["AliasTarget"] = map[string]any{
				"DNSName":              rs.AliasTarget.DNSName,
				"HostedZoneId":         rs.AliasTarget.HostedZoneId,
				"EvaluateTargetHealth": rs.AliasTarget.EvaluateTargetHealth,
			}
		} else {
			recs := make([]map[string]any, 0, len(rs.Records))
			for _, v := range rs.Records {
				recs = append(recs, map[string]any{"Value": v})
			}
			item["TTL"] = rs.TTL
			item["ResourceRecords"] = recs
		}
		items = append(items, item)
	}
	return json.Marshal(map[string]any{"ResourceRecordSets": items})
}

func (s *Server) writeRoute53OK(w http.ResponseWriter, payload []byte) {
	w.Header().Set("Content-Type", route53JSONContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeRoute53Error(
	w http.ResponseWriter, r *http.Request, body []byte, requestID string, status int, code, message string,
	readOnly bool, eventID string, verified *authn.Verified,
) {
	w.Header().Set("Content-Type", route53JSONContentType)
	w.Header().Set("x-amzn-ErrorType", code)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"__type": code, "message": message})
	_ = body
	s.writeSuccessAudit(r, requestID, eventID, verified, route53EventSource, code, readOnly)
}
