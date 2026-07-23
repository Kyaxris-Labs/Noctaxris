package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	ecrsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/ecr"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const (
	ecrJSONContentType = "application/x-amz-json-1.1"
	ecrEventSource     = "ecr.amazonaws.com"
)

type ecrRepositoryMeta struct {
	repo      store.Repository
	policy    string
	accountID string
}

func (s *Server) handleECR(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID, action string,
	verified *authn.Verified,
	readOnly bool,
) {
	params := jsonBodyMap(body)
	action = ecrAction(action)

	switch action {
	case catalog.ActionECRCreateRepository:
		s.ecrCreateRepository(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionECRDescribeRepositories:
		s.ecrDescribeRepositories(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionECRDeleteRepository:
		s.ecrDeleteRepository(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionECRGetAuthorizationToken:
		s.ecrGetAuthorizationToken(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionECRGetRepositoryPolicy:
		s.ecrGetRepositoryPolicy(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionECRSetRepositoryPolicy:
		s.ecrSetRepositoryPolicy(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionECRDeleteRepositoryPolicy:
		s.ecrDeleteRepositoryPolicy(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionECRPutImage:
		s.ecrPutImage(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionECRBatchGetImage:
		s.ecrBatchGetImage(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionECRListImages:
		s.ecrListImages(w, r, body, requestID, eventID, verified, readOnly, params)
	case catalog.ActionECRBatchDeleteImage:
		s.ecrBatchDeleteImage(w, r, body, requestID, eventID, verified, readOnly, params)
	default:
		s.writeECRError(w, r, body, requestID, http.StatusNotImplemented, "InternalFailure",
			"This ECR action is not implemented.", readOnly, eventID, verified)
	}
}

func ecrAction(action string) string {
	if strings.Contains(action, ":") {
		return action
	}
	switch action {
	case "CreateRepository":
		return catalog.ActionECRCreateRepository
	case "DescribeRepositories":
		return catalog.ActionECRDescribeRepositories
	case "DeleteRepository":
		return catalog.ActionECRDeleteRepository
	case "GetAuthorizationToken":
		return catalog.ActionECRGetAuthorizationToken
	case "GetRepositoryPolicy":
		return catalog.ActionECRGetRepositoryPolicy
	case "SetRepositoryPolicy":
		return catalog.ActionECRSetRepositoryPolicy
	case "DeleteRepositoryPolicy":
		return catalog.ActionECRDeleteRepositoryPolicy
	case "PutImage":
		return catalog.ActionECRPutImage
	case "BatchGetImage":
		return catalog.ActionECRBatchGetImage
	case "ListImages":
		return catalog.ActionECRListImages
	case "BatchDeleteImage":
		return catalog.ActionECRBatchDeleteImage
	default:
		return action
	}
}

func (s *Server) ecrRegion(verified *authn.Verified) string {
	if verified.Region != "" {
		return verified.Region
	}
	return store.DefaultECRRegion
}

func (s *Server) authorizeECR(verified *authn.Verified, action, resource, repositoryPolicy string) bool {
	resourceAccountID := resourceAccountIDFromARN(resource)
	if resourceAccountID == "" {
		resourceAccountID = verified.AccountID
	}
	return s.authorizeDataplaneOR(verified, action, resource, resourceAccountID, func(caller authz.RequestContext, identityDocs []string, resourceAccountID string) authz.Decision {
		return authz.EvaluateDynamoDB(authz.DynamoDBRequest{
			Caller:            caller,
			IdentityDocs:      identityDocs,
			ResourcePolicyDoc: repositoryPolicy,
			ResourceAccountID: resourceAccountID,
		})
	})
}

func (s *Server) ecrRepositoryPolicy(accountID, name string) (string, error) {
	policy, err := s.store.GetRepositoryPolicy(accountID, name)
	if errors.Is(err, store.ErrNoSuchResourcePolicy) {
		return "", nil
	}
	return policy, err
}

func (s *Server) ecrRepositoryMetaOrErr(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	name string,
	registryID string,
) (ecrRepositoryMeta, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		s.writeECRError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			"repositoryName is required.", readOnly, eventID, verified)
		return ecrRepositoryMeta{}, false
	}
	registryID = strings.TrimSpace(registryID)
	if registryID == "" {
		registryID = verified.AccountID
	}
	repos, err := s.store.DescribeRepositories(registryID, []string{name})
	if err != nil {
		s.writeECRError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to load repository.", readOnly, eventID, verified)
		return ecrRepositoryMeta{}, false
	}
	if len(repos) == 0 {
		s.writeECRError(w, r, body, requestID, http.StatusBadRequest, "RepositoryNotFoundException",
			"The repository with name '"+name+"' does not exist in the registry with id '"+registryID+"'.", readOnly, eventID, verified)
		return ecrRepositoryMeta{}, false
	}
	policy, err := s.ecrRepositoryPolicy(registryID, name)
	if err != nil {
		s.writeECRError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to load repository policy.", readOnly, eventID, verified)
		return ecrRepositoryMeta{}, false
	}
	return ecrRepositoryMeta{repo: repos[0], policy: policy, accountID: registryID}, true
}

func ecrRegistryID(params map[string]any, verified *authn.Verified) string {
	if id, _ := params["registryId"].(string); strings.TrimSpace(id) != "" {
		return strings.TrimSpace(id)
	}
	return verified.AccountID
}

func ecrRepositoryName(params map[string]any) string {
	if name, _ := params["repositoryName"].(string); strings.TrimSpace(name) != "" {
		return strings.TrimSpace(name)
	}
	return ""
}

func ecrRepositoryNames(params map[string]any) []string {
	raw, ok := params["repositoryNames"].([]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if name, ok := v.(string); ok && strings.TrimSpace(name) != "" {
			out = append(out, strings.TrimSpace(name))
		}
	}
	return out
}

func ecrImageIDs(params map[string]any) (digests, tags []string) {
	raw, ok := params["imageIds"].([]any)
	if !ok {
		return nil, nil
	}
	for _, v := range raw {
		m, ok := v.(map[string]any)
		if !ok {
			continue
		}
		if d, _ := m["imageDigest"].(string); strings.TrimSpace(d) != "" {
			digests = append(digests, strings.TrimSpace(d))
		}
		if t, _ := m["imageTag"].(string); strings.TrimSpace(t) != "" {
			tags = append(tags, strings.TrimSpace(t))
		}
	}
	return digests, tags
}

func (s *Server) ecrCreateRepository(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	name := ecrRepositoryName(params)
	if name == "" {
		s.writeECRError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			"repositoryName is required.", readOnly, eventID, verified)
		return
	}
	arn := store.RepositoryARN(s.ecrRegion(verified), verified.AccountID, name)
	if !s.authorize(verified, catalog.ActionECRCreateRepository, arn) {
		s.writeECRError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform ecr:CreateRepository.", readOnly, eventID, verified)
		return
	}

	repo, err := s.store.CreateRepository(verified.AccountID, s.ecrRegion(verified), name)
	if errors.Is(err, store.ErrRepositoryAlreadyExists) {
		s.writeECRError(w, r, body, requestID, http.StatusBadRequest, "RepositoryAlreadyExistsException",
			"The repository with name '"+name+"' already exists in the registry with id '"+verified.AccountID+"'.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeECRError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to create repository.", readOnly, eventID, verified)
		return
	}

	payload, err := ecrsvc.CreateRepositoryJSON(repo, verified.AccountID)
	if err != nil {
		s.writeECRError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeECROK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ecrEventSource, "CreateRepository", readOnly)
}

func (s *Server) ecrDescribeRepositories(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	names := ecrRepositoryNames(params)
	if len(names) == 1 {
		meta, ok := s.ecrRepositoryMetaOrErr(w, r, body, requestID, eventID, verified, readOnly, names[0], ecrRegistryID(params, verified))
		if !ok {
			return
		}
		if !s.authorizeECR(verified, catalog.ActionECRDescribeRepositories, meta.repo.ARN, meta.policy) {
			s.writeECRError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
				"User is not authorized to perform ecr:DescribeRepositories.", readOnly, eventID, verified)
			return
		}
		repos := []store.Repository{meta.repo}
		payload, err := ecrsvc.DescribeRepositoriesJSON(repos, meta.accountID)
		if err != nil {
			s.writeECRError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
				"Unable to build response.", readOnly, eventID, verified)
			return
		}
		s.writeECROK(w, requestID, payload)
		s.writeSuccessAudit(r, requestID, eventID, verified, ecrEventSource, "DescribeRepositories", readOnly)
		return
	}

	if !s.authorize(verified, catalog.ActionECRDescribeRepositories, "*") {
		s.writeECRError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform ecr:DescribeRepositories.", readOnly, eventID, verified)
		return
	}
	repos, err := s.store.DescribeRepositories(verified.AccountID, names)
	if err != nil {
		s.writeECRError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to describe repositories.", readOnly, eventID, verified)
		return
	}
	payload, err := ecrsvc.DescribeRepositoriesJSON(repos, verified.AccountID)
	if err != nil {
		s.writeECRError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeECROK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ecrEventSource, "DescribeRepositories", readOnly)
}

func (s *Server) ecrDeleteRepository(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	name := ecrRepositoryName(params)
	meta, ok := s.ecrRepositoryMetaOrErr(w, r, body, requestID, eventID, verified, readOnly, name, ecrRegistryID(params, verified))
	if !ok {
		return
	}
	if !s.authorizeECR(verified, catalog.ActionECRDeleteRepository, meta.repo.ARN, meta.policy) {
		s.writeECRError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform ecr:DeleteRepository.", readOnly, eventID, verified)
		return
	}
	if err := s.store.DeleteRepository(verified.AccountID, name); errors.Is(err, store.ErrRepositoryNotFound) {
		s.writeECRError(w, r, body, requestID, http.StatusBadRequest, "RepositoryNotFoundException",
			"The repository with name '"+name+"' does not exist in the registry with id '"+verified.AccountID+"'.", readOnly, eventID, verified)
		return
	} else if err != nil {
		s.writeECRError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete repository.", readOnly, eventID, verified)
		return
	}
	payload, err := ecrsvc.DeleteRepositoryJSON(meta.repo, verified.AccountID)
	if err != nil {
		s.writeECRError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeECROK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ecrEventSource, "DeleteRepository", readOnly)
}

func (s *Server) ecrGetAuthorizationToken(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	_ = params
	if !s.authorize(verified, catalog.ActionECRGetAuthorizationToken, "*") {
		s.writeECRError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform ecr:GetAuthorizationToken.", readOnly, eventID, verified)
		return
	}
	password, expiresAt, err := s.store.IssueAuthorizationToken(verified.AccountID, verified.Principal.ARN(), 0)
	if err != nil {
		s.writeECRError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to issue authorization token.", readOnly, eventID, verified)
		return
	}
	payload, err := ecrsvc.GetAuthorizationTokenJSON(password, expiresAt)
	if err != nil {
		s.writeECRError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeECROK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ecrEventSource, "GetAuthorizationToken", readOnly)
}

func (s *Server) ecrGetRepositoryPolicy(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	name := ecrRepositoryName(params)
	meta, ok := s.ecrRepositoryMetaOrErr(w, r, body, requestID, eventID, verified, readOnly, name, ecrRegistryID(params, verified))
	if !ok {
		return
	}
	if !s.authorizeECR(verified, catalog.ActionECRGetRepositoryPolicy, meta.repo.ARN, meta.policy) {
		s.writeECRError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform ecr:GetRepositoryPolicy.", readOnly, eventID, verified)
		return
	}
	policy, err := s.store.GetRepositoryPolicy(verified.AccountID, name)
	if errors.Is(err, store.ErrNoSuchResourcePolicy) {
		s.writeECRError(w, r, body, requestID, http.StatusBadRequest, "RepositoryPolicyNotFoundException",
			"Repository policy does not exist for the repository with name '"+name+"'.", readOnly, eventID, verified)
		return
	}
	if errors.Is(err, store.ErrRepositoryNotFound) {
		s.writeECRError(w, r, body, requestID, http.StatusBadRequest, "RepositoryNotFoundException",
			"The repository with name '"+name+"' does not exist in the registry with id '"+verified.AccountID+"'.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeECRError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to get repository policy.", readOnly, eventID, verified)
		return
	}
	payload, err := ecrsvc.GetRepositoryPolicyJSON(name, verified.AccountID, policy)
	if err != nil {
		s.writeECRError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeECROK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ecrEventSource, "GetRepositoryPolicy", readOnly)
}

func (s *Server) ecrSetRepositoryPolicy(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	name := ecrRepositoryName(params)
	meta, ok := s.ecrRepositoryMetaOrErr(w, r, body, requestID, eventID, verified, readOnly, name, ecrRegistryID(params, verified))
	if !ok {
		return
	}
	if !s.authorizeECR(verified, catalog.ActionECRSetRepositoryPolicy, meta.repo.ARN, meta.policy) {
		s.writeECRError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform ecr:SetRepositoryPolicy.", readOnly, eventID, verified)
		return
	}
	policy, _ := params["policyText"].(string)
	if strings.TrimSpace(policy) == "" {
		s.writeECRError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			"policyText is required.", readOnly, eventID, verified)
		return
	}
	if err := authz.ValidateResourcePolicyDocument(policy); err != nil {
		s.writeECRError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			err.Error(), readOnly, eventID, verified)
		return
	}
	if err := s.store.SetRepositoryPolicy(verified.AccountID, name, policy); err != nil {
		if errors.Is(err, store.ErrRepositoryNotFound) {
			s.writeECRError(w, r, body, requestID, http.StatusBadRequest, "RepositoryNotFoundException",
				"The repository with name '"+name+"' does not exist in the registry with id '"+verified.AccountID+"'.", readOnly, eventID, verified)
			return
		}
		s.writeECRError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to set repository policy.", readOnly, eventID, verified)
		return
	}
	payload, err := ecrsvc.SetRepositoryPolicyJSON(name, verified.AccountID, policy)
	if err != nil {
		s.writeECRError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeECROK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ecrEventSource, "SetRepositoryPolicy", readOnly)
}

func (s *Server) ecrDeleteRepositoryPolicy(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	name := ecrRepositoryName(params)
	meta, ok := s.ecrRepositoryMetaOrErr(w, r, body, requestID, eventID, verified, readOnly, name, ecrRegistryID(params, verified))
	if !ok {
		return
	}
	if !s.authorizeECR(verified, catalog.ActionECRDeleteRepositoryPolicy, meta.repo.ARN, meta.policy) {
		s.writeECRError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform ecr:DeleteRepositoryPolicy.", readOnly, eventID, verified)
		return
	}
	if err := s.store.DeleteRepositoryPolicy(verified.AccountID, name); errors.Is(err, store.ErrRepositoryNotFound) {
		s.writeECRError(w, r, body, requestID, http.StatusBadRequest, "RepositoryNotFoundException",
			"The repository with name '"+name+"' does not exist in the registry with id '"+verified.AccountID+"'.", readOnly, eventID, verified)
		return
	} else if err != nil {
		s.writeECRError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to delete repository policy.", readOnly, eventID, verified)
		return
	}
	payload, err := ecrsvc.DeleteRepositoryPolicyJSON(name, verified.AccountID)
	if err != nil {
		s.writeECRError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeECROK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ecrEventSource, "DeleteRepositoryPolicy", readOnly)
}

func (s *Server) ecrPutImage(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	name := ecrRepositoryName(params)
	meta, ok := s.ecrRepositoryMetaOrErr(w, r, body, requestID, eventID, verified, readOnly, name, ecrRegistryID(params, verified))
	if !ok {
		return
	}
	if !s.authorizeECR(verified, catalog.ActionECRPutImage, meta.repo.ARN, meta.policy) {
		s.writeECRError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform ecr:PutImage.", readOnly, eventID, verified)
		return
	}
	manifest, _ := params["imageManifest"].(string)
	digest := ""
	tag := ""
	if imageID, ok := params["imageId"].(map[string]any); ok {
		digest, _ = imageID["imageDigest"].(string)
		tag, _ = imageID["imageTag"].(string)
	}
	if strings.TrimSpace(digest) == "" {
		digest, _ = params["imageDigest"].(string)
	}
	if strings.TrimSpace(digest) == "" {
		s.writeECRError(w, r, body, requestID, http.StatusBadRequest, "InvalidParameterException",
			"imageDigest is required.", readOnly, eventID, verified)
		return
	}
	var tags []string
	if strings.TrimSpace(tag) != "" {
		tags = []string{strings.TrimSpace(tag)}
	}
	img, err := s.store.PutImage(verified.AccountID, name, strings.TrimSpace(digest), tags, manifest)
	if errors.Is(err, store.ErrRepositoryNotFound) {
		s.writeECRError(w, r, body, requestID, http.StatusBadRequest, "RepositoryNotFoundException",
			"The repository with name '"+name+"' does not exist in the registry with id '"+verified.AccountID+"'.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeECRError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to put image.", readOnly, eventID, verified)
		return
	}
	payload, err := ecrsvc.PutImageJSON(img)
	if err != nil {
		s.writeECRError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeECROK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ecrEventSource, "PutImage", readOnly)
}

func (s *Server) ecrListImages(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	name := ecrRepositoryName(params)
	meta, ok := s.ecrRepositoryMetaOrErr(w, r, body, requestID, eventID, verified, readOnly, name, ecrRegistryID(params, verified))
	if !ok {
		return
	}
	if !s.authorizeECR(verified, catalog.ActionECRListImages, meta.repo.ARN, meta.policy) {
		s.writeECRError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform ecr:ListImages.", readOnly, eventID, verified)
		return
	}
	images, err := s.store.ListImages(meta.accountID, name)
	if errors.Is(err, store.ErrRepositoryNotFound) {
		s.writeECRError(w, r, body, requestID, http.StatusBadRequest, "RepositoryNotFoundException",
			"The repository with name '"+name+"' does not exist in the registry with id '"+meta.accountID+"'.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeECRError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to list images.", readOnly, eventID, verified)
		return
	}
	payload, err := ecrsvc.ListImagesJSON(images)
	if err != nil {
		s.writeECRError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeECROK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ecrEventSource, "ListImages", readOnly)
}

func (s *Server) ecrBatchGetImage(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	name := ecrRepositoryName(params)
	meta, ok := s.ecrRepositoryMetaOrErr(w, r, body, requestID, eventID, verified, readOnly, name, ecrRegistryID(params, verified))
	if !ok {
		return
	}
	if !s.authorizeECR(verified, catalog.ActionECRBatchGetImage, meta.repo.ARN, meta.policy) {
		s.writeECRError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform ecr:BatchGetImage.", readOnly, eventID, verified)
		return
	}
	digests, tags := ecrImageIDs(params)
	images, err := s.store.BatchGetImage(meta.accountID, name, digests, tags)
	if errors.Is(err, store.ErrRepositoryNotFound) {
		s.writeECRError(w, r, body, requestID, http.StatusBadRequest, "RepositoryNotFoundException",
			"The repository with name '"+name+"' does not exist in the registry with id '"+meta.accountID+"'.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeECRError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to batch get images.", readOnly, eventID, verified)
		return
	}
	payload, err := ecrsvc.BatchGetImageJSON(images)
	if err != nil {
		s.writeECRError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeECROK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ecrEventSource, "BatchGetImage", readOnly)
}

func (s *Server) ecrBatchDeleteImage(
	w http.ResponseWriter,
	r *http.Request,
	body []byte,
	requestID, eventID string,
	verified *authn.Verified,
	readOnly bool,
	params map[string]any,
) {
	name := ecrRepositoryName(params)
	meta, ok := s.ecrRepositoryMetaOrErr(w, r, body, requestID, eventID, verified, readOnly, name, ecrRegistryID(params, verified))
	if !ok {
		return
	}
	if !s.authorizeECR(verified, catalog.ActionECRBatchDeleteImage, meta.repo.ARN, meta.policy) {
		s.writeECRError(w, r, body, requestID, http.StatusForbidden, "AccessDeniedException",
			"User is not authorized to perform ecr:BatchDeleteImage.", readOnly, eventID, verified)
		return
	}
	digests, tags := ecrImageIDs(params)
	images, err := s.store.BatchGetImage(verified.AccountID, name, digests, tags)
	if errors.Is(err, store.ErrRepositoryNotFound) {
		s.writeECRError(w, r, body, requestID, http.StatusBadRequest, "RepositoryNotFoundException",
			"The repository with name '"+name+"' does not exist in the registry with id '"+verified.AccountID+"'.", readOnly, eventID, verified)
		return
	}
	if err != nil {
		s.writeECRError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to resolve images for delete.", readOnly, eventID, verified)
		return
	}
	if err := s.store.BatchDeleteImage(verified.AccountID, name, digests, tags); err != nil {
		s.writeECRError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to batch delete images.", readOnly, eventID, verified)
		return
	}
	payload, err := ecrsvc.BatchDeleteImageJSON(images)
	if err != nil {
		s.writeECRError(w, r, body, requestID, http.StatusInternalServerError, "InternalFailure",
			"Unable to build response.", readOnly, eventID, verified)
		return
	}
	s.writeECROK(w, requestID, payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, ecrEventSource, "BatchDeleteImage", readOnly)
}

func (s *Server) writeECROK(w http.ResponseWriter, requestID string, payload []byte) {
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", ecrJSONContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeECRError(
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
	w.Header().Set("Content-Type", ecrJSONContentType)
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
