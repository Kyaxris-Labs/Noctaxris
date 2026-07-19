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

// ARN returns the IAM ARN for this principal.
// Root principals use arn:aws:iam::ACCOUNT:root.
func (p Principal) ARN() string {
	if p.IsRoot || p.Kind == KindRoot {
		return fmt.Sprintf("arn:aws:iam::%s:root", p.AccountID)
	}
	return ""
}
