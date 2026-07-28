package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func mustCFNForm(t *testing.T, handler http.Handler, values url.Values, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	body := []byte(values.Encode())
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	signHeader(t, req, body, testAccessKey, testSecret, testRegion, "cloudformation", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestCloudFormationCreateDescribeDelete(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	tpl := `{"Resources":{"B":{"Type":"AWS::S3::Bucket","Properties":{"BucketName":"cfn-srv-bucket-1"}}}}`
	create := mustCFNForm(t, handler, url.Values{
		"Action":       {"CreateStack"},
		"Version":      {"2010-05-15"},
		"StackName":    {"srv-stack"},
		"TemplateBody": {tpl},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateStack status=%d body=%q", create.Code, create.Body.String())
	}
	if !strings.Contains(create.Body.String(), "StackId") {
		t.Fatalf("missing StackId: %s", create.Body.String())
	}

	desc := mustCFNForm(t, handler, url.Values{
		"Action":    {"DescribeStacks"},
		"Version":   {"2010-05-15"},
		"StackName": {"srv-stack"},
	}, now)
	if desc.Code != http.StatusOK || !strings.Contains(desc.Body.String(), "CREATE_COMPLETE") {
		t.Fatalf("DescribeStacks status=%d body=%q", desc.Code, desc.Body.String())
	}

	del := mustCFNForm(t, handler, url.Values{
		"Action":    {"DeleteStack"},
		"Version":   {"2010-05-15"},
		"StackName": {"srv-stack"},
	}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteStack status=%d body=%q", del.Code, del.Body.String())
	}
}

func mustJSONTarget(t *testing.T, handler http.Handler, target, service string, payload map[string]any, now time.Time) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", raw)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", target)
	signHeader(t, req, raw, testAccessKey, testSecret, testRegion, service, now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestGlueCreateGetTable(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createDB := mustJSONTarget(t, handler, "AWSGlue.CreateDatabase", "glue", map[string]any{
		"DatabaseInput": map[string]any{"Name": "labdb"},
	}, now)
	if createDB.Code != http.StatusOK {
		t.Fatalf("CreateDatabase status=%d body=%q", createDB.Code, createDB.Body.String())
	}

	createTbl := mustJSONTarget(t, handler, "AWSGlue.CreateTable", "glue", map[string]any{
		"DatabaseName": "labdb",
		"TableInput": map[string]any{
			"Name": "t1",
			"StorageDescriptor": map[string]any{
				"Location": "s3://b/p",
				"Columns":  []map[string]any{{"Name": "id", "Type": "string"}},
			},
		},
	}, now)
	if createTbl.Code != http.StatusOK {
		t.Fatalf("CreateTable status=%d body=%q", createTbl.Code, createTbl.Body.String())
	}

	get := mustJSONTarget(t, handler, "AWSGlue.GetTable", "glue", map[string]any{
		"DatabaseName": "labdb", "Name": "t1",
	}, now)
	if get.Code != http.StatusOK {
		t.Fatalf("GetTable status=%d body=%q", get.Code, get.Body.String())
	}
}

func TestCodePipelineStart(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	const cbTrust = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"codebuild.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	mustCreateIAMRole(t, handler, "cp-cb-role", cbTrust, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/cp-cb-role"

	createProj := mustCodeBuildJSON(t, handler, "CreateProject", map[string]any{
		"name":        "proj",
		"serviceRole": roleARN,
		"source": map[string]any{
			"type":      "NO_SOURCE",
			"buildspec": "version: 0.2\nphases:\n  build:\n    commands:\n      - echo ok\n",
		},
		"environment": map[string]any{
			"type":        "LINUX_CONTAINER",
			"image":       "alpine:3.20",
			"computeType": "BUILD_GENERAL1_SMALL",
		},
		"artifacts": map[string]any{"type": "NO_ARTIFACTS"},
	}, now)
	if createProj.Code != http.StatusOK {
		t.Fatalf("CreateProject status=%d body=%q", createProj.Code, createProj.Body.String())
	}

	create := mustJSONTarget(t, handler, "CodePipeline_20150709.CreatePipeline", "codepipeline", map[string]any{
		"pipeline": map[string]any{
			"name": "lab-pipe",
			"stages": []map[string]any{{
				"name": "Build",
				"actions": []map[string]any{{
					"name": "Build",
					"actionTypeId": map[string]any{
						"category": "Build", "owner": "AWS", "provider": "CodeBuild", "version": "1",
					},
					"configuration": map[string]string{"ProjectName": "proj"},
				}},
			}},
		},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreatePipeline status=%d body=%q", create.Code, create.Body.String())
	}

	start := mustJSONTarget(t, handler, "CodePipeline_20150709.StartPipelineExecution", "codepipeline", map[string]any{
		"name": "lab-pipe",
	}, now)
	if start.Code != http.StatusOK {
		t.Fatalf("StartPipelineExecution status=%d body=%q", start.Code, start.Body.String())
	}
	if !strings.Contains(start.Body.String(), "pipelineExecutionId") {
		t.Fatalf("missing pipelineExecutionId in %q", start.Body.String())
	}

	list := mustCodeBuildJSON(t, handler, "ListBuilds", map[string]any{"projectName": "proj"}, now)
	if list.Code != http.StatusOK {
		t.Fatalf("ListBuilds status=%d body=%q", list.Code, list.Body.String())
	}
	var listOut map[string]any
	_ = json.Unmarshal(list.Body.Bytes(), &listOut)
	ids, _ := listOut["ids"].([]any)
	if len(ids) < 1 {
		t.Fatalf("expected at least one CodeBuild build from pipeline execution, got %q", list.Body.String())
	}
}

func TestFirehosePutRecord(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)
	if _, err := st.CreateBucket(testAccountID, "fh-srv-bucket"); err != nil {
		t.Fatal(err)
	}
	policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"firehose.amazonaws.com"},"Action":"s3:PutObject","Resource":"*"}]}`
	if err := st.PutBucketPolicy(testAccountID, "fh-srv-bucket", policy); err != nil {
		t.Fatal(err)
	}

	create := mustJSONTarget(t, handler, "Firehose_20150804.CreateDeliveryStream", "firehose", map[string]any{
		"DeliveryStreamName": "lab-fh",
		"S3DestinationConfiguration": map[string]any{
			"BucketARN": "arn:aws:s3:::fh-srv-bucket",
			"Prefix":    "data/",
		},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateDeliveryStream status=%d body=%q", create.Code, create.Body.String())
	}

	put := mustJSONTarget(t, handler, "Firehose_20150804.PutRecord", "firehose", map[string]any{
		"DeliveryStreamName": "lab-fh",
		"Record":             map[string]any{"Data": "aGVsbG8="},
	}, now)
	if put.Code != http.StatusOK {
		t.Fatalf("PutRecord status=%d body=%q", put.Code, put.Body.String())
	}
}

func TestWAFCreateAndEvaluate(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	create := mustJSONTarget(t, handler, "AWSWAF_20190729.CreateWebACL", "wafv2", map[string]any{
		"Name": "lab-acl", "Scope": "REGIONAL",
		"DefaultAction": map[string]any{"Allow": map[string]any{}},
		"Rules": []map[string]any{{
			"Name": "block-x", "Priority": 1, "Action": map[string]any{"Block": map[string]any{}}, "Label": "x",
		}},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateWebACL status=%d body=%q", create.Code, create.Body.String())
	}
	var out map[string]any
	_ = json.Unmarshal(create.Body.Bytes(), &out)
	sum, _ := out["Summary"].(map[string]any)
	arn, _ := sum["ARN"].(string)

	eval := mustJSONTarget(t, handler, "AWSWAF_20190729.Evaluate", "wafv2", map[string]any{
		"WebACLArn": arn, "Label": "x",
	}, now)
	if eval.Code != http.StatusOK || !strings.Contains(eval.Body.String(), "Block") {
		t.Fatalf("Evaluate status=%d body=%q", eval.Code, eval.Body.String())
	}
}

func TestConfigRecorder(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	if _, err := st.CreateBucket(testAccountID, "config-lab"); err != nil {
		t.Fatal(err)
	}

	body := []byte(url.Values{
		"Action":                     {"PutConfigurationRecorder"},
		"Version":                    {"2014-11-12"},
		"ConfigurationRecorder.Name": {"default"},
	}.Encode())
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	signHeader(t, req, body, testAccessKey, testSecret, testRegion, "config", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PutConfigurationRecorder status=%d body=%q", rec.Code, rec.Body.String())
	}

	delivBody := []byte(url.Values{
		"Action":                       {"PutDeliveryChannel"},
		"Version":                      {"2014-11-12"},
		"DeliveryChannel.Name":         {"default"},
		"DeliveryChannel.s3BucketName": {"config-lab"},
	}.Encode())
	delivReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", delivBody)
	delivReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	signHeader(t, delivReq, delivBody, testAccessKey, testSecret, testRegion, "config", now)
	delivRec := httptest.NewRecorder()
	handler.ServeHTTP(delivRec, delivReq)
	if delivRec.Code != http.StatusOK {
		t.Fatalf("PutDeliveryChannel status=%d body=%q", delivRec.Code, delivRec.Body.String())
	}

	startBody := []byte(url.Values{
		"Action":                    {"StartConfigurationRecorder"},
		"Version":                   {"2014-11-12"},
		"ConfigurationRecorderName": {"default"},
	}.Encode())
	startReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", startBody)
	startReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	signHeader(t, startReq, startBody, testAccessKey, testSecret, testRegion, "config", now)
	startRec := httptest.NewRecorder()
	handler.ServeHTTP(startRec, startReq)
	if startRec.Code != http.StatusOK {
		t.Fatalf("StartConfigurationRecorder status=%d body=%q", startRec.Code, startRec.Body.String())
	}
	listed, err := st.ListObjectsV2(testAccountID, "config-lab", "AWSLogs/", "")
	if err != nil || len(listed.Contents) == 0 {
		t.Fatalf("list config history: %v %#v", err, listed)
	}
	var snapKey string
	for _, obj := range listed.Contents {
		if strings.Contains(obj.Key, "noctaxris-config-snapshot-default-") {
			snapKey = obj.Key
			break
		}
	}
	if snapKey == "" {
		t.Fatalf("missing snapshot object: %#v", listed.Contents)
	}
	if _, data, err := st.GetObject(testAccountID, "config-lab", snapKey); err != nil || len(data) == 0 {
		t.Fatalf("get snapshot: %v len=%d", err, len(data))
	}
}

func TestConfigGetResourceConfigHistory(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	if _, err := st.CreateBucket(testAccountID, "config-hist"); err != nil {
		t.Fatal(err)
	}
	putRec := []byte(url.Values{
		"Action":                     {"PutConfigurationRecorder"},
		"Version":                    {"2014-11-12"},
		"ConfigurationRecorder.Name": {"default"},
	}.Encode())
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", putRec)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	signHeader(t, req, putRec, testAccessKey, testSecret, testRegion, "config", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PutConfigurationRecorder status=%d body=%q", rec.Code, rec.Body.String())
	}

	delivBody := []byte(url.Values{
		"Action":                       {"PutDeliveryChannel"},
		"Version":                      {"2014-11-12"},
		"DeliveryChannel.Name":         {"default"},
		"DeliveryChannel.s3BucketName": {"config-hist"},
	}.Encode())
	delivReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", delivBody)
	delivReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	signHeader(t, delivReq, delivBody, testAccessKey, testSecret, testRegion, "config", now)
	delivRec := httptest.NewRecorder()
	handler.ServeHTTP(delivRec, delivReq)
	if delivRec.Code != http.StatusOK {
		t.Fatalf("PutDeliveryChannel status=%d body=%q", delivRec.Code, delivRec.Body.String())
	}

	startBody := []byte(url.Values{
		"Action":                    {"StartConfigurationRecorder"},
		"Version":                   {"2014-11-12"},
		"ConfigurationRecorderName": {"default"},
	}.Encode())
	startReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", startBody)
	startReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	signHeader(t, startReq, startBody, testAccessKey, testSecret, testRegion, "config", now)
	startRec := httptest.NewRecorder()
	handler.ServeHTTP(startRec, startReq)
	if startRec.Code != http.StatusOK {
		t.Fatalf("StartConfigurationRecorder status=%d body=%q", startRec.Code, startRec.Body.String())
	}

	bucketReq := mustNewRequest(t, http.MethodPut, "http://127.0.0.1:4566/config-tracked", []byte{})
	signHeader(t, bucketReq, []byte{}, testAccessKey, testSecret, testRegion, "s3", now)
	bucketRec := httptest.NewRecorder()
	handler.ServeHTTP(bucketRec, bucketReq)
	if bucketRec.Code != http.StatusOK {
		t.Fatalf("CreateBucket status=%d body=%q", bucketRec.Code, bucketRec.Body.String())
	}

	histBody := []byte(url.Values{
		"Action":       {"GetResourceConfigHistory"},
		"Version":      {"2014-11-12"},
		"resourceType": {"AWS::S3::Bucket"},
		"resourceId":   {"config-tracked"},
	}.Encode())
	histReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", histBody)
	histReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	signHeader(t, histReq, histBody, testAccessKey, testSecret, testRegion, "config", now)
	histRec := httptest.NewRecorder()
	handler.ServeHTTP(histRec, histReq)
	if histRec.Code != http.StatusOK {
		t.Fatalf("GetResourceConfigHistory status=%d body=%q", histRec.Code, histRec.Body.String())
	}
	body := histRec.Body.String()
	if !strings.Contains(body, "config-tracked") || !strings.Contains(body, "<configurationItemStatus>OK</configurationItemStatus>") {
		t.Fatalf("unexpected history XML: %q", body)
	}
}

func TestConfigGetResourceConfigHistoryObjectPutDelete(t *testing.T) {
	srv, _, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/config-obj-delivery", nil, "s3", now, nil)
	mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/config-obj-data", nil, "s3", now, nil)

	putRec := []byte(url.Values{
		"Action":                     {"PutConfigurationRecorder"},
		"Version":                    {"2014-11-12"},
		"ConfigurationRecorder.Name": {"default"},
	}.Encode())
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", putRec)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	signHeader(t, req, putRec, testAccessKey, testSecret, testRegion, "config", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PutConfigurationRecorder status=%d body=%q", rec.Code, rec.Body.String())
	}

	delivBody := []byte(url.Values{
		"Action":                       {"PutDeliveryChannel"},
		"Version":                      {"2014-11-12"},
		"DeliveryChannel.Name":         {"default"},
		"DeliveryChannel.s3BucketName": {"config-obj-delivery"},
	}.Encode())
	delivReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", delivBody)
	delivReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	signHeader(t, delivReq, delivBody, testAccessKey, testSecret, testRegion, "config", now)
	delivRec := httptest.NewRecorder()
	handler.ServeHTTP(delivRec, delivReq)
	if delivRec.Code != http.StatusOK {
		t.Fatalf("PutDeliveryChannel status=%d body=%q", delivRec.Code, delivRec.Body.String())
	}

	startBody := []byte(url.Values{
		"Action":                    {"StartConfigurationRecorder"},
		"Version":                   {"2014-11-12"},
		"ConfigurationRecorderName": {"default"},
	}.Encode())
	startReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", startBody)
	startReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	signHeader(t, startReq, startBody, testAccessKey, testSecret, testRegion, "config", now)
	startRec := httptest.NewRecorder()
	handler.ServeHTTP(startRec, startReq)
	if startRec.Code != http.StatusOK {
		t.Fatalf("StartConfigurationRecorder status=%d body=%q", startRec.Code, startRec.Body.String())
	}

	payload := []byte("config-object-body")
	putObj := mustS3(t, handler, http.MethodPut, "http://127.0.0.1:4566/config-obj-data/inbox/item.txt", payload, "s3", now, map[string]string{
		"Content-Type": "text/plain",
	})
	if putObj.Code != http.StatusOK {
		t.Fatalf("PutObject status=%d body=%q", putObj.Code, putObj.Body.String())
	}

	resourceID := "config-obj-data/inbox/item.txt"
	histBody := []byte(url.Values{
		"Action":       {"GetResourceConfigHistory"},
		"Version":      {"2014-11-12"},
		"resourceType": {"AWS::S3::Object"},
		"resourceId":   {resourceID},
	}.Encode())
	histReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", histBody)
	histReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	signHeader(t, histReq, histBody, testAccessKey, testSecret, testRegion, "config", now)
	histRec := httptest.NewRecorder()
	handler.ServeHTTP(histRec, histReq)
	if histRec.Code != http.StatusOK {
		t.Fatalf("GetResourceConfigHistory after put status=%d body=%q", histRec.Code, histRec.Body.String())
	}
	hist := histRec.Body.String()
	if !strings.Contains(hist, resourceID) || !strings.Contains(hist, "<configurationItemStatus>OK</configurationItemStatus>") {
		t.Fatalf("unexpected put history XML: %q", hist)
	}

	delObj := mustS3(t, handler, http.MethodDelete, "http://127.0.0.1:4566/config-obj-data/inbox/item.txt", nil, "s3", now, nil)
	if delObj.Code != http.StatusNoContent {
		t.Fatalf("DeleteObject status=%d body=%q", delObj.Code, delObj.Body.String())
	}

	histBody2 := []byte(url.Values{
		"Action":       {"GetResourceConfigHistory"},
		"Version":      {"2014-11-12"},
		"resourceType": {"AWS::S3::Object"},
		"resourceId":   {resourceID},
	}.Encode())
	histReq2 := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", histBody2)
	histReq2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	signHeader(t, histReq2, histBody2, testAccessKey, testSecret, testRegion, "config", now)
	histRec2 := httptest.NewRecorder()
	handler.ServeHTTP(histRec2, histReq2)
	if histRec2.Code != http.StatusOK {
		t.Fatalf("GetResourceConfigHistory after delete status=%d body=%q", histRec2.Code, histRec2.Body.String())
	}
	hist2 := histRec2.Body.String()
	if !strings.Contains(hist2, "<configurationItemStatus>ResourceDeleted</configurationItemStatus>") {
		t.Fatalf("unexpected delete history XML: %q", hist2)
	}
}
