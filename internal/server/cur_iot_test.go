package server_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestCURPutDescribeDelete(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC()

	createBucket := mustJSONTarget(t, handler, "S3.CreateBucket", "s3", map[string]any{
		"Bucket": "cur-billing",
	}, now)
	_ = createBucket

	put := mustJSONTarget(t, handler, "AWSOrigamiServiceGatewayService.PutReportDefinition", "cur", map[string]any{
		"ReportDefinition": map[string]any{
			"ReportName":               "lab-cur",
			"TimeUnit":                 "MONTHLY",
			"Format":                   "textORcsv",
			"Compression":              "GZIP",
			"S3Bucket":                 "cur-billing",
			"S3Prefix":                 "reports",
			"S3Region":                 "us-east-1",
			"AdditionalSchemaElements": []string{"RESOURCES"},
			"ReportVersioning":         "OVERWRITE_REPORT",
		},
	}, now)
	if put.Code != http.StatusOK {
		t.Fatalf("PutReportDefinition status=%d body=%s", put.Code, put.Body.String())
	}

	desc := mustJSONTarget(t, handler, "AWSOrigamiServiceGatewayService.DescribeReportDefinitions", "cur", map[string]any{}, now)
	if desc.Code != http.StatusOK {
		t.Fatalf("DescribeReportDefinitions status=%d body=%s", desc.Code, desc.Body.String())
	}
	var parsed map[string]any
	if err := json.Unmarshal(desc.Body.Bytes(), &parsed); err != nil {
		t.Fatal(err)
	}
	defs, _ := parsed["ReportDefinitions"].([]any)
	if len(defs) < 1 {
		t.Fatalf("expected report definitions: %s", desc.Body.String())
	}

	del := mustJSONTarget(t, handler, "AWSOrigamiServiceGatewayService.DeleteReportDefinition", "cur", map[string]any{
		"ReportName": "lab-cur",
	}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteReportDefinition status=%d body=%s", del.Code, del.Body.String())
	}
}

func TestIoTThingCertShadowHandlers(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC()
	prefix := "lab-iot-" + now.Format("150405")

	create := mustJSONTarget(t, handler, "AWSIotService.CreateThing", "iot", map[string]any{
		"thingName": prefix,
		"attributePayload": map[string]any{
			"attributes": map[string]string{"env": "lab"},
		},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateThing status=%d body=%s", create.Code, create.Body.String())
	}

	cert := mustJSONTarget(t, handler, "AWSIotService.CreateKeysAndCertificate", "iot", map[string]any{
		"setAsActive": true,
	}, now)
	if cert.Code != http.StatusOK {
		t.Fatalf("CreateKeysAndCertificate status=%d body=%s", cert.Code, cert.Body.String())
	}
	var certParsed map[string]any
	if err := json.Unmarshal(cert.Body.Bytes(), &certParsed); err != nil {
		t.Fatal(err)
	}
	certARN, _ := certParsed["certificateArn"].(string)
	certID, _ := certParsed["certificateId"].(string)
	if certARN == "" || certID == "" {
		t.Fatalf("missing cert fields: %s", cert.Body.String())
	}

	polDoc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"iot:*","Resource":"*"}]}`
	pol := mustJSONTarget(t, handler, "AWSIotService.CreatePolicy", "iot", map[string]any{
		"policyName":     prefix + "-pol",
		"policyDocument": polDoc,
	}, now)
	if pol.Code != http.StatusOK {
		t.Fatalf("CreatePolicy status=%d body=%s", pol.Code, pol.Body.String())
	}

	attachPol := mustJSONTarget(t, handler, "AWSIotService.AttachPolicy", "iot", map[string]any{
		"policyName": prefix + "-pol",
		"target":     certARN,
	}, now)
	if attachPol.Code != http.StatusOK {
		t.Fatalf("AttachPolicy status=%d body=%s", attachPol.Code, attachPol.Body.String())
	}

	attachThing := mustJSONTarget(t, handler, "AWSIotService.AttachThingPrincipal", "iot", map[string]any{
		"thingName": prefix,
		"principal": certARN,
	}, now)
	if attachThing.Code != http.StatusOK {
		t.Fatalf("AttachThingPrincipal status=%d body=%s", attachThing.Code, attachThing.Body.String())
	}

	shadow := mustJSONTarget(t, handler, "AWSIotDataService.UpdateThingShadow", "iot-data", map[string]any{
		"thingName": prefix,
		"state": map[string]any{
			"desired": map[string]any{"color": "blue"},
		},
	}, now)
	if shadow.Code != http.StatusOK {
		t.Fatalf("UpdateThingShadow status=%d body=%s", shadow.Code, shadow.Body.String())
	}

	getShadow := mustJSONTarget(t, handler, "AWSIotDataService.GetThingShadow", "iot-data", map[string]any{
		"thingName": prefix,
	}, now)
	if getShadow.Code != http.StatusOK {
		t.Fatalf("GetThingShadow status=%d body=%s", getShadow.Code, getShadow.Body.String())
	}

	delShadow := mustJSONTarget(t, handler, "AWSIotDataService.DeleteThingShadow", "iot-data", map[string]any{
		"thingName": prefix,
	}, now)
	if delShadow.Code != http.StatusOK {
		t.Fatalf("DeleteThingShadow status=%d body=%s", delShadow.Code, delShadow.Body.String())
	}

	_ = mustJSONTarget(t, handler, "AWSIotService.DetachPolicy", "iot", map[string]any{
		"policyName": prefix + "-pol",
		"target":     certARN,
	}, now)
	_ = mustJSONTarget(t, handler, "AWSIotService.UpdateCertificate", "iot", map[string]any{
		"certificateId": certID,
		"newStatus":     "INACTIVE",
	}, now)
}
