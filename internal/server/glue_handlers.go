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

func (s *Server) glueCrawlerARN(verified *authn.Verified, name string) string {
	region := verified.Region
	if region == "" {
		region = store.DefaultGlueRegion
	}
	return "arn:aws:glue:" + region + ":" + verified.AccountID + ":crawler/" + name
}

func (s *Server) checkGluePassRole(verified *authn.Verified, roleARN, sourceARN string) error {
	accountID, roleName, ok := sts.ParseRoleARN(roleARN)
	if !ok {
		return errors.New("Role must be a valid IAM role ARN")
	}
	if accountID != verified.AccountID {
		return errors.New("Role must be in the same account")
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
		return errors.New("not authorized to pass role to Glue")
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
		ServicePrincipal: "glue.amazonaws.com",
		SourceArn:        sourceARN,
	})
	if decision != authz.Allow {
		return errors.New("not authorized to pass role to Glue")
	}
	return nil
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
