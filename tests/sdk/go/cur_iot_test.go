package sdk_test

import (
	"testing"
)

func TestCURPutDescribeDelete(t *testing.T) {
	requireReady(t)
	prefix := uniquePrefix(t)

	bucket := "cur-sdk-" + prefix
	// Best-effort: Put succeeds even if bucket missing (status ERROR).
	putStatus, putBody, putParsed := signedJSONTarget(t, "cur", "AWSOrigamiServiceGatewayService.PutReportDefinition", map[string]any{
		"ReportDefinition": map[string]any{
			"ReportName":               "sdk-" + prefix,
			"TimeUnit":                 "MONTHLY",
			"Format":                   "textORcsv",
			"Compression":              "GZIP",
			"S3Bucket":                 bucket,
			"S3Prefix":                 "reports",
			"S3Region":                 "us-east-1",
			"AdditionalSchemaElements": []string{"RESOURCES"},
			"ReportVersioning":         "OVERWRITE_REPORT",
		},
	})
	if putStatus != 200 {
		t.Fatalf("PutReportDefinition status=%d body=%s", putStatus, putBody)
	}
	if name, _ := putParsed["ReportName"].(string); name == "" {
		t.Fatalf("PutReportDefinition missing ReportName: %s", putBody)
	}

	descStatus, descBody, descParsed := signedJSONTarget(t, "cur", "AWSOrigamiServiceGatewayService.DescribeReportDefinitions", map[string]any{})
	if descStatus != 200 {
		t.Fatalf("DescribeReportDefinitions status=%d body=%s", descStatus, descBody)
	}
	defs, _ := descParsed["ReportDefinitions"].([]any)
	if len(defs) < 1 {
		t.Fatalf("DescribeReportDefinitions empty: %s", descBody)
	}

	delStatus, delBody, _ := signedJSONTarget(t, "cur", "AWSOrigamiServiceGatewayService.DeleteReportDefinition", map[string]any{
		"ReportName": "sdk-" + prefix,
	})
	if delStatus != 200 {
		t.Fatalf("DeleteReportDefinition status=%d body=%s", delStatus, delBody)
	}
}

func TestIoTThingCertShadow(t *testing.T) {
	requireReady(t)
	prefix := "sdk-iot-" + uniquePrefix(t)

	createStatus, createBody, _ := signedJSONTarget(t, "iot", "AWSIotService.CreateThing", map[string]any{
		"thingName": prefix,
		"attributePayload": map[string]any{
			"attributes": map[string]string{"env": "sdk"},
		},
	})
	if createStatus != 200 {
		t.Fatalf("CreateThing status=%d body=%s", createStatus, createBody)
	}

	certStatus, certBody, certParsed := signedJSONTarget(t, "iot", "AWSIotService.CreateKeysAndCertificate", map[string]any{
		"setAsActive": true,
	})
	if certStatus != 200 {
		t.Fatalf("CreateKeysAndCertificate status=%d body=%s", certStatus, certBody)
	}
	certARN, _ := certParsed["certificateArn"].(string)
	if certARN == "" {
		t.Fatalf("missing certificateArn: %s", certBody)
	}

	polStatus, polBody, _ := signedJSONTarget(t, "iot", "AWSIotService.CreatePolicy", map[string]any{
		"policyName":     prefix + "-pol",
		"policyDocument": `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"iot:*","Resource":"*"}]}`,
	})
	if polStatus != 200 {
		t.Fatalf("CreatePolicy status=%d body=%s", polStatus, polBody)
	}

	attachStatus, attachBody, _ := signedJSONTarget(t, "iot", "AWSIotService.AttachThingPrincipal", map[string]any{
		"thingName": prefix,
		"principal": certARN,
	})
	if attachStatus != 200 {
		t.Fatalf("AttachThingPrincipal status=%d body=%s", attachStatus, attachBody)
	}

	shadowStatus, shadowBody, _ := signedJSONTarget(t, "iot-data", "AWSIotDataService.UpdateThingShadow", map[string]any{
		"thingName": prefix,
		"state": map[string]any{
			"desired": map[string]any{"color": "green"},
		},
	})
	if shadowStatus != 200 {
		t.Fatalf("UpdateThingShadow status=%d body=%s", shadowStatus, shadowBody)
	}

	getStatus, getBody, getParsed := signedJSONTarget(t, "iot-data", "AWSIotDataService.GetThingShadow", map[string]any{
		"thingName": prefix,
	})
	if getStatus != 200 {
		t.Fatalf("GetThingShadow status=%d body=%s", getStatus, getBody)
	}
	state, _ := getParsed["state"].(map[string]any)
	if state == nil {
		t.Fatalf("GetThingShadow missing state: %s", getBody)
	}
}
