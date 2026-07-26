package server

import (
	"context"
	"fmt"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/compute"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/sts"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// mintRoleSessionEnv mints temporary credentials for roleARN and returns AWS_*
// keys plus lab endpoint URLs to overlay after container environment merge.
func (s *Server) mintRoleSessionEnv(roleARN, sessionName, endpoint, region string) (map[string]string, error) {
	roleAccountID, _, ok := sts.ParseRoleARN(roleARN)
	if !ok {
		return nil, fmt.Errorf("role ARN is invalid")
	}
	secret, err := randomSecret()
	if err != nil {
		return nil, fmt.Errorf("mint credentials: %w", err)
	}
	sessionToken, err := randomSecret()
	if err != nil {
		return nil, fmt.Errorf("mint credentials: %w", err)
	}
	expires := s.now().UTC().Add(defaultSessionDuration)
	accessKeyID, err := s.store.MintTempCredentialsOpts(store.MintTempOpts{
		AccountID:    roleAccountID,
		RoleARN:      roleARN,
		SessionName:  sessionName,
		Secret:       secret,
		SessionToken: sessionToken,
		Expires:      expires,
	})
	if err != nil {
		return nil, fmt.Errorf("mint role credentials: %w", err)
	}
	endpoint = strings.TrimSpace(endpoint)
	region = strings.TrimSpace(region)
	if region == "" {
		region = "us-east-1"
	}
	return map[string]string{
		"AWS_ACCESS_KEY_ID":                accessKeyID,
		"AWS_SECRET_ACCESS_KEY":            secret,
		"AWS_SESSION_TOKEN":                sessionToken,
		"AWS_DEFAULT_REGION":               region,
		"AWS_REGION":                       region,
		"AWS_ENDPOINT_URL":                 endpoint,
		"AWS_ENDPOINT_URL_STS":             endpoint,
		"AWS_ENDPOINT_URL_IAM":             endpoint,
		"AWS_ENDPOINT_URL_S3":              endpoint,
		"AWS_ENDPOINT_URL_DYNAMODB":        endpoint,
		"AWS_ENDPOINT_URL_SQS":             endpoint,
		"AWS_ENDPOINT_URL_LAMBDA":          endpoint,
		"AWS_ENDPOINT_URL_KMS":             endpoint,
		"AWS_ENDPOINT_URL_ECR":             endpoint,
		"AWS_ENDPOINT_URL_ECS":             endpoint,
		"AWS_ENDPOINT_URL_SNS":             endpoint,
		"AWS_ENDPOINT_URL_LOGS":            endpoint,
		"AWS_ENDPOINT_URL_CODEBUILD":       endpoint,
		"AWS_ENDPOINT_URL_SECRETSMANAGER":  endpoint,
		"AWS_ENDPOINT_URL_SECRETS_MANAGER": endpoint,
	}, nil
}

// prepareLabRegistryImage rewrites lab ECR refs for DinD, authorizes the pull principal,
// and pulls when auth is required (CodeBuild / Batch nested starts).
func (s *Server) prepareLabRegistryImage(
	ctx context.Context,
	cli *compute.Client,
	accountID, imageURI, principal string,
) (pullRef string, err error) {
	pullRef, useAuth, username, password, err := s.labRegistryPullOpts(accountID, imageURI, principal)
	if err != nil {
		return "", err
	}
	if useAuth {
		if err := cli.PullLabRegistryImage(ctx, pullRef, username, password); err != nil {
			return "", fmt.Errorf("pull lab registry image: %w", err)
		}
	}
	return pullRef, nil
}

// labRegistryPullOpts rewrites lab ECR refs for DinD and issues a token as roleARN.
// Docker pull happens inside RunECSTask / RunImageInvoke (single authenticated pull site).
func (s *Server) labRegistryPullOpts(accountID, imageURI, roleARN string) (pullRef string, useAuth bool, username, password string, err error) {
	pullRef, useAuth, username, password, err = compute.IssueLabRegistryPull(
		s.store, s.cfg.ListenAddr, accountID, imageURI, roleARN,
	)
	if err != nil {
		return "", false, "", "", err
	}
	if !useAuth {
		return pullRef, false, "", "", nil
	}
	if err := s.ensureLabRegistryPullAuthorized(accountID, roleARN, imageURI); err != nil {
		return "", true, "", "", err
	}
	return pullRef, true, username, password, nil
}

// ensureLabRegistryPullAuthorized fail-closes when the pull principal lacks ecr:BatchGetImage
// on the lab repository (Registry V2 IAM path).
func (s *Server) ensureLabRegistryPullAuthorized(accountID, principalARN, imageURI string) error {
	repoName := labRegistryRepoName(imageURI)
	if repoName == "" {
		return fmt.Errorf("lab registry image URI is invalid")
	}
	verified := verifiedFromRegistryPrincipal(accountID, principalARN)
	if verified == nil {
		return fmt.Errorf("registry pull principal %q is not evaluable", principalARN)
	}
	policy, err := s.ecrRepositoryPolicy(accountID, repoName)
	if err != nil {
		return fmt.Errorf("load repository policy: %w", err)
	}
	arn := store.RepositoryARN(store.DefaultECRRegion, accountID, repoName)
	if !s.authorizeECR(verified, catalog.ActionECRBatchGetImage, arn, policy) {
		return fmt.Errorf("role is not authorized to perform ecr:BatchGetImage on repository %q", repoName)
	}
	return nil
}

func labRegistryRepoName(imageURI string) string {
	uri := strings.TrimSpace(imageURI)
	prefix := store.LabRegistryHost + "/"
	path := ""
	switch {
	case strings.HasPrefix(uri, prefix):
		path = strings.TrimPrefix(uri, prefix)
	case strings.HasPrefix(uri, "host.docker.internal:"):
		rest := strings.TrimPrefix(uri, "host.docker.internal:")
		slash := strings.Index(rest, "/")
		if slash < 0 {
			return ""
		}
		path = rest[slash+1:]
	default:
		return ""
	}
	// ACCOUNT/REPO:tag or ACCOUNT/REPO@sha256:...
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 || parts[1] == "" {
		return ""
	}
	repoTag := parts[1]
	if i := strings.IndexAny(repoTag, "@:"); i >= 0 {
		repoTag = repoTag[:i]
	}
	return repoTag
}
