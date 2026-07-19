package sts

import (
	"encoding/xml"
	"fmt"
)

type decodeAuthorizationMessageResponse struct {
	XMLName                          xml.Name `xml:"DecodeAuthorizationMessageResponse"`
	XMLNS                            string   `xml:"xmlns,attr"`
	DecodeAuthorizationMessageResult struct {
		DecodedMessage string `xml:"DecodedMessage"`
	} `xml:"DecodeAuthorizationMessageResult"`
	ResponseMetadata struct {
		RequestId string `xml:"RequestId"`
	} `xml:"ResponseMetadata"`
}

// DecodeAuthorizationMessageXML builds an AWS CLI-compatible DecodeAuthorizationMessage XML body.
func DecodeAuthorizationMessageXML(decodedMessage, requestID string) ([]byte, error) {
	resp := decodeAuthorizationMessageResponse{XMLNS: stsXMLNS}
	resp.DecodeAuthorizationMessageResult.DecodedMessage = decodedMessage
	resp.ResponseMetadata.RequestId = requestID

	out, err := xml.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("marshal DecodeAuthorizationMessage response: %w", err)
	}
	return out, nil
}
