package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGlueDatabaseTableCrawlerSchemaMgmt(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createDB := mustJSONTarget(t, handler, "AWSGlue.CreateDatabase", "glue", map[string]any{
		"DatabaseInput": map[string]any{"Name": "mgmt_db"},
	}, now)
	if createDB.Code != http.StatusOK {
		t.Fatalf("CreateDatabase status=%d body=%q", createDB.Code, createDB.Body.String())
	}
	getDB := mustJSONTarget(t, handler, "AWSGlue.GetDatabase", "glue", map[string]any{"Name": "mgmt_db"}, now)
	if getDB.Code != http.StatusOK || !strings.Contains(getDB.Body.String(), "mgmt_db") {
		t.Fatalf("GetDatabase status=%d body=%q", getDB.Code, getDB.Body.String())
	}
	getDBs := mustJSONTarget(t, handler, "AWSGlue.GetDatabases", "glue", map[string]any{}, now)
	if getDBs.Code != http.StatusOK || !strings.Contains(getDBs.Body.String(), "mgmt_db") {
		t.Fatalf("GetDatabases status=%d body=%q", getDBs.Code, getDBs.Body.String())
	}

	createTbl := mustJSONTarget(t, handler, "AWSGlue.CreateTable", "glue", map[string]any{
		"DatabaseName": "mgmt_db",
		"TableInput": map[string]any{
			"Name": "mgmt_tbl",
			"StorageDescriptor": map[string]any{
				"Columns": []map[string]any{{"Name": "id", "Type": "string"}},
				"Location": "s3://mgmt/tbl/",
			},
		},
	}, now)
	if createTbl.Code != http.StatusOK {
		t.Fatalf("CreateTable status=%d body=%q", createTbl.Code, createTbl.Body.String())
	}
	getTables := mustJSONTarget(t, handler, "AWSGlue.GetTables", "glue", map[string]any{"DatabaseName": "mgmt_db"}, now)
	if getTables.Code != http.StatusOK || !strings.Contains(getTables.Body.String(), "mgmt_tbl") {
		t.Fatalf("GetTables status=%d body=%q", getTables.Code, getTables.Body.String())
	}

	mustCreateIAMRole(t, handler, "glue-crawler-role", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"glue.amazonaws.com"},"Action":"sts:AssumeRole"}]}`, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/glue-crawler-role"
	createCrawler := mustJSONTarget(t, handler, "AWSGlue.CreateCrawler", "glue", map[string]any{
		"Name": "mgmt-crawler", "Role": roleARN, "DatabaseName": "mgmt_db",
		"Targets": map[string]any{"S3Targets": []map[string]any{{"Path": "s3://mgmt/src/"}}},
	}, now)
	if createCrawler.Code != http.StatusOK {
		t.Fatalf("CreateCrawler status=%d body=%q", createCrawler.Code, createCrawler.Body.String())
	}
	listCrawlers := mustJSONTarget(t, handler, "AWSGlue.ListCrawlers", "glue", map[string]any{}, now)
	if listCrawlers.Code != http.StatusOK || !strings.Contains(listCrawlers.Body.String(), "mgmt-crawler") {
		t.Fatalf("ListCrawlers status=%d body=%q", listCrawlers.Code, listCrawlers.Body.String())
	}

	createReg := mustJSONTarget(t, handler, "AWSGlue.CreateRegistry", "glue", map[string]any{
		"RegistryName": "mgmt-reg",
	}, now)
	if createReg.Code != http.StatusOK {
		t.Fatalf("CreateRegistry status=%d body=%q", createReg.Code, createReg.Body.String())
	}
	getReg := mustJSONTarget(t, handler, "AWSGlue.GetRegistry", "glue", map[string]any{
		"RegistryId": map[string]any{"RegistryName": "mgmt-reg"},
	}, now)
	if getReg.Code != http.StatusOK || !strings.Contains(getReg.Body.String(), "mgmt-reg") {
		t.Fatalf("GetRegistry status=%d body=%q", getReg.Code, getReg.Body.String())
	}
	createSchema := mustJSONTarget(t, handler, "AWSGlue.CreateSchema", "glue", map[string]any{
		"RegistryId":   map[string]any{"RegistryName": "mgmt-reg"},
		"SchemaName":   "mgmt-schema",
		"DataFormat":   "JSON",
		"SchemaDefinition": `{"type":"object","properties":{"id":{"type":"string"}}}`,
	}, now)
	if createSchema.Code != http.StatusOK {
		t.Fatalf("CreateSchema status=%d body=%q", createSchema.Code, createSchema.Body.String())
	}
	getSchema := mustJSONTarget(t, handler, "AWSGlue.GetSchema", "glue", map[string]any{
		"SchemaId": map[string]any{"RegistryName": "mgmt-reg", "SchemaName": "mgmt-schema"},
	}, now)
	if getSchema.Code != http.StatusOK {
		t.Fatalf("GetSchema status=%d body=%q", getSchema.Code, getSchema.Body.String())
	}
	listSchemas := mustJSONTarget(t, handler, "AWSGlue.ListSchemas", "glue", map[string]any{
		"RegistryId": map[string]any{"RegistryName": "mgmt-reg"},
	}, now)
	if listSchemas.Code != http.StatusOK || !strings.Contains(listSchemas.Body.String(), "mgmt-schema") {
		t.Fatalf("ListSchemas status=%d body=%q", listSchemas.Code, listSchemas.Body.String())
	}

	delSchema := mustJSONTarget(t, handler, "AWSGlue.DeleteSchema", "glue", map[string]any{
		"SchemaId": map[string]any{"RegistryName": "mgmt-reg", "SchemaName": "mgmt-schema"},
	}, now)
	if delSchema.Code != http.StatusOK {
		t.Fatalf("DeleteSchema status=%d body=%q", delSchema.Code, delSchema.Body.String())
	}
	delReg := mustJSONTarget(t, handler, "AWSGlue.DeleteRegistry", "glue", map[string]any{
		"RegistryId": map[string]any{"RegistryName": "mgmt-reg"},
	}, now)
	if delReg.Code != http.StatusOK {
		t.Fatalf("DeleteRegistry status=%d body=%q", delReg.Code, delReg.Body.String())
	}
	delCrawler := mustJSONTarget(t, handler, "AWSGlue.DeleteCrawler", "glue", map[string]any{"Name": "mgmt-crawler"}, now)
	if delCrawler.Code != http.StatusOK {
		t.Fatalf("DeleteCrawler status=%d body=%q", delCrawler.Code, delCrawler.Body.String())
	}
	delTbl := mustJSONTarget(t, handler, "AWSGlue.DeleteTable", "glue", map[string]any{
		"DatabaseName": "mgmt_db", "Name": "mgmt_tbl",
	}, now)
	if delTbl.Code != http.StatusOK {
		t.Fatalf("DeleteTable status=%d body=%q", delTbl.Code, delTbl.Body.String())
	}
	delDB := mustJSONTarget(t, handler, "AWSGlue.DeleteDatabase", "glue", map[string]any{"Name": "mgmt_db"}, now)
	if delDB.Code != http.StatusOK {
		t.Fatalf("DeleteDatabase status=%d body=%q", delDB.Code, delDB.Body.String())
	}
	missing := mustJSONTarget(t, handler, "AWSGlue.GetDatabase", "glue", map[string]any{"Name": "missing"}, now)
	if missing.Code == http.StatusOK {
		t.Fatalf("missing database should fail: %q", missing.Body.String())
	}

	_, denyARN, err := st.CreateUser(testAccountID, "glue-deny")
	if err != nil {
		t.Fatal(err)
	}
	denyAK, denySecret, err := st.CreateUserAccessKey(testAccountID, "glue-deny")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutInlinePolicy(denyARN, "deny", `{
		"Version":"2012-10-17",
		"Statement":[{"Effect":"Deny","Action":"glue:GetDatabases","Resource":"*"}]
	}`); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{})
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "AWSGlue.GetDatabases")
	signHeader(t, req, raw, denyAK, denySecret, testRegion, "glue", now)
	denyRec := httptest.NewRecorder()
	handler.ServeHTTP(denyRec, req)
	if denyRec.Code != http.StatusForbidden || !strings.Contains(denyRec.Body.String(), "AccessDeniedException") {
		t.Fatalf("authz deny status=%d body=%q", denyRec.Code, denyRec.Body.String())
	}
}

func TestIoTThingDescribeListUpdateDelete(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustJSONTarget(t, handler, "AWSIotService.CreateThing", "iot", map[string]any{
		"thingName": "mgmt-thing",
		"attributePayload": map[string]any{"attributes": map[string]string{"env": "lab"}},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateThing status=%d body=%q", create.Code, create.Body.String())
	}
	desc := mustJSONTarget(t, handler, "AWSIotService.DescribeThing", "iot", map[string]any{
		"thingName": "mgmt-thing",
	}, now)
	if desc.Code != http.StatusOK || !strings.Contains(desc.Body.String(), "mgmt-thing") {
		t.Fatalf("DescribeThing status=%d body=%q", desc.Code, desc.Body.String())
	}
	list := mustJSONTarget(t, handler, "AWSIotService.ListThings", "iot", map[string]any{}, now)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "mgmt-thing") {
		t.Fatalf("ListThings status=%d body=%q", list.Code, list.Body.String())
	}
	upd := mustJSONTarget(t, handler, "AWSIotService.UpdateThing", "iot", map[string]any{
		"thingName": "mgmt-thing",
		"attributePayload": map[string]any{"attributes": map[string]string{"env": "prod"}},
	}, now)
	if upd.Code != http.StatusOK {
		t.Fatalf("UpdateThing status=%d body=%q", upd.Code, upd.Body.String())
	}
	cert := mustJSONTarget(t, handler, "AWSIotService.CreateKeysAndCertificate", "iot", map[string]any{
		"setAsActive": true,
	}, now)
	if cert.Code != http.StatusOK {
		t.Fatalf("CreateKeysAndCertificate status=%d body=%q", cert.Code, cert.Body.String())
	}
	var certOut map[string]any
	_ = json.Unmarshal(cert.Body.Bytes(), &certOut)
	certID, _ := certOut["certificateId"].(string)
	descCert := mustJSONTarget(t, handler, "AWSIotService.DescribeCertificate", "iot", map[string]any{
		"certificateId": certID,
	}, now)
	if descCert.Code != http.StatusOK {
		t.Fatalf("DescribeCertificate status=%d body=%q", descCert.Code, descCert.Body.String())
	}
	del := mustJSONTarget(t, handler, "AWSIotService.DeleteThing", "iot", map[string]any{
		"thingName": "mgmt-thing",
	}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteThing status=%d body=%q", del.Code, del.Body.String())
	}
	missing := mustJSONTarget(t, handler, "AWSIotService.DescribeThing", "iot", map[string]any{
		"thingName": "missing",
	}, now)
	if missing.Code == http.StatusOK {
		t.Fatalf("missing thing should fail: %q", missing.Body.String())
	}

	_, denyARN, err := st.CreateUser(testAccountID, "iot-deny")
	if err != nil {
		t.Fatal(err)
	}
	denyAK, denySecret, err := st.CreateUserAccessKey(testAccountID, "iot-deny")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutInlinePolicy(denyARN, "deny", `{
		"Version":"2012-10-17",
		"Statement":[{"Effect":"Deny","Action":"iot:ListThings","Resource":"*"}]
	}`); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{})
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "AWSIotService.ListThings")
	signHeader(t, req, raw, denyAK, denySecret, testRegion, "iot", now)
	denyRec := httptest.NewRecorder()
	handler.ServeHTTP(denyRec, req)
	if denyRec.Code != http.StatusForbidden || !strings.Contains(denyRec.Body.String(), "AccessDeniedException") {
		t.Fatalf("authz deny status=%d body=%q", denyRec.Code, denyRec.Body.String())
	}
}

func TestWAFWebACLGetListUpdate(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustJSONTarget(t, handler, "AWSWAF_20190729.CreateWebACL", "wafv2", map[string]any{
		"Name": "mgmt-acl", "Scope": "REGIONAL",
		"DefaultAction": map[string]any{"Allow": map[string]any{}},
		"Rules":         []map[string]any{},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateWebACL status=%d body=%q", create.Code, create.Body.String())
	}
	var createOut map[string]any
	_ = json.Unmarshal(create.Body.Bytes(), &createOut)
	sum, _ := createOut["Summary"].(map[string]any)
	id, _ := sum["Id"].(string)
	lock, _ := sum["LockToken"].(string)
	if id == "" {
		t.Fatalf("CreateWebACL summary: %s", create.Body.String())
	}

	get := mustJSONTarget(t, handler, "AWSWAF_20190729.GetWebACL", "wafv2", map[string]any{
		"Name": "mgmt-acl", "Scope": "REGIONAL", "Id": id,
	}, now)
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), "mgmt-acl") {
		t.Fatalf("GetWebACL status=%d body=%q", get.Code, get.Body.String())
	}
	list := mustJSONTarget(t, handler, "AWSWAF_20190729.ListWebACLs", "wafv2", map[string]any{
		"Scope": "REGIONAL",
	}, now)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), id) {
		t.Fatalf("ListWebACLs status=%d body=%q", list.Code, list.Body.String())
	}
	upd := mustJSONTarget(t, handler, "AWSWAF_20190729.UpdateWebACL", "wafv2", map[string]any{
		"Name": "mgmt-acl", "Scope": "REGIONAL", "Id": id, "LockToken": lock,
		"DefaultAction": map[string]any{"Block": map[string]any{}},
		"Rules": []map[string]any{{
			"Name": "byte-match", "Priority": 1,
			"Action": map[string]any{"Block": map[string]any{}},
			"Statement": map[string]any{
				"ByteMatchStatement": map[string]any{
					"SearchString": "bad",
					"FieldToMatch": map[string]any{"UriPath": map[string]any{}},
					"TextTransformations": []map[string]any{{"Priority": 0, "Type": "NONE"}},
					"PositionalConstraint": "CONTAINS",
				},
			},
			"VisibilityConfig": map[string]any{
				"SampledRequestsEnabled": true, "CloudWatchMetricsEnabled": true, "MetricName": "byte",
			},
		}},
	}, now)
	if upd.Code != http.StatusOK {
		t.Fatalf("UpdateWebACL status=%d body=%q", upd.Code, upd.Body.String())
	}
	createRG := mustJSONTarget(t, handler, "AWSWAF_20190729.CreateRuleGroup", "wafv2", map[string]any{
		"Name": "mgmt-rg", "Scope": "REGIONAL", "Capacity": 10,
		"VisibilityConfig": map[string]any{
			"SampledRequestsEnabled": true, "CloudWatchMetricsEnabled": true, "MetricName": "rg",
		},
	}, now)
	if createRG.Code != http.StatusOK {
		t.Fatalf("CreateRuleGroup status=%d body=%q", createRG.Code, createRG.Body.String())
	}
}

func TestSSMGetParametersByPath(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	put := mustSSMJSON(t, handler, "PutParameter", map[string]any{
		"Name": "/mgmt/app/url", "Value": "https://example.local", "Type": "String",
	}, now)
	if put.Code != http.StatusOK {
		t.Fatalf("PutParameter status=%d body=%q", put.Code, put.Body.String())
	}
	byPath := mustSSMJSON(t, handler, "GetParametersByPath", map[string]any{
		"Path": "/mgmt", "Recursive": true,
	}, now)
	if byPath.Code != http.StatusOK || !strings.Contains(byPath.Body.String(), "/mgmt/app/url") {
		t.Fatalf("GetParametersByPath status=%d body=%q", byPath.Code, byPath.Body.String())
	}
	empty := mustSSMJSON(t, handler, "GetParametersByPath", map[string]any{}, now)
	if empty.Code == http.StatusOK {
		t.Fatalf("empty Path should fail: %q", empty.Body.String())
	}

	_, denyARN, err := st.CreateUser(testAccountID, "ssm-deny")
	if err != nil {
		t.Fatal(err)
	}
	denyAK, denySecret, err := st.CreateUserAccessKey(testAccountID, "ssm-deny")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutInlinePolicy(denyARN, "deny", `{
		"Version":"2012-10-17",
		"Statement":[{"Effect":"Deny","Action":"ssm:GetParametersByPath","Resource":"*"}]
	}`); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{"Path": "/mgmt", "Recursive": true})
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "AmazonSSM.GetParametersByPath")
	signHeader(t, req, raw, denyAK, denySecret, testRegion, "ssm", now)
	denyRec := httptest.NewRecorder()
	handler.ServeHTTP(denyRec, req)
	if denyRec.Code != http.StatusForbidden || !strings.Contains(denyRec.Body.String(), "AccessDeniedException") {
		t.Fatalf("authz deny status=%d body=%q", denyRec.Code, denyRec.Body.String())
	}
}

func TestSecretsRestoreSecret(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustSecretsJSON(t, handler, "CreateSecret", map[string]any{
		"Name": "restore-secret", "SecretString": "v1",
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateSecret status=%d body=%q", create.Code, create.Body.String())
	}
	del := mustSecretsJSON(t, handler, "DeleteSecret", map[string]any{
		"SecretId": "restore-secret", "RecoveryWindowInDays": 7,
	}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteSecret status=%d body=%q", del.Code, del.Body.String())
	}
	restore := mustSecretsJSON(t, handler, "RestoreSecret", map[string]any{"SecretId": "restore-secret"}, now)
	if restore.Code != http.StatusOK {
		t.Fatalf("RestoreSecret status=%d body=%q", restore.Code, restore.Body.String())
	}
	desc := mustSecretsJSON(t, handler, "DescribeSecret", map[string]any{"SecretId": "restore-secret"}, now)
	if desc.Code != http.StatusOK {
		t.Fatalf("DescribeSecret after restore status=%d body=%q", desc.Code, desc.Body.String())
	}
	missing := mustSecretsJSON(t, handler, "RestoreSecret", map[string]any{"SecretId": "no-such"}, now)
	if missing.Code == http.StatusOK {
		t.Fatalf("restore missing should fail: %q", missing.Body.String())
	}
}
