package sts

import (
	"encoding/xml"
	"fmt"
	"time"
)

// AssumeRootResult holds fields for an AssumeRoot XML response.
// Shape follows AWS STS AssumeRoot: Credentials plus optional SourceIdentity.
type AssumeRootResult struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	Expiration      time.Time
	SourceIdentity  string
	RequestID       string
}

type assumeRootResponse struct {
	XMLName          xml.Name `xml:"AssumeRootResponse"`
	XMLNS            string   `xml:"xmlns,attr"`
	AssumeRootResult struct {
		Credentials struct {
			AccessKeyId     string `xml:"AccessKeyId"`
			SecretAccessKey string `xml:"SecretAccessKey"`
			SessionToken    string `xml:"SessionToken"`
			Expiration      string `xml:"Expiration"`
		} `xml:"Credentials"`
		SourceIdentity string `xml:"SourceIdentity,omitempty"`
	} `xml:"AssumeRootResult"`
	ResponseMetadata struct {
		RequestId string `xml:"RequestId"`
	} `xml:"ResponseMetadata"`
}

// AssumeRootXML builds an AWS CLI-compatible AssumeRoot XML body.
func AssumeRootXML(r AssumeRootResult) ([]byte, error) {
	resp := assumeRootResponse{XMLNS: stsXMLNS}
	resp.AssumeRootResult.Credentials.AccessKeyId = r.AccessKeyID
	resp.AssumeRootResult.Credentials.SecretAccessKey = r.SecretAccessKey
	resp.AssumeRootResult.Credentials.SessionToken = r.SessionToken
	resp.AssumeRootResult.Credentials.Expiration = r.Expiration.UTC().Format(time.RFC3339)
	resp.AssumeRootResult.SourceIdentity = r.SourceIdentity
	resp.ResponseMetadata.RequestId = r.RequestID

	out, err := xml.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("marshal AssumeRoot response: %w", err)
	}
	return out, nil
}
