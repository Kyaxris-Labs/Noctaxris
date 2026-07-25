package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/sts"
	fhsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/firehose"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	firehoseJSONContentType = "application/x-amz-json-1.1"
	firehoseEventSource     = "firehose.amazonaws.com"
)

func (s *Server) handleFirehose(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = firehoseAction(action)

	switch action {
	case catalog.ActionFirehoseCreateDeliveryStream:
		s.fhCreate(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionFirehoseDeleteDeliveryStream:
		s.fhDelete(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionFirehoseDescribeDeliveryStream:
		s.fhDescribe(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionFirehoseListDeliveryStreams:
		s.fhList(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionFirehosePutRecord:
		s.fhPutRecord(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionFirehosePutRecordBatch:
		s.fhPutRecordBatch(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeFirehoseError(w, r, body, requestID, http.StatusNotImplemented, "InternalFailure",
			"This Firehose action is not implemented.", readOnly, eventID, verified)
	}
}

func firehoseAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "CreateDeliveryStream":
		return catalog.ActionFirehoseCreateDeliveryStream
	case "DeleteDeliveryStream":
		return catalog.ActionFirehoseDeleteDeliveryStream
	case "DescribeDeliveryStream":
		return catalog.ActionFirehoseDescribeDeliveryStream
	case "ListDeliveryStreams":
		return catalog.ActionFirehoseListDeliveryStreams
	case "PutRecord":
		return catalog.ActionFirehosePutRecord
	case "PutRecordBatch":
		return catalog.ActionFirehosePutRecordBatch
	default:
		return action
	}
}

func (s *Server) checkFirehosePassRole(verified *authn.Verified, roleARN string) error {
	accountID, roleName, ok := sts.ParseRoleARN(roleARN)
	if !ok {
		return errors.New("RoleARN must be a valid IAM role ARN")
	}
	if accountID != verified.AccountID {
		return errors.New("RoleARN must be in the same account")
	}
	storedARN, trust, err := s.store.GetRole(accountID, roleName)
	if err != nil {
		return errors.New("Role not found")
	}
	if storedARN != "" {
		roleARN = storedARN
	}
	in, ok := s.evalInputs(verified)
	if !ok {
		return errors.New("not authorized to pass role to Firehose")
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
		ServicePrincipal: authz.ServicePrincipalFirehose,
	})
	if decision != authz.Allow {
		return errors.New("not authorized to pass role to Firehose")
	}
	return nil
}

func (s *Server) fhCreate(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name, _ := params["DeliveryStreamName"].(string)
	if strings.TrimSpace(name) == "" {
		s.writeFirehoseError(w, r, body, requestID, http.StatusBadRequest, "InvalidArgumentException",
			"DeliveryStreamName is required.", readOnly, eventID, verified)
		return
	}
	if !s.authorize(verified, catalog.ActionFirehoseCreateDeliveryStream, "*") {
		s.writeFirehoseError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform firehose:CreateDeliveryStream.", readOnly, eventID, verified)
		return
	}
	destType, bucket, prefix, lambdaARN, roleARN, osDomain, osIndex := parseFirehoseDestination(params)
	if roleARN != "" {
		if err := s.checkFirehosePassRole(verified, roleARN); err != nil {
			s.writeFirehoseError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
				err.Error(), readOnly, eventID, verified)
			return
		}
	}
	region := verified.Region
	if region == "" {
		region = store.DefaultFirehoseRegion
	}
	var st store.FirehoseStream
	var err error
	if strings.EqualFold(destType, "OpenSearch") {
		st, err = s.store.CreateFirehoseOpenSearchStream(verified.AccountID, region, name, roleARN, osDomain, osIndex)
	} else {
		st, err = s.store.CreateFirehoseStream(verified.AccountID, region, name, roleARN, destType, bucket, prefix, lambdaARN)
	}
	if errors.Is(err, store.ErrFirehoseExists) {
		s.writeFirehoseError(w, r, body, requestID, http.StatusBadRequest, "ResourceInUseException",
			"Delivery stream already exists.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrFirehoseBadReq) {
		s.writeFirehoseError(w, r, body, requestID, http.StatusBadRequest, "InvalidArgumentException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeFirehoseError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create delivery stream.", readOnly, eventID, verified)
		return
	}
	payload, _ := fhsvc.CreateDeliveryStreamJSON(st)
	s.writeFirehoseOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, firehoseEventSource, "CreateDeliveryStream", readOnly)
}

func parseFirehoseDestination(params map[string]any) (destType, bucket, prefix, lambdaARN, roleARN, osDomain, osIndex string) {
	if s3, ok := params["S3DestinationConfiguration"].(map[string]any); ok {
		destType = "S3"
		bucket, _ = s3["BucketARN"].(string)
		if bucket == "" {
			bucket, _ = s3["Bucket"].(string)
		}
		prefix, _ = s3["Prefix"].(string)
		roleARN, _ = s3["RoleARN"].(string)
		return
	}
	if s3, ok := params["ExtendedS3DestinationConfiguration"].(map[string]any); ok {
		destType = "S3"
		bucket, _ = s3["BucketARN"].(string)
		prefix, _ = s3["Prefix"].(string)
		roleARN, _ = s3["RoleARN"].(string)
		return
	}
	if lam, ok := params["LambdaDestinationConfiguration"].(map[string]any); ok {
		destType = "Lambda"
		lambdaARN, _ = lam["LambdaArn"].(string)
		if lambdaARN == "" {
			lambdaARN, _ = lam["FunctionArn"].(string)
		}
		roleARN, _ = lam["RoleARN"].(string)
		return
	}
	if osCfg, ok := params["AmazonopensearchserviceDestinationConfiguration"].(map[string]any); ok {
		return parseFirehoseOpenSearchDest(osCfg)
	}
	if osCfg, ok := params["AmazonOpenSearchServiceDestinationConfiguration"].(map[string]any); ok {
		return parseFirehoseOpenSearchDest(osCfg)
	}
	if osCfg, ok := params["OpenSearchDestinationConfiguration"].(map[string]any); ok {
		return parseFirehoseOpenSearchDest(osCfg)
	}
	if vpc, ok := params["VpcFlowLogsDestinationConfiguration"].(map[string]any); ok {
		destType = "VPCFlow"
		bucket, _ = vpc["BucketARN"].(string)
		if bucket == "" {
			bucket, _ = vpc["Bucket"].(string)
		}
		prefix, _ = vpc["Prefix"].(string)
		roleARN, _ = vpc["RoleARN"].(string)
		return
	}
	if vpc, ok := params["NoctaxrisVpcFlowDestinationConfiguration"].(map[string]any); ok {
		destType = "VPCFlow"
		bucket, _ = vpc["BucketARN"].(string)
		if bucket == "" {
			bucket, _ = vpc["Bucket"].(string)
		}
		prefix, _ = vpc["Prefix"].(string)
		roleARN, _ = vpc["RoleARN"].(string)
		return
	}
	destType = "S3"
	return
}

func parseFirehoseOpenSearchDest(cfg map[string]any) (destType, bucket, prefix, lambdaARN, roleARN, osDomain, osIndex string) {
	destType = "OpenSearch"
	domainARN, _ := cfg["DomainARN"].(string)
	domainName, _ := cfg["DomainName"].(string)
	osDomain = store.ParseFirehoseOpenSearchDomainRef(domainARN, domainName)
	osIndex, _ = cfg["IndexName"].(string)
	roleARN, _ = cfg["RoleARN"].(string)
	return
}

func (s *Server) fhDelete(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name, _ := params["DeliveryStreamName"].(string)
	if !s.authorize(verified, catalog.ActionFirehoseDeleteDeliveryStream, "*") {
		s.writeFirehoseError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform firehose:DeleteDeliveryStream.", readOnly, eventID, verified)
		return
	}
	err := s.store.DeleteFirehoseStream(verified.AccountID, name)
	if errors.Is(err, store.ErrFirehoseNotFound) {
		s.writeFirehoseError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Delivery stream not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeFirehoseError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete delivery stream.", readOnly, eventID, verified)
		return
	}
	payload, _ := fhsvc.DeleteDeliveryStreamJSON()
	s.writeFirehoseOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, firehoseEventSource, "DeleteDeliveryStream", readOnly)
}

func (s *Server) fhDescribe(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name, _ := params["DeliveryStreamName"].(string)
	if !s.authorize(verified, catalog.ActionFirehoseDescribeDeliveryStream, "*") {
		s.writeFirehoseError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform firehose:DescribeDeliveryStream.", readOnly, eventID, verified)
		return
	}
	st, err := s.store.GetFirehoseStream(verified.AccountID, name)
	if errors.Is(err, store.ErrFirehoseNotFound) {
		s.writeFirehoseError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Delivery stream not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeFirehoseError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe delivery stream.", readOnly, eventID, verified)
		return
	}
	payload, _ := fhsvc.DescribeDeliveryStreamJSON(st)
	s.writeFirehoseOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, firehoseEventSource, "DescribeDeliveryStream", readOnly)
}

func (s *Server) fhList(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool,
) {
	if !s.authorize(verified, catalog.ActionFirehoseListDeliveryStreams, "*") {
		s.writeFirehoseError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform firehose:ListDeliveryStreams.", readOnly, eventID, verified)
		return
	}
	streams, err := s.store.ListFirehoseStreams(verified.AccountID)
	if err != nil {
		s.writeFirehoseError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list delivery streams.", readOnly, eventID, verified)
		return
	}
	payload, _ := fhsvc.ListDeliveryStreamsJSON(streams)
	s.writeFirehoseOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, firehoseEventSource, "ListDeliveryStreams", readOnly)
}

func (s *Server) fhPutRecord(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name, _ := params["DeliveryStreamName"].(string)
	if !s.authorize(verified, catalog.ActionFirehosePutRecord, "*") {
		s.writeFirehoseError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform firehose:PutRecord.", readOnly, eventID, verified)
		return
	}
	rec, _ := params["Record"].(map[string]any)
	data, err := store.DecodeFirehoseData(rec["Data"])
	if err != nil {
		s.writeFirehoseError(w, r, body, requestID, http.StatusBadRequest, "InvalidArgumentException",
			"Record.Data must be base64.", readOnly, eventID, verified)
		return
	}
	id, err := s.store.PutFirehoseRecord(verified.AccountID, name, data)
	if errors.Is(err, store.ErrFirehoseNotFound) {
		s.writeFirehoseError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Delivery stream not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeFirehoseError(w, r, body, requestID, http.StatusInternalServerError, "ServiceUnavailableException",
			"Unable to put record.", readOnly, eventID, verified)
		return
	}
	payload, _ := fhsvc.PutRecordJSON(id)
	s.writeFirehoseOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, firehoseEventSource, "PutRecord", readOnly)
}

func (s *Server) fhPutRecordBatch(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name, _ := params["DeliveryStreamName"].(string)
	if !s.authorize(verified, catalog.ActionFirehosePutRecordBatch, "*") {
		s.writeFirehoseError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform firehose:PutRecordBatch.", readOnly, eventID, verified)
		return
	}
	rawRecords, _ := params["Records"].([]any)
	records := make([][]byte, 0, len(rawRecords))
	for _, rr := range rawRecords {
		m, _ := rr.(map[string]any)
		data, err := store.DecodeFirehoseData(m["Data"])
		if err != nil {
			s.writeFirehoseError(w, r, body, requestID, http.StatusBadRequest, "InvalidArgumentException",
				"Records[].Data must be base64.", readOnly, eventID, verified)
			return
		}
		records = append(records, data)
	}
	failed, err := s.store.PutFirehoseRecordBatch(verified.AccountID, name, records)
	if errors.Is(err, store.ErrFirehoseNotFound) {
		s.writeFirehoseError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Delivery stream not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeFirehoseError(w, r, body, requestID, http.StatusInternalServerError, "ServiceUnavailableException",
			"Unable to put record batch.", readOnly, eventID, verified)
		return
	}
	payload, _ := fhsvc.PutRecordBatchJSON(failed, len(records))
	s.writeFirehoseOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, firehoseEventSource, "PutRecordBatch", readOnly)
}

func (s *Server) writeFirehoseOK(w http.ResponseWriter, payload []byte) {
	w.Header().Set("Content-Type", firehoseJSONContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeFirehoseError(
	w http.ResponseWriter, r *http.Request, body []byte, requestID string, status int, code, message string,
	readOnly bool, eventID string, verified *authn.Verified,
) {
	w.Header().Set("Content-Type", firehoseJSONContentType)
	w.Header().Set("x-amzn-ErrorType", code)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"__type": code, "message": message})
	_ = body
	s.writeSuccessAudit(r, requestID, eventID, verified, firehoseEventSource, code, readOnly)
}
