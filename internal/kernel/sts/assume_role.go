package sts

import (
	"encoding/xml"
	"fmt"
	"time"
)

// AssumeRoleResult holds fields for an AssumeRole XML response.
type AssumeRoleResult struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	Expiration      time.Time
	AssumedRoleARN  string
	AssumedRoleID   string
	RequestID       string
}

type assumeRoleResponse struct {
	XMLName          xml.Name `xml:"AssumeRoleResponse"`
	XMLNS            string   `xml:"xmlns,attr"`
	AssumeRoleResult struct {
		Credentials struct {
			AccessKeyId     string `xml:"AccessKeyId"`
			SecretAccessKey string `xml:"SecretAccessKey"`
			SessionToken    string `xml:"SessionToken"`
			Expiration      string `xml:"Expiration"`
		} `xml:"Credentials"`
		AssumedRoleUser struct {
			AssumedRoleId string `xml:"AssumedRoleId"`
			Arn           string `xml:"Arn"`
		} `xml:"AssumedRoleUser"`
	} `xml:"AssumeRoleResult"`
	ResponseMetadata struct {
		RequestId string `xml:"RequestId"`
	} `xml:"ResponseMetadata"`
}

// AssumeRoleXML builds an AWS CLI-compatible AssumeRole XML body.
func AssumeRoleXML(r AssumeRoleResult) ([]byte, error) {
	resp := assumeRoleResponse{XMLNS: stsXMLNS}
	resp.AssumeRoleResult.Credentials.AccessKeyId = r.AccessKeyID
	resp.AssumeRoleResult.Credentials.SecretAccessKey = r.SecretAccessKey
	resp.AssumeRoleResult.Credentials.SessionToken = r.SessionToken
	resp.AssumeRoleResult.Credentials.Expiration = r.Expiration.UTC().Format(time.RFC3339)
	resp.AssumeRoleResult.AssumedRoleUser.Arn = r.AssumedRoleARN
	resp.AssumeRoleResult.AssumedRoleUser.AssumedRoleId = r.AssumedRoleID
	resp.ResponseMetadata.RequestId = r.RequestID

	out, err := xml.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("marshal AssumeRole response: %w", err)
	}
	return out, nil
}

// ParseRoleARN extracts account id and role name from an IAM role ARN.
func ParseRoleARN(roleARN string) (accountID, roleName string, ok bool) {
	const prefix = "arn:aws:iam::"
	if len(roleARN) < len(prefix)+15 {
		return "", "", false
	}
	if roleARN[:len(prefix)] != prefix {
		return "", "", false
	}
	rest := roleARN[len(prefix):]
	// ACCOUNT:role/NAME
	var account string
	for i := 0; i < len(rest); i++ {
		if rest[i] == ':' {
			account = rest[:i]
			rest = rest[i+1:]
			break
		}
	}
	if len(account) != 12 || len(rest) < 6 || rest[:5] != "role/" {
		return "", "", false
	}
	return account, rest[5:], true
}
