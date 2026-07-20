package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestACMRequestListDelete(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	req := mustJSONTarget(t, handler, "CertificateManager.RequestCertificate", "acm", map[string]any{
		"DomainName": "srv.example.com",
	}, now)
	if req.Code != http.StatusOK {
		t.Fatalf("RequestCertificate status=%d body=%q", req.Code, req.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(req.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	arn, _ := created["CertificateArn"].(string)
	if arn == "" {
		t.Fatalf("missing arn: %s", req.Body.String())
	}

	list := mustJSONTarget(t, handler, "CertificateManager.ListCertificates", "acm", map[string]any{}, now)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), arn) {
		t.Fatalf("ListCertificates status=%d body=%q", list.Code, list.Body.String())
	}

	del := mustJSONTarget(t, handler, "CertificateManager.DeleteCertificate", "acm", map[string]any{
		"CertificateArn": arn,
	}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteCertificate status=%d body=%q", del.Code, del.Body.String())
	}
}

func TestRoute53HostedZoneRecords(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustJSONTarget(t, handler, "AWSRoute53.CreateHostedZone", "route53", map[string]any{
		"Name":            "srv.example.com",
		"CallerReference": "srv-ref-1",
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateHostedZone status=%d body=%q", create.Code, create.Body.String())
	}
	var zoneResp map[string]any
	_ = json.Unmarshal(create.Body.Bytes(), &zoneResp)
	hz, _ := zoneResp["HostedZone"].(map[string]any)
	zoneID, _ := hz["Id"].(string)
	zoneID = strings.TrimPrefix(zoneID, "/hostedzone/")

	change := mustJSONTarget(t, handler, "AWSRoute53.ChangeResourceRecordSets", "route53", map[string]any{
		"HostedZoneId": zoneID,
		"ChangeBatch": map[string]any{
			"Changes": []map[string]any{{
				"Action": "CREATE",
				"ResourceRecordSet": map[string]any{
					"Name": "www.srv.example.com",
					"Type": "A",
					"TTL":  60,
					"ResourceRecords": []map[string]any{
						{"Value": "127.0.0.1"},
					},
				},
			}},
		},
	}, now)
	if change.Code != http.StatusOK {
		t.Fatalf("ChangeResourceRecordSets status=%d body=%q", change.Code, change.Body.String())
	}

	list := mustJSONTarget(t, handler, "AWSRoute53.ListResourceRecordSets", "route53", map[string]any{
		"HostedZoneId": zoneID,
	}, now)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "127.0.0.1") {
		t.Fatalf("ListResourceRecordSets status=%d body=%q", list.Code, list.Body.String())
	}
}

func TestServiceDiscoveryDiscover(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	ns := mustJSONTarget(t, handler, "Route53AutoNaming_v20170314.CreatePrivateDnsNamespace", "servicediscovery", map[string]any{
		"Name": "srv.local",
	}, now)
	if ns.Code != http.StatusOK {
		t.Fatalf("CreatePrivateDnsNamespace status=%d body=%q", ns.Code, ns.Body.String())
	}
	var nsResp map[string]any
	_ = json.Unmarshal(ns.Body.Bytes(), &nsResp)
	namespace, _ := nsResp["Namespace"].(map[string]any)
	nsID, _ := namespace["Id"].(string)

	svc := mustJSONTarget(t, handler, "Route53AutoNaming_v20170314.CreateService", "servicediscovery", map[string]any{
		"Name":        "web",
		"NamespaceId": nsID,
	}, now)
	if svc.Code != http.StatusOK {
		t.Fatalf("CreateService status=%d body=%q", svc.Code, svc.Body.String())
	}
	var svcResp map[string]any
	_ = json.Unmarshal(svc.Body.Bytes(), &svcResp)
	service, _ := svcResp["Service"].(map[string]any)
	svcID, _ := service["Id"].(string)

	reg := mustJSONTarget(t, handler, "Route53AutoNaming_v20170314.RegisterInstance", "servicediscovery", map[string]any{
		"ServiceId":  svcID,
		"InstanceId": "i-web-1",
		"Attributes": map[string]any{"AWS_INSTANCE_IPV4": "10.0.0.9"},
	}, now)
	if reg.Code != http.StatusOK {
		t.Fatalf("RegisterInstance status=%d body=%q", reg.Code, reg.Body.String())
	}

	disc := mustJSONTarget(t, handler, "Route53AutoNaming_v20170314.DiscoverInstances", "servicediscovery", map[string]any{
		"NamespaceName": "srv.local",
		"ServiceName":   "web",
	}, now)
	if disc.Code != http.StatusOK || !strings.Contains(disc.Body.String(), "10.0.0.9") {
		t.Fatalf("DiscoverInstances status=%d body=%q", disc.Code, disc.Body.String())
	}
}

func TestPricingGetProducts(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	desc := mustJSONTarget(t, handler, "AWSPriceListService.DescribeServices", "pricing", map[string]any{}, now)
	if desc.Code != http.StatusOK || !strings.Contains(desc.Body.String(), "AmazonEC2") {
		t.Fatalf("DescribeServices status=%d body=%q", desc.Code, desc.Body.String())
	}

	prod := mustJSONTarget(t, handler, "AWSPriceListService.GetProducts", "pricing", map[string]any{
		"ServiceCode": "AmazonS3",
	}, now)
	if prod.Code != http.StatusOK || !strings.Contains(prod.Body.String(), "NOCTAXRIS-S3-STANDARD") {
		t.Fatalf("GetProducts status=%d body=%q", prod.Code, prod.Body.String())
	}
}

func TestAppSyncCreateAPIKeyAuthAndGraphQLComputeUnavailable(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustJSONTarget(t, handler, "AWSAppSync.CreateGraphqlApi", "appsync", map[string]any{
		"name":               "gql-lab",
		"authenticationType": "API_KEY",
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateGraphqlApi status=%d body=%q", create.Code, create.Body.String())
	}
	var apiResp map[string]any
	_ = json.Unmarshal(create.Body.Bytes(), &apiResp)
	gql, _ := apiResp["graphqlApi"].(map[string]any)
	apiID, _ := gql["apiId"].(string)

	reject := mustJSONTarget(t, handler, "AWSAppSync.CreateGraphqlApi", "appsync", map[string]any{
		"name":               "cognito-nope",
		"authenticationType": "AMAZON_COGNITO_USER_POOLS",
	}, now)
	if reject.Code == http.StatusOK {
		t.Fatalf("expected Cognito reject, got %s", reject.Body.String())
	}

	schema := mustJSONTarget(t, handler, "AWSAppSync.StartSchemaCreation", "appsync", map[string]any{
		"apiId":      apiID,
		"definition": "type Query { hello: String }",
	}, now)
	if schema.Code != http.StatusOK {
		t.Fatalf("StartSchemaCreation status=%d body=%q", schema.Code, schema.Body.String())
	}

	keyRec := mustJSONTarget(t, handler, "AWSAppSync.CreateApiKey", "appsync", map[string]any{
		"apiId": apiID,
	}, now)
	if keyRec.Code != http.StatusOK {
		t.Fatalf("CreateApiKey status=%d body=%q", keyRec.Code, keyRec.Body.String())
	}
	var keyResp map[string]any
	_ = json.Unmarshal(keyRec.Body.Bytes(), &keyResp)
	apiKeyObj, _ := keyResp["apiKey"].(map[string]any)
	apiKey, _ := apiKeyObj["id"].(string)
	if !strings.HasPrefix(apiKey, "da2-") {
		t.Fatalf("api key id=%q", apiKey)
	}

	ds := mustJSONTarget(t, handler, "AWSAppSync.CreateDataSource", "appsync", map[string]any{
		"apiId": apiID,
		"name":  "HelloDS",
		"type":  "AWS_LAMBDA",
		"lambdaConfig": map[string]any{
			"lambdaFunctionArn": "arn:aws:lambda:us-east-1:" + testAccountID + ":function:appsync-hello",
		},
	}, now)
	if ds.Code != http.StatusOK {
		t.Fatalf("CreateDataSource status=%d body=%q", ds.Code, ds.Body.String())
	}
	res := mustJSONTarget(t, handler, "AWSAppSync.CreateResolver", "appsync", map[string]any{
		"apiId":          apiID,
		"typeName":       "Query",
		"fieldName":      "hello",
		"dataSourceName": "HelloDS",
	}, now)
	if res.Code != http.StatusOK {
		t.Fatalf("CreateResolver status=%d body=%q", res.Code, res.Body.String())
	}

	// Lambda function missing: GraphQL returns an errors envelope.
	body, _ := json.Marshal(map[string]any{"query": "{ hello }"})
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/appsync/"+apiID+"/graphql", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest && rec.Code != http.StatusOK {
		t.Fatalf("graphql status=%d body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "errors") && !strings.Contains(rec.Body.String(), "not found") {
		t.Fatalf("expected graphql error body=%q", rec.Body.String())
	}
}

func TestAppSyncIAMGraphqlUnauthorizedWithoutSigV4(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustJSONTarget(t, handler, "AWSAppSync.CreateGraphqlApi", "appsync", map[string]any{
		"name":               "iam-gql",
		"authenticationType": "AWS_IAM",
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateGraphqlApi status=%d body=%q", create.Code, create.Body.String())
	}
	var apiResp map[string]any
	_ = json.Unmarshal(create.Body.Bytes(), &apiResp)
	gql, _ := apiResp["graphqlApi"].(map[string]any)
	apiID, _ := gql["apiId"].(string)

	body, _ := json.Marshal(map[string]any{"query": "{ hello }"})
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/appsync/"+apiID+"/graphql", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 without SigV4, got %d body=%q", rec.Code, rec.Body.String())
	}
}
