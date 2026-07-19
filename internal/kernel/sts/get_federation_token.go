package sts

import (
	"encoding/xml"
	"fmt"
	"time"
)

// GetFederationTokenResult holds fields for a GetFederationToken XML response.
type GetFederationTokenResult struct {
	AccessKeyID      string
	SecretAccessKey  string
	SessionToken     string
	Expiration       time.Time
	FederatedUserARN string
	FederatedUserID  string
	PackedPolicySize int
	RequestID        string
}

type getFederationTokenResponse struct {
	XMLName                  xml.Name `xml:"GetFederationTokenResponse"`
	XMLNS                    string   `xml:"xmlns,attr"`
	GetFederationTokenResult struct {
		Credentials struct {
			AccessKeyId     string `xml:"AccessKeyId"`
			SecretAccessKey string `xml:"SecretAccessKey"`
			SessionToken    string `xml:"SessionToken"`
			Expiration      string `xml:"Expiration"`
		} `xml:"Credentials"`
		FederatedUser struct {
			Arn             string `xml:"Arn"`
			FederatedUserId string `xml:"FederatedUserId"`
		} `xml:"FederatedUser"`
		PackedPolicySize int `xml:"PackedPolicySize"`
	} `xml:"GetFederationTokenResult"`
	ResponseMetadata struct {
		RequestId string `xml:"RequestId"`
	} `xml:"ResponseMetadata"`
}

// GetFederationTokenXML builds an AWS CLI-compatible GetFederationToken XML body.
func GetFederationTokenXML(r GetFederationTokenResult) ([]byte, error) {
	resp := getFederationTokenResponse{XMLNS: stsXMLNS}
	resp.GetFederationTokenResult.Credentials.AccessKeyId = r.AccessKeyID
	resp.GetFederationTokenResult.Credentials.SecretAccessKey = r.SecretAccessKey
	resp.GetFederationTokenResult.Credentials.SessionToken = r.SessionToken
	resp.GetFederationTokenResult.Credentials.Expiration = r.Expiration.UTC().Format(time.RFC3339)
	resp.GetFederationTokenResult.FederatedUser.Arn = r.FederatedUserARN
	resp.GetFederationTokenResult.FederatedUser.FederatedUserId = r.FederatedUserID
	resp.GetFederationTokenResult.PackedPolicySize = r.PackedPolicySize
	resp.ResponseMetadata.RequestId = r.RequestID

	out, err := xml.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("marshal GetFederationToken response: %w", err)
	}
	return out, nil
}
