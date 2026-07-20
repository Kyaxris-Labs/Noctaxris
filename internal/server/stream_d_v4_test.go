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
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

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

	startBody := []byte(url.Values{
		"Action":                     {"StartConfigurationRecorder"},
		"Version":                    {"2014-11-12"},
		"ConfigurationRecorderName":  {"default"},
	}.Encode())
	startReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", startBody)
	startReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	signHeader(t, startReq, startBody, testAccessKey, testSecret, testRegion, "config", now)
	startRec := httptest.NewRecorder()
	handler.ServeHTTP(startRec, startReq)
	if startRec.Code != http.StatusOK {
		t.Fatalf("StartConfigurationRecorder status=%d body=%q", startRec.Code, startRec.Body.String())
	}
}
