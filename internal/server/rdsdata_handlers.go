package server

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	rdsdatasvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/rdsdata"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	rdsDataJSONContentType = "application/x-amz-json-1.0"
	rdsDataEventSource     = "rds-data.amazonaws.com"
)

var (
	rdsDataExecutorMu       sync.Mutex
	rdsDataExecutorOverride store.RDSDataExecutor // nil = production path; override is test injection only
)

// SetRDSDataExecutor replaces the Data API executor (tests).
// Nil clears the override so ExecuteStatement requires nested Postgres
// (fail closed with DatabaseUnavailableException when no engine).
func (s *Server) SetRDSDataExecutor(exec store.RDSDataExecutor) {
	rdsDataExecutorMu.Lock()
	defer rdsDataExecutorMu.Unlock()
	rdsDataExecutorOverride = exec
}

func getRDSDataExecutorOverride() store.RDSDataExecutor {
	rdsDataExecutorMu.Lock()
	defer rdsDataExecutorMu.Unlock()
	return rdsDataExecutorOverride
}

func (s *Server) handleRDSData(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = rdsDataAction(action)

	switch action {
	case catalog.ActionRDSDataExecuteStatement:
		s.rdsDataExecute(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionRDSDataBatchExecuteStatement:
		s.rdsDataBatchExecute(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionRDSDataBeginTransaction:
		s.rdsDataBegin(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionRDSDataCommitTransaction:
		s.rdsDataCommit(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionRDSDataRollbackTransaction:
		s.rdsDataRollback(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeRDSDataError(w, r, body, requestID, http.StatusNotImplemented, "InternalFailure",
			"This RDS Data API action is not implemented.", readOnly, eventID, verified)
	}
}

func rdsDataAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "ExecuteStatement":
		return catalog.ActionRDSDataExecuteStatement
	case "BatchExecuteStatement":
		return catalog.ActionRDSDataBatchExecuteStatement
	case "BeginTransaction":
		return catalog.ActionRDSDataBeginTransaction
	case "CommitTransaction":
		return catalog.ActionRDSDataCommitTransaction
	case "RollbackTransaction":
		return catalog.ActionRDSDataRollbackTransaction
	default:
		return action
	}
}

func (s *Server) rdsDataExecute(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionRDSDataExecuteStatement, "*") {
		s.writeRDSDataError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform rds-data:ExecuteStatement.", readOnly, eventID, verified)
		return
	}
	req := store.RDSDataExecuteRequest{
		ResourceARN:     stringParam(params["resourceArn"]),
		SecretARN:       stringParam(params["secretArn"]),
		Database:        stringParam(params["database"]),
		SQL:             stringParam(params["sql"]),
		TransactionID:   stringParam(params["transactionId"]),
		Parameters:      parseRDSDataParameters(params["parameters"]),
		FormatRecordsAs: stringParam(params["formatRecordsAs"]),
	}
	if req.SQL == "" {
		s.writeRDSDataError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			"sql is required.", readOnly, eventID, verified)
		return
	}
	if format := strings.TrimSpace(req.FormatRecordsAs); format != "" &&
		!strings.EqualFold(format, "NONE") && !strings.EqualFold(format, "JSON") {
		s.writeRDSDataError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			"formatRecordsAs must be NONE or JSON.", readOnly, eventID, verified)
		return
	}
	var (
		res store.RDSDataExecuteResult
		err error
	)
	if strings.TrimSpace(req.TransactionID) != "" {
		res, err = s.executeRDSDataOnTransaction(r.Context(), verified.AccountID, req)
	} else {
		inst, resolveErr := s.store.ResolveRDSDataResource(verified.AccountID, req.ResourceARN, req.SecretARN)
		if resolveErr != nil {
			s.writeRDSDataResolveError(w, r, body, requestID, resolveErr, readOnly, eventID, verified)
			return
		}
		res, err = s.preferNestedRDSDataExecute(r.Context(), verified.AccountID, inst, req)
	}
	if err != nil {
		if errors.Is(err, store.ErrRDSDataUnavailable) ||
			errors.Is(err, store.ErrRDSDataTxnNotFound) ||
			errors.Is(err, store.ErrRDSDataBadRequest) ||
			errors.Is(err, store.ErrRDSDataInvalidSecret) ||
			errors.Is(err, store.ErrRDSDataSecretsError) ||
			errors.Is(err, store.ErrRDSDataNotFound) {
			s.writeRDSDataResolveError(w, r, body, requestID, err, readOnly, eventID, verified)
			return
		}
		s.writeRDSDataError(w, r, body, requestID, http.StatusBadRequest, "DatabaseErrorException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	_ = s.store.RecordRDSDataStatement(verified.AccountID, req)
	if rdsdatasvc.WantsFormatRecordsAsJSON(req.FormatRecordsAs) {
		res, err = rdsdatasvc.ApplyFormatRecordsAsJSON(res)
		if err != nil {
			s.writeRDSDataError(w, r, body, requestID, http.StatusInternalServerError, "InternalServerErrorException",
				"Unable to format records as JSON.", readOnly, eventID, verified)
			return
		}
	}
	payload, err := rdsdatasvc.ExecuteStatementJSON(res)
	if err != nil {
		s.writeRDSDataError(w, r, body, requestID, http.StatusInternalServerError, "InternalServerErrorException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeRDSDataOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, rdsDataEventSource, "ExecuteStatement", readOnly)
}

func (s *Server) rdsDataBatchExecute(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionRDSDataBatchExecuteStatement, "*") {
		s.writeRDSDataError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform rds-data:BatchExecuteStatement.", readOnly, eventID, verified)
		return
	}
	req := store.RDSDataBatchExecuteRequest{
		ResourceARN:   stringParam(params["resourceArn"]),
		SecretARN:     stringParam(params["secretArn"]),
		Database:      stringParam(params["database"]),
		SQL:           stringParam(params["sql"]),
		TransactionID: stringParam(params["transactionId"]),
		ParameterSets: parseRDSDataParameterSets(params["parameterSets"]),
	}
	if req.SQL == "" {
		s.writeRDSDataError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			"sql is required.", readOnly, eventID, verified)
		return
	}
	if len(req.ParameterSets) == 0 {
		s.writeRDSDataError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			"parameterSets is required.", readOnly, eventID, verified)
		return
	}
	var (
		results []store.RDSDataExecuteResult
		err     error
	)
	if strings.TrimSpace(req.TransactionID) != "" {
		results, err = s.preferNestedRDSDataBatch(r.Context(), verified.AccountID, store.RDSDBInstance{}, req)
	} else {
		inst, resolveErr := s.store.ResolveRDSDataResource(verified.AccountID, req.ResourceARN, req.SecretARN)
		if resolveErr != nil {
			s.writeRDSDataResolveError(w, r, body, requestID, resolveErr, readOnly, eventID, verified)
			return
		}
		results, err = s.preferNestedRDSDataBatch(r.Context(), verified.AccountID, inst, req)
	}
	if err != nil {
		if errors.Is(err, store.ErrRDSDataUnavailable) ||
			errors.Is(err, store.ErrRDSDataTxnNotFound) ||
			errors.Is(err, store.ErrRDSDataBadRequest) ||
			errors.Is(err, store.ErrRDSDataInvalidSecret) ||
			errors.Is(err, store.ErrRDSDataSecretsError) ||
			errors.Is(err, store.ErrRDSDataNotFound) {
			s.writeRDSDataResolveError(w, r, body, requestID, err, readOnly, eventID, verified)
			return
		}
		s.writeRDSDataError(w, r, body, requestID, http.StatusBadRequest, "DatabaseErrorException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	payload, err := rdsdatasvc.BatchExecuteStatementJSON(results)
	if err != nil {
		s.writeRDSDataError(w, r, body, requestID, http.StatusInternalServerError, "InternalServerErrorException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeRDSDataOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, rdsDataEventSource, "BatchExecuteStatement", readOnly)
}

func (s *Server) rdsDataBegin(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionRDSDataBeginTransaction, "*") {
		s.writeRDSDataError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform rds-data:BeginTransaction.", readOnly, eventID, verified)
		return
	}
	resourceARN := stringParam(params["resourceArn"])
	secretARN := stringParam(params["secretArn"])
	database := stringParam(params["database"])
	inst, err := s.store.ResolveRDSDataResource(verified.AccountID, resourceARN, secretARN)
	if err != nil {
		s.writeRDSDataResolveError(w, r, body, requestID, err, readOnly, eventID, verified)
		return
	}
	txnID, err := s.BeginRDSDataSQLTransaction(r.Context(), verified.AccountID, inst, secretARN, database)
	if err != nil {
		if errors.Is(err, store.ErrRDSDataUnavailable) ||
			errors.Is(err, store.ErrRDSDataBadRequest) ||
			errors.Is(err, store.ErrRDSDataInvalidSecret) ||
			errors.Is(err, store.ErrRDSDataSecretsError) {
			s.writeRDSDataResolveError(w, r, body, requestID, err, readOnly, eventID, verified)
			return
		}
		s.writeRDSDataError(w, r, body, requestID, http.StatusBadRequest, "DatabaseErrorException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	payload, err := rdsdatasvc.BeginTransactionJSON(txnID)
	if err != nil {
		s.writeRDSDataError(w, r, body, requestID, http.StatusInternalServerError, "InternalServerErrorException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeRDSDataOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, rdsDataEventSource, "BeginTransaction", readOnly)
}

func (s *Server) rdsDataCommit(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionRDSDataCommitTransaction, "*") {
		s.writeRDSDataError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform rds-data:CommitTransaction.", readOnly, eventID, verified)
		return
	}
	resourceARN := stringParam(params["resourceArn"])
	secretARN := stringParam(params["secretArn"])
	txnID := stringParam(params["transactionId"])
	if resourceARN == "" || secretARN == "" || txnID == "" {
		s.writeRDSDataError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			"resourceArn, secretArn, and transactionId are required.", readOnly, eventID, verified)
		return
	}
	if _, err := s.store.ResolveRDSDataResource(verified.AccountID, resourceARN, secretARN); err != nil {
		s.writeRDSDataResolveError(w, r, body, requestID, err, readOnly, eventID, verified)
		return
	}
	if err := s.CommitRDSDataSQLTransaction(r.Context(), verified.AccountID, txnID, resourceARN, secretARN); err != nil {
		if errors.Is(err, store.ErrRDSDataTxnNotFound) || errors.Is(err, store.ErrRDSDataBadRequest) {
			s.writeRDSDataResolveError(w, r, body, requestID, err, readOnly, eventID, verified)
			return
		}
		s.writeRDSDataError(w, r, body, requestID, http.StatusBadRequest, "DatabaseErrorException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	payload, err := rdsdatasvc.CommitTransactionJSON()
	if err != nil {
		s.writeRDSDataError(w, r, body, requestID, http.StatusInternalServerError, "InternalServerErrorException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeRDSDataOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, rdsDataEventSource, "CommitTransaction", readOnly)
}

func (s *Server) rdsDataRollback(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionRDSDataRollbackTransaction, "*") {
		s.writeRDSDataError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform rds-data:RollbackTransaction.", readOnly, eventID, verified)
		return
	}
	resourceARN := stringParam(params["resourceArn"])
	secretARN := stringParam(params["secretArn"])
	txnID := stringParam(params["transactionId"])
	if resourceARN == "" || secretARN == "" || txnID == "" {
		s.writeRDSDataError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			"resourceArn, secretArn, and transactionId are required.", readOnly, eventID, verified)
		return
	}
	if _, err := s.store.ResolveRDSDataResource(verified.AccountID, resourceARN, secretARN); err != nil {
		s.writeRDSDataResolveError(w, r, body, requestID, err, readOnly, eventID, verified)
		return
	}
	if err := s.RollbackRDSDataSQLTransaction(r.Context(), verified.AccountID, txnID, resourceARN, secretARN); err != nil {
		if errors.Is(err, store.ErrRDSDataTxnNotFound) || errors.Is(err, store.ErrRDSDataBadRequest) {
			s.writeRDSDataResolveError(w, r, body, requestID, err, readOnly, eventID, verified)
			return
		}
		s.writeRDSDataError(w, r, body, requestID, http.StatusBadRequest, "DatabaseErrorException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	payload, err := rdsdatasvc.RollbackTransactionJSON()
	if err != nil {
		s.writeRDSDataError(w, r, body, requestID, http.StatusInternalServerError, "InternalServerErrorException",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeRDSDataOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, rdsDataEventSource, "RollbackTransaction", readOnly)
}

func (s *Server) writeRDSDataResolveError(
	w http.ResponseWriter, r *http.Request, body []byte, requestID string, err error,
	readOnly bool, eventID string, verified *authn.Verified,
) {
	switch {
	case errors.Is(err, store.ErrRDSDataNotFound):
		s.writeRDSDataError(w, r, body, requestID, http.StatusNotFound, "DatabaseNotFoundException",
			"DB instance not found for resourceArn.", readOnly, eventID, verified)
	case errors.Is(err, store.ErrRDSDataInvalidSecret):
		s.writeRDSDataError(w, r, body, requestID, http.StatusBadRequest, "InvalidSecretException",
			"The Secrets Manager secret used with the request is not valid.", readOnly, eventID, verified)
	case errors.Is(err, store.ErrRDSDataSecretsError):
		s.writeRDSDataError(w, r, body, requestID, http.StatusBadRequest, "SecretsErrorException",
			"There was a problem with the Secrets Manager secret.", readOnly, eventID, verified)
	case errors.Is(err, store.ErrRDSDataTxnNotFound):
		s.writeRDSDataError(w, r, body, requestID, http.StatusNotFound, "TransactionNotFoundException",
			"Transaction not found.", readOnly, eventID, verified)
	case errors.Is(err, store.ErrRDSDataBadRequest):
		s.writeRDSDataError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			err.Error(), readOnly, eventID, verified)
	case errors.Is(err, store.ErrRDSDataUnavailable):
		s.writeRDSDataError(w, r, body, requestID, http.StatusGatewayTimeout, "DatabaseUnavailableException",
			"DB instance is not available.", readOnly, eventID, verified)
	default:
		s.writeRDSDataError(w, r, body, requestID, http.StatusBadRequest, "BadRequestException",
			err.Error(), readOnly, eventID, verified)
	}
}

func parseRDSDataParameterSets(raw any) [][]store.RDSDataSqlParameter {
	list, ok := raw.([]any)
	if !ok || len(list) == 0 {
		return nil
	}
	out := make([][]store.RDSDataSqlParameter, 0, len(list))
	for _, item := range list {
		out = append(out, parseRDSDataParameters(item))
	}
	return out
}

func parseRDSDataParameters(raw any) []store.RDSDataSqlParameter {
	list, ok := raw.([]any)
	if !ok || len(list) == 0 {
		return nil
	}
	out := make([]store.RDSDataSqlParameter, 0, len(list))
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		p := store.RDSDataSqlParameter{Name: stringParam(m["name"])}
		val, _ := m["value"].(map[string]any)
		if val == nil {
			out = append(out, p)
			continue
		}
		if b, ok := val["isNull"].(bool); ok && b {
			t := true
			p.IsNull = &t
		}
		if s, ok := val["stringValue"].(string); ok {
			p.StringValue = &s
		}
		if n, ok := asInt64(val["longValue"]); ok {
			p.LongValue = &n
		}
		if f, ok := asFloat64(val["doubleValue"]); ok {
			p.DoubleValue = &f
		}
		if b, ok := val["booleanValue"].(bool); ok {
			p.BooleanValue = &b
		}
		if s, ok := val["blobValue"].(string); ok && s != "" {
			if raw, err := decodeRDSDataBlob(s); err == nil {
				p.BlobValue = raw
			}
		}
		out = append(out, p)
	}
	return out
}

func decodeRDSDataBlob(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(strings.TrimSpace(s))
}

func asInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case int64:
		return n, true
	case int:
		return int64(n), true
	case json.Number:
		i, err := n.Int64()
		return i, err == nil
	default:
		return 0, false
	}
}

func asFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int64:
		return float64(n), true
	case int:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

func (s *Server) writeRDSDataOK(w http.ResponseWriter, requestID string, payload []byte) {
	w.Header().Set("Content-Type", rdsDataJSONContentType)
	w.Header().Set("x-amzn-RequestId", requestID)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeRDSDataError(
	w http.ResponseWriter, r *http.Request, body []byte, requestID string,
	status int, code, message string, readOnly bool, eventID string, verified *authn.Verified,
) {
	w.Header().Set("Content-Type", rdsDataJSONContentType)
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
