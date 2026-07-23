package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	ddb "github.com/Kyaxris-Labs/Noctaxris/internal/services/dynamodb"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	dynamoJSONContentType = "application/x-amz-json-1.0"
	dynamoBatchLimit      = 25
	dynamoEventSource     = "dynamodb.amazonaws.com"
)

func (s *Server) handleDynamoDB(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	accountID := verified.AccountID
	action = dynamoAction(action)

	switch action {
	case catalog.ActionDynamoDBListTables:
		s.dynamoListTables(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionDynamoDBCreateTable:
		s.dynamoCreateTable(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionDynamoDBDescribeTable:
		s.dynamoDescribeTable(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionDynamoDBDeleteTable:
		s.dynamoDeleteTable(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionDynamoDBUpdateTable:
		s.dynamoUpdateTable(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionDynamoDBPutItem:
		s.dynamoPutItem(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionDynamoDBGetItem:
		s.dynamoGetItem(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionDynamoDBDeleteItem:
		s.dynamoDeleteItem(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionDynamoDBUpdateItem:
		s.dynamoUpdateItem(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionDynamoDBQuery:
		s.dynamoQuery(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionDynamoDBScan:
		s.dynamoScan(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionDynamoDBBatchGetItem:
		s.dynamoBatchGetItem(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionDynamoDBBatchWriteItem:
		s.dynamoBatchWriteItem(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionDynamoDBTransactGetItems:
		s.dynamoTransactGetItems(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionDynamoDBTransactWriteItems:
		s.dynamoTransactWriteItems(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionDynamoDBPutResourcePolicy:
		s.dynamoPutResourcePolicy(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionDynamoDBGetResourcePolicy:
		s.dynamoGetResourcePolicy(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionDynamoDBDeleteResourcePolicy:
		s.dynamoDeleteResourcePolicy(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionDynamoDBUpdateTimeToLive:
		s.dynamoUpdateTimeToLive(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionDynamoDBDescribeTimeToLive:
		s.dynamoDescribeTimeToLive(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionDynamoDBDescribeContinuousBackups:
		s.dynamoDescribeContinuousBackups(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionDynamoDBListTagsOfResource:
		s.dynamoListTagsOfResource(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionDynamoDBTagResource:
		s.dynamoTagResource(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionDynamoDBUntagResource:
		s.dynamoUntagResource(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		_ = accountID
		s.writeDynamoError(w, r, body, requestID, http.StatusNotImplemented, "InternalFailure",
			"This DynamoDB action is not implemented.", readOnly, eventID, verified)
	}
}

func dynamoAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "CreateTable":
		return catalog.ActionDynamoDBCreateTable
	case "DescribeTable":
		return catalog.ActionDynamoDBDescribeTable
	case "DeleteTable":
		return catalog.ActionDynamoDBDeleteTable
	case "ListTables":
		return catalog.ActionDynamoDBListTables
	case "UpdateTable":
		return catalog.ActionDynamoDBUpdateTable
	case "PutItem":
		return catalog.ActionDynamoDBPutItem
	case "GetItem":
		return catalog.ActionDynamoDBGetItem
	case "DeleteItem":
		return catalog.ActionDynamoDBDeleteItem
	case "UpdateItem":
		return catalog.ActionDynamoDBUpdateItem
	case "Query":
		return catalog.ActionDynamoDBQuery
	case "Scan":
		return catalog.ActionDynamoDBScan
	case "BatchGetItem":
		return catalog.ActionDynamoDBBatchGetItem
	case "BatchWriteItem":
		return catalog.ActionDynamoDBBatchWriteItem
	case "TransactGetItems":
		return catalog.ActionDynamoDBTransactGetItems
	case "TransactWriteItems":
		return catalog.ActionDynamoDBTransactWriteItems
	case "PutResourcePolicy":
		return catalog.ActionDynamoDBPutResourcePolicy
	case "GetResourcePolicy":
		return catalog.ActionDynamoDBGetResourcePolicy
	case "DeleteResourcePolicy":
		return catalog.ActionDynamoDBDeleteResourcePolicy
	case "UpdateTimeToLive":
		return catalog.ActionDynamoDBUpdateTimeToLive
	case "DescribeTimeToLive":
		return catalog.ActionDynamoDBDescribeTimeToLive
	case "DescribeContinuousBackups":
		return catalog.ActionDynamoDBDescribeContinuousBackups
	case "ListTagsOfResource":
		return catalog.ActionDynamoDBListTagsOfResource
	case "TagResource":
		return catalog.ActionDynamoDBTagResource
	case "UntagResource":
		return catalog.ActionDynamoDBUntagResource
	default:
		return action
	}
}

func (s *Server) authorizeDynamoDB(verified *authn.Verified, action, resource, resourcePolicy string) bool {
	resourceAccountID := resourceAccountIDFromARN(resource)
	if resourceAccountID == "" {
		resourceAccountID = verified.AccountID
	}
	return s.authorizeDataplaneOR(verified, action, resource, resourceAccountID, func(caller authz.RequestContext, identityDocs []string, resourceAccountID string) authz.Decision {
		return authz.EvaluateDynamoDB(authz.DynamoDBRequest{
			Caller:            caller,
			IdentityDocs:      identityDocs,
			ResourcePolicyDoc: resourcePolicy,
			ResourceAccountID: resourceAccountID,
		})
	})
}

func (s *Server) dynamoTableOrErr(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	tableName string,
) (store.DynamoTable, bool) {
	if strings.TrimSpace(tableName) == "" {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"TableName is required.", readOnly, eventID, verified)
		return store.DynamoTable{}, false
	}
	accountID := verified.AccountID
	name := tableName
	if acct, parsed, ok := store.ParseTableARN(tableName); ok {
		accountID = acct
		name = parsed
	}
	table, err := s.store.GetTable(accountID, name)
	if errors.Is(err, store.ErrNoSuchTable) {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Requested resource not found.", readOnly, eventID, verified)
		return store.DynamoTable{}, false
	}
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to load table.", readOnly, eventID, verified)
		return store.DynamoTable{}, false
	}
	return table, true
}

func (s *Server) dynamoListTables(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
) {
	if !s.authorizeDynamoDB(verified, catalog.ActionDynamoDBListTables, "*", "") {
		s.writeDynamoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform dynamodb:ListTables.", readOnly, eventID, verified)
		return
	}
	tables, err := s.store.ListTables(verified.AccountID)
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list tables.", readOnly, eventID, verified)
		return
	}
	payload, err := ddb.ListTablesJSON(tables)
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeDynamoOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, dynamoEventSource, "ListTables", readOnly)
}

func (s *Server) dynamoCreateTable(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	tableName, _ := params["TableName"].(string)
	if strings.TrimSpace(tableName) == "" {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"TableName is required.", readOnly, eventID, verified)
		return
	}
	region := verified.Region
	if region == "" {
		region = store.DefaultDynamoRegion
	}
	arn := store.TableARN(verified.AccountID, region, tableName)
	if !s.authorizeDynamoDB(verified, catalog.ActionDynamoDBCreateTable, arn, "") {
		s.writeDynamoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform dynamodb:CreateTable.", readOnly, eventID, verified)
		return
	}

	attrTypes := map[string]string{}
	if defs, ok := params["AttributeDefinitions"].([]any); ok {
		for _, raw := range defs {
			m, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			name, _ := m["AttributeName"].(string)
			typ, _ := m["AttributeType"].(string)
			if name != "" && typ != "" {
				attrTypes[name] = typ
			}
		}
	}
	var hashKey, hashType, rangeKey, rangeType string
	schema, _ := params["KeySchema"].([]any)
	for _, raw := range schema {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name, _ := m["AttributeName"].(string)
		keyType, _ := m["KeyType"].(string)
		switch strings.ToUpper(keyType) {
		case "HASH":
			hashKey = name
			hashType = attrTypes[name]
		case "RANGE":
			rangeKey = name
			rangeType = attrTypes[name]
		}
	}
	if hashKey == "" || hashType == "" {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"KeySchema must include a HASH key with AttributeDefinitions.", readOnly, eventID, verified)
		return
	}
	if rangeKey != "" && rangeType == "" {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"RANGE key AttributeType is required.", readOnly, eventID, verified)
		return
	}

	sseType := store.SSETypeAWSOwned
	kmsKeyID := ""
	if sseSpec, ok := params["SSESpecification"].(map[string]any); ok {
		enabled, _ := sseSpec["Enabled"].(bool)
		if enabled {
			sseTypeParam, _ := sseSpec["SSEType"].(string)
			switch strings.ToUpper(sseTypeParam) {
			case "", "KMS":
				sseType = store.SSETypeKMS
			case "AES256":
				sseType = store.SSETypeAWSOwned
			default:
				s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
					"Unsupported SSEType.", readOnly, eventID, verified)
				return
			}
			kmsKeyID, _ = sseSpec["KMSMasterKeyId"].(string)
			if sseType == store.SSETypeKMS {
				if kmsKeyID == "" {
					s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
						"KMSMasterKeyId is required for SSEType KMS.", readOnly, eventID, verified)
					return
				}
				resolved, err := s.store.ResolveKeyID(verified.AccountID, kmsKeyID)
				if err != nil {
					s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
						"KMSMasterKeyId not found.", readOnly, eventID, verified)
					return
				}
				key, err := s.store.GetKey(resolved)
				if err != nil || key.AccountID != verified.AccountID {
					s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
						"KMSMasterKeyId not found.", readOnly, eventID, verified)
					return
				}
				encCtx := map[string]string{
					"aws:dynamodb:tableName":    tableName,
					"aws:dynamodb:subscriberId": verified.AccountID,
				}
				if !s.authorizeKMSOp(verified, catalog.ActionKMSDescribeKey, key, encCtx) ||
					!s.authorizeKMSOp(verified, catalog.ActionKMSCreateGrant, key, encCtx) {
					s.writeDynamoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
						"User is not authorized to perform kms:DescribeKey or kms:CreateGrant on the table SSE-KMS key.", readOnly, eventID, verified)
					return
				}
				kmsKeyID = key.ARN
			}
		}
	}

	var gsis []store.DynamoGSI
	if indexes, ok := params["GlobalSecondaryIndexes"].([]any); ok && len(indexes) > 0 {
		if len(indexes) > 2 {
			s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
				"Lab supports at most two GSIs per table.", readOnly, eventID, verified)
			return
		}
		for _, rawIdx := range indexes {
			raw, ok := rawIdx.(map[string]any)
			if !ok {
				s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
					"Invalid GlobalSecondaryIndexes entry.", readOnly, eventID, verified)
				return
			}
			parsed, err := dynamoParseGSISpec(raw, attrTypes)
			if err != nil {
				s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
					err.Error(), readOnly, eventID, verified)
				return
			}
			gsis = append(gsis, parsed)
		}
	}

	table, err := s.store.CreateTableWithGSIs(
		verified.AccountID, region, tableName,
		hashKey, hashType, rangeKey, rangeType, sseType, kmsKeyID,
		gsis,
	)
	if errors.Is(err, store.ErrTableAlreadyExists) {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ResourceInUseException",
			"Table already exists.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrInvalidKeyType) {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"Invalid key AttributeType.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create table.", readOnly, eventID, verified)
		return
	}
	if streamSpec, ok := params["StreamSpecification"].(map[string]any); ok {
		enabled, _ := streamSpec["StreamEnabled"].(bool)
		viewType, _ := streamSpec["StreamViewType"].(string)
		if enabled {
			table, err = s.store.UpdateTableStreamSpec(verified.AccountID, tableName, true, viewType)
			if errors.Is(err, store.ErrDynamoStreamBadView) {
				s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
					"StreamViewType must be NEW_IMAGE or KEYS_ONLY.", readOnly, eventID, verified)
				return
			}
			if err != nil {
				s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
					"Unable to enable stream.", readOnly, eventID, verified)
				return
			}
		}
	}
	payload, err := ddb.CreateTableJSON(table)
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeDynamoOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, dynamoEventSource, "CreateTable", readOnly)
}

func (s *Server) dynamoDescribeTable(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	tableName, _ := params["TableName"].(string)
	table, ok := s.dynamoTableOrErr(w, r, body, requestID, eventID, verified, readOnly, tableName)
	if !ok {
		return
	}
	if !s.authorizeDynamoDB(verified, catalog.ActionDynamoDBDescribeTable, table.TableARN, table.ResourcePolicy) {
		s.writeDynamoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform dynamodb:DescribeTable.", readOnly, eventID, verified)
		return
	}
	payload, err := ddb.DescribeTableJSON(table)
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeDynamoOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, dynamoEventSource, "DescribeTable", readOnly)
}

func (s *Server) dynamoDeleteTable(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	tableName, _ := params["TableName"].(string)
	table, ok := s.dynamoTableOrErr(w, r, body, requestID, eventID, verified, readOnly, tableName)
	if !ok {
		return
	}
	if !s.authorizeDynamoDB(verified, catalog.ActionDynamoDBDeleteTable, table.TableARN, table.ResourcePolicy) {
		s.writeDynamoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform dynamodb:DeleteTable.", readOnly, eventID, verified)
		return
	}
	err := s.store.DeleteTable(table.AccountID, table.TableName)
	if errors.Is(err, store.ErrTableNotEmpty) {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ResourceInUseException",
			"Table is not empty.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete table.", readOnly, eventID, verified)
		return
	}
	payload, err := ddb.CreateTableJSON(table)
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeDynamoOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, dynamoEventSource, "DeleteTable", readOnly)
}

func (s *Server) dynamoUpdateTable(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	tableName, _ := params["TableName"].(string)
	table, ok := s.dynamoTableOrErr(w, r, body, requestID, eventID, verified, readOnly, tableName)
	if !ok {
		return
	}
	if !s.authorizeDynamoDB(verified, catalog.ActionDynamoDBUpdateTable, table.TableARN, table.ResourcePolicy) {
		s.writeDynamoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform dynamodb:UpdateTable.", readOnly, eventID, verified)
		return
	}

	updated := false
	if gsiUpdates, ok := params["GlobalSecondaryIndexUpdates"].([]any); ok && len(gsiUpdates) > 0 {
		for _, raw := range gsiUpdates {
			entry, ok := raw.(map[string]any)
			if !ok {
				s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
					"Invalid GlobalSecondaryIndexUpdates entry.", readOnly, eventID, verified)
				return
			}
			createSpec, ok := entry["Create"].(map[string]any)
			if !ok {
				s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
					"Only Create GlobalSecondaryIndexUpdates are supported.", readOnly, eventID, verified)
				return
			}
			attrTypes := dynamoAttrTypes(params)
			gsi, err := dynamoParseGSISpec(createSpec, attrTypes)
			if err != nil {
				s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
					err.Error(), readOnly, eventID, verified)
				return
			}
			if err := s.store.UpdateTableGSI(verified.AccountID, tableName, gsi); err != nil {
				if errors.Is(err, store.ErrGSIAlreadyExists) {
					s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ResourceInUseException",
						"GSI already exists.", readOnly, eventID, verified)
					return
				}
				if errors.Is(err, store.ErrTooManyGSIs) {
					s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
						"Lab supports at most two GSIs per table.", readOnly, eventID, verified)
					return
				}
				if errors.Is(err, store.ErrInvalidKeyType) {
					s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
						"Invalid GSI key AttributeType.", readOnly, eventID, verified)
					return
				}
				s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
					"Unable to update table.", readOnly, eventID, verified)
				return
			}
			updated = true
		}
	}

	if sseSpec, ok := params["SSESpecification"].(map[string]any); ok {
		enabled, _ := sseSpec["Enabled"].(bool)
		sseType := store.SSETypeAWSOwned
		kmsKeyID := ""
		if enabled {
			sseTypeParam, _ := sseSpec["SSEType"].(string)
			switch strings.ToUpper(sseTypeParam) {
			case "", "KMS":
				sseType = store.SSETypeKMS
			case "AES256":
				sseType = store.SSETypeAWSOwned
			default:
				s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
					"Unsupported SSEType.", readOnly, eventID, verified)
				return
			}
			kmsKeyID, _ = sseSpec["KMSMasterKeyId"].(string)
			if sseType == store.SSETypeKMS {
				if kmsKeyID == "" {
					s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
						"KMSMasterKeyId is required for SSEType KMS.", readOnly, eventID, verified)
					return
				}
				resolved, err := s.store.ResolveKeyID(verified.AccountID, kmsKeyID)
				if err != nil {
					s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
						"KMSMasterKeyId not found.", readOnly, eventID, verified)
					return
				}
				key, err := s.store.GetKey(resolved)
				if err != nil || key.AccountID != verified.AccountID {
					s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
						"KMSMasterKeyId not found.", readOnly, eventID, verified)
					return
				}
				encCtx := map[string]string{
					"aws:dynamodb:tableName":    tableName,
					"aws:dynamodb:subscriberId": verified.AccountID,
				}
				if !s.authorizeKMSOp(verified, catalog.ActionKMSDescribeKey, key, encCtx) ||
					!s.authorizeKMSOp(verified, catalog.ActionKMSCreateGrant, key, encCtx) {
					s.writeDynamoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
						"User is not authorized to perform kms:DescribeKey or kms:CreateGrant on the table SSE-KMS key.", readOnly, eventID, verified)
					return
				}
				kmsKeyID = key.ARN
			}
		}
		if err := s.store.UpdateTableSSE(verified.AccountID, tableName, sseType, kmsKeyID); err != nil {
			s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to update table.", readOnly, eventID, verified)
			return
		}
		updated = true
	}

	if streamSpec, ok := params["StreamSpecification"].(map[string]any); ok {
		enabled, _ := streamSpec["StreamEnabled"].(bool)
		viewType, _ := streamSpec["StreamViewType"].(string)
		var err error
		table, err = s.store.UpdateTableStreamSpec(verified.AccountID, tableName, enabled, viewType)
		if errors.Is(err, store.ErrDynamoStreamBadView) {
			s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
				"StreamViewType must be NEW_IMAGE or KEYS_ONLY.", readOnly, eventID, verified)
			return
		}
		if err != nil {
			s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to update stream specification.", readOnly, eventID, verified)
			return
		}
		updated = true
		_ = table
	}

	if !updated {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"SSESpecification, GlobalSecondaryIndexUpdates, or StreamSpecification is required.", readOnly, eventID, verified)
		return
	}
	refreshed, err := s.store.GetTable(verified.AccountID, tableName)
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to load table.", readOnly, eventID, verified)
		return
	}
	payload, err := ddb.CreateTableJSON(refreshed)
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeDynamoOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, dynamoEventSource, "UpdateTable", readOnly)
}

func (s *Server) dynamoUpdateTimeToLive(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	tableName, _ := params["TableName"].(string)
	table, ok := s.dynamoTableOrErr(w, r, body, requestID, eventID, verified, readOnly, tableName)
	if !ok {
		return
	}
	if !s.authorizeDynamoDB(verified, catalog.ActionDynamoDBUpdateTimeToLive, table.TableARN, table.ResourcePolicy) {
		s.writeDynamoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform dynamodb:UpdateTimeToLive.", readOnly, eventID, verified)
		return
	}
	spec, ok := params["TimeToLiveSpecification"].(map[string]any)
	if !ok {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"TimeToLiveSpecification is required.", readOnly, eventID, verified)
		return
	}
	attrName, _ := spec["AttributeName"].(string)
	enabled, _ := spec["Enabled"].(bool)
	if strings.TrimSpace(attrName) == "" {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"AttributeName is required.", readOnly, eventID, verified)
		return
	}
	if err := s.store.UpdateTimeToLive(verified.AccountID, tableName, attrName, enabled); err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to update TTL.", readOnly, eventID, verified)
		return
	}
	updated, err := s.store.GetTable(verified.AccountID, tableName)
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to load table.", readOnly, eventID, verified)
		return
	}
	payload, err := ddb.UpdateTimeToLiveJSON(updated)
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeDynamoOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, dynamoEventSource, "UpdateTimeToLive", readOnly)
}

func (s *Server) dynamoDescribeTimeToLive(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	tableName, _ := params["TableName"].(string)
	table, ok := s.dynamoTableOrErr(w, r, body, requestID, eventID, verified, readOnly, tableName)
	if !ok {
		return
	}
	if !s.authorizeDynamoDB(verified, catalog.ActionDynamoDBDescribeTimeToLive, table.TableARN, table.ResourcePolicy) {
		s.writeDynamoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform dynamodb:DescribeTimeToLive.", readOnly, eventID, verified)
		return
	}
	payload, err := ddb.DescribeTimeToLiveJSON(table)
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeDynamoOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, dynamoEventSource, "DescribeTimeToLive", readOnly)
}

func (s *Server) dynamoDescribeContinuousBackups(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	tableName, _ := params["TableName"].(string)
	table, ok := s.dynamoTableOrErr(w, r, body, requestID, eventID, verified, readOnly, tableName)
	if !ok {
		return
	}
	if !s.authorizeDynamoDB(verified, catalog.ActionDynamoDBDescribeContinuousBackups, table.TableARN, table.ResourcePolicy) {
		s.writeDynamoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform dynamodb:DescribeContinuousBackups.", readOnly, eventID, verified)
		return
	}
	payload, err := ddb.DescribeContinuousBackupsJSON()
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeDynamoOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, dynamoEventSource, "DescribeContinuousBackups", readOnly)
}

func (s *Server) dynamoTableFromResourceArn(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) (store.DynamoTable, bool) {
	resourceArn, _ := params["ResourceArn"].(string)
	if strings.TrimSpace(resourceArn) == "" {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"ResourceArn is required.", readOnly, eventID, verified)
		return store.DynamoTable{}, false
	}
	tableName := resourceArn
	if strings.Contains(tableName, ":table/") {
		tableName = tableName[strings.LastIndex(tableName, "/")+1:]
	}
	return s.dynamoTableOrErr(w, r, body, requestID, eventID, verified, readOnly, tableName)
}

func (s *Server) dynamoListTagsOfResource(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	table, ok := s.dynamoTableFromResourceArn(w, r, body, requestID, eventID, verified, readOnly, params)
	if !ok {
		return
	}
	if !s.authorizeDynamoDB(verified, catalog.ActionDynamoDBListTagsOfResource, table.TableARN, table.ResourcePolicy) {
		s.writeDynamoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform dynamodb:ListTagsOfResource.", readOnly, eventID, verified)
		return
	}
	tags, err := s.store.ListResourceTags(table.AccountID, table.TableARN)
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list tags.", readOnly, eventID, verified)
		return
	}
	payload, err := ddb.ListTagsOfResourceJSON(tags)
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeDynamoOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, dynamoEventSource, "ListTagsOfResource", readOnly)
}

func (s *Server) dynamoTagResource(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	table, ok := s.dynamoTableFromResourceArn(w, r, body, requestID, eventID, verified, readOnly, params)
	if !ok {
		return
	}
	tags := parseKeyValueTags(params["Tags"])
	if len(tags) == 0 {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"Tags is required.", readOnly, eventID, verified)
		return
	}
	if !s.authorizeDynamoDB(verified, catalog.ActionDynamoDBTagResource, table.TableARN, table.ResourcePolicy) {
		s.writeDynamoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform dynamodb:TagResource.", readOnly, eventID, verified)
		return
	}
	if _, err := s.store.TagResources(table.AccountID, []string{table.TableARN}, tags); err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to tag resource.", readOnly, eventID, verified)
		return
	}
	payload, err := ddb.EmptyOKJSON()
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeDynamoOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, dynamoEventSource, "TagResource", readOnly)
}

func (s *Server) dynamoUntagResource(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	table, ok := s.dynamoTableFromResourceArn(w, r, body, requestID, eventID, verified, readOnly, params)
	if !ok {
		return
	}
	keys := stringSliceParam(params["TagKeys"])
	if len(keys) == 0 {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"TagKeys is required.", readOnly, eventID, verified)
		return
	}
	if !s.authorizeDynamoDB(verified, catalog.ActionDynamoDBUntagResource, table.TableARN, table.ResourcePolicy) {
		s.writeDynamoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform dynamodb:UntagResource.", readOnly, eventID, verified)
		return
	}
	if _, err := s.store.UntagResources(table.AccountID, []string{table.TableARN}, keys); err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to untag resource.", readOnly, eventID, verified)
		return
	}
	payload, err := ddb.EmptyOKJSON()
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeDynamoOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, dynamoEventSource, "UntagResource", readOnly)
}

func (s *Server) dynamoPutItem(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	tableName, _ := params["TableName"].(string)
	table, ok := s.dynamoTableOrErr(w, r, body, requestID, eventID, verified, readOnly, tableName)
	if !ok {
		return
	}
	if !s.authorizeDynamoDB(verified, catalog.ActionDynamoDBPutItem, table.TableARN, table.ResourcePolicy) {
		s.writeDynamoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform dynamodb:PutItem.", readOnly, eventID, verified)
		return
	}
	item, err := ddb.ParseItemMap(params["Item"])
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if cond, _ := params["ConditionExpression"].(string); strings.TrimSpace(cond) != "" {
		itemPK, itemSK, keyErr := ddb.PrimaryKeyStrings(table, item)
		if keyErr != nil {
			s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
				keyErr.Error(), readOnly, eventID, verified)
			return
		}
		key, keyErr := ddb.KeyFromCanonical(table, itemPK, itemSK)
		if keyErr != nil {
			s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
				keyErr.Error(), readOnly, eventID, verified)
			return
		}
		if !s.dynamoCheckCondition(w, r, body, requestID, eventID, verified, readOnly, table, params, key) {
			return
		}
	}
	if err := s.dynamoStoreItem(w, r, body, requestID, eventID, verified, readOnly, table, item); err != nil {
		return
	}
	payload, _ := ddb.EmptyOKJSON()
	s.writeDynamoOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, dynamoEventSource, "PutItem", readOnly)
}

func (s *Server) dynamoGetItem(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	tableName, _ := params["TableName"].(string)
	table, ok := s.dynamoTableOrErr(w, r, body, requestID, eventID, verified, readOnly, tableName)
	if !ok {
		return
	}
	if !s.authorizeDynamoDB(verified, catalog.ActionDynamoDBGetItem, table.TableARN, table.ResourcePolicy) {
		s.writeDynamoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform dynamodb:GetItem.", readOnly, eventID, verified)
		return
	}
	key, err := ddb.ParseItemMap(params["Key"])
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	item, found, err := s.dynamoLoadItem(w, r, body, requestID, eventID, verified, readOnly, table, key)
	if err != nil {
		return
	}
	payload, err := ddb.GetItemJSON(item, found)
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeDynamoOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, dynamoEventSource, "GetItem", readOnly)
}

func (s *Server) dynamoDeleteItem(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	tableName, _ := params["TableName"].(string)
	table, ok := s.dynamoTableOrErr(w, r, body, requestID, eventID, verified, readOnly, tableName)
	if !ok {
		return
	}
	if !s.authorizeDynamoDB(verified, catalog.ActionDynamoDBDeleteItem, table.TableARN, table.ResourcePolicy) {
		s.writeDynamoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform dynamodb:DeleteItem.", readOnly, eventID, verified)
		return
	}
	key, err := ddb.ParseItemMap(params["Key"])
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if !s.dynamoCheckCondition(w, r, body, requestID, eventID, verified, readOnly, table, params, key) {
		return
	}
	itemPK, itemSK, err := ddb.PrimaryKeyStrings(table, key)
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	keysJSON, keysErr := store.DynamoStreamKeysJSON(table, itemPK, itemSK)
	if keysErr != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build stream keys.", readOnly, eventID, verified)
		return
	}
	if err := s.store.DeleteItem(table.AccountID, table.TableName, itemPK, itemSK); err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete item.", readOnly, eventID, verified)
		return
	}
	if err := s.store.AppendDynamoStreamRecord(table.AccountID, table.TableName, "REMOVE", keysJSON, nil); err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to append stream record.", readOnly, eventID, verified)
		return
	}
	payload, _ := ddb.DeleteItemJSON()
	s.writeDynamoOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, dynamoEventSource, "DeleteItem", readOnly)
}

func (s *Server) dynamoUpdateItem(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	tableName, _ := params["TableName"].(string)
	table, ok := s.dynamoTableOrErr(w, r, body, requestID, eventID, verified, readOnly, tableName)
	if !ok {
		return
	}
	if !s.authorizeDynamoDB(verified, catalog.ActionDynamoDBUpdateItem, table.TableARN, table.ResourcePolicy) {
		s.writeDynamoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform dynamodb:UpdateItem.", readOnly, eventID, verified)
		return
	}
	key, err := ddb.ParseItemMap(params["Key"])
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	updateExpr, _ := params["UpdateExpression"].(string)
	names, _ := params["ExpressionAttributeNames"].(map[string]any)
	values, _ := params["ExpressionAttributeValues"].(map[string]any)

	existing, found, err := s.dynamoLoadItem(w, r, body, requestID, eventID, verified, readOnly, table, key)
	if err != nil {
		return
	}
	if !found {
		existing = ddb.ItemMap{}
	}
	if !s.dynamoEvalConditionOnItem(w, r, body, requestID, eventID, verified, readOnly, params, existing) {
		return
	}
	updated, err := ddb.ApplyUpdateExpression(existing, key, updateExpr, names, values)
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err := s.dynamoStoreItem(w, r, body, requestID, eventID, verified, readOnly, table, updated); err != nil {
		return
	}
	payload, err := ddb.UpdateItemJSON(updated)
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeDynamoOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, dynamoEventSource, "UpdateItem", readOnly)
}

func (s *Server) dynamoQuery(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	tableName, _ := params["TableName"].(string)
	table, ok := s.dynamoTableOrErr(w, r, body, requestID, eventID, verified, readOnly, tableName)
	if !ok {
		return
	}
	if !s.authorizeDynamoDB(verified, catalog.ActionDynamoDBQuery, table.TableARN, table.ResourcePolicy) {
		s.writeDynamoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform dynamodb:Query.", readOnly, eventID, verified)
		return
	}
	indexName, _ := params["IndexName"].(string)
	keyCond, err := ddb.KeyConditionFromQueryIndex(table, indexName, params)
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	queryPK, err := ddb.CanonicalAV(keyCond.HashAV)
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	limit := dynamoLimit(params)
	startSK := ""
	gsiSlot := 0
	rangeAttr := table.RangeKeyName
	if indexName != "" {
		gsi, slot, ok := table.GSIByName(indexName)
		if !ok {
			s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
				fmt.Sprintf("index %q not found", indexName), readOnly, eventID, verified)
			return
		}
		gsiSlot = slot
		rangeAttr = gsi.RangeKeyName
	}
	if esk, ok := params["ExclusiveStartKey"]; ok && esk != nil {
		startKey, parseErr := ddb.ParseItemMap(esk)
		if parseErr != nil {
			s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
				parseErr.Error(), readOnly, eventID, verified)
			return
		}
		itemPK, itemSK, err := ddb.PrimaryKeyStrings(table, startKey)
		if err != nil {
			s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
				err.Error(), readOnly, eventID, verified)
			return
		}
		if gsiSlot > 0 {
			_, startSK, err = s.store.GetItemGSISlotKeys(table.AccountID, table.TableName, itemPK, itemSK, gsiSlot)
			if errors.Is(err, store.ErrNoSuchItem) {
				startSK = ""
			} else if err != nil {
				s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
					"Unable to query items.", readOnly, eventID, verified)
				return
			}
		} else {
			startSK = itemSK
		}
	}
	items, lastKey, hasMore, err := s.dynamoQueryCollectLive(
		w, r, body, requestID, eventID, verified, readOnly,
		table, gsiSlot, queryPK, startSK, limit, keyCond.RangeOp, rangeAttr, keyCond.RangeValues,
	)
	if err != nil {
		return
	}
	items, ok = s.dynamoApplyFilterExpression(w, r, body, requestID, eventID, verified, readOnly, params, items)
	if !ok {
		return
	}
	payload, err := ddb.QueryJSON(items, lastKey, hasMore)
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeDynamoOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, dynamoEventSource, "Query", readOnly)
}

func (s *Server) dynamoScan(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	tableName, _ := params["TableName"].(string)
	table, ok := s.dynamoTableOrErr(w, r, body, requestID, eventID, verified, readOnly, tableName)
	if !ok {
		return
	}
	if !s.authorizeDynamoDB(verified, catalog.ActionDynamoDBScan, table.TableARN, table.ResourcePolicy) {
		s.writeDynamoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform dynamodb:Scan.", readOnly, eventID, verified)
		return
	}
	limit := dynamoLimit(params)
	startPK, startSK := "", ""
	if esk, ok := params["ExclusiveStartKey"]; ok && esk != nil {
		startKey, parseErr := ddb.ParseItemMap(esk)
		if parseErr != nil {
			s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
				parseErr.Error(), readOnly, eventID, verified)
			return
		}
		var err error
		startPK, startSK, err = ddb.PrimaryKeyStrings(table, startKey)
		if err != nil {
			s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
				err.Error(), readOnly, eventID, verified)
			return
		}
	}
	items, lastKey, hasMore, err := s.dynamoScanCollectLive(
		w, r, body, requestID, eventID, verified, readOnly,
		table, startPK, startSK, limit,
	)
	if err != nil {
		return
	}
	items, ok = s.dynamoApplyFilterExpression(w, r, body, requestID, eventID, verified, readOnly, params, items)
	if !ok {
		return
	}
	payload, err := ddb.ScanJSON(items, lastKey, hasMore)
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeDynamoOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, dynamoEventSource, "Scan", readOnly)
}

func (s *Server) dynamoBatchGetItem(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	reqItems, ok := params["RequestItems"].(map[string]any)
	if !ok || len(reqItems) == 0 {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"RequestItems is required.", readOnly, eventID, verified)
		return
	}
	responses := map[string][]ddb.ItemMap{}
	unprocessed := map[string]any{}
	total := 0
	for tableName, raw := range reqItems {
		entry, ok := raw.(map[string]any)
		if !ok {
			s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
				"Invalid RequestItems entry.", readOnly, eventID, verified)
			return
		}
		table, ok := s.dynamoTableOrErr(w, r, body, requestID, eventID, verified, readOnly, tableName)
		if !ok {
			return
		}
		if !s.authorizeDynamoDB(verified, catalog.ActionDynamoDBBatchGetItem, table.TableARN, table.ResourcePolicy) {
			s.writeDynamoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
				"User is not authorized to perform dynamodb:BatchGetItem.", readOnly, eventID, verified)
			return
		}
		keys, _ := entry["Keys"].([]any)
		out := []ddb.ItemMap{}
		var leftover []any
		for i, kraw := range keys {
			if total >= dynamoBatchLimit {
				leftover = keys[i:]
				break
			}
			total++
			key, err := ddb.ParseItemMap(kraw)
			if err != nil {
				s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
					err.Error(), readOnly, eventID, verified)
				return
			}
			item, found, err := s.dynamoLoadItem(w, r, body, requestID, eventID, verified, readOnly, table, key)
			if err != nil {
				return
			}
			if found {
				out = append(out, item)
			}
		}
		responses[tableName] = out
		if len(leftover) > 0 {
			rest := map[string]any{"Keys": leftover}
			if proj, ok := entry["ProjectionExpression"]; ok {
				rest["ProjectionExpression"] = proj
			}
			if attrs, ok := entry["AttributesToGet"]; ok {
				rest["AttributesToGet"] = attrs
			}
			unprocessed[tableName] = rest
		}
	}
	payload, err := ddb.BatchGetItemJSON(responses, unprocessed)
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeDynamoOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, dynamoEventSource, "BatchGetItem", readOnly)
}

func (s *Server) dynamoBatchWriteItem(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	reqItems, ok := params["RequestItems"].(map[string]any)
	if !ok || len(reqItems) == 0 {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"RequestItems is required.", readOnly, eventID, verified)
		return
	}
	unprocessed := map[string]any{}
	total := 0
	for tableName, raw := range reqItems {
		ops, ok := raw.([]any)
		if !ok {
			s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
				"Invalid RequestItems entry.", readOnly, eventID, verified)
			return
		}
		table, ok := s.dynamoTableOrErr(w, r, body, requestID, eventID, verified, readOnly, tableName)
		if !ok {
			return
		}
		if !s.authorizeDynamoDB(verified, catalog.ActionDynamoDBBatchWriteItem, table.TableARN, table.ResourcePolicy) {
			s.writeDynamoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
				"User is not authorized to perform dynamodb:BatchWriteItem.", readOnly, eventID, verified)
			return
		}
		var leftover []any
		for i, opRaw := range ops {
			if total >= dynamoBatchLimit {
				leftover = ops[i:]
				break
			}
			total++
			op, ok := opRaw.(map[string]any)
			if !ok {
				s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
					"Invalid batch write request.", readOnly, eventID, verified)
				return
			}
			if put, ok := op["PutRequest"].(map[string]any); ok {
				item, err := ddb.ParseItemMap(put["Item"])
				if err != nil {
					s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
						err.Error(), readOnly, eventID, verified)
					return
				}
				if err := s.dynamoStoreItem(w, r, body, requestID, eventID, verified, readOnly, table, item); err != nil {
					return
				}
				continue
			}
			if del, ok := op["DeleteRequest"].(map[string]any); ok {
				key, err := ddb.ParseItemMap(del["Key"])
				if err != nil {
					s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
						err.Error(), readOnly, eventID, verified)
					return
				}
				itemPK, itemSK, err := ddb.PrimaryKeyStrings(table, key)
				if err != nil {
					s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
						err.Error(), readOnly, eventID, verified)
					return
				}
				if err := s.store.DeleteItem(table.AccountID, table.TableName, itemPK, itemSK); err != nil {
					s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
						"Unable to delete item.", readOnly, eventID, verified)
					return
				}
				continue
			}
			s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
				"PutRequest or DeleteRequest is required.", readOnly, eventID, verified)
			return
		}
		if len(leftover) > 0 {
			unprocessed[tableName] = leftover
		}
	}
	payload, _ := ddb.BatchWriteItemJSON(unprocessed)
	s.writeDynamoOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, dynamoEventSource, "BatchWriteItem", readOnly)
}

func (s *Server) dynamoTransactWriteItems(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	rawItems, ok := params["TransactItems"].([]any)
	if !ok || len(rawItems) == 0 {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"TransactItems is required.", readOnly, eventID, verified)
		return
	}
	if len(rawItems) > dynamoBatchLimit {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"TransactItems exceeds lab limit of 25.", readOnly, eventID, verified)
		return
	}

	actions := make([]store.TransactWriteAction, 0, len(rawItems))
	for _, raw := range rawItems {
		entry, ok := raw.(map[string]any)
		if !ok || len(entry) != 1 {
			s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
				"Each TransactItems entry must contain exactly one of Put, Delete, ConditionCheck, or Update.", readOnly, eventID, verified)
			return
		}
		if _, hasUpdate := entry["Update"]; hasUpdate {
			s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
				"TransactWriteItems Update is not supported in this lab; use Put or Delete.", readOnly, eventID, verified)
			return
		}
		if put, ok := entry["Put"].(map[string]any); ok {
			action, ok := s.dynamoBuildTransactPut(w, r, body, requestID, eventID, verified, readOnly, put)
			if !ok {
				return
			}
			actions = append(actions, action)
			continue
		}
		if del, ok := entry["Delete"].(map[string]any); ok {
			action, ok := s.dynamoBuildTransactDelete(w, r, body, requestID, eventID, verified, readOnly, del)
			if !ok {
				return
			}
			actions = append(actions, action)
			continue
		}
		if check, ok := entry["ConditionCheck"].(map[string]any); ok {
			action, ok := s.dynamoBuildTransactConditionCheck(w, r, body, requestID, eventID, verified, readOnly, check)
			if !ok {
				return
			}
			actions = append(actions, action)
			continue
		}
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"Unsupported TransactItems action.", readOnly, eventID, verified)
		return
	}

	if err := s.store.TransactWriteItems(verified.AccountID, actions); err != nil {
		var canceled *store.TransactionCanceledError
		if errors.As(err, &canceled) {
			s.writeDynamoTransactionCanceled(w, r, body, requestID, canceled, readOnly, eventID, verified)
			return
		}
		if errors.Is(err, store.ErrDynamoTransactUnsupported) ||
			errors.Is(err, store.ErrDynamoTransactLimit) ||
			errors.Is(err, store.ErrDynamoTransactEmpty) {
			s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
				err.Error(), readOnly, eventID, verified)
			return
		}
		if errors.Is(err, store.ErrNoSuchTable) {
			s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
				"Requested resource not found.", readOnly, eventID, verified)
			return
		}
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to execute TransactWriteItems.", readOnly, eventID, verified)
		return
	}
	payload, err := ddb.TransactWriteItemsJSON()
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeDynamoOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, dynamoEventSource, "TransactWriteItems", readOnly)
}

func (s *Server) dynamoTransactGetItems(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	rawItems, ok := params["TransactItems"].([]any)
	if !ok || len(rawItems) == 0 {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"TransactItems is required.", readOnly, eventID, verified)
		return
	}
	if len(rawItems) > dynamoBatchLimit {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"TransactItems exceeds lab limit of 25.", readOnly, eventID, verified)
		return
	}

	keys := make([]store.TransactGetKey, 0, len(rawItems))
	tables := make([]store.DynamoTable, 0, len(rawItems))
	for _, raw := range rawItems {
		entry, ok := raw.(map[string]any)
		if !ok {
			s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
				"Invalid TransactItems entry.", readOnly, eventID, verified)
			return
		}
		get, ok := entry["Get"].(map[string]any)
		if !ok {
			s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
				"Get is required in each TransactItems entry.", readOnly, eventID, verified)
			return
		}
		tableName, _ := get["TableName"].(string)
		table, ok := s.dynamoTableOrErr(w, r, body, requestID, eventID, verified, readOnly, tableName)
		if !ok {
			return
		}
		if table.AccountID != verified.AccountID {
			s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
				"TransactGetItems supports same-account tables only.", readOnly, eventID, verified)
			return
		}
		if !s.authorizeDynamoDB(verified, catalog.ActionDynamoDBGetItem, table.TableARN, table.ResourcePolicy) &&
			!s.authorizeDynamoDB(verified, catalog.ActionDynamoDBTransactGetItems, table.TableARN, table.ResourcePolicy) {
			s.writeDynamoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
				"User is not authorized to perform dynamodb:TransactGetItems.", readOnly, eventID, verified)
			return
		}
		key, err := ddb.ParseItemMap(get["Key"])
		if err != nil {
			s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
				err.Error(), readOnly, eventID, verified)
			return
		}
		itemPK, itemSK, err := ddb.PrimaryKeyStrings(table, key)
		if err != nil {
			s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
				err.Error(), readOnly, eventID, verified)
			return
		}
		keys = append(keys, store.TransactGetKey{TableName: table.TableName, ItemPK: itemPK, ItemSK: itemSK})
		tables = append(tables, table)
	}

	rawItemsJSON, err := s.store.TransactGetItems(verified.AccountID, keys)
	if err != nil {
		if errors.Is(err, store.ErrDynamoTransactLimit) || errors.Is(err, store.ErrDynamoTransactEmpty) {
			s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
				err.Error(), readOnly, eventID, verified)
			return
		}
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to execute TransactGetItems.", readOnly, eventID, verified)
		return
	}

	responses := make([]ddb.ItemMap, len(rawItemsJSON))
	for i, raw := range rawItemsJSON {
		if raw == nil {
			responses[i] = nil
			continue
		}
		stored := store.DynamoStoredItem{ItemJSON: raw}
		// Detect sealed flag via GetItemBytes path when needed.
		full, err := s.store.GetItemBytes(verified.AccountID, keys[i].TableName, keys[i].ItemPK, keys[i].ItemSK)
		if errors.Is(err, store.ErrNoSuchItem) {
			responses[i] = nil
			continue
		}
		if err != nil {
			s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to load item.", readOnly, eventID, verified)
			return
		}
		stored = full
		var materials [][]byte
		if stored.Sealed {
			_, materials, _, err = s.dynamoUnsealTableCMK(w, r, body, requestID, eventID, verified, readOnly, tables[i])
			if err != nil {
				return
			}
		}
		plain, err := ddb.LoadItemJSONAny(tables[i], stored, materials)
		if err != nil {
			s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to decrypt item.", readOnly, eventID, verified)
			return
		}
		item, err := ddb.UnmarshalItemJSON(plain)
		if err != nil {
			s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to decode item.", readOnly, eventID, verified)
			return
		}
		if ddb.ItemExpired(tables[i], item, time.Now().Unix()) {
			responses[i] = nil
			continue
		}
		responses[i] = item
	}

	payload, err := ddb.TransactGetItemsJSON(responses)
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeDynamoOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, dynamoEventSource, "TransactGetItems", readOnly)
}

func (s *Server) dynamoBuildTransactPut(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	put map[string]any,
) (store.TransactWriteAction, bool) {
	tableName, _ := put["TableName"].(string)
	table, ok := s.dynamoTableOrErr(w, r, body, requestID, eventID, verified, readOnly, tableName)
	if !ok {
		return store.TransactWriteAction{}, false
	}
	if table.AccountID != verified.AccountID {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"TransactWriteItems supports same-account tables only.", readOnly, eventID, verified)
		return store.TransactWriteAction{}, false
	}
	if !s.authorizeDynamoDB(verified, catalog.ActionDynamoDBPutItem, table.TableARN, table.ResourcePolicy) &&
		!s.authorizeDynamoDB(verified, catalog.ActionDynamoDBTransactWriteItems, table.TableARN, table.ResourcePolicy) {
		s.writeDynamoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform dynamodb:TransactWriteItems.", readOnly, eventID, verified)
		return store.TransactWriteAction{}, false
	}
	if cond, _ := put["ConditionExpression"].(string); strings.TrimSpace(cond) != "" {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"ConditionExpression on TransactWriteItems Put is not supported in this lab.", readOnly, eventID, verified)
		return store.TransactWriteAction{}, false
	}
	item, err := ddb.ParseItemMap(put["Item"])
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			err.Error(), readOnly, eventID, verified)
		return store.TransactWriteAction{}, false
	}
	itemPK, itemSK, err := ddb.PrimaryKeyStrings(table, item)
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			err.Error(), readOnly, eventID, verified)
		return store.TransactWriteAction{}, false
	}
	plain, err := ddb.MarshalItemJSON(item)
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"Unable to encode item.", readOnly, eventID, verified)
		return store.TransactWriteAction{}, false
	}
	var cmk []byte
	keyID := ""
	if table.SSEType == store.SSETypeKMS {
		cmk, _, keyID, err = s.dynamoUnsealTableCMK(w, r, body, requestID, eventID, verified, readOnly, table)
		if err != nil {
			return store.TransactWriteAction{}, false
		}
	}
	data, sealed, sealedDEK, err := ddb.StoragePayload(table, plain, cmk, keyID)
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to seal item.", readOnly, eventID, verified)
		return store.TransactWriteAction{}, false
	}
	gsiPK, gsiSK, err := ddb.GSIKeyStrings(table, item)
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			err.Error(), readOnly, eventID, verified)
		return store.TransactWriteAction{}, false
	}
	gsi2PK, gsi2SK, err := ddb.GSI2KeyStrings(table, item)
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			err.Error(), readOnly, eventID, verified)
		return store.TransactWriteAction{}, false
	}
	return store.TransactWriteAction{
		Kind: "Put", TableName: table.TableName,
		ItemPK: itemPK, ItemSK: itemSK,
		ItemJSON: data, Sealed: sealed, SealedDEK: sealedDEK,
		GSIPK: gsiPK, GSISK: gsiSK, GSI2PK: gsi2PK, GSI2SK: gsi2SK,
	}, true
}

func (s *Server) dynamoBuildTransactDelete(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	del map[string]any,
) (store.TransactWriteAction, bool) {
	tableName, _ := del["TableName"].(string)
	table, ok := s.dynamoTableOrErr(w, r, body, requestID, eventID, verified, readOnly, tableName)
	if !ok {
		return store.TransactWriteAction{}, false
	}
	if table.AccountID != verified.AccountID {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"TransactWriteItems supports same-account tables only.", readOnly, eventID, verified)
		return store.TransactWriteAction{}, false
	}
	if !s.authorizeDynamoDB(verified, catalog.ActionDynamoDBDeleteItem, table.TableARN, table.ResourcePolicy) &&
		!s.authorizeDynamoDB(verified, catalog.ActionDynamoDBTransactWriteItems, table.TableARN, table.ResourcePolicy) {
		s.writeDynamoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform dynamodb:TransactWriteItems.", readOnly, eventID, verified)
		return store.TransactWriteAction{}, false
	}
	if cond, _ := del["ConditionExpression"].(string); strings.TrimSpace(cond) != "" {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"ConditionExpression on TransactWriteItems Delete is not supported in this lab.", readOnly, eventID, verified)
		return store.TransactWriteAction{}, false
	}
	key, err := ddb.ParseItemMap(del["Key"])
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			err.Error(), readOnly, eventID, verified)
		return store.TransactWriteAction{}, false
	}
	itemPK, itemSK, err := ddb.PrimaryKeyStrings(table, key)
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			err.Error(), readOnly, eventID, verified)
		return store.TransactWriteAction{}, false
	}
	return store.TransactWriteAction{
		Kind: "Delete", TableName: table.TableName,
		ItemPK: itemPK, ItemSK: itemSK,
	}, true
}

func (s *Server) dynamoBuildTransactConditionCheck(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	check map[string]any,
) (store.TransactWriteAction, bool) {
	tableName, _ := check["TableName"].(string)
	table, ok := s.dynamoTableOrErr(w, r, body, requestID, eventID, verified, readOnly, tableName)
	if !ok {
		return store.TransactWriteAction{}, false
	}
	if table.AccountID != verified.AccountID {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"TransactWriteItems supports same-account tables only.", readOnly, eventID, verified)
		return store.TransactWriteAction{}, false
	}
	if !s.authorizeDynamoDB(verified, catalog.ActionDynamoDBConditionCheckItem, table.TableARN, table.ResourcePolicy) &&
		!s.authorizeDynamoDB(verified, catalog.ActionDynamoDBTransactWriteItems, table.TableARN, table.ResourcePolicy) {
		s.writeDynamoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform dynamodb:ConditionCheckItem.", readOnly, eventID, verified)
		return store.TransactWriteAction{}, false
	}
	cond, _ := check["ConditionExpression"].(string)
	cond = strings.TrimSpace(cond)
	// Lab subset: empty expression or attribute_exists(...) → item must exist.
	if cond != "" && !dynamoLabAttributeExistsOnly(cond) {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"ConditionCheck supports only existence (attribute_exists) in this lab.", readOnly, eventID, verified)
		return store.TransactWriteAction{}, false
	}
	key, err := ddb.ParseItemMap(check["Key"])
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			err.Error(), readOnly, eventID, verified)
		return store.TransactWriteAction{}, false
	}
	itemPK, itemSK, err := ddb.PrimaryKeyStrings(table, key)
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			err.Error(), readOnly, eventID, verified)
		return store.TransactWriteAction{}, false
	}
	return store.TransactWriteAction{
		Kind: "ConditionCheck", TableName: table.TableName,
		ItemPK: itemPK, ItemSK: itemSK, ConditionEmpty: true,
	}, true
}

func dynamoLabAttributeExistsOnly(expr string) bool {
	expr = strings.TrimSpace(expr)
	lower := strings.ToLower(expr)
	return strings.HasPrefix(lower, "attribute_exists(") && strings.HasSuffix(expr, ")")
}

func (s *Server) writeDynamoTransactionCanceled(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID string,
	canceled *store.TransactionCanceledError,
	readOnly bool,
	eventID string,
	verified *authn.Verified,
) {
	_ = body
	payload, err := ddb.TransactionCanceledJSON("", canceled.Reasons)
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", dynamoJSONContentType)
	w.WriteHeader(http.StatusBadRequest)
	_, _ = w.Write(payload)

	accessKeyID := ""
	accountID := ""
	if verified != nil {
		accessKeyID = verified.AccessKeyID
		accountID = verified.AccountID
	}
	s.auditAPIError(r, requestID, eventID, "TransactionCanceledException",
		"Transaction cancelled, please refer cancellation reasons for specific reasons",
		readOnly, accessKeyID, accountID, verified != nil)
}

func (s *Server) dynamoPutResourcePolicy(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	tableName, _ := params["ResourceArn"].(string)
	if tableName == "" {
		tableName, _ = params["TableName"].(string)
	}
	// ResourceArn may be a full ARN; extract table name when needed.
	if strings.Contains(tableName, ":table/") {
		tableName = tableName[strings.LastIndex(tableName, "/")+1:]
	}
	table, ok := s.dynamoTableOrErr(w, r, body, requestID, eventID, verified, readOnly, tableName)
	if !ok {
		return
	}
	if !s.authorizeDynamoDB(verified, catalog.ActionDynamoDBPutResourcePolicy, table.TableARN, table.ResourcePolicy) {
		s.writeDynamoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform dynamodb:PutResourcePolicy.", readOnly, eventID, verified)
		return
	}
	policy, _ := params["Policy"].(string)
	if strings.TrimSpace(policy) == "" {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"Policy is required.", readOnly, eventID, verified)
		return
	}
	if err := authz.ValidateResourcePolicyDocument(policy); err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err := s.store.PutResourcePolicy(table.AccountID, table.TableName, policy); err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to put resource policy.", readOnly, eventID, verified)
		return
	}
	payload, _ := ddb.EmptyOKJSON()
	s.writeDynamoOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, dynamoEventSource, "PutResourcePolicy", readOnly)
}

func (s *Server) dynamoGetResourcePolicy(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	tableName, _ := params["ResourceArn"].(string)
	if tableName == "" {
		tableName, _ = params["TableName"].(string)
	}
	if strings.Contains(tableName, ":table/") {
		tableName = tableName[strings.LastIndex(tableName, "/")+1:]
	}
	table, ok := s.dynamoTableOrErr(w, r, body, requestID, eventID, verified, readOnly, tableName)
	if !ok {
		return
	}
	if !s.authorizeDynamoDB(verified, catalog.ActionDynamoDBGetResourcePolicy, table.TableARN, table.ResourcePolicy) {
		s.writeDynamoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform dynamodb:GetResourcePolicy.", readOnly, eventID, verified)
		return
	}
	policy, err := s.store.GetResourcePolicy(table.AccountID, table.TableName)
	if errors.Is(err, store.ErrNoSuchResourcePolicy) {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "PolicyNotFoundException",
			"Resource policy not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to get resource policy.", readOnly, eventID, verified)
		return
	}
	payload, err := ddb.GetResourcePolicyJSON(policy)
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeDynamoOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, dynamoEventSource, "GetResourcePolicy", readOnly)
}

func (s *Server) dynamoDeleteResourcePolicy(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	tableName, _ := params["ResourceArn"].(string)
	if tableName == "" {
		tableName, _ = params["TableName"].(string)
	}
	if strings.Contains(tableName, ":table/") {
		tableName = tableName[strings.LastIndex(tableName, "/")+1:]
	}
	table, ok := s.dynamoTableOrErr(w, r, body, requestID, eventID, verified, readOnly, tableName)
	if !ok {
		return
	}
	if !s.authorizeDynamoDB(verified, catalog.ActionDynamoDBDeleteResourcePolicy, table.TableARN, table.ResourcePolicy) {
		s.writeDynamoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform dynamodb:DeleteResourcePolicy.", readOnly, eventID, verified)
		return
	}
	if err := s.store.DeleteResourcePolicy(table.AccountID, table.TableName); err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete resource policy.", readOnly, eventID, verified)
		return
	}
	payload, _ := ddb.EmptyOKJSON()
	s.writeDynamoOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, dynamoEventSource, "DeleteResourcePolicy", readOnly)
}

func (s *Server) dynamoStoreItem(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	table store.DynamoTable,
	item ddb.ItemMap,
) error {
	itemPK, itemSK, err := ddb.PrimaryKeyStrings(table, item)
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			err.Error(), readOnly, eventID, verified)
		return err
	}
	eventName := "INSERT"
	if table.StreamEnabled {
		if _, err := s.store.GetItemBytes(table.AccountID, table.TableName, itemPK, itemSK); err == nil {
			eventName = "MODIFY"
		} else if !errors.Is(err, store.ErrNoSuchItem) {
			s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to put item.", readOnly, eventID, verified)
			return err
		}
	}
	plain, err := ddb.MarshalItemJSON(item)
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"Unable to encode item.", readOnly, eventID, verified)
		return err
	}
	var cmk []byte
	keyID := ""
	if table.SSEType == store.SSETypeKMS {
		cmk, _, keyID, err = s.dynamoUnsealTableCMK(w, r, body, requestID, eventID, verified, readOnly, table)
		if err != nil {
			return err
		}
	}
	data, sealed, sealedDEK, err := ddb.StoragePayload(table, plain, cmk, keyID)
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to seal item.", readOnly, eventID, verified)
		return err
	}
	gsiPK, gsiSK, err := ddb.GSIKeyStrings(table, item)
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			err.Error(), readOnly, eventID, verified)
		return err
	}
	gsi2PK, gsi2SK, err := ddb.GSI2KeyStrings(table, item)
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			err.Error(), readOnly, eventID, verified)
		return err
	}
	if err := s.store.PutItemBytesMultiGSI(table.AccountID, table.TableName, itemPK, itemSK, gsiPK, gsiSK, gsi2PK, gsi2SK, data, sealed, sealedDEK); err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to put item.", readOnly, eventID, verified)
		return err
	}
	if table.StreamEnabled {
		keysJSON, keysErr := store.DynamoStreamKeysJSON(table, itemPK, itemSK)
		if keysErr != nil {
			s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to build stream keys.", readOnly, eventID, verified)
			return keysErr
		}
		if err := s.store.AppendDynamoStreamRecord(table.AccountID, table.TableName, eventName, keysJSON, plain); err != nil {
			s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to append stream record.", readOnly, eventID, verified)
			return err
		}
	}
	return nil
}

func (s *Server) dynamoLoadItem(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	table store.DynamoTable,
	key ddb.ItemMap,
) (ddb.ItemMap, bool, error) {
	itemPK, itemSK, err := ddb.PrimaryKeyStrings(table, key)
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			err.Error(), readOnly, eventID, verified)
		return nil, false, err
	}
	stored, err := s.store.GetItemBytes(table.AccountID, table.TableName, itemPK, itemSK)
	if errors.Is(err, store.ErrNoSuchItem) {
		return nil, false, nil
	}
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to get item.", readOnly, eventID, verified)
		return nil, false, err
	}
	var materials [][]byte
	if stored.Sealed {
		_, materials, _, err = s.dynamoUnsealTableCMK(w, r, body, requestID, eventID, verified, readOnly, table)
		if err != nil {
			return nil, false, err
		}
	}
	plain, err := ddb.LoadItemJSONAny(table, stored, materials)
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to decrypt item.", readOnly, eventID, verified)
		return nil, false, err
	}
	item, err := ddb.UnmarshalItemJSON(plain)
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to decode item.", readOnly, eventID, verified)
		return nil, false, err
	}
	if ddb.ItemExpired(table, item, time.Now().Unix()) {
		return nil, false, nil
	}
	return item, true, nil
}

// dynamoCheckCondition loads the current item for key and evaluates ConditionExpression.
// Returns false when the handler already wrote an error response.
func (s *Server) dynamoCheckCondition(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	table store.DynamoTable,
	params map[string]any,
	key ddb.ItemMap,
) bool {
	cond, _ := params["ConditionExpression"].(string)
	if strings.TrimSpace(cond) == "" {
		return true
	}
	existing, _, err := s.dynamoLoadItem(w, r, body, requestID, eventID, verified, readOnly, table, key)
	if err != nil {
		return false
	}
	if existing == nil {
		existing = ddb.ItemMap{}
	}
	return s.dynamoEvalConditionOnItem(w, r, body, requestID, eventID, verified, readOnly, params, existing)
}

func (s *Server) dynamoEvalConditionOnItem(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
	item ddb.ItemMap,
) bool {
	cond, _ := params["ConditionExpression"].(string)
	if strings.TrimSpace(cond) == "" {
		return true
	}
	names, _ := params["ExpressionAttributeNames"].(map[string]any)
	values, _ := params["ExpressionAttributeValues"].(map[string]any)
	if err := ddb.EvaluateConditionExpression(item, cond, names, values); err != nil {
		if errors.Is(err, ddb.ErrConditionalCheckFailed) {
			s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ConditionalCheckFailedException",
				"The conditional request failed", readOnly, eventID, verified)
			return false
		}
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			err.Error(), readOnly, eventID, verified)
		return false
	}
	return true
}

// dynamoApplyFilterExpression filters Query/Scan items. Unsupported FilterExpression
// operators fail closed with ValidationException (never silently ignored).
func (s *Server) dynamoApplyFilterExpression(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
	items []ddb.ItemMap,
) ([]ddb.ItemMap, bool) {
	filter, _ := params["FilterExpression"].(string)
	if strings.TrimSpace(filter) == "" {
		return items, true
	}
	names, _ := params["ExpressionAttributeNames"].(map[string]any)
	values, _ := params["ExpressionAttributeValues"].(map[string]any)
	// Fail closed on unsupported expressions even when the page is empty.
	if err := ddb.EvaluateFilterExpression(ddb.ItemMap{}, filter, names, values); err != nil &&
		!errors.Is(err, ddb.ErrFilterNoMatch) {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			err.Error(), readOnly, eventID, verified)
		return nil, false
	}
	out := make([]ddb.ItemMap, 0, len(items))
	for _, it := range items {
		err := ddb.EvaluateFilterExpression(it, filter, names, values)
		if errors.Is(err, ddb.ErrFilterNoMatch) {
			continue
		}
		if err != nil {
			s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
				err.Error(), readOnly, eventID, verified)
			return nil, false
		}
		out = append(out, it)
	}
	return out, true
}

// dynamoQueryCollectLive over-fetches Query pages until Limit live (non-TTL-expired,
// sort-key matching) items are collected or the store is exhausted.
func (s *Server) dynamoQueryCollectLive(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	table store.DynamoTable,
	gsiSlot int,
	queryPK, startSK string,
	limit int,
	rangeOp, rangeAttr string,
	rangeValues []map[string]any,
) (items []ddb.ItemMap, lastKey ddb.ItemMap, hasMore bool, err error) {
	curSK := startSK
	const maxRounds = 32
	for round := 0; round < maxRounds; round++ {
		need := 0
		if limit > 0 {
			need = limit - len(items)
			if need <= 0 {
				break
			}
		}
		fetchLimit := need
		if fetchLimit > 0 {
			fetchLimit = need * 4
			if fetchLimit < 32 {
				fetchLimit = 32
			}
		}
		var page store.ItemPage
		if gsiSlot > 0 {
			page, err = s.store.QueryGSISlotItems(table.AccountID, table.TableName, gsiSlot, queryPK, fetchLimit, curSK)
		} else {
			page, err = s.store.QueryItems(table.AccountID, table.TableName, queryPK, fetchLimit, curSK)
		}
		if err != nil {
			s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to query items.", readOnly, eventID, verified)
			return nil, nil, false, err
		}
		decoded, pageLast, decErr := s.dynamoDecodePage(w, r, body, requestID, eventID, verified, readOnly, table, page)
		if decErr != nil {
			return nil, nil, false, decErr
		}
		for _, it := range decoded {
			if rangeOp != "" && !ddb.ItemMatchesSortKey(it, rangeAttr, rangeOp, rangeValues) {
				continue
			}
			items = append(items, it)
			if limit > 0 && len(items) >= limit {
				break
			}
		}
		if limit > 0 && len(items) >= limit {
			items = items[:limit]
			hasMore = page.HasMore || len(decoded) > 0
			if hasMore && len(items) > 0 {
				last := items[len(items)-1]
				itemPK, itemSK, keyErr := ddb.PrimaryKeyStrings(table, last)
				if keyErr != nil {
					s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
						"Unable to build LastEvaluatedKey.", readOnly, eventID, verified)
					return nil, nil, false, keyErr
				}
				lastKey, keyErr = ddb.KeyFromCanonical(table, itemPK, itemSK)
				if keyErr != nil {
					s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
						"Unable to build LastEvaluatedKey.", readOnly, eventID, verified)
					return nil, nil, false, keyErr
				}
			}
			return items, lastKey, hasMore, nil
		}
		if !page.HasMore {
			return items, nil, false, nil
		}
		hasMore = true
		lastKey = pageLast
		curSK = page.LastSK
		if limit == 0 {
			// Unlimited: one store page (decoded) is enough.
			return items, lastKey, hasMore, nil
		}
	}
	return items, lastKey, hasMore, nil
}

func (s *Server) dynamoScanCollectLive(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	table store.DynamoTable,
	startPK, startSK string,
	limit int,
) (items []ddb.ItemMap, lastKey ddb.ItemMap, hasMore bool, err error) {
	curPK, curSK := startPK, startSK
	const maxRounds = 32
	for round := 0; round < maxRounds; round++ {
		need := 0
		if limit > 0 {
			need = limit - len(items)
			if need <= 0 {
				break
			}
		}
		fetchLimit := need
		if fetchLimit > 0 {
			fetchLimit = need * 4
			if fetchLimit < 32 {
				fetchLimit = 32
			}
		}
		page, err := s.store.ScanItems(table.AccountID, table.TableName, fetchLimit, curPK, curSK)
		if err != nil {
			s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to scan items.", readOnly, eventID, verified)
			return nil, nil, false, err
		}
		decoded, pageLast, decErr := s.dynamoDecodePage(w, r, body, requestID, eventID, verified, readOnly, table, page)
		if decErr != nil {
			return nil, nil, false, decErr
		}
		items = append(items, decoded...)
		if limit > 0 && len(items) >= limit {
			items = items[:limit]
			hasMore = page.HasMore || len(decoded) > len(items) // truncated from this page
			if page.HasMore || len(decoded) > 0 {
				hasMore = true
			}
			if hasMore && len(items) > 0 {
				last := items[len(items)-1]
				itemPK, itemSK, keyErr := ddb.PrimaryKeyStrings(table, last)
				if keyErr != nil {
					s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
						"Unable to build LastEvaluatedKey.", readOnly, eventID, verified)
					return nil, nil, false, keyErr
				}
				lastKey, keyErr = ddb.KeyFromCanonical(table, itemPK, itemSK)
				if keyErr != nil {
					s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
						"Unable to build LastEvaluatedKey.", readOnly, eventID, verified)
					return nil, nil, false, keyErr
				}
			}
			return items, lastKey, hasMore, nil
		}
		if !page.HasMore {
			return items, nil, false, nil
		}
		hasMore = true
		lastKey = pageLast
		curPK, curSK = page.LastPK, page.LastSK
		if limit == 0 {
			return items, lastKey, hasMore, nil
		}
	}
	return items, lastKey, hasMore, nil
}

func (s *Server) dynamoDecodePage(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	table store.DynamoTable,
	page store.ItemPage,
) ([]ddb.ItemMap, ddb.ItemMap, error) {
	var materials [][]byte
	var err error
	needCMK := false
	for _, it := range page.Items {
		if it.Sealed {
			needCMK = true
			break
		}
	}
	if needCMK {
		_, materials, _, err = s.dynamoUnsealTableCMK(w, r, body, requestID, eventID, verified, readOnly, table)
		if err != nil {
			return nil, nil, err
		}
	}
	items := make([]ddb.ItemMap, 0, len(page.Items))
	nowUnix := time.Now().Unix()
	for _, stored := range page.Items {
		plain, err := ddb.LoadItemJSONAny(table, stored, materials)
		if err != nil {
			s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to decrypt item.", readOnly, eventID, verified)
			return nil, nil, err
		}
		item, err := ddb.UnmarshalItemJSON(plain)
		if err != nil {
			s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to decode item.", readOnly, eventID, verified)
			return nil, nil, err
		}
		if ddb.ItemExpired(table, item, nowUnix) {
			continue
		}
		items = append(items, item)
	}
	var lastKey ddb.ItemMap
	if page.HasMore {
		lastKey, err = ddb.KeyFromCanonical(table, page.LastPK, page.LastSK)
		if err != nil {
			s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to build LastEvaluatedKey.", readOnly, eventID, verified)
			return nil, nil, err
		}
	}
	return items, lastKey, nil
}

func (s *Server) dynamoUnsealTableCMK(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	table store.DynamoTable,
) (cmk []byte, materials [][]byte, keyID string, err error) {
	if table.KMSKeyID == "" {
		err = errors.New("table KMS key missing")
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"Table SSE-KMS key is not configured.", readOnly, eventID, verified)
		return nil, nil, "", err
	}
	// Resolve under the table owner account (XA GetItem uses caller credentials).
	keyID, err = s.store.ResolveKeyID(table.AccountID, table.KMSKeyID)
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"Table SSE-KMS key not found.", readOnly, eventID, verified)
		return nil, nil, "", err
	}
	key, err := s.store.GetKey(keyID)
	if err != nil || key.AccountID != table.AccountID {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"Table SSE-KMS key not found.", readOnly, eventID, verified)
		return nil, nil, "", err
	}
	if key.KeyState != store.KeyStateEnabled {
		err = errors.New("kms key disabled")
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"Table SSE-KMS key is disabled.", readOnly, eventID, verified)
		return nil, nil, "", err
	}
	encCtx := ddb.EncryptionContext(table)
	if !s.authorizeKMSOp(verified, catalog.ActionKMSDecrypt, key, encCtx) {
		err = errors.New("kms decrypt denied")
		s.writeDynamoError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform kms:Decrypt on the table SSE-KMS key.", readOnly, eventID, verified)
		return nil, nil, "", err
	}
	materials, err = s.store.UnsealAllKeyMaterials(keyID)
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to load key material.", readOnly, eventID, verified)
		return nil, nil, "", err
	}
	if len(materials) == 0 {
		err = errors.New("no key material")
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to load key material.", readOnly, eventID, verified)
		return nil, nil, "", err
	}
	return materials[0], materials, keyID, nil
}

func dynamoLimit(params map[string]any) int {
	switch v := params["Limit"].(type) {
	case float64:
		if v > 0 {
			return int(v)
		}
	case int:
		if v > 0 {
			return v
		}
	}
	return 0
}

func (s *Server) writeDynamoOK(w http.ResponseWriter, requestID string, payload []byte) {
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", dynamoJSONContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeDynamoError(
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
	w.Header().Set("Content-Type", dynamoJSONContentType)
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
