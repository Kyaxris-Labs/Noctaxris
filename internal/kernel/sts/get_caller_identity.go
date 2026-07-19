package sts

import (
	"encoding/xml"
	"fmt"
)

const stsXMLNS = "https://sts.amazonaws.com/doc/2011-06-15/"

// RootCallerIDs returns the UserId and ARN for the account root principal.
// For root callers, UserId is the account ID.
func RootCallerIDs(accountID string) (userID, arn string) {
	return accountID, fmt.Sprintf("arn:aws:iam::%s:root", accountID)
}

type getCallerIdentityResponse struct {
	XMLName                 xml.Name `xml:"GetCallerIdentityResponse"`
	XMLNS                   string   `xml:"xmlns,attr"`
	GetCallerIdentityResult struct {
		Arn     string `xml:"Arn"`
		UserId  string `xml:"UserId"`
		Account string `xml:"Account"`
	} `xml:"GetCallerIdentityResult"`
	ResponseMetadata struct {
		RequestId string `xml:"RequestId"`
	} `xml:"ResponseMetadata"`
}

// GetCallerIdentityXML builds an AWS CLI-compatible GetCallerIdentity XML body.
func GetCallerIdentityXML(accountID, userID, arn, requestID string) ([]byte, error) {
	resp := getCallerIdentityResponse{
		XMLNS: stsXMLNS,
	}
	resp.GetCallerIdentityResult.Arn = arn
	resp.GetCallerIdentityResult.UserId = userID
	resp.GetCallerIdentityResult.Account = accountID
	resp.ResponseMetadata.RequestId = requestID

	out, err := xml.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("marshal GetCallerIdentity response: %w", err)
	}
	return out, nil
}
