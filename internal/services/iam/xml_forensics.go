package iam

import (
	"encoding/base64"
	"encoding/xml"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

type accessKeyLastUsedXML struct {
	LastUsedDate string `xml:"LastUsedDate,omitempty"`
	ServiceName  string `xml:"ServiceName,omitempty"`
	Region       string `xml:"Region,omitempty"`
}

type getAccessKeyLastUsedResponse struct {
	XMLName                   xml.Name `xml:"GetAccessKeyLastUsedResponse"`
	XMLNS                     string   `xml:"xmlns,attr"`
	GetAccessKeyLastUsedResult struct {
		UserName          string               `xml:"UserName,omitempty"`
		AccessKeyLastUsed accessKeyLastUsedXML `xml:"AccessKeyLastUsed"`
	} `xml:"GetAccessKeyLastUsedResult"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// GetAccessKeyLastUsedXML builds GetAccessKeyLastUsed response XML.
func GetAccessKeyLastUsedXML(u store.AccessKeyLastUsed, requestID string) ([]byte, error) {
	resp := getAccessKeyLastUsedResponse{XMLNS: iamXMLNS}
	resp.GetAccessKeyLastUsedResult.UserName = u.UserName
	if u.HasLastUsed {
		resp.GetAccessKeyLastUsedResult.AccessKeyLastUsed = accessKeyLastUsedXML{
			LastUsedDate: u.LastUsedDate.Format(time.RFC3339),
			ServiceName:  u.ServiceName,
			Region:       u.Region,
		}
	}
	resp.ResponseMetadata.RequestId = requestID
	return marshalResponse(resp)
}

type generateCredentialReportResponse struct {
	XMLName                        xml.Name `xml:"GenerateCredentialReportResponse"`
	XMLNS                          string   `xml:"xmlns,attr"`
	GenerateCredentialReportResult struct {
		State       string `xml:"State"`
		Description string `xml:"Description,omitempty"`
	} `xml:"GenerateCredentialReportResult"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// GenerateCredentialReportXML builds GenerateCredentialReport response XML.
func GenerateCredentialReportXML(state, requestID string) ([]byte, error) {
	resp := generateCredentialReportResponse{XMLNS: iamXMLNS}
	resp.GenerateCredentialReportResult.State = state
	resp.GenerateCredentialReportResult.Description = "No report exists. Starting a new report generation task"
	resp.ResponseMetadata.RequestId = requestID
	return marshalResponse(resp)
}

type getCredentialReportResponse struct {
	XMLName                   xml.Name `xml:"GetCredentialReportResponse"`
	XMLNS                     string   `xml:"xmlns,attr"`
	GetCredentialReportResult struct {
		Content       string `xml:"Content"`
		GeneratedTime string `xml:"GeneratedTime"`
		ReportFormat  string `xml:"ReportFormat"`
		State         string `xml:"State"`
	} `xml:"GetCredentialReportResult"`
	ResponseMetadata responseMetadata `xml:"ResponseMetadata"`
}

// GetCredentialReportXML builds GetCredentialReport response XML.
func GetCredentialReportXML(csv []byte, generated time.Time, state, requestID string) ([]byte, error) {
	resp := getCredentialReportResponse{XMLNS: iamXMLNS}
	resp.GetCredentialReportResult.Content = base64.StdEncoding.EncodeToString(csv)
	resp.GetCredentialReportResult.GeneratedTime = generated.UTC().Format(time.RFC3339)
	resp.GetCredentialReportResult.ReportFormat = "text/csv"
	resp.GetCredentialReportResult.State = state
	resp.ResponseMetadata.RequestId = requestID
	return marshalResponse(resp)
}
