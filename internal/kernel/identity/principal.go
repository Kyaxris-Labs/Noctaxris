package identity

import "fmt"

// Kind classifies an IAM principal.
type Kind string

const (
	KindRoot Kind = "Root"
	KindUser Kind = "User"
	KindRole Kind = "Role"
)

// Principal is a verified IAM identity for request evaluation.
type Principal struct {
	Kind        Kind
	AccountID   string
	AccessKeyID string
	IsRoot      bool
	RoleName    string
	SessionName string
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
// Role session: arn:aws:sts::ACCOUNT:assumed-role/ROLE/SESSION
// Role without session: arn:aws:iam::ACCOUNT:role/NAME
func (p Principal) ARN() string {
	if p.IsRoot || p.Kind == KindRoot {
		return fmt.Sprintf("arn:aws:iam::%s:root", p.AccountID)
	}
	if p.Kind == KindRole && p.RoleName != "" {
		if p.SessionName != "" {
			return fmt.Sprintf("arn:aws:sts::%s:assumed-role/%s/%s", p.AccountID, p.RoleName, p.SessionName)
		}
		return fmt.Sprintf("arn:aws:iam::%s:role/%s", p.AccountID, p.RoleName)
	}
	return ""
}
