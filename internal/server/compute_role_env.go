package server

import (
	"context"
	"fmt"
	"strings"

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
		"AWS_ACCESS_KEY_ID":         accessKeyID,
		"AWS_SECRET_ACCESS_KEY":     secret,
		"AWS_SESSION_TOKEN":         sessionToken,
		"AWS_DEFAULT_REGION":        region,
		"AWS_REGION":                region,
		"AWS_ENDPOINT_URL":          endpoint,
		"AWS_ENDPOINT_URL_STS":      endpoint,
		"AWS_ENDPOINT_URL_IAM":      endpoint,
		"AWS_ENDPOINT_URL_S3":       endpoint,
		"AWS_ENDPOINT_URL_DYNAMODB": endpoint,
		"AWS_ENDPOINT_URL_SQS":      endpoint,
		"AWS_ENDPOINT_URL_LAMBDA":   endpoint,
		"AWS_ENDPOINT_URL_KMS":      endpoint,
		"AWS_ENDPOINT_URL_ECR":      endpoint,
		"AWS_ENDPOINT_URL_ECS":      endpoint,
	}, nil
}

// prepareLabRegistryImage rewrites lab ECR refs for DinD and authenticates the pull when needed.
func (s *Server) prepareLabRegistryImage(
	ctx context.Context,
	cli *compute.Client,
	accountID, imageURI, principal string,
) (pullRef string, err error) {
	pullRef, useAuth, username, password, err := compute.IssueLabRegistryPull(
		s.store, s.cfg.ListenAddr, accountID, imageURI, principal,
	)
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
