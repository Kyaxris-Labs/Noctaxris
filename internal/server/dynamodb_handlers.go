package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

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
	case catalog.ActionDynamoDBPutResourcePolicy:
		s.dynamoPutResourcePolicy(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionDynamoDBGetResourcePolicy:
		s.dynamoGetResourcePolicy(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionDynamoDBDeleteResourcePolicy:
		s.dynamoDeleteResourcePolicy(w, r, body, requestID, eventID, verified, readOnly, params)
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
	case "PutResourcePolicy":
		return catalog.ActionDynamoDBPutResourcePolicy
	case "GetResourcePolicy":
		return catalog.ActionDynamoDBGetResourcePolicy
	case "DeleteResourcePolicy":
		return catalog.ActionDynamoDBDeleteResourcePolicy
	default:
		return action
	}
}

func (s *Server) authorizeDynamoDB(verified *authn.Verified, action, resource, resourcePolicy string) bool {
	identityDocs := s.identityDocs(verified.Principal)
	decision := authz.EvaluateDynamoDB(authz.DynamoDBRequest{
		Caller: authz.RequestContext{
			Principal: verified.Principal,
			Action:    action,
			Resource:  resource,
			Region:    verified.Region,
			ConditionKeys: map[string]string{
				"aws:PrincipalAccount": verified.AccountID,
				"aws:RequestedRegion":  verified.Region,
			},
		},
		IdentityDocs:      identityDocs,
		ResourcePolicyDoc: resourcePolicy,
	})
	if decision != authz.Allow {
		return false
	}
	if sessionDocs := s.sessionPolicyDocs(verified.AccessKeyID); len(sessionDocs) > 0 {
		return authz.EvaluateWithSession(authz.RequestContext{
			Principal: verified.Principal,
			Action:    action,
			Resource:  resource,
			Region:    verified.Region,
			ConditionKeys: map[string]string{
				"aws:PrincipalAccount": verified.AccountID,
				"aws:RequestedRegion":  verified.Region,
			},
		}, identityDocs, sessionDocs) == authz.Allow
	}
	return true
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
	table, err := s.store.GetTable(verified.AccountID, tableName)
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
				kmsKeyID = key.ARN
			}
		}
	}

	table, err := s.store.CreateTable(
		verified.AccountID, region, tableName,
		hashKey, hashType, rangeKey, rangeType, sseType, kmsKeyID,
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
	err := s.store.DeleteTable(verified.AccountID, tableName)
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
	sseSpec, ok := params["SSESpecification"].(map[string]any)
	if !ok {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"SSESpecification is required.", readOnly, eventID, verified)
		return
	}
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
			kmsKeyID = key.ARN
		}
	}
	if err := s.store.UpdateTableSSE(verified.AccountID, tableName, sseType, kmsKeyID); err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to update table.", readOnly, eventID, verified)
		return
	}
	updated, err := s.store.GetTable(verified.AccountID, tableName)
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to load table.", readOnly, eventID, verified)
		return
	}
	payload, err := ddb.CreateTableJSON(updated)
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeDynamoOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, dynamoEventSource, "UpdateTable", readOnly)
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
	itemPK, itemSK, err := ddb.PrimaryKeyStrings(table, key)
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err := s.store.DeleteItem(verified.AccountID, table.TableName, itemPK, itemSK); err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete item.", readOnly, eventID, verified)
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
	hashAV, err := ddb.HashKeyFromQuery(table, params)
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	itemPK, err := ddb.CanonicalAV(hashAV)
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	limit := dynamoLimit(params)
	startSK := ""
	if esk, ok := params["ExclusiveStartKey"]; ok && esk != nil {
		startKey, parseErr := ddb.ParseItemMap(esk)
		if parseErr != nil {
			s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
				parseErr.Error(), readOnly, eventID, verified)
			return
		}
		_, startSK, err = ddb.PrimaryKeyStrings(table, startKey)
		if err != nil {
			s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
				err.Error(), readOnly, eventID, verified)
			return
		}
	}
	page, err := s.store.QueryItems(verified.AccountID, table.TableName, itemPK, limit, startSK)
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to query items.", readOnly, eventID, verified)
		return
	}
	items, lastKey, err := s.dynamoDecodePage(w, r, body, requestID, eventID, verified, readOnly, table, page)
	if err != nil {
		return
	}
	payload, err := ddb.QueryJSON(items, lastKey, page.HasMore)
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
	page, err := s.store.ScanItems(verified.AccountID, table.TableName, limit, startPK, startSK)
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to scan items.", readOnly, eventID, verified)
		return
	}
	items, lastKey, err := s.dynamoDecodePage(w, r, body, requestID, eventID, verified, readOnly, table, page)
	if err != nil {
		return
	}
	payload, err := ddb.ScanJSON(items, lastKey, page.HasMore)
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
	total := 0
	for tableName, raw := range reqItems {
		if total >= dynamoBatchLimit {
			break
		}
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
		for _, kraw := range keys {
			if total >= dynamoBatchLimit {
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
	}
	payload, err := ddb.BatchGetItemJSON(responses, map[string]any{})
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
		for _, opRaw := range ops {
			if total >= dynamoBatchLimit {
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
				if err := s.store.DeleteItem(verified.AccountID, table.TableName, itemPK, itemSK); err != nil {
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
	}
	payload, _ := ddb.BatchWriteItemJSON(map[string]any{})
	s.writeDynamoOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, dynamoEventSource, "BatchWriteItem", readOnly)
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
	if err := s.store.PutResourcePolicy(verified.AccountID, table.TableName, policy); err != nil {
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
	policy, err := s.store.GetResourcePolicy(verified.AccountID, table.TableName)
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
	if err := s.store.DeleteResourcePolicy(verified.AccountID, table.TableName); err != nil {
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
	plain, err := ddb.MarshalItemJSON(item)
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"Unable to encode item.", readOnly, eventID, verified)
		return err
	}
	var cmk []byte
	keyID := ""
	if table.SSEType == store.SSETypeKMS {
		cmk, keyID, err = s.dynamoUnsealTableCMK(w, r, body, requestID, eventID, verified, readOnly, table)
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
	if err := s.store.PutItemBytes(verified.AccountID, table.TableName, itemPK, itemSK, data, sealed, sealedDEK); err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to put item.", readOnly, eventID, verified)
		return err
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
	stored, err := s.store.GetItemBytes(verified.AccountID, table.TableName, itemPK, itemSK)
	if errors.Is(err, store.ErrNoSuchItem) {
		return nil, false, nil
	}
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to get item.", readOnly, eventID, verified)
		return nil, false, err
	}
	var cmk []byte
	if stored.Sealed {
		cmk, _, err = s.dynamoUnsealTableCMK(w, r, body, requestID, eventID, verified, readOnly, table)
		if err != nil {
			return nil, false, err
		}
	}
	plain, err := ddb.LoadItemJSON(table, stored, cmk)
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
	return item, true, nil
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
	var cmk []byte
	var err error
	needCMK := false
	for _, it := range page.Items {
		if it.Sealed {
			needCMK = true
			break
		}
	}
	if needCMK {
		cmk, _, err = s.dynamoUnsealTableCMK(w, r, body, requestID, eventID, verified, readOnly, table)
		if err != nil {
			return nil, nil, err
		}
	}
	items := make([]ddb.ItemMap, 0, len(page.Items))
	for _, stored := range page.Items {
		plain, err := ddb.LoadItemJSON(table, stored, cmk)
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
) ([]byte, string, error) {
	if table.KMSKeyID == "" {
		err := errors.New("table KMS key missing")
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"Table SSE-KMS key is not configured.", readOnly, eventID, verified)
		return nil, "", err
	}
	keyID, err := s.store.ResolveKeyID(verified.AccountID, table.KMSKeyID)
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"Table SSE-KMS key not found.", readOnly, eventID, verified)
		return nil, "", err
	}
	key, err := s.store.GetKey(keyID)
	if err != nil || key.AccountID != verified.AccountID {
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"Table SSE-KMS key not found.", readOnly, eventID, verified)
		return nil, "", err
	}
	if key.KeyState != store.KeyStateEnabled {
		err := errors.New("kms key disabled")
		s.writeDynamoError(w, r, body, requestID, http.StatusBadRequest, "ValidationException",
			"Table SSE-KMS key is disabled.", readOnly, eventID, verified)
		return nil, "", err
	}
	cmk, err := s.store.UnsealKeyMaterial(keyID)
	if err != nil {
		s.writeDynamoError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to load key material.", readOnly, eventID, verified)
		return nil, "", err
	}
	return cmk, keyID, nil
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
