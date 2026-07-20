package cloudformation

import (
	"encoding/xml"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const xmlns = "http://cloudformation.amazonaws.com/doc/2010-05-15/"

type responseMetadata struct {
	RequestID string `xml:"RequestId"`
}

func marshal(name string, result any, requestID string) ([]byte, error) {
	type envelope struct {
		XMLName  xml.Name `xml:""`
		Result   any      `xml:",any"`
		Metadata responseMetadata `xml:"ResponseMetadata"`
	}
	env := envelope{
		XMLName:  xml.Name{Local: name, Space: xmlns},
		Result:   result,
		Metadata: responseMetadata{RequestID: requestID},
	}
	data, err := xml.Marshal(env)
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), data...), nil
}

func rfc3339(ms int64) string {
	return time.UnixMilli(ms).UTC().Format(time.RFC3339)
}

// CreateStackXML builds a CreateStack response.
func CreateStackXML(stackID, requestID string) ([]byte, error) {
	type result struct {
		XMLName xml.Name `xml:"CreateStackResult"`
		StackID string   `xml:"StackId"`
	}
	return marshal("CreateStackResponse", result{StackID: stackID}, requestID)
}

// DeleteStackXML builds a DeleteStack response.
func DeleteStackXML(requestID string) ([]byte, error) {
	type result struct {
		XMLName xml.Name `xml:"DeleteStackResult"`
	}
	return marshal("DeleteStackResponse", result{}, requestID)
}

// DescribeStacksXML builds a DescribeStacks response.
func DescribeStacksXML(stacks []store.CFNStack, requestID string) ([]byte, error) {
	type member struct {
		StackID         string `xml:"StackId"`
		StackName       string `xml:"StackName"`
		StackStatus     string `xml:"StackStatus"`
		CreationTime    string `xml:"CreationTime"`
		LastUpdatedTime string `xml:"LastUpdatedTime,omitempty"`
		RoleARN         string `xml:"RoleARN,omitempty"`
	}
	type result struct {
		XMLName xml.Name `xml:"DescribeStacksResult"`
		Stacks  struct {
			Member []member `xml:"member"`
		} `xml:"Stacks"`
	}
	var r result
	for _, st := range stacks {
		m := member{
			StackID: st.StackID, StackName: st.StackName, StackStatus: st.Status,
			CreationTime: rfc3339(st.CreationTime),
		}
		if st.LastUpdated > 0 {
			m.LastUpdatedTime = rfc3339(st.LastUpdated)
		}
		if st.RoleARN != "" {
			m.RoleARN = st.RoleARN
		}
		r.Stacks.Member = append(r.Stacks.Member, m)
	}
	return marshal("DescribeStacksResponse", r, requestID)
}

// ListStacksXML builds a ListStacks response.
func ListStacksXML(stacks []store.CFNStack, requestID string) ([]byte, error) {
	type member struct {
		StackID         string `xml:"StackId"`
		StackName       string `xml:"StackName"`
		StackStatus     string `xml:"StackStatus"`
		CreationTime    string `xml:"CreationTime"`
		TemplateDescription string `xml:"TemplateDescription,omitempty"`
	}
	type result struct {
		XMLName       xml.Name `xml:"ListStacksResult"`
		StackSummaries struct {
			Member []member `xml:"member"`
		} `xml:"StackSummaries"`
	}
	var r result
	for _, st := range stacks {
		r.StackSummaries.Member = append(r.StackSummaries.Member, member{
			StackID: st.StackID, StackName: st.StackName, StackStatus: st.Status,
			CreationTime: rfc3339(st.CreationTime),
		})
	}
	return marshal("ListStacksResponse", r, requestID)
}
