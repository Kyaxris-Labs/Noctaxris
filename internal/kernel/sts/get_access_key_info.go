package sts

import (
	"encoding/xml"
	"fmt"
)

type getAccessKeyInfoResponse struct {
	XMLName                xml.Name `xml:"GetAccessKeyInfoResponse"`
	XMLNS                  string   `xml:"xmlns,attr"`
	GetAccessKeyInfoResult struct {
		Account string `xml:"Account"`
	} `xml:"GetAccessKeyInfoResult"`
	ResponseMetadata struct {
		RequestId string `xml:"RequestId"`
	} `xml:"ResponseMetadata"`
}

// GetAccessKeyInfoXML builds an AWS CLI-compatible GetAccessKeyInfo XML body.
func GetAccessKeyInfoXML(accountID, requestID string) ([]byte, error) {
	resp := getAccessKeyInfoResponse{XMLNS: stsXMLNS}
	resp.GetAccessKeyInfoResult.Account = accountID
	resp.ResponseMetadata.RequestId = requestID

	out, err := xml.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("marshal GetAccessKeyInfo response: %w", err)
	}
	return out, nil
}
