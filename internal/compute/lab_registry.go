package compute

import (
	"fmt"
	"net"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/sts"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// DinDPullHost returns the host:port DinD uses to reach the lab API/registry
// published on the host (host.docker.internal plus the listen port).
func DinDPullHost(listenAddr string) string {
	_, port, err := net.SplitHostPort(strings.TrimSpace(listenAddr))
	if err != nil || port == "" {
		if _, fallbackPort, splitErr := net.SplitHostPort(store.LabRegistryHost); splitErr == nil && fallbackPort != "" {
			port = fallbackPort
		} else {
			port = "4566"
		}
	}
	return "host.docker.internal:" + port
}

// LabImageForDinD rewrites a lab registry ImageUri (127.0.0.1:4566/ACCOUNT/REPO:tag)
// to a DinD-reachable pull reference. Non-lab URIs are returned unchanged.
func LabImageForDinD(imageURI, listenAddr string) (ref string, isLabRegistry bool) {
	uri := strings.TrimSpace(imageURI)
	prefix := store.LabRegistryHost + "/"
	if strings.HasPrefix(uri, prefix) {
		return DinDPullHost(listenAddr) + "/" + strings.TrimPrefix(uri, prefix), true
	}
	return uri, false
}

// LabRegistryPullCreds are Docker Registry credentials for an authenticated pull.
type LabRegistryPullCreds struct {
	Username string
	Password string
}

// ResolveLabImagePull prepares a DinD pull reference and whether lab ECR auth is required.
// Public and third-party refs return useAuth=false. Lab host refs (127.0.0.1 lab registry
// rewrite or already-rewritten DinDPullHost(listenAddr)/ACCOUNT/REPO) require credentials.
// host.docker.internal on any other port does not set useAuth.
func ResolveLabImagePull(imageURI, listenAddr string) (pullRef string, useAuth bool) {
	uri := strings.TrimSpace(imageURI)
	pullRef, isLab := LabImageForDinD(uri, listenAddr)
	if isLab {
		return pullRef, true
	}
	if strings.TrimSpace(listenAddr) == "" {
		return uri, false
	}
	dindPrefix := DinDPullHost(listenAddr) + "/"
	if strings.HasPrefix(uri, dindPrefix) && isLabECRPath(strings.TrimPrefix(uri, dindPrefix)) {
		return uri, true
	}
	return uri, false
}

// RequireLabRegistryCreds fails closed when a lab registry pull is requested without credentials.
func RequireLabRegistryCreds(useAuth bool, creds LabRegistryPullCreds) error {
	if !useAuth {
		return nil
	}
	if strings.TrimSpace(creds.Username) == "" || strings.TrimSpace(creds.Password) == "" {
		return fmt.Errorf("compute: lab registry image requires registry credentials")
	}
	return nil
}

// IsLabRegistryPullPrincipal reports whether principal is an IAM/STS ARN Registry V2 can evaluate.
// Bare service names (e.g. ecs-tasks.amazonaws.com) are rejected; pull tokens must use a role ARN.
func IsLabRegistryPullPrincipal(principal string) bool {
	principal = strings.TrimSpace(principal)
	if principal == "" {
		return false
	}
	if _, _, ok := sts.ParseRoleARN(principal); ok {
		return true
	}
	// assumed-role / user / root ARNs used by GetAuthorizationToken and role sessions
	if strings.HasPrefix(principal, "arn:aws:iam::") || strings.HasPrefix(principal, "arn:aws:sts::") {
		return strings.Contains(principal, ":root") ||
			strings.Contains(principal, ":user/") ||
			strings.Contains(principal, ":role/") ||
			strings.Contains(principal, ":assumed-role/") ||
			strings.Contains(principal, ":federated-user/")
	}
	return false
}

// IssueLabRegistryPull rewrites a lab ECR image URI for DinD and issues a registry
// authorization token. principal must be an IAM/STS ARN (execution/service/job role).
// Public images return useAuth=false.
func IssueLabRegistryPull(st *store.Store, listenAddr, accountID, imageURI, principal string) (pullRef string, useAuth bool, username, password string, err error) {
	pullRef, useAuth = ResolveLabImagePull(imageURI, listenAddr)
	if !useAuth {
		return pullRef, false, "", "", nil
	}
	if st == nil {
		return "", true, "", "", fmt.Errorf("issue registry token: store is nil")
	}
	principal = strings.TrimSpace(principal)
	if !IsLabRegistryPullPrincipal(principal) {
		return "", true, "", "", fmt.Errorf("issue registry token: principal must be an IAM role or STS ARN (got %q)", principal)
	}
	token, _, err := st.IssueAuthorizationToken(accountID, principal, store.DefaultAuthTokenTTL)
	if err != nil {
		return "", true, "", "", fmt.Errorf("issue registry token: %w", err)
	}
	username, password = "AWS", token
	if err := RequireLabRegistryCreds(true, LabRegistryPullCreds{
		Username: username,
		Password: password,
	}); err != nil {
		return "", true, "", "", err
	}
	return pullRef, true, username, password, nil
}
