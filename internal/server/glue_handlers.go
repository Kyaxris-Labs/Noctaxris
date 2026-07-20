package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	gluesvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/glue"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	glueJSONContentType = "application/x-amz-json-1.1"
	glueEventSource     = "glue.amazonaws.com"
)

func (s *Server) handleGlue(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = glueAction(action)

	switch action {
	case catalog.ActionGlueCreateDatabase:
		s.glueCreateDatabase(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionGlueGetDatabase:
		s.glueGetDatabase(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionGlueGetDatabases:
		s.glueGetDatabases(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionGlueDeleteDatabase:
		s.glueDeleteDatabase(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionGlueCreateTable:
		s.glueCreateTable(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionGlueGetTable:
		s.glueGetTable(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionGlueGetTables:
		s.glueGetTables(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionGlueDeleteTable:
		s.glueDeleteTable(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeGlueError(w, r, body, requestID, http.StatusNotImplemented, "InternalFailure",
			"This Glue action is not implemented.", readOnly, eventID, verified)
	}
}

func glueAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "CreateDatabase":
		return catalog.ActionGlueCreateDatabase
	case "GetDatabase":
		return catalog.ActionGlueGetDatabase
	case "GetDatabases":
		return catalog.ActionGlueGetDatabases
	case "DeleteDatabase":
		return catalog.ActionGlueDeleteDatabase
	case "CreateTable":
		return catalog.ActionGlueCreateTable
	case "GetTable":
		return catalog.ActionGlueGetTable
	case "GetTables":
		return catalog.ActionGlueGetTables
	case "DeleteTable":
		return catalog.ActionGlueDeleteTable
	default:
		return action
	}
}

func (s *Server) glueCreateDatabase(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	dbIn, _ := params["DatabaseInput"].(map[string]any)
	name, _ := dbIn["Name"].(string)
	desc, _ := dbIn["Description"].(string)
	if strings.TrimSpace(name) == "" {
		s.writeGlueError(w, r, body, requestID, http.StatusBadRequest, "InvalidInputException",
			"DatabaseInput.Name is required.", readOnly, eventID, verified)
		return
	}
	if !s.authorize(verified, catalog.ActionGlueCreateDatabase, "*") {
		s.writeGlueError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform glue:CreateDatabase.", readOnly, eventID, verified)
		return
	}
	_, err := s.store.CreateGlueDatabase(verified.AccountID, name, desc)
	if errors.Is(err, store.ErrGlueAlreadyExists) {
		s.writeGlueError(w, r, body, requestID, http.StatusBadRequest, "AlreadyExistsException",
			"Database already exists.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeGlueError(w, r, body, requestID, http.StatusInternalServerError, "InternalServiceException",
			"Unable to create database.", readOnly, eventID, verified)
		return
	}
	payload, _ := gluesvc.CreateDatabaseJSON()
	s.writeGlueOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, glueEventSource, "CreateDatabase", readOnly)
}

func (s *Server) glueGetDatabase(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name, _ := params["Name"].(string)
	if !s.authorize(verified, catalog.ActionGlueGetDatabase, "*") {
		s.writeGlueError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform glue:GetDatabase.", readOnly, eventID, verified)
		return
	}
	d, err := s.store.GetGlueDatabase(verified.AccountID, name)
	if errors.Is(err, store.ErrGlueNotFound) {
		s.writeGlueError(w, r, body, requestID, http.StatusBadRequest, "EntityNotFoundException",
			"Database not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeGlueError(w, r, body, requestID, http.StatusInternalServerError, "InternalServiceException",
			"Unable to get database.", readOnly, eventID, verified)
		return
	}
	payload, _ := gluesvc.GetDatabaseJSON(d)
	s.writeGlueOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, glueEventSource, "GetDatabase", readOnly)
}

func (s *Server) glueGetDatabases(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool,
) {
	if !s.authorize(verified, catalog.ActionGlueGetDatabases, "*") {
		s.writeGlueError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform glue:GetDatabases.", readOnly, eventID, verified)
		return
	}
	dbs, err := s.store.GetGlueDatabases(verified.AccountID)
	if err != nil {
		s.writeGlueError(w, r, body, requestID, http.StatusInternalServerError, "InternalServiceException",
			"Unable to get databases.", readOnly, eventID, verified)
		return
	}
	payload, _ := gluesvc.GetDatabasesJSON(dbs)
	s.writeGlueOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, glueEventSource, "GetDatabases", readOnly)
}

func (s *Server) glueDeleteDatabase(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name, _ := params["Name"].(string)
	if !s.authorize(verified, catalog.ActionGlueDeleteDatabase, "*") {
		s.writeGlueError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform glue:DeleteDatabase.", readOnly, eventID, verified)
		return
	}
	err := s.store.DeleteGlueDatabase(verified.AccountID, name)
	if errors.Is(err, store.ErrGlueNotFound) {
		s.writeGlueError(w, r, body, requestID, http.StatusBadRequest, "EntityNotFoundException",
			"Database not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeGlueError(w, r, body, requestID, http.StatusInternalServerError, "InternalServiceException",
			"Unable to delete database.", readOnly, eventID, verified)
		return
	}
	payload, _ := gluesvc.DeleteDatabaseJSON()
	s.writeGlueOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, glueEventSource, "DeleteDatabase", readOnly)
}

func (s *Server) glueCreateTable(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	dbName, _ := params["DatabaseName"].(string)
	tblIn, _ := params["TableInput"].(map[string]any)
	name, _ := tblIn["Name"].(string)
	desc, _ := tblIn["Description"].(string)
	in := store.GlueTableCreate{
		DatabaseName:  dbName,
		Name:          name,
		Description:   desc,
		Columns:       []store.GlueColumn{},
		PartitionKeys: []store.GlueColumn{},
		SerDeInfo:     store.GlueSerDeInfo{Parameters: map[string]string{}},
	}
	if sd, ok := tblIn["StorageDescriptor"].(map[string]any); ok {
		in.StorageLocation, _ = sd["Location"].(string)
		in.InputFormat, _ = sd["InputFormat"].(string)
		in.OutputFormat, _ = sd["OutputFormat"].(string)
		if cols, ok := sd["Columns"].([]any); ok {
			for _, c := range cols {
				cm, _ := c.(map[string]any)
				cn, _ := cm["Name"].(string)
				ct, _ := cm["Type"].(string)
				in.Columns = append(in.Columns, store.GlueColumn{Name: cn, Type: ct})
			}
		}
		if serde, ok := sd["SerdeInfo"].(map[string]any); ok {
			in.SerDeInfo.Name, _ = serde["Name"].(string)
			in.SerDeInfo.SerializationLibrary, _ = serde["SerializationLibrary"].(string)
			if paramsMap, ok := serde["Parameters"].(map[string]any); ok {
				for k, v := range paramsMap {
					if vs, ok := v.(string); ok {
						in.SerDeInfo.Parameters[k] = vs
					}
				}
			}
		}
	}
	if pks, ok := tblIn["PartitionKeys"].([]any); ok {
		for _, c := range pks {
			cm, _ := c.(map[string]any)
			cn, _ := cm["Name"].(string)
			ct, _ := cm["Type"].(string)
			in.PartitionKeys = append(in.PartitionKeys, store.GlueColumn{Name: cn, Type: ct})
		}
	}
	if !s.authorize(verified, catalog.ActionGlueCreateTable, "*") {
		s.writeGlueError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform glue:CreateTable.", readOnly, eventID, verified)
		return
	}
	_, err := s.store.CreateGlueTable(verified.AccountID, in)
	if errors.Is(err, store.ErrGlueNotFound) {
		s.writeGlueError(w, r, body, requestID, http.StatusBadRequest, "EntityNotFoundException",
			"Database not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrGlueAlreadyExists) {
		s.writeGlueError(w, r, body, requestID, http.StatusBadRequest, "AlreadyExistsException",
			"Table already exists.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeGlueError(w, r, body, requestID, http.StatusInternalServerError, "InternalServiceException",
			"Unable to create table.", readOnly, eventID, verified)
		return
	}
	payload, _ := gluesvc.CreateTableJSON()
	s.writeGlueOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, glueEventSource, "CreateTable", readOnly)
}

func (s *Server) glueGetTable(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	dbName, _ := params["DatabaseName"].(string)
	name, _ := params["Name"].(string)
	if !s.authorize(verified, catalog.ActionGlueGetTable, "*") {
		s.writeGlueError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform glue:GetTable.", readOnly, eventID, verified)
		return
	}
	t, err := s.store.GetGlueTable(verified.AccountID, dbName, name)
	if errors.Is(err, store.ErrGlueNotFound) {
		s.writeGlueError(w, r, body, requestID, http.StatusBadRequest, "EntityNotFoundException",
			"Table not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeGlueError(w, r, body, requestID, http.StatusInternalServerError, "InternalServiceException",
			"Unable to get table.", readOnly, eventID, verified)
		return
	}
	payload, _ := gluesvc.GetTableJSON(t)
	s.writeGlueOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, glueEventSource, "GetTable", readOnly)
}

func (s *Server) glueGetTables(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	dbName, _ := params["DatabaseName"].(string)
	if !s.authorize(verified, catalog.ActionGlueGetTables, "*") {
		s.writeGlueError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform glue:GetTables.", readOnly, eventID, verified)
		return
	}
	tables, err := s.store.GetGlueTables(verified.AccountID, dbName)
	if errors.Is(err, store.ErrGlueNotFound) {
		s.writeGlueError(w, r, body, requestID, http.StatusBadRequest, "EntityNotFoundException",
			"Database not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeGlueError(w, r, body, requestID, http.StatusInternalServerError, "InternalServiceException",
			"Unable to get tables.", readOnly, eventID, verified)
		return
	}
	payload, _ := gluesvc.GetTablesJSON(tables)
	s.writeGlueOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, glueEventSource, "GetTables", readOnly)
}

func (s *Server) glueDeleteTable(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	dbName, _ := params["DatabaseName"].(string)
	name, _ := params["Name"].(string)
	if !s.authorize(verified, catalog.ActionGlueDeleteTable, "*") {
		s.writeGlueError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform glue:DeleteTable.", readOnly, eventID, verified)
		return
	}
	err := s.store.DeleteGlueTable(verified.AccountID, dbName, name)
	if errors.Is(err, store.ErrGlueNotFound) {
		s.writeGlueError(w, r, body, requestID, http.StatusBadRequest, "EntityNotFoundException",
			"Table not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeGlueError(w, r, body, requestID, http.StatusInternalServerError, "InternalServiceException",
			"Unable to delete table.", readOnly, eventID, verified)
		return
	}
	payload, _ := gluesvc.DeleteTableJSON()
	s.writeGlueOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, glueEventSource, "DeleteTable", readOnly)
}

func (s *Server) writeGlueOK(w http.ResponseWriter, payload []byte) {
	w.Header().Set("Content-Type", glueJSONContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeGlueError(
	w http.ResponseWriter, r *http.Request, body []byte, requestID string, status int, code, message string,
	readOnly bool, eventID string, verified *authn.Verified,
) {
	w.Header().Set("Content-Type", glueJSONContentType)
	w.Header().Set("x-amzn-ErrorType", code)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"__type": code, "message": message})
	_ = body
	s.writeSuccessAudit(r, requestID, eventID, verified, glueEventSource, code, readOnly)
}
