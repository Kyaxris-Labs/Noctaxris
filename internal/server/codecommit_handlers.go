package server

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	ccsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/codecommit"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	codecommitJSONContentType = "application/x-amz-json-1.1"
	codecommitEventSource     = "codecommit.amazonaws.com"
)

func (s *Server) handleCodeCommit(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = codecommitAction(action)

	switch action {
	case catalog.ActionCodeCommitCreateRepository:
		s.codecommitCreateRepository(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCodeCommitGetRepository:
		s.codecommitGetRepository(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCodeCommitListRepositories:
		s.codecommitListRepositories(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCodeCommitDeleteRepository:
		s.codecommitDeleteRepository(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCodeCommitPutFile:
		s.codecommitPutFile(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCodeCommitGetFile:
		s.codecommitGetFile(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionCodeCommitGetFolder:
		s.codecommitGetFolder(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeCodeCommitError(w, r, body, requestID, http.StatusNotImplemented, "InternalFailure",
			"This CodeCommit action is not implemented.", readOnly, eventID, verified)
	}
}

func codecommitAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "CreateRepository":
		return catalog.ActionCodeCommitCreateRepository
	case "GetRepository":
		return catalog.ActionCodeCommitGetRepository
	case "ListRepositories":
		return catalog.ActionCodeCommitListRepositories
	case "DeleteRepository":
		return catalog.ActionCodeCommitDeleteRepository
	case "PutFile":
		return catalog.ActionCodeCommitPutFile
	case "GetFile":
		return catalog.ActionCodeCommitGetFile
	case "GetFolder":
		return catalog.ActionCodeCommitGetFolder
	default:
		return action
	}
}

func (s *Server) codecommitRegion(verified *authn.Verified) string {
	if verified.Region != "" {
		return verified.Region
	}
	return store.DefaultCodeCommitRegion
}

func (s *Server) codecommitCreateRepository(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name := stringParam(params["repositoryName"])
	resource := "*"
	if name != "" {
		resource = store.CodeCommitRepositoryARN(s.codecommitRegion(verified), verified.AccountID, name)
	}
	if !s.authorize(verified, catalog.ActionCodeCommitCreateRepository, resource) {
		s.writeCodeCommitError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform codecommit:CreateRepository.", readOnly, eventID, verified)
		return
	}
	repo, err := s.store.CreateCodeCommitRepository(
		verified.AccountID, s.codecommitRegion(verified), name, stringParam(params["repositoryDescription"]),
	)
	if errors.Is(err, store.ErrCodeCommitRepoExists) {
		s.writeCodeCommitError(w, r, body, requestID, http.StatusBadRequest, "RepositoryNameExistsException",
			"A repository with this name already exists.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCodeCommitInvalidInput) {
		s.writeCodeCommitError(w, r, body, requestID, http.StatusBadRequest, "InvalidRepositoryNameException",
			"The repository name is not valid.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCodeCommitError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create repository.", readOnly, eventID, verified)
		return
	}
	payload, err := ccsvc.CreateRepositoryJSON(repo)
	if err != nil {
		s.writeCodeCommitError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeCodeCommitOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, codecommitEventSource, "CreateRepository", readOnly)
}

func (s *Server) codecommitGetRepository(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name := stringParam(params["repositoryName"])
	resource := store.CodeCommitRepositoryARN(s.codecommitRegion(verified), verified.AccountID, name)
	if name == "" {
		resource = "*"
	}
	if !s.authorize(verified, catalog.ActionCodeCommitGetRepository, resource) {
		s.writeCodeCommitError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform codecommit:GetRepository.", readOnly, eventID, verified)
		return
	}
	repo, err := s.store.GetCodeCommitRepository(verified.AccountID, s.codecommitRegion(verified), name)
	if errors.Is(err, store.ErrCodeCommitRepoNotFound) {
		s.writeCodeCommitError(w, r, body, requestID, http.StatusBadRequest, "RepositoryDoesNotExistException",
			"The specified repository does not exist.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCodeCommitInvalidInput) {
		s.writeCodeCommitError(w, r, body, requestID, http.StatusBadRequest, "InvalidRepositoryNameException",
			"The repository name is not valid.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCodeCommitError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to get repository.", readOnly, eventID, verified)
		return
	}
	payload, err := ccsvc.GetRepositoryJSON(repo)
	if err != nil {
		s.writeCodeCommitError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeCodeCommitOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, codecommitEventSource, "GetRepository", readOnly)
}

func (s *Server) codecommitListRepositories(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	_ = params
	if !s.authorize(verified, catalog.ActionCodeCommitListRepositories, "*") {
		s.writeCodeCommitError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform codecommit:ListRepositories.", readOnly, eventID, verified)
		return
	}
	repos, err := s.store.ListCodeCommitRepositories(verified.AccountID)
	if err != nil {
		s.writeCodeCommitError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list repositories.", readOnly, eventID, verified)
		return
	}
	payload, err := ccsvc.ListRepositoriesJSON(repos)
	if err != nil {
		s.writeCodeCommitError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeCodeCommitOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, codecommitEventSource, "ListRepositories", readOnly)
}

func (s *Server) codecommitDeleteRepository(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name := stringParam(params["repositoryName"])
	resource := store.CodeCommitRepositoryARN(s.codecommitRegion(verified), verified.AccountID, name)
	if name == "" {
		resource = "*"
	}
	if !s.authorize(verified, catalog.ActionCodeCommitDeleteRepository, resource) {
		s.writeCodeCommitError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform codecommit:DeleteRepository.", readOnly, eventID, verified)
		return
	}
	id, err := s.store.DeleteCodeCommitRepository(verified.AccountID, name)
	if errors.Is(err, store.ErrCodeCommitInvalidInput) {
		s.writeCodeCommitError(w, r, body, requestID, http.StatusBadRequest, "InvalidRepositoryNameException",
			"The repository name is not valid.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCodeCommitError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete repository.", readOnly, eventID, verified)
		return
	}
	payload, err := ccsvc.DeleteRepositoryJSON(id)
	if err != nil {
		s.writeCodeCommitError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeCodeCommitOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, codecommitEventSource, "DeleteRepository", readOnly)
}

func decodeCodeCommitFileContent(v any) ([]byte, error) {
	switch t := v.(type) {
	case string:
		if t == "" {
			return nil, store.ErrCodeCommitFileContent
		}
		raw, err := base64.StdEncoding.DecodeString(t)
		if err != nil {
			// Lab accepts plain UTF-8 when callers skip base64 (CLI JSON helpers).
			return []byte(t), nil
		}
		if len(raw) == 0 {
			return nil, store.ErrCodeCommitFileContent
		}
		return raw, nil
	case []byte:
		if len(t) == 0 {
			return nil, store.ErrCodeCommitFileContent
		}
		return t, nil
	default:
		return nil, store.ErrCodeCommitFileContent
	}
}

func (s *Server) codecommitPutFile(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name := stringParam(params["repositoryName"])
	resource := store.CodeCommitRepositoryARN(s.codecommitRegion(verified), verified.AccountID, name)
	if name == "" {
		resource = "*"
	}
	if !s.authorize(verified, catalog.ActionCodeCommitPutFile, resource) {
		s.writeCodeCommitError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform codecommit:PutFile.", readOnly, eventID, verified)
		return
	}
	content, err := decodeCodeCommitFileContent(params["fileContent"])
	if err != nil {
		s.writeCodeCommitError(w, r, body, requestID, http.StatusBadRequest, "FileContentRequiredException",
			"File content is required.", readOnly, eventID, verified)
		return
	}
	result, err := s.store.PutCodeCommitFile(
		verified.AccountID,
		s.codecommitRegion(verified),
		name,
		stringParam(params["branchName"]),
		stringParam(params["filePath"]),
		content,
		stringParam(params["parentCommitId"]),
	)
	if errors.Is(err, store.ErrCodeCommitRepoNotFound) {
		s.writeCodeCommitError(w, r, body, requestID, http.StatusBadRequest, "RepositoryDoesNotExistException",
			"The specified repository does not exist.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCodeCommitParentOutdated) {
		s.writeCodeCommitError(w, r, body, requestID, http.StatusBadRequest, "ParentCommitIdOutdatedException",
			"The parent commit ID is outdated.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCodeCommitPathEscape) {
		s.writeCodeCommitError(w, r, body, requestID, http.StatusBadRequest, "InvalidPathException",
			"The specified path is not valid.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCodeCommitFileTooLarge) {
		s.writeCodeCommitError(w, r, body, requestID, http.StatusBadRequest, "FileContentSizeLimitExceededException",
			"The file is too large.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCodeCommitBranchMissing) {
		s.writeCodeCommitError(w, r, body, requestID, http.StatusBadRequest, "BranchDoesNotExistException",
			"The specified branch does not exist.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCodeCommitFileContent) {
		s.writeCodeCommitError(w, r, body, requestID, http.StatusBadRequest, "FileContentRequiredException",
			"File content is required.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCodeCommitError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to put file.", readOnly, eventID, verified)
		return
	}
	payload, err := ccsvc.PutFileJSON(result)
	if err != nil {
		s.writeCodeCommitError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeCodeCommitOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, codecommitEventSource, "PutFile", readOnly)
}

func (s *Server) codecommitGetFile(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name := stringParam(params["repositoryName"])
	resource := store.CodeCommitRepositoryARN(s.codecommitRegion(verified), verified.AccountID, name)
	if name == "" {
		resource = "*"
	}
	if !s.authorize(verified, catalog.ActionCodeCommitGetFile, resource) {
		s.writeCodeCommitError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform codecommit:GetFile.", readOnly, eventID, verified)
		return
	}
	file, err := s.store.GetCodeCommitFile(
		verified.AccountID, s.codecommitRegion(verified), name, stringParam(params["filePath"]),
	)
	if errors.Is(err, store.ErrCodeCommitRepoNotFound) {
		s.writeCodeCommitError(w, r, body, requestID, http.StatusBadRequest, "RepositoryDoesNotExistException",
			"The specified repository does not exist.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCodeCommitFileNotFound) {
		s.writeCodeCommitError(w, r, body, requestID, http.StatusBadRequest, "FileDoesNotExistException",
			"The specified file does not exist.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCodeCommitPathEscape) {
		s.writeCodeCommitError(w, r, body, requestID, http.StatusBadRequest, "InvalidPathException",
			"The specified path is not valid.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCodeCommitError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to get file.", readOnly, eventID, verified)
		return
	}
	payload, err := ccsvc.GetFileJSON(file)
	if err != nil {
		s.writeCodeCommitError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeCodeCommitOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, codecommitEventSource, "GetFile", readOnly)
}

func (s *Server) codecommitGetFolder(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	name := stringParam(params["repositoryName"])
	resource := store.CodeCommitRepositoryARN(s.codecommitRegion(verified), verified.AccountID, name)
	if name == "" {
		resource = "*"
	}
	if !s.authorize(verified, catalog.ActionCodeCommitGetFolder, resource) {
		s.writeCodeCommitError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform codecommit:GetFolder.", readOnly, eventID, verified)
		return
	}
	folder, err := s.store.GetCodeCommitFolder(
		verified.AccountID, s.codecommitRegion(verified), name, stringParam(params["folderPath"]),
	)
	if errors.Is(err, store.ErrCodeCommitRepoNotFound) {
		s.writeCodeCommitError(w, r, body, requestID, http.StatusBadRequest, "RepositoryDoesNotExistException",
			"The specified repository does not exist.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCodeCommitFolderNotFound) {
		s.writeCodeCommitError(w, r, body, requestID, http.StatusBadRequest, "FolderDoesNotExistException",
			"The specified folder does not exist.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrCodeCommitPathEscape) {
		s.writeCodeCommitError(w, r, body, requestID, http.StatusBadRequest, "InvalidPathException",
			"The specified path is not valid.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeCodeCommitError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to get folder.", readOnly, eventID, verified)
		return
	}
	payload, err := ccsvc.GetFolderJSON(folder)
	if err != nil {
		s.writeCodeCommitError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeCodeCommitOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, codecommitEventSource, "GetFolder", readOnly)
}

func (s *Server) writeCodeCommitOK(w http.ResponseWriter, requestID string, payload []byte) {
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", codecommitJSONContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeCodeCommitError(
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
	w.Header().Set("Content-Type", codecommitJSONContentType)
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
