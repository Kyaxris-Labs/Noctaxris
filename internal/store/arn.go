package store

import (
	"fmt"
	"strings"
)

// UserARN builds an IAM user ARN for accountID, path, and userName.
func UserARN(accountID, path, userName string) string {
	return iamResourceARN(accountID, "user", path, userName)
}

// PolicyARN builds a customer-managed IAM policy ARN.
func PolicyARN(accountID, path, policyName string) string {
	return iamResourceARN(accountID, "policy", path, policyName)
}

// RoleARN builds an IAM role ARN for the given account and role name (path "/").
func RoleARN(accountID, roleName string) string {
	return RoleARNWithPath(accountID, "/", roleName)
}

// RoleARNWithPath builds an IAM role ARN including path.
func RoleARNWithPath(accountID, path, roleName string) string {
	return iamResourceARN(accountID, "role", path, roleName)
}

// GroupARN builds an IAM group ARN.
func GroupARN(accountID, path, groupName string) string {
	return iamResourceARN(accountID, "group", path, groupName)
}

// InstanceProfileARN builds an IAM instance profile ARN.
func InstanceProfileARN(accountID, path, profileName string) string {
	return iamResourceARN(accountID, "instance-profile", path, profileName)
}

// MFADeviceARN builds a virtual MFA device serial ARN.
func MFADeviceARN(accountID, serialSuffix string) string {
	return fmt.Sprintf("arn:aws:iam::%s:mfa/%s", accountID, serialSuffix)
}

func iamResourceARN(accountID, resourceType, path, name string) string {
	p := normalizeIAMPath(path)
	if p == "/" {
		return fmt.Sprintf("arn:aws:iam::%s:%s/%s", accountID, resourceType, name)
	}
	trimmed := strings.Trim(p, "/")
	return fmt.Sprintf("arn:aws:iam::%s:%s/%s/%s", accountID, resourceType, trimmed, name)
}

// arnAccountID extracts the 12-digit account id from arn:aws:service:region:ACCOUNT:...
func arnAccountID(arn string) (string, bool) {
	arn = strings.TrimSpace(arn)
	parts := strings.Split(arn, ":")
	if len(parts) < 5 || parts[0] != "arn" || parts[1] != "aws" {
		return "", false
	}
	acct := strings.TrimSpace(parts[4])
	if len(acct) != 12 {
		return "", false
	}
	for _, r := range acct {
		if r < '0' || r > '9' {
			return "", false
		}
	}
	return acct, true
}

func normalizeIAMPath(path string) string {
	if path == "" {
		return "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}
	return path
}
