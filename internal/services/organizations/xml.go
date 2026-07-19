package organizations

import (
	"encoding/xml"
	"fmt"
)

const orgsXMLNS = "https://organizations.amazonaws.com/documents/2016-11-28/Organization_v1/"

// CreateAccountResult holds fields for CreateAccount XML.
type CreateAccountResult struct {
	RequestID   string
	CreateID    string // CreateAccountRequestId
	AccountName string
	State       string
}

type createAccountResponse struct {
	XMLName             xml.Name `xml:"CreateAccountResponse"`
	XMLNS               string   `xml:"xmlns,attr"`
	CreateAccountResult struct {
		CreateAccountStatus struct {
			Id          string `xml:"Id"`
			AccountName string `xml:"AccountName"`
			State       string `xml:"State"`
		} `xml:"CreateAccountStatus"`
	} `xml:"CreateAccountResult"`
	ResponseMetadata struct {
		RequestId string `xml:"RequestId"`
	} `xml:"ResponseMetadata"`
}

// CreateAccountXML builds Organizations CreateAccount response XML.
func CreateAccountXML(r CreateAccountResult) ([]byte, error) {
	resp := createAccountResponse{XMLNS: orgsXMLNS}
	resp.CreateAccountResult.CreateAccountStatus.Id = r.CreateID
	resp.CreateAccountResult.CreateAccountStatus.AccountName = r.AccountName
	resp.CreateAccountResult.CreateAccountStatus.State = r.State
	resp.ResponseMetadata.RequestId = r.RequestID
	out, err := xml.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("marshal CreateAccount: %w", err)
	}
	return out, nil
}

// DescribeCreateAccountStatusResult holds DescribeCreateAccountStatus fields.
type DescribeCreateAccountStatusResult struct {
	RequestID   string
	CreateID    string
	AccountName string
	State       string
	AccountID   string
	FailureReason string
}

type describeCreateAccountStatusResponse struct {
	XMLName                              xml.Name `xml:"DescribeCreateAccountStatusResponse"`
	XMLNS                                string   `xml:"xmlns,attr"`
	DescribeCreateAccountStatusResult    struct {
		CreateAccountStatus struct {
			Id            string `xml:"Id"`
			AccountName   string `xml:"AccountName"`
			State         string `xml:"State"`
			AccountId     string `xml:"AccountId,omitempty"`
			FailureReason string `xml:"FailureReason,omitempty"`
		} `xml:"CreateAccountStatus"`
	} `xml:"DescribeCreateAccountStatusResult"`
	ResponseMetadata struct {
		RequestId string `xml:"RequestId"`
	} `xml:"ResponseMetadata"`
}

// DescribeCreateAccountStatusXML builds DescribeCreateAccountStatus response XML.
func DescribeCreateAccountStatusXML(r DescribeCreateAccountStatusResult) ([]byte, error) {
	resp := describeCreateAccountStatusResponse{XMLNS: orgsXMLNS}
	resp.DescribeCreateAccountStatusResult.CreateAccountStatus.Id = r.CreateID
	resp.DescribeCreateAccountStatusResult.CreateAccountStatus.AccountName = r.AccountName
	resp.DescribeCreateAccountStatusResult.CreateAccountStatus.State = r.State
	resp.DescribeCreateAccountStatusResult.CreateAccountStatus.AccountId = r.AccountID
	resp.DescribeCreateAccountStatusResult.CreateAccountStatus.FailureReason = r.FailureReason
	resp.ResponseMetadata.RequestId = r.RequestID
	out, err := xml.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("marshal DescribeCreateAccountStatus: %w", err)
	}
	return out, nil
}
