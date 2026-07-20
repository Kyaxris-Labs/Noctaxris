package configsvc

import (
	"encoding/xml"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

const xmlns = "http://config.amazonaws.com/doc/2014-11-12/"

type responseMetadata struct {
	RequestID string `xml:"RequestId"`
}

func marshal(name string, result any, requestID string) ([]byte, error) {
	type envelope struct {
		XMLName  xml.Name          `xml:""`
		Result   any               `xml:",any"`
		Metadata responseMetadata  `xml:"ResponseMetadata"`
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

// PutConfigurationRecorderXML builds a PutConfigurationRecorder response.
func PutConfigurationRecorderXML(requestID string) ([]byte, error) {
	type result struct {
		XMLName xml.Name `xml:"PutConfigurationRecorderResult"`
	}
	return marshal("PutConfigurationRecorderResponse", result{}, requestID)
}

// PutDeliveryChannelXML builds a PutDeliveryChannel response.
func PutDeliveryChannelXML(requestID string) ([]byte, error) {
	type result struct {
		XMLName xml.Name `xml:"PutDeliveryChannelResult"`
	}
	return marshal("PutDeliveryChannelResponse", result{}, requestID)
}

// StartConfigurationRecorderXML builds a StartConfigurationRecorder response.
func StartConfigurationRecorderXML(requestID string) ([]byte, error) {
	type result struct {
		XMLName xml.Name `xml:"StartConfigurationRecorderResult"`
	}
	return marshal("StartConfigurationRecorderResponse", result{}, requestID)
}

// DescribeComplianceByConfigRuleXML builds a DescribeComplianceByConfigRule response.
func DescribeComplianceByConfigRuleXML(results []store.ConfigComplianceResult, requestID string) ([]byte, error) {
	type compliance struct {
		ComplianceType         string `xml:"ComplianceType"`
		ComplianceContributorCount struct {
			CappedCount int  `xml:"CappedCount"`
			CapExceeded bool `xml:"CapExceeded"`
		} `xml:"ComplianceContributorCount"`
	}
	type member struct {
		ConfigRuleName string     `xml:"ConfigRuleName"`
		Compliance     compliance `xml:"Compliance"`
	}
	type result struct {
		XMLName               xml.Name `xml:"DescribeComplianceByConfigRuleResult"`
		ComplianceByConfigRules struct {
			Member []member `xml:"member"`
		} `xml:"ComplianceByConfigRules"`
	}
	var r result
	byRule := map[string]string{}
	for _, res := range results {
		byRule[res.ConfigRuleName] = res.ComplianceType
	}
	for name, ctype := range byRule {
		m := member{ConfigRuleName: name}
		m.Compliance.ComplianceType = ctype
		m.Compliance.ComplianceContributorCount.CappedCount = len(results)
		r.ComplianceByConfigRules.Member = append(r.ComplianceByConfigRules.Member, m)
	}
	return marshal("DescribeComplianceByConfigRuleResponse", r, requestID)
}
