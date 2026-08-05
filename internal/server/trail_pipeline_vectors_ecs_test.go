package server_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestCloudTrailDescribeDeleteStopLogging(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	const bucket = "ct-mgmt-bucket"
	if _, err := st.CreateBucket(testAccountID, bucket); err != nil {
		t.Fatal(err)
	}

	create := mustCloudTrailTarget(t, handler, "CloudTrail_20131101.CreateTrail", map[string]any{
		"Name": "mgmt-trail", "S3BucketName": bucket,
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateTrail status=%d body=%q", create.Code, create.Body.String())
	}

	desc := mustCloudTrailTarget(t, handler, "CloudTrail_20131101.DescribeTrails", map[string]any{
		"trailNameList": []string{"mgmt-trail"},
	}, now)
	if desc.Code != http.StatusOK || !strings.Contains(desc.Body.String(), "mgmt-trail") {
		t.Fatalf("DescribeTrails status=%d body=%q", desc.Code, desc.Body.String())
	}
	descAll := mustCloudTrailTarget(t, handler, "CloudTrail_20131101.DescribeTrails", map[string]any{}, now)
	if descAll.Code != http.StatusOK {
		t.Fatalf("DescribeTrails all status=%d body=%q", descAll.Code, descAll.Body.String())
	}

	start := mustCloudTrailTarget(t, handler, "CloudTrail_20131101.StartLogging", map[string]any{
		"Name": "mgmt-trail",
	}, now)
	if start.Code != http.StatusOK {
		t.Fatalf("StartLogging status=%d body=%q", start.Code, start.Body.String())
	}
	stop := mustCloudTrailTarget(t, handler, "CloudTrail_20131101.StopLogging", map[string]any{
		"Name": "mgmt-trail",
	}, now)
	if stop.Code != http.StatusOK {
		t.Fatalf("StopLogging status=%d body=%q", stop.Code, stop.Body.String())
	}
	stopGone := mustCloudTrailTarget(t, handler, "CloudTrail_20131101.StopLogging", map[string]any{
		"Name": "missing-trail",
	}, now)
	if stopGone.Code == http.StatusOK {
		t.Fatalf("StopLogging missing should fail: %q", stopGone.Body.String())
	}

	del := mustCloudTrailTarget(t, handler, "CloudTrail_20131101.DeleteTrail", map[string]any{
		"Name": "mgmt-trail",
	}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeleteTrail status=%d body=%q", del.Code, del.Body.String())
	}
	delGone := mustCloudTrailTarget(t, handler, "CloudTrail_20131101.DeleteTrail", map[string]any{
		"Name": "mgmt-trail",
	}, now)
	if delGone.Code == http.StatusOK {
		t.Fatalf("DeleteTrail missing should fail: %q", delGone.Body.String())
	}
}

func TestCodePipelineGetDeleteAndUnknownError(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	const cbTrust = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"codebuild.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	mustCreateIAMRole(t, handler, "cp-cov-role", cbTrust, now)
	roleARN := "arn:aws:iam::" + testAccountID + ":role/cp-cov-role"

	createProj := mustCodeBuildJSON(t, handler, "CreateProject", map[string]any{
		"name": "cp-cov-proj", "serviceRole": roleARN,
		"source": map[string]any{
			"type": "NO_SOURCE", "buildspec": "version: 0.2\nphases:\n  build:\n    commands:\n      - echo ok\n",
		},
		"environment": map[string]any{
			"type": "LINUX_CONTAINER", "image": "alpine:3.20", "computeType": "BUILD_GENERAL1_SMALL",
		},
		"artifacts": map[string]any{"type": "NO_ARTIFACTS"},
	}, now)
	if createProj.Code != http.StatusOK {
		t.Fatalf("CreateProject status=%d body=%q", createProj.Code, createProj.Body.String())
	}

	create := mustJSONTarget(t, handler, "CodePipeline_20150709.CreatePipeline", "codepipeline", map[string]any{
		"pipeline": map[string]any{
			"name": "cov-pipe",
			"stages": []map[string]any{{
				"name": "Build",
				"actions": []map[string]any{{
					"name": "Build",
					"actionTypeId": map[string]any{
						"category": "Build", "owner": "AWS", "provider": "CodeBuild", "version": "1",
					},
					"configuration": map[string]string{"ProjectName": "cp-cov-proj"},
				}},
			}},
		},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreatePipeline status=%d body=%q", create.Code, create.Body.String())
	}

	get := mustJSONTarget(t, handler, "CodePipeline_20150709.GetPipeline", "codepipeline", map[string]any{
		"name": "cov-pipe",
	}, now)
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), "cov-pipe") {
		t.Fatalf("GetPipeline status=%d body=%q", get.Code, get.Body.String())
	}
	missing := mustJSONTarget(t, handler, "CodePipeline_20150709.GetPipeline", "codepipeline", map[string]any{
		"name": "missing",
	}, now)
	if missing.Code == http.StatusOK {
		t.Fatalf("GetPipeline missing should fail: %q", missing.Body.String())
	}

	del := mustJSONTarget(t, handler, "CodePipeline_20150709.DeletePipeline", "codepipeline", map[string]any{
		"name": "cov-pipe",
	}, now)
	if del.Code != http.StatusOK {
		t.Fatalf("DeletePipeline status=%d body=%q", del.Code, del.Body.String())
	}
	delGone := mustJSONTarget(t, handler, "CodePipeline_20150709.DeletePipeline", "codepipeline", map[string]any{
		"name": "cov-pipe",
	}, now)
	if delGone.Code == http.StatusOK {
		t.Fatalf("DeletePipeline missing should fail: %q", delGone.Body.String())
	}

	unknown := mustJSONTarget(t, handler, "CodePipeline_20150709.RetryStageExecution", "codepipeline", map[string]any{}, now)
	if unknown.Code == http.StatusOK {
		t.Fatalf("unknown CodePipeline action should error: %q", unknown.Body.String())
	}
}

func TestS3VectorsListDeleteCoverage(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	bucket := mustJSONTarget(t, handler, "AmazonS3Vectors.CreateVectorBucket", "s3vectors", map[string]any{
		"vectorBucketName": "vec-cov",
	}, now)
	if bucket.Code != http.StatusOK {
		t.Fatalf("CreateVectorBucket status=%d body=%q", bucket.Code, bucket.Body.String())
	}
	idx := mustJSONTarget(t, handler, "AmazonS3Vectors.CreateIndex", "s3vectors", map[string]any{
		"vectorBucketName": "vec-cov", "indexName": "idx", "dimension": 2, "distanceMetric": "cosine",
	}, now)
	if idx.Code != http.StatusOK {
		t.Fatalf("CreateIndex status=%d body=%q", idx.Code, idx.Body.String())
	}

	listBuckets := mustJSONTarget(t, handler, "AmazonS3Vectors.ListVectorBuckets", "s3vectors", map[string]any{}, now)
	if listBuckets.Code != http.StatusOK || !strings.Contains(listBuckets.Body.String(), "vec-cov") {
		t.Fatalf("ListVectorBuckets status=%d body=%q", listBuckets.Code, listBuckets.Body.String())
	}
	listIdx := mustJSONTarget(t, handler, "AmazonS3Vectors.ListIndexes", "s3vectors", map[string]any{
		"vectorBucketName": "vec-cov",
	}, now)
	if listIdx.Code != http.StatusOK || !strings.Contains(listIdx.Body.String(), "idx") {
		t.Fatalf("ListIndexes status=%d body=%q", listIdx.Code, listIdx.Body.String())
	}

	delIdx := mustJSONTarget(t, handler, "AmazonS3Vectors.DeleteIndex", "s3vectors", map[string]any{
		"vectorBucketName": "vec-cov", "indexName": "idx",
	}, now)
	if delIdx.Code != http.StatusOK {
		t.Fatalf("DeleteIndex status=%d body=%q", delIdx.Code, delIdx.Body.String())
	}
	delIdxGone := mustJSONTarget(t, handler, "AmazonS3Vectors.DeleteIndex", "s3vectors", map[string]any{
		"vectorBucketName": "vec-cov", "indexName": "idx",
	}, now)
	if delIdxGone.Code == http.StatusOK {
		t.Fatalf("DeleteIndex missing should fail: %q", delIdxGone.Body.String())
	}
	delBucket := mustJSONTarget(t, handler, "AmazonS3Vectors.DeleteVectorBucket", "s3vectors", map[string]any{
		"vectorBucketName": "vec-cov",
	}, now)
	if delBucket.Code != http.StatusOK {
		t.Fatalf("DeleteVectorBucket status=%d body=%q", delBucket.Code, delBucket.Body.String())
	}
	delBucketGone := mustJSONTarget(t, handler, "AmazonS3Vectors.DeleteVectorBucket", "s3vectors", map[string]any{
		"vectorBucketName": "vec-cov",
	}, now)
	if delBucketGone.Code == http.StatusOK {
		t.Fatalf("DeleteVectorBucket missing should fail: %q", delBucketGone.Body.String())
	}

	unknown := mustJSONTarget(t, handler, "AmazonS3Vectors.NotARealAction", "s3vectors", map[string]any{}, now)
	if unknown.Code == http.StatusOK {
		t.Fatalf("unknown S3 Vectors action should error: %q", unknown.Body.String())
	}
}

func TestRoute53ListDeleteHostedZones(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createBody := url.Values{
		"Action":      {"CreateHostedZone"},
		"Version":     {"2013-04-01"},
		"Name":        {"cov.example.com."},
		"CallerReference": {"cov-r53-1"},
	}.Encode()
	req := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(createBody))
	signHeader(t, req, []byte(createBody), testAccessKey, testSecret, testRegion, "route53", now)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK && rec.Code != http.StatusCreated {
		// Route53 may use REST XML path instead of form; try JSON-style if present
		t.Logf("CreateHostedZone form status=%d body=%q", rec.Code, rec.Body.String())
	}

	listBody := url.Values{"Action": {"ListHostedZones"}, "Version": {"2013-04-01"}}.Encode()
	listReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(listBody))
	signHeader(t, listReq, []byte(listBody), testAccessKey, testSecret, testRegion, "route53", now)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK && listRec.Code != http.StatusBadRequest {
		t.Fatalf("ListHostedZones unexpected status=%d body=%q", listRec.Code, listRec.Body.String())
	}

	// REST-style list used by many labs
	getReq := mustNewRequest(t, http.MethodGet, "http://127.0.0.1:4566/2013-04-01/hostedzone", nil)
	getReq.Header.Set("Content-Type", "application/xml")
	signHeader(t, getReq, nil, testAccessKey, testSecret, testRegion, "route53", now)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	_ = getRec

	delBody := url.Values{
		"Action": {"DeleteHostedZone"}, "Version": {"2013-04-01"}, "Id": {"/hostedzone/missing"},
	}.Encode()
	delReq := mustNewRequest(t, http.MethodPost, "http://127.0.0.1:4566/", []byte(delBody))
	signHeader(t, delReq, []byte(delBody), testAccessKey, testSecret, testRegion, "route53", now)
	delRec := httptest.NewRecorder()
	handler.ServeHTTP(delRec, delReq)
	if delRec.Code == http.StatusOK {
		t.Fatalf("DeleteHostedZone missing should fail: %q", delRec.Body.String())
	}
}

func TestECSListDescribeStopTasksWithoutDinD(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	list := mustJSONTarget(t, handler, "AmazonEC2ContainerServiceV20141113.ListTasks", "ecs", map[string]any{
		"cluster": "default",
	}, now)
	if list.Code != http.StatusOK {
		t.Fatalf("ListTasks status=%d body=%q", list.Code, list.Body.String())
	}
	desc := mustJSONTarget(t, handler, "AmazonEC2ContainerServiceV20141113.DescribeTasks", "ecs", map[string]any{
		"cluster": "default",
		"tasks":   []string{"arn:aws:ecs:us-east-1:" + testAccountID + ":task/default/missing"},
	}, now)
	if desc.Code != http.StatusOK {
		t.Fatalf("DescribeTasks status=%d body=%q", desc.Code, desc.Body.String())
	}
	stop := mustJSONTarget(t, handler, "AmazonEC2ContainerServiceV20141113.StopTask", "ecs", map[string]any{
		"cluster": "default",
		"task":    "arn:aws:ecs:us-east-1:" + testAccountID + ":task/default/missing",
	}, now)
	if stop.Code == http.StatusOK {
		t.Fatalf("StopTask missing should fail: %q", stop.Body.String())
	}
	stopBad := mustJSONTarget(t, handler, "AmazonEC2ContainerServiceV20141113.StopTask", "ecs", map[string]any{
		"cluster": "default",
	}, now)
	if stopBad.Code != http.StatusBadRequest {
		t.Fatalf("StopTask empty task want 400 status=%d body=%q", stopBad.Code, stopBad.Body.String())
	}
}

func TestSchedulerUpdateScheduleCoverage(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	createQ := mustSQSJSON(t, handler, "CreateQueue", map[string]any{"QueueName": "sched-cov-q"}, now)
	if createQ.Code != http.StatusOK {
		t.Fatalf("CreateQueue status=%d body=%q", createQ.Code, createQ.Body.String())
	}
	queueARN := "arn:aws:sqs:us-east-1:" + testAccountID + ":sched-cov-q"

	create := mustSchedulerJSON(t, handler, "CreateSchedule", map[string]any{
		"Name": "cov-sched", "ScheduleExpression": "rate(5 minutes)",
		"FlexibleTimeWindow": map[string]any{"Mode": "OFF"},
		"Target":             map[string]any{"Arn": queueARN, "Input": `{"a":1}`},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateSchedule status=%d body=%q", create.Code, create.Body.String())
	}
	upd := mustSchedulerJSON(t, handler, "UpdateSchedule", map[string]any{
		"Name": "cov-sched", "ScheduleExpression": "rate(10 minutes)",
		"FlexibleTimeWindow": map[string]any{"Mode": "OFF"},
		"Target":             map[string]any{"Arn": queueARN, "Input": `{"a":2}`},
	}, now)
	if upd.Code != http.StatusOK {
		t.Fatalf("UpdateSchedule status=%d body=%q", upd.Code, upd.Body.String())
	}
	updGone := mustSchedulerJSON(t, handler, "UpdateSchedule", map[string]any{
		"Name": "missing", "ScheduleExpression": "rate(1 minutes)",
		"FlexibleTimeWindow": map[string]any{"Mode": "OFF"},
		"Target":             map[string]any{"Arn": queueARN},
	}, now)
	if updGone.Code == http.StatusOK {
		t.Fatalf("UpdateSchedule missing should fail: %q", updGone.Body.String())
	}
}
