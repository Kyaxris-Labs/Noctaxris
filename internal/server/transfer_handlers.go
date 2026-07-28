package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/sts"
	transfersvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/transfer"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	transferJSONContentType = "application/x-amz-json-1.1"
	transferEventSource     = "transfer.amazonaws.com"
)

func (s *Server) handleTransfer(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = transferAction(action)

	switch action {
	case catalog.ActionTransferCreateServer:
		s.transferCreateServer(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionTransferDescribeServer:
		s.transferDescribeServer(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionTransferListServers:
		s.transferListServers(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionTransferDeleteServer:
		s.transferDeleteServer(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionTransferCreateUser:
		s.transferCreateUser(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionTransferDescribeUser:
		s.transferDescribeUser(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionTransferListUsers:
		s.transferListUsers(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionTransferDeleteUser:
		s.transferDeleteUser(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionTransferImportSshPublicKey:
		s.transferImportSshPublicKey(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionTransferDeleteSshPublicKey:
		s.transferDeleteSshPublicKey(w, r, body, requestID, eventID, verified, readOnly, params)
	case labActionTransferPutFile:
		serverID, _ := params["ServerId"].(string)
		arn := store.TransferServerARN(s.transferRegion(verified), verified.AccountID, serverID)
		s.transferPutFile(w, r, body, requestID, eventID, verified, readOnly, params, arn)
	case labActionTransferGetFile:
		serverID, _ := params["ServerId"].(string)
		arn := store.TransferServerARN(s.transferRegion(verified), verified.AccountID, serverID)
		s.transferGetFile(w, r, body, requestID, eventID, verified, readOnly, params, arn)
	case labActionTransferListDirectory:
		s.transferListDirectory(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeTransferError(w, r, body, requestID, http.StatusNotImplemented, "InternalFailure",
			"This Transfer action is not implemented.", readOnly, eventID, verified)
	}
}

func transferAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "CreateServer":
		return catalog.ActionTransferCreateServer
	case "DescribeServer":
		return catalog.ActionTransferDescribeServer
	case "ListServers":
		return catalog.ActionTransferListServers
	case "DeleteServer":
		return catalog.ActionTransferDeleteServer
	case "CreateUser":
		return catalog.ActionTransferCreateUser
	case "DescribeUser":
		return catalog.ActionTransferDescribeUser
	case "ListUsers":
		return catalog.ActionTransferListUsers
	case "DeleteUser":
		return catalog.ActionTransferDeleteUser
	case "ImportSshPublicKey":
		return catalog.ActionTransferImportSshPublicKey
	case "DeleteSshPublicKey":
		return catalog.ActionTransferDeleteSshPublicKey
	case "PutFile":
		return labActionTransferPutFile
	case "GetFile":
		return labActionTransferGetFile
	case "ListDirectory":
		return labActionTransferListDirectory
	default:
		return action
	}
}

func (s *Server) transferRegion(verified *authn.Verified) string {
	if verified.Region != "" {
		return verified.Region
	}
	return store.DefaultTransferRegion
}

func (s *Server) checkTransferPassRole(verified *authn.Verified, roleARN string) error {
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
		return errors.New("not authorized to pass role to Transfer")
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
		ServicePrincipal: authz.ServicePrincipalTransfer,
	})
	if decision != authz.Allow {
		return errors.New("not authorized to pass role to Transfer")
	}
	return nil
}

func (s *Server) transferCreateServer(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	if !s.authorize(verified, catalog.ActionTransferCreateServer, "*") {
		s.writeTransferError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform transfer:CreateServer.", readOnly, eventID, verified)
		return
	}
	if details, ok := params["EndpointDetails"]; ok && details != nil {
		if m, ok := details.(map[string]any); ok && len(m) > 0 {
			s.writeTransferError(w, r, body, requestID, http.StatusBadRequest, "InvalidRequestException",
				"EndpointDetails is not supported; lab Transfer has no VPC or listener.", readOnly, eventID, verified)
			return
		}
	}
	if et, ok := params["EndpointType"].(string); ok && strings.TrimSpace(et) != "" {
		s.writeTransferError(w, r, body, requestID, http.StatusBadRequest, "InvalidRequestException",
			"EndpointType is not supported; lab Transfer omits EndpointType (no VPC listener).", readOnly, eventID, verified)
		return
	}
	var protocols []string
	if raw, ok := params["Protocols"].([]any); ok {
		for _, p := range raw {
			if s, ok := p.(string); ok {
				protocols = append(protocols, s)
			}
		}
	}
	sv, err := s.store.CreateTransferServer(verified.AccountID, s.transferRegion(verified), protocols)
	if errors.Is(err, store.ErrTransferBadRequest) {
		s.writeTransferError(w, r, body, requestID, http.StatusBadRequest, "InvalidRequestException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeTransferError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create server.", readOnly, eventID, verified)
		return
	}
	payload, _ := transfersvc.CreateServerJSON(sv)
	s.writeTransferOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, transferEventSource, "CreateServer", readOnly)
}

func (s *Server) transferDescribeServer(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	id, _ := params["ServerId"].(string)
	arn := store.TransferServerARN(s.transferRegion(verified), verified.AccountID, id)
	if !s.authorize(verified, catalog.ActionTransferDescribeServer, arn) {
		s.writeTransferError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform transfer:DescribeServer.", readOnly, eventID, verified)
		return
	}
	sv, err := s.store.DescribeTransferServer(verified.AccountID, id)
	if errors.Is(err, store.ErrTransferServerNotFound) {
		s.writeTransferError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Server not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeTransferError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe server.", readOnly, eventID, verified)
		return
	}
	payload, _ := transfersvc.DescribeServerJSON(sv)
	s.writeTransferOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, transferEventSource, "DescribeServer", readOnly)
}

func (s *Server) transferListServers(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	_ = params
	if !s.authorize(verified, catalog.ActionTransferListServers, "*") {
		s.writeTransferError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform transfer:ListServers.", readOnly, eventID, verified)
		return
	}
	servers, err := s.store.ListTransferServers(verified.AccountID)
	if err != nil {
		s.writeTransferError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list servers.", readOnly, eventID, verified)
		return
	}
	payload, _ := transfersvc.ListServersJSON(servers)
	s.writeTransferOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, transferEventSource, "ListServers", readOnly)
}

func (s *Server) transferDeleteServer(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	id, _ := params["ServerId"].(string)
	arn := store.TransferServerARN(s.transferRegion(verified), verified.AccountID, id)
	if !s.authorize(verified, catalog.ActionTransferDeleteServer, arn) {
		s.writeTransferError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform transfer:DeleteServer.", readOnly, eventID, verified)
		return
	}
	err := s.store.DeleteTransferServer(verified.AccountID, id)
	if errors.Is(err, store.ErrTransferServerNotFound) {
		s.writeTransferError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Server not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeTransferError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete server.", readOnly, eventID, verified)
		return
	}
	w.Header().Set("Content-Type", transferJSONContentType)
	w.Header().Set("x-amzn-RequestId", requestID)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{}`))
	s.writeSuccessAudit(r, requestID, eventID, verified, transferEventSource, "DeleteServer", readOnly)
}

func (s *Server) transferCreateUser(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	serverID, _ := params["ServerId"].(string)
	userName, _ := params["UserName"].(string)
	home, _ := params["HomeDirectory"].(string)
	roleARN, _ := params["Role"].(string)
	arn := store.TransferServerARN(s.transferRegion(verified), verified.AccountID, serverID)
	if !s.authorize(verified, catalog.ActionTransferCreateUser, arn) {
		s.writeTransferError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform transfer:CreateUser.", readOnly, eventID, verified)
		return
	}
	if strings.TrimSpace(roleARN) != "" {
		if err := s.checkTransferPassRole(verified, roleARN); err != nil {
			s.writeTransferError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
				err.Error(), readOnly, eventID, verified)
			return
		}
	}
	u, err := s.store.CreateTransferUser(verified.AccountID, serverID, userName, home, roleARN)
	if errors.Is(err, store.ErrTransferServerNotFound) {
		s.writeTransferError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Server not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrTransferUserExists) {
		s.writeTransferError(w, r, body, requestID, http.StatusConflict, "ResourceExistsException",
			"User already exists.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrTransferBadRequest) {
		s.writeTransferError(w, r, body, requestID, http.StatusBadRequest, "InvalidRequestException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeTransferError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create user.", readOnly, eventID, verified)
		return
	}
	payload, _ := transfersvc.CreateUserJSON(u)
	s.writeTransferOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, transferEventSource, "CreateUser", readOnly)
}

func (s *Server) transferDeleteUser(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	serverID, _ := params["ServerId"].(string)
	userName, _ := params["UserName"].(string)
	arn := store.TransferServerARN(s.transferRegion(verified), verified.AccountID, serverID)
	if !s.authorize(verified, catalog.ActionTransferDeleteUser, arn) {
		s.writeTransferError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform transfer:DeleteUser.", readOnly, eventID, verified)
		return
	}
	err := s.store.DeleteTransferUser(verified.AccountID, serverID, userName)
	if errors.Is(err, store.ErrTransferUserNotFound) {
		s.writeTransferError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"User not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeTransferError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete user.", readOnly, eventID, verified)
		return
	}
	w.Header().Set("Content-Type", transferJSONContentType)
	w.Header().Set("x-amzn-RequestId", requestID)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{}`))
	s.writeSuccessAudit(r, requestID, eventID, verified, transferEventSource, "DeleteUser", readOnly)
}

func (s *Server) transferDescribeUser(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	serverID, _ := params["ServerId"].(string)
	userName, _ := params["UserName"].(string)
	arn := store.TransferServerARN(s.transferRegion(verified), verified.AccountID, serverID)
	if !s.authorize(verified, catalog.ActionTransferDescribeUser, arn) {
		s.writeTransferError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform transfer:DescribeUser.", readOnly, eventID, verified)
		return
	}
	u, err := s.store.DescribeTransferUser(verified.AccountID, serverID, userName)
	if errors.Is(err, store.ErrTransferServerNotFound) || errors.Is(err, store.ErrTransferUserNotFound) {
		s.writeTransferError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"User not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeTransferError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe user.", readOnly, eventID, verified)
		return
	}
	keys, err := s.store.ListTransferUserSshPublicKeys(verified.AccountID, serverID, userName)
	if err != nil {
		s.writeTransferError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe user.", readOnly, eventID, verified)
		return
	}
	payload, _ := transfersvc.DescribeUserJSON(s.transferRegion(verified), verified.AccountID, u, keys)
	s.writeTransferOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, transferEventSource, "DescribeUser", readOnly)
}

func (s *Server) transferListUsers(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	serverID, _ := params["ServerId"].(string)
	arn := store.TransferServerARN(s.transferRegion(verified), verified.AccountID, serverID)
	if !s.authorize(verified, catalog.ActionTransferListUsers, arn) {
		s.writeTransferError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform transfer:ListUsers.", readOnly, eventID, verified)
		return
	}
	users, err := s.store.ListTransferUsers(verified.AccountID, serverID)
	if errors.Is(err, store.ErrTransferServerNotFound) {
		s.writeTransferError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"Server not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeTransferError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list users.", readOnly, eventID, verified)
		return
	}
	keyCounts := make(map[string]int, len(users))
	for _, u := range users {
		n, err := s.store.CountTransferUserSshPublicKeys(verified.AccountID, serverID, u.UserName)
		if err != nil {
			s.writeTransferError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to list users.", readOnly, eventID, verified)
			return
		}
		keyCounts[u.UserName] = n
	}
	payload, _ := transfersvc.ListUsersJSON(s.transferRegion(verified), verified.AccountID, serverID, users, keyCounts)
	s.writeTransferOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, transferEventSource, "ListUsers", readOnly)
}

func (s *Server) transferImportSshPublicKey(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	serverID, _ := params["ServerId"].(string)
	userName, _ := params["UserName"].(string)
	keyBody, _ := params["SshPublicKeyBody"].(string)
	arn := store.TransferServerARN(s.transferRegion(verified), verified.AccountID, serverID)
	if !s.authorize(verified, catalog.ActionTransferImportSshPublicKey, arn) {
		s.writeTransferError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform transfer:ImportSshPublicKey.", readOnly, eventID, verified)
		return
	}
	key, err := s.store.ImportTransferSshPublicKey(verified.AccountID, serverID, userName, keyBody)
	if errors.Is(err, store.ErrTransferServerNotFound) || errors.Is(err, store.ErrTransferUserNotFound) {
		s.writeTransferError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"User not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrTransferBadRequest) {
		s.writeTransferError(w, r, body, requestID, http.StatusBadRequest, "InvalidRequestException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeTransferError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to import SSH public key.", readOnly, eventID, verified)
		return
	}
	payload, _ := transfersvc.ImportSshPublicKeyJSON(serverID, userName, key.SshPublicKeyID)
	s.writeTransferOK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, transferEventSource, "ImportSshPublicKey", readOnly)
}

func (s *Server) transferDeleteSshPublicKey(
	w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string,
	verified *authn.Verified, readOnly bool, params map[string]any,
) {
	serverID, _ := params["ServerId"].(string)
	userName, _ := params["UserName"].(string)
	keyID, _ := params["SshPublicKeyId"].(string)
	arn := store.TransferServerARN(s.transferRegion(verified), verified.AccountID, serverID)
	if !s.authorize(verified, catalog.ActionTransferDeleteSshPublicKey, arn) {
		s.writeTransferError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform transfer:DeleteSshPublicKey.", readOnly, eventID, verified)
		return
	}
	err := s.store.DeleteTransferSshPublicKey(verified.AccountID, serverID, userName, keyID)
	if errors.Is(err, store.ErrTransferServerNotFound) || errors.Is(err, store.ErrTransferUserNotFound) {
		s.writeTransferError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"User not found.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrTransferSshKeyNotFound) {
		s.writeTransferError(w, r, body, requestID, http.StatusBadRequest, "ResourceNotFoundException",
			"SSH public key not found.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeTransferError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete SSH public key.", readOnly, eventID, verified)
		return
	}
	w.Header().Set("Content-Type", transferJSONContentType)
	w.Header().Set("x-amzn-RequestId", requestID)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{}`))
	s.writeSuccessAudit(r, requestID, eventID, verified, transferEventSource, "DeleteSshPublicKey", readOnly)
}

func (s *Server) writeTransferOK(w http.ResponseWriter, requestID string, payload []byte) {
	w.Header().Set("Content-Type", transferJSONContentType)
	w.Header().Set("x-amzn-RequestId", requestID)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeTransferError(
	w http.ResponseWriter, r *http.Request, body []byte, requestID string,
	status int, code, message string, readOnly bool, eventID string, verified *authn.Verified,
) {
	w.Header().Set("Content-Type", transferJSONContentType)
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
