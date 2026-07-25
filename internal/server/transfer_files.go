package server

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// Lab Transfer file API action names (not in AWS control plane).
const (
	labActionTransferPutFile         = "transfer:PutFile"
	labActionTransferGetFile         = "transfer:GetFile"
	labActionTransferListDirectory   = "transfer:ListDirectory"
	transferLabHomePathPrefix        = "/transfer/"
)

// IsTransferLabHomePath reports whether path is /transfer/{serverId}/home/{user}/...
func IsTransferLabHomePath(path string) bool {
	_, _, _, ok := ParseTransferLabHomePath(path)
	return ok
}

// ParseTransferLabHomePath splits /transfer/{serverId}/home/{user}/{relative...}.
func ParseTransferLabHomePath(path string) (serverID, userName, relativePath string, ok bool) {
	path = strings.TrimSuffix(path, "/")
	if !strings.HasPrefix(path, transferLabHomePathPrefix) {
		return "", "", "", false
	}
	rest := strings.TrimPrefix(path, transferLabHomePathPrefix)
	parts := strings.Split(rest, "/")
	if len(parts) < 3 || parts[1] != "home" {
		return "", "", "", false
	}
	serverID = parts[0]
	userName = parts[2]
	if serverID == "" || userName == "" {
		return "", "", "", false
	}
	if len(parts) > 3 {
		relativePath = strings.Join(parts[3:], "/")
	}
	return serverID, userName, relativePath, true
}

// handleTransferLabHome serves GET/PUT on /transfer/{serverId}/home/{user}/... with SigV4 transfer service.
func (s *Server) handleTransferLabHome(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
) {
	serverID, userName, relPath, ok := ParseTransferLabHomePath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	arn := store.TransferServerARN(s.transferRegion(verified), verified.AccountID, serverID)
	switch r.Method {
	case http.MethodGet:
		if readOnly {
			s.transferGetFile(w, r, body, requestID, eventID, verified, readOnly, map[string]any{
				"ServerId": serverID, "UserName": userName, "Path": relPath,
			}, arn)
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	case http.MethodPut:
		if readOnly {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.transferPutFile(w, r, body, requestID, eventID, verified, readOnly, map[string]any{
			"ServerId": serverID, "UserName": userName, "Path": relPath, "Body": string(body),
		}, arn)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) transferPutFile(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any, serverARN string,
) {
	if readOnly {
		s.writeTransferError(w, r, body, requestID, http.StatusBadRequest, "InvalidRequestException",
			"PutFile requires a write request.", readOnly, eventID, verified)
		return
	}
	if !s.authorize(verified, labActionTransferPutFile, serverARN) {
		s.writeTransferError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform transfer:PutFile.", readOnly, eventID, verified)
		return
	}
	serverID, _ := params["ServerId"].(string)
	userName, _ := params["UserName"].(string)
	path, _ := params["Path"].(string)
	content, err := transferFileContentFromParams(params)
	if err != nil {
		s.writeTransferError(w, r, body, requestID, http.StatusBadRequest, "InvalidRequestException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err := s.store.TransferPutFile(verified.AccountID, serverID, userName, path, content); err != nil {
		s.writeTransferFileStoreError(w, r, body, requestID, err, readOnly, eventID, verified)
		return
	}
	payload, _ := json.Marshal(map[string]any{"Path": path})
	s.writeTransferOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, transferEventSource, "PutFile", readOnly)
}

func (s *Server) transferGetFile(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any, serverARN string,
) {
	if !s.authorize(verified, labActionTransferGetFile, serverARN) {
		s.writeTransferError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform transfer:GetFile.", readOnly, eventID, verified)
		return
	}
	serverID, _ := params["ServerId"].(string)
	userName, _ := params["UserName"].(string)
	path, _ := params["Path"].(string)
	data, err := s.store.TransferGetFile(verified.AccountID, serverID, userName, path)
	if err != nil {
		s.writeTransferFileStoreError(w, r, body, requestID, err, readOnly, eventID, verified)
		return
	}
	payload, _ := json.Marshal(map[string]any{
		"Path":        path,
		"BodyBase64":  base64.StdEncoding.EncodeToString(data),
		"ContentType": http.DetectContentType(data),
	})
	s.writeTransferOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, transferEventSource, "GetFile", readOnly)
}

func (s *Server) transferListDirectory(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	serverID, _ := params["ServerId"].(string)
	userName, _ := params["UserName"].(string)
	path, _ := params["Path"].(string)
	arn := store.TransferServerARN(s.transferRegion(verified), verified.AccountID, serverID)
	if !s.authorize(verified, labActionTransferListDirectory, arn) {
		s.writeTransferError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform transfer:ListDirectory.", readOnly, eventID, verified)
		return
	}
	entries, err := s.store.TransferListDirectory(verified.AccountID, serverID, userName, path)
	if err != nil {
		s.writeTransferFileStoreError(w, r, body, requestID, err, readOnly, eventID, verified)
		return
	}
	listed := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		listed = append(listed, map[string]any{
			"Name":  e.Name,
			"IsDir": e.IsDir,
			"Size":  e.Size,
		})
	}
	payload, _ := json.Marshal(map[string]any{"Path": path, "Entries": listed})
	s.writeTransferOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, transferEventSource, "ListDirectory", readOnly)
}

func transferFileContentFromParams(params map[string]any) ([]byte, error) {
	if b64, ok := params["BodyBase64"].(string); ok && strings.TrimSpace(b64) != "" {
		return base64.StdEncoding.DecodeString(b64)
	}
	if raw, ok := params["Body"].(string); ok {
		return []byte(raw), nil
	}
	return nil, errors.New("Body or BodyBase64 is required")
}

func (s *Server) writeTransferFileStoreError(
	w http.ResponseWriter, r *http.Request, body []byte, requestID string, err error,
	readOnly bool, eventID string, verified *authn.Verified,
) {
	switch {
	case errors.Is(err, store.ErrTransferPathEscape), errors.Is(err, store.ErrTransferBadRequest):
		s.writeTransferError(w, r, body, requestID, http.StatusBadRequest, "InvalidRequestException",
			err.Error(), readOnly, eventID, verified)
	case errors.Is(err, store.ErrTransferServerNotFound), errors.Is(err, store.ErrTransferUserNotFound),
		errors.Is(err, store.ErrTransferPathNotFound):
		s.writeTransferError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Path not found.", readOnly, eventID, verified)
	default:
		s.writeTransferError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to complete file operation.", readOnly, eventID, verified)
	}
}
