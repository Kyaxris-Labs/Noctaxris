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

// ListAccountsResult holds ListAccounts XML fields.
type ListAccountsResult struct {
	RequestID  string
	AccountIDs []string
}

type listAccountsResponse struct {
	XMLName           xml.Name `xml:"ListAccountsResponse"`
	XMLNS             string   `xml:"xmlns,attr"`
	ListAccountsResult struct {
		Accounts struct {
			Member []struct {
				Id     string `xml:"Id"`
				Status string `xml:"Status"`
			} `xml:"member"`
		} `xml:"Accounts"`
	} `xml:"ListAccountsResult"`
	ResponseMetadata struct {
		RequestId string `xml:"RequestId"`
	} `xml:"ResponseMetadata"`
}

// ListAccountsXML builds ListAccounts response XML.
func ListAccountsXML(r ListAccountsResult) ([]byte, error) {
	resp := listAccountsResponse{XMLNS: orgsXMLNS}
	for _, id := range r.AccountIDs {
		resp.ListAccountsResult.Accounts.Member = append(resp.ListAccountsResult.Accounts.Member, struct {
			Id     string `xml:"Id"`
			Status string `xml:"Status"`
		}{Id: id, Status: "ACTIVE"})
	}
	resp.ResponseMetadata.RequestId = r.RequestID
	out, err := xml.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("marshal ListAccounts: %w", err)
	}
	return out, nil
}

// CreateOrganizationalUnitResult holds CreateOrganizationalUnit XML fields.
type CreateOrganizationalUnitResult struct {
	RequestID string
	ID        string
	Name      string
	Arn       string
}

type createOrganizationalUnitResponse struct {
	XMLName                         xml.Name `xml:"CreateOrganizationalUnitResponse"`
	XMLNS                           string   `xml:"xmlns,attr"`
	CreateOrganizationalUnitResult  struct {
		OrganizationalUnit struct {
			Id   string `xml:"Id"`
			Name string `xml:"Name"`
			Arn  string `xml:"Arn"`
		} `xml:"OrganizationalUnit"`
	} `xml:"CreateOrganizationalUnitResult"`
	ResponseMetadata struct {
		RequestId string `xml:"RequestId"`
	} `xml:"ResponseMetadata"`
}

// CreateOrganizationalUnitXML builds CreateOrganizationalUnit response XML.
func CreateOrganizationalUnitXML(r CreateOrganizationalUnitResult) ([]byte, error) {
	resp := createOrganizationalUnitResponse{XMLNS: orgsXMLNS}
	resp.CreateOrganizationalUnitResult.OrganizationalUnit.Id = r.ID
	resp.CreateOrganizationalUnitResult.OrganizationalUnit.Name = r.Name
	resp.CreateOrganizationalUnitResult.OrganizationalUnit.Arn = r.Arn
	resp.ResponseMetadata.RequestId = r.RequestID
	out, err := xml.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("marshal CreateOrganizationalUnit: %w", err)
	}
	return out, nil
}

// ListOrganizationalUnitsForParentResult holds list OU XML fields.
type ListOrganizationalUnitsForParentResult struct {
	RequestID string
	Units     []CreateOrganizationalUnitResult
}

type listOrganizationalUnitsForParentResponse struct {
	XMLName                                  xml.Name `xml:"ListOrganizationalUnitsForParentResponse"`
	XMLNS                                    string   `xml:"xmlns,attr"`
	ListOrganizationalUnitsForParentResult   struct {
		OrganizationalUnits struct {
			Member []struct {
				Id   string `xml:"Id"`
				Name string `xml:"Name"`
				Arn  string `xml:"Arn"`
			} `xml:"member"`
		} `xml:"OrganizationalUnits"`
	} `xml:"ListOrganizationalUnitsForParentResult"`
	ResponseMetadata struct {
		RequestId string `xml:"RequestId"`
	} `xml:"ResponseMetadata"`
}

// ListOrganizationalUnitsForParentXML builds ListOrganizationalUnitsForParent response XML.
func ListOrganizationalUnitsForParentXML(r ListOrganizationalUnitsForParentResult) ([]byte, error) {
	resp := listOrganizationalUnitsForParentResponse{XMLNS: orgsXMLNS}
	for _, u := range r.Units {
		resp.ListOrganizationalUnitsForParentResult.OrganizationalUnits.Member = append(
			resp.ListOrganizationalUnitsForParentResult.OrganizationalUnits.Member,
			struct {
				Id   string `xml:"Id"`
				Name string `xml:"Name"`
				Arn  string `xml:"Arn"`
			}{Id: u.ID, Name: u.Name, Arn: u.Arn},
		)
	}
	resp.ResponseMetadata.RequestId = r.RequestID
	out, err := xml.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("marshal ListOrganizationalUnitsForParent: %w", err)
	}
	return out, nil
}

// EnablePolicyTypeResult holds EnablePolicyType XML fields.
type EnablePolicyTypeResult struct {
	RequestID    string
	RootID       string
	RootArn      string
	RootName     string
	PolicyTypes  []string // API names e.g. SERVICE_CONTROL_POLICY
}

type enablePolicyTypeResponse struct {
	XMLName               xml.Name `xml:"EnablePolicyTypeResponse"`
	XMLNS                 string   `xml:"xmlns,attr"`
	EnablePolicyTypeResult struct {
		Root struct {
			Id          string `xml:"Id"`
			Arn         string `xml:"Arn"`
			Name        string `xml:"Name"`
			PolicyTypes struct {
				Member []struct {
					Type   string `xml:"Type"`
					Status string `xml:"Status"`
				} `xml:"member"`
			} `xml:"PolicyTypes"`
		} `xml:"Root"`
	} `xml:"EnablePolicyTypeResult"`
	ResponseMetadata struct {
		RequestId string `xml:"RequestId"`
	} `xml:"ResponseMetadata"`
}

// EnablePolicyTypeXML builds EnablePolicyType response XML.
func EnablePolicyTypeXML(r EnablePolicyTypeResult) ([]byte, error) {
	resp := enablePolicyTypeResponse{XMLNS: orgsXMLNS}
	resp.EnablePolicyTypeResult.Root.Id = r.RootID
	resp.EnablePolicyTypeResult.Root.Arn = r.RootArn
	resp.EnablePolicyTypeResult.Root.Name = r.RootName
	for _, t := range r.PolicyTypes {
		resp.EnablePolicyTypeResult.Root.PolicyTypes.Member = append(
			resp.EnablePolicyTypeResult.Root.PolicyTypes.Member,
			struct {
				Type   string `xml:"Type"`
				Status string `xml:"Status"`
			}{Type: t, Status: "ENABLED"},
		)
	}
	resp.ResponseMetadata.RequestId = r.RequestID
	out, err := xml.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("marshal EnablePolicyType: %w", err)
	}
	return out, nil
}

// OrgPolicyResult holds CreatePolicy / DescribePolicy XML fields.
type OrgPolicyResult struct {
	RequestID   string
	ID          string
	Name        string
	Type        string // SERVICE_CONTROL_POLICY or RESOURCE_CONTROL_POLICY
	Arn         string
	Content     string
	Description string
}

type createPolicyResponse struct {
	XMLName            xml.Name `xml:"CreatePolicyResponse"`
	XMLNS              string   `xml:"xmlns,attr"`
	CreatePolicyResult struct {
		Policy struct {
			Content       string `xml:"Content"`
			PolicySummary struct {
				Id          string `xml:"Id"`
				Arn         string `xml:"Arn"`
				Name        string `xml:"Name"`
				Type        string `xml:"Type"`
				Description string `xml:"Description"`
				AwsManaged  bool   `xml:"AwsManaged"`
			} `xml:"PolicySummary"`
		} `xml:"Policy"`
	} `xml:"CreatePolicyResult"`
	ResponseMetadata struct {
		RequestId string `xml:"RequestId"`
	} `xml:"ResponseMetadata"`
}

// CreatePolicyXML builds Organizations CreatePolicy response XML.
func CreatePolicyXML(r OrgPolicyResult) ([]byte, error) {
	resp := createPolicyResponse{XMLNS: orgsXMLNS}
	resp.CreatePolicyResult.Policy.Content = r.Content
	resp.CreatePolicyResult.Policy.PolicySummary.Id = r.ID
	resp.CreatePolicyResult.Policy.PolicySummary.Arn = r.Arn
	resp.CreatePolicyResult.Policy.PolicySummary.Name = r.Name
	resp.CreatePolicyResult.Policy.PolicySummary.Type = r.Type
	resp.CreatePolicyResult.Policy.PolicySummary.Description = r.Description
	resp.ResponseMetadata.RequestId = r.RequestID
	out, err := xml.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("marshal CreatePolicy: %w", err)
	}
	return out, nil
}

type describePolicyResponse struct {
	XMLName              xml.Name `xml:"DescribePolicyResponse"`
	XMLNS                string   `xml:"xmlns,attr"`
	DescribePolicyResult struct {
		Policy struct {
			Content       string `xml:"Content"`
			PolicySummary struct {
				Id          string `xml:"Id"`
				Arn         string `xml:"Arn"`
				Name        string `xml:"Name"`
				Type        string `xml:"Type"`
				Description string `xml:"Description"`
				AwsManaged  bool   `xml:"AwsManaged"`
			} `xml:"PolicySummary"`
		} `xml:"Policy"`
	} `xml:"DescribePolicyResult"`
	ResponseMetadata struct {
		RequestId string `xml:"RequestId"`
	} `xml:"ResponseMetadata"`
}

// DescribePolicyXML builds DescribePolicy response XML.
func DescribePolicyXML(r OrgPolicyResult) ([]byte, error) {
	resp := describePolicyResponse{XMLNS: orgsXMLNS}
	resp.DescribePolicyResult.Policy.Content = r.Content
	resp.DescribePolicyResult.Policy.PolicySummary.Id = r.ID
	resp.DescribePolicyResult.Policy.PolicySummary.Arn = r.Arn
	resp.DescribePolicyResult.Policy.PolicySummary.Name = r.Name
	resp.DescribePolicyResult.Policy.PolicySummary.Type = r.Type
	resp.DescribePolicyResult.Policy.PolicySummary.Description = r.Description
	resp.ResponseMetadata.RequestId = r.RequestID
	out, err := xml.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("marshal DescribePolicy: %w", err)
	}
	return out, nil
}

// AttachPolicyXML builds AttachPolicy response XML.
func AttachPolicyXML(requestID string) ([]byte, error) {
	type resp struct {
		XMLName          xml.Name `xml:"AttachPolicyResponse"`
		XMLNS            string   `xml:"xmlns,attr"`
		AttachPolicyResult struct{} `xml:"AttachPolicyResult"`
		ResponseMetadata struct {
			RequestId string `xml:"RequestId"`
		} `xml:"ResponseMetadata"`
	}
	out, err := xml.Marshal(resp{XMLNS: orgsXMLNS, ResponseMetadata: struct {
		RequestId string `xml:"RequestId"`
	}{RequestId: requestID}})
	if err != nil {
		return nil, fmt.Errorf("marshal AttachPolicy: %w", err)
	}
	return out, nil
}

// DetachPolicyXML builds DetachPolicy response XML.
func DetachPolicyXML(requestID string) ([]byte, error) {
	type resp struct {
		XMLName          xml.Name `xml:"DetachPolicyResponse"`
		XMLNS            string   `xml:"xmlns,attr"`
		DetachPolicyResult struct{} `xml:"DetachPolicyResult"`
		ResponseMetadata struct {
			RequestId string `xml:"RequestId"`
		} `xml:"ResponseMetadata"`
	}
	out, err := xml.Marshal(resp{XMLNS: orgsXMLNS, ResponseMetadata: struct {
		RequestId string `xml:"RequestId"`
	}{RequestId: requestID}})
	if err != nil {
		return nil, fmt.Errorf("marshal DetachPolicy: %w", err)
	}
	return out, nil
}

// MoveAccountXML builds MoveAccount response XML.
func MoveAccountXML(requestID string) ([]byte, error) {
	type resp struct {
		XMLName           xml.Name `xml:"MoveAccountResponse"`
		XMLNS             string   `xml:"xmlns,attr"`
		MoveAccountResult struct{} `xml:"MoveAccountResult"`
		ResponseMetadata  struct {
			RequestId string `xml:"RequestId"`
		} `xml:"ResponseMetadata"`
	}
	out, err := xml.Marshal(resp{XMLNS: orgsXMLNS, ResponseMetadata: struct {
		RequestId string `xml:"RequestId"`
	}{RequestId: requestID}})
	if err != nil {
		return nil, fmt.Errorf("marshal MoveAccount: %w", err)
	}
	return out, nil
}
