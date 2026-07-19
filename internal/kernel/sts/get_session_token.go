package sts

import (
	"encoding/xml"
	"fmt"
	"time"
)

// GetSessionTokenResult holds fields for a GetSessionToken XML response.
type GetSessionTokenResult struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	Expiration      time.Time
	RequestID       string
}

type getSessionTokenResponse struct {
	XMLName               xml.Name `xml:"GetSessionTokenResponse"`
	XMLNS                 string   `xml:"xmlns,attr"`
	GetSessionTokenResult struct {
		Credentials struct {
			AccessKeyId     string `xml:"AccessKeyId"`
			SecretAccessKey string `xml:"SecretAccessKey"`
			SessionToken    string `xml:"SessionToken"`
			Expiration      string `xml:"Expiration"`
		} `xml:"Credentials"`
	} `xml:"GetSessionTokenResult"`
	ResponseMetadata struct {
		RequestId string `xml:"RequestId"`
	} `xml:"ResponseMetadata"`
}

// GetSessionTokenXML builds an AWS CLI-compatible GetSessionToken XML body.
func GetSessionTokenXML(r GetSessionTokenResult) ([]byte, error) {
	resp := getSessionTokenResponse{XMLNS: stsXMLNS}
	resp.GetSessionTokenResult.Credentials.AccessKeyId = r.AccessKeyID
	resp.GetSessionTokenResult.Credentials.SecretAccessKey = r.SecretAccessKey
	resp.GetSessionTokenResult.Credentials.SessionToken = r.SessionToken
	resp.GetSessionTokenResult.Credentials.Expiration = r.Expiration.UTC().Format(time.RFC3339)
	resp.ResponseMetadata.RequestId = r.RequestID

	out, err := xml.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("marshal GetSessionToken response: %w", err)
	}
	return out, nil
}
