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

	actionGlueCreateCrawler  = "glue:CreateCrawler"
	actionGlueStartCrawler   = "glue:StartCrawler"
	actionGlueGetCrawler     = "glue:GetCrawler"
	actionGlueDeleteCrawler  = "glue:DeleteCrawler"
	actionGlueListCrawlers   = "glue:ListCrawlers"
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
	case actionGlueCreateCrawler:
		s.glueCreateCrawler(w, r, body, requestID, eventID, verified, readOnly, params)
	case actionGlueStartCrawler:
		s.glueStartCrawler(w, r, body, requestID, eventID, verified, readOnly, params)
	case actionGlueGetCrawler:
		s.glueGetCrawler(w, r, body, requestID, eventID, verified, readOnly, params)
	case actionGlueDeleteCrawler:
		s.glueDeleteCrawler(w, r, body, requestID, eventID, verified, readOnly, params)
	case actionGlueListCrawlers:
		s.glueListCrawlers(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionGlueCreateRegistry:
		s.glueCreateRegistry(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionGlueGetRegistry:
		s.glueGetRegistry(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionGlueListRegistries:
		s.glueListRegistries(w, r, body, requestID, eventID, verified, readOnly)
	case catalog.ActionGlueDeleteRegistry:
		s.glueDeleteRegistry(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionGlueCreateSchema:
		s.glueCreateSchema(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionGlueGetSchema:
		s.glueGetSchema(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionGlueListSchemas:
		s.glueListSchemas(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionGlueDeleteSchema:
		s.glueDeleteSchema(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionGlueRegisterSchemaVersion:
		s.glueRegisterSchemaVersion(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionGlueGetSchemaVersion:
		s.glueGetSchemaVersion(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionGlueListSchemaVersions:
		s.glueListSchemaVersions(w, r, body, requestID, eventID, verified, readOnly, params)
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
	case "CreateCrawler":
		return actionGlueCreateCrawler
	case "StartCrawler":
		return actionGlueStartCrawler
	case "GetCrawler":
		return actionGlueGetCrawler
	case "DeleteCrawler":
		return actionGlueDeleteCrawler
	case "ListCrawlers":
		return actionGlueListCrawlers
	case "CreateRegistry":
		return catalog.ActionGlueCreateRegistry
	case "GetRegistry":
		return catalog.ActionGlueGetRegistry
	case "ListRegistries":
		return catalog.ActionGlueListRegistries
	case "DeleteRegistry":
		return catalog.ActionGlueDeleteRegistry
	case "CreateSchema":
		return catalog.ActionGlueCreateSchema
	case "GetSchema":
		return catalog.ActionGlueGetSchema
	case "ListSchemas":
		return catalog.ActionGlueListSchemas
	case "DeleteSchema":
		return catalog.ActionGlueDeleteSchema
	case "RegisterSchemaVersion":
		return catalog.ActionGlueRegisterSchemaVersion
	case "GetSchemaVersion":
		return catalog.ActionGlueGetSchemaVersion
	case "ListSchemaVersions":
		return catalog.ActionGlueListSchemaVersions
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
		Parameters:    map[string]string{},
	}
	if paramsMap, ok := tblIn["Parameters"].(map[string]any); ok {
		for k, v := range paramsMap {
			if vs, ok := v.(string); ok {
				in.Parameters[k] = vs
			}
		}
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
		in.SchemaReference = parseGlueSchemaReference(sd["SchemaReference"])
	}
	if in.SchemaReference.SchemaVersionID == "" && in.SchemaReference.RegistryName == "" {
		if ref, ok := store.SchemaReferenceFromTableParams(in.Parameters); ok {
			in.SchemaReference = ref
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

func (s *Server) glueCrawlerARN(verified *authn.Verified, name string) string {
	region := verified.Region
	if region == "" {
		region = store.DefaultGlueRegion
	}
	return "arn:aws:glue:" + region + ":" + verified.AccountID + ":crawler/" + name
}

func (s *Server) checkGluePassRole(verified *authn.Verified, roleARN, sourceARN string) error {
	return s.checkServicePassRole(verified, roleARN, sourceARN, glueEventSource, "Glue", passRoleMsgs{
		InvalidARN:   "Role must be a valid IAM role ARN",
		WrongAccount: "Role must be in the same account",
	})
}

func parseGlueS3Targets(params map[string]any) []store.GlueS3Target {
	targetsIn, _ := params["Targets"].(map[string]any)
	raw, _ := targetsIn["S3Targets"].([]any)
	var out []store.GlueS3Target
	for _, item := range raw {
		m, _ := item.(map[string]any)
		path, _ := m["Path"].(string)
		if strings.TrimSpace(path) == "" {
			continue
		}
		out = append(out, store.GlueS3Target{Path: path})
	}
	return out
}

func glueCrawlerJSON(cr store.GlueCrawler) map[string]any {
	s3Targets := make([]map[string]any, 0, len(cr.Targets))
	for _, t := range cr.Targets {
		s3Targets = append(s3Targets, map[string]any{"Path": t.Path})
	}
	return map[string]any{
		"Name":         cr.Name,
		"Role":         cr.Role,
		"DatabaseName": cr.DatabaseName,
		"State":        cr.State,
		"Targets": map[string]any{
			"S3Targets": s3Targets,
		},
		"CreationTime": float64(cr.CreatedAt) / 1000.0,
		"LastUpdated":  float64(cr.UpdatedAt) / 1000.0,
	}
}

func (s *Server) glueCreateCrawler(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name, _ := params["Name"].(string)
	role, _ := params["Role"].(string)
	dbName, _ := params["DatabaseName"].(string)
	targets := parseGlueS3Targets(params)
	if strings.TrimSpace(name) == "" {
		s.writeGlueError(w, r, body, requestID, http.StatusBadRequest, "InvalidInputException",
			"Name is required.", readOnly, eventID, verified)
		return
	}
	if !s.authorize(verified, actionGlueCreateCrawler, "*") {
		s.writeGlueError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform glue:CreateCrawler.", readOnly, eventID, verified)
		return
	}
	if strings.TrimSpace(role) != "" {
		if err := s.checkGluePassRole(verified, role, s.glueCrawlerARN(verified, name)); err != nil {
			s.writeGlueError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
				err.Error(), readOnly, eventID, verified)
			return
		}
	}
	_, err := s.store.CreateGlueCrawler(verified.AccountID, store.GlueCrawlerCreate{
		Name: name, Role: role, DatabaseName: dbName, Targets: targets,
	})
	if errors.Is(err, store.ErrGlueAlreadyExists) {
		s.writeGlueError(w, r, body, requestID, http.StatusBadRequest, "AlreadyExistsException",
			"Crawler already exists.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrGlueNotFound) {
		s.writeGlueError(w, r, body, requestID, http.StatusBadRequest, "EntityNotFoundException",
			"Database not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrGlueBadRequest) {
		s.writeGlueError(w, r, body, requestID, http.StatusBadRequest, "InvalidInputException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeGlueError(w, r, body, requestID, http.StatusInternalServerError, "InternalServiceException",
			"Unable to create crawler.", readOnly, eventID, verified)
		return
	}
	s.writeGlueOK(w, []byte(`{}`))
	s.writeSuccessAudit(r, requestID, eventID, verified, glueEventSource, "CreateCrawler", readOnly)
}

func (s *Server) glueStartCrawler(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name, _ := params["Name"].(string)
	if strings.TrimSpace(name) == "" {
		s.writeGlueError(w, r, body, requestID, http.StatusBadRequest, "InvalidInputException",
			"Name is required.", readOnly, eventID, verified)
		return
	}
	if !s.authorize(verified, actionGlueStartCrawler, "*") {
		s.writeGlueError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform glue:StartCrawler.", readOnly, eventID, verified)
		return
	}
	_, err := s.store.StartGlueCrawler(verified.AccountID, name)
	if errors.Is(err, store.ErrGlueNotFound) {
		s.writeGlueError(w, r, body, requestID, http.StatusBadRequest, "EntityNotFoundException",
			"Crawler not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrGlueBadRequest) {
		s.writeGlueError(w, r, body, requestID, http.StatusBadRequest, "InvalidInputException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeGlueError(w, r, body, requestID, http.StatusInternalServerError, "InternalServiceException",
			"Unable to start crawler.", readOnly, eventID, verified)
		return
	}
	s.writeGlueOK(w, []byte(`{}`))
	s.writeSuccessAudit(r, requestID, eventID, verified, glueEventSource, "StartCrawler", readOnly)
}

func (s *Server) glueGetCrawler(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name, _ := params["Name"].(string)
	if !s.authorize(verified, actionGlueGetCrawler, "*") {
		s.writeGlueError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform glue:GetCrawler.", readOnly, eventID, verified)
		return
	}
	cr, err := s.store.GetGlueCrawler(verified.AccountID, name)
	if errors.Is(err, store.ErrGlueNotFound) {
		s.writeGlueError(w, r, body, requestID, http.StatusBadRequest, "EntityNotFoundException",
			"Crawler not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeGlueError(w, r, body, requestID, http.StatusInternalServerError, "InternalServiceException",
			"Unable to get crawler.", readOnly, eventID, verified)
		return
	}
	payload, _ := json.Marshal(map[string]any{"Crawler": glueCrawlerJSON(cr)})
	s.writeGlueOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, glueEventSource, "GetCrawler", readOnly)
}

func (s *Server) glueDeleteCrawler(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name, _ := params["Name"].(string)
	if !s.authorize(verified, actionGlueDeleteCrawler, "*") {
		s.writeGlueError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform glue:DeleteCrawler.", readOnly, eventID, verified)
		return
	}
	err := s.store.DeleteGlueCrawler(verified.AccountID, name)
	if errors.Is(err, store.ErrGlueNotFound) {
		s.writeGlueError(w, r, body, requestID, http.StatusBadRequest, "EntityNotFoundException",
			"Crawler not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeGlueError(w, r, body, requestID, http.StatusInternalServerError, "InternalServiceException",
			"Unable to delete crawler.", readOnly, eventID, verified)
		return
	}
	s.writeGlueOK(w, []byte(`{}`))
	s.writeSuccessAudit(r, requestID, eventID, verified, glueEventSource, "DeleteCrawler", readOnly)
}

func (s *Server) glueListCrawlers(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool,
) {
	if !s.authorize(verified, actionGlueListCrawlers, "*") {
		s.writeGlueError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform glue:ListCrawlers.", readOnly, eventID, verified)
		return
	}
	crawlers, err := s.store.ListGlueCrawlers(verified.AccountID)
	if err != nil {
		s.writeGlueError(w, r, body, requestID, http.StatusInternalServerError, "InternalServiceException",
			"Unable to list crawlers.", readOnly, eventID, verified)
		return
	}
	items := make([]map[string]any, 0, len(crawlers))
	for _, cr := range crawlers {
		items = append(items, glueCrawlerJSON(cr))
	}
	payload, _ := json.Marshal(map[string]any{"Crawlers": items})
	s.writeGlueOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, glueEventSource, "ListCrawlers", readOnly)
}

func parseGlueSchemaReference(raw any) store.GlueSchemaReference {
	m, ok := raw.(map[string]any)
	if !ok || m == nil {
		return store.GlueSchemaReference{}
	}
	ref := store.GlueSchemaReference{}
	ref.SchemaVersionID, _ = m["SchemaVersionId"].(string)
	switch v := m["SchemaVersionNumber"].(type) {
	case float64:
		ref.SchemaVersionNumber = int64(v)
	case int64:
		ref.SchemaVersionNumber = v
	case json.Number:
		n, _ := v.Int64()
		ref.SchemaVersionNumber = n
	}
	if schemaID, ok := m["SchemaId"].(map[string]any); ok {
		ref.RegistryName, _ = schemaID["RegistryName"].(string)
		ref.SchemaName, _ = schemaID["SchemaName"].(string)
	}
	if ref.RegistryName == "" {
		ref.RegistryName, _ = m["RegistryName"].(string)
	}
	if ref.SchemaName == "" {
		ref.SchemaName, _ = m["SchemaName"].(string)
	}
	return ref
}

func glueRegistryNameFromParams(params map[string]any) string {
	if id, ok := params["RegistryId"].(map[string]any); ok {
		if name, _ := id["RegistryName"].(string); strings.TrimSpace(name) != "" {
			return name
		}
	}
	name, _ := params["RegistryName"].(string)
	return name
}

func glueSchemaIDFromParams(params map[string]any) (registryName, schemaName string) {
	if id, ok := params["SchemaId"].(map[string]any); ok {
		registryName, _ = id["RegistryName"].(string)
		schemaName, _ = id["SchemaName"].(string)
		return registryName, schemaName
	}
	registryName, _ = params["RegistryName"].(string)
	schemaName, _ = params["SchemaName"].(string)
	return registryName, schemaName
}

func glueRegistryJSON(r store.GlueRegistry) map[string]any {
	return map[string]any{
		"RegistryName": r.Name,
		"Description":  r.Description,
		"RegistryArn":  r.RegistryARN,
		"CreatedTime":  float64(r.CreatedAt) / 1000.0,
		"UpdatedTime":  float64(r.UpdatedAt) / 1000.0,
	}
}

func glueSchemaJSON(sc store.GlueSchema) map[string]any {
	return map[string]any{
		"RegistryName":          sc.RegistryName,
		"SchemaName":            sc.SchemaName,
		"Description":           sc.Description,
		"DataFormat":            sc.DataFormat,
		"Compatibility":         sc.Compatibility,
		"SchemaArn":             sc.SchemaARN,
		"SchemaStatus":          sc.SchemaStatus,
		"LatestSchemaVersion":   sc.LatestSchemaVersion,
		"NextSchemaVersion":     sc.NextSchemaVersion,
		"CreatedTime":           float64(sc.CreatedAt) / 1000.0,
		"UpdatedTime":           float64(sc.UpdatedAt) / 1000.0,
	}
}

func (s *Server) glueCreateRegistry(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name, _ := params["RegistryName"].(string)
	desc, _ := params["Description"].(string)
	if strings.TrimSpace(name) == "" {
		s.writeGlueError(w, r, body, requestID, http.StatusBadRequest, "InvalidInputException",
			"RegistryName is required.", readOnly, eventID, verified)
		return
	}
	if !s.authorize(verified, catalog.ActionGlueCreateRegistry, "*") {
		s.writeGlueError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform glue:CreateRegistry.", readOnly, eventID, verified)
		return
	}
	reg, err := s.store.CreateGlueRegistry(verified.AccountID, verified.Region, name, desc)
	if errors.Is(err, store.ErrGlueAlreadyExists) {
		s.writeGlueError(w, r, body, requestID, http.StatusBadRequest, "AlreadyExistsException",
			"Registry already exists.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrGlueBadRequest) {
		s.writeGlueError(w, r, body, requestID, http.StatusBadRequest, "InvalidInputException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeGlueError(w, r, body, requestID, http.StatusInternalServerError, "InternalServiceException",
			"Unable to create registry.", readOnly, eventID, verified)
		return
	}
	payload, _ := json.Marshal(glueRegistryJSON(reg))
	s.writeGlueOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, glueEventSource, "CreateRegistry", readOnly)
}

func (s *Server) glueGetRegistry(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name := glueRegistryNameFromParams(params)
	if !s.authorize(verified, catalog.ActionGlueGetRegistry, "*") {
		s.writeGlueError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform glue:GetRegistry.", readOnly, eventID, verified)
		return
	}
	reg, err := s.store.GetGlueRegistry(verified.AccountID, name)
	if errors.Is(err, store.ErrGlueNotFound) {
		s.writeGlueError(w, r, body, requestID, http.StatusBadRequest, "EntityNotFoundException",
			"Registry not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeGlueError(w, r, body, requestID, http.StatusInternalServerError, "InternalServiceException",
			"Unable to get registry.", readOnly, eventID, verified)
		return
	}
	payload, _ := json.Marshal(glueRegistryJSON(reg))
	s.writeGlueOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, glueEventSource, "GetRegistry", readOnly)
}

func (s *Server) glueListRegistries(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool,
) {
	if !s.authorize(verified, catalog.ActionGlueListRegistries, "*") {
		s.writeGlueError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform glue:ListRegistries.", readOnly, eventID, verified)
		return
	}
	regs, err := s.store.ListGlueRegistries(verified.AccountID)
	if err != nil {
		s.writeGlueError(w, r, body, requestID, http.StatusInternalServerError, "InternalServiceException",
			"Unable to list registries.", readOnly, eventID, verified)
		return
	}
	items := make([]map[string]any, 0, len(regs))
	for _, reg := range regs {
		items = append(items, glueRegistryJSON(reg))
	}
	payload, _ := json.Marshal(map[string]any{"Registries": items})
	s.writeGlueOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, glueEventSource, "ListRegistries", readOnly)
}

func (s *Server) glueDeleteRegistry(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name := glueRegistryNameFromParams(params)
	if !s.authorize(verified, catalog.ActionGlueDeleteRegistry, "*") {
		s.writeGlueError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform glue:DeleteRegistry.", readOnly, eventID, verified)
		return
	}
	err := s.store.DeleteGlueRegistry(verified.AccountID, name)
	if errors.Is(err, store.ErrGlueNotFound) {
		s.writeGlueError(w, r, body, requestID, http.StatusBadRequest, "EntityNotFoundException",
			"Registry not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeGlueError(w, r, body, requestID, http.StatusInternalServerError, "InternalServiceException",
			"Unable to delete registry.", readOnly, eventID, verified)
		return
	}
	payload, _ := json.Marshal(map[string]any{"RegistryName": name})
	s.writeGlueOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, glueEventSource, "DeleteRegistry", readOnly)
}

func (s *Server) glueCreateSchema(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	registryName := glueRegistryNameFromParams(params)
	schemaName, _ := params["SchemaName"].(string)
	dataFormat, _ := params["DataFormat"].(string)
	compatibility, _ := params["Compatibility"].(string)
	description, _ := params["Description"].(string)
	definition, _ := params["SchemaDefinition"].(string)
	if strings.TrimSpace(schemaName) == "" {
		s.writeGlueError(w, r, body, requestID, http.StatusBadRequest, "InvalidInputException",
			"SchemaName is required.", readOnly, eventID, verified)
		return
	}
	if !s.authorize(verified, catalog.ActionGlueCreateSchema, "*") {
		s.writeGlueError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform glue:CreateSchema.", readOnly, eventID, verified)
		return
	}
	res, err := s.store.CreateGlueSchema(
		verified.AccountID, verified.Region, registryName, schemaName, dataFormat, compatibility, description, definition,
	)
	if errors.Is(err, store.ErrGlueAlreadyExists) {
		s.writeGlueError(w, r, body, requestID, http.StatusBadRequest, "AlreadyExistsException",
			"Schema already exists.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrGlueBadRequest) {
		s.writeGlueError(w, r, body, requestID, http.StatusBadRequest, "InvalidInputException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeGlueError(w, r, body, requestID, http.StatusInternalServerError, "InternalServiceException",
			"Unable to create schema.", readOnly, eventID, verified)
		return
	}
	out := glueSchemaJSON(res.Schema)
	out["SchemaVersionId"] = res.SchemaVersionID
	out["SchemaVersionStatus"] = res.VersionStatus
	payload, _ := json.Marshal(out)
	s.writeGlueOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, glueEventSource, "CreateSchema", readOnly)
}

func (s *Server) glueGetSchema(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	registryName, schemaName := glueSchemaIDFromParams(params)
	if !s.authorize(verified, catalog.ActionGlueGetSchema, "*") {
		s.writeGlueError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform glue:GetSchema.", readOnly, eventID, verified)
		return
	}
	sc, err := s.store.GetGlueSchema(verified.AccountID, registryName, schemaName)
	if errors.Is(err, store.ErrGlueNotFound) {
		s.writeGlueError(w, r, body, requestID, http.StatusBadRequest, "EntityNotFoundException",
			"Schema not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeGlueError(w, r, body, requestID, http.StatusInternalServerError, "InternalServiceException",
			"Unable to get schema.", readOnly, eventID, verified)
		return
	}
	payload, _ := json.Marshal(glueSchemaJSON(sc))
	s.writeGlueOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, glueEventSource, "GetSchema", readOnly)
}

func (s *Server) glueListSchemas(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionGlueListSchemas, "*") {
		s.writeGlueError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform glue:ListSchemas.", readOnly, eventID, verified)
		return
	}
	schemas, err := s.store.ListGlueSchemas(verified.AccountID, glueRegistryNameFromParams(params))
	if err != nil {
		s.writeGlueError(w, r, body, requestID, http.StatusInternalServerError, "InternalServiceException",
			"Unable to list schemas.", readOnly, eventID, verified)
		return
	}
	items := make([]map[string]any, 0, len(schemas))
	for _, sc := range schemas {
		items = append(items, glueSchemaJSON(sc))
	}
	payload, _ := json.Marshal(map[string]any{"Schemas": items})
	s.writeGlueOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, glueEventSource, "ListSchemas", readOnly)
}

func (s *Server) glueDeleteSchema(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	registryName, schemaName := glueSchemaIDFromParams(params)
	if !s.authorize(verified, catalog.ActionGlueDeleteSchema, "*") {
		s.writeGlueError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform glue:DeleteSchema.", readOnly, eventID, verified)
		return
	}
	err := s.store.DeleteGlueSchema(verified.AccountID, registryName, schemaName)
	if errors.Is(err, store.ErrGlueNotFound) {
		s.writeGlueError(w, r, body, requestID, http.StatusBadRequest, "EntityNotFoundException",
			"Schema not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeGlueError(w, r, body, requestID, http.StatusInternalServerError, "InternalServiceException",
			"Unable to delete schema.", readOnly, eventID, verified)
		return
	}
	payload, _ := json.Marshal(map[string]any{
		"SchemaName":   schemaName,
		"RegistryName": registryName,
	})
	s.writeGlueOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, glueEventSource, "DeleteSchema", readOnly)
}

func (s *Server) glueRegisterSchemaVersion(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	registryName, schemaName := glueSchemaIDFromParams(params)
	definition, _ := params["SchemaDefinition"].(string)
	if !s.authorize(verified, catalog.ActionGlueRegisterSchemaVersion, "*") {
		s.writeGlueError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform glue:RegisterSchemaVersion.", readOnly, eventID, verified)
		return
	}
	ver, err := s.store.RegisterGlueSchemaVersion(verified.AccountID, registryName, schemaName, definition)
	if errors.Is(err, store.ErrGlueNotFound) {
		s.writeGlueError(w, r, body, requestID, http.StatusBadRequest, "EntityNotFoundException",
			"Schema not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrGlueBadRequest) {
		s.writeGlueError(w, r, body, requestID, http.StatusBadRequest, "InvalidInputException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeGlueError(w, r, body, requestID, http.StatusInternalServerError, "InternalServiceException",
			"Unable to register schema version.", readOnly, eventID, verified)
		return
	}
	payload, _ := json.Marshal(map[string]any{
		"SchemaVersionId": ver.SchemaVersionID,
		"VersionNumber":   ver.VersionNumber,
		"Status":          ver.Status,
	})
	s.writeGlueOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, glueEventSource, "RegisterSchemaVersion", readOnly)
}

func (s *Server) glueGetSchemaVersion(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionGlueGetSchemaVersion, "*") {
		s.writeGlueError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform glue:GetSchemaVersion.", readOnly, eventID, verified)
		return
	}
	var (
		ver store.GlueSchemaVersion
		err error
	)
	if id, _ := params["SchemaVersionId"].(string); strings.TrimSpace(id) != "" {
		ver, err = s.store.GetGlueSchemaVersionByID(verified.AccountID, id)
	} else {
		registryName, schemaName := glueSchemaIDFromParams(params)
		var versionNumber int64
		switch v := params["SchemaVersionNumber"].(type) {
		case float64:
			versionNumber = int64(v)
		case int64:
			versionNumber = v
		case map[string]any:
			if latest, _ := v["LatestVersion"].(bool); latest {
				sc, getErr := s.store.GetGlueSchema(verified.AccountID, registryName, schemaName)
				if getErr != nil {
					err = getErr
				} else {
					versionNumber = sc.LatestSchemaVersion
				}
			} else if n, ok := v["VersionNumber"].(float64); ok {
				versionNumber = int64(n)
			}
		}
		if err == nil {
			ver, err = s.store.GetGlueSchemaVersionByNumber(verified.AccountID, registryName, schemaName, versionNumber)
		}
	}
	if errors.Is(err, store.ErrGlueNotFound) {
		s.writeGlueError(w, r, body, requestID, http.StatusBadRequest, "EntityNotFoundException",
			"Schema version not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrGlueBadRequest) {
		s.writeGlueError(w, r, body, requestID, http.StatusBadRequest, "InvalidInputException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeGlueError(w, r, body, requestID, http.StatusInternalServerError, "InternalServiceException",
			"Unable to get schema version.", readOnly, eventID, verified)
		return
	}
	payload, _ := json.Marshal(map[string]any{
		"SchemaVersionId": ver.SchemaVersionID,
		"VersionNumber":   ver.VersionNumber,
		"Status":          ver.Status,
		"DataFormat":      ver.DataFormat,
		"SchemaDefinition": ver.Definition,
		"CreatedTime":     float64(ver.CreatedAt) / 1000.0,
	})
	s.writeGlueOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, glueEventSource, "GetSchemaVersion", readOnly)
}

func (s *Server) glueListSchemaVersions(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	registryName, schemaName := glueSchemaIDFromParams(params)
	if !s.authorize(verified, catalog.ActionGlueListSchemaVersions, "*") {
		s.writeGlueError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform glue:ListSchemaVersions.", readOnly, eventID, verified)
		return
	}
	versions, err := s.store.ListGlueSchemaVersions(verified.AccountID, registryName, schemaName)
	if errors.Is(err, store.ErrGlueNotFound) {
		s.writeGlueError(w, r, body, requestID, http.StatusBadRequest, "EntityNotFoundException",
			"Schema not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeGlueError(w, r, body, requestID, http.StatusInternalServerError, "InternalServiceException",
			"Unable to list schema versions.", readOnly, eventID, verified)
		return
	}
	items := make([]map[string]any, 0, len(versions))
	for _, ver := range versions {
		items = append(items, map[string]any{
			"SchemaVersionId": ver.SchemaVersionID,
			"VersionNumber":   ver.VersionNumber,
			"Status":          ver.Status,
			"CreatedTime":     float64(ver.CreatedAt) / 1000.0,
		})
	}
	payload, _ := json.Marshal(map[string]any{"Schemas": items})
	s.writeGlueOK(w, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, glueEventSource, "ListSchemaVersions", readOnly)
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
