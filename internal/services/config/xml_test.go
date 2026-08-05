package configsvc

import (
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestGetResourceConfigHistoryXML(t *testing.T) {
	items := []store.ConfigConfigurationItem{{
		ConfigurationItemVersion:     "1.3",
		ConfigurationItemCaptureTime: "2024-06-01T12:00:00Z",
		ConfigurationItemStatus:      store.ConfigItemStatusOK,
		ResourceType:                 "AWS::S3::Bucket",
		ResourceID:                   "lab-bucket",
		ResourceName:                 "lab-bucket",
		AWSAccountID:                 "000000000001",
		Configuration:                `{"name":"lab-bucket"}`,
	}}
	out, err := GetResourceConfigHistoryXML(items, "req-1")
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{
		"GetResourceConfigHistoryResult",
		"<resourceId>lab-bucket</resourceId>",
		"<configurationItemStatus>OK</configurationItemStatus>",
		"<awsAccountId>000000000001</awsAccountId>",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in %q", want, s)
		}
	}
}

func TestConfigServiceXML(t *testing.T) {
	req := "req-config"
	for _, fn := range []func(string) ([]byte, error){
		PutConfigurationRecorderXML, PutDeliveryChannelXML, StartConfigurationRecorderXML,
	} {
		out, err := fn(req)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(out), req) {
			t.Fatalf("missing request id: %s", out)
		}
	}

	compliance := []store.ConfigComplianceResult{{
		ConfigRuleName: "s3-bucket-public-read-prohibited",
		ComplianceType: "COMPLIANT",
	}}
	cOut, err := DescribeComplianceByConfigRuleXML(compliance, req)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cOut), "COMPLIANT") {
		t.Fatalf("compliance=%s", cOut)
	}
}
