package identity

import "fmt"

// Kind classifies an IAM principal.
type Kind string

const (
	KindRoot      Kind = "Root"
	KindUser      Kind = "User"
	KindRole      Kind = "Role"
	KindFederated Kind = "Federated"
)

// Principal is a verified IAM identity for request evaluation.
type Principal struct {
	Kind                 Kind
	AccountID            string
	AccessKeyID          string
	IsRoot               bool
	UserName             string
	RoleName             string
	SessionName          string
	FederatedProviderARN string // OIDC/SAML provider ARN for trust Principal.Federated match
}

// RootPrincipal returns the account root principal for the given access key.
func RootPrincipal(accountID, accessKeyID string) Principal {
	return Principal{
		Kind:        KindRoot,
		AccountID:   accountID,
		AccessKeyID: accessKeyID,
		IsRoot:      true,
	}
}

// UserPrincipal returns an IAM user principal.
func UserPrincipal(accountID, userName, accessKeyID string) Principal {
	return Principal{
		Kind:        KindUser,
		AccountID:   accountID,
		AccessKeyID: accessKeyID,
		UserName:    userName,
	}
}

// FederatedUserPrincipal returns an STS federated-user principal.
func FederatedUserPrincipal(accountID, name, accessKeyID string) Principal {
	return Principal{
		Kind:        KindFederated,
		AccountID:   accountID,
		AccessKeyID: accessKeyID,
		SessionName: name,
	}
}

// FederatedProviderPrincipal returns a federation caller principal for trust
// evaluation (AssumeRoleWithSAML / AssumeRoleWithWebIdentity). FederatedProviderARN
// is matched against trust Principal.Federated.
func FederatedProviderPrincipal(accountID, providerARN, sessionName, accessKeyID string) Principal {
	if sessionName == "" {
		sessionName = "federated"
	}
	return Principal{
		Kind:                 KindFederated,
		AccountID:            accountID,
		AccessKeyID:          accessKeyID,
		SessionName:          sessionName,
		FederatedProviderARN: providerARN,
	}
}

// RoleSessionPrincipal returns an assumed-role session principal.
func RoleSessionPrincipal(accountID, roleName, sessionName, accessKeyID string) Principal {
	return Principal{
		Kind:        KindRole,
		AccountID:   accountID,
		AccessKeyID: accessKeyID,
		RoleName:    roleName,
		SessionName: sessionName,
	}
}

// ARN returns the IAM or STS ARN for this principal.
// Root: arn:aws:iam::ACCOUNT:root
// User: arn:aws:iam::ACCOUNT:user/NAME
// Federated: arn:aws:sts::ACCOUNT:federated-user/NAME
// Role session: arn:aws:sts::ACCOUNT:assumed-role/ROLE/SESSION
// Role without session: arn:aws:iam::ACCOUNT:role/NAME
func (p Principal) ARN() string {
	if p.IsRoot || p.Kind == KindRoot {
		return fmt.Sprintf("arn:aws:iam::%s:root", p.AccountID)
	}
	if p.Kind == KindUser && p.UserName != "" {
		return fmt.Sprintf("arn:aws:iam::%s:user/%s", p.AccountID, p.UserName)
	}
	if p.Kind == KindFederated && p.SessionName != "" {
		return fmt.Sprintf("arn:aws:sts::%s:federated-user/%s", p.AccountID, p.SessionName)
	}
	if p.Kind == KindRole && p.RoleName != "" {
		if p.SessionName != "" {
			return fmt.Sprintf("arn:aws:sts::%s:assumed-role/%s/%s", p.AccountID, p.RoleName, p.SessionName)
		}
		return fmt.Sprintf("arn:aws:iam::%s:role/%s", p.AccountID, p.RoleName)
	}
	return ""
}
